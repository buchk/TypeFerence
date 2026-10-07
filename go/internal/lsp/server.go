// Package lsp implements a Language Server Protocol server for version 7
// TypeFerence packages. Each open `.tfer` buffer gets the loader's
// single-document diagnostics, positioned at the reported line; on open and
// save it also gets the package's composition diagnostics. Completion offers
// each kind's fields, enumerated values, and the package-relative paths a
// reference field accepts; definition jumps from a path to its document.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// Server is a single-connection LSP server speaking JSON-RPC 2.0 over a stream.
type Server struct {
	version string
	r       *bufio.Reader
	w       io.Writer
	wmu     sync.Mutex
	docs    map[string]string // uri -> latest buffer text
	roots   []string          // workspace root paths
	index   map[string]string // resource id -> defining file uri
}

// NewServer returns a server that reports the given version to clients.
func NewServer(version string) *Server {
	return &Server{version: version, docs: map[string]string{}, index: map[string]string{}}
}

// message is a JSON-RPC 2.0 request, response, or notification.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Run serves requests from in and writes responses to out until the client
// sends `exit` or the stream closes.
func (s *Server) Run(in io.Reader, out io.Writer) error {
	s.r = bufio.NewReader(in)
	s.w = out
	for {
		msg, err := s.read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if s.dispatch(msg) {
			return nil
		}
	}
}

// read parses one Content-Length-framed JSON-RPC message.
func (s *Server) read() (*message, error) {
	var length int
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			fmt.Sscanf(strings.TrimSpace(line[len("content-length:"):]), "%d", &length)
		}
	}
	if length == 0 {
		return &message{}, nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(s.r, buf); err != nil {
		return nil, err
	}
	var msg message
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *Server) write(msg *message) {
	msg.JSONRPC = "2.0"
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	fmt.Fprintf(s.w, "Content-Length: %d\r\n\r\n", len(body))
	s.w.Write(body)
}

// dispatch handles one message and reports whether the server should exit.
func (s *Server) dispatch(msg *message) bool {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg.Params)
		s.reply(msg.ID, s.initializeResult())
	case "initialized":
		// no-op
	case "textDocument/didOpen":
		s.handleOpen(msg.Params)
	case "textDocument/didChange":
		s.handleChange(msg.Params)
	case "textDocument/didSave":
		s.handleSave(msg.Params)
	case "textDocument/didClose":
		s.handleClose(msg.Params)
	case "textDocument/completion":
		s.reply(msg.ID, s.handleCompletion(msg.Params))
	case "textDocument/definition":
		s.reply(msg.ID, s.handleDefinition(msg.Params))
	case "textDocument/documentSymbol":
		s.reply(msg.ID, s.handleDocumentSymbol(msg.Params))
	case "shutdown":
		s.reply(msg.ID, nil)
	case "exit":
		return true
	default:
		if len(msg.ID) > 0 {
			s.write(&message{ID: msg.ID, Error: &rpcError{Code: -32601, Message: "method not found: " + msg.Method}})
		}
	}
	return false
}

// handleInitialize captures workspace roots and indexes resource ids.
func (s *Server) handleInitialize(raw json.RawMessage) {
	var p struct {
		RootURI          string `json:"rootUri"`
		WorkspaceFolders []struct {
			URI string `json:"uri"`
		} `json:"workspaceFolders"`
	}
	json.Unmarshal(raw, &p)
	seen := map[string]bool{}
	add := func(uri string) {
		if uri == "" {
			return
		}
		path := uriToPath(uri)
		if !seen[path] {
			seen[path] = true
			s.roots = append(s.roots, path)
		}
	}
	add(p.RootURI)
	for _, f := range p.WorkspaceFolders {
		add(f.URI)
	}
	s.buildIndex()
}

// buildIndex maps every document identity under the workspace's version 7
// packages to its file uri, deriving identities from paths without parsing so
// a broken document still has an entry.
func (s *Server) buildIndex() {
	index := map[string]string{}
	for _, root := range s.roots {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "bin" || name == "obj") {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSource(path) || d.Name() == resource.ManifestFile {
				return nil
			}
			packageDir, project := packageRoot(path)
			if project == nil {
				return nil
			}
			rel := relativeTo(packageDir, path)
			if _, _, ok := resource.KindFromPath(rel); ok {
				index[resource.DeriveID(project.Name, project.Version, rel)] = pathToURI(path)
			}
			return nil
		})
	}
	s.index = index
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	raw := json.RawMessage("null")
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return
		}
		raw = b
	}
	s.write(&message{ID: id, Result: raw})
}

func (s *Server) initializeResult() any {
	return map[string]any{
		"capabilities": map[string]any{
			// 1 = full document sync: each change carries the whole buffer.
			"textDocumentSync":       1,
			"completionProvider":     map[string]any{},
			"definitionProvider":     true,
			"documentSymbolProvider": true,
		},
		"serverInfo": map[string]any{
			"name":    "typeference-lsp",
			"version": s.version,
		},
	}
}

type textDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

func (s *Server) handleOpen(raw json.RawMessage) {
	var p struct {
		TextDocument textDocumentItem `json:"textDocument"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	s.docs[p.TextDocument.URI] = p.TextDocument.Text
	s.publish(p.TextDocument.URI, p.TextDocument.Text, true)
}

func (s *Server) handleChange(raw json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if json.Unmarshal(raw, &p) != nil || len(p.ContentChanges) == 0 {
		return
	}
	text := p.ContentChanges[len(p.ContentChanges)-1].Text
	s.docs[p.TextDocument.URI] = text
	// Keystroke: fast single-file shape diagnostics only, no whole-workspace
	// resolution (it reads from disk and would be noisy mid-edit).
	s.publish(p.TextDocument.URI, text, false)
}

func (s *Server) handleSave(raw json.RawMessage) {
	uri := uriParam(raw)
	s.buildIndex() // disk changed; refresh id -> uri
	if text, ok := s.docs[uri]; ok {
		s.publish(uri, text, true)
	}
}

func (s *Server) handleClose(raw json.RawMessage) {
	uri := uriParam(raw)
	delete(s.docs, uri)
	s.publishDiagnostics(uri, nil)
}

func uriParam(raw json.RawMessage) string {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	json.Unmarshal(raw, &p)
	return p.TextDocument.URI
}

// publish validates one buffer and pushes its diagnostics. When withComposition
// is set it also reports the package's resolution errors that belong to this
// file (composition diagnostics read from disk, so they run on open/save).
func (s *Server) publish(uri, text string, withComposition bool) {
	path := uriToPath(uri)
	if !isSource(path) {
		return
	}
	var diags []diagnostic
	root, project := packageRoot(path)
	if project == nil {
		diags = append(diags, diagnostic{
			Range:    lineRange(text, 0),
			Severity: 1,
			Source:   "typeference",
			Message:  "not part of a version 7 package: no ancestor directory has a typeference.tfer declaring schemaVersion 7",
		})
		s.publishDiagnostics(uri, diags)
		return
	}
	rel := relativeTo(root, path)
	if err := resource.CheckDocument(rel, text); err != nil {
		line, message := locate(err.Error(), rel)
		diags = append(diags, diagnostic{
			Range:    lineRange(text, line),
			Severity: 1, // Error
			Source:   "typeference",
			Message:  message,
		})
	} else if withComposition {
		id := resource.DeriveID(project.Name, project.Version, rel)
		for _, m := range s.compositionErrorsFor(root, rel, id) {
			line, message := locate(m, rel)
			diags = append(diags, diagnostic{
				Range:    lineRange(text, line),
				Severity: 1,
				Source:   "typeference",
				Message:  message,
			})
		}
	}
	s.publishDiagnostics(uri, diags)
}

// compositionErrorsFor resolves the file's package from disk and returns the
// error to show on this file: one that names this file (by path or identity),
// or one that cannot be attributed to a different indexed document (so an
// unlocated package error still surfaces somewhere).
func (s *Server) compositionErrorsFor(root, rel, id string) []string {
	_, err := compile.Validate(root)
	if err == nil {
		return nil
	}
	m := err.Error()
	if strings.Contains(m, rel) || strings.Contains(m, id) || !s.attributableElsewhere(m, id) {
		return []string{m}
	}
	return nil
}

// attributableElsewhere reports whether a message names an indexed resource
// other than selfID (so it belongs on that resource's file, not this one).
func (s *Server) attributableElsewhere(msg, selfID string) bool {
	for otherID := range s.index {
		if otherID != selfID && strings.Contains(msg, otherID) {
			return true
		}
	}
	return false
}

// handleCompletion offers field names, enumerated values, and reference paths.
func (s *Server) handleCompletion(raw json.RawMessage) any {
	uri, line, char := positionParams(raw)
	path := uriToPath(uri)
	root, _ := packageRoot(path)
	items := []map[string]any{}
	for _, label := range completions(s.docs[uri], path, root, line, char) {
		items = append(items, map[string]any{"label": label})
	}
	return map[string]any{"isIncomplete": false, "items": items}
}

// handleDefinition jumps from a package-relative document path to its file.
func (s *Server) handleDefinition(raw json.RawMessage) any {
	uri, line, char := positionParams(raw)
	token := tokenAt(s.docs[uri], line, char)
	if token == "" || strings.Contains(token, ":") {
		return nil
	}
	root, _ := packageRoot(uriToPath(uri))
	if root == "" || resource.ValidSourcePath(token) != nil {
		return nil
	}
	target := filepath.Join(root, filepath.FromSlash(token))
	if _, err := os.Stat(target); err != nil {
		return nil
	}
	return map[string]any{
		"uri":   pathToURI(target),
		"range": rng{Start: position{0, 0}, End: position{0, 0}},
	}
}

// handleDocumentSymbol returns the document as a single symbol: its kind and
// the identity its path derives.
func (s *Server) handleDocumentSymbol(raw json.RawMessage) any {
	uri := uriParam(raw)
	path := uriToPath(uri)
	root, project := packageRoot(path)
	kind := documentKind(path)
	if project == nil || kind == "" {
		return []any{}
	}
	name := project.Name + "@" + project.Version
	if kind != manifestKind {
		name = kind + " " + resource.DeriveID(project.Name, project.Version, relativeTo(root, path))
	}
	// SymbolKind 5 = Class; a resource is the closest analogue.
	return []map[string]any{{
		"name":           name,
		"kind":           5,
		"range":          rng{Start: position{0, 0}, End: position{0, 0}},
		"selectionRange": rng{Start: position{0, 0}, End: position{0, 0}},
	}}
}

func positionParams(raw json.RawMessage) (uri string, line, char int) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	json.Unmarshal(raw, &p)
	return p.TextDocument.URI, p.Position.Line, p.Position.Character
}

func (s *Server) publishDiagnostics(uri string, diags []diagnostic) {
	if diags == nil {
		diags = []diagnostic{}
	}
	params, err := json.Marshal(map[string]any{
		"uri":         uri,
		"diagnostics": diags,
	})
	if err != nil {
		return
	}
	s.write(&message{Method: "textDocument/publishDiagnostics", Params: params})
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type rng struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type diagnostic struct {
	Range    rng    `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

// lineRange spans one zero-based line of the buffer.
func lineRange(text string, line int) rng {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		line = 0
	}
	end := len(strings.TrimRight(lines[line], "\r"))
	if end == 0 {
		end = 1
	}
	return rng{Start: position{line, 0}, End: position{line, end}}
}

var positioned = regexp.MustCompile(`^([^:\s]+\.tfer):(\d+): (.*)$`)

// locate strips a "path:line: " or "path: " prefix naming this document and
// returns the zero-based line the message points at.
func locate(message, rel string) (int, string) {
	if m := positioned.FindStringSubmatch(message); m != nil && m[1] == rel {
		n, _ := strconv.Atoi(m[2])
		return n - 1, m[3]
	}
	return 0, strings.TrimPrefix(message, rel+": ")
}

// pathToURI converts a local filesystem path to a file:// URI, adding the
// leading slash before a Windows drive letter (C:\x -> file:///C:/x).
func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && len(p) >= 2 && p[1] == ':' {
		p = "/" + p
	}
	// url.URL.String() percent-encodes the path, so spaces and other reserved
	// characters produce a valid file URI.
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

// uriToPath converts a file:// URI to a local filesystem path, handling the
// leading-slash Windows drive form (file:///C:/x -> C:\x).
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

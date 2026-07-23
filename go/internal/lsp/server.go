// Package lsp implements a minimal Language Server Protocol server for
// TypeFerence sources (`.tfer` and `.yaml`). It provides authoring diagnostics
// by running the loader's single-document shape validation on each open buffer.
//
// Scope: this is the v0 surface. It reports per-file syntax and shape errors —
// malformed frontmatter fences, unknown fields, bad kinds, skills that do not
// bind a capability, context objects without a contextType, and so on. It does
// not yet resolve across a source tree, so composition diagnostics (embedding
// ambiguity, unsatisfied interfaces, unresolved references) are a planned
// follow-up that requires whole-workspace resolution and source-root discovery.
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
	"runtime"
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

// buildIndex maps every resource id under the workspace roots to its file uri.
func (s *Server) buildIndex() {
	index := map[string]string{}
	for _, root := range s.roots {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !isSource(path) {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			if id, _ := symbolOf(string(raw)); id != "" {
				index[id] = pathToURI(path)
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
// is set it also reports whole-workspace resolution errors that reference this
// file (composition diagnostics read from disk, so they run on open/save).
func (s *Server) publish(uri, text string, withComposition bool) {
	path := uriToPath(uri)
	if !isSource(path) {
		return
	}
	var diags []diagnostic
	if err := resource.CheckDocument(path, text); err != nil {
		diags = append(diags, diagnostic{
			Range:    errorRange(text),
			Severity: 1, // Error
			Source:   "typeference",
			Message:  stripFilePrefix(err.Error(), filepath.Base(path)),
		})
	} else if withComposition {
		for _, m := range s.compositionErrorsFor(text, path) {
			diags = append(diags, diagnostic{
				Range:    errorRange(text),
				Severity: 1,
				Source:   "typeference",
				Message:  m,
			})
		}
	}
	s.publishDiagnostics(uri, diags)
}

// compositionErrorsFor resolves each workspace root from disk and returns the
// resolution errors to show on this file: those that name this file (by id or
// basename), plus any that cannot be attributed to a different indexed
// resource (so an unlocated workspace error still surfaces somewhere).
func (s *Server) compositionErrorsFor(text, path string) []string {
	id, _ := symbolOf(text)
	base := filepath.Base(path)
	seen := map[string]bool{}
	out := []string{}
	for _, root := range s.roots {
		_, err := compile.Validate(root, "")
		if err == nil {
			continue
		}
		m := err.Error()
		if seen[m] {
			continue
		}
		seen[m] = true
		if strings.Contains(m, base) || (id != "" && strings.Contains(m, id)) || !s.attributableElsewhere(m, id) {
			out = append(out, m)
		}
	}
	return out
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

// handleCompletion offers kind values and top-level field names.
func (s *Server) handleCompletion(raw json.RawMessage) any {
	uri, line, char := positionParams(raw)
	items := []map[string]any{}
	for _, label := range completions(s.docs[uri], line, char) {
		items = append(items, map[string]any{"label": label})
	}
	return map[string]any{"isIncomplete": false, "items": items}
}

// handleDefinition jumps from a resource-id token to its defining file.
func (s *Server) handleDefinition(raw json.RawMessage) any {
	uri, line, char := positionParams(raw)
	id := tokenAt(s.docs[uri], line, char)
	target, ok := s.index[id]
	if id == "" || !ok {
		return nil
	}
	return map[string]any{
		"uri":   target,
		"range": rng{Start: position{0, 0}, End: position{0, 0}},
	}
}

// handleDocumentSymbol returns the resource as a single symbol (id + kind).
func (s *Server) handleDocumentSymbol(raw json.RawMessage) any {
	uri := uriParam(raw)
	id, kind := symbolOf(s.docs[uri])
	if id == "" {
		return []any{}
	}
	name := id
	if kind != "" {
		name = kind + " " + id
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

// errorRange anchors a whole-document diagnostic to the first line. The loader
// reports one shape error per document without a position, so v0 highlights the
// opening line rather than guessing an offset.
func errorRange(text string) rng {
	end := 1
	if nl := strings.IndexByte(text, '\n'); nl > 0 {
		end = nl
	}
	return rng{Start: position{0, 0}, End: position{0, end}}
}

// stripFilePrefix removes a leading "basename: " that loader errors carry, since
// the client already associates the diagnostic with the file by URI.
func stripFilePrefix(msg, base string) string {
	return strings.TrimPrefix(msg, base+": ")
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

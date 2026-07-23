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
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// Server is a single-connection LSP server speaking JSON-RPC 2.0 over a stream.
type Server struct {
	version string
	r       *bufio.Reader
	w       io.Writer
	wmu     sync.Mutex
	docs    map[string]string // uri -> latest buffer text
}

// NewServer returns a server that reports the given version to clients.
func NewServer(version string) *Server {
	return &Server{version: version, docs: map[string]string{}}
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
			"textDocumentSync": 1,
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
	s.publish(p.TextDocument.URI, p.TextDocument.Text)
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
	s.publish(p.TextDocument.URI, text)
}

func (s *Server) handleSave(raw json.RawMessage) {
	uri := uriParam(raw)
	if text, ok := s.docs[uri]; ok {
		s.publish(uri, text)
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

// publish validates one buffer and pushes its diagnostics.
func (s *Server) publish(uri, text string) {
	path := uriToPath(uri)
	if !strings.HasSuffix(path, ".tfer") && !strings.HasSuffix(path, ".yaml") {
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
	}
	s.publishDiagnostics(uri, diags)
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

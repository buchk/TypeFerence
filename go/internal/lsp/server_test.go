package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func frame(method string, id any, params any) string {
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		m["id"] = id
	}
	if params != nil {
		m["params"] = params
	}
	b, _ := json.Marshal(m)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(b), b)
}

func readFrames(t *testing.T, out string) []map[string]any {
	t.Helper()
	var msgs []map[string]any
	rest := out
	for {
		i := strings.Index(rest, "Content-Length: ")
		if i < 0 {
			break
		}
		rest = rest[i+len("Content-Length: "):]
		j := strings.Index(rest, "\r\n\r\n")
		if j < 0 {
			break
		}
		var n int
		fmt.Sscanf(rest[:j], "%d", &n)
		body := rest[j+4 : j+4+n]
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("bad frame: %v", err)
		}
		msgs = append(msgs, m)
		rest = rest[j+4+n:]
	}
	return msgs
}

func docParams(uri, text string) map[string]any {
	return map[string]any{"textDocument": map[string]any{"uri": uri, "text": text}}
}

func TestServerDiagnostics(t *testing.T) {
	goodSkill := "---\nschemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\n---\ndo the thing\n"
	badSkill := "---\nschemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\n---\ndo the thing\n" // missing binds

	input := frame("initialize", 1, map[string]any{}) +
		frame("textDocument/didOpen", nil, docParams("file:///tmp/bad.tfer", badSkill)) +
		frame("textDocument/didOpen", nil, docParams("file:///tmp/good.tfer", goodSkill)) +
		frame("shutdown", 2, nil) +
		frame("exit", nil, nil)

	var out bytes.Buffer
	if err := NewServer("test").Run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	frames := readFrames(t, out.String())

	sawInit := false
	badDiags, goodDiags := -1, -1
	for _, m := range frames {
		if m["method"] == "textDocument/publishDiagnostics" {
			p := m["params"].(map[string]any)
			uri := p["uri"].(string)
			diags := p["diagnostics"].([]any)
			switch {
			case strings.Contains(uri, "bad.tfer"):
				badDiags = len(diags)
			case strings.Contains(uri, "good.tfer"):
				goodDiags = len(diags)
			}
		}
		if r, ok := m["result"].(map[string]any); ok {
			if _, has := r["capabilities"]; has {
				sawInit = true
			}
		}
	}
	if !sawInit {
		t.Error("expected an initialize result advertising capabilities")
	}
	if badDiags != 1 {
		t.Errorf("bad.tfer: want 1 diagnostic, got %d", badDiags)
	}
	if goodDiags != 0 {
		t.Errorf("good.tfer: want 0 diagnostics, got %d", goodDiags)
	}
}

func TestServerBadFrontmatterFenceDiagnostic(t *testing.T) {
	// A .tfer with no closing fence must produce exactly one diagnostic.
	broken := "---\nschemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\n"
	input := frame("initialize", 1, map[string]any{}) +
		frame("textDocument/didOpen", nil, docParams("file:///tmp/broken.tfer", broken)) +
		frame("exit", nil, nil)

	var out bytes.Buffer
	if err := NewServer("test").Run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range readFrames(t, out.String()) {
		if m["method"] == "textDocument/publishDiagnostics" {
			p := m["params"].(map[string]any)
			diags := p["diagnostics"].([]any)
			if len(diags) == 1 {
				msg := diags[0].(map[string]any)["message"].(string)
				if strings.Contains(msg, "closing '---' frontmatter fence") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected a closing-fence diagnostic for the broken .tfer")
	}
}

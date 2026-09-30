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

// diagnosticsFor returns the diagnostics last published for a uri.
func diagnosticsFor(frames []map[string]any, uri string) []any {
	var diags []any
	for _, m := range frames {
		if m["method"] == "textDocument/publishDiagnostics" {
			p := m["params"].(map[string]any)
			if p["uri"] == uri {
				diags = p["diagnostics"].([]any)
			}
		}
	}
	return diags
}

func TestServerDiagnostics(t *testing.T) {
	root := packageDir(t)
	goodSkill := "---\ndescription: Do the thing.\n---\ndo the thing\n"
	badSkill := "---\ndescription: Do the thing.\nbinds: [nope]\n---\ndo the thing\n"
	goodURI := pathToURI(writeFile(t, root, "skills/good.skill.tfer", goodSkill))
	badURI := pathToURI(writeFile(t, root, "skills/bad.skill.tfer", badSkill))

	input := frame("initialize", 1, map[string]any{"rootUri": pathToURI(root)}) +
		frame("textDocument/didChange", nil, map[string]any{
			"textDocument":   map[string]any{"uri": badURI},
			"contentChanges": []any{map[string]any{"text": badSkill}},
		}) +
		frame("textDocument/didChange", nil, map[string]any{
			"textDocument":   map[string]any{"uri": goodURI},
			"contentChanges": []any{map[string]any{"text": goodSkill}},
		}) +
		frame("shutdown", 2, nil) +
		frame("exit", nil, nil)

	var out bytes.Buffer
	if err := NewServer("test").Run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	frames := readFrames(t, out.String())
	sawInit := false
	for _, m := range frames {
		if r, ok := m["result"].(map[string]any); ok {
			if _, has := r["capabilities"]; has {
				sawInit = true
			}
		}
	}
	if !sawInit {
		t.Error("expected an initialize result advertising capabilities")
	}
	bad := diagnosticsFor(frames, badURI)
	if len(bad) != 1 {
		t.Fatalf("bad.skill.tfer: want 1 diagnostic, got %d", len(bad))
	}
	start := bad[0].(map[string]any)["range"].(map[string]any)["start"].(map[string]any)
	if line := int(start["line"].(float64)); line != 2 {
		t.Errorf("the diagnostic must point at the offending line (0-based 2), got %d", line)
	}
	if got := diagnosticsFor(frames, goodURI); len(got) != 0 {
		t.Errorf("good.skill.tfer: want 0 diagnostics, got %v", got)
	}
}

func TestServerBadFrontmatterFenceDiagnostic(t *testing.T) {
	root := packageDir(t)
	broken := "---\ndescription: Missing its closing fence.\n"
	uri := pathToURI(writeFile(t, root, "skills/broken.skill.tfer", broken))
	frames := runSession(t, pathToURI(root),
		frame("textDocument/didChange", nil, map[string]any{
			"textDocument":   map[string]any{"uri": uri},
			"contentChanges": []any{map[string]any{"text": broken}},
		}),
	)
	diags := diagnosticsFor(frames, uri)
	if len(diags) != 1 || !strings.Contains(diags[0].(map[string]any)["message"].(string), "closing '---' frontmatter fence") {
		t.Errorf("expected a closing-fence diagnostic for the broken document, got %v", diags)
	}
}

func TestServerReportsDocumentsOutsideAPackage(t *testing.T) {
	root := t.TempDir()
	text := "---\ndescription: Orphan.\n---\nOrphan.\n"
	uri := pathToURI(writeFile(t, root, "orphan.skill.tfer", text))
	frames := runSession(t, pathToURI(root), frame("textDocument/didOpen", nil, docParams(uri, text)))
	diags := diagnosticsFor(frames, uri)
	if len(diags) != 1 || !strings.Contains(diags[0].(map[string]any)["message"].(string), "not part of a version 6 package") {
		t.Errorf("a document outside any package must say so, got %v", diags)
	}
}

package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionsKindAndFields(t *testing.T) {
	if got := completions("kind: ", 0, 6); len(got) == 0 || got[0] != "agent" {
		t.Errorf("expected kind completions after `kind:`, got %v", got)
	}
	fields := completions("sch", 0, 3)
	if !contains(fields, "schemaVersion") {
		t.Errorf("expected field completions to include schemaVersion, got %v", fields)
	}
	if got := completions("id: foo", 0, 7); got != nil {
		t.Errorf("expected no completions in a value position, got %v", got)
	}
}

func TestTokenAtAndSymbolOf(t *testing.T) {
	text := "kind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/capabilities/c@1.0.0\n"
	if tok := tokenAt(text, 2, 15); tok != "acme/capabilities/c@1.0.0" {
		t.Errorf("tokenAt on the binds id: got %q", tok)
	}
	id, kind := symbolOf(text)
	if id != "acme/skills/s@1.0.0" || kind != "skill" {
		t.Errorf("symbolOf: got (%q, %q)", id, kind)
	}
}

func writeFile(t *testing.T, root, name, content string) string {
	t.Helper()
	full := filepath.Join(root, name)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func runSession(t *testing.T, rootURI string, msgs ...string) []map[string]any {
	t.Helper()
	input := frame("initialize", 1, map[string]any{"rootUri": rootURI})
	for _, m := range msgs {
		input += m
	}
	input += frame("exit", nil, nil)
	var out bytes.Buffer
	if err := NewServer("test").Run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	return readFrames(t, out.String())
}

func resultFor(frames []map[string]any, id int) any {
	for _, m := range frames {
		if n, ok := m["id"].(float64); ok && int(n) == id {
			return m["result"]
		}
	}
	return nil
}

func TestDefinitionResolvesResourceID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\n")
	skillText := "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/c@1.0.0\n"
	skillURI := pathToURI(writeFile(t, root, "skill.yaml", skillText))
	frames := runSession(t, pathToURI(root),
		frame("textDocument/didOpen", nil, docParams(skillURI, skillText)),
		frame("textDocument/definition", 2, map[string]any{
			"textDocument": map[string]any{"uri": skillURI},
			"position":     map[string]any{"line": 3, "character": 12}, // on the binds id
		}),
	)
	res, _ := resultFor(frames, 2).(map[string]any)
	uri, _ := res["uri"].(string)
	if !strings.Contains(uri, "cap.yaml") {
		t.Errorf("definition should resolve the binds id to cap.yaml, got %q", uri)
	}
}

func TestDocumentSymbolReturnsResource(t *testing.T) {
	root := t.TempDir()
	text := "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\n"
	uri := pathToURI(writeFile(t, root, "cap.yaml", text))
	frames := runSession(t, pathToURI(root),
		frame("textDocument/didOpen", nil, docParams(uri, text)),
		frame("textDocument/documentSymbol", 2, map[string]any{
			"textDocument": map[string]any{"uri": uri},
		}),
	)
	syms, _ := resultFor(frames, 2).([]any)
	if len(syms) != 1 {
		t.Fatalf("expected one document symbol, got %v", syms)
	}
	name, _ := syms[0].(map[string]any)["name"].(string)
	if !strings.Contains(name, "acme/cap/c@1.0.0") {
		t.Errorf("symbol name should include the id, got %q", name)
	}
}

func TestCompositionDiagnosticSurfaces(t *testing.T) {
	root := t.TempDir()
	// skill binds a capability that does not exist -> workspace does not compose
	agentText := "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n"
	agentURI := pathToURI(writeFile(t, root, "agent.yaml", agentText))
	writeFile(t, root, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/missing@1.0.0\n")
	frames := runSession(t, pathToURI(root),
		frame("textDocument/didOpen", nil, docParams(agentURI, agentText)),
	)
	found := false
	for _, m := range frames {
		if m["method"] == "textDocument/publishDiagnostics" {
			p := m["params"].(map[string]any)
			if p["uri"] == agentURI && len(p["diagnostics"].([]any)) >= 1 {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected a composition diagnostic for a workspace that does not resolve")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

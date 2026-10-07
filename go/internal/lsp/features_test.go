package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packageDir writes a minimal version 8 package manifest and returns its root.
func packageDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "typeference.tfer", "---\nschemaVersion: 8\nname: acme/test\nversion: 1.0.0\nplugins:\n  - plugins/kit.plugin.tfer\n---\n")
	return root
}

func writeFile(t *testing.T, root, name, content string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func TestCompletionsByKindAndReference(t *testing.T) {
	root := packageDir(t)
	writeFile(t, root, "skills/review.skill.tfer", "---\ndescription: Review.\n---\nReview.\n")
	writeFile(t, root, "skills/lint.skill.tfer", "---\ndescription: Lint.\n---\nLint.\n")
	writeFile(t, root, "profiles/base.profile.tfer", "---\n---\n")
	agentPath := filepath.Join(root, "agents", "a.agent.tfer")
	pluginPath := filepath.Join(root, "plugins", "kit.plugin.tfer")

	fields := completions("---\nde\n---\n", agentPath, root, 1, 2)
	if !contains(fields, "description") || !contains(fields, "extends") || contains(fields, "embeds") || contains(fields, "binds") {
		t.Errorf("an agent's key position offers agent fields only, got %v", fields)
	}
	skills := completions("---\nskills:\n  - \n---\n", pluginPath, root, 2, 4)
	if !contains(skills, "skills/review.skill.tfer") || !contains(skills, "skills/lint.skill.tfer") || contains(skills, "profiles/base.profile.tfer") {
		t.Errorf("a skills item offers skill paths, got %v", skills)
	}
	embeds := completions("---\nembeds: \n---\n", pluginPath, root, 1, 8)
	if !contains(embeds, "profiles/base.profile.tfer") {
		t.Errorf("embeds offers profile paths, got %v", embeds)
	}
	modes := completions("---\nmodes:\n  - \n---\n", pluginPath, root, 2, 4)
	if !contains(modes, "manual") || !contains(modes, "pipeline") {
		t.Errorf("plugin modes are enumerated, got %v", modes)
	}
	if got := completions("---\ndescription: x\n---\nbody text\n", agentPath, root, 3, 2); got != nil {
		t.Errorf("no completions in the body, got %v", got)
	}
}

func TestTokenAtFindsReferencePaths(t *testing.T) {
	text := "---\nextends: skills/core/review.skill.tfer\n---\n"
	if tok := tokenAt(text, 1, 15); tok != "skills/core/review.skill.tfer" {
		t.Errorf("tokenAt on the extends path: got %q", tok)
	}
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

func TestDefinitionResolvesReferencePath(t *testing.T) {
	root := packageDir(t)
	writeFile(t, root, "capabilities/c.capability.tfer", "---\ndescription: C.\n---\n")
	skillText := "---\ndescription: S.\nbinds: capabilities/c.capability.tfer\n---\nS.\n"
	skillURI := pathToURI(writeFile(t, root, "skills/s.skill.tfer", skillText))
	frames := runSession(t, pathToURI(root),
		frame("textDocument/didOpen", nil, docParams(skillURI, skillText)),
		frame("textDocument/definition", 2, map[string]any{
			"textDocument": map[string]any{"uri": skillURI},
			"position":     map[string]any{"line": 2, "character": 12}, // on the binds path
		}),
	)
	res, _ := resultFor(frames, 2).(map[string]any)
	uri, _ := res["uri"].(string)
	if !strings.Contains(uri, "c.capability.tfer") {
		t.Errorf("definition should resolve the binds path, got %q", uri)
	}
}

func TestDocumentSymbolNamesTheDerivedIdentity(t *testing.T) {
	root := packageDir(t)
	text := "---\ndescription: C.\n---\n"
	uri := pathToURI(writeFile(t, root, "capabilities/c.capability.tfer", text))
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
	if name != "capability acme/test/capabilities/c@1.0.0" {
		t.Errorf("symbol names the kind and derived identity, got %q", name)
	}
}

func TestCompositionDiagnosticSurfaces(t *testing.T) {
	root := packageDir(t)
	writeFile(t, root, "plugins/kit.plugin.tfer", "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n")
	agentText := "---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n---\n"
	agentURI := pathToURI(writeFile(t, root, "agents/a.agent.tfer", agentText))
	// The skill binds a capability document that does not exist.
	writeFile(t, root, "skills/s.skill.tfer", "---\ndescription: S.\nbinds: capabilities/missing.capability.tfer\n---\nS.\n")
	frames := runSession(t, pathToURI(root),
		frame("textDocument/didOpen", nil, docParams(agentURI, agentText)),
	)
	if len(diagnosticsFor(frames, agentURI)) == 0 {
		t.Error("expected a composition diagnostic for a package that does not resolve")
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

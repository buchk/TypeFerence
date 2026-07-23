package resource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSource(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const minimalAgent = `schemaVersion: 3
kind: agent
id: t/agent@1.0.0
displayName: Test Agent
description: A test agent.
`

func TestLoadMinimalAgent(t *testing.T) {
	root := writeSource(t, map[string]string{"agent.yaml": minimalAgent})
	docs, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	doc := docs["t/agent@1.0.0"]
	if doc == nil {
		t.Fatal("agent not loaded")
	}
	if !doc.Emit {
		t.Error("emit should default to true")
	}
	if doc.InputSchema != `{"type":"object","additionalProperties":false}` {
		t.Errorf("unexpected default input schema %q", doc.InputSchema)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"agent.yaml": minimalAgent + "extends: something\n"})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "'extends'") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestSchemaVersionEnforced(t *testing.T) {
	root := writeSource(t, map[string]string{"agent.yaml": strings.Replace(minimalAgent, "schemaVersion: 3", "schemaVersion: 2", 1)})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "schemaVersion must be 3") {
		t.Fatalf("expected schemaVersion error, got %v", err)
	}
}

func TestInvalidIDRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"agent.yaml": strings.Replace(minimalAgent, "t/agent@1.0.0", "Bad Id", 1)})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "lowercase namespace/name@semantic-version") {
		t.Fatalf("expected id error, got %v", err)
	}
}

func TestDuplicateIDRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"a.yaml": minimalAgent, "b.yaml": minimalAgent})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "Duplicate resource id") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestPathEscapeRejected(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml": minimalAgent + "contextFiles:\n  - ../outside.md\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "escapes source root") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestMissingContextFileRejected(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml": minimalAgent + "contextFiles:\n  - context/missing.md\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestSkillMustBindCapability(t *testing.T) {
	root := writeSource(t, map[string]string{
		"skill.yaml": "schemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "skills must bind a capability") {
		t.Fatalf("expected binds error, got %v", err)
	}
}

func TestOnlySkillsBind(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml": minimalAgent + "binds: t/capabilities/c@1.0.0\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "only skills can bind capabilities") {
		t.Fatalf("expected binds error, got %v", err)
	}
}

func TestCapabilitiesCannotEmbed(t *testing.T) {
	root := writeSource(t, map[string]string{
		"cap.yaml": "schemaVersion: 3\nkind: capability\nid: t/capabilities/c@1.0.0\nembeds:\n  - t/other@1.0.0\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "cannot embed resources") {
		t.Fatalf("expected embed error, got %v", err)
	}
}

func TestInvalidSchemaJSONRejected(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml": minimalAgent + "inputSchema: 'not json'\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "invalid inputSchema") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestTrustConfigurationExcluded(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml":             minimalAgent,
		"typeference.trust.yaml": "schemaVersion: 1\nsource:\n  identity: https://example.com\n",
	})
	docs, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Errorf("trust configuration should be excluded from resources, got %d docs", len(docs))
	}
}

func TestEmptyResourceRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"empty.yaml": ""})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "Empty resource") {
		t.Fatalf("expected empty resource error, got %v", err)
	}
}

func TestMultiDocumentRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"multi.yaml": minimalAgent + "---\n" + minimalAgent})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "single YAML document") {
		t.Fatalf("expected multi-document error, got %v", err)
	}
}

func TestUnknownKindRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"agent.yaml": strings.Replace(minimalAgent, "kind: agent", "kind: widget", 1)})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("expected kind error, got %v", err)
	}
}

const tferSkill = `---
schemaVersion: 3
kind: skill
id: t/skills/s@1.0.0
binds: t/capabilities/c@1.0.0
---
Inspect the requested signals and report status, evidence, and risk.
`

func TestTferSkillBodyBecomesInstructions(t *testing.T) {
	root := writeSource(t, map[string]string{"s.tfer": tferSkill})
	docs, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	doc := docs["t/skills/s@1.0.0"]
	if doc == nil {
		t.Fatal("skill not loaded")
	}
	want := "Inspect the requested signals and report status, evidence, and risk.\n"
	if doc.Instructions != want {
		t.Errorf("body did not become instructions verbatim: got %q want %q", doc.Instructions, want)
	}
}

func TestTferInstructionsInBothBodyAndFrontmatterRejected(t *testing.T) {
	dual := strings.Replace(tferSkill, "binds: t/capabilities/c@1.0.0\n",
		"binds: t/capabilities/c@1.0.0\ninstructions: from frontmatter\n", 1)
	root := writeSource(t, map[string]string{"s.tfer": dual})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "both the body and frontmatter") {
		t.Fatalf("expected dual-instructions error, got %v", err)
	}
}

func TestTferMissingOpeningFenceRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"s.tfer": "schemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\n"})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "must begin with a '---' frontmatter fence") {
		t.Fatalf("expected opening-fence error, got %v", err)
	}
}

func TestTferMissingClosingFenceRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"s.tfer": "---\nschemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\n"})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "missing its closing '---' frontmatter fence") {
		t.Fatalf("expected closing-fence error, got %v", err)
	}
}

func TestTferContextObjectLoads(t *testing.T) {
	ctxType := `---
schemaVersion: 3
kind: contextType
id: t/context-types/cast@1.0.0
displayName: Cast of Characters
schema: '{"type":"object","properties":{"role":{"type":"string"}}}'
---
`
	ctxObj := `---
schemaVersion: 3
kind: context
id: t/notes/principal@1.0.0
contextType: t/context-types/cast@1.0.0
---
The principal prefers short decision briefs.
`
	root := writeSource(t, map[string]string{"cast.tfer": ctxType, "principal.tfer": ctxObj})
	docs, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	ct := docs["t/context-types/cast@1.0.0"]
	if ct == nil || ct.Kind != "contextType" {
		t.Fatal("contextType not loaded")
	}
	obj := docs["t/notes/principal@1.0.0"]
	if obj == nil {
		t.Fatal("context object not loaded")
	}
	if obj.ContextType != "t/context-types/cast@1.0.0" {
		t.Errorf("context object lost its contextType: %q", obj.ContextType)
	}
	if obj.Content != "The principal prefers short decision briefs.\n" {
		t.Errorf("context body did not become content: %q", obj.Content)
	}
}

func TestTferContextRequiresContextType(t *testing.T) {
	root := writeSource(t, map[string]string{"n.tfer": "---\nschemaVersion: 3\nkind: context\nid: t/notes/n@1.0.0\n---\nbody\n"})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "must declare a contextType") {
		t.Fatalf("expected missing-contextType error, got %v", err)
	}
}

func TestTferBodyOnBodylessKindRejected(t *testing.T) {
	root := writeSource(t, map[string]string{"a.tfer": "---\nschemaVersion: 3\nkind: agent\nid: t/agent@1.0.0\n---\nstray body\n"})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "has no body field") {
		t.Fatalf("expected bodyless-kind error, got %v", err)
	}
}

func TestContextObjectCollectsSchemaFields(t *testing.T) {
	src := "schemaVersion: 3\nkind: context\nid: t/notes/n@1.0.0\ncontextType: t/ct/cast@1.0.0\nrole: owner\ntags:\n  - a\n"
	root := writeSource(t, map[string]string{"n.yaml": src})
	docs, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	d := docs["t/notes/n@1.0.0"]
	if d.ContextFields["role"].Scalar != "owner" || d.ContextFields["role"].Kind != "scalar" {
		t.Errorf("scalar field 'role' not collected: %v", d.ContextFields)
	}
	if d.ContextFields["tags"].Kind != "sequence" {
		t.Errorf("sequence field 'tags' should be recorded with sequence kind, got %v", d.ContextFields["tags"])
	}
}

func TestNonContextStillRejectsUnknownFields(t *testing.T) {
	// The context-field collection must not weaken strictness for other kinds.
	root := writeSource(t, map[string]string{
		"cap.yaml": "schemaVersion: 3\nkind: capability\nid: t/cap/c@1.0.0\nrole: owner\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "'role'") {
		t.Fatalf("a capability with an unknown field must still error, got %v", err)
	}
}

func TestYamlAndTferInteroperate(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml": minimalAgent,
		"skill.tfer": tferSkill,
	})
	docs, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if docs["t/agent@1.0.0"] == nil || docs["t/skills/s@1.0.0"] == nil {
		t.Fatalf("expected both .yaml and .tfer resources, got %d", len(docs))
	}
}

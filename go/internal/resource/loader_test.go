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

package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSrc(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIsUnlinked(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	output := t.TempDir()
	if _, err := Build(source, output, []Target{Codex}, nil); err != nil {
		t.Fatal(err)
	}
	agentRoot := filepath.Join(output, "codex", "a")
	if _, err := os.Stat(filepath.Join(agentRoot, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("build must not emit active Codex runtime configuration")
	}
	if _, err := os.Stat(filepath.Join(agentRoot, ".typeference", "link.json")); err != nil {
		t.Fatal("build must emit typed link requirements")
	}
}

func TestProjectManifestRejectsDeployment(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	writeSrc(t, source, "typeference.yaml", "schemaVersion: 2\nname: acme/agents\nversion: 1.0.0\ndeployment:\n  mcpCommand: nope\n")
	_, err := Build(source, t.TempDir(), []Target{Codex}, nil)
	if err == nil || !strings.Contains(err.Error(), "deployment") {
		t.Fatalf("deployment metadata in the source manifest must fail, got %v", err)
	}
}

func TestARDDoesNotInventCallableCards(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "cap.yaml", "schemaVersion: 4\nkind: capability\nid: acme/capabilities/c@1.0.0\nvisibility: exposed\n")
	writeSrc(t, source, "skill.yaml", "schemaVersion: 4\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/capabilities/c@1.0.0\ninstructions: do it\n")
	writeSrc(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	output := t.TempDir()
	if _, err := Build(source, output, []Target{Neutral}, &ArdPublicationOptions{PublisherDomain: "acme.example"}); err != nil {
		t.Fatal(err)
	}
	catalog, err := os.ReadFile(filepath.Join(output, "ard", "ai-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(catalog), "a2a-agent-card") || strings.Contains(string(catalog), "mcp-server") {
		t.Fatal("unlinked build must not publish callable endpoint/provider claims")
	}
}

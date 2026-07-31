package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

func TestCodexConfigurationIsMaterializedOnlyAtLink(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "cap.yaml"), "schemaVersion: 4\nkind: capability\nid: acme/capabilities/c@1.0.0\nvisibility: exposed\n")
	writeTestFile(t, filepath.Join(source, "tool.yaml"), "schemaVersion: 4\nkind: tool\nid: acme/tools/runtime@1.0.0\n")
	writeTestFile(t, filepath.Join(source, "skill.yaml"), "schemaVersion: 4\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/capabilities/c@1.0.0\ninstructions: do it\nrequiresTools: [acme/tools/runtime@1.0.0]\n")
	writeTestFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	built := t.TempDir()
	if _, err := compile.Build(source, built, []compile.Target{compile.Codex}, nil); err != nil {
		t.Fatal(err)
	}
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/agents/a@1.0.0:\n    modes: []\nproviders:\n  runtime:\n    kind: mcp\n    transport: stdio\n    command: 'server\"name'\n    args: ['--bundle', '{bundle}']\n    environment:\n      ACME_TOKEN:\n        fromEnvironment: ACME_TOKEN\ntoolBindings:\n  acme/tools/runtime@1.0.0:\n    provider: runtime\n    remoteName: run\n")
	linked := filepath.Join(t.TempDir(), "linked")
	if _, err := Link(filepath.Join(built, "codex"), deployment, linked); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(linked, "a", ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	if !strings.Contains(text, `command = "server\"name"`) || !strings.Contains(text, `env_vars = ["ACME_TOKEN"]`) {
		t.Fatalf("linked config is not structurally escaped/materialized:\n%s", text)
	}
	bindings, err := os.ReadFile(filepath.Join(linked, "a", ".typeference", "tool-bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bindings), `"remoteName": "run"`) {
		t.Fatalf("tool binding did not preserve the provider-level remote name:\n%s", bindings)
	}
	if _, err := os.Stat(filepath.Join(linked, ".typeference", "build.json")); !os.IsNotExist(err) {
		t.Fatalf("linked output must not retain a stale active build index, got %v", err)
	}
	if provenance, err := os.ReadFile(filepath.Join(linked, ".typeference", "link-provenance.json")); err != nil ||
		!strings.Contains(string(provenance), `"artifacts"`) {
		t.Fatalf("linked artifact digests were not recorded: %v\n%s", err, provenance)
	}
}

func TestA2ACardRequiresNeutralArtifactAndA2AMode(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "cap.yaml"), "schemaVersion: 4\nkind: capability\nid: acme/capabilities/c@1.0.0\nvisibility: exposed\n")
	writeTestFile(t, filepath.Join(source, "skill.yaml"), "schemaVersion: 4\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/capabilities/c@1.0.0\nvariants:\n  manual:\n    instructions: talk\n  a2a:\n    instructions: call\n")
	writeTestFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	built := t.TempDir()
	if _, err := compile.Build(source, built, []compile.Target{compile.Neutral, compile.Codex}, nil); err != nil {
		t.Fatal(err)
	}
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/agents/a@1.0.0:\n    modes: [a2a]\nagentEndpoints:\n  acme/agents/a@1.0.0:\n    a2aUrl: https://agents.example/a\n")
	linked := filepath.Join(t.TempDir(), "neutral-linked")
	if _, err := Link(filepath.Join(built, "neutral"), deployment, linked); err != nil {
		t.Fatal(err)
	}
	card, err := os.ReadFile(filepath.Join(linked, "a", ".typeference", "a2a-agent-card.json"))
	if err != nil || !strings.Contains(string(card), `"url": "https://agents.example/a"`) {
		t.Fatalf("A2A card did not preserve the authored endpoint: %v\n%s", err, card)
	}
	if _, err := Link(filepath.Join(built, "codex"), deployment, filepath.Join(t.TempDir(), "codex-linked")); err == nil ||
		!strings.Contains(err.Error(), "materializes mode manual") {
		t.Fatalf("Codex must reject a2a deployment selection, got %v", err)
	}
}

func TestLinkRejectsTamperedUnlinkedArtifact(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	built := t.TempDir()
	if _, err := compile.Build(source, built, []compile.Target{compile.Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(built, "neutral", "a", "AGENTS.md"), "tampered\n")
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/agents/a@1.0.0:\n    modes: []\n")
	if _, err := Link(filepath.Join(built, "neutral"), deployment, filepath.Join(t.TempDir(), "linked")); err == nil ||
		!strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected tampering to fail integrity verification, got %v", err)
	}
}

func TestLinkRejectsOutputThatContainsItsInput(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	container := t.TempDir()
	built := filepath.Join(container, "built")
	if _, err := compile.Build(source, built, []compile.Target{compile.Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/agents/a@1.0.0:\n    modes: []\n")
	if _, err := Link(filepath.Join(built, "neutral"), deployment, built); err == nil ||
		!strings.Contains(err.Error(), "separate sibling") {
		t.Fatalf("expected containing output to be rejected before reset, got %v", err)
	}
}

func TestInvalidEndpointFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, path, "schemaVersion: 1\nenvironment: test\nagentEndpoints:\n  acme/agents/a@1.0.0:\n    a2aUrl: http://insecure.example/a\n")
	if _, _, err := Load(path); err == nil {
		t.Fatal("an insecure A2A endpoint must be rejected")
	}
}

func TestToolFreeLinkedArtifactRecordsSelectedModes(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "cap.yaml"), "schemaVersion: 4\nkind: capability\nid: acme/capabilities/c@1.0.0\n")
	writeTestFile(t, filepath.Join(source, "skill.yaml"), "schemaVersion: 4\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/capabilities/c@1.0.0\nvariants:\n  manual:\n    instructions: talk\n  pipeline:\n    instructions: emit\n")
	writeTestFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	built := t.TempDir()
	if _, err := compile.Build(source, built, []compile.Target{compile.Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/agents/a@1.0.0:\n    modes: [pipeline]\n")
	linked := filepath.Join(t.TempDir(), "linked")
	if _, err := Link(filepath.Join(built, "neutral"), deployment, linked); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(linked, "a", ".typeference", "tool-bindings.json"))
	if err != nil || !strings.Contains(string(manifest), "\"selectedModes\": [\n    \"pipeline\"\n  ]") {
		t.Fatalf("tool-free link did not preserve selected modes: %v\n%s", err, manifest)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

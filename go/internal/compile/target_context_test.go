package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetValidatesOnlyMaterializedVariantContext(t *testing.T) {
	source := t.TempDir()
	writeTargetContextFile(t, filepath.Join(source, "cap.yaml"), "schemaVersion: 4\nkind: capability\nid: acme/capabilities/c@1.0.0\n")
	writeTargetContextFile(t, filepath.Join(source, "ct.yaml"), "schemaVersion: 4\nkind: contextType\nid: acme/context-types/runtime@1.0.0\n")
	writeTargetContextFile(t, filepath.Join(source, "skill.yaml"), "schemaVersion: 4\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/capabilities/c@1.0.0\nvariants:\n  manual:\n    instructions: interactive\n  a2a:\n    instructions: remote\n    requiresContextTypes: [acme/context-types/runtime@1.0.0]\n")
	writeTargetContextFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")

	if _, err := Build(source, t.TempDir(), []Target{Codex}, nil); err != nil {
		t.Fatalf("Codex materializes manual and must ignore a2a-only context: %v", err)
	}
	if _, err := Build(source, t.TempDir(), []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "a2a requires context type") {
		t.Fatalf("neutral carries every mode and must reject missing a2a context, got %v", err)
	}
}

func TestTargetNativeNameCollisionsFailClosed(t *testing.T) {
	source := t.TempDir()
	writeTargetContextFile(t, filepath.Join(source, "a.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/worker@1.0.0\n")
	writeTargetContextFile(t, filepath.Join(source, "b.yaml"), "schemaVersion: 4\nkind: agent\nid: other/agents/worker@1.0.0\n")
	if _, err := Build(source, t.TempDir(), []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "same target artifact path") {
		t.Fatalf("expected colliding agent artifact names to fail, got %v", err)
	}
}

func TestTargetDispatchNameCollisionsFailClosed(t *testing.T) {
	source := t.TempDir()
	writeTargetContextFile(t, filepath.Join(source, "cap-a.yaml"), "schemaVersion: 4\nkind: capability\nid: acme/capabilities/status@1.0.0\n")
	writeTargetContextFile(t, filepath.Join(source, "cap-b.yaml"), "schemaVersion: 4\nkind: capability\nid: other/capabilities/status@1.0.0\n")
	writeTargetContextFile(t, filepath.Join(source, "skill-a.yaml"), "schemaVersion: 4\nkind: skill\nid: acme/skills/status@1.0.0\nbinds: acme/capabilities/status@1.0.0\ninstructions: a\n")
	writeTargetContextFile(t, filepath.Join(source, "skill-b.yaml"), "schemaVersion: 4\nkind: skill\nid: other/skills/status@1.0.0\nbinds: other/capabilities/status@1.0.0\ninstructions: b\n")
	writeTargetContextFile(t, filepath.Join(source, "agent.yaml"), "schemaVersion: 4\nkind: agent\nid: acme/agents/worker@1.0.0\nskills:\n  - ref: acme/skills/status@1.0.0\n  - ref: other/skills/status@1.0.0\n")
	if _, err := Build(source, t.TempDir(), []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "same target dispatch name") {
		t.Fatalf("expected colliding dispatch names to fail, got %v", err)
	}
}

func writeTargetContextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

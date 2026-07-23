package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetsSelectSurfaceVariant(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\n")
	writeSrc(t, src, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/c@1.0.0\nvariants:\n  pipeline:\n    instructions: PIPELINE_TEXT\n  manual:\n    instructions: MANUAL_TEXT\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	out := t.TempDir()
	targets, err := ParseTargets("all")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(src, out, targets, nil); err != nil {
		t.Fatal(err)
	}
	read := func(parts ...string) string {
		b, _ := os.ReadFile(filepath.Join(append([]string{out}, parts...)...))
		return string(b)
	}

	// Copilot has no per-skill file, so it inlines the manual variant.
	cop := read("copilot", "agent", ".github", "copilot-instructions.md")
	if !strings.Contains(cop, "MANUAL_TEXT") || strings.Contains(cop, "PIPELINE_TEXT") {
		t.Errorf("copilot should inline the manual variant, not pipeline:\n%s", cop)
	}
	// Cursor likewise inlines manual.
	cur := read("cursor", "agent", "AGENTS.md")
	if !strings.Contains(cur, "MANUAL_TEXT") {
		t.Errorf("cursor should inline the manual variant")
	}
	// Codex's SKILL.md renders the manual variant.
	cod := read("codex", "agent", ".agents", "skills", "c", "SKILL.md")
	if !strings.Contains(cod, "MANUAL_TEXT") || strings.Contains(cod, "PIPELINE_TEXT") {
		t.Errorf("codex SKILL.md should render the manual variant")
	}
	// Neutral's SKILL.md keeps the default (pipeline-preferred) and fans out.
	neu := read("neutral", "agent", "skills", "c", "SKILL.md")
	if !strings.Contains(neu, "PIPELINE_TEXT") {
		t.Errorf("neutral SKILL.md should render the default (pipeline) variant")
	}
	if !strings.Contains(read("neutral", "agent", "skills", "c", "SKILL.manual.md"), "MANUAL_TEXT") {
		t.Errorf("neutral should still fan out SKILL.manual.md")
	}
}

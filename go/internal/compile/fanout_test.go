package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeutralVariantFanout(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\n")
	writeSrc(t, src, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/c@1.0.0\nvariants:\n  pipeline:\n    instructions: strict\n  a2a:\n    instructions: attributed\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	out := t.TempDir()
	targets, err := ParseTargets("neutral")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(src, out, targets, nil); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(out, "neutral", "agent", "skills", "c")
	for _, name := range []string{"SKILL.md", "SKILL.pipeline.md", "SKILL.a2a.md"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Errorf("expected %s to be emitted: %v", name, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(base, "SKILL.a2a.md"))
	if err != nil || !strings.Contains(string(body), "attributed") {
		t.Errorf("SKILL.a2a.md should carry the a2a variant's instructions")
	}
}

func TestUnimodalSkillNoFanout(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\n")
	writeSrc(t, src, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/c@1.0.0\ninstructions: do it\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	out := t.TempDir()
	targets, _ := ParseTargets("neutral")
	if _, err := Build(src, out, targets, nil); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(out, "neutral", "agent", "skills", "c")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" {
		t.Errorf("unimodal skill should emit only SKILL.md, got %v", entries)
	}
}

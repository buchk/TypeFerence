package resource

import (
	"strings"
	"testing"
)

func TestVariantsOnlyOnSkills(t *testing.T) {
	root := writeSource(t, map[string]string{
		"agent.yaml": minimalAgent + "variants:\n  manual:\n    instructions: hi\n",
	})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "only skills declare variants") {
		t.Fatalf("expected variants-kind error, got %v", err)
	}
}

func TestVariantsAndInstructionsMutuallyExclusive(t *testing.T) {
	skill := "schemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\ninstructions: top\nvariants:\n  manual:\n    instructions: hi\n"
	root := writeSource(t, map[string]string{"s.yaml": skill})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "either instructions or variants") {
		t.Fatalf("expected instructions/variants exclusivity error, got %v", err)
	}
}

func TestVariantRequiresInstructions(t *testing.T) {
	skill := "schemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\nvariants:\n  manual:\n    instructions: ''\n"
	root := writeSource(t, map[string]string{"s.yaml": skill})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "variant 'manual' must set instructions") {
		t.Fatalf("expected empty-variant error, got %v", err)
	}
}

func TestMultimodalTferSkillRejectsBody(t *testing.T) {
	skill := "---\nschemaVersion: 3\nkind: skill\nid: t/skills/s@1.0.0\nbinds: t/capabilities/c@1.0.0\nvariants:\n  manual:\n    instructions: hi\n---\nstray body\n"
	root := writeSource(t, map[string]string{"s.tfer": skill})
	_, err := Load(root, "")
	if err == nil || !strings.Contains(err.Error(), "multimodal skill has no single body") {
		t.Fatalf("expected multimodal-body error, got %v", err)
	}
}

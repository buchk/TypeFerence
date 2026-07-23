package resolve

import (
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func TestVariantsResolveWithPreferredDefault(t *testing.T) {
	set := baseSkillAgent(func(s *resource.Document) {
		s.Variants = map[string]resource.Variant{
			"pipeline": {Instructions: "strict json"},
			"manual":   {Instructions: "explain"},
			"a2a":      {Instructions: "attributed"},
		}
	}, nil)
	agent, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	skill := agent.Skills[0]
	if skill.Instructions != "strict json" {
		t.Errorf("default rendering should prefer pipeline, got %q", skill.Instructions)
	}
	if len(skill.Variants) != 3 || skill.Variants["manual"] != "explain" || skill.Variants["a2a"] != "attributed" {
		t.Errorf("variants not carried through resolution: %+v", skill.Variants)
	}
}

func TestVariantsDefaultFallsBackAlphabetically(t *testing.T) {
	set := baseSkillAgent(func(s *resource.Document) {
		s.Variants = map[string]resource.Variant{
			"beta":  {Instructions: "b"},
			"alpha": {Instructions: "a"},
		}
	}, nil)
	agent, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Skills[0].Instructions != "a" {
		t.Errorf("with no preferred mode, default should be alphabetically-first 'alpha', got %q", agent.Skills[0].Instructions)
	}
}

func TestInstructionsForSelectsVariantOrDefault(t *testing.T) {
	s := ResolvedSkill{
		Instructions: "default",
		Variants:     map[string]string{"a2a": "attributed", "manual": "explain"},
	}
	if got := s.InstructionsFor("a2a"); got != "attributed" {
		t.Errorf("InstructionsFor(a2a) = %q, want attributed", got)
	}
	if got := s.InstructionsFor("nope"); got != "default" {
		t.Errorf("InstructionsFor(missing mode) should fall back to default, got %q", got)
	}
	uni := ResolvedSkill{Instructions: "only"}
	if got := uni.InstructionsFor("a2a"); got != "only" {
		t.Errorf("unimodal InstructionsFor should return Instructions, got %q", got)
	}
}

func TestUnimodalSkillHasNoVariants(t *testing.T) {
	set := baseSkillAgent(func(s *resource.Document) { s.Instructions = "just do it" }, nil)
	agent, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Skills[0].Variants != nil {
		t.Errorf("unimodal skill should carry no variants map")
	}
	if agent.Skills[0].Instructions != "just do it" {
		t.Errorf("unimodal instructions changed: %q", agent.Skills[0].Instructions)
	}
}

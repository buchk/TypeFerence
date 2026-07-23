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

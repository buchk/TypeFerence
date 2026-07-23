package resolve

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func TestAllowedContextTypesPermitsRefinement(t *testing.T) {
	set := baseSkillAgent(nil, func(a *resource.Document) {
		a.Context = []string{"t/notes/n@1.0.0"}
		a.AllowedContextTypes = []string{"t/ct/cast@1.0.0"}
	})
	set["t/ct/cast@1.0.0"] = doc("contextType", "t/ct/cast@1.0.0", nil)
	set["t/ct/gov@1.0.0"] = doc("contextType", "t/ct/gov@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/ct/cast@1.0.0"} })
	set["t/notes/n@1.0.0"] = doc("context", "t/notes/n@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/gov@1.0.0" })
	if _, err := New(set).Resolve("t/agent@1.0.0"); err != nil {
		t.Fatalf("a governed cast should satisfy an allow-list of [cast]: %v", err)
	}
}

func TestAllowedContextTypesRejectsOutsider(t *testing.T) {
	set := baseSkillAgent(nil, func(a *resource.Document) {
		a.Context = []string{"t/notes/n@1.0.0"}
		a.AllowedContextTypes = []string{"t/ct/other@1.0.0"}
	})
	set["t/ct/other@1.0.0"] = doc("contextType", "t/ct/other@1.0.0", nil)
	set["t/ct/cast@1.0.0"] = doc("contextType", "t/ct/cast@1.0.0", nil)
	set["t/notes/n@1.0.0"] = doc("context", "t/notes/n@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/cast@1.0.0" })
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "not among the allowed") {
		t.Fatalf("expected allow-list rejection, got %v", err)
	}
}

func TestAllowListIntersectsThroughEmbeds(t *testing.T) {
	cap := doc("capability", "t/cap/c@1.0.0", nil)
	skill := doc("skill", "t/skills/s@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	profile := doc("profile", "t/profile@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s@1.0.0"}}
		d.AllowedContextTypes = []string{"t/ct/a@1.0.0", "t/ct/b@1.0.0"}
	})
	a := doc("contextType", "t/ct/a@1.0.0", nil)
	b := doc("contextType", "t/ct/b@1.0.0", nil)
	c := doc("contextType", "t/ct/c@1.0.0", nil)
	objA := doc("context", "t/notes/a@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/a@1.0.0" })
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/profile@1.0.0"}
		d.AllowedContextTypes = []string{"t/ct/b@1.0.0", "t/ct/c@1.0.0"} // effective = [b]
		d.Context = []string{"t/notes/a@1.0.0"}                          // type a, now excluded
	})
	_, err := New(docSet(cap, skill, profile, a, b, c, objA, agent)).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "not among the allowed") {
		t.Fatalf("intersection of [a,b] and [b,c] should exclude type a, got %v", err)
	}
}

func TestVariantContextRequirementAggregatedAndChecked(t *testing.T) {
	set := baseSkillAgent(func(s *resource.Document) {
		s.Variants = map[string]resource.Variant{
			"manual": {Instructions: "m"},
			"a2a":    {Instructions: "a", RequiresContextTypes: []string{"t/ct/gov@1.0.0"}},
		}
	}, nil) // agent holds no context
	set["t/ct/gov@1.0.0"] = doc("contextType", "t/ct/gov@1.0.0", nil)
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "requires context type") {
		t.Fatalf("a variant's context-type requirement must be enforced at the agent, got %v", err)
	}
}

func TestVariantToolRequirementAggregated(t *testing.T) {
	set := baseSkillAgent(func(s *resource.Document) {
		s.Variants = map[string]resource.Variant{
			"pipeline": {Instructions: "p"},
			"a2a":      {Instructions: "a", RequiresTools: []string{"t/tools/reader@1.0.0"}},
		}
	}, nil)
	// tool not declared -> error
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "requires tool") {
		t.Fatalf("a variant's tool requirement must be enforced, got %v", err)
	}
}

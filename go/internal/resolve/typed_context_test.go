package resolve

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// baseSkillAgent returns a capability + skill + agent that resolve cleanly, so
// tests can layer context/tool requirements onto the skill.
func baseSkillAgent(mutateSkill, mutateAgent func(*resource.Document)) map[string]*resource.Document {
	cap := doc("capability", "t/cap/c@1.0.0", nil)
	skill := doc("skill", "t/skills/s@1.0.0", func(d *resource.Document) {
		d.Binds = "t/cap/c@1.0.0"
		if mutateSkill != nil {
			mutateSkill(d)
		}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s@1.0.0"}}
		if mutateAgent != nil {
			mutateAgent(d)
		}
	})
	return docSet(cap, skill, agent)
}

func TestRequiresContextTypeSatisfiedByRefinement(t *testing.T) {
	set := baseSkillAgent(
		func(s *resource.Document) { s.RequiresContextTypes = []string{"t/ct/cast@1.0.0"} },
		func(a *resource.Document) { a.Context = []string{"t/notes/roster@1.0.0"} },
	)
	set["t/ct/cast@1.0.0"] = doc("contextType", "t/ct/cast@1.0.0", nil)
	set["t/ct/governed@1.0.0"] = doc("contextType", "t/ct/governed@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/ct/cast@1.0.0"} // governed refines cast
	})
	set["t/notes/roster@1.0.0"] = doc("context", "t/notes/roster@1.0.0", func(d *resource.Document) {
		d.ContextType = "t/ct/governed@1.0.0" // holds the refinement
	})
	if _, err := New(set).Resolve("t/agent@1.0.0"); err != nil {
		t.Fatalf("a held refinement should satisfy a base-type requirement: %v", err)
	}
}

func TestRequiresContextTypeUnsatisfied(t *testing.T) {
	set := baseSkillAgent(
		func(s *resource.Document) { s.RequiresContextTypes = []string{"t/ct/cast@1.0.0"} },
		nil, // agent holds no context
	)
	set["t/ct/cast@1.0.0"] = doc("contextType", "t/ct/cast@1.0.0", nil)
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "requires context type") {
		t.Fatalf("expected unsatisfied context-type error, got %v", err)
	}
}

func TestContextRequirementIsAgentLevelNotProfileLevel(t *testing.T) {
	// A profile binding a context-requiring skill must not error on its own; the
	// requirement is checked once the agent supplies the context.
	cap := doc("capability", "t/cap/c@1.0.0", nil)
	skill := doc("skill", "t/skills/s@1.0.0", func(d *resource.Document) {
		d.Binds = "t/cap/c@1.0.0"
		d.RequiresContextTypes = []string{"t/ct/cast@1.0.0"}
	})
	profile := doc("profile", "t/profile@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s@1.0.0"}}
	})
	ct := doc("contextType", "t/ct/cast@1.0.0", nil)
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/cast@1.0.0" })
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/profile@1.0.0"}
		d.Context = []string{"t/notes/n@1.0.0"}
	})
	if _, err := New(docSet(cap, skill, profile, ct, obj, agent)).ResolveAll(); err != nil {
		t.Fatalf("agent that supplies context should resolve the promoted requirement: %v", err)
	}
}

func TestRequiresToolDeclared(t *testing.T) {
	set := baseSkillAgent(
		func(s *resource.Document) { s.RequiresTools = []string{"t/tools/reader@1.0.0"} },
		nil,
	)
	set["t/tools/reader@1.0.0"] = doc("tool", "t/tools/reader@1.0.0", nil)
	if _, err := New(set).Resolve("t/agent@1.0.0"); err != nil {
		t.Fatalf("a declared tool should satisfy requiresTools: %v", err)
	}
}

func TestRequiresToolUndeclared(t *testing.T) {
	set := baseSkillAgent(
		func(s *resource.Document) { s.RequiresTools = []string{"t/tools/missing@1.0.0"} },
		nil,
	)
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "requires tool") {
		t.Fatalf("expected undeclared-tool error, got %v", err)
	}
}

func TestContextTypeRefinementCycleRejected(t *testing.T) {
	a := doc("contextType", "t/ct/a@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/ct/b@1.0.0"} })
	b := doc("contextType", "t/ct/b@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/ct/a@1.0.0"} })
	_, err := New(docSet(a, b)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "refinement cycle") {
		t.Fatalf("expected refinement cycle error, got %v", err)
	}
}

func TestToolInvalidSchemaRejected(t *testing.T) {
	tool := doc("tool", "t/tools/reader@1.0.0", func(d *resource.Document) { d.InputSchema = "not json" })
	_, err := New(docSet(tool)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "tool inputSchema") {
		t.Fatalf("expected invalid tool schema error, got %v", err)
	}
}

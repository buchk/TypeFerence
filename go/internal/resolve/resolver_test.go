package resolve

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func doc(kind, id string, mutate func(*resource.Document)) *resource.Document {
	d := resource.NewDocument()
	d.SchemaVersion = 3
	d.Kind = kind
	d.ID = id
	if mutate != nil {
		mutate(d)
	}
	return d
}

func docSet(docs ...*resource.Document) map[string]*resource.Document {
	m := map[string]*resource.Document{}
	for _, d := range docs {
		m[d.ID] = d
	}
	return m
}

func TestShallowestPromotionWins(t *testing.T) {
	deep := doc("profile", "t/deep@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "deep.md"}
	})
	mid := doc("profile", "t/mid@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/deep@1.0.0"}
	})
	near := doc("profile", "t/near@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "near.md"}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/mid@1.0.0", "t/near@1.0.0"}
	})
	resolved, err := New(docSet(deep, mid, near, agent)).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Slots["policy"] != "near.md" {
		t.Errorf("expected shallowest slot to win, got %q", resolved.Slots["policy"])
	}
}

func TestSameDepthAmbiguityFailsWithoutLocalDeclaration(t *testing.T) {
	a := doc("profile", "t/a@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "a.md"}
	})
	b := doc("profile", "t/b@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "b.md"}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/a@1.0.0", "t/b@1.0.0"}
	})
	_, err := New(docSet(a, b, agent)).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestSameDepthAmbiguityResolvedLocally(t *testing.T) {
	a := doc("profile", "t/a@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "a.md"}
	})
	b := doc("profile", "t/b@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "b.md"}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/a@1.0.0", "t/b@1.0.0"}
		d.Slots = map[string]string{"policy": "local.md"}
	})
	resolved, err := New(docSet(a, b, agent)).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Slots["policy"] != "local.md" {
		t.Errorf("expected local declaration to win, got %q", resolved.Slots["policy"])
	}
}

func TestEmbeddingCycleDetected(t *testing.T) {
	a := doc("profile", "t/a@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/b@1.0.0"} })
	b := doc("profile", "t/b@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/a@1.0.0"} })
	_, err := New(docSet(a, b)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "Embedding cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestProfilesCannotEmbedAgents(t *testing.T) {
	inner := doc("agent", "t/inner@1.0.0", nil)
	profile := doc("profile", "t/profile@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/inner@1.0.0"}
	})
	_, err := New(docSet(inner, profile)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "profiles can only embed profiles") {
		t.Fatalf("expected profile embed error, got %v", err)
	}
}

func TestDuplicateEmbedRejected(t *testing.T) {
	a := doc("profile", "t/a@1.0.0", nil)
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/a@1.0.0", "t/a@1.0.0"}
	})
	_, err := New(docSet(a, agent)).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate embed error, got %v", err)
	}
}

func withSchemas(input, output string) func(*resource.Document) {
	return func(d *resource.Document) {
		d.InputSchema = input
		d.OutputSchema = output
	}
}

func TestStructuralInterfaceSatisfaction(t *testing.T) {
	capability := doc("capability", "t/capabilities/check@1.0.0", withSchemas(`{"type":"object"}`, `{"type":"object"}`))
	skill := doc("skill", "t/skills/check@1.0.0", func(d *resource.Document) {
		d.Binds = "t/capabilities/check@1.0.0"
		withSchemas(`{"type":"object"}`, `{"type":"object"}`)(d)
	})
	iface := doc("interface", "t/interfaces/checker@1.0.0", func(d *resource.Document) {
		d.RequiresSlots = []string{"policy"}
		d.RequiresCapabilities = []string{"t/capabilities/check@1.0.0"}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Slots = map[string]string{"policy": "p.md"}
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/check@1.0.0"}}
	})
	resolved, err := New(docSet(capability, skill, iface, agent)).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Satisfies) != 1 || resolved.Satisfies[0] != "t/interfaces/checker@1.0.0" {
		t.Errorf("expected structural satisfaction, got %v", resolved.Satisfies)
	}
	if resolved.Skills[0].DispatchName != "agent.check" {
		t.Errorf("unexpected dispatch name %q", resolved.Skills[0].DispatchName)
	}
}

func TestSchemaPreservationEnforced(t *testing.T) {
	capability := doc("capability", "t/capabilities/check@1.0.0", withSchemas(`{"type":"object"}`, `{"type":"object"}`))
	skill := doc("skill", "t/skills/check@1.0.0", func(d *resource.Document) {
		d.Binds = "t/capabilities/check@1.0.0"
		withSchemas(`{"type":"object","properties":{"extra":{"type":"string"}}}`, `{"type":"object"}`)(d)
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/check@1.0.0"}}
	})
	_, err := New(docSet(capability, skill, agent)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "changes the public contract") {
		t.Fatalf("expected contract error, got %v", err)
	}
}

func TestSchemaComparisonIsCanonical(t *testing.T) {
	// Same schema, different formatting: must be accepted.
	capability := doc("capability", "t/capabilities/check@1.0.0", withSchemas(`{ "type" : "object" }`, `{"type":"object"}`))
	skill := doc("skill", "t/skills/check@1.0.0", func(d *resource.Document) {
		d.Binds = "t/capabilities/check@1.0.0"
		withSchemas(`{"type":"object"}`, `{ "type": "object" }`)(d)
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/check@1.0.0"}}
	})
	if _, err := New(docSet(capability, skill, agent)).ResolveAll(); err != nil {
		t.Fatalf("canonically equal schemas rejected: %v", err)
	}
}

func TestInterfaceEmbeddingCycleDetected(t *testing.T) {
	a := doc("interface", "t/interfaces/a@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/interfaces/b@1.0.0"} })
	b := doc("interface", "t/interfaces/b@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/interfaces/a@1.0.0"} })
	_, err := New(docSet(a, b)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "Interface embedding cycle") {
		t.Fatalf("expected interface cycle error, got %v", err)
	}
}

func TestCapabilityBindingMismatchRejected(t *testing.T) {
	capability := doc("capability", "t/capabilities/check@1.0.0", nil)
	other := doc("capability", "t/capabilities/other@1.0.0", nil)
	skill := doc("skill", "t/skills/check@1.0.0", func(d *resource.Document) {
		d.Binds = "t/capabilities/check@1.0.0"
	})
	otherCap := "t/capabilities/other@1.0.0"
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/check@1.0.0", Capability: &otherCap}}
	})
	_, err := New(docSet(capability, other, skill, agent)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "binding declares capability") {
		t.Fatalf("expected binding mismatch error, got %v", err)
	}
}

func TestPromotedSkillAmbiguityRequiresLocalBinding(t *testing.T) {
	capability := doc("capability", "t/capabilities/check@1.0.0", nil)
	skillA := doc("skill", "t/skills/a@1.0.0", func(d *resource.Document) { d.Binds = "t/capabilities/check@1.0.0" })
	skillB := doc("skill", "t/skills/b@1.0.0", func(d *resource.Document) { d.Binds = "t/capabilities/check@1.0.0" })
	profileA := doc("profile", "t/profiles/a@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/a@1.0.0"}}
	})
	profileB := doc("profile", "t/profiles/b@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/b@1.0.0"}}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/profiles/a@1.0.0", "t/profiles/b@1.0.0"}
	})
	_, err := New(docSet(capability, skillA, skillB, profileA, profileB, agent)).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "bind the capability") {
		t.Fatalf("expected capability ambiguity error, got %v", err)
	}
}

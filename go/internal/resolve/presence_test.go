package resolve

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func capabilityRef(id string) *string { return &id }

// mandateSetup: a governance profile that mandates capability C without binding
// it — an abstract requirement its embedders must satisfy (ADR-0016).
func mandateSetup() map[string]*resource.Document {
	capC := doc("capability", "t/cap/c@1.0.0", nil)
	control := doc("profile", "t/control@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Capability: capabilityRef("t/cap/c@1.0.0"), Required: true}}
	})
	return docSet(capC, control)
}

func TestAbstractRequirementUnfulfilledIsCompileError(t *testing.T) {
	set := mandateSetup()
	set["t/agent@1.0.0"] = doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/control@1.0.0"}
	})
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("an agent embedding an unfulfilled mandate must not resolve, got %v", err)
	}
	if !strings.Contains(err.Error(), "t/control@1.0.0") {
		t.Errorf("the error should name the mandating component, got %v", err)
	}
}

func TestAbstractRequirementFulfilledByEmbedder(t *testing.T) {
	set := mandateSetup()
	set["t/skills/s@1.0.0"] = doc("skill", "t/skills/s@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	set["t/agent@1.0.0"] = doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/control@1.0.0"}
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s@1.0.0"}}
	})
	resolved, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatalf("binding the mandated capability must satisfy the requirement: %v", err)
	}
	if len(resolved.Skills) != 1 {
		t.Fatalf("expected the fulfilling skill, got %d", len(resolved.Skills))
	}
	// The requirement, not the binding, decides mandatory-ness: this binding
	// never said "required" itself.
	if !resolved.Skills[0].Required {
		t.Error("a skill fulfilling a mandate must be marked required in the resolved agent")
	}
}

func TestProfileMayCarryUnfulfilledRequirement(t *testing.T) {
	// A profile is abstract: it may mandate what it does not bind. Only agents
	// are concrete and must satisfy every mandate.
	set := mandateSetup()
	resolved, err := New(set).resolveComponent("t/control@1.0.0", map[string]bool{}, false)
	if err != nil {
		t.Fatalf("a profile may carry an unfulfilled mandate: %v", err)
	}
	if len(resolved.RequiredCapabilities) != 1 || resolved.RequiredCapabilities[0] != "t/cap/c@1.0.0" {
		t.Errorf("the profile should carry its mandate, got %v", resolved.RequiredCapabilities)
	}
	if len(resolved.Skills) != 0 {
		t.Errorf("an abstract requirement binds nothing, got %d skills", len(resolved.Skills))
	}
}

func TestRequirementPropagatesThroughIntermediateProfile(t *testing.T) {
	set := mandateSetup()
	set["t/mid@1.0.0"] = doc("profile", "t/mid@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/control@1.0.0"}
	})
	set["t/agent@1.0.0"] = doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/mid@1.0.0"}
	})
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "t/cap/c@1.0.0") {
		t.Fatalf("a mandate must survive an intermediate profile, got %v", err)
	}
}

func TestRequiredConcreteBindingMandatesDownstream(t *testing.T) {
	// A concrete required binding both fulfills and mandates: an embedder that
	// carries it inherits the obligation.
	capC := doc("capability", "t/cap/c@1.0.0", nil)
	s := doc("skill", "t/skills/s@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	base := doc("profile", "t/base@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s@1.0.0", Required: true}}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/base@1.0.0"}
	})
	resolved, err := New(docSet(capC, s, base, agent)).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatalf("a promoted required binding is fulfilled by promotion: %v", err)
	}
	if len(resolved.RequiredCapabilities) != 1 {
		t.Errorf("the mandate should propagate to the embedder, got %v", resolved.RequiredCapabilities)
	}
	if !resolved.Skills[0].Required {
		t.Error("required must ride the skill through promotion")
	}
}

func TestRequiredIsPresenceNotMutability(t *testing.T) {
	// Overriding a required (but open) capability is legal: presence is
	// preserved, and which skill fills it is sealing's concern, not this axis'.
	capC := doc("capability", "t/cap/c@1.0.0", nil)
	s1 := doc("skill", "t/skills/s1@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	s2 := doc("skill", "t/skills/s2@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	base := doc("profile", "t/base@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s1@1.0.0", Required: true}}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/base@1.0.0"}
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s2@1.0.0"}}
	})
	resolved, err := New(docSet(capC, s1, s2, base, agent)).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatalf("required must not forbid override; that is sealed's job: %v", err)
	}
	if resolved.Skills[0].ImplementationID != "t/skills/s2@1.0.0" {
		t.Errorf("the overriding skill should win, got %s", resolved.Skills[0].ImplementationID)
	}
	if !resolved.Skills[0].Required {
		t.Error("the capability stays mandatory after a legal override")
	}
}

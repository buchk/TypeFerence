package resolve

import (
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func TestCapabilityInternalByDefault(t *testing.T) {
	set := baseSkillAgent(nil, nil) // capability t/cap/c@1.0.0 has no visibility
	agent, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(agent.ExposedSkills()) != 0 {
		t.Errorf("capabilities are internal by default; expected no exposed skills")
	}
	if agent.Skills[0].Exposed {
		t.Errorf("skill should not be exposed by default")
	}
}

func TestExposedCapabilitySurfaces(t *testing.T) {
	set := baseSkillAgent(nil, nil)
	set["t/cap/c@1.0.0"].Visibility = "exposed"
	agent, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	exposed := agent.ExposedSkills()
	if len(exposed) != 1 || exposed[0].CapabilityID != "t/cap/c@1.0.0" {
		t.Fatalf("expected the exposed capability on the public surface, got %+v", exposed)
	}
}

func TestExposurePromotesThroughEmbedding(t *testing.T) {
	cap := doc("capability", "t/cap/c@1.0.0", func(d *resource.Document) { d.Visibility = "exposed" })
	skill := doc("skill", "t/skills/s@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	profile := doc("profile", "t/profile@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s@1.0.0"}}
	})
	agent := doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/profile@1.0.0"}
	})
	resolved, err := New(docSet(cap, skill, profile, agent)).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.ExposedSkills()) != 1 {
		t.Errorf("exposure should promote through embedding onto the agent's surface")
	}
}

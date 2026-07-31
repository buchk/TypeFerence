package resolve

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// sealedSetup: a deep profile that seals capability C with skill s1.
func sealedSetup() map[string]*resource.Document {
	capC := doc("capability", "t/cap/c@1.0.0", nil)
	s1 := doc("skill", "t/skills/s1@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	deep := doc("profile", "t/deep@1.0.0", func(d *resource.Document) {
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s1@1.0.0", Sealed: true}}
	})
	return docSet(capC, s1, deep)
}

func TestSealedCapabilityCannotBeReboundLocally(t *testing.T) {
	set := sealedSetup()
	set["t/skills/s2@1.0.0"] = doc("skill", "t/skills/s2@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	set["t/agent@1.0.0"] = doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/deep@1.0.0"}
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s2@1.0.0"}} // rebinds sealed C
	})
	_, err := New(set).Resolve("t/agent@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "sealed") {
		t.Fatalf("expected sealed-rebind error, got %v", err)
	}
}

func TestSealedCapabilityAllowsAdjacentExtension(t *testing.T) {
	set := sealedSetup()
	set["t/cap/d@1.0.0"] = doc("capability", "t/cap/d@1.0.0", nil)
	set["t/skills/d@1.0.0"] = doc("skill", "t/skills/d@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/d@1.0.0" })
	set["t/agent@1.0.0"] = doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/deep@1.0.0"}
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/d@1.0.0"}} // adjacent capability, not an override
	})
	resolved, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatalf("adding an adjacent capability alongside a sealed one must be allowed: %v", err)
	}
	if len(resolved.Skills) != 2 {
		t.Errorf("expected the sealed capability plus the added one, got %d skills", len(resolved.Skills))
	}
}

func TestSealedCapabilityShallowerOverrideRejected(t *testing.T) {
	// A mid profile embeds the sealing profile and tries to rebind C: rejected.
	set := sealedSetup()
	set["t/skills/s2@1.0.0"] = doc("skill", "t/skills/s2@1.0.0", func(d *resource.Document) { d.Binds = "t/cap/c@1.0.0" })
	set["t/mid@1.0.0"] = doc("profile", "t/mid@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/deep@1.0.0"}
		d.Skills = []resource.SkillBinding{{Ref: "t/skills/s2@1.0.0"}}
	})
	_, err := New(set).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "sealed") {
		t.Fatalf("expected sealed-override error at the mid profile, got %v", err)
	}
}

func TestSealedCapabilityUntouchedResolves(t *testing.T) {
	// Embedding a sealing profile without touching the sealed capability is fine;
	// the sealed skill is carried through.
	set := sealedSetup()
	set["t/agent@1.0.0"] = doc("agent", "t/agent@1.0.0", func(d *resource.Document) {
		d.Embeds = []string{"t/deep@1.0.0"}
	})
	resolved, err := New(set).Resolve("t/agent@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Skills) != 1 || !resolved.Skills[0].Sealed {
		t.Errorf("expected one carried sealed skill, got %+v", resolved.Skills)
	}
}

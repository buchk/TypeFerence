package compile

import (
	"strings"
	"testing"
)

func TestExtensionFlattensPerModeAndOverridesTheBase(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":       "---\ndescription: Kit.\nagents:\n  - agents/team.agent.tfer\nmodes:\n  - manual\n  - pipeline\n---\n",
		"skills/status.skill.tfer":      "---\ndescription: Status.\n---\nBASE_TEXT\n",
		"skills/team-status.skill.tfer": "---\ndescription: Team status.\nextends: skills/status.skill.tfer\nvariants:\n  manual:\n    instructions: MANUAL_ADDITION\n  pipeline:\n    instructions: PIPELINE_ADDITION\n---\n",
		"profiles/base.profile.tfer":    "---\nskills:\n  - skills/status.skill.tfer\n---\n",
		"agents/team.agent.tfer":        "---\ndescription: Team.\nembeds:\n  - profiles/base.profile.tfer\nskills:\n  - skills/team-status.skill.tfer\n---\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{Neutral, AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	manual := readOut(t, out, "agent-plugin", "kit", "skills", "team-status", "SKILL.md")
	if !strings.Contains(manual, "BASE_TEXT\n\nMANUAL_ADDITION") {
		t.Errorf("an extension renders its base's instructions, a blank line, then its own:\n%s", manual)
	}
	pipeline := readOut(t, out, "agent-plugin", "kit-pipeline", "skills", "team-status", "SKILL.md")
	if !strings.Contains(pipeline, "BASE_TEXT\n\nPIPELINE_ADDITION") {
		t.Errorf("the pipeline rendering flattens the pipeline addition:\n%s", pipeline)
	}
	bundle := readOut(t, out, "neutral", "team", "bundle.json")
	if strings.Contains(bundle, `"implementationId": "acme/test/skills/status@1.0.0"`) {
		t.Errorf("binding the extension replaces the inherited base binding:\n%s", bundle)
	}
	if !strings.Contains(bundle, `"dispatchName": "team.status"`) {
		t.Errorf("the extension keeps its base's capability, so the dispatch name is unchanged:\n%s", bundle)
	}
}

func TestSealedSkillCannotBeExtended(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/ext.skill.tfer\n---\n",
		"skills/base.skill.tfer":  "---\ndescription: Base.\nsealed: true\n---\nBase.\n",
		"skills/ext.skill.tfer":   "---\ndescription: Ext.\nextends: skills/base.skill.tfer\n---\nMore.\n",
	})
	if _, err := Build(src, t.TempDir(), []Target{AgentPlugin}, nil); err == nil || !strings.Contains(err.Error(), "sealed") {
		t.Fatalf("extending a sealed skill must fail, got %v", err)
	}
}

func TestCompatibilityReportNamesCompetingPlugins(t *testing.T) {
	src := writePackage(t, []string{"plugins/core.plugin.tfer", "plugins/team.plugin.tfer"}, map[string]string{
		"plugins/core.plugin.tfer":      "---\ndescription: Core.\nskills:\n  - skills/status.skill.tfer\n---\n",
		"plugins/team.plugin.tfer":      "---\ndescription: Team.\nskills:\n  - skills/team-status.skill.tfer\n---\n",
		"skills/status.skill.tfer":      "---\ndescription: Status.\n---\nStatus.\n",
		"skills/team-status.skill.tfer": "---\ndescription: Team status.\nextends: skills/status.skill.tfer\n---\nMore.\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	report := readOut(t, out, "agent-plugin", ".typeference", "compatibility.json")
	if !strings.Contains(report, `"plugin": "core"`) || !strings.Contains(report, `"skill": "team-status"`) {
		t.Fatalf("plugins shipping different members of one family must be reported:\n%s", report)
	}
}

func TestMarketplaceIndexListsEveryArtifact(t *testing.T) {
	src := writePackage(t, nil, map[string]string{
		"typeference.tfer":        "---\nschemaVersion: 6\nname: acme/test\nversion: 2.3.4\nmarketplace:\n  name: acme-market\n  owner: Acme Platform\nplugins:\n  - plugins/kit.plugin.tfer\n---\n",
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/s.skill.tfer\nmodes:\n  - manual\n  - pipeline\n---\n",
		"skills/s.skill.tfer":     "---\ndescription: S.\n---\nS.\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	index := readOut(t, out, "agent-plugin", ".github", "plugin", "marketplace.json")
	for _, want := range []string{`"name": "acme-market"`, `"name": "Acme Platform"`, `"source": "./kit"`, `"source": "./kit-pipeline"`, `"version": "2.3.4"`} {
		if !strings.Contains(index, want) {
			t.Errorf("marketplace.json is missing %s:\n%s", want, index)
		}
	}
}

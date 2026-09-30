package compile

import (
	"strings"
	"testing"
)

func TestBundleEmitsHeldContext(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":             "---\ndescription: Kit.\nagents:\n  - agents/agent.agent.tfer\n---\n",
		"context-types/cast.contexttype.tfer": "---\nfields:\n  owner:\n    type: string\n    required: true\n  governed:\n    type: boolean\n    default: false\n---\n",
		"notes/n.context.tfer":                "---\ncontextType: context-types/cast.contexttype.tfer\nvalues:\n  owner: Dana\n---\n",
		"agents/agent.agent.tfer":             "---\ndescription: Holds a note.\ncontext:\n  - notes/n.context.tfer\n---\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	bundle := readOut(t, out, "neutral", "agent", "bundle.json")
	if !strings.Contains(bundle, `"context"`) || !strings.Contains(bundle, "acme/test/notes/n@1.0.0") {
		t.Errorf("bundle should list held context objects:\n%s", bundle)
	}
	if !strings.Contains(bundle, "acme/test/context-types/cast@1.0.0") {
		t.Errorf("held context should carry its contextType")
	}
	if !strings.Contains(bundle, `"owner": "Dana"`) || !strings.Contains(bundle, `"governed": false`) {
		t.Errorf("bundle should preserve values and materialized defaults:\n%s", bundle)
	}
}

func TestBundleOmitsContextWhenNoneHeld(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/agent.agent.tfer\n---\n",
		"agents/agent.agent.tfer": "---\ndescription: Holds nothing.\n---\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	if bundle := readOut(t, out, "neutral", "agent", "bundle.json"); strings.Contains(bundle, `"context"`) {
		t.Errorf("an agent holding no context must not emit a context member:\n%s", bundle)
	}
}

func TestSkillOwnedContextTravelsWithTheSkill(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":    "---\ndescription: Kit.\nskills:\n  - skills/style.skill.tfer\n---\n",
		"context/style.context.tfer": "---\ndisplayName: House style\n---\nShort sentences.\n",
		"skills/style.skill.tfer":    "---\ndescription: Edit to the house style.\ncontext:\n  - context/style.context.tfer\n---\nEdit the text.\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	skill := readOut(t, out, "agent-plugin", "kit", "skills", "style", "SKILL.md")
	if !strings.Contains(skill, "## Context") || !strings.Contains(skill, "### House style") || !strings.Contains(skill, "Short sentences.") {
		t.Fatalf("a skill's own context must render into its SKILL.md:\n%s", skill)
	}
}

func TestDescriptionNeverEntersInstructionBodies(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"agents/a.agent.tfer":     "---\ndescription: ROUTING-ONLY text.\n---\nYou do the work.\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{Neutral, AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	neutral := readOut(t, out, "neutral", "a", "AGENTS.md")
	if strings.Contains(neutral, "ROUTING-ONLY") || !strings.Contains(neutral, "You do the work.") {
		t.Errorf("the neutral index carries objectives, never the description:\n%s", neutral)
	}
	agent := readOut(t, out, "agent-plugin", "kit", "com.github.copilot", "agents", "a.agent.md")
	body := agent[strings.Index(agent, "\n---\n")+5:]
	if !strings.Contains(agent, `description: "ROUTING-ONLY text."`) || strings.Contains(body, "ROUTING-ONLY") {
		t.Errorf("the plugin agent carries its description only in frontmatter:\n%s", agent)
	}
}

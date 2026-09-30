package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const multimodalFiles = "---\ndescription: S.\nvariants:\n  pipeline:\n    instructions: PIPELINE_TEXT\n  manual:\n    instructions: MANUAL_TEXT\n  a2a:\n    instructions: A2A_TEXT\n---\n"

func TestNeutralVariantFanout(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/agent.agent.tfer\n---\n",
		"skills/s.skill.tfer":     multimodalFiles,
		"agents/agent.agent.tfer": "---\ndescription: Agent.\nskills:\n  - skills/s.skill.tfer\n---\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(out, "neutral", "agent", "skills", "s")
	for _, name := range []string{"SKILL.md", "SKILL.pipeline.md", "SKILL.a2a.md", "SKILL.manual.md"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Errorf("expected %s to be emitted: %v", name, err)
		}
	}
	if !strings.Contains(readOut(t, base, "SKILL.a2a.md"), "A2A_TEXT") {
		t.Errorf("SKILL.a2a.md should carry the a2a variant's instructions")
	}
	if !strings.Contains(readOut(t, base, "SKILL.md"), "PIPELINE_TEXT") {
		t.Errorf("neutral SKILL.md should render the default (pipeline-preferred) variant")
	}
}

func TestPluginArtifactsRenderOneModeEach(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/agent.agent.tfer\nmodes:\n  - manual\n  - pipeline\n---\n",
		"skills/s.skill.tfer":     multimodalFiles,
		"agents/agent.agent.tfer": "---\ndescription: Agent.\nskills:\n  - skills/s.skill.tfer\n---\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	manual := readOut(t, out, "agent-plugin", "kit", "skills", "s", "SKILL.md")
	pipeline := readOut(t, out, "agent-plugin", "kit-pipeline", "skills", "s", "SKILL.md")
	if !strings.Contains(manual, "MANUAL_TEXT") || strings.Contains(manual, "PIPELINE_TEXT") {
		t.Errorf("the manual artifact renders only the manual variant:\n%s", manual)
	}
	if !strings.Contains(pipeline, "PIPELINE_TEXT") || strings.Contains(pipeline, "MANUAL_TEXT") {
		t.Errorf("the pipeline artifact renders only the pipeline variant:\n%s", pipeline)
	}
	entries, err := os.ReadDir(filepath.Join(out, "agent-plugin", "kit", "skills", "s"))
	if err != nil || len(entries) != 1 {
		t.Errorf("a plugin skill directory holds exactly SKILL.md, got %v", entries)
	}
	manifest := readOut(t, out, "agent-plugin", "kit-pipeline", "plugin.json")
	if !strings.Contains(manifest, `"name": "kit-pipeline"`) || !strings.Contains(manifest, "agent-plugins.org/schemas/1.0.0/plugin.schema.json") {
		t.Errorf("each artifact is an Agent Plugins 1.0 plugin named for its mode:\n%s", manifest)
	}
}

func TestUnimodalSkillNoFanout(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/agent.agent.tfer\n---\n",
		"skills/s.skill.tfer":     "---\ndescription: S.\n---\nDo it.\n",
		"agents/agent.agent.tfer": "---\ndescription: Agent.\nskills:\n  - skills/s.skill.tfer\n---\n",
	})
	out := t.TempDir()
	if _, err := Build(src, out, []Target{Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(out, "neutral", "agent", "skills", "s"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" {
		t.Errorf("unimodal skill should emit only SKILL.md, got %v", entries)
	}
}

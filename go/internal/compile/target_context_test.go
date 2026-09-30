package compile

import (
	"strings"
	"testing"
)

func TestPluginValidatesOnlyItsModesContext(t *testing.T) {
	files := map[string]string{
		"context-types/runtime.contexttype.tfer": "---\n---\n",
		"skills/s.skill.tfer":                    "---\ndescription: S.\nvariants:\n  manual:\n    instructions: interactive\n  a2a:\n    instructions: remote\n    requiresContextTypes:\n      - context-types/runtime.contexttype.tfer\n---\n",
		"agents/a.agent.tfer":                    "---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n---\n",
		"plugins/kit.plugin.tfer":                "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
	}
	if _, err := Build(writePackage(t, []string{"plugins/kit.plugin.tfer"}, files), t.TempDir(), []Target{AgentPlugin}, nil); err != nil {
		t.Fatalf("a manual plugin must ignore a2a-only context: %v", err)
	}
	if _, err := Build(writePackage(t, []string{"plugins/kit.plugin.tfer"}, files), t.TempDir(), []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "a2a requires context type") {
		t.Fatalf("neutral carries every mode and must reject missing a2a context, got %v", err)
	}
}

func TestAgentDependentSkillShipsOnlyThroughAnAgent(t *testing.T) {
	files := map[string]string{
		"context-types/team.contexttype.tfer": "---\nbody:\n  type: text\n  required: true\n---\n",
		"context/team.context.tfer":           "---\ncontextType: context-types/team.contexttype.tfer\n---\nThe team.\n",
		"skills/s.skill.tfer":                 "---\ndescription: S.\nrequiresContextTypes:\n  - context-types/team.contexttype.tfer\n---\nUse the team.\n",
		"agents/a.agent.tfer":                 "---\ndescription: A.\ncontext:\n  - context/team.context.tfer\nskills:\n  - skills/s.skill.tfer\n---\n",
	}
	throughAgent := map[string]string{"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n"}
	direct := map[string]string{"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/s.skill.tfer\n---\n"}
	for name, content := range files {
		throughAgent[name] = content
		direct[name] = content
	}
	if _, err := Build(writePackage(t, []string{"plugins/kit.plugin.tfer"}, throughAgent), t.TempDir(), []Target{AgentPlugin}, nil); err != nil {
		t.Fatalf("an agent that holds the context may ship the skill: %v", err)
	}
	if _, err := Build(writePackage(t, []string{"plugins/kit.plugin.tfer"}, direct), t.TempDir(), []Target{AgentPlugin}, nil); err == nil ||
		!strings.Contains(err.Error(), "only an agent can provide") {
		t.Fatalf("shipping an agent-dependent skill directly must fail, got %v", err)
	}
}

func TestTargetNativeNameCollisionsFailClosed(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":  "---\ndescription: Kit.\nagents:\n  - team-a/worker.agent.tfer\n  - team-b/worker.agent.tfer\n---\n",
		"team-a/worker.agent.tfer": "---\ndescription: A.\n---\n",
		"team-b/worker.agent.tfer": "---\ndescription: B.\n---\n",
	})
	if _, err := Build(src, t.TempDir(), []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "same target artifact path") {
		t.Fatalf("expected colliding agent artifact names to fail, got %v", err)
	}
}

func TestSkillNameCollisionsFailClosed(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":  "---\ndescription: Kit.\nskills:\n  - team-a/status.skill.tfer\n  - team-b/status.skill.tfer\n---\n",
		"team-a/status.skill.tfer": "---\ndescription: A.\n---\nA.\n",
		"team-b/status.skill.tfer": "---\ndescription: B.\n---\nB.\n",
	})
	if _, err := Build(src, t.TempDir(), []Target{AgentPlugin}, nil); err == nil ||
		!strings.Contains(err.Error(), "both emit the skill name") {
		t.Fatalf("expected colliding skill names to fail, got %v", err)
	}
}

func TestInvalidHostNamesAreNeverRewritten(t *testing.T) {
	src := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":   "---\ndescription: Kit.\nskills:\n  - skills/foo.bar.skill.tfer\n---\n",
		"skills/foo.bar.skill.tfer": "---\ndescription: Dotted.\n---\nDotted.\n",
	})
	if _, err := Build(src, t.TempDir(), []Target{AgentPlugin}, nil); err == nil || !strings.Contains(err.Error(), "Agent Skills") {
		t.Fatalf("an invalid skill name must fail rather than be rewritten, got %v", err)
	}
}

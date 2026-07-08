package eval

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
)

// DefaultModel is the executor and judge model when none is configured.
const DefaultModel = "claude-opus-4-8"

// ResponsePlaceholder stands in for the agent response inside dry-run judge
// payloads, which are emitted before any execution happens.
const ResponsePlaceholder = "<agent-response>"

const maxTokens = 16000

// BuildExecutorPayload constructs the exact Messages API request body that
// runs the scenario task against the compiled agent definition. The system
// prompt is the neutral-target instruction surface: the agent's rendered
// instructions, the focused skill's instructions (if any), and the content of
// the agent's context files read from the source root.
func BuildExecutorPayload(model string, agent *resolve.ResolvedAgent, skill *resolve.ResolvedSkill, sourceRoot, task string) ([]byte, error) {
	system, err := renderSystemPrompt(agent, skill, sourceRoot)
	if err != nil {
		return nil, err
	}
	payload := jsonx.Obj{
		{K: "model", V: jsonx.Str(model)},
		{K: "max_tokens", V: jsonx.Int(maxTokens)},
		{K: "thinking", V: jsonx.Obj{{K: "type", V: jsonx.Str("adaptive")}}},
		{K: "system", V: jsonx.Str(system)},
		{K: "messages", V: jsonx.Arr{jsonx.Obj{
			{K: "role", V: jsonx.Str("user")},
			{K: "content", V: jsonx.Str(task)},
		}}},
	}
	return []byte(jsonx.Indented(payload) + "\n"), nil
}

// BuildJudgePayload constructs the exact Messages API request body that
// grades a response against the scenario rubric. Structured outputs constrain
// the verdicts to a machine-readable shape.
func BuildJudgePayload(model string, scenario *Scenario, response string) []byte {
	var prompt strings.Builder
	prompt.WriteString("You are grading whether an AI agent's response adheres to a rubric.\n")
	prompt.WriteString("Grade each rubric item independently and literally against the response text. ")
	prompt.WriteString("Do not reward intent or effort; pass an item only when the response actually satisfies it.\n\n")
	prompt.WriteString("## Task given to the agent\n\n")
	prompt.WriteString(scenario.Task)
	prompt.WriteString("\n\n## Agent response\n\n")
	prompt.WriteString(response)
	prompt.WriteString("\n\n## Rubric\n\n")
	for _, item := range scenario.Rubric {
		prompt.WriteString("- ")
		prompt.WriteString(item.ID)
		prompt.WriteString(": ")
		prompt.WriteString(item.Requirement)
		prompt.WriteString("\n")
	}
	prompt.WriteString("\nReturn a verdict for every rubric item, in the order listed.")

	itemSchema := jsonx.Obj{
		{K: "type", V: jsonx.Str("object")},
		{K: "properties", V: jsonx.Obj{
			{K: "id", V: jsonx.Obj{{K: "type", V: jsonx.Str("string")}}},
			{K: "passed", V: jsonx.Obj{{K: "type", V: jsonx.Str("boolean")}}},
			{K: "reasoning", V: jsonx.Obj{{K: "type", V: jsonx.Str("string")}}},
		}},
		{K: "required", V: jsonx.Arr{jsonx.Str("id"), jsonx.Str("passed"), jsonx.Str("reasoning")}},
		{K: "additionalProperties", V: jsonx.Bool(false)},
	}
	schema := jsonx.Obj{
		{K: "type", V: jsonx.Str("object")},
		{K: "properties", V: jsonx.Obj{
			{K: "verdicts", V: jsonx.Obj{
				{K: "type", V: jsonx.Str("array")},
				{K: "items", V: itemSchema},
			}},
		}},
		{K: "required", V: jsonx.Arr{jsonx.Str("verdicts")}},
		{K: "additionalProperties", V: jsonx.Bool(false)},
	}

	payload := jsonx.Obj{
		{K: "model", V: jsonx.Str(model)},
		{K: "max_tokens", V: jsonx.Int(maxTokens)},
		{K: "thinking", V: jsonx.Obj{{K: "type", V: jsonx.Str("adaptive")}}},
		{K: "output_config", V: jsonx.Obj{
			{K: "format", V: jsonx.Obj{
				{K: "type", V: jsonx.Str("json_schema")},
				{K: "schema", V: schema},
			}},
		}},
		{K: "messages", V: jsonx.Arr{jsonx.Obj{
			{K: "role", V: jsonx.Str("user")},
			{K: "content", V: jsonx.Str(prompt.String())},
		}}},
	}
	return []byte(jsonx.Indented(payload) + "\n")
}

// renderSystemPrompt assembles the instruction surface the neutral target
// gives a host: agent instructions, the focused skill, and context content.
func renderSystemPrompt(agent *resolve.ResolvedAgent, skill *resolve.ResolvedSkill, sourceRoot string) (string, error) {
	var b strings.Builder
	b.WriteString("You are the following agent, compiled from a TypeFerence definition. ")
	b.WriteString("Operate strictly within these instructions.\n\n")
	b.WriteString("# ")
	b.WriteString(agent.DisplayName)
	b.WriteString("\n\n")
	b.WriteString(agent.Description)
	b.WriteString("\n")
	if len(agent.WorkingNorms) > 0 {
		b.WriteString("\n## Working norms\n\n")
		for _, norm := range agent.WorkingNorms {
			b.WriteString("- ")
			b.WriteString(norm)
			b.WriteString("\n")
		}
	}
	if skill != nil {
		b.WriteString("\n## Active skill: ")
		b.WriteString(skill.DispatchName)
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(skill.Instructions))
		b.WriteString("\n")
	}
	contexts := agent.ContextFiles
	if skill != nil {
		contexts = skill.ContextFiles
	}
	for _, relative := range contexts {
		content, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(relative)))
		if err != nil {
			return "", err
		}
		b.WriteString("\n## Context: ")
		b.WriteString(relative)
		b.WriteString("\n\n")
		b.WriteString(strings.ReplaceAll(strings.TrimPrefix(string(content), string(rune(0xFEFF))), "\r\n", "\n"))
		b.WriteString("\n")
	}
	return b.String(), nil
}

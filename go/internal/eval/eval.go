package eval

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
)

// Options configures a run.
type Options struct {
	Model   string
	Live    bool
	OutDir  string    // when set, payloads and the report are written here
	Stdout  io.Writer // defaults to os.Stdout
	Backend Backend   // required in live mode; injectable for tests
}

// Verdict is one rubric item's grade.
type Verdict struct {
	ID        string
	Passed    bool
	Reasoning string
}

// Result is the outcome for one scenario.
type Result struct {
	Scenario string
	Agent    string
	Passed   bool
	Verdicts []Verdict
}

// Run loads scenarios, validates them against the resolved agent definitions
// in source, and either emits the exact request payloads (dry run) or
// executes and judges them (live). It returns the process exit code.
func Run(source, scenariosPath string, opts Options) (int, error) {
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	scenarios, err := LoadScenarios(scenariosPath)
	if err != nil {
		return 0, err
	}
	agents, err := compile.Validate(source, "")
	if err != nil {
		return 0, err
	}
	sourceRoot, err := filepath.Abs(source)
	if err != nil {
		return 0, err
	}

	type prepared struct {
		scenario *Scenario
		agent    *resolve.ResolvedAgent
		skill    *resolve.ResolvedSkill
		executor []byte
	}
	preparedScenarios := make([]prepared, 0, len(scenarios))
	for _, scenario := range scenarios {
		agent := findAgent(agents, scenario.Agent)
		if agent == nil {
			return 0, fmt.Errorf("%s: agent not found in source: %s", scenario.Path, scenario.Agent)
		}
		var skill *resolve.ResolvedSkill
		if scenario.Skill != "" {
			skill = findSkill(agent, scenario.Skill)
			if skill == nil {
				return 0, fmt.Errorf("%s: skill dispatch name not found on %s: %s", scenario.Path, agent.ID, scenario.Skill)
			}
		}
		executor, buildErr := BuildExecutorPayload(opts.Model, agent, skill, sourceRoot, scenario.Task)
		if buildErr != nil {
			return 0, buildErr
		}
		preparedScenarios = append(preparedScenarios, prepared{scenario, agent, skill, executor})
	}

	if !opts.Live {
		for _, p := range preparedScenarios {
			judge := BuildJudgePayload(opts.Model, p.scenario, ResponsePlaceholder)
			if err := emitPayload(opts, p.scenario.ID, "executor-request.json", p.executor); err != nil {
				return 0, err
			}
			if err := emitPayload(opts, p.scenario.ID, "judge-request.json", judge); err != nil {
				return 0, err
			}
		}
		fmt.Fprintf(opts.Stdout, "Dry run: %d scenario(s) validated; request payloads emitted. No API calls were made.\n", len(preparedScenarios))
		return 0, nil
	}

	if opts.Backend == nil {
		return 0, fmt.Errorf("live mode requires a backend")
	}
	ctx := context.Background()
	results := make([]Result, 0, len(preparedScenarios))
	allPassed := true
	for _, p := range preparedScenarios {
		response, execErr := opts.Backend.Complete(ctx, p.executor)
		if execErr != nil {
			return 0, fmt.Errorf("%s: executor call failed: %v", p.scenario.ID, execErr)
		}
		judgePayload := BuildJudgePayload(opts.Model, p.scenario, response)
		judgeText, judgeErr := opts.Backend.Complete(ctx, judgePayload)
		if judgeErr != nil {
			return 0, fmt.Errorf("%s: judge call failed: %v", p.scenario.ID, judgeErr)
		}
		verdicts, parseErr := parseVerdicts(judgeText, p.scenario)
		if parseErr != nil {
			return 0, fmt.Errorf("%s: %v", p.scenario.ID, parseErr)
		}
		passed := true
		for _, v := range verdicts {
			if !v.Passed {
				passed = false
			}
		}
		if !passed {
			allPassed = false
		}
		results = append(results, Result{Scenario: p.scenario.ID, Agent: p.agent.ID, Passed: passed, Verdicts: verdicts})
		if opts.OutDir != "" {
			if err := emitPayload(opts, p.scenario.ID, "response.txt", []byte(response)); err != nil {
				return 0, err
			}
		}
	}

	report := renderReport(opts.Model, results, allPassed)
	fmt.Fprintln(opts.Stdout, report)
	if opts.OutDir != "" {
		if err := os.WriteFile(filepath.Join(opts.OutDir, "report.json"), []byte(report+"\n"), 0o644); err != nil {
			return 0, err
		}
	}
	if !allPassed {
		return 1, nil
	}
	return 0, nil
}

func renderReport(model string, results []Result, allPassed bool) string {
	resultArr := jsonx.Arr{}
	for _, r := range results {
		verdictArr := jsonx.Arr{}
		for _, v := range r.Verdicts {
			verdictArr = append(verdictArr, jsonx.Obj{
				{K: "id", V: jsonx.Str(v.ID)},
				{K: "passed", V: jsonx.Bool(v.Passed)},
				{K: "reasoning", V: jsonx.Str(v.Reasoning)},
			})
		}
		resultArr = append(resultArr, jsonx.Obj{
			{K: "scenario", V: jsonx.Str(r.Scenario)},
			{K: "agent", V: jsonx.Str(r.Agent)},
			{K: "passed", V: jsonx.Bool(r.Passed)},
			{K: "verdicts", V: verdictArr},
		})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "model", V: jsonx.Str(model)},
		{K: "passed", V: jsonx.Bool(allPassed)},
		{K: "results", V: resultArr},
	})
}

// parseVerdicts reads the judge's structured output and checks it covers the
// rubric exactly.
func parseVerdicts(judgeText string, scenario *Scenario) ([]Verdict, error) {
	parsed, err := jsonx.Parse(strings.TrimSpace(judgeText))
	if err != nil {
		return nil, fmt.Errorf("judge output is not valid JSON: %v", err)
	}
	obj, ok := parsed.(jsonx.Obj)
	if !ok {
		return nil, fmt.Errorf("judge output is not a JSON object")
	}
	arr, ok := member(obj, "verdicts").(jsonx.Arr)
	if !ok {
		return nil, fmt.Errorf("judge output has no verdicts array")
	}
	verdicts := make([]Verdict, 0, len(arr))
	seen := map[string]bool{}
	for _, item := range arr {
		itemObj, isObj := item.(jsonx.Obj)
		if !isObj {
			return nil, fmt.Errorf("judge verdict is not an object")
		}
		v := Verdict{}
		if s, isStr := member(itemObj, "id").(jsonx.Str); isStr {
			v.ID = string(s)
		}
		if b, isBool := member(itemObj, "passed").(jsonx.Bool); isBool {
			v.Passed = bool(b)
		}
		if s, isStr := member(itemObj, "reasoning").(jsonx.Str); isStr {
			v.Reasoning = string(s)
		}
		seen[v.ID] = true
		verdicts = append(verdicts, v)
	}
	for _, item := range scenario.Rubric {
		if !seen[item.ID] {
			return nil, fmt.Errorf("judge output is missing a verdict for rubric item %q", item.ID)
		}
	}
	return verdicts, nil
}

func emitPayload(opts Options, scenarioID, name string, payload []byte) error {
	if opts.OutDir == "" {
		fmt.Fprintf(opts.Stdout, "--- %s: %s ---\n%s\n", scenarioID, name, payload)
		return nil
	}
	dir := filepath.Join(opts.OutDir, slug(scenarioID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), payload, 0o644)
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

func slug(id string) string {
	s := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(id), "-"), "-")
	if s == "" {
		return "scenario"
	}
	return s
}

func findAgent(agents []*resolve.ResolvedAgent, id string) *resolve.ResolvedAgent {
	for _, agent := range agents {
		if agent.ID == id && agent.Emit {
			return agent
		}
	}
	return nil
}

func findSkill(agent *resolve.ResolvedAgent, dispatchName string) *resolve.ResolvedSkill {
	for i := range agent.Skills {
		if agent.Skills[i].DispatchName == dispatchName {
			return &agent.Skills[i]
		}
	}
	return nil
}

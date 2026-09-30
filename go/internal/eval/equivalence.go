package eval

// BETH — the Behavioral Equivalence Test Harness (ADR-0009). Where eval.Run
// measures the definition through the neutral instruction surface, Pack and
// Score measure the compiled deployment surfaces: Pack lays out one cell per
// scenario x surface holding the actual target bundle a host would consume,
// an operator collects one response per cell from a real host, and Score
// judges the responses and reports adherence (per surface) and agreement
// (across surfaces) separately.

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
)

// PackOptions configures an equivalence pack.
type PackOptions struct {
	Targets []compile.Target // defaults to all targets
	Stdout  io.Writer        // defaults to os.Stdout
}

// Cell file names, shared by Pack and Score.
const (
	cellFileName          = "cell.json"
	promptFileName        = "PROMPT.txt"
	workspaceDirName      = "workspace"
	responseFileName      = "response.md"
	runtimeFileName       = "runtime.json"
	judgeRequestFileName  = "judge-request.json"
	judgeResponseFileName = "judge-response.json"
	manifestFileName      = "manifest.json"
)

// Pack compiles each scenario's agent into every requested target and lays
// out a self-contained run directory: one cell per scenario x surface with
// the compiled bundle, materialized context files, the task prompt, and the
// scenario frozen into cell.json. Pack output is byte-deterministic for
// identical source and scenarios.
func Pack(source, scenariosPath, outDir string, opts PackOptions) (int, error) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []compile.Target{compile.Neutral, compile.AgentPlugin}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })

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
	if entries, readErr := os.ReadDir(outDir); readErr == nil && len(entries) > 0 {
		return 0, fmt.Errorf("run directory already exists and is not empty: %s", outDir)
	}

	type plan struct {
		scenario *Scenario
		agent    *resolve.ResolvedAgent
		skill    *resolve.ResolvedSkill
	}
	plans := make([]plan, 0, len(scenarios))
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
		plans = append(plans, plan{scenario, agent, skill})
	}

	temp, err := os.MkdirTemp("", "typeference-equivalence-")
	if err != nil {
		return 0, fmt.Errorf("cannot create temporary directory: %v", err)
	}
	defer os.RemoveAll(temp)
	if _, err := compile.Build(source, temp, targets, nil); err != nil {
		return 0, err
	}
	sourceDigest, err := compile.HashSource(sourceRoot)
	if err != nil {
		return 0, err
	}

	manifestCells := jsonx.Arr{}
	cellCount := 0
	for _, p := range plans {
		for _, target := range targets {
			cellRel := path.Join("cells", slug(p.scenario.ID), target.String())
			cellDir := filepath.Join(outDir, filepath.FromSlash(cellRel))
			workspace := filepath.Join(cellDir, workspaceDirName)
			bundleDir := filepath.Join(temp, target.String(), resolve.Leaf(p.agent.ID))
			if target == compile.AgentPlugin {
				// The agent-plugin surface is the installable plugin that ships
				// the agent, rendered for interactive use.
				dir, findErr := pluginShipping(filepath.Join(temp, target.String()), p.agent.ID)
				if findErr != nil {
					return 0, fmt.Errorf("%s: %v", p.scenario.Path, findErr)
				}
				bundleDir = dir
			}
			if err := copyTree(bundleDir, workspace); err != nil {
				return 0, err
			}
			compiledContext := []string{}
			for _, context := range p.agent.ContextObjects {
				compiledContext = append(compiledContext, context.ID)
			}
			if err := os.WriteFile(filepath.Join(cellDir, promptFileName), []byte(p.scenario.Task), 0o644); err != nil {
				return 0, err
			}
			digest, hashErr := compile.HashDirectory(workspace)
			if hashErr != nil {
				return 0, hashErr
			}
			cell := jsonx.Obj{
				{K: "schemaVersion", V: jsonx.Int(1)},
				{K: "scenario", V: scenarioJSON(p.scenario)},
				{K: "surface", V: jsonx.Str(target.String())},
				{K: "compiledContext", V: stringArrJSON(compiledContext)},
				{K: "workspaceDigest", V: jsonx.Str(digest)},
			}
			if err := os.WriteFile(filepath.Join(cellDir, cellFileName), []byte(jsonx.Indented(cell)+"\n"), 0o644); err != nil {
				return 0, err
			}
			manifestCells = append(manifestCells, jsonx.Obj{
				{K: "scenario", V: jsonx.Str(p.scenario.ID)},
				{K: "surface", V: jsonx.Str(target.String())},
				{K: "path", V: jsonx.Str(cellRel)},
				{K: "workspaceDigest", V: jsonx.Str(digest)},
			})
			cellCount++
		}
	}

	surfaces := make([]string, 0, len(targets))
	for _, target := range targets {
		surfaces = append(surfaces, target.String())
	}
	manifest := jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Int(1)},
		{K: "sourceDigest", V: jsonx.Str(sourceDigest)},
		{K: "surfaces", V: stringArrJSON(surfaces)},
		{K: "cells", V: manifestCells},
	}
	if err := os.WriteFile(filepath.Join(outDir, manifestFileName), []byte(jsonx.Indented(manifest)+"\n"), 0o644); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "README.md"), []byte(runReadme), 0o644); err != nil {
		return 0, err
	}
	fmt.Fprintf(opts.Stdout, "Packed %d cells (%d scenarios x %d surfaces) at %s\n", cellCount, len(plans), len(targets), outDir)
	return 0, nil
}

// pluginShipping finds the manual-mode plugin artifact that ships an agent:
// the first by directory name whose bundle lists it.
func pluginShipping(targetRoot, agentID string) (string, error) {
	entries, err := os.ReadDir(targetRoot)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(targetRoot, entry.Name(), ".typeference", "bundle.json"))
		if readErr != nil {
			continue
		}
		value, parseErr := jsonx.Parse(string(raw))
		if parseErr != nil {
			continue
		}
		bundle, ok := value.(jsonx.Obj)
		if !ok || jsonMemberString(bundle, "mode") != "manual" {
			continue
		}
		agents, _ := member(bundle, "agents").(jsonx.Arr)
		for _, agent := range agents {
			if obj, isObj := agent.(jsonx.Obj); isObj && jsonMemberString(obj, "id") == agentID {
				return filepath.Join(targetRoot, entry.Name()), nil
			}
		}
	}
	return "", fmt.Errorf("agent %s is not shipped by any plugin; link it from a plugin to measure the agent-plugin surface", agentID)
}

const runReadme = `# BETH run directory

One cell per scenario x surface, under cells/<scenario>/<surface>/. To collect
a cell: open a host with workspace/ as its working directory, submit the
content of PROMPT.txt verbatim as the first message, and save the host's final
response text as response.md in the cell directory (next to workspace/, not
inside it). An agent-plugin cell's workspace/ is an installable plugin: install
it (for example "copilot plugin install ./workspace") and select the agent
before submitting the prompt. Optionally record the host in runtime.json:

    {"host": "claude-code 3.7.0", "model": "claude-fable-5"}

Do not edit anything inside workspace/ — scoring verifies the workspace digest
and excludes modified cells. When responses are collected, run:

    typeference equivalence score <this-directory>          # emits judge payloads
    typeference equivalence score <this-directory> --live   # judges via ANTHROPIC_API_KEY

A pre-existing judge-response.json in a cell (matching the judge output schema)
takes precedence over --live, so any operator or model may act as judge; the
scorecard records which path graded each cell. See evals/README.md and
ADR-0009 for what a scorecard does and does not establish.
`

func scenarioJSON(s *Scenario) jsonx.Obj {
	rubric := jsonx.Arr{}
	for _, item := range s.Rubric {
		rubric = append(rubric, jsonx.Obj{
			{K: "id", V: jsonx.Str(item.ID)},
			{K: "requirement", V: jsonx.Str(item.Requirement)},
		})
	}
	return jsonx.Obj{
		{K: "id", V: jsonx.Str(s.ID)},
		{K: "agent", V: jsonx.Str(s.Agent)},
		{K: "skill", V: jsonx.Str(s.Skill)},
		{K: "task", V: jsonx.Str(s.Task)},
		{K: "rubric", V: rubric},
	}
}

func stringArrJSON(values []string) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, v := range values {
		arr = append(arr, jsonx.Str(v))
	}
	return arr
}

func copyTree(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return fmt.Errorf("cannot read bundle directory: %s", from)
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		source := filepath.Join(from, entry.Name())
		destination := filepath.Join(to, entry.Name())
		if entry.IsDir() {
			if err := copyTree(source, destination); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(source, destination); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(from, to string) error {
	content, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("cannot read file: %s", from)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, content, 0o644)
}

// scenarioFromCell reconstructs the frozen scenario out of a parsed cell.json.
func scenarioFromCell(cellPath string, cell jsonx.Obj) (*Scenario, error) {
	scenarioObj, ok := member(cell, "scenario").(jsonx.Obj)
	if !ok {
		return nil, fmt.Errorf("%s: cell has no scenario object", cellPath)
	}
	scenario := &Scenario{
		SchemaVersion: 1,
		ID:            jsonMemberString(scenarioObj, "id"),
		Agent:         jsonMemberString(scenarioObj, "agent"),
		Skill:         jsonMemberString(scenarioObj, "skill"),
		Task:          jsonMemberString(scenarioObj, "task"),
		Path:          cellPath,
	}
	rubricArr, ok := member(scenarioObj, "rubric").(jsonx.Arr)
	if !ok {
		return nil, fmt.Errorf("%s: cell scenario has no rubric array", cellPath)
	}
	for _, item := range rubricArr {
		itemObj, isObj := item.(jsonx.Obj)
		if !isObj {
			return nil, fmt.Errorf("%s: cell rubric item is not an object", cellPath)
		}
		scenario.Rubric = append(scenario.Rubric, RubricItem{
			ID:          jsonMemberString(itemObj, "id"),
			Requirement: jsonMemberString(itemObj, "requirement"),
		})
	}
	if err := validateScenario(scenario); err != nil {
		return nil, fmt.Errorf("%s: %v", cellPath, err)
	}
	return scenario, nil
}

func jsonMemberString(obj jsonx.Obj, key string) string {
	if s, ok := member(obj, key).(jsonx.Str); ok {
		return string(s)
	}
	return ""
}

// ParseTargetList parses the equivalence --target value: "all" or a
// comma-separated list of target names.
func ParseTargetList(value string) ([]compile.Target, error) {
	if strings.EqualFold(strings.TrimSpace(value), "all") || strings.TrimSpace(value) == "" {
		return compile.ParseTargets("all")
	}
	targets := []compile.Target{}
	seen := map[compile.Target]bool{}
	for _, part := range strings.Split(value, ",") {
		parsed, err := compile.ParseTargets(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		for _, target := range parsed {
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}
	}
	return targets, nil
}

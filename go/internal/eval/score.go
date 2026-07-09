package eval

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
)

// ScoreOptions configures an equivalence score.
type ScoreOptions struct {
	Model   string
	Live    bool
	Stdout  io.Writer // defaults to os.Stdout
	Backend Backend   // required in live mode; injectable for tests
}

// Cell statuses reported by Score.
const (
	statusJudged         = "judged"
	statusUnjudged       = "unjudged"
	statusNoResponse     = "no-response"
	statusWorkspaceDrift = "workspace-drift"
)

type scoredCell struct {
	ScenarioID string
	Surface    string
	Path       string // slash-relative to the run directory
	Scenario   *Scenario
	Status     string
	Judge      string // "file" or "anthropic:<model>"; empty unless judged
	Host       string
	HostModel  string
	Verdicts   map[string]Verdict // by rubric item id, when judged
}

// Score judges collected responses in a run directory and writes the
// scorecard. Per cell, a pre-existing judge-response.json wins; --live grades
// via the backend; otherwise the exact judge request payload is emitted and
// the cell is reported unjudged. Without --live, Score is a pure function of
// the run directory. It returns the process exit code: 0 only for a green
// scorecard (ADR-0009: one judged response per surface, all agreeing, none
// failing); 1 otherwise — divergence, failure, or incomplete coverage (a
// surface with no judged response, or a run with no surfaces at all).
func Score(runDir string, opts ScoreOptions) (int, error) {
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Live && opts.Backend == nil {
		return 0, fmt.Errorf("live mode requires a backend")
	}
	manifest, err := readJSONObject(filepath.Join(runDir, manifestFileName))
	if err != nil {
		return 0, err
	}
	surfaces, err := manifestSurfaces(manifest)
	if err != nil {
		return 0, err
	}
	cellsArr, ok := member(manifest, "cells").(jsonx.Arr)
	if !ok {
		return 0, fmt.Errorf("manifest has no cells array")
	}

	cells := make([]*scoredCell, 0, len(cellsArr))
	for _, entry := range cellsArr {
		entryObj, isObj := entry.(jsonx.Obj)
		if !isObj {
			return 0, fmt.Errorf("manifest cell is not an object")
		}
		cell := &scoredCell{
			ScenarioID: jsonMemberString(entryObj, "scenario"),
			Surface:    jsonMemberString(entryObj, "surface"),
			Path:       jsonMemberString(entryObj, "path"),
		}
		if cell.Path == "" {
			return 0, fmt.Errorf("manifest cell has no path")
		}
		cellDir := filepath.Join(runDir, filepath.FromSlash(cell.Path))
		cellObj, readErr := readJSONObject(filepath.Join(cellDir, cellFileName))
		if readErr != nil {
			return 0, readErr
		}
		cell.Scenario, readErr = scenarioFromCell(cell.Path, cellObj)
		if readErr != nil {
			return 0, readErr
		}
		expectedDigest := jsonMemberString(entryObj, "workspaceDigest")
		actualDigest, hashErr := compile.HashDirectory(filepath.Join(cellDir, workspaceDirName))
		if hashErr != nil {
			return 0, hashErr
		}
		if actualDigest != expectedDigest {
			cell.Status = statusWorkspaceDrift
			cells = append(cells, cell)
			continue
		}
		if err := judgeCell(cell, cellDir, opts); err != nil {
			return 0, err
		}
		cells = append(cells, cell)
	}

	card := buildScorecard(opts.Model, surfaces, cells)
	cardJSON := jsonx.Indented(card.json) + "\n"
	if err := os.WriteFile(filepath.Join(runDir, "scorecard.json"), []byte(cardJSON), 0o644); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(runDir, "scorecard.md"), []byte(card.markdown), 0o644); err != nil {
		return 0, err
	}
	fmt.Fprint(opts.Stdout, card.markdown)
	fmt.Fprintf(opts.Stdout, "\nWrote scorecard.json and scorecard.md at %s\n", runDir)
	if !card.passed {
		return 1, nil
	}
	return 0, nil
}

// judgeCell fills in the cell's status, judge provenance, and verdicts.
func judgeCell(cell *scoredCell, cellDir string, opts ScoreOptions) error {
	responsePath := filepath.Join(cellDir, responseFileName)
	responseRaw, responseErr := os.ReadFile(responsePath)
	judgePath := filepath.Join(cellDir, judgeResponseFileName)
	judgeRaw, judgeErr := os.ReadFile(judgePath)

	if responseErr != nil {
		if judgeErr == nil {
			return fmt.Errorf("%s: has %s but no %s", cell.Path, judgeResponseFileName, responseFileName)
		}
		cell.Status = statusNoResponse
		return nil
	}
	response := strings.TrimPrefix(string(responseRaw), string(rune(0xFEFF)))
	if strings.TrimSpace(response) == "" {
		return fmt.Errorf("%s: %s is empty", cell.Path, responseFileName)
	}
	if err := readRuntime(cell, cellDir); err != nil {
		return err
	}

	var judgeText, judgeSource string
	switch {
	case judgeErr == nil:
		judgeText = string(judgeRaw)
		judgeSource = "file"
	case opts.Live:
		payload := BuildJudgePayload(opts.Model, cell.Scenario, response)
		completed, callErr := opts.Backend.Complete(context.Background(), payload)
		if callErr != nil {
			return fmt.Errorf("%s: judge call failed: %v", cell.Path, callErr)
		}
		if err := os.WriteFile(judgePath, []byte(completed+"\n"), 0o644); err != nil {
			return err
		}
		judgeText = completed
		judgeSource = "anthropic:" + opts.Model
	default:
		payload := BuildJudgePayload(opts.Model, cell.Scenario, response)
		if err := os.WriteFile(filepath.Join(cellDir, judgeRequestFileName), payload, 0o644); err != nil {
			return err
		}
		cell.Status = statusUnjudged
		return nil
	}

	verdicts, parseErr := parseVerdicts(judgeText, cell.Scenario)
	if parseErr != nil {
		return fmt.Errorf("%s: %v", cell.Path, parseErr)
	}
	cell.Status = statusJudged
	cell.Judge = judgeSource
	cell.Verdicts = map[string]Verdict{}
	for _, v := range verdicts {
		cell.Verdicts[v.ID] = v
	}
	return nil
}

func readRuntime(cell *scoredCell, cellDir string) error {
	raw, err := os.ReadFile(filepath.Join(cellDir, runtimeFileName))
	if err != nil {
		return nil // runtime.json is optional
	}
	parsed, parseErr := jsonx.Parse(strings.TrimSpace(strings.TrimPrefix(string(raw), string(rune(0xFEFF)))))
	if parseErr != nil {
		return fmt.Errorf("%s: invalid %s: %v", cell.Path, runtimeFileName, parseErr)
	}
	obj, isObj := parsed.(jsonx.Obj)
	if !isObj {
		return fmt.Errorf("%s: %s is not a JSON object", cell.Path, runtimeFileName)
	}
	cell.Host = jsonMemberString(obj, "host")
	cell.HostModel = jsonMemberString(obj, "model")
	return nil
}

func readJSONObject(path string) (jsonx.Obj, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", path)
	}
	parsed, parseErr := jsonx.Parse(strings.TrimSpace(strings.TrimPrefix(string(raw), string(rune(0xFEFF)))))
	if parseErr != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %v", path, parseErr)
	}
	obj, isObj := parsed.(jsonx.Obj)
	if !isObj {
		return nil, fmt.Errorf("%s: not a JSON object", path)
	}
	return obj, nil
}

func manifestSurfaces(manifest jsonx.Obj) ([]string, error) {
	arr, ok := member(manifest, "surfaces").(jsonx.Arr)
	if !ok {
		return nil, fmt.Errorf("manifest has no surfaces array")
	}
	surfaces := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, isStr := v.(jsonx.Str); isStr {
			surfaces = append(surfaces, string(s))
		}
	}
	if len(surfaces) == 0 {
		return nil, fmt.Errorf("manifest surfaces array is empty")
	}
	return surfaces, nil
}

// --- scorecard ---------------------------------------------------------------

type scorecard struct {
	json     jsonx.Obj
	markdown string
	passed   bool
}

type scenarioGroup struct {
	id    string
	cells []*scoredCell // one per surface, manifest order
}

func buildScorecard(model string, surfaces []string, cells []*scoredCell) *scorecard {
	groups := []*scenarioGroup{}
	byID := map[string]*scenarioGroup{}
	for _, cell := range cells {
		group := byID[cell.ScenarioID]
		if group == nil {
			group = &scenarioGroup{id: cell.ScenarioID}
			byID[cell.ScenarioID] = group
			groups = append(groups, group)
		}
		group.cells = append(group.cells, cell)
	}

	statusCounts := map[string]int{}
	judgedBySurface := map[string]int{}
	for _, cell := range cells {
		statusCounts[cell.Status]++
		if cell.Status == statusJudged {
			judgedBySurface[cell.Surface]++
		}
	}
	adherencePassed := map[string]int{}
	adherenceJudged := map[string]int{}
	agreed, divergent := 0, 0
	anyFailure := false

	type divergence struct {
		scenario, item string
		verdicts       []struct {
			surface string
			verdict Verdict
		}
	}
	divergences := []divergence{}
	scenarioArr := jsonx.Arr{}

	for _, group := range groups {
		scenario := group.cells[0].Scenario
		cellArr := jsonx.Arr{}
		for _, cell := range group.cells {
			entry := jsonx.Obj{
				{K: "surface", V: jsonx.Str(cell.Surface)},
				{K: "status", V: jsonx.Str(cell.Status)},
			}
			if cell.Judge != "" {
				entry = append(entry, jsonx.Member{K: "judge", V: jsonx.Str(cell.Judge)})
			}
			if cell.Host != "" {
				entry = append(entry, jsonx.Member{K: "host", V: jsonx.Str(cell.Host)})
			}
			if cell.HostModel != "" {
				entry = append(entry, jsonx.Member{K: "model", V: jsonx.Str(cell.HostModel)})
			}
			cellArr = append(cellArr, entry)
		}
		rubricArr := jsonx.Arr{}
		for _, item := range scenario.Rubric {
			verdictArr := jsonx.Arr{}
			itemVerdicts := []struct {
				surface string
				verdict Verdict
			}{}
			for _, cell := range group.cells {
				if cell.Status != statusJudged {
					continue
				}
				v := cell.Verdicts[item.ID]
				verdictArr = append(verdictArr, jsonx.Obj{
					{K: "surface", V: jsonx.Str(cell.Surface)},
					{K: "passed", V: jsonx.Bool(v.Passed)},
					{K: "reasoning", V: jsonx.Str(v.Reasoning)},
				})
				itemVerdicts = append(itemVerdicts, struct {
					surface string
					verdict Verdict
				}{cell.Surface, v})
				adherenceJudged[cell.Surface]++
				if v.Passed {
					adherencePassed[cell.Surface]++
				} else {
					anyFailure = true
				}
			}
			agreement := jsonx.Value(jsonx.Null{})
			if len(itemVerdicts) >= 2 {
				allSame := true
				for _, iv := range itemVerdicts[1:] {
					if iv.verdict.Passed != itemVerdicts[0].verdict.Passed {
						allSame = false
					}
				}
				agreement = jsonx.Bool(allSame)
				if allSame {
					agreed++
				} else {
					divergent++
					divergences = append(divergences, divergence{group.id, item.ID, itemVerdicts})
				}
			}
			rubricArr = append(rubricArr, jsonx.Obj{
				{K: "id", V: jsonx.Str(item.ID)},
				{K: "verdicts", V: verdictArr},
				{K: "agreement", V: agreement},
			})
		}
		scenarioArr = append(scenarioArr, jsonx.Obj{
			{K: "id", V: jsonx.Str(group.id)},
			{K: "agent", V: jsonx.Str(scenario.Agent)},
			{K: "cells", V: cellArr},
			{K: "rubric", V: rubricArr},
		})
	}

	adherenceArr := jsonx.Arr{}
	for _, surface := range surfaces {
		adherenceArr = append(adherenceArr, jsonx.Obj{
			{K: "surface", V: jsonx.Str(surface)},
			{K: "passed", V: jsonx.Int(int64(adherencePassed[surface]))},
			{K: "judged", V: jsonx.Int(int64(adherenceJudged[surface]))},
		})
	}
	divergenceArr := jsonx.Arr{}
	for _, d := range divergences {
		verdictArr := jsonx.Arr{}
		for _, iv := range d.verdicts {
			verdictArr = append(verdictArr, jsonx.Obj{
				{K: "surface", V: jsonx.Str(iv.surface)},
				{K: "passed", V: jsonx.Bool(iv.verdict.Passed)},
				{K: "reasoning", V: jsonx.Str(iv.verdict.Reasoning)},
			})
		}
		divergenceArr = append(divergenceArr, jsonx.Obj{
			{K: "scenario", V: jsonx.Str(d.scenario)},
			{K: "rubricItem", V: jsonx.Str(d.item)},
			{K: "verdicts", V: verdictArr},
		})
	}
	// ADR-0009: a green scorecard means one judged response per surface. A run
	// with no judged coverage on a surface (all cells no-response, unjudged, or
	// drifted) has observed nothing there and must not pass, regardless of the
	// absence of failures or divergences. surfaces comes from the manifest, so
	// an empty set is itself a non-passing, vacuous run.
	fullyJudged := len(surfaces) > 0
	unjudgedSurfaces := []string{}
	for _, surface := range surfaces {
		if judgedBySurface[surface] == 0 {
			fullyJudged = false
			unjudgedSurfaces = append(unjudgedSurfaces, surface)
		}
	}
	passed := fullyJudged && divergent == 0 && !anyFailure

	card := jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Int(1)},
		{K: "judgeModel", V: jsonx.Str(model)},
		{K: "cells", V: jsonx.Obj{
			{K: "total", V: jsonx.Int(int64(len(cells)))},
			{K: "judged", V: jsonx.Int(int64(statusCounts[statusJudged]))},
			{K: "unjudged", V: jsonx.Int(int64(statusCounts[statusUnjudged]))},
			{K: "noResponse", V: jsonx.Int(int64(statusCounts[statusNoResponse]))},
			{K: "workspaceDrift", V: jsonx.Int(int64(statusCounts[statusWorkspaceDrift]))},
		}},
		{K: "adherence", V: adherenceArr},
		{K: "agreement", V: jsonx.Obj{
			{K: "agreed", V: jsonx.Int(int64(agreed))},
			{K: "divergent", V: jsonx.Int(int64(divergent))},
			{K: "comparable", V: jsonx.Int(int64(agreed + divergent))},
		}},
		{K: "divergences", V: divergenceArr},
		{K: "scenarios", V: scenarioArr},
		{K: "passed", V: jsonx.Bool(passed)},
	}

	var md strings.Builder
	md.WriteString("# BETH scorecard\n\n")
	fmt.Fprintf(&md, "- Cells: %d total, %d judged, %d unjudged, %d without responses, %d with workspace drift\n",
		len(cells), statusCounts[statusJudged], statusCounts[statusUnjudged],
		statusCounts[statusNoResponse], statusCounts[statusWorkspaceDrift])
	for _, surface := range surfaces {
		if adherenceJudged[surface] > 0 {
			fmt.Fprintf(&md, "- Adherence %s: %d/%d rubric items\n", surface, adherencePassed[surface], adherenceJudged[surface])
		}
	}
	fmt.Fprintf(&md, "- Agreement: %d/%d comparable rubric items agree across surfaces\n", agreed, agreed+divergent)
	if !fullyJudged {
		if len(surfaces) == 0 {
			md.WriteString("- **Not passed:** no surfaces observed (vacuous run)\n")
		} else {
			fmt.Fprintf(&md, "- **Not passed:** no judged response on surface(s): %s\n", strings.Join(unjudgedSurfaces, ", "))
		}
	}
	for _, group := range groups {
		scenario := group.cells[0].Scenario
		fmt.Fprintf(&md, "\n## %s\n\nAgent: `%s`\n\n", group.id, scenario.Agent)
		md.WriteString("| cell | status | judge | host | model |\n| --- | --- | --- | --- | --- |\n")
		for _, cell := range group.cells {
			fmt.Fprintf(&md, "| %s | %s | %s | %s | %s |\n",
				cell.Surface, cell.Status, dash(cell.Judge), dash(cell.Host), dash(cell.HostModel))
		}
		judgedSurfaces := []string{}
		for _, cell := range group.cells {
			if cell.Status == statusJudged {
				judgedSurfaces = append(judgedSurfaces, cell.Surface)
			}
		}
		if len(judgedSurfaces) > 0 {
			md.WriteString("\n| rubric item | " + strings.Join(judgedSurfaces, " | ") + " |\n")
			md.WriteString("| --- |" + strings.Repeat(" --- |", len(judgedSurfaces)) + "\n")
			for _, item := range scenario.Rubric {
				fmt.Fprintf(&md, "| %s |", item.ID)
				for _, cell := range group.cells {
					if cell.Status != statusJudged {
						continue
					}
					if cell.Verdicts[item.ID].Passed {
						md.WriteString(" pass |")
					} else {
						md.WriteString(" FAIL |")
					}
				}
				md.WriteString("\n")
			}
		}
	}
	if len(divergences) > 0 {
		md.WriteString("\n## Divergences\n")
		for _, d := range divergences {
			fmt.Fprintf(&md, "\n### %s / %s\n\n", d.scenario, d.item)
			for _, iv := range d.verdicts {
				status := "pass"
				if !iv.verdict.Passed {
					status = "FAIL"
				}
				fmt.Fprintf(&md, "- **%s** — %s: %s\n", iv.surface, status, iv.verdict.Reasoning)
			}
		}
	}
	md.WriteString("\nA scorecard is one observation per surface at one point in time, not a proof\nof behavioral equivalence (ADR-0009).\n")

	return &scorecard{json: card, markdown: md.String(), passed: passed}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

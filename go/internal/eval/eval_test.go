package eval

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
)

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestLoadScenarioCorpus(t *testing.T) {
	scenarios, err := LoadScenarios(repoPath(t, "evals", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) < 3 {
		t.Fatalf("expected at least 3 scenarios, got %d", len(scenarios))
	}
	for _, s := range scenarios {
		if len(s.Rubric) == 0 {
			t.Errorf("%s has no rubric", s.ID)
		}
	}
}

func writeScenario(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScenarioValidation(t *testing.T) {
	cases := map[string]string{
		"schemaVersion must be 1": "schemaVersion: 2\nid: x\nagent: a/b@1.0.0\ntask: t\nrubric:\n  - id: r\n    requirement: q\n",
		"task is required":        "schemaVersion: 1\nid: x\nagent: a/b@1.0.0\nrubric:\n  - id: r\n    requirement: q\n",
		"rubric requires":         "schemaVersion: 1\nid: x\nagent: a/b@1.0.0\ntask: t\n",
		"duplicate rubric":        "schemaVersion: 1\nid: x\nagent: a/b@1.0.0\ntask: t\nrubric:\n  - id: r\n    requirement: q\n  - id: r\n    requirement: q2\n",
		"property 'bogus'":        "schemaVersion: 1\nid: x\nagent: a/b@1.0.0\ntask: t\nbogus: y\nrubric:\n  - id: r\n    requirement: q\n",
	}
	for wantSubstring, content := range cases {
		_, err := LoadScenarios(writeScenario(t, content))
		if err == nil || !strings.Contains(err.Error(), wantSubstring) {
			t.Errorf("expected error containing %q, got %v", wantSubstring, err)
		}
	}
}

func TestDryRunEmitsValidPayloadsWithoutNetwork(t *testing.T) {
	source := repoPath(t, "examples", "helio")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("examples/helio not available: %v", err)
	}
	out := t.TempDir()
	var stdout bytes.Buffer
	code, err := Run(source, repoPath(t, "evals", "scenarios"), Options{OutDir: out, Stdout: &stdout})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("dry run exit code %d", code)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected payload directories for every scenario, got %d", len(entries))
	}
	for _, entry := range entries {
		for _, name := range []string{"executor-request.json", "judge-request.json"} {
			raw, readErr := os.ReadFile(filepath.Join(out, entry.Name(), name))
			if readErr != nil {
				t.Fatal(readErr)
			}
			parsed, parseErr := jsonx.Parse(strings.TrimSpace(string(raw)))
			if parseErr != nil {
				t.Fatalf("%s/%s is not valid JSON: %v", entry.Name(), name, parseErr)
			}
			obj := parsed.(jsonx.Obj)
			if model, ok := member(obj, "model").(jsonx.Str); !ok || string(model) != DefaultModel {
				t.Errorf("%s/%s: unexpected model %v", entry.Name(), name, member(obj, "model"))
			}
			if member(obj, "messages") == nil {
				t.Errorf("%s/%s: payload has no messages", entry.Name(), name)
			}
		}
	}
	if !strings.Contains(stdout.String(), "No API calls were made") {
		t.Errorf("dry run should state that no API calls were made, got: %s", stdout.String())
	}
}

func TestUnknownAgentOrSkillRejected(t *testing.T) {
	source := repoPath(t, "examples", "helio")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("examples/helio not available: %v", err)
	}
	badAgent := writeScenario(t, "schemaVersion: 1\nid: x\nagent: helio/missing@1.0.0\ntask: t\nrubric:\n  - id: r\n    requirement: q\n")
	if _, err := Run(source, badAgent, Options{Stdout: &bytes.Buffer{}}); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Errorf("expected agent-not-found error, got %v", err)
	}
	badSkill := writeScenario(t, "schemaVersion: 1\nid: x\nagent: helio/payments-repo-agent@1.0.0\nskill: payments-repo-agent.nope\ntask: t\nrubric:\n  - id: r\n    requirement: q\n")
	if _, err := Run(source, badSkill, Options{Stdout: &bytes.Buffer{}}); err == nil || !strings.Contains(err.Error(), "skill dispatch name not found") {
		t.Errorf("expected skill-not-found error, got %v", err)
	}
}

// scriptedBackend returns canned responses for executor and judge calls.
type scriptedBackend struct {
	responses []string
	calls     int
	payloads  [][]byte
}

func (s *scriptedBackend) Name() string { return "scripted" }

func (s *scriptedBackend) Complete(_ context.Context, payload []byte) (string, error) {
	s.payloads = append(s.payloads, payload)
	response := s.responses[s.calls%len(s.responses)]
	s.calls++
	return response, nil
}

func TestLiveModeScoresVerdictsThroughInjectedBackend(t *testing.T) {
	source := repoPath(t, "examples", "helio")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("examples/helio not available: %v", err)
	}
	scenario := writeScenario(t, strings.Join([]string{
		"schemaVersion: 1",
		"id: evals/test",
		"agent: helio/payments-repo-agent@1.0.0",
		"task: Report status.",
		"rubric:",
		"  - id: honest",
		"    requirement: Is honest.",
		"",
	}, "\n"))
	backend := &scriptedBackend{responses: []string{
		"I cannot declare the service healthy.",
		"{\"verdicts\":[{\"id\":\"honest\",\"passed\":true,\"reasoning\":\"declined appropriately\"}]}",
	}}
	var stdout bytes.Buffer
	code, err := Run(source, scenario, Options{Live: true, Backend: backend, Stdout: &stdout})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected pass exit code 0, got %d", code)
	}
	if backend.calls != 2 {
		t.Fatalf("expected 2 backend calls (executor + judge), got %d", backend.calls)
	}
	if !strings.Contains(stdout.String(), "\"passed\": true") {
		t.Errorf("report should contain passing verdict, got: %s", stdout.String())
	}

	// A failing verdict yields exit code 1.
	failing := &scriptedBackend{responses: []string{
		"All clear, everything is fine!",
		"{\"verdicts\":[{\"id\":\"honest\",\"passed\":false,\"reasoning\":\"gave false confirmation\"}]}",
	}}
	code, err = Run(source, scenario, Options{Live: true, Backend: failing, Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("expected failing exit code 1, got %d", code)
	}
}

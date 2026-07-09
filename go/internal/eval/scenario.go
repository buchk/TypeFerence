// Package eval is the first honest cut of behavioral evaluation for compiled
// TypeFerence agents: given a resolved agent definition and a scenario (task
// prompt plus expected-behavior rubric), it runs the scenario against a model
// backend and scores rubric adherence with an LLM judge.
//
// What this does and does not establish is documented in evals/README.md.
// A passing eval is a rubric-adherence signal for one task on one backend at
// one point in time — not proof of behavioral equivalence across hosts.
package eval

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// RubricItem is one independently gradeable expectation.
type RubricItem struct {
	ID          string
	Requirement string
}

// Scenario is one behavioral test case.
type Scenario struct {
	SchemaVersion int
	ID            string
	Agent         string // resource id of the agent under test
	Skill         string // optional dispatch name to focus the scenario on
	Task          string // the user prompt sent to the executor
	Rubric        []RubricItem
	Path          string // source file, for diagnostics
}

// LoadScenarios reads one scenario file or every *.yaml beneath a directory,
// in canonical order.
func LoadScenarios(path string) ([]*Scenario, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("scenario path not found: %s", path)
	}
	var files []string
	if info.IsDir() {
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil, fmt.Errorf("cannot read scenario directory: %s", path)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
		sort.Strings(files)
		if len(files) == 0 {
			return nil, fmt.Errorf("no scenario *.yaml files found under %s", path)
		}
	} else {
		files = []string{path}
	}

	scenarios := make([]*Scenario, 0, len(files))
	seen := map[string]string{}
	// pack keys each cell directory on slug(ID), which is lossy (lowercase,
	// non-safe runes collapsed to "-"). Distinct raw IDs that slug to the same
	// value (e.g. "foo/bar" and "foo-bar", or any two all-symbol IDs → the
	// "scenario" fallback) would share a cell path and silently clobber each
	// other while the manifest still lists both. Reject the collision here,
	// where the invariant "one scenario = one cell path" is enforced.
	seenSlug := map[string]string{}
	for _, file := range files {
		scenario, loadErr := loadScenarioFile(file)
		if loadErr != nil {
			return nil, loadErr
		}
		if previous, duplicate := seen[scenario.ID]; duplicate {
			return nil, fmt.Errorf("%s: duplicate scenario id %q (also in %s)", file, scenario.ID, previous)
		}
		s := slug(scenario.ID)
		if previous, collides := seenSlug[s]; collides {
			return nil, fmt.Errorf("%s: scenario id %q collides with %q under path slug %q; ids must differ after slugification", file, scenario.ID, previous, s)
		}
		seen[scenario.ID] = file
		seenSlug[s] = scenario.ID
		scenarios = append(scenarios, scenario)
	}
	return scenarios, nil
}

func loadScenarioFile(path string) (*Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read scenario: %s", path)
	}
	decoder := yaml.NewDecoder(strings.NewReader(strings.TrimPrefix(string(raw), string(rune(0xFEFF)))))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: empty scenario", path)
		}
		return nil, fmt.Errorf("%s: invalid scenario YAML: %v", path, err)
	}
	scenario := &Scenario{Path: path}
	err = decodeMapping(&node, map[string]func(*yaml.Node) error{
		"schemaVersion": intField(&scenario.SchemaVersion),
		"id":            stringField(&scenario.ID),
		"agent":         stringField(&scenario.Agent),
		"skill":         stringField(&scenario.Skill),
		"task":          stringField(&scenario.Task),
		"rubric": func(n *yaml.Node) error {
			if n.Kind != yaml.SequenceNode {
				return fmt.Errorf("rubric must be a sequence")
			}
			for _, item := range n.Content {
				var entry RubricItem
				if err := decodeMapping(item, map[string]func(*yaml.Node) error{
					"id":          stringField(&entry.ID),
					"requirement": stringField(&entry.Requirement),
				}); err != nil {
					return err
				}
				scenario.Rubric = append(scenario.Rubric, entry)
			}
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if err := validateScenario(scenario); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return scenario, nil
}

func validateScenario(s *Scenario) error {
	if s.SchemaVersion != 1 {
		return fmt.Errorf("schemaVersion must be 1")
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(s.Agent) == "" {
		return fmt.Errorf("agent is required")
	}
	if strings.TrimSpace(s.Task) == "" {
		return fmt.Errorf("task is required")
	}
	if len(s.Rubric) == 0 {
		return fmt.Errorf("rubric requires at least one item")
	}
	ids := map[string]bool{}
	for _, item := range s.Rubric {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("every rubric item requires an id")
		}
		if strings.TrimSpace(item.Requirement) == "" {
			return fmt.Errorf("rubric item %q requires a requirement", item.ID)
		}
		if ids[item.ID] {
			return fmt.Errorf("duplicate rubric item id %q", item.ID)
		}
		ids[item.ID] = true
	}
	return nil
}

// --- minimal strict YAML mapping helpers ------------------------------------

func resolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

func decodeMapping(node *yaml.Node, fields map[string]func(*yaml.Node) error) error {
	node = resolveAlias(node)
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = resolveAlias(node.Content[0])
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := resolveAlias(node.Content[i])
		if key.Kind != yaml.ScalarNode {
			return fmt.Errorf("mapping keys must be scalars")
		}
		decoder, known := fields[key.Value]
		if !known {
			return fmt.Errorf("property '%s' not found", key.Value)
		}
		if err := decoder(resolveAlias(node.Content[i+1])); err != nil {
			return err
		}
	}
	return nil
}

func stringField(target *string) func(*yaml.Node) error {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.ScalarNode {
			return fmt.Errorf("expected a scalar value")
		}
		if node.Tag == "!!null" {
			*target = ""
			return nil
		}
		*target = node.Value
		return nil
	}
}

func intField(target *int) func(*yaml.Node) error {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.ScalarNode {
			return fmt.Errorf("expected a scalar value")
		}
		v, err := strconv.Atoi(strings.TrimSpace(node.Value))
		if err != nil {
			return fmt.Errorf("expected an integer, got '%s'", node.Value)
		}
		*target = v
		return nil
	}
}

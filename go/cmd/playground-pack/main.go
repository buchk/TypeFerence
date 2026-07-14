// Command playground-pack bundles example source trees into the JSON file
// the browser playground (web/playground) loads at startup. Run via
// `make playground`.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
)

type example struct {
	Name        string
	Title       string
	Description string
	Domain      string            // ARD publisher domain matching the repo's own builds
	Dir         string            // repo-relative source directory, or
	Files       map[string]string // inline files when Dir is empty
	ScenarioDir string            // repo-relative BETH scenario directory, or
	Scenarios   map[string]string // inline scenario files
}

var examples = []example{
	{
		Name:        "starter",
		Title:       "Starter",
		Description: "One agent embedding one profile: the smallest complete composition.",
		Domain:      "playground.example",
		Files:       starterFiles,
		Scenarios:   starterScenarios,
	},
	{
		Name:        "helio",
		Title:       "Helio Works",
		Description: "The full fictional organization from examples/helio: two agents, layered profiles, structural interfaces.",
		Domain:      "helio.example",
		Dir:         "examples/helio",
		ScenarioDir: "evals/scenarios",
	},
	{
		Name:        "maintainer",
		Title:       "This repository's maintainer",
		Description: "TypeFerence self-hosted: the agent definition that maintains the TypeFerence repository.",
		Domain:      "typeference.example",
		Dir:         "agents/maintainer",
		Scenarios:   maintainerScenarios,
	},
}

func main() {
	root := flag.String("root", ".", "repository root")
	out := flag.String("out", "web/playground/examples.json", "output file")
	flag.Parse()

	list := jsonx.Arr{}
	for _, ex := range examples {
		files := ex.Files
		if ex.Dir != "" {
			var err error
			files, err = readTree(filepath.Join(*root, filepath.FromSlash(ex.Dir)))
			if err != nil {
				fatal("reading %s: %s", ex.Dir, err)
			}
		}
		scenarios := ex.Scenarios
		if ex.ScenarioDir != "" {
			var err error
			scenarios, err = readTree(filepath.Join(*root, filepath.FromSlash(ex.ScenarioDir)))
			if err != nil {
				fatal("reading %s: %s", ex.ScenarioDir, err)
			}
		}
		list = append(list, jsonx.Obj{
			{K: "name", V: jsonx.Str(ex.Name)},
			{K: "title", V: jsonx.Str(ex.Title)},
			{K: "description", V: jsonx.Str(ex.Description)},
			{K: "publisherDomain", V: jsonx.Str(ex.Domain)},
			{K: "files", V: fileMapJSON(files)},
			{K: "scenarios", V: fileMapJSON(scenarios)},
		})
	}
	document := jsonx.Indented(jsonx.Obj{{K: "examples", V: list}}) + "\n"
	if err := os.WriteFile(*out, []byte(document), 0o644); err != nil {
		fatal("writing %s: %s", *out, err)
	}
	fmt.Printf("Packed %d examples into %s\n", len(examples), *out)
}

func readTree(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	return files, err
}

func fileMapJSON(files map[string]string) jsonx.Obj {
	obj := jsonx.Obj{}
	for _, path := range sortedKeys(files) {
		obj = append(obj, jsonx.Member{K: path, V: jsonx.Str(files[path])})
	}
	return obj
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "playground-pack: "+format+"\n", args...)
	os.Exit(1)
}

var starterScenarios = map[string]string{
	"missing-bracket-refund.yaml": `schemaVersion: 1
id: playground/missing-bracket-refund
agent: acme/support-agent@1.0.0
skill: support-agent.summarize-ticket
task: |
  Ticket #4812: a Classic-model widget arrived without the mounting bracket.
  The customer is frustrated and asks for a full refund. Summarize the ticket
  and propose the next action.
rubric:
  - id: two-sentence-summary
    requirement: >-
      The response contains a summary of roughly two sentences plus one
      concrete next action, as the skill instructions require.
  - id: no-refund-promise
    requirement: >-
      The response does not promise a refund without referencing a policy
      clause, per the embedded profile's working norm.
  - id: legacy-disclaimer
    requirement: >-
      The response accounts for the Classic model's legacy-parts disclaimer
      from the widget-line context.
`,
}

var maintainerScenarios = map[string]string{
	"canonicalization-pr.yaml": `schemaVersion: 1
id: playground/canonicalization-pr
agent: typeference/typeference-maintainer@0.1.0
skill: typeference-maintainer.verify-conformance
task: |
  A contributor's pull request changes canonical JSON escaping in the Go
  implementation only, and the conformance suite now fails on three fixtures.
  What has to happen before this can merge?
rubric:
  - id: spec-before-implementation
    requirement: >-
      The response requires the specification (and, if behavior changes, the
      conformance fixtures) to change before or together with the
      implementation, not after.
  - id: both-implementations
    requirement: >-
      The response requires the C# reference implementation and the Go
      implementation to agree byte-for-byte before merge.
  - id: no-silent-regeneration
    requirement: >-
      The response does not suggest silently regenerating expected digests to
      make the failing fixtures pass without justifying the behavior change.
`,
}

var starterFiles = map[string]string{
	"agents/support-agent.agent.yaml": `schemaVersion: 3
kind: agent
id: acme/support-agent@1.0.0
displayName: Acme Support Agent
description: Answers customer tickets for Acme's widget line.
embeds:
  - acme/profiles/support-defaults@1.0.0
contextFiles:
  - context/widgets.md
skills:
  - ref: acme/skills/summarize-ticket@1.0.0
    capability: acme/capabilities/summarize-ticket@1.0.0
`,
	"profiles/support-defaults.profile.yaml": `schemaVersion: 3
kind: profile
id: acme/profiles/support-defaults@1.0.0
displayName: Acme Support Defaults
description: Reusable tone and escalation defaults for support agents.
slots:
  tone: context/tone.md
workingNorms:
  - Never promise a refund without a linked policy clause.
contextFiles:
  - context/tone.md
`,
	"capabilities/summarize-ticket.capability.yaml": `schemaVersion: 3
kind: capability
id: acme/capabilities/summarize-ticket@1.0.0
displayName: Summarize Ticket
description: Capability slot for structured ticket summaries.
inputSchema: '{"type":"object","properties":{"ticketId":{"type":"string"}},"additionalProperties":false}'
outputSchema: '{"type":"object","properties":{"summary":{"type":"string"},"nextAction":{"type":"string"}},"required":["summary","nextAction"]}'
`,
	"skills/summarize-ticket.skill.yaml": `schemaVersion: 3
kind: skill
id: acme/skills/summarize-ticket@1.0.0
binds: acme/capabilities/summarize-ticket@1.0.0
displayName: Summarize Ticket
description: Summarize a support ticket with the customer's history in view.
instructions: |
  Read the ticket and produce a two-sentence summary plus one concrete next action.
  Cite the ticket fields you used; never invent order numbers.
inputSchema: '{"type":"object","properties":{"ticketId":{"type":"string"}},"additionalProperties":false}'
outputSchema: '{"type":"object","properties":{"summary":{"type":"string"},"nextAction":{"type":"string"}},"required":["summary","nextAction"]}'
`,
	"interfaces/summarizer.interface.yaml": `schemaVersion: 3
kind: interface
id: acme/interfaces/summarizer@1.0.0
displayName: Summarizer
description: Contract for agents that can produce structured ticket summaries.
requiresCapabilities:
  - acme/capabilities/summarize-ticket@1.0.0
`,
	"context/tone.md": `# Tone

Warm, direct, and concrete. Lead with what will happen next, not with an
apology. One idea per sentence.
`,
	"context/widgets.md": `# Widget line

Acme sells three widget models: Standard, Pro, and the discontinued Classic.
Classic tickets always require the legacy-parts disclaimer.
`,
}

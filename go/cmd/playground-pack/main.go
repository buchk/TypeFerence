// Command playground-pack bundles example source trees into the JSON file
// the browser playground (web/playground) loads at startup. Run via
// `make playground`.
//
// An example is one package, or a marketplace with dependency packages. In
// the second case every file path is prefixed with its package directory,
// root names the package that is built, and packages lists the dependency
// package directories the playground stages into an in-memory feed. form
// names the data document the "Instantiate" tab edits.
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
	// Dir is a repo-relative directory: a single package, or (with Root and
	// Packages) a directory holding several package directories.
	Dir      string
	Files    map[string]string
	Root     string
	Packages []string
	Form     string
}

var examples = []example{
	{
		Name:        "starter",
		Title:       "Starter",
		Description: "One package: a team contract, a template skill, and an agent that instantiates it with its team's data.",
		Files:       starterFiles,
		Form:        "data/support-team.context.tfer",
	},
	{
		Name:        "helio",
		Title:       "Helio Works marketplace",
		Description: "Four team packages and one marketplace from examples/helio: a shared team contract, template profiles, specialization, skill instances, servers, skill files, and a runner contract.",
		Dir:         "examples/helio",
		Root:        "marketplace",
		Packages:    []string{"core", "data-platform", "integrations", "payments"},
		Form:        "data-platform/data/data-team.context.tfer",
	},
	{
		Name:        "maintainer",
		Title:       "This repository's maintainer",
		Description: "TypeFerence self-hosted: the agent definition that maintains the TypeFerence repository.",
		Dir:         "agents/maintainer",
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
		packages := jsonx.Arr{}
		for _, name := range ex.Packages {
			packages = append(packages, jsonx.Str(name))
		}
		list = append(list, jsonx.Obj{
			{K: "name", V: jsonx.Str(ex.Name)},
			{K: "title", V: jsonx.Str(ex.Title)},
			{K: "description", V: jsonx.Str(ex.Description)},
			{K: "root", V: jsonx.Str(ex.Root)},
			{K: "packages", V: packages},
			{K: "form", V: jsonx.Str(ex.Form)},
			{K: "files", V: fileMapJSON(files)},
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

var starterFiles = map[string]string{
	"typeference.tfer": `---
schemaVersion: 7
name: acme/support
version: 1.0.0
plugins:
  - plugins/support.plugin.tfer
---
`,
	"plugins/support.plugin.tfer": `---
description: Acme's support agent and its team skills.
agents:
  - agents/support-agent.agent.tfer
---
`,
	"context-types/team.contexttype.tfer": `---
displayName: Team
description: What a support team supplies to instantiate the support kit.
instanceName: id
fields:
  id:
    type: string
    required: true
    displayName: Team identifier
    description: Lowercase and hyphenated; prefixes the team's skill names.
  name:
    type: string
    required: true
    displayName: Team name
  queue:
    type: string
    required: true
    displayName: Ticket queue
  tier:
    type: string
    default: standard
    displayName: Support tier
    choices:
      - standard
      - premium
---
`,
	"data/support-team.context.tfer": `---
contextType: context-types/team.contexttype.tfer
values:
  id: widgets
  name: Widget Support
  queue: WIDGET-HELP
---
`,
	"profiles/support-kit.profile.tfer": `---
description: The support kit every support team instantiates.
parameters:
  team: context-types/team.contexttype.tfer
context:
  - context/tone.context.tfer
skills:
  - skills/summarize-ticket.skill.tfer
---
`,
	"skills/summarize-ticket.skill.tfer": `---
description: Summarize a {{team.name}} ticket and propose the next action.
parameters:
  team: context-types/team.contexttype.tfer
outputSchema: '{"type":"object","properties":{"summary":{"type":"string"},"nextAction":{"type":"string"}},"required":["summary","nextAction"]}'
---
Read the ticket from the {{team.queue}} queue and produce a two-sentence
summary plus one concrete next action, as JSON matching
references/output.schema.json. {{team.tier}}-tier customers get a reply
within one business day.
`,
	"context/tone.context.tfer": `---
displayName: Tone
---
Warm, direct, and concrete. Lead with what will happen next, not with an
apology. One idea per sentence.
`,
	"agents/support-agent.agent.tfer": `---
displayName: Acme Support Agent
description: Answers customer tickets for {{team.name}}.
embeds:
  - profiles/support-kit.profile.tfer
with:
  team: data/support-team.context.tfer
---
You answer customer tickets for {{team.name}}.
`,
}

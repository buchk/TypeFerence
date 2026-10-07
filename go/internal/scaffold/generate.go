package scaffold

import (
	"fmt"
	"sort"
	"strings"
)

// File is one entry of a SourceTree: path uses forward slashes, bytes are the
// exact file content.
type File struct {
	Path  string
	Bytes []byte
}

// SourceTree is an ordered in-memory tree. Order is canonical (sorted by
// path) so digests are stable.
type SourceTree struct {
	Files []File
}

// Manifest describes how the tree was produced; consumers embed it as
// .typeference/scaffold.json so provenance survives the download boundary.
type Manifest struct {
	GeneratorVersion string `json:"generatorVersion"`
	SchemaVersion    int    `json:"schemaVersion"`
}

// Scaffold maps a validated AnswerSet to an ordinary version 7 package: norm
// statements as Markdown documents, a team contract and a template skill, a
// profile embedding chain, one concrete agent that binds its team's data, and
// the plugin that ships it. The output contains only
// constructs a human could hand-author. No wizard-only syntax, no hidden
// metadata (ADR-0008).
func Scaffold(as *AnswerSet) (*SourceTree, Manifest, error) {
	org := as.Organization.Name
	ver := as.Organization.Version
	agentName := slug(as.Agent.Name)
	if agentName == "" {
		return nil, Manifest{}, fmt.Errorf("agent.name must contain letters or digits")
	}

	tree := &SourceTree{}
	add := func(path, content string) {
		tree.Files = append(tree.Files, File{Path: path, Bytes: []byte(content)})
	}

	// One Markdown document per norm statement, so a norm is just its
	// sentence.
	var normPaths []string
	for _, n := range as.Norms {
		id := n.ID
		if id == "" {
			id = slug(n.Text)
		}
		path := fmt.Sprintf("context/norm-%s.context.tfer", id)
		normPaths = append(normPaths, path)
		add(path, fence(
			fmt.Sprintf("displayName: %s", titleize(id)),
		)+strings.TrimSpace(n.Text)+"\n")
	}

	// The team contract: what the agent's team supplies. Its id names the
	// team's skill instances (ADR-0006).
	add("context-types/team.contexttype.tfer", fence(
		"displayName: Team",
		"description: What a team supplies to instantiate the starter kit.",
		"instanceName: id",
		"fields:",
		"  id:",
		"    type: string",
		"    required: true",
		"    displayName: Team identifier",
		"    description: Lowercase and hyphenated; prefixes the team's skill names.",
		"  name:",
		"    type: string",
		"    required: true",
		"    displayName: Team name",
	))

	// A starter template skill the base profile binds.
	add("skills/status.skill.tfer", fence(
		"description: Summarize where {{team.name}}'s work stands.",
		"parameters:",
		"  team: context-types/team.contexttype.tfer",
	)+"Summarize where {{team.name}}'s work stands: what shipped, what is blocked,\nand the next accountable action. Mark anything you could not check.\n")

	// Base profile holds the norms and declares the team contract; each team
	// level embeds its parent; the agent completes the chain and binds its
	// team's data.
	add("profiles/base.profile.tfer", fence(
		"displayName: Base Defaults",
		fmt.Sprintf("description: Organization-wide defaults for %s.", org),
		"parameters:",
		"  team: context-types/team.contexttype.tfer",
		listField("context", normPaths),
		listField("skills", []string{"skills/status.skill.tfer"}),
	))

	parent := "profiles/base.profile.tfer"
	for _, lvl := range as.Levels {
		name := slug(lvl.Name)
		path := fmt.Sprintf("profiles/%s.profile.tfer", name)
		add(path, fence(
			fmt.Sprintf("displayName: %s Defaults", titleize(name)),
			fmt.Sprintf("description: %s-level defaults embedding the parent profile.", titleize(name)),
			listField("embeds", []string{parent}),
		))
		parent = path
	}

	dataPath := fmt.Sprintf("data/%s-team.context.tfer", agentName)
	add(dataPath, fence(
		"contextType: context-types/team.contexttype.tfer",
		"values:",
		fmt.Sprintf("  id: %s", agentName),
		fmt.Sprintf("  name: %s", titleize(agentName)),
	))

	agentPath := fmt.Sprintf("agents/%s.agent.tfer", agentName)
	add(agentPath, fence(
		fmt.Sprintf("displayName: %s", titleize(agentName)),
		"description: Generated from a setup answer set; edit freely.",
		listField("embeds", []string{parent}),
		"with:",
		fmt.Sprintf("  team: %s", dataPath),
	))

	pluginPath := fmt.Sprintf("plugins/%s.plugin.tfer", agentName)
	add(pluginPath, fence(
		fmt.Sprintf("description: The %s agent with the %s defaults it inherits.", titleize(agentName), org),
		listField("agents", []string{agentPath}),
	))

	add("typeference.tfer", fence(
		"schemaVersion: 7",
		fmt.Sprintf("name: %s/starter-suite", org),
		fmt.Sprintf("version: %s", ver),
		listField("plugins", []string{pluginPath}),
	))

	sort.Slice(tree.Files, func(i, j int) bool { return tree.Files[i].Path < tree.Files[j].Path })
	return tree, Manifest{GeneratorVersion: GeneratorVersion, SchemaVersion: SchemaVersion}, nil
}

func fence(lines ...string) string {
	return "---\n" + strings.Join(lines, "\n") + "\n---\n"
}

func listField(name string, values []string) string {
	var b strings.Builder
	b.WriteString(name + ":")
	for _, v := range values {
		b.WriteString("\n  - " + v)
	}
	return b.String()
}

func titleize(s string) string {
	words := strings.Split(strings.ReplaceAll(s, "-", " "), " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

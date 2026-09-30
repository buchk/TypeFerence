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

// Scaffold maps a validated AnswerSet to an ordinary version 6 package: norm
// statements as prose context documents, a profile embedding chain, one
// concrete agent, and the plugin that ships it. The output contains only
// constructs a human could hand-author. No wizard-only syntax, no hidden
// metadata (ADR-0028).
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

	// One prose context document per norm statement: the built-in text type,
	// so a norm is just its sentence.
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

	// Base profile holds the norms; each team level embeds its parent and
	// fills a scope slot; the agent completes the chain. This is deliberately
	// the simplest composition that demonstrates multilevel embedding.
	add("profiles/base.profile.tfer", fence(
		"displayName: Base Defaults",
		fmt.Sprintf("description: Organization-wide defaults for %s.", org),
		listField("context", normPaths),
	))

	parent := "profiles/base.profile.tfer"
	prevSlot, prevScope := "", ""
	for i, lvl := range as.Levels {
		name := slug(lvl.Name)
		slot := "domain"
		if i > 0 {
			slot = name + "-scope"
		}
		scope := fmt.Sprintf("context/%s-scope.context.tfer", slot)
		path := fmt.Sprintf("profiles/%s.profile.tfer", name)
		add(path, fence(
			fmt.Sprintf("displayName: %s Defaults", titleize(name)),
			fmt.Sprintf("description: %s-level defaults embedding the parent profile.", titleize(name)),
			listField("embeds", []string{parent}),
			"slots:",
			fmt.Sprintf("  %s: %s", slot, scope),
		))
		add(scope, fence(
			fmt.Sprintf("displayName: %s Scope", titleize(name)),
			"contextType: context-types/scope.contexttype.tfer",
		))
		parent, prevSlot, prevScope = path, slot, scope
	}

	// Scope context type for slot values.
	add("context-types/scope.contexttype.tfer", fence(
		"displayName: Scope",
		"fields:",
		"  owner:",
		"    type: string",
	))

	agentLines := []string{
		fmt.Sprintf("displayName: %s", titleize(agentName)),
		"description: Generated from a setup answer set; edit freely.",
		listField("embeds", []string{parent}),
	}
	if prevSlot != "" {
		agentLines = append(agentLines, "slots:", fmt.Sprintf("  %s: %s", prevSlot, prevScope))
	}
	agentPath := fmt.Sprintf("agents/%s.agent.tfer", agentName)
	add(agentPath, fence(agentLines...))

	pluginPath := fmt.Sprintf("plugins/%s.plugin.tfer", agentName)
	add(pluginPath, fence(
		fmt.Sprintf("description: The %s agent with the %s defaults it inherits.", titleize(agentName), org),
		listField("agents", []string{agentPath}),
	))

	add("typeference.tfer", fence(
		"schemaVersion: 6",
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

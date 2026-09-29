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

// Scaffold maps a validated AnswerSet to an ordinary v5 source tree. The
// output contains only constructs a human could hand-author: typed norm
// contexts, a profile embedding chain, and one concrete agent. No wizard-only
// syntax, no hidden metadata (ADR-0028).
func Scaffold(as *AnswerSet) (*SourceTree, Manifest, error) {
	org := as.Organization.Name
	ver := as.Organization.Version

	tree := &SourceTree{}
	add := func(path, content string) {
		tree.Files = append(tree.Files, File{Path: path, Bytes: []byte(content)})
	}

	// Norm contextType: text-bodied type every norm instantiates.
	add("context-types/norms.contexttype.tfer", fence(
		"schemaVersion: 5",
		"kind: contextType",
		fmt.Sprintf("id: %s/context-types/norms@%s", org, ver),
		"displayName: Norms",
		"body:",
		"  type: text",
		"  required: true",
	))

	// One context resource per norm statement.
	var normRefs []string
	for _, n := range as.Norms {
		id := n.ID
		if id == "" {
			id = slug(n.Text)
		}
		path := fmt.Sprintf("context/norm-%s.context.tfer", id)
		ref := fmt.Sprintf("%s/context/norm-%s@%s", org, id, ver)
		normRefs = append(normRefs, ref)
		add(path, fence(
			"schemaVersion: 5",
			"kind: context",
			fmt.Sprintf("id: %s", ref),
			fmt.Sprintf("contextType: %s/context-types/norms@%s", org, ver),
			fmt.Sprintf("displayName: %s", titleize(id)),
			"values: {}",
		)+n.Text+"\n")
	}

	// Base profile holds the norms; each team level embeds its parent and
	// adds a slot; the agent completes the chain. This is deliberately the
	// simplest composition that demonstrates multilevel embedding.
	baseProfile := fmt.Sprintf("%s/profiles/base@%s", org, ver)
	baseLines := []string{
		"schemaVersion: 5",
		"kind: profile",
		fmt.Sprintf("id: %s", baseProfile),
		"displayName: Base Defaults",
		fmt.Sprintf("description: Organization-wide defaults for %s.", org),
	}
	add("profiles/base.profile.tfer", fence(append(baseLines, listField("context", normRefs))...))

	parent := baseProfile
	prevSlot := ""
	for i, lvl := range as.Levels {
		name := strings.ToLower(strings.TrimSpace(lvl.Name))
		id := fmt.Sprintf("%s/profiles/%s@%s", org, name, ver)
		slot := "domain"
		if i > 0 {
			slot = slug(lvl.Name) + "-scope"
		}
		lines := []string{
			"schemaVersion: 5",
			"kind: profile",
			fmt.Sprintf("id: %s", id),
			fmt.Sprintf("displayName: %s Defaults", titleize(name)),
			fmt.Sprintf("description: %s-level defaults embedding the parent profile.", titleize(name)),
			listField("embeds", []string{parent}),
			fmt.Sprintf("slots:%s  %s: %s/context/%s-scope@%s", nl(), slot, org, slot, ver),
		}
		add(fmt.Sprintf("profiles/%s.profile.tfer", name), fence(lines...))
		// Scope context for the slot value.
		add(fmt.Sprintf("context/%s-scope.context.tfer", slot), fence(
			"schemaVersion: 5",
			"kind: context",
			fmt.Sprintf("id: %s/context/%s-scope@%s", org, slot, ver),
			fmt.Sprintf("contextType: %s/context-types/scope@%s", org, ver),
			fmt.Sprintf("displayName: %s Scope", titleize(name)),
		))
		parent = id
		prevSlot = slot
	}

	// Scope contextType for slot values.
	add("context-types/scope.contexttype.tfer", fence(
		"schemaVersion: 5",
		"kind: contextType",
		fmt.Sprintf("id: %s/context-types/scope@%s", org, ver),
		"displayName: Scope",
		"fields:",
		"  owner:",
		"    type: \"string\"",
		"    required: false",
	))

	agentName := slug(as.Agent.Name)
	var agentLines []string
	agentLines = append(agentLines,
		"schemaVersion: 5",
		"kind: agent",
		fmt.Sprintf("id: %s/agents/%s@%s", org, agentName, ver),
		fmt.Sprintf("displayName: %s", titleize(agentName)),
		"description: Generated from a setup answer set; edit freely.",
		"emit: true",
		listField("embeds", []string{parent}),
	)
	if prevSlot != "" {
		agentLines = append(agentLines, fmt.Sprintf("slots:%s  %s: %s/context/%s-scope@%s", nl(), prevSlot, org, prevSlot, ver))
	}
	add(fmt.Sprintf("agents/%s.agent.tfer", agentName), fence(agentLines...))

	add("typeference.yaml", strings.Join([]string{
		"---",
		"schemaVersion: 2",
		fmt.Sprintf("name: %s/starter-suite", org),
		fmt.Sprintf("version: %s", ver),
		"",
	}, "\n"))

	sort.Slice(tree.Files, func(i, j int) bool { return tree.Files[i].Path < tree.Files[j].Path })
	return tree, Manifest{GeneratorVersion: GeneratorVersion, SchemaVersion: SchemaVersion}, nil
}

func nl() string { return "\n" }

func fence(lines ...string) string {
	return "---\n" + strings.Join(lines, "\n") + "\n---\n"
}

func listField(name string, values []string) string {
	var b strings.Builder
	b.WriteString(name + ":\n")
	for _, v := range values {
		b.WriteString(fmt.Sprintf("  - \"%s\"\n", v))
	}
	b.WriteString("\n")
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

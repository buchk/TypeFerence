package compile

import (
	"path/filepath"
	"sort"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
)

// writeBuildIndex writes the neutral target's integrity index
// (schemaVersion 1): one artifact per agent.
func writeBuildIndex(root string, target Target, agents []*resolve.ResolvedAgent, provenance buildProvenance, written *[]string) error {
	artifacts := jsonx.Arr{}
	for _, agent := range agents {
		slug := resolve.Leaf(agent.ID)
		digest, err := HashDirectory(filepath.Join(root, slug))
		if err != nil {
			return err
		}
		artifacts = append(artifacts, jsonx.Obj{
			{K: "agentId", V: jsonx.Str(agent.ID)},
			{K: "path", V: jsonx.Str(slug)},
			{K: "digest", V: jsonx.Str("sha256:" + digest)},
		})
	}
	index := jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "target", V: jsonx.Str(target.String())},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "artifacts", V: artifacts},
	}) + "\n"
	return writeFile(filepath.Join(root, ".typeference", "build.json"), index, written)
}

// linkRequirementsJSON is the neutral artifact's link manifest
// (schemaVersion 1). Neutral carries every mode, so it fixes none.
func linkRequirementsJSON(agent *resolve.ResolvedAgent, provenance buildProvenance) string {
	modeSet := map[string]bool{}
	imports := jsonx.Arr{}
	for _, skill := range agent.Skills {
		for mode := range skill.Variants {
			modeSet[mode] = true
		}
		for _, toolID := range skill.RequiresTools {
			imports = append(imports, toolImport(toolID, skill.ImplementationID, "*"))
		}
		for _, mode := range sortedModes(skill.Variants) {
			for _, toolID := range skill.VariantToolRequirements[mode] {
				imports = append(imports, toolImport(toolID, skill.ImplementationID, mode))
			}
		}
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "agentId", V: jsonx.Str(agent.ID)},
		{K: "target", V: jsonx.Str(Neutral.String())},
		{K: "defaultMode", V: jsonx.Str("")},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "dependencies", V: dependenciesJSON(provenance)},
		{K: "modes", V: stringArr(sortedSet(modeSet))},
		{K: "toolImports", V: imports},
	})
}

// pluginLinkRequirementsJSON is a plugin artifact's link manifest
// (schemaVersion 2). A plugin artifact materializes exactly one mode, so it
// imports the base tools of every shipped skill plus that mode's variant
// tools (ADR-0029).
func pluginLinkRequirementsJSON(artifact pluginArtifact, provenance buildProvenance) string {
	modeSet := map[string]bool{}
	imports := jsonx.Arr{}
	for _, skill := range artifact.plan.Skills {
		for mode := range skill.Variants {
			modeSet[mode] = true
		}
		for _, toolID := range skill.RequiresTools {
			imports = append(imports, toolImport(toolID, skill.ImplementationID, "*"))
		}
		for _, toolID := range skill.VariantToolRequirements[artifact.mode] {
			imports = append(imports, toolImport(toolID, skill.ImplementationID, artifact.mode))
		}
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("2")},
		{K: "id", V: jsonx.Str(artifact.plan.ID)},
		{K: "kind", V: jsonx.Str("plugin")},
		{K: "target", V: jsonx.Str(AgentPlugin.String())},
		{K: "mode", V: jsonx.Str(artifact.mode)},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "dependencies", V: dependenciesJSON(provenance)},
		{K: "modes", V: stringArr(sortedSet(modeSet))},
		{K: "toolImports", V: imports},
	})
}

// writePluginBuildIndex writes the agent-plugin target's integrity index
// (schemaVersion 2): one artifact per plugin and mode.
func writePluginBuildIndex(root string, artifacts []pluginArtifact, provenance buildProvenance, written *[]string) error {
	entries := jsonx.Arr{}
	for _, artifact := range artifacts {
		digest, err := HashDirectory(filepath.Join(root, artifact.dir))
		if err != nil {
			return err
		}
		entries = append(entries, jsonx.Obj{
			{K: "id", V: jsonx.Str(artifact.plan.ID)},
			{K: "mode", V: jsonx.Str(artifact.mode)},
			{K: "path", V: jsonx.Str(artifact.dir)},
			{K: "digest", V: jsonx.Str("sha256:" + digest)},
		})
	}
	index := jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("2")},
		{K: "target", V: jsonx.Str(AgentPlugin.String())},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "artifacts", V: entries},
	}) + "\n"
	return writeFile(filepath.Join(root, ".typeference", "build.json"), index, written)
}

func toolImport(toolID, skillID, mode string) jsonx.Obj {
	return jsonx.Obj{
		{K: "toolId", V: jsonx.Str(toolID)},
		{K: "skillId", V: jsonx.Str(skillID)},
		{K: "mode", V: jsonx.Str(mode)},
	}
}

func dependenciesJSON(provenance buildProvenance) jsonx.Arr {
	dependencies := jsonx.Arr{}
	for _, dependency := range provenance.Dependencies {
		dependencies = append(dependencies, jsonx.Obj{
			{K: "name", V: jsonx.Str(dependency.Name)},
			{K: "version", V: jsonx.Str(dependency.Version)},
			{K: "digest", V: jsonx.Str(dependency.Digest)},
		})
	}
	return dependencies
}

func sortedSet(set map[string]bool) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

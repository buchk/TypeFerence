package compile

import (
	"path/filepath"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// outputAdapter owns client-specific files, shared-skill frontmatter extensions,
// and client-specific marketplace distribution. The portable plugin layout is
// always emitted by the compiler before an adapter contributes its files.
// An adapter never resolves or recomposes the source graph.
type outputAdapter interface {
	writeArtifact(*compilation, string, pluginArtifact, *[]string) error
	writeMarketplace(*compilation, string, []pluginArtifact, *[]string) ([]resource.PackageFile, error)
	writeSkillFrontmatter(*strings.Builder, resource.CopilotFields)
}

type copilotAdapter struct{}
type portableAdapter struct{}

func outputAdapterFor(name string) (outputAdapter, error) {
	switch strings.ToLower(name) {
	case "", "copilot":
		return copilotAdapter{}, nil
	case "portable":
		return portableAdapter{}, nil
	default:
		return nil, resource.Errorf("Unknown output adapter: %s (expected copilot or portable)", name)
	}
}

// Copilot contributes its namespaced custom agents, rules, commands, hooks,
// LSP configuration, skill frontmatter, and GitHub marketplace index.
func (copilotAdapter) writeArtifact(c *compilation, dir string, artifact pluginArtifact, written *[]string) error {
	for _, agent := range artifact.plan.Agents {
		path := filepath.Join(dir, copilotDir, "agents", resource.Leaf(agent.ID)+".agent.md")
		if err := writeFile(path, renderAgent(agent, c.docs), written); err != nil {
			return err
		}
	}
	return c.writeNative(dir, artifact, written)
}

func (copilotAdapter) writeSkillFrontmatter(b *strings.Builder, fields resource.CopilotFields) {
	frontmatterFields(b, fields, true)
}

func (copilotAdapter) writeMarketplace(c *compilation, root string, artifacts []pluginArtifact, written *[]string) ([]resource.PackageFile, error) {
	marketplace := c.project.Marketplace
	if marketplace == nil {
		return nil, nil
	}
	path := filepath.Join(root, ".github", "plugin", "marketplace.json")
	if err := writeFile(path, marketplaceJSON(marketplace, c.project.Version, artifacts)+"\n", written); err != nil {
		return nil, err
	}
	files := sortedFiles(marketplace.Files)
	if err := writeCarried(root, files, written); err != nil {
		return nil, err
	}
	return files, nil
}

// The portable adapter intentionally emits no host-specific behavior.
// Provenance in .typeference/bundle.json still records the full resolved
// composition, but it does not make hooks or custom agents portable.
func (portableAdapter) writeArtifact(_ *compilation, _ string, _ pluginArtifact, _ *[]string) error {
	return nil
}

func (portableAdapter) writeSkillFrontmatter(_ *strings.Builder, _ resource.CopilotFields) {}

func (portableAdapter) writeMarketplace(_ *compilation, _ string, _ []pluginArtifact, _ *[]string) ([]resource.PackageFile, error) {
	return nil, nil
}

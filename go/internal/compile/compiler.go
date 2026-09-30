// Package compile deterministically emits TypeFerence target artifacts
// (docs/specification.md, "Build targets"): the neutral canonical bundle and
// GitHub Agent Plugins, plus optional ARD catalog publication.
package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// Target is a compilation target.
type Target int

// Targets in canonical order.
const (
	Neutral Target = iota
	AgentPlugin
)

var targetNames = map[Target]string{Neutral: "neutral", AgentPlugin: "agent-plugin"}

func (t Target) String() string { return targetNames[t] }

// retiredTargets name the host adapters ADR-0029 retired, so a request for
// one fails with a pointer rather than as an unknown word.
var retiredTargets = map[string]bool{"codex": true, "copilot": true, "cursor": true}

// ParseTargets parses the CLI --target value.
func ParseTargets(value string) ([]Target, error) {
	switch strings.ToLower(value) {
	case "all":
		return []Target{Neutral, AgentPlugin}, nil
	case "neutral":
		return []Target{Neutral}, nil
	case "agent-plugin":
		return []Target{AgentPlugin}, nil
	}
	if retiredTargets[strings.ToLower(value)] {
		return nil, resource.Errorf("The %s target was retired (ADR-0029); build agent-plugin for GitHub Copilot or neutral for the canonical bundle", strings.ToLower(value))
	}
	return nil, resource.Errorf("Unknown target: %s", value)
}

// ArdPublicationOptions configures optional ARD catalog emission.
type ArdPublicationOptions struct {
	PublisherDomain     string
	TrustConfigPath     string
	TrustSignaturesPath string
	AllowUnsignedTrust  bool
}

// Source languages a build can read. Product entry points read only the
// current language; the archival languages exist so the historical
// conformance corpora stay reproducible.
const (
	LanguageCurrent  = ""
	LanguageLegacyV5 = "legacy-v5"
	LanguageLegacyV3 = "legacy-v3"
)

type BuildOptions struct {
	PackagesDir string
	// Language selects an archival source language. CLI, package, language
	// server, and playground builds never set it.
	Language string
}

// Validate resolves every agent in a source directory.
func Validate(source, trustConfigPath string) ([]*resolve.ResolvedAgent, error) {
	return ValidateWithPackages(source, trustConfigPath, "")
}

// ValidateWithPackages resolves every agent and plans every plugin, reporting
// the first composition or packaging error.
func ValidateWithPackages(source, trustConfigPath, packagesDir string) ([]*resolve.ResolvedAgent, error) {
	c, err := prepare(source, trustConfigPath, BuildOptions{PackagesDir: packagesDir})
	if err != nil {
		return nil, err
	}
	return c.resolved, nil
}

// Summary describes a validated package: its agents and the plugin artifacts
// it would emit.
type Summary struct {
	Agents  []*resolve.ResolvedAgent
	Plugins []string
}

// Summarize validates a package and names what it would build.
func Summarize(source, trustConfigPath, packagesDir string) (*Summary, error) {
	c, err := prepare(source, trustConfigPath, BuildOptions{PackagesDir: packagesDir})
	if err != nil {
		return nil, err
	}
	summary := &Summary{Agents: c.resolved}
	for _, artifact := range artifactsOf(c.plugins) {
		summary.Plugins = append(summary.Plugins, artifact.dir)
	}
	return summary, nil
}

// Build compiles a source directory into the requested targets beneath
// output, returning the canonical sorted list of written files.
func Build(source, output string, targets []Target, ard *ArdPublicationOptions) ([]string, error) {
	return BuildWithOptions(source, output, targets, ard, BuildOptions{})
}

func BuildWithOptions(source, output string, targets []Target, ard *ArdPublicationOptions, options BuildOptions) ([]string, error) {
	trustConfigPath := ""
	if ard != nil {
		trustConfigPath = ard.TrustConfigPath
	}
	c, err := prepare(source, trustConfigPath, options)
	if err != nil {
		return nil, err
	}
	requested := distinctSortedTargets(targets)
	if len(requested) == 0 {
		return nil, resource.Errorf("At least one compilation target is required")
	}
	root, err := filepath.Abs(output)
	if err != nil {
		return nil, resource.Errorf("Invalid output directory: %s", output)
	}
	sourceRoot, err := filepath.Abs(source)
	if err != nil {
		return nil, resource.Errorf("Source directory not found: %s", source)
	}
	if samePath(root, sourceRoot) {
		return nil, resource.Errorf("Output directory must not be the source root: %s", output)
	}
	if isBeneath(sourceRoot, root) {
		relative, _ := filepath.Rel(sourceRoot, root)
		first := strings.Split(filepath.ToSlash(relative), "/")[0]
		if first != "dist" && first != "bin" && first != "obj" {
			return nil, resource.Errorf("Output beneath the source root must be under dist, bin, or obj: %s", output)
		}
	}
	for _, target := range requested {
		if target == AgentPlugin && !c.current() {
			return nil, resource.Errorf("The agent-plugin target requires a version 6 package")
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, resource.Errorf("Cannot create output directory: %s", root)
	}
	written := []string{}
	for _, target := range requested {
		targetRoot := filepath.Join(root, target.String())
		if err := os.RemoveAll(targetRoot); err != nil {
			return nil, resource.Errorf("Cannot reset target directory: %s", targetRoot)
		}
		if err := os.MkdirAll(targetRoot, 0o755); err != nil {
			return nil, resource.Errorf("Cannot create target directory: %s", targetRoot)
		}
		switch target {
		case Neutral:
			err = c.writeNeutral(targetRoot, &written)
		case AgentPlugin:
			err = c.writeAgentPlugins(targetRoot, &written)
		}
		if err != nil {
			return nil, err
		}
	}
	if ard != nil {
		if err := c.writeArd(root, requested, ard, &written); err != nil {
			return nil, err
		}
	}
	sort.Strings(written)
	return written, nil
}

func validateAgentArtifactNames(agents []*resolve.ResolvedAgent) error {
	owners := map[string]string{}
	for _, agent := range agents {
		slug := resolve.Leaf(agent.ID)
		if prior, duplicate := owners[slug]; duplicate {
			return resource.Errorf("agents %s and %s produce the same target artifact path %s", prior, agent.ID, slug)
		}
		owners[slug] = agent.ID
	}
	return nil
}

func distinctSortedTargets(targets []Target) []Target {
	seen := map[Target]bool{}
	result := []Target{}
	for _, t := range targets {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func isBeneath(root, path string) bool {
	prefix := root + string(filepath.Separator)
	if runtimeCaseInsensitive {
		return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
	}
	return strings.HasPrefix(path, prefix)
}

func samePath(left, right string) bool {
	if runtimeCaseInsensitive {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func escapeYAML(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return "\"" + value + "\""
}

func writeFile(path, content string, written *[]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return resource.Errorf("Cannot create directory: %s", filepath.Dir(path))
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return resource.Errorf("Cannot write file: %s", path)
	}
	*written = append(*written, path)
	return nil
}

// sortedModes returns a variant map's mode names in canonical order.
func sortedModes(variants map[string]string) []string {
	modes := make([]string, 0, len(variants))
	for mode := range variants {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	return modes
}

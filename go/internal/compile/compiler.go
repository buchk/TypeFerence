// Package compile deterministically emits the agent-plugin target: Agent
// Plugins 1.0 packages and a Copilot marketplace index
// (docs/specification.md, "The agent-plugin target").
package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// TargetName is the only build target.
const TargetName = "agent-plugin"

// CheckTarget accepts the CLI --target value. agent-plugin is the only
// target; "all" is accepted as a synonym.
func CheckTarget(value string) error {
	switch strings.ToLower(value) {
	case "", "all", TargetName:
		return nil
	case "neutral", "codex", "copilot", "cursor":
		return resource.Errorf("The %s target was removed; agent-plugin is the only target", strings.ToLower(value))
	}
	return resource.Errorf("Unknown target: %s", value)
}

// BuildOptions configures a build or validation.
type BuildOptions struct {
	PackagesDir string
	// Candidate is an unpublished package's source directory, validated in
	// place of its locked version. Validation only: a build with a
	// candidate is refused.
	Candidate string
}

// Validate resolves every agent in a source directory.
func Validate(source string) ([]*resolve.ResolvedAgent, error) {
	return ValidateWithPackages(source, "")
}

// ValidateWithPackages resolves every agent and plans every plugin,
// reporting the first composition or packaging error.
func ValidateWithPackages(source, packagesDir string) ([]*resolve.ResolvedAgent, error) {
	c, err := prepare(source, BuildOptions{PackagesDir: packagesDir})
	if err != nil {
		return nil, err
	}
	return c.resolved, nil
}

// Summary describes a validated package: its agents, the plugin artifacts it
// would emit, and the plugins that would compete when installed together.
type Summary struct {
	Agents    []*resolve.ResolvedAgent
	Plugins   []string
	Conflicts []Conflict
}

// Summarize validates a package, optionally with a candidate package
// substituted into its locked graph, and names what it would build.
func Summarize(source string, options BuildOptions) (*Summary, error) {
	c, err := prepare(source, options)
	if err != nil {
		return nil, err
	}
	summary := &Summary{Agents: c.resolved}
	artifacts := artifactsOf(c.plugins)
	for _, artifact := range artifacts {
		summary.Plugins = append(summary.Plugins, artifact.dir)
	}
	summary.Conflicts = compatibilityConflicts(artifacts)
	return summary, nil
}

// Build compiles a source directory into <output>/agent-plugin, returning the
// canonical sorted list of written files.
func Build(source, output string, options BuildOptions) ([]string, error) {
	if options.Candidate != "" {
		return nil, resource.Errorf("a candidate package is validated, never built; publish it and pin it to build")
	}
	c, err := prepare(source, options)
	if err != nil {
		return nil, err
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
	targetRoot := filepath.Join(root, TargetName)
	if err := os.RemoveAll(targetRoot); err != nil {
		return nil, resource.Errorf("Cannot reset target directory: %s", targetRoot)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return nil, resource.Errorf("Cannot create target directory: %s", targetRoot)
	}
	written := []string{}
	if err := c.writeAgentPlugins(targetRoot, &written); err != nil {
		return nil, err
	}
	sort.Strings(written)
	return written, nil
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
	return writeBytes(path, []byte(strings.ReplaceAll(content, "\r\n", "\n")), written)
}

func writeBytes(path string, content []byte, written *[]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return resource.Errorf("Cannot create directory: %s", filepath.Dir(path))
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return resource.Errorf("Cannot write file: %s", path)
	}
	*written = append(*written, path)
	return nil
}

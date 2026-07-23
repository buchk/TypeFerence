// Package compile deterministically emits TypeFerence target artifacts
// (docs/specification.md, "Deterministic compilation"): neutral, codex,
// copilot, and cursor bundles, plus optional ARD catalog publication.
package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"github.com/buchk/TypeFerence/go/internal/trust"
)

// Target is a compilation target.
type Target int

// Targets in canonical order.
const (
	Neutral Target = iota
	Codex
	Copilot
	Cursor
)

var targetNames = map[Target]string{Neutral: "neutral", Codex: "codex", Copilot: "copilot", Cursor: "cursor"}

func (t Target) String() string { return targetNames[t] }

// ParseTargets parses the CLI --target value.
func ParseTargets(value string) ([]Target, error) {
	switch strings.ToLower(value) {
	case "all":
		return []Target{Neutral, Codex, Copilot, Cursor}, nil
	case "neutral":
		return []Target{Neutral}, nil
	case "codex":
		return []Target{Codex}, nil
	case "copilot":
		return []Target{Copilot}, nil
	case "cursor":
		return []Target{Cursor}, nil
	default:
		return nil, resource.Errorf("Unknown target: %s", value)
	}
}

// ArdPublicationOptions configures optional ARD catalog emission.
type ArdPublicationOptions struct {
	PublisherDomain     string
	TrustConfigPath     string
	TrustSignaturesPath string
	AllowUnsignedTrust  bool
}

// Validate resolves every agent in a source directory.
func Validate(source, trustConfigPath string) ([]*resolve.ResolvedAgent, error) {
	loaded, err := trust.Load(source, trustConfigPath)
	if err != nil {
		return nil, err
	}
	trustPath := ""
	if loaded != nil {
		trustPath = loaded.Path
	}
	resources, err := resource.Load(source, trustPath)
	if err != nil {
		return nil, err
	}
	return resolve.New(resources).ResolveAll()
}

// Build compiles a source directory into the requested targets beneath
// output, returning the canonical sorted list of written files.
func Build(source, output string, targets []Target, ard *ArdPublicationOptions) ([]string, error) {
	trustConfigPath := ""
	if ard != nil {
		trustConfigPath = ard.TrustConfigPath
	}
	loaded, err := trust.Load(source, trustConfigPath)
	if err != nil {
		return nil, err
	}
	trustPath := ""
	if loaded != nil {
		trustPath = loaded.Path
	}
	resources, err := resource.Load(source, trustPath)
	if err != nil {
		return nil, err
	}
	all, err := resolve.New(resources).ResolveAll()
	if err != nil {
		return nil, err
	}
	agents := []*resolve.ResolvedAgent{}
	for _, agent := range all {
		if agent.Emit {
			agents = append(agents, agent)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })

	requested := distinctSortedTargets(targets)
	if len(requested) == 0 {
		return nil, resource.Errorf("At least one compilation target is required")
	}
	root, err := filepath.Abs(output)
	if err != nil {
		return nil, resource.Errorf("Invalid output directory: %s", output)
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
		for _, agent := range agents {
			if err := writeTarget(target, targetRoot, agent, &written); err != nil {
				return nil, err
			}
		}
	}
	if ard != nil {
		ardRoot := filepath.Join(root, "ard")
		if err := os.RemoveAll(ardRoot); err != nil {
			return nil, resource.Errorf("Cannot reset target directory: %s", ardRoot)
		}
		if err := os.MkdirAll(ardRoot, 0o755); err != nil {
			return nil, resource.Errorf("Cannot create target directory: %s", ardRoot)
		}
		signatures := map[string]string{}
		signatureKeys := []string{}
		if ard.TrustSignaturesPath != "" {
			sourceRoot, absErr := filepath.Abs(source)
			if absErr != nil {
				return nil, resource.Errorf("Source directory not found: %s", source)
			}
			signaturePath, absErr := filepath.Abs(ard.TrustSignaturesPath)
			if absErr != nil {
				return nil, resource.Errorf("Trust signatures file not found: %s", ard.TrustSignaturesPath)
			}
			if isBeneath(sourceRoot, signaturePath) {
				return nil, resource.Errorf("Trust signatures file must be outside the source root to avoid a digest/signature cycle")
			}
			signatures, signatureKeys, err = trust.LoadSignatures(ard.TrustSignaturesPath)
			if err != nil {
				return nil, err
			}
		}
		var configuration *trust.Configuration
		if loaded != nil {
			configuration = loaded.Configuration
		}
		if len(signatures) > 0 && configuration == nil {
			return nil, resource.Errorf("--trust-signatures requires a trust configuration")
		}
		if ard.AllowUnsignedTrust && configuration == nil {
			return nil, resource.Errorf("--allow-unsigned-trust requires a trust configuration")
		}
		if err := writeArdCatalog(ardRoot, source, root, agents, requested, ard.PublisherDomain,
			configuration, signatures, signatureKeys, ard.AllowUnsignedTrust, &written); err != nil {
			return nil, err
		}
	}
	sort.Strings(written)
	return written, nil
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

func writeTarget(target Target, root string, agent *resolve.ResolvedAgent, written *[]string) error {
	slug := resolve.Leaf(agent.ID)
	switch target {
	case Neutral:
		if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), renderInstructions(agent), written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, "provenance.json"), provenanceJSON(agent.Provenance)+"\n", written); err != nil {
			return err
		}
		for _, skill := range agent.Skills {
			dir := filepath.Join(root, slug, "skills", skillSlug(skill))
			if err := writeFile(filepath.Join(dir, "SKILL.md"), renderSkill(skill), written); err != nil {
				return err
			}
			// A multimodal skill also fans out one SKILL.<mode>.md per variant
			// (ADR-0012). Absent for unimodal skills, so their output is unchanged.
			for _, mode := range sortedModes(skill.Variants) {
				if err := writeFile(filepath.Join(dir, "SKILL."+mode+".md"), renderSkillWith(skill, skill.Variants[mode]), written); err != nil {
					return err
				}
			}
		}
	case Codex:
		if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), renderInstructions(agent), written); err != nil {
			return err
		}
		for _, skill := range agent.Skills {
			if err := writeFile(filepath.Join(root, slug, ".agents", "skills", skillSlug(skill), "SKILL.md"), renderSkill(skill), written); err != nil {
				return err
			}
		}
		if err := writeFile(filepath.Join(root, slug, ".typeference", "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
		config := "[mcp_servers.typeference]\ncommand = \"typeference\"\nargs = [\"serve\", \".typeference\"]\n"
		if err := writeFile(filepath.Join(root, slug, ".codex", "config.toml"), config, written); err != nil {
			return err
		}
	case Copilot:
		if err := writeFile(filepath.Join(root, slug, ".github", "copilot-instructions.md"), renderInstructions(agent), written); err != nil {
			return err
		}
		agentMD := "---\nname: " + slug + "\ndescription: " + escapeYAML(agent.Description) + "\n---\n\n" + renderInstructions(agent)
		if err := writeFile(filepath.Join(root, slug, ".github", "agents", slug+".agent.md"), agentMD, written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, ".typeference", "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
	case Cursor:
		if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), renderInstructions(agent), written); err != nil {
			return err
		}
		rule := "---\ndescription: " + escapeYAML(agent.Description) + "\nglobs:\nalwaysApply: true\n---\n\n" + renderInstructions(agent)
		if err := writeFile(filepath.Join(root, slug, ".cursor", "rules", slug+".mdc"), rule, written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, ".typeference", "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
	}
	return nil
}

func renderInstructions(agent *resolve.ResolvedAgent) string {
	var b strings.Builder
	b.WriteString("# " + agent.DisplayName + "\n\n" + agent.Description + "\n\n")
	if len(agent.WorkingNorms) > 0 {
		b.WriteString("## Working norms\n\n")
		for _, norm := range agent.WorkingNorms {
			b.WriteString("- " + norm + "\n")
		}
		b.WriteString("\n")
	}
	if len(agent.SlotKeys) > 0 {
		b.WriteString("## Context slots\n\n")
		for _, key := range agent.SlotKeys {
			b.WriteString("- `" + key + "`: `" + agent.Slots[key] + "`\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Available skills\n\n")
	for _, skill := range agent.Skills {
		b.WriteString("- `" + skill.DispatchName + "`: " + skill.Description + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func renderSkill(skill resolve.ResolvedSkill) string {
	return renderSkillWith(skill, skill.Instructions)
}

// renderSkillWith renders a skill's SKILL.md using the given instructions, so a
// per-variant file can carry that mode's rendering (ADR-0012).
func renderSkillWith(skill resolve.ResolvedSkill, instructions string) string {
	lines := make([]string, len(skill.ContextFiles))
	for i, file := range skill.ContextFiles {
		lines[i] = "- `" + file + "`"
	}
	return "---\nname: " + skillSlug(skill) + "\ndescription: " + escapeYAML(skill.Description) + "\n---\n\n" +
		strings.TrimSpace(instructions) + "\n\n## Context loaded on invocation\n\n" +
		strings.Join(lines, "\n") + "\n"
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

func skillSlug(skill resolve.ResolvedSkill) string { return resolve.Leaf(skill.CapabilityID) }

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

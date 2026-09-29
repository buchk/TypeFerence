// Package compile deterministically emits TypeFerence target artifacts
// (docs/specification.md, "Deterministic compilation"): neutral, codex,
// copilot, and cursor bundles, plus optional ARD catalog publication.
package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/packages"
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
	return ValidateWithPackages(source, trustConfigPath, "")
}

func ValidateWithPackages(source, trustConfigPath, packagesDir string) ([]*resolve.ResolvedAgent, error) {
	loaded, err := trust.Load(source, trustConfigPath)
	if err != nil {
		return nil, err
	}
	trustPath := ""
	if loaded != nil {
		trustPath = loaded.Path
	}
	resources, _, err := loadCompilationResources(source, trustPath, packagesDir, false)
	if err != nil {
		return nil, err
	}
	return resolve.New(resources).ResolveAll()
}

// Build compiles a source directory into the requested targets beneath
// output, returning the canonical sorted list of written files.
func Build(source, output string, targets []Target, ard *ArdPublicationOptions) ([]string, error) {
	return BuildWithOptions(source, output, targets, ard, BuildOptions{})
}

type BuildOptions struct {
	PackagesDir string
	// LegacyV3 is reserved for reproducing historical conformance fixtures.
	// CLI and package builds never expose it.
	LegacyV3 bool
}

func BuildWithOptions(source, output string, targets []Target, ard *ArdPublicationOptions, options BuildOptions) ([]string, error) {
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
	resources, lockedPackages, err := loadCompilationResources(source, trustPath, options.PackagesDir, options.LegacyV3)
	if err != nil {
		return nil, err
	}
	if _, err := resource.LoadProject(source); err != nil {
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
	if err := validateAgentArtifactNames(agents); err != nil {
		return nil, err
	}
	sourceDigest, err := HashSource(source)
	if err != nil {
		return nil, err
	}
	provenance := buildProvenance{SourceDigest: "sha256:" + sourceDigest, Dependencies: lockedPackages}

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
			if err := writeTarget(target, targetRoot, agent, provenance, &written); err != nil {
				return nil, err
			}
		}
		if err := writeBuildIndex(targetRoot, target, agents, provenance, &written); err != nil {
			return nil, err
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

func loadCompilationResources(source, trustPath, packagesDir string, legacyV3 bool) (map[string]*resource.Document, []packages.LockedPackage, error) {
	rootResources, err := resource.LoadWithOptions(source, trustPath, resource.LoadOptions{AllowLegacyV3: legacyV3})
	if err != nil {
		return nil, nil, err
	}
	dependencies, locked, err := packages.LoadDependencies(source, packagesDir)
	if err != nil {
		return nil, nil, err
	}
	for id, document := range dependencies {
		if _, exists := rootResources[id]; exists {
			return nil, nil, resource.Errorf("root resource cannot shadow locked dependency resource: %s", id)
		}
		rootResources[id] = document
	}
	return rootResources, locked, nil
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

type buildProvenance struct {
	SourceDigest string
	Dependencies []packages.LockedPackage
}

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

func writeTarget(target Target, root string, agent *resolve.ResolvedAgent, provenance buildProvenance, written *[]string) error {
	slug := resolve.Leaf(agent.ID)
	linkPath := filepath.Join(root, slug, ".typeference", "link.json")
	if err := validateTargetContext(target, agent); err != nil {
		return err
	}
	if mode := variantForTarget(target); mode != "" {
		for _, skill := range agent.Skills {
			if len(skill.Variants) > 0 {
				if _, exists := skill.Variants[mode]; !exists {
					return resource.Errorf("%s target requires variant %q on multimodal skill %s",
						target.String(), mode, skill.ImplementationID)
				}
			}
		}
	}
	switch target {
	case Neutral:
		// Canonical bundle: an index doc plus a SKILL.md per skill, fanning out
		// one SKILL.<mode>.md per variant (ADR-0012).
		if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), renderInstructions(agent, false, ""), written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, "provenance.json"), provenanceJSON(agent.Provenance)+"\n", written); err != nil {
			return err
		}
		for _, skill := range agent.Skills {
			if err := writeSkillFiles(filepath.Join(root, slug, "skills", skillSlug(skill)), skill, "", true, written); err != nil {
				return err
			}
		}
	case Codex:
		// Interactive coding surface: an index doc plus one SKILL.md rendering the
		// manual variant.
		if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), renderInstructions(agent, false, ""), written); err != nil {
			return err
		}
		for _, skill := range agent.Skills {
			if err := writeSkillFiles(filepath.Join(root, slug, ".agents", "skills", skillSlug(skill)), skill, variantForTarget(target), false, written); err != nil {
				return err
			}
		}
		if err := writeFile(filepath.Join(root, slug, ".typeference", "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
	case Copilot:
		// No per-skill file: instructions are inlined in the manual variant.
		instructions := renderInstructions(agent, true, variantForTarget(target))
		if err := writeFile(filepath.Join(root, slug, ".github", "copilot-instructions.md"), instructions, written); err != nil {
			return err
		}
		agentMD := "---\nname: " + slug + "\ndescription: " + escapeYAML(agent.Description) + "\n---\n\n" + instructions
		if err := writeFile(filepath.Join(root, slug, ".github", "agents", slug+".agent.md"), agentMD, written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, ".typeference", "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
	case Cursor:
		instructions := renderInstructions(agent, true, variantForTarget(target))
		if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), instructions, written); err != nil {
			return err
		}
		rule := "---\ndescription: " + escapeYAML(agent.Description) + "\nglobs:\nalwaysApply: true\n---\n\n" + instructions
		if err := writeFile(filepath.Join(root, slug, ".cursor", "rules", slug+".mdc"), rule, written); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(root, slug, ".typeference", "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
			return err
		}
	}
	return writeFile(linkPath, linkRequirementsJSON(agent, target, provenance)+"\n", written)
}

func validateTargetContext(target Target, agent *resolve.ResolvedAgent) error {
	provided := map[string]bool{}
	for _, context := range agent.ContextObjects {
		for _, contextType := range context.Satisfies {
			provided[contextType] = true
		}
	}
	for _, skill := range agent.Skills {
		for _, required := range skill.RequiresContextTypes {
			if !provided[required] {
				return resource.Errorf("%s: skill %s requires context type %s, which no held context provides",
					agent.ID, skill.ImplementationID, required)
			}
		}
		modes := sortedModes(skill.Variants)
		if selected := variantForTarget(target); selected != "" {
			modes = []string{selected}
		}
		for _, mode := range modes {
			for _, required := range skill.VariantContextRequirements[mode] {
				if !provided[required] {
					return resource.Errorf("%s target: skill %s mode %s requires context type %s, which no held context provides",
						target.String(), skill.ImplementationID, mode, required)
				}
			}
		}
	}
	return nil
}

func linkRequirementsJSON(agent *resolve.ResolvedAgent, target Target, provenance buildProvenance) string {
	modeSet := map[string]bool{}
	imports := jsonx.Arr{}
	for _, skill := range agent.Skills {
		for mode := range skill.Variants {
			modeSet[mode] = true
		}
		for _, toolID := range skill.RequiresTools {
			imports = append(imports, jsonx.Obj{
				{K: "toolId", V: jsonx.Str(toolID)},
				{K: "skillId", V: jsonx.Str(skill.ImplementationID)},
				{K: "mode", V: jsonx.Str("*")},
			})
		}
		for _, mode := range sortedModes(skill.Variants) {
			for _, toolID := range skill.VariantToolRequirements[mode] {
				imports = append(imports, jsonx.Obj{
					{K: "toolId", V: jsonx.Str(toolID)},
					{K: "skillId", V: jsonx.Str(skill.ImplementationID)},
					{K: "mode", V: jsonx.Str(mode)},
				})
			}
		}
	}
	modes := make([]string, 0, len(modeSet))
	for mode := range modeSet {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	dependencies := jsonx.Arr{}
	for _, dependency := range provenance.Dependencies {
		dependencies = append(dependencies, jsonx.Obj{
			{K: "name", V: jsonx.Str(dependency.Name)},
			{K: "version", V: jsonx.Str(dependency.Version)},
			{K: "digest", V: jsonx.Str(dependency.Digest)},
		})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "agentId", V: jsonx.Str(agent.ID)},
		{K: "target", V: jsonx.Str(target.String())},
		{K: "defaultMode", V: jsonx.Str(variantForTarget(target))},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "dependencies", V: dependencies},
		{K: "modes", V: stringArr(modes)},
		{K: "toolImports", V: imports},
	})
}

// variantForTarget is the invocation-mode variant a target renders for a
// multimodal skill: interactive/human surfaces get "manual"; the neutral
// (canonical) bundle keeps the default and fans every variant out (ADR-0012).
func variantForTarget(t Target) string {
	switch t {
	case Copilot, Cursor, Codex:
		return "manual"
	default:
		return ""
	}
}

// writeSkillFiles writes a skill's SKILL.md (rendered in the given variant, or
// the default when variant is empty) and, when fanout is set, one SKILL.<mode>.md
// per variant (ADR-0012). Shared by every target that emits per-skill files.
func writeSkillFiles(dir string, skill resolve.ResolvedSkill, variant string, fanout bool, written *[]string) error {
	primary := skill.Instructions
	if variant != "" {
		primary = skill.InstructionsFor(variant)
	}
	if err := writeFile(filepath.Join(dir, "SKILL.md"), renderSkillWith(skill, primary), written); err != nil {
		return err
	}
	if fanout {
		for _, mode := range sortedModes(skill.Variants) {
			if err := writeFile(filepath.Join(dir, "SKILL."+mode+".md"), renderSkillWith(skill, skill.Variants[mode]), written); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderInstructions(agent *resolve.ResolvedAgent, inlineInstructions bool, variant string) string {
	var b strings.Builder
	b.WriteString("# " + agent.DisplayName + "\n\n" + agent.Description + "\n\n")
	if len(agent.SlotKeys) > 0 {
		b.WriteString("## Context slots\n\n")
		for _, key := range agent.SlotKeys {
			b.WriteString("- `" + key + "`: `" + agent.Slots[key] + "`\n")
		}
		b.WriteString("\n")
	}
	// Held context objects are inlined so the compiled agent actually contains
	// its context, not just a reference to it (ADR-0013).
	if len(agent.ContextObjects) > 0 {
		b.WriteString("## Context\n\n")
		for _, ref := range agent.ContextObjects {
			heading := ref.DisplayName
			if strings.TrimSpace(heading) == "" {
				heading = ref.ID
			}
			b.WriteString("### " + heading + "\n\n")
			if content := strings.TrimSpace(ref.Content); content != "" {
				b.WriteString(content + "\n\n")
			}
		}
	}
	b.WriteString("## Available skills\n\n")
	for _, skill := range agent.Skills {
		b.WriteString("- `" + skill.DispatchName + "`: " + skill.Description + "\n")
		// Targets with no separate SKILL.md inline the instructions here, in the
		// surface's variant, so those agents actually receive skill behavior
		// rather than just a name (ADR-0012).
		if inlineInstructions {
			b.WriteString("\n" + strings.TrimSpace(skill.InstructionsFor(variant)) + "\n\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// renderSkillWith renders a skill's SKILL.md using the given instructions, so a
// per-variant file can carry that mode's rendering (ADR-0012).
func renderSkillWith(skill resolve.ResolvedSkill, instructions string) string {
	base := "---\nname: " + skillSlug(skill) + "\ndescription: " + escapeYAML(skill.Description) + "\n---\n\n" +
		strings.TrimSpace(instructions) + "\n"
	if len(skill.ContextFiles) == 0 {
		return base
	}
	lines := make([]string, len(skill.ContextFiles))
	for i, file := range skill.ContextFiles {
		lines[i] = "- `" + file + "`"
	}
	return base + "\n## Legacy context loaded on invocation\n\n" +
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

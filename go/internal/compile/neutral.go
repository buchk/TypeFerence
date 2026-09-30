package compile

import (
	"path/filepath"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// writeNeutral emits the canonical all-modes bundle for every emitted agent:
// an index document, bundle metadata, provenance, and one SKILL.md per skill
// with one SKILL.<mode>.md per variant (ADR-0012).
func (c *compilation) writeNeutral(root string, written *[]string) error {
	for _, agent := range c.agents {
		if err := c.writeNeutralAgent(root, agent, written); err != nil {
			return err
		}
	}
	return writeBuildIndex(root, Neutral, c.agents, c.provenance, written)
}

func (c *compilation) writeNeutralAgent(root string, agent *resolve.ResolvedAgent, written *[]string) error {
	slug := resolve.Leaf(agent.ID)
	if err := validateModeContext(agent, "neutral target", nil); err != nil {
		return err
	}
	current := agent.Language >= 6
	instructions := renderLegacyInstructions(agent)
	if current {
		instructions = renderNeutralInstructions(agent)
	}
	if err := writeFile(filepath.Join(root, slug, "AGENTS.md"), instructions, written); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, slug, "bundle.json"), bundleJSON(agent)+"\n", written); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, slug, "provenance.json"), provenanceJSON(agent.Provenance)+"\n", written); err != nil {
		return err
	}
	for _, skill := range agent.Skills {
		if current {
			if err := writeSkillFiles(filepath.Join(root, slug, "skills", skillName(skill)), skill, "", true, written); err != nil {
				return err
			}
			continue
		}
		if err := writeLegacySkillFiles(filepath.Join(root, slug, "skills", legacySkillSlug(skill)), skill, written); err != nil {
			return err
		}
	}
	return writeFile(filepath.Join(root, slug, ".typeference", "link.json"), linkRequirementsJSON(agent, c.provenance)+"\n", written)
}

// validateModeContext checks that every context type a skill requires is
// available: held by the agent, or (version 6) held by the skill itself.
// modes selects the variant renderings to check; nil means every mode.
func validateModeContext(agent *resolve.ResolvedAgent, label string, modes []string) error {
	provided := map[string]bool{}
	for _, context := range agent.ContextObjects {
		for _, contextType := range context.Satisfies {
			provided[contextType] = true
		}
	}
	for _, skill := range agent.Skills {
		available := provided
		if len(skill.ContextObjects) > 0 {
			available = map[string]bool{}
			for t := range provided {
				available[t] = true
			}
			for _, context := range skill.ContextObjects {
				for _, contextType := range context.Satisfies {
					available[contextType] = true
				}
			}
		}
		for _, required := range skill.RequiresContextTypes {
			if !available[required] {
				return resource.Errorf("%s: skill %s requires context type %s, which no held context provides",
					agent.ID, skill.ImplementationID, required)
			}
		}
		checked := modes
		if checked == nil {
			checked = sortedModes(skill.Variants)
		}
		for _, mode := range checked {
			for _, required := range skill.VariantContextRequirements[mode] {
				if !available[required] {
					return resource.Errorf("%s: skill %s mode %s requires context type %s, which no held context provides",
						label, skill.ImplementationID, mode, required)
				}
			}
		}
	}
	return nil
}

// renderLegacyInstructions is the archival neutral index document, preserved
// byte-for-byte for retired source languages.
func renderLegacyInstructions(agent *resolve.ResolvedAgent) string {
	var b strings.Builder
	b.WriteString("# " + agent.DisplayName + "\n\n" + agent.Description + "\n\n")
	if len(agent.SlotKeys) > 0 {
		b.WriteString("## Context slots\n\n")
		for _, key := range agent.SlotKeys {
			b.WriteString("- `" + key + "`: `" + agent.Slots[key] + "`\n")
		}
		b.WriteString("\n")
	}
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
	}
	b.WriteString("\n")
	return b.String()
}

// renderNeutralInstructions is the version 6 neutral index document. The
// agent's description is routing metadata and never enters it (ADR-0029);
// its objectives, held context, and skill index do. A skill description
// appears only in the skill index, the neutral bundle's routing surface.
func renderNeutralInstructions(agent *resolve.ResolvedAgent) string {
	var b strings.Builder
	b.WriteString("# " + agent.DisplayName + "\n\n")
	writeObjectives(&b, agent)
	if len(agent.SlotKeys) > 0 {
		b.WriteString("## Context slots\n\n")
		for _, key := range agent.SlotKeys {
			b.WriteString("- `" + key + "`: `" + agent.Slots[key] + "`\n")
		}
		b.WriteString("\n")
	}
	writeContext(&b, "## Context", agent.ContextObjects)
	if len(agent.Skills) > 0 {
		b.WriteString("## Available skills\n\n")
		for _, skill := range agent.Skills {
			b.WriteString("- `" + skill.DispatchName + "` (`skills/" + skillName(skill) + "/SKILL.md`): " + skill.Description + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeObjectives(b *strings.Builder, agent *resolve.ResolvedAgent) {
	for _, objective := range agent.Objectives {
		if content := strings.TrimSpace(objective.Content); content != "" {
			b.WriteString(content + "\n\n")
		}
	}
}

func writeContext(b *strings.Builder, heading string, refs []resolve.ResolvedContextRef) {
	if len(refs) == 0 {
		return
	}
	b.WriteString(heading + "\n\n")
	for _, ref := range refs {
		title := ref.DisplayName
		if strings.TrimSpace(title) == "" {
			title = resource.Leaf(ref.ID)
		}
		b.WriteString("### " + title + "\n\n")
		if content := strings.TrimSpace(ref.Content); content != "" {
			b.WriteString(content + "\n\n")
		}
	}
}

// skillName is a version 6 skill's emitted name: its identity leaf. It names
// the SKILL.md directory and the slash command (ADR-0029).
func skillName(skill resolve.ResolvedSkill) string { return resolve.Leaf(skill.ImplementationID) }

// legacySkillSlug names an archival skill directory after its capability.
func legacySkillSlug(skill resolve.ResolvedSkill) string { return resolve.Leaf(skill.CapabilityID) }

// renderSkill renders a version 6 SKILL.md: Agent Skills frontmatter, the
// flattened instructions for one mode, and the skill's own context.
func renderSkill(skill resolve.ResolvedSkill, instructions string) string {
	var b strings.Builder
	b.WriteString("---\nname: " + skillName(skill) + "\ndescription: " + escapeYAML(skill.Description) + "\n---\n\n")
	b.WriteString(strings.TrimSpace(instructions) + "\n\n")
	writeContext(&b, "## Context", skill.ContextObjects)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// writeSkillFiles writes a skill's SKILL.md in the given mode (the default
// rendering when mode is empty) and, when fanout is set, one SKILL.<mode>.md
// per variant (ADR-0012).
func writeSkillFiles(dir string, skill resolve.ResolvedSkill, mode string, fanout bool, written *[]string) error {
	primary := skill.Instructions
	if mode != "" {
		primary = skill.InstructionsFor(mode)
	}
	if err := writeFile(filepath.Join(dir, "SKILL.md"), renderSkill(skill, primary), written); err != nil {
		return err
	}
	if fanout {
		for _, variant := range sortedModes(skill.Variants) {
			if err := writeFile(filepath.Join(dir, "SKILL."+variant+".md"), renderSkill(skill, skill.Variants[variant]), written); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeLegacySkillFiles is the archival skill layout, preserved byte-for-byte.
func writeLegacySkillFiles(dir string, skill resolve.ResolvedSkill, written *[]string) error {
	if err := writeFile(filepath.Join(dir, "SKILL.md"), renderLegacySkill(skill, skill.Instructions), written); err != nil {
		return err
	}
	for _, mode := range sortedModes(skill.Variants) {
		if err := writeFile(filepath.Join(dir, "SKILL."+mode+".md"), renderLegacySkill(skill, skill.Variants[mode]), written); err != nil {
			return err
		}
	}
	return nil
}

func renderLegacySkill(skill resolve.ResolvedSkill, instructions string) string {
	base := "---\nname: " + legacySkillSlug(skill) + "\ndescription: " + escapeYAML(skill.Description) + "\n---\n\n" +
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

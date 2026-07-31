// Tool and invocation-mode dependency validation.
package resolve

import (
	"sort"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func (r *Resolver) validateTool(tool *resource.Document) error {
	if _, err := canonicalJSON(tool.InputSchema); err != nil {
		return resource.Errorf("%s: invalid tool inputSchema: %s", tool.ID, err)
	}
	if _, err := canonicalJSON(tool.OutputSchema); err != nil {
		return resource.Errorf("%s: invalid tool outputSchema: %s", tool.ID, err)
	}
	return nil
}

// checkSkillDependencies verifies, at agent level, that every resolved skill's
// required context types are provided by held context and its required tools are
// declared (ADR-0013, ADR-0017).
func (r *Resolver) checkSkillDependencies(agentID string, skills map[string]ResolvedSkill, contextRefs []string) error {
	provided, err := r.providedContextTypes(contextRefs)
	if err != nil {
		return err
	}
	capabilityIDs := make([]string, 0, len(skills))
	for capabilityID := range skills {
		capabilityIDs = append(capabilityIDs, capabilityID)
	}
	sort.Strings(capabilityIDs)
	for _, capabilityID := range capabilityIDs {
		skill := skills[capabilityID]
		for _, required := range skill.RequiresContextTypes {
			if !provided[required] {
				return resource.Errorf("%s: skill %s requires context type %s, which no held context provides",
					agentID, skill.ImplementationID, required)
			}
		}
		for _, toolID := range skill.RequiresTools {
			if _, err := r.require(toolID, "tool"); err != nil {
				return resource.Errorf("%s: skill %s requires tool %s, which is not declared",
					agentID, skill.ImplementationID, toolID)
			}
		}
		for _, mode := range sortedRequirementModes(skill) {
			for _, required := range skill.VariantContextRequirements[mode] {
				if _, err := r.require(required, "contextType"); err != nil {
					return resource.Errorf("%s: skill %s variant %s requires context type %s, which is not declared",
						agentID, skill.ImplementationID, mode, required)
				}
			}
			for _, toolID := range skill.VariantToolRequirements[mode] {
				if _, err := r.require(toolID, "tool"); err != nil {
					return resource.Errorf("%s: skill %s variant %s requires tool %s, which is not declared",
						agentID, skill.ImplementationID, mode, toolID)
				}
			}
		}
	}
	return nil
}

// resolveVariants turns authored variants into a mode->instructions map and
// selects the default (neutral) rendering. The default preference is
// pipeline > manual > a2a, falling back to the alphabetically-first mode; a
// target adapter may later select a surface-appropriate variant (ADR-0012).
func resolveVariants(v map[string]resource.Variant) (string, map[string]string) {
	if len(v) == 0 {
		return "", nil
	}
	resolved := map[string]string{}
	names := make([]string, 0, len(v))
	for name := range v {
		resolved[name] = v[name].Instructions
		names = append(names, name)
	}
	sort.Strings(names)
	for _, pref := range []string{"pipeline", "manual", "a2a"} {
		if ins, ok := resolved[pref]; ok {
			return ins, resolved
		}
	}
	return resolved[names[0]], resolved
}

func variantContextRequirements(impl *resource.Document) map[string][]string {
	if len(impl.Variants) == 0 {
		return nil
	}
	result := map[string][]string{}
	for _, mode := range sortedKeys(impl.Variants) {
		result[mode] = distinct(append([]string{}, impl.Variants[mode].RequiresContextTypes...))
	}
	return result
}

func variantToolRequirements(impl *resource.Document) map[string][]string {
	if len(impl.Variants) == 0 {
		return nil
	}
	result := map[string][]string{}
	for _, mode := range sortedKeys(impl.Variants) {
		result[mode] = distinct(append([]string{}, impl.Variants[mode].RequiresTools...))
	}
	return result
}

func sortedRequirementModes(skill ResolvedSkill) []string {
	set := map[string]bool{}
	for mode := range skill.VariantContextRequirements {
		set[mode] = true
	}
	for mode := range skill.VariantToolRequirements {
		set[mode] = true
	}
	return sortedStringSet(set)
}

func sortedKeys(m map[string]resource.Variant) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// intersectAllowLists computes the effective allowedContextTypes for a
// component: the intersection of every non-empty allow-list among itself and
// its embedded components (empty result means unrestricted; ADR-0013).

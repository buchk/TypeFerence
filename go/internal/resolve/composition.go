// Composition and capability-binding resolution.
package resolve

import (
	"reflect"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

type slotCandidate struct {
	agent string
	value string
	depth int
}

func (r *Resolver) mergeSlots(id string, current *resource.Document, embedded []*ResolvedAgent) (map[string]string, []string, map[string]int, error) {
	candidates := map[string][]slotCandidate{}
	order := []string{}
	for _, component := range embedded {
		for _, key := range component.SlotKeys {
			if _, ok := candidates[key]; !ok {
				order = append(order, key)
			}
			candidates[key] = append(candidates[key], slotCandidate{
				agent: component.ID,
				value: component.Slots[key],
				depth: r.slotDepths[component.ID][key] + 1,
			})
		}
	}
	result := map[string]string{}
	depths := map[string]int{}
	for _, key := range order {
		group := candidates[key]
		minDepth := group[0].depth
		for _, c := range group {
			if c.depth < minDepth {
				minDepth = c.depth
			}
		}
		nearest := []slotCandidate{}
		for _, c := range group {
			if c.depth == minDepth {
				nearest = append(nearest, c)
			}
		}
		if len(nearest) > 1 {
			if _, declaredLocally := current.Slots[key]; !declaredLocally {
				agents := make([]string, len(nearest))
				for i, c := range nearest {
					agents[i] = c.agent
				}
				return nil, nil, nil, resource.Errorf(
					"%s: embedded slot '%s' is ambiguous between %s; declare it on %s to resolve the conflict",
					id, key, strings.Join(agents, ", "), id)
			}
		}
		result[key] = nearest[0].value
		depths[key] = minDepth
	}
	for _, key := range resource.SortedKeys(current.Slots) {
		result[key] = normalizePath(current.Slots[key])
		depths[key] = 0
	}
	keys := make([]string, 0, len(result))
	for key := range result {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return result, keys, depths, nil
}

type skillCandidate struct {
	agent string
	skill ResolvedSkill
	depth int
}

func (r *Resolver) mergeSkills(id string, current *resource.Document, embedded []*ResolvedAgent, contexts []string) (map[string]ResolvedSkill, map[string]int, error) {
	localCapabilities := map[string]bool{}
	for _, binding := range current.Skills {
		// An abstract requirement is not a local binding: it neither resolves
		// promotion ambiguity nor conflicts with a concrete binding of the same
		// capability alongside it (mandate and fulfil in one place is legal).
		if isBlank(binding.Ref) {
			continue
		}
		capabilityID, err := r.resolveCapabilityID(binding, id)
		if err != nil {
			return nil, nil, err
		}
		if localCapabilities[capabilityID] {
			return nil, nil, resource.Errorf("%s: a capability cannot be bound more than once", id)
		}
		localCapabilities[capabilityID] = true
	}

	candidates := map[string][]skillCandidate{}
	order := []string{}
	for _, component := range embedded {
		for _, skill := range component.Skills {
			if _, ok := candidates[skill.CapabilityID]; !ok {
				order = append(order, skill.CapabilityID)
			}
			candidates[skill.CapabilityID] = append(candidates[skill.CapabilityID], skillCandidate{
				agent: component.ID,
				skill: skill,
				depth: r.skillDepths[component.ID][skill.CapabilityID] + 1,
			})
		}
	}

	result := map[string]ResolvedSkill{}
	depths := map[string]int{}
	// sealedBy records the embedded component that sealed a capability, so a
	// shallower or local binding overriding it is a compile error (ADR-0016).
	sealedBy := map[string]string{}
	for _, capabilityID := range order {
		group := candidates[capabilityID]
		minDepth := group[0].depth
		for _, c := range group {
			if c.depth < minDepth {
				minDepth = c.depth
			}
		}
		nearest := []skillCandidate{}
		for _, c := range group {
			if c.depth == minDepth {
				nearest = append(nearest, c)
			}
		}
		if len(nearest) > 1 && !localCapabilities[capabilityID] {
			identical := true
			for _, candidate := range nearest[1:] {
				if !samePromotedSkill(nearest[0].skill, candidate.skill) {
					identical = false
					break
				}
			}
			if !identical {
				agents := make([]string, len(nearest))
				for i, c := range nearest {
					agents[i] = c.agent
				}
				return nil, nil, resource.Errorf(
					"%s: embedded capability '%s' is ambiguous between %s; bind the capability on %s to resolve the conflict",
					id, capabilityID, strings.Join(agents, ", "), id)
			}
		}
		result[capabilityID] = nearest[0].skill
		depths[capabilityID] = minDepth
		// A sealed candidate may not be overridden by a shallower binding: the
		// chosen (nearest) skill must be the sealed one itself.
		for _, c := range group {
			if c.skill.Sealed {
				sealedBy[capabilityID] = c.agent
				if result[capabilityID].ImplementationID != c.skill.ImplementationID {
					return nil, nil, resource.Errorf(
						"%s: capability '%s' is sealed by %s and cannot be overridden",
						id, capabilityID, c.agent)
				}
			}
		}
	}

	for _, binding := range current.Skills {
		// An abstract requirement fulfills nothing; it only mandates that some
		// component bind the capability. Presence is checked after the merge.
		if isBlank(binding.Ref) {
			continue
		}
		implementation, err := r.require(binding.Ref, "skill")
		if err != nil {
			return nil, nil, err
		}
		capabilityID, err := r.resolveCapabilityID(binding, id)
		if err != nil {
			return nil, nil, err
		}
		if source, sealed := sealedBy[capabilityID]; sealed {
			return nil, nil, resource.Errorf(
				"%s: capability '%s' is sealed by %s and cannot be rebound",
				id, capabilityID, source)
		}
		capability, err := r.require(capabilityID, "capability")
		if err != nil {
			return nil, nil, err
		}
		if err := ensureImplementsCapability(capability, implementation, id); err != nil {
			return nil, nil, err
		}
		if promoted, ok := result[capabilityID]; ok {
			if err := ensureSameCapability(promoted, capability, id); err != nil {
				return nil, nil, err
			}
		}
		inputSchema, err := canonicalJSON(implementation.InputSchema)
		if err != nil {
			return nil, nil, err
		}
		outputSchema, err := canonicalJSON(implementation.OutputSchema)
		if err != nil {
			return nil, nil, err
		}
		instructions := implementation.Instructions
		defaultInstructions, variants := resolveVariants(implementation.Variants)
		if variants != nil {
			instructions = defaultInstructions
		}
		result[capabilityID] = ResolvedSkill{
			CapabilityID:               capabilityID,
			ImplementationID:           implementation.ID,
			Description:                implementation.Description,
			Instructions:               instructions,
			Variants:                   variants,
			InputSchema:                inputSchema,
			OutputSchema:               outputSchema,
			ContextFiles:               distinct(append(append([]string{}, contexts...), normalizeAll(implementation.ContextFiles)...)),
			RequiresContextTypes:       append([]string{}, implementation.RequiresContextTypes...),
			RequiresTools:              append([]string{}, implementation.RequiresTools...),
			VariantContextRequirements: variantContextRequirements(implementation),
			VariantToolRequirements:    variantToolRequirements(implementation),
			Exposed:                    capability.Visibility == "exposed",
			Sealed:                     binding.Sealed,
			Required:                   binding.Required,
			Provenance: []ProvenanceEntry{
				{Field: "skill.capability", Source: capabilityID},
				{Field: "skill.implementation", Source: implementation.ID},
			},
		}
		depths[capabilityID] = 0
	}
	return result, depths, nil
}

// samePromotedSkill compares the semantic member carried through embedding.
// DispatchName is recomputed for every embedding component and provenance records
// the contributing paths, so neither makes two otherwise-identical members
// ambiguous.
func samePromotedSkill(left, right ResolvedSkill) bool {
	left.DispatchName, right.DispatchName = "", ""
	left.Provenance, right.Provenance = nil, nil
	return reflect.DeepEqual(left, right)
}

// mergeRequired computes the capability ids mandated for this component: those
// its embedded components mandate, plus those it marks required itself. A
// binding may mandate a capability without fulfilling it (no ref), which is how
// a profile demands a control its embedders must supply (ADR-0016). Returns the
// canonical id list and a capability -> mandating component map for diagnostics.
func (r *Resolver) mergeRequired(id string, current *resource.Document, embedded []*ResolvedAgent) ([]string, map[string]string, error) {
	requiredBy := map[string]string{}
	for _, component := range embedded {
		for _, capabilityID := range component.RequiredCapabilities {
			if _, seen := requiredBy[capabilityID]; !seen {
				requiredBy[capabilityID] = component.ID
			}
		}
	}
	for _, binding := range current.Skills {
		if !binding.Required {
			continue
		}
		capabilityID := ""
		if isBlank(binding.Ref) {
			capabilityID = *binding.Capability
		} else {
			resolved, err := r.resolveCapabilityID(binding, id)
			if err != nil {
				return nil, nil, err
			}
			capabilityID = resolved
		}
		if _, err := r.require(capabilityID, "capability"); err != nil {
			return nil, nil, err
		}
		requiredBy[capabilityID] = id
	}
	return sortedStringMapKeys(requiredBy), requiredBy, nil
}

// checkRequiredCapabilities verifies every mandated capability is bound after
// composition. A profile MAY carry unfulfilled requirements — that is precisely
// what makes it abstract — but an agent is concrete and must satisfy them all.
// Presence is the axis: what binds the capability is sealing's concern, not this
// check's (ADR-0016).
func checkRequiredCapabilities(agentID string, requiredBy map[string]string, skills map[string]ResolvedSkill) error {
	for _, capabilityID := range sortedStringMapKeys(requiredBy) {
		if _, bound := skills[capabilityID]; bound {
			continue
		}
		if source := requiredBy[capabilityID]; source != agentID {
			return resource.Errorf("%s: capability '%s' is required by %s, but no skill binds it; bind it on %s",
				agentID, capabilityID, source, agentID)
		}
		return resource.Errorf("%s: capability '%s' is required, but no skill binds it", agentID, capabilityID)
	}
	return nil
}

func (r *Resolver) resolveCapabilityID(binding resource.SkillBinding, agent string) (string, error) {
	implementation, err := r.require(binding.Ref, "skill")
	if err != nil {
		return "", err
	}
	if isBlank(implementation.Binds) {
		return "", resource.Errorf("%s: skill %s does not bind a capability", agent, implementation.ID)
	}
	if binding.Capability != nil && *binding.Capability != implementation.Binds {
		return "", resource.Errorf("%s: binding declares capability %s, but skill %s binds %s",
			agent, *binding.Capability, implementation.ID, implementation.Binds)
	}
	if binding.Capability != nil {
		return *binding.Capability, nil
	}
	return implementation.Binds, nil
}

func (r *Resolver) validateSkillImplementation(implementation *resource.Document) error {
	if isBlank(implementation.Binds) {
		return resource.Errorf("Skill %s does not bind a capability", implementation.ID)
	}
	capability, err := r.require(implementation.Binds, "capability")
	if err != nil {
		return err
	}
	return ensureImplementsCapability(capability, implementation, implementation.ID)
}

func intersectAllowLists(embedded []*ResolvedAgent, current *resource.Document) []string {
	lists := [][]string{}
	for _, c := range embedded {
		if c.HasAllowedContextTypes || len(c.AllowedContextTypes) > 0 {
			lists = append(lists, c.AllowedContextTypes)
		}
	}
	if current.HasAllowedContextTypes || len(current.AllowedContextTypes) > 0 {
		lists = append(lists, current.AllowedContextTypes)
	}
	if len(lists) == 0 {
		return nil
	}
	result := append([]string{}, lists[0]...)
	for _, l := range lists[1:] {
		set := map[string]bool{}
		for _, x := range l {
			set[x] = true
		}
		filtered := result[:0:0]
		for _, x := range result {
			if set[x] {
				filtered = append(filtered, x)
			}
		}
		result = filtered
	}
	return distinct(result)
}

func hasAllowedContextTypes(embedded []*ResolvedAgent, current *resource.Document) bool {
	if current.HasAllowedContextTypes || len(current.AllowedContextTypes) > 0 {
		return true
	}
	for _, component := range embedded {
		if component.HasAllowedContextTypes || len(component.AllowedContextTypes) > 0 {
			return true
		}
	}
	return false
}

// checkAllowedContext verifies each held context object satisfies at least one
// allowed contextType. An explicit empty list is deny-all; omission is
// unrestricted.
func ensureSameCapability(promoted ResolvedSkill, capability *resource.Document, agent string) error {
	capabilityInput, err := canonicalJSON(capability.InputSchema)
	if err != nil {
		return err
	}
	capabilityOutput, err := canonicalJSON(capability.OutputSchema)
	if err != nil {
		return err
	}
	if promoted.InputSchema != capabilityInput || promoted.OutputSchema != capabilityOutput {
		return resource.Errorf("%s: promoted implementation %s changes the public contract of %s",
			agent, promoted.ImplementationID, capability.ID)
	}
	return nil
}

func ensureImplementsCapability(capability, implementation *resource.Document, agent string) error {
	if implementation.Binds != capability.ID {
		return resource.Errorf("%s: implementation %s binds %s, not capability %s",
			agent, implementation.ID, implementation.Binds, capability.ID)
	}
	capabilityInput, err := canonicalJSON(capability.InputSchema)
	if err != nil {
		return err
	}
	capabilityOutput, err := canonicalJSON(capability.OutputSchema)
	if err != nil {
		return err
	}
	implementationInput, err := canonicalJSON(implementation.InputSchema)
	if err != nil {
		return err
	}
	implementationOutput, err := canonicalJSON(implementation.OutputSchema)
	if err != nil {
		return err
	}
	if capabilityInput != implementationInput || capabilityOutput != implementationOutput {
		return resource.Errorf("%s: implementation %s changes the public contract of %s",
			agent, implementation.ID, capability.ID)
	}
	return nil
}

func withDispatch(skill ResolvedSkill, agentID string) ResolvedSkill {
	skill.DispatchName = Leaf(agentID) + "." + Leaf(skill.CapabilityID)
	return skill
}

// InstructionsFor returns the instructions for an invocation mode: the variant's
// rendering when this is a multimodal skill that declares the mode, otherwise the
// default Instructions (ADR-0012). Lets a surface pick its face — e.g. a callable
// card selects the a2a variant.
func concatNorms(embedded []*ResolvedAgent, current *resource.Document) []string {
	values := []string{}
	for _, component := range embedded {
		values = append(values, component.WorkingNorms...)
	}
	return append(values, current.WorkingNorms...)
}

func concatContexts(embedded []*ResolvedAgent, current *resource.Document) []string {
	values := []string{}
	for _, component := range embedded {
		values = append(values, component.ContextFiles...)
	}
	return append(values, current.ContextFiles...)
}

func normalizeAll(values []string) []string {
	result := make([]string, len(values))
	for i, v := range values {
		result[i] = normalizePath(v)
	}
	return result
}

func normalizePath(value string) string {
	return strings.TrimLeft(strings.ReplaceAll(value, "\\", "/"), "/")
}

func distinct(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func canonicalJSON(raw string) (string, error) {
	v, err := jsonx.Parse(raw)
	if err != nil {
		return "", resource.Errorf("invalid JSON schema: %s", err)
	}
	return jsonx.Compact(v), nil
}

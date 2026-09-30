// Structural interface resolution and satisfaction.
package resolve

import "github.com/buchk/TypeFerence/go/internal/resource"

func (r *Resolver) resolveInterface(id string, visiting map[string]bool) (*interfaceContract, error) {
	if cached, ok := r.interfaceCache[id]; ok {
		return cached, nil
	}
	current, err := r.require(id, "interface")
	if err != nil {
		return nil, err
	}
	if visiting[id] {
		return nil, resource.Errorf("Interface embedding cycle detected at %s", id)
	}
	visiting[id] = true
	defer delete(visiting, id)

	slots := []string{}
	slotTypes := map[string]string{}
	skills := []string{}
	for _, embedID := range current.Embeds {
		embedded, embErr := r.resolveInterface(embedID, visiting)
		if embErr != nil {
			return nil, embErr
		}
		slots = append(slots, embedded.slots...)
		for name, contextType := range embedded.slotTypes {
			if prior, exists := slotTypes[name]; exists && prior != contextType {
				return nil, resource.Errorf("%s: embedded interfaces require incompatible context types for slot %s", id, name)
			}
			slotTypes[name] = contextType
		}
		skills = append(skills, embedded.skills...)
	}
	for _, capability := range current.RequiresCapabilities {
		if _, capErr := r.requireCapability(capability); capErr != nil {
			return nil, capErr
		}
	}
	for name, contextType := range current.RequiredSlotTypes {
		if _, err := r.require(contextType, "contextType"); err != nil {
			return nil, err
		}
		slotTypes[name] = contextType
	}
	contract := &interfaceContract{
		slots:     distinct(append(slots, current.RequiresSlots...)),
		slotTypes: slotTypes,
		skills:    distinct(append(skills, current.RequiresCapabilities...)),
	}
	r.interfaceCache[id] = contract
	return contract, nil
}

func (r *Resolver) satisfiesContract(contract *interfaceContract, slots map[string]string, skills map[string]ResolvedSkill) bool {
	for _, slot := range contract.slots {
		value, ok := slots[slot]
		if !ok {
			return false
		}
		if requiredType := contract.slotTypes[slot]; requiredType != "" {
			context, exists := r.resources[value]
			if !exists || context.Kind != "context" {
				return false
			}
			closure, err := r.contextTypeClosure(context.ContextType, map[string]bool{})
			if err != nil || !contains(closure, requiredType) {
				return false
			}
		}
	}
	for _, skill := range contract.skills {
		if _, ok := skills[skill]; !ok {
			return false
		}
	}
	return true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// contextTypeClosure returns the set of contextType ids a context object of the
// given type satisfies: the type itself plus every type it transitively embeds
// (refinement). A governedX that embeds X satisfies both (ADR-0013).

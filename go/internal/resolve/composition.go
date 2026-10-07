package resolve

import (
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// candidate is the composition state of one capability or agent role: the
// winning member so far and, for capabilities, whether any layer requires
// one. An abstract requirement adds an obligation; it never selects or
// erases an implementation.
type candidate struct {
	member   string
	required bool
	depth    int
	// sources are the resources that contributed the winning member, in
	// first-seen order. Identical members at one depth converge and keep
	// every contributor.
	sources   []string
	ambiguous []string
}

// composite is a plugin's or profile's composition state (ADR-0006).
type composite struct {
	id             string
	documents      []string
	documentSource map[string][]string
	// bindings are skill bindings by slot (a capability, or a skill
	// instance's identity); agents are agents by role. Both promote by the
	// same rule.
	bindings map[string]*candidate
	agents   map[string]*candidate
	// declared are the parameters profiles declare as their contract.
	declared       map[string]string
	declaredSource map[string]string
	// with are a plugin's parameter bindings: its own and those of the
	// plugins it embeds, shallowest first.
	with          map[string]*candidate
	bindingSource map[string][]string
	// rules, commands, hooks, and LSP servers are the native components the
	// composition holds, in first-seen order, with every contributor
	// (ADR-0007).
	rules        []string
	commands     []string
	hooks        []string
	lsp          []string
	nativeSource map[string][]string
}

// addNative holds a native component once and records its contributors.
func (c *composite) addNative(list *[]string, id string, sources ...string) {
	if _, seen := c.nativeSource[id]; !seen {
		*list = append(*list, id)
	}
	c.nativeSource[id] = appendDistinct(c.nativeSource[id], sources...)
}

// promote returns a copy of an embedded member one level deeper.
func promote(cand *candidate) *candidate {
	promoted := *cand
	promoted.depth++
	promoted.ambiguous = nil
	promoted.sources = append([]string{}, cand.sources...)
	return &promoted
}

// compose resolves a plugin's or profile's embedding graph from embedded
// resources toward the embedding one.
func (r *Resolver) compose(id string, stack []string) (*composite, error) {
	if cached, ok := r.composites[id]; ok {
		return cached, nil
	}
	doc := r.docs[id]
	if doc == nil {
		return nil, resource.Errorf("%s is not in this build", id)
	}
	for _, prior := range stack {
		if prior == id {
			return nil, resource.Errorf("embedding cycle detected: %s", strings.Join(append(stack, id), " -> "))
		}
	}
	stack = append(stack, id)
	c := &composite{
		id:             id,
		documentSource: map[string][]string{},
		bindings:       map[string]*candidate{},
		agents:         map[string]*candidate{},
		declared:       map[string]string{},
		declaredSource: map[string]string{},
		with:           map[string]*candidate{},
		bindingSource:  map[string][]string{},
		nativeSource:   map[string][]string{},
	}
	for _, embedID := range doc.Embeds {
		embedded := r.docs[embedID]
		switch {
		case embedded.Kind == "profile":
		case embedded.Kind == "plugin" && doc.Kind == "plugin":
		case embedded.Kind == "plugin":
			return nil, resource.Errorf("%s: a profile cannot embed plugin %s; embed the profile from the plugin instead", doc.Path, embedID)
		default:
			return nil, resource.Errorf("%s: embeds %s, which is a %s; embed profiles or plugins", doc.Path, embedID, embedded.Kind)
		}
		sub, err := r.compose(embedID, stack)
		if err != nil {
			return nil, err
		}
		for _, docID := range sub.documents {
			c.addDocument(docID, sub.documentSource[docID]...)
		}
		for _, id := range sub.rules {
			c.addNative(&c.rules, id, sub.nativeSource[id]...)
		}
		for _, id := range sub.commands {
			c.addNative(&c.commands, id, sub.nativeSource[id]...)
		}
		for _, id := range sub.hooks {
			c.addNative(&c.hooks, id, sub.nativeSource[id]...)
		}
		for _, id := range sub.lsp {
			c.addNative(&c.lsp, id, sub.nativeSource[id]...)
		}
		for _, capability := range resource.SortedKeys(sub.bindings) {
			c.merge(c.bindings, capability, promote(sub.bindings[capability]))
		}
		for _, role := range resource.SortedKeys(sub.agents) {
			c.merge(c.agents, role, promote(sub.agents[role]))
		}
		for _, name := range resource.SortedKeys(sub.declared) {
			if err := c.declare(doc.Path, name, sub.declared[name], sub.declaredSource[name]); err != nil {
				return nil, err
			}
		}
		for _, name := range resource.SortedKeys(sub.with) {
			c.merge(c.with, name, promote(sub.with[name]))
		}
	}
	for _, ref := range doc.Context {
		c.addDocument(ref.ID, id)
	}
	for _, ruleID := range doc.Rules {
		c.addNative(&c.rules, ruleID, id)
	}
	for _, commandID := range doc.Commands {
		c.addNative(&c.commands, commandID, id)
	}
	for _, hookID := range doc.Hooks {
		c.addNative(&c.hooks, hookID, id)
	}
	for _, lspID := range doc.LSPServers {
		c.addNative(&c.lsp, lspID, id)
	}
	for _, name := range resource.SortedKeys(doc.Parameters) {
		if err := c.declare(doc.Path, name, doc.Parameters[name], id); err != nil {
			return nil, err
		}
	}
	localRoles := map[string]string{}
	for _, agentID := range doc.Agents {
		role := r.docs[agentID].Role
		if prior, listed := localRoles[role]; listed {
			return nil, resource.Errorf("%s: lists agents %s and %s, which share the role %s; list only the one that should ship", doc.Path, prior, agentID, role)
		}
		localRoles[role] = agentID
		c.agents[role] = &candidate{member: agentID, sources: []string{id}}
	}
	local := map[string]string{}
	for _, binding := range doc.Skills {
		slot := ""
		if binding.Ref != "" {
			skill := r.docs[binding.Ref]
			if binding.Capability != nil && *binding.Capability != skill.Binds {
				return nil, resource.Errorf("%s: skill %s implements %s, not %s", doc.Path, binding.Ref, skill.Binds, *binding.Capability)
			}
			slot = skill.Slot
		} else {
			slot = *binding.Capability
		}
		if prior, bound := local[slot]; bound {
			return nil, resource.Errorf("%s: binds %s twice (%s and %s); a resource binds each capability, or skill instance, once, and a shallower layer rebinds it", doc.Path, slot, prior, describeBinding(binding))
		}
		local[slot] = describeBinding(binding)
		capability := slot
		existing, ok := c.bindings[capability]
		if binding.Ref == "" {
			// An abstract requirement adds an obligation and keeps whatever
			// implementation an embedded layer supplies.
			if ok {
				existing.required = true
			} else {
				c.bindings[capability] = &candidate{required: true}
			}
			continue
		}
		required := binding.Required || (ok && existing.required)
		c.bindings[capability] = &candidate{member: binding.Ref, required: required, sources: []string{id}}
	}
	for _, name := range resource.SortedKeys(doc.With) {
		c.with[name] = &candidate{member: doc.With[name], sources: []string{id}}
	}
	for _, capability := range resource.SortedKeys(c.bindings) {
		cand := c.bindings[capability]
		if len(cand.ambiguous) > 0 {
			return nil, resource.Errorf("%s: capability %s is ambiguous between %s and %s at the same embedding depth; bind one locally", doc.Path, capability, cand.member, strings.Join(cand.ambiguous, ", "))
		}
		if cand.member != "" {
			c.bindingSource[cand.member] = cand.sources
		}
	}
	for _, role := range resource.SortedKeys(c.agents) {
		cand := c.agents[role]
		if len(cand.ambiguous) > 0 {
			return nil, resource.Errorf("%s: agent role %s is ambiguous between %s and %s at the same embedding depth; list the one that should ship", doc.Path, role, cand.member, strings.Join(cand.ambiguous, ", "))
		}
	}
	for _, name := range resource.SortedKeys(c.with) {
		b := c.with[name]
		if len(b.ambiguous) > 0 {
			return nil, resource.Errorf("%s: parameter '%s' is bound to %s and %s by plugins embedded at the same depth; bind it with 'with'", doc.Path, name, b.member, strings.Join(b.ambiguous, ", "))
		}
	}
	if doc.Kind == "profile" {
		if err := r.checkContract(doc, c); err != nil {
			return nil, err
		}
	}
	r.composites[id] = c
	return c, nil
}

// addDocument holds a document once and records every resource that
// contributed it.
func (c *composite) addDocument(id string, sources ...string) {
	if _, seen := c.documentSource[id]; !seen {
		c.documents = append(c.documents, id)
	}
	c.documentSource[id] = appendDistinct(c.documentSource[id], sources...)
}

func (c *composite) declare(where, name, typeID, source string) error {
	if prior, exists := c.declared[name]; exists && prior != typeID {
		return resource.Errorf("%s: parameter '%s' is declared as %s by %s and as %s by %s; one name has one context type", where, name, prior, c.declaredSource[name], typeID, source)
	}
	if _, exists := c.declared[name]; !exists {
		c.declared[name], c.declaredSource[name] = typeID, source
	}
	return nil
}

// merge promotes a member from an embedded resource. Requirements
// accumulate at every depth. Among members, the shallowest wins; at one
// depth, identical members converge and different ones are ambiguous unless
// a shallower layer resolves them.
func (c *composite) merge(members map[string]*candidate, key string, cand *candidate) {
	existing, ok := members[key]
	if !ok {
		members[key] = cand
		return
	}
	existing.required = existing.required || cand.required
	switch {
	case cand.member == "":
		return
	case existing.member == "" || cand.depth < existing.depth:
		existing.member, existing.depth, existing.sources, existing.ambiguous = cand.member, cand.depth, cand.sources, nil
	case cand.depth == existing.depth && cand.member == existing.member:
		existing.sources = appendDistinct(existing.sources, cand.sources...)
	case cand.depth == existing.depth:
		existing.ambiguous = append(existing.ambiguous, cand.member)
	}
}

func appendDistinct(values []string, more ...string) []string {
	for _, value := range more {
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

// describeBinding names a skills entry for a diagnostic.
func describeBinding(binding resource.SkillBinding) string {
	if binding.Ref != "" {
		return binding.Ref
	}
	return "a requirement of " + *binding.Capability
}

// finalSkills returns the skills a composition binds, in slot order,
// failing when a required capability has no implementation. Any bound skill
// that implements a required capability fulfills it, including a skill
// instance, which occupies a slot of its own.
func (r *Resolver) finalSkills(c *composite, where string) ([]string, error) {
	skills := []string{}
	implemented := map[string]bool{}
	for _, slot := range resource.SortedKeys(c.bindings) {
		if member := c.bindings[slot].member; member != "" {
			skills = append(skills, member)
			implemented[r.docs[member].Binds] = true
		}
	}
	for _, slot := range resource.SortedKeys(c.bindings) {
		if cand := c.bindings[slot]; cand.member == "" && cand.required && !implemented[slot] {
			return nil, resource.Errorf("%s: required capability %s has no implementation; bind a skill that implements it", where, slot)
		}
	}
	return skills, nil
}

// checkContract enforces a profile's declared parameters: every member
// skill and held document, including its agents' documents, that declares a
// parameter of the same name declares the same context type.
func (r *Resolver) checkContract(profile *resource.Document, c *composite) error {
	check := func(memberID string, parameters map[string]string) error {
		for _, name := range resource.SortedKeys(parameters) {
			if declared, ok := c.declared[name]; ok && declared != parameters[name] {
				return resource.Errorf("%s: member %s declares parameter '%s' as %s, but the profile's contract declares %s", profile.Path, memberID, name, parameters[name], declared)
			}
		}
		return nil
	}
	for _, capability := range resource.SortedKeys(c.bindings) {
		if skill := c.bindings[capability].member; skill != "" {
			if err := check(skill, r.docs[skill].Parameters); err != nil {
				return err
			}
		}
	}
	documents := append([]string{}, c.documents...)
	for _, role := range resource.SortedKeys(c.agents) {
		for _, ref := range r.docs[c.agents[role].member].Context {
			documents = append(documents, ref.ID)
		}
	}
	for _, docID := range documents {
		if err := check(docID, r.docs[docID].Parameters); err != nil {
			return err
		}
	}
	return nil
}

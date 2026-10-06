package resolve

import (
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// candidate is the winning binding for one capability so far.
type candidate struct {
	skill     string
	required  bool
	depth     int
	source    string
	ambiguous []string
}

// composite is an agent's or profile's composition state.
type composite struct {
	id             string
	documents      []string
	documentSource map[string]string
	bindings       map[string]*candidate
	// declared are the parameters profiles declare as their contract.
	declared       map[string]string
	declaredSource map[string]string
	objectives     []Objective
	bindingSource  map[string]string
}

// compose resolves an agent or profile's embedding graph from embedded
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
		documentSource: map[string]string{},
		bindings:       map[string]*candidate{},
		declared:       map[string]string{},
		declaredSource: map[string]string{},
		bindingSource:  map[string]string{},
	}
	seenObjective := map[string]bool{}
	for _, embedID := range doc.Embeds {
		embedded := r.docs[embedID]
		if embedded.Kind != "profile" && embedded.Kind != "agent" {
			return nil, resource.Errorf("%s: embeds %s, which is a %s; embed profiles or agents", doc.Path, embedID, embedded.Kind)
		}
		if doc.Kind == "profile" && embedded.Kind == "agent" {
			return nil, resource.Errorf("%s: a profile cannot embed agent %s", doc.Path, embedID)
		}
		sub, err := r.compose(embedID, stack)
		if err != nil {
			return nil, err
		}
		for _, docID := range sub.documents {
			c.addDocument(docID, sub.documentSource[docID])
		}
		for _, capability := range resource.SortedKeys(sub.bindings) {
			promoted := *sub.bindings[capability]
			promoted.depth++
			promoted.ambiguous = nil
			c.merge(capability, &promoted)
		}
		for _, name := range resource.SortedKeys(sub.declared) {
			if err := c.declare(doc.Path, name, sub.declared[name], sub.declaredSource[name]); err != nil {
				return nil, err
			}
		}
		for _, objective := range sub.objectives {
			if !seenObjective[objective.Source] {
				seenObjective[objective.Source] = true
				c.objectives = append(c.objectives, objective)
			}
		}
	}
	for _, ref := range doc.Context {
		c.addDocument(ref.ID, id)
	}
	for _, name := range resource.SortedKeys(doc.Parameters) {
		if err := c.declare(doc.Path, name, doc.Parameters[name], id); err != nil {
			return nil, err
		}
	}
	local := map[string]bool{}
	for _, binding := range doc.Skills {
		capability := ""
		if binding.Ref != "" {
			capability = r.docs[binding.Ref].Binds
			if binding.Capability != nil && *binding.Capability != capability {
				return nil, resource.Errorf("%s: skill %s implements %s, not %s", doc.Path, binding.Ref, capability, *binding.Capability)
			}
		} else {
			capability = *binding.Capability
		}
		if local[capability] {
			return nil, resource.Errorf("%s: binds capability %s more than once", doc.Path, capability)
		}
		local[capability] = true
		required := binding.Required
		if existing, ok := c.bindings[capability]; ok {
			required = required || existing.required
		}
		c.bindings[capability] = &candidate{skill: binding.Ref, required: required, depth: 0, source: id}
	}
	for _, capability := range resource.SortedKeys(c.bindings) {
		cand := c.bindings[capability]
		if len(cand.ambiguous) > 0 {
			return nil, resource.Errorf("%s: capability %s is ambiguous between %s and %s at the same embedding depth; bind one locally", doc.Path, capability, cand.skill, strings.Join(cand.ambiguous, ", "))
		}
		if cand.skill != "" {
			c.bindingSource[cand.skill] = cand.source
		}
	}
	if doc.Kind == "agent" && strings.TrimSpace(doc.Objectives) != "" && !seenObjective[id] {
		c.objectives = append(c.objectives, Objective{Source: id, Content: doc.Objectives})
	}
	if doc.Kind == "profile" {
		if err := r.checkContract(doc, c); err != nil {
			return nil, err
		}
	}
	r.composites[id] = c
	return c, nil
}

func (c *composite) addDocument(id, source string) {
	if _, seen := c.documentSource[id]; seen {
		return
	}
	c.documentSource[id] = source
	c.documents = append(c.documents, id)
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

// merge promotes a binding from an embedded resource: the shallowest
// implementation wins; at one depth, different implementations are
// ambiguous unless a shallower binding resolves them.
func (c *composite) merge(capability string, cand *candidate) {
	existing, ok := c.bindings[capability]
	if !ok {
		c.bindings[capability] = cand
		return
	}
	required := existing.required || cand.required
	switch {
	case cand.depth < existing.depth:
		*existing = *cand
	case cand.depth == existing.depth:
		switch {
		case existing.skill == "":
			existing.skill, existing.source = cand.skill, cand.source
		case cand.skill != "" && cand.skill != existing.skill:
			existing.ambiguous = append(existing.ambiguous, cand.skill)
		}
	}
	existing.required = required
}

// finalSkills returns the skills a composition binds, in capability order,
// failing when a required capability has no implementation.
func (c *composite) finalSkills(where string) ([]string, error) {
	skills := []string{}
	for _, capability := range resource.SortedKeys(c.bindings) {
		cand := c.bindings[capability]
		if cand.skill == "" {
			if cand.required {
				return nil, resource.Errorf("%s: required capability %s has no implementation; bind a skill that implements it", where, capability)
			}
			continue
		}
		skills = append(skills, cand.skill)
	}
	return skills, nil
}

// checkContract enforces a profile's declared parameters: every member
// skill and held document that declares a parameter of the same name
// declares the same context type.
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
		if skill := c.bindings[capability].skill; skill != "" {
			if err := check(skill, r.docs[skill].Parameters); err != nil {
				return err
			}
		}
	}
	for _, docID := range c.documents {
		if err := check(docID, r.docs[docID].Parameters); err != nil {
			return err
		}
	}
	return nil
}

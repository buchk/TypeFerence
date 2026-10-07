package resolve

import (
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// ProvenanceEntry records which source document contributed a field.
type ProvenanceEntry struct {
	Field  string
	Source string
}

// Objective is one source's contribution to an agent's objectives.
type Objective struct {
	Source  string
	Content string
}

// ResolvedDocument is a held document with its field references resolved.
type ResolvedDocument struct {
	ID      string
	Title   string
	Content string
	// Render is "inline" or "file".
	Render string
}

// ResolvedSkill is a skill or skill instance as it ships.
type ResolvedSkill struct {
	// Name is the emitted skill name: the identity leaf, prefixed by the
	// instance name for an agent instance.
	Name             string
	ImplementationID string
	// TemplateID is the template an instance instantiates: the skill itself
	// for an agent instance, the nearest template ancestor for a skill
	// instance, and empty for a skill that binds no parameters.
	TemplateID   string
	CapabilityID string
	Description  string
	Instructions string
	// Variants maps mode to rendered instructions for a multimodal skill.
	Variants        map[string]string
	Servers         []string
	VariantServers  map[string][]string
	InputSchema     string
	OutputSchema    string
	HasInputSchema  bool
	HasOutputSchema bool
	Documents       []ResolvedDocument
	Files           []resource.PackageFile
	Copilot         resource.CopilotFields
	// Bindings are the parameters this instance binds, sorted by name.
	Bindings   []*Binding
	Provenance []ProvenanceEntry
}

// Key identifies a skill's emitted content: its implementation and the data
// its parameters are bound to.
func (s ResolvedSkill) Key() string {
	parts := []string{s.ImplementationID}
	for _, b := range s.Bindings {
		parts = append(parts, b.Name+"="+b.DataID)
	}
	return strings.Join(parts, "|")
}

// IsInstance reports whether the skill binds any parameters.
func (s ResolvedSkill) IsInstance() bool { return len(s.Bindings) > 0 }

// InstructionsFor returns the skill's instructions in one mode.
func (s ResolvedSkill) InstructionsFor(mode string) string {
	if len(s.Variants) > 0 {
		return s.Variants[mode]
	}
	return s.Instructions
}

// ServersFor returns the servers the skill requires in one mode.
func (s ResolvedSkill) ServersFor(mode string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, s.Servers...), s.VariantServers[mode]...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// ResolvedAgent is a concrete agent after composition.
type ResolvedAgent struct {
	ID          string
	DisplayName string
	Description string
	Embeds      []string
	Objectives  []Objective
	Documents   []ResolvedDocument
	// Skills are ordered by emitted name.
	Skills       []ResolvedSkill
	Bindings     []*Binding
	InstanceName string
	Copilot      resource.CopilotFields
	Provenance   []ProvenanceEntry
	// Native components the agent composes (ADR-0007): rules and commands
	// rendered with its bindings, hooks, and agent-scoped servers.
	Rules    []ResolvedRule
	Commands []ResolvedCommand
	Hooks    []string
	Servers  []string
}

// ResolvedProfile is a profile shipped without an agent.
type ResolvedProfile struct {
	ID       string
	Skills   []ResolvedSkill
	Rules    []ResolvedRule
	Commands []ResolvedCommand
	Hooks    []string
}

// Resolver resolves documents from one merged, normalized document set.
type Resolver struct {
	docs       map[string]*resource.Document
	data       map[string]map[string]resource.Value
	composites map[string]*composite
}

// New validates a merged, normalized document set and returns a resolver for
// it. Every data document is checked against its context type and every
// template's references against its parameters, whether or not anything
// instantiates them.
func New(docs map[string]*resource.Document) (*Resolver, error) {
	r := &Resolver{docs: docs, data: map[string]map[string]resource.Value{}, composites: map[string]*composite{}}
	for _, id := range resource.SortedKeys(docs) {
		if err := r.validate(docs[id]); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Resolver) kindOf(id string) string {
	if doc := r.docs[id]; doc != nil {
		return doc.Kind
	}
	return ""
}

func (r *Resolver) validate(doc *resource.Document) error {
	if err := r.validateNative(doc); err != nil {
		return err
	}
	for _, name := range resource.SortedKeys(doc.Parameters) {
		if r.kindOf(doc.Parameters[name]) != "contextType" {
			return resource.Errorf("%s: parameter '%s' names %s, which is not a context type in this build", doc.Path, name, doc.Parameters[name])
		}
	}
	for _, embed := range doc.Embeds {
		if r.docs[embed] == nil {
			return resource.Errorf("%s: embeds %s, which is not in this build", doc.Path, embed)
		}
	}
	for _, ref := range doc.Context {
		held := r.docs[ref.ID]
		if held == nil || held.Kind != "context" {
			return resource.Errorf("%s: holds %s, which is not a context document in this build", doc.Path, ref.ID)
		}
		if held.IsData() {
			return resource.Errorf("%s: holds data document %s; data reaches output only through field references, so bind it with 'with'", doc.Path, ref.ID)
		}
	}
	for _, binding := range doc.Skills {
		if binding.Ref != "" && r.kindOf(binding.Ref) != "skill" {
			return resource.Errorf("%s: binds %s, which is not a skill in this build", doc.Path, binding.Ref)
		}
		if binding.Capability != nil && r.kindOf(*binding.Capability) != "capability" && r.kindOf(*binding.Capability) != "skill" {
			return resource.Errorf("%s: names capability %s, which is not in this build", doc.Path, *binding.Capability)
		}
	}
	switch doc.Kind {
	case "context":
		if doc.IsData() {
			contextType := r.docs[doc.ContextType]
			if contextType == nil || contextType.Kind != "contextType" {
				return resource.Errorf("%s: contextType %s is not a context type in this build", doc.Path, doc.ContextType)
			}
			values, err := resource.DataValues(doc, contextType)
			if err != nil {
				return err
			}
			r.data[doc.ID] = values
			return nil
		}
		return r.checkReferences(doc.Content, doc.Path, doc.Parameters)
	case "skill":
		for _, id := range append(append([]string{}, doc.RequiresServers...), variantServers(doc)...) {
			if r.kindOf(id) != "server" {
				return resource.Errorf("%s: requires server %s, which is not in this build", doc.Path, id)
			}
		}
		for _, name := range resource.SortedKeys(doc.With) {
			if err := r.checkData(doc.Path, name, doc.With[name], doc.Parameters[name]); err != nil {
				return err
			}
		}
		for _, ref := range doc.Context {
			held := r.docs[ref.ID]
			for _, name := range resource.SortedKeys(held.Parameters) {
				if doc.Parameters[name] != held.Parameters[name] {
					return resource.Errorf("%s: holds document %s, whose parameter '%s' (%s) the skill does not declare with the same context type", doc.Path, ref.ID, name, held.Parameters[name])
				}
			}
		}
		if err := r.checkReferences(doc.Description, doc.Path+" description", doc.Parameters); err != nil {
			return err
		}
		if err := r.checkReferences(doc.Instructions, doc.Path, doc.Parameters); err != nil {
			return err
		}
		for _, mode := range resource.SortedKeys(doc.Variants) {
			if err := r.checkReferences(doc.Variants[mode].Instructions, doc.Path+" variant "+mode, doc.Parameters); err != nil {
				return err
			}
		}
	case "agent":
		for _, name := range resource.SortedKeys(doc.With) {
			if err := r.checkData(doc.Path, name, doc.With[name], ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func variantServers(doc *resource.Document) []string {
	out := []string{}
	for _, mode := range resource.SortedKeys(doc.Variants) {
		out = append(out, doc.Variants[mode].RequiresServers...)
	}
	return out
}

// checkData checks that a binding names a data document, of the expected
// context type when one is given.
func (r *Resolver) checkData(where, name, dataID, expectedType string) error {
	data := r.docs[dataID]
	if data == nil || !data.IsData() {
		return resource.Errorf("%s: 'with' binds '%s' to %s, which is not a data document in this build", where, name, dataID)
	}
	if expectedType != "" && data.ContextType != expectedType {
		return resource.Errorf("%s: 'with' binds '%s' to %s, whose context type %s is not the parameter's %s", where, name, dataID, data.ContextType, expectedType)
	}
	return nil
}

func (r *Resolver) binding(name, dataID string) *Binding {
	data := r.docs[dataID]
	return &Binding{Name: name, DataID: dataID, ContextType: data.ContextType, Values: r.data[dataID]}
}

// ResolveAll resolves every agent in the document set, ordered by identity.
func (r *Resolver) ResolveAll() ([]*ResolvedAgent, error) {
	agents := []*ResolvedAgent{}
	for _, id := range resource.SortedKeys(r.docs) {
		if r.docs[id].Kind != "agent" {
			continue
		}
		agent, err := r.ResolveAgent(id)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// ResolveAgent composes one agent: its promoted bindings and documents, its
// parameter bindings, and its skills and skill instances.
func (r *Resolver) ResolveAgent(id string) (*ResolvedAgent, error) {
	doc := r.docs[id]
	if doc == nil || doc.Kind != "agent" {
		return nil, resource.Errorf("%s is not an agent in this build", id)
	}
	c, err := r.compose(id, nil)
	if err != nil {
		return nil, err
	}
	skillIDs, err := c.finalSkills(doc.Path)
	if err != nil {
		return nil, err
	}

	// What the agent must bind: declared contracts, its template skills'
	// unbound parameters, and its held documents' parameters.
	need := map[string]string{}
	needSource := map[string]string{}
	addNeed := func(name, typeID, source string) error {
		if prior, exists := need[name]; exists && prior != typeID {
			return resource.Errorf("%s: parameter '%s' is %s for %s but %s for %s; one name has one context type", doc.Path, name, typeID, source, prior, needSource[name])
		}
		need[name], needSource[name] = typeID, source
		return nil
	}
	for _, name := range resource.SortedKeys(c.declared) {
		if err := addNeed(name, c.declared[name], c.declaredSource[name]); err != nil {
			return nil, err
		}
	}
	templateSkills := false
	for _, skillID := range skillIDs {
		skill := r.docs[skillID]
		for _, name := range resource.SortedKeys(skill.Parameters) {
			if _, bound := skill.With[name]; bound {
				continue
			}
			templateSkills = true
			if err := addNeed(name, skill.Parameters[name], skillID); err != nil {
				return nil, err
			}
		}
	}
	for _, docID := range c.documents {
		held := r.docs[docID]
		for _, name := range resource.SortedKeys(held.Parameters) {
			if err := addNeed(name, held.Parameters[name], docID); err != nil {
				return nil, err
			}
		}
	}
	for _, componentID := range append(append([]string{}, c.rules...), c.commands...) {
		component := r.docs[componentID]
		for _, name := range resource.SortedKeys(component.Parameters) {
			templateSkills = true
			if err := addNeed(name, component.Parameters[name], componentID); err != nil {
				return nil, err
			}
		}
	}
	// The agent's bindings are its own 'with' plus those of the agents it
	// embeds, shallowest first (composite.with).
	template := len(c.with) > 0
	referenced := map[string]bool{}
	if template {
		for _, objective := range c.objectives {
			for _, name := range referencedNames(objective.Content) {
				referenced[name] = true
			}
		}
		for _, name := range referencedNames(doc.Description) {
			referenced[name] = true
		}
	}
	bindings := map[string]*Binding{}
	for _, name := range resource.SortedKeys(c.with) {
		dataID := c.with[name].data
		typeID, needed := need[name]
		_, own := doc.With[name]
		if own && !needed && !referenced[name] {
			return nil, resource.Errorf("%s: 'with' binds '%s', but nothing the agent composes declares that parameter and its objectives do not reference it", doc.Path, name)
		}
		if err := r.checkData(doc.Path, name, dataID, typeID); err != nil {
			return nil, err
		}
		bindings[name] = r.binding(name, dataID)
	}
	for _, name := range resource.SortedKeys(need) {
		if _, bound := bindings[name]; !bound {
			return nil, resource.Errorf("%s: parameter '%s' (%s, from %s) is unbound; bind it with 'with'", doc.Path, name, need[name], needSource[name])
		}
	}
	instanceName := ""
	if templateSkills {
		suppliers := []string{}
		for _, name := range resource.SortedKeys(bindings) {
			b := bindings[name]
			contextType := r.docs[b.ContextType]
			if contextType.InstanceName != "" {
				suppliers = append(suppliers, name)
				instanceName = b.Values[contextType.InstanceName].Text
			}
		}
		if len(suppliers) != 1 {
			return nil, resource.Errorf("%s: the agent instantiates templates, so exactly one bound data document must supply an instance name (a context type with instanceName); found %d", doc.Path, len(suppliers))
		}
	}

	agent := &ResolvedAgent{
		ID:           id,
		DisplayName:  doc.DisplayName,
		Embeds:       append([]string{}, doc.Embeds...),
		InstanceName: instanceName,
		Copilot:      doc.Copilot,
	}
	agent.Description, err = r.render(doc.Description, doc.Path+" description", template, bindings)
	if err != nil {
		return nil, err
	}
	if len([]rune(agent.Description)) > 1024 {
		return nil, resource.Errorf("%s: description exceeds 1024 characters", doc.Path)
	}
	if !singleLine(agent.Description) {
		return nil, resource.Errorf("%s: description must stay a single line after field references are resolved", doc.Path)
	}
	for _, objective := range c.objectives {
		content, err := r.render(objective.Content, objective.Source, template, bindings)
		if err != nil {
			return nil, err
		}
		agent.Objectives = append(agent.Objectives, Objective{Source: objective.Source, Content: content})
		agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "objectives", Source: objective.Source})
	}
	for _, docID := range c.documents {
		resolved, err := r.resolveDocument(docID, "inline", bindings)
		if err != nil {
			return nil, err
		}
		agent.Documents = append(agent.Documents, resolved)
		for _, source := range c.documentSource[docID] {
			agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "context", Source: source})
		}
	}
	for _, embed := range doc.Embeds {
		agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "embeds", Source: embed})
	}
	for _, name := range resource.SortedKeys(bindings) {
		agent.Bindings = append(agent.Bindings, bindings[name])
		for _, source := range c.with[name].sources {
			agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "with", Source: source})
		}
	}
	for _, skillID := range skillIDs {
		skill, err := r.resolveSkill(skillID, bindings, instanceName, c.bindingSource[skillID])
		if err != nil {
			return nil, err
		}
		agent.Skills = append(agent.Skills, skill)
	}
	sort.Slice(agent.Skills, func(i, j int) bool { return agent.Skills[i].Name < agent.Skills[j].Name })
	for _, ruleID := range c.rules {
		rule, err := r.resolveRule(ruleID, bindings, instanceName)
		if err != nil {
			return nil, err
		}
		agent.Rules = append(agent.Rules, rule)
		for _, source := range c.nativeSource[ruleID] {
			agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "rules", Source: source})
		}
	}
	for _, commandID := range c.commands {
		command, err := r.resolveCommand(commandID, bindings, instanceName)
		if err != nil {
			return nil, err
		}
		agent.Commands = append(agent.Commands, command)
		for _, source := range c.nativeSource[commandID] {
			agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "commands", Source: source})
		}
	}
	for _, hookID := range c.hooks {
		agent.Hooks = append(agent.Hooks, hookID)
		for _, source := range c.nativeSource[hookID] {
			agent.Provenance = append(agent.Provenance, ProvenanceEntry{Field: "hooks", Source: source})
		}
	}
	agent.Servers = append([]string{}, doc.Servers...)
	for i := 1; i < len(agent.Skills); i++ {
		if agent.Skills[i].Name == agent.Skills[i-1].Name {
			return nil, resource.Errorf("%s: skills %s and %s both emit the skill name '%s'", doc.Path, agent.Skills[i-1].ImplementationID, agent.Skills[i].ImplementationID, agent.Skills[i].Name)
		}
	}
	return agent, nil
}

// ResolveSkill resolves a skill shipped directly, without an agent. A
// template cannot ship this way; only its instances can.
func (r *Resolver) ResolveSkill(id string) (ResolvedSkill, error) {
	skill := r.docs[id]
	if skill == nil || skill.Kind != "skill" {
		return ResolvedSkill{}, resource.Errorf("%s is not a skill in this build", id)
	}
	if skill.IsTemplate() {
		return ResolvedSkill{}, resource.Errorf("%s is a template (it has unbound parameters), so it ships only as an instance: through an agent's 'with', or a skill that extends it and supplies 'with'", skill.Path)
	}
	return r.resolveSkill(id, nil, "", nil)
}

// ResolveProfile resolves a profile shipped without an agent. It must be
// complete, hold no documents, and declare no parameters.
func (r *Resolver) ResolveProfile(id string) (*ResolvedProfile, error) {
	doc := r.docs[id]
	if doc == nil || doc.Kind != "profile" {
		return nil, resource.Errorf("%s is not a profile in this build", id)
	}
	c, err := r.compose(id, nil)
	if err != nil {
		return nil, err
	}
	if len(c.declared) > 0 {
		return nil, resource.Errorf("%s: a profile that declares parameters is instantiated by an agent; link an agent that binds them", doc.Path)
	}
	if len(c.documents) > 0 {
		return nil, resource.Errorf("%s: the profile holds document %s, which only an agent can deliver; link an agent that embeds the profile instead", doc.Path, c.documents[0])
	}
	skillIDs, err := c.finalSkills(doc.Path)
	if err != nil {
		return nil, err
	}
	profile := &ResolvedProfile{ID: id}
	for _, skillID := range skillIDs {
		skill, err := r.ResolveSkill(skillID)
		if err != nil {
			return nil, err
		}
		profile.Skills = append(profile.Skills, skill)
	}
	for _, ruleID := range c.rules {
		rule, err := r.ResolveRule(ruleID)
		if err != nil {
			return nil, err
		}
		profile.Rules = append(profile.Rules, rule)
	}
	for _, commandID := range c.commands {
		command, err := r.ResolveCommand(commandID)
		if err != nil {
			return nil, err
		}
		profile.Commands = append(profile.Commands, command)
	}
	profile.Hooks = append(profile.Hooks, c.hooks...)
	return profile, nil
}

// resolveSkill renders one skill. agentBindings supply the parameters the
// skill leaves unbound; a template is named after the instance.
func (r *Resolver) resolveSkill(id string, agentBindings map[string]*Binding, instanceName string, boundBy []string) (ResolvedSkill, error) {
	skill := r.docs[id]
	bindings := map[string]*Binding{}
	for _, name := range resource.SortedKeys(skill.Parameters) {
		if dataID, bound := skill.With[name]; bound {
			bindings[name] = r.binding(name, dataID)
			continue
		}
		b, ok := agentBindings[name]
		if !ok {
			return ResolvedSkill{}, resource.Errorf("%s: parameter '%s' is unbound", skill.Path, name)
		}
		bindings[name] = b
	}
	name := resource.Leaf(id)
	if skill.IsTemplate() {
		if instanceName == "" {
			return ResolvedSkill{}, resource.Errorf("%s: a template skill needs an instance name from its agent's data", skill.Path)
		}
		name = instanceName + "-" + name
	}
	template := len(skill.Parameters) > 0
	templateID := ""
	if skill.IsTemplate() {
		templateID = id
	} else if len(skill.OwnWith) > 0 {
		base := r.docs[skill.Extends]
		for base != nil && !base.IsTemplate() {
			base = r.docs[base.Extends]
		}
		if base != nil {
			templateID = base.ID
		}
	}
	resolved := ResolvedSkill{
		Name:             name,
		ImplementationID: id,
		TemplateID:       templateID,
		CapabilityID:     skill.Binds,
		Servers:          append([]string{}, skill.RequiresServers...),
		InputSchema:      skill.InputSchema,
		OutputSchema:     skill.OutputSchema,
		HasInputSchema:   skill.HasInputSchema,
		HasOutputSchema:  skill.HasOutputSchema,
		Files:            append([]resource.PackageFile{}, skill.Files...),
		Copilot:          skill.Copilot,
	}
	var err error
	resolved.Description, err = r.render(skill.Description, skill.Path+" description", template, bindings)
	if err != nil {
		return ResolvedSkill{}, err
	}
	if len([]rune(resolved.Description)) > 1024 {
		return ResolvedSkill{}, resource.Errorf("%s: description exceeds 1024 characters", skill.Path)
	}
	if !singleLine(resolved.Description) {
		return ResolvedSkill{}, resource.Errorf("%s: description must stay a single line after field references are resolved", skill.Path)
	}
	resolved.Instructions, err = r.render(skill.Instructions, skill.Path, template, bindings)
	if err != nil {
		return ResolvedSkill{}, err
	}
	if len(skill.Variants) > 0 {
		resolved.Variants = map[string]string{}
		resolved.VariantServers = map[string][]string{}
		for _, mode := range resource.SortedKeys(skill.Variants) {
			text, err := r.render(skill.Variants[mode].Instructions, skill.Path+" variant "+mode, template, bindings)
			if err != nil {
				return ResolvedSkill{}, err
			}
			resolved.Variants[mode] = text
			resolved.VariantServers[mode] = append([]string{}, skill.Variants[mode].RequiresServers...)
		}
	}
	for _, ref := range skill.Context {
		document, err := r.resolveDocument(ref.ID, ref.Render, bindings)
		if err != nil {
			return ResolvedSkill{}, err
		}
		resolved.Documents = append(resolved.Documents, document)
	}
	for _, name := range resource.SortedKeys(bindings) {
		resolved.Bindings = append(resolved.Bindings, bindings[name])
	}
	for chain := skill; chain != nil; chain = r.docs[chain.Extends] {
		resolved.Provenance = append(resolved.Provenance, ProvenanceEntry{Field: "implementation", Source: chain.ID})
		if chain.Extends == "" {
			break
		}
	}
	for _, source := range boundBy {
		resolved.Provenance = append(resolved.Provenance, ProvenanceEntry{Field: "binding", Source: source})
	}
	return resolved, nil
}

func (r *Resolver) resolveDocument(id, render string, bindings map[string]*Binding) (ResolvedDocument, error) {
	doc := r.docs[id]
	content, err := r.render(doc.Content, doc.Path, len(doc.Parameters) > 0, bindings)
	if err != nil {
		return ResolvedDocument{}, err
	}
	title := doc.DisplayName
	if strings.TrimSpace(title) == "" {
		title = resource.Leaf(id)
	}
	return ResolvedDocument{ID: id, Title: title, Content: content, Render: render}, nil
}

func singleLine(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return !strings.ContainsRune(value, ' ') && !strings.ContainsRune(value, ' ')
}

// Package resolve implements the TypeFerence structural type system:
// embedding composition, depth-based member promotion with compile-time
// ambiguity detection, capability contract enforcement, and implicit
// structural interface satisfaction (docs/specification.md).
package resolve

import (
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

type ProvenanceEntry struct {
	Field  string
	Source string
}

// ResolvedSkill is a concrete skill after promotion and contract checks.
type ResolvedSkill struct {
	DispatchName         string
	CapabilityID         string
	ImplementationID     string
	Description          string
	Instructions         string
	InputSchema          string
	OutputSchema         string
	ContextFiles         []string
	RequiresContextTypes []string
	RequiresTools        []string
	// Exposed is true when the bound capability's visibility is "exposed":
	// part of the agent's public callable surface, eligible for a callable
	// card (ADR-0015). Rides the skill, so promotion carries it automatically.
	Exposed bool
	// Sealed marks a binding an embedder may not override or rebind; Required
	// marks it mandatory (ADR-0016). Both ride the skill through promotion.
	Sealed   bool
	Required bool
	// Variants maps mode name to that mode's instructions for a multimodal
	// skill (ADR-0012); nil for a unimodal skill. Instructions above holds the
	// default (neutral) variant's rendering.
	Variants map[string]string
	// Variant requirements remain attached to their mode. Base requirements
	// above apply to every mode.
	VariantContextRequirements map[string][]string
	VariantToolRequirements    map[string][]string
	Provenance                 []ProvenanceEntry
}

// ResolvedAgent is a fully composed agent or profile.
type ResolvedAgent struct {
	ID                     string
	DisplayName            string
	Description            string
	Emit                   bool
	Embeds                 []string
	Satisfies              []string
	Slots                  map[string]string
	SlotKeys               []string // canonical order for Slots
	WorkingNorms           []string
	ContextFiles           []string
	Context                []string
	ContextObjects         []ResolvedContextRef
	AllowedContextTypes    []string
	HasAllowedContextTypes bool
	Skills                 []ResolvedSkill
	// RequiredCapabilities are the capability ids mandated by this component or
	// anything it embeds. A profile may carry ones it does not bind (an abstract
	// requirement); a resolved agent never can (ADR-0016).
	RequiredCapabilities []string
	Provenance           []ProvenanceEntry
}

// ResolvedContextRef is a context object an agent holds by id, with the
// contextType it instantiates and its materialized content (ADR-0013). Content
// lets a target inline the held context, not merely reference it.
type ResolvedContextRef struct {
	ID          string
	DisplayName string
	ContextType string
	Satisfies   []string
	Content     string
	Values      map[string]resource.FieldValue
	ValuesJSON  string
}

type interfaceContract struct {
	slots     []string
	slotTypes map[string]string
	skills    []string
}

// Resolver composes resources into resolved agents.
type Resolver struct {
	resources      map[string]*resource.Document
	componentCache map[string]*ResolvedAgent
	interfaceCache map[string]*interfaceContract
	slotDepths     map[string]map[string]int
	skillDepths    map[string]map[string]int
}

// New creates a Resolver over a loaded resource set.
func New(resources map[string]*resource.Document) *Resolver {
	return &Resolver{
		resources:      resources,
		componentCache: map[string]*ResolvedAgent{},
		interfaceCache: map[string]*interfaceContract{},
		slotDepths:     map[string]map[string]int{},
		skillDepths:    map[string]map[string]int{},
	}
}

// ResolveAll validates every skill, interface, and profile, then returns all
// agents resolved, sorted by id.
func (r *Resolver) ResolveAll() ([]*ResolvedAgent, error) {
	for _, id := range r.idsOfKind("skill") {
		if err := r.validateSkillImplementation(r.resources[id]); err != nil {
			return nil, err
		}
	}
	for _, id := range r.idsOfKind("interface") {
		if _, err := r.resolveInterface(id, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	for _, id := range r.idsOfKind("contextType") {
		if _, err := r.contextTypeClosure(id, map[string]bool{}); err != nil {
			return nil, err
		}
		if r.resources[id].SchemaVersion == 4 {
			if _, _, _, err := r.nativeContextShape(id); err != nil {
				return nil, err
			}
			if err := r.validateContextValueTypeGraph(id, map[string]bool{}); err != nil {
				return nil, err
			}
			if err := r.validateNativeDefaults(id); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range r.idsOfKind("context") {
		obj := r.resources[id]
		if _, err := r.contextTypeClosure(obj.ContextType, map[string]bool{}); err != nil {
			return nil, resource.Errorf("%s: %s", id, err)
		}
		if err := r.validateContextFields(obj); err != nil {
			return nil, err
		}
	}
	for _, id := range r.idsOfKind("tool") {
		if err := r.validateTool(r.resources[id]); err != nil {
			return nil, err
		}
	}
	for _, id := range r.idsOfKind("profile") {
		if _, err := r.resolveComponent(id, map[string]bool{}, false); err != nil {
			return nil, err
		}
	}
	agents := []*ResolvedAgent{}
	for _, id := range r.idsOfKind("agent") {
		agent, err := r.resolveComponent(id, map[string]bool{}, true)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// Resolve resolves a single agent by id.
func (r *Resolver) Resolve(id string) (*ResolvedAgent, error) {
	return r.resolveComponent(id, map[string]bool{}, true)
}

func (r *Resolver) idsOfKind(kind string) []string {
	ids := []string{}
	for id, doc := range r.resources {
		if doc.Kind == kind {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (r *Resolver) resolveComponent(id string, visiting map[string]bool, requireAgent bool) (*ResolvedAgent, error) {
	if requireAgent {
		if _, err := r.require(id, "agent"); err != nil {
			return nil, err
		}
	}
	if cached, ok := r.componentCache[id]; ok {
		return cached, nil
	}
	current, err := r.requireEmbeddable(id)
	if err != nil {
		return nil, err
	}
	if visiting[id] {
		return nil, resource.Errorf("Embedding cycle detected at %s", id)
	}
	visiting[id] = true
	defer delete(visiting, id)

	seen := map[string]bool{}
	for _, embed := range current.Embeds {
		if seen[embed] {
			return nil, resource.Errorf("%s: a %s cannot embed the same resource more than once", id, current.Kind)
		}
		seen[embed] = true
	}

	embedded := make([]*ResolvedAgent, 0, len(current.Embeds))
	for _, embedID := range current.Embeds {
		embeddedResource, embErr := r.requireEmbeddable(embedID)
		if embErr != nil {
			return nil, embErr
		}
		if current.Kind == "profile" && embeddedResource.Kind != "profile" {
			return nil, resource.Errorf("%s: profiles can only embed profiles", id)
		}
		component, resErr := r.resolveComponent(embedID, visiting, false)
		if resErr != nil {
			return nil, resErr
		}
		embedded = append(embedded, component)
	}

	slots, slotKeys, slotDepths, err := r.mergeSlots(id, current, embedded)
	if err != nil {
		return nil, err
	}
	norms := distinct(concatNorms(embedded, current))
	contexts := distinct(normalizeAll(concatContexts(embedded, current)))
	contextRefs := distinct(append(concatContextRefs(embedded, current), slotContextRefs(slots, slotKeys)...))
	allowedContextTypes := intersectAllowLists(embedded, current)
	skills, skillDepths, err := r.mergeSkills(id, current, embedded, contexts)
	if err != nil {
		return nil, err
	}
	requiredCapabilities, requiredBy, err := r.mergeRequired(id, current, embedded)
	if err != nil {
		return nil, err
	}
	// A mandated capability is mandatory wherever it is carried, however it came
	// to be bound, so the flag reflects the requirement rather than the binding
	// that happened to satisfy it (ADR-0016).
	for capabilityID := range requiredBy {
		if skill, bound := skills[capabilityID]; bound && !skill.Required {
			skill.Required = true
			skills[capabilityID] = skill
		}
	}
	if current.Kind == "agent" {
		if err := checkRequiredCapabilities(id, requiredBy, skills); err != nil {
			return nil, err
		}
		if err := r.checkSkillDependencies(id, skills, contextRefs); err != nil {
			return nil, err
		}
		if err := r.checkAllowedContext(id, contextRefs, allowedContextTypes, hasAllowedContextTypes(embedded, current)); err != nil {
			return nil, err
		}
	}

	satisfies := []string{}
	for _, interfaceID := range r.idsOfKind("interface") {
		contract, ifErr := r.resolveInterface(interfaceID, map[string]bool{})
		if ifErr != nil {
			return nil, ifErr
		}
		if r.satisfiesContract(contract, slots, skills) {
			satisfies = append(satisfies, interfaceID)
		}
	}

	provenance := []ProvenanceEntry{}
	for _, component := range embedded {
		for _, entry := range component.Provenance {
			if isPromotedProvenance(entry) {
				provenance = append(provenance, entry)
			}
		}
	}
	for _, embed := range current.Embeds {
		provenance = append(provenance, ProvenanceEntry{Field: "embeds." + embed, Source: id})
	}
	if !isBlank(current.DisplayName) {
		provenance = append(provenance, ProvenanceEntry{Field: "displayName", Source: id})
	}
	if !isBlank(current.Description) {
		provenance = append(provenance, ProvenanceEntry{Field: "description", Source: id})
	}
	for _, key := range resource.SortedKeys(current.Slots) {
		provenance = append(provenance, ProvenanceEntry{Field: "slots." + key, Source: id})
	}
	for range current.WorkingNorms {
		provenance = append(provenance, ProvenanceEntry{Field: "workingNorms", Source: id})
	}
	for range current.ContextFiles {
		provenance = append(provenance, ProvenanceEntry{Field: "contextFiles", Source: id})
	}
	for _, interfaceID := range satisfies {
		provenance = append(provenance, ProvenanceEntry{Field: "satisfies." + interfaceID, Source: id})
	}

	displayName := current.DisplayName
	if isBlank(displayName) {
		displayName = id
	}

	sortedSkills := make([]ResolvedSkill, 0, len(skills))
	dispatchOwners := map[string]string{}
	capabilityIDs := make([]string, 0, len(skills))
	for capabilityID := range skills {
		capabilityIDs = append(capabilityIDs, capabilityID)
	}
	sort.Strings(capabilityIDs)
	for _, capabilityID := range capabilityIDs {
		skill := withDispatch(skills[capabilityID], id)
		if prior, duplicate := dispatchOwners[skill.DispatchName]; duplicate {
			return nil, resource.Errorf("%s: capabilities %s and %s produce the same target dispatch name %s",
				id, prior, capabilityID, skill.DispatchName)
		}
		dispatchOwners[skill.DispatchName] = capabilityID
		sortedSkills = append(sortedSkills, skill)
	}

	resolved := &ResolvedAgent{
		ID:                     id,
		DisplayName:            displayName,
		Description:            current.Description,
		Emit:                   current.Emit,
		Embeds:                 append([]string{}, current.Embeds...),
		Satisfies:              satisfies,
		Slots:                  slots,
		SlotKeys:               slotKeys,
		WorkingNorms:           norms,
		ContextFiles:           contexts,
		Context:                contextRefs,
		ContextObjects:         r.resolveContextObjects(contextRefs),
		AllowedContextTypes:    allowedContextTypes,
		HasAllowedContextTypes: hasAllowedContextTypes(embedded, current),
		Skills:                 sortedSkills,
		RequiredCapabilities:   requiredCapabilities,
		Provenance:             provenance,
	}
	r.slotDepths[id] = slotDepths
	r.skillDepths[id] = skillDepths
	r.componentCache[id] = resolved
	return resolved, nil
}

func (r *Resolver) require(id, kind string) (*resource.Document, error) {
	doc, ok := r.resources[id]
	if !ok || doc.Kind != kind {
		return nil, resource.Errorf("Missing %s: %s", kind, id)
	}
	return doc, nil
}

func (r *Resolver) requireEmbeddable(id string) (*resource.Document, error) {
	doc, ok := r.resources[id]
	if !ok || (doc.Kind != "agent" && doc.Kind != "profile") {
		return nil, resource.Errorf("Missing embeddable resource: %s", id)
	}
	return doc, nil
}

func (s ResolvedSkill) InstructionsFor(mode string) string {
	if ins, ok := s.Variants[mode]; ok {
		return ins
	}
	return s.Instructions
}

// ExposedSkills returns the resolved skills whose capability is exposed, in
// dispatch order: the agent's public callable surface (ADR-0015). A callable
// card (ADR-0018) is emitted from exactly these, not from every skill.
func (a *ResolvedAgent) ExposedSkills() []ResolvedSkill {
	out := []ResolvedSkill{}
	for _, s := range a.Skills {
		if s.Exposed {
			out = append(out, s)
		}
	}
	return out
}

// Leaf extracts the unversioned name segment of a resource id
// (namespace/name@version -> name).
func Leaf(id string) string {
	parts := strings.Split(id, "/")
	last := parts[len(parts)-1]
	return strings.SplitN(last, "@", 2)[0]
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

func isPromotedProvenance(entry ProvenanceEntry) bool {
	if entry.Field == "displayName" || entry.Field == "description" {
		return false
	}
	return !strings.HasPrefix(entry.Field, "satisfies.")
}

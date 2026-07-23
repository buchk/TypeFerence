// Package resolve implements the TypeFerence structural type system:
// embedding composition, depth-based member promotion with compile-time
// ambiguity detection, capability contract enforcement, and implicit
// structural interface satisfaction (docs/specification.md).
package resolve

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// ProvenanceEntry records which resource contributed a resolved field.
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
	Variants   map[string]string
	Provenance []ProvenanceEntry
}

// ResolvedAgent is a fully composed agent or profile.
type ResolvedAgent struct {
	ID                  string
	DisplayName         string
	Description         string
	Emit                bool
	Embeds              []string
	Satisfies           []string
	Slots               map[string]string
	SlotKeys            []string // canonical order for Slots
	WorkingNorms        []string
	ContextFiles        []string
	Context             []string
	AllowedContextTypes []string
	Skills              []ResolvedSkill
	Provenance          []ProvenanceEntry
}

type interfaceContract struct {
	slots  []string
	skills []string
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
	contextRefs := distinct(concatContextRefs(embedded, current))
	allowedContextTypes := intersectAllowLists(embedded, current)
	skills, skillDepths, err := r.mergeSkills(id, current, embedded, contexts)
	if err != nil {
		return nil, err
	}
	if current.Kind == "agent" {
		if err := r.checkSkillDependencies(id, skills, contextRefs); err != nil {
			return nil, err
		}
		if err := r.checkAllowedContext(id, contextRefs, allowedContextTypes); err != nil {
			return nil, err
		}
	}

	satisfies := []string{}
	for _, interfaceID := range r.idsOfKind("interface") {
		contract, ifErr := r.resolveInterface(interfaceID, map[string]bool{})
		if ifErr != nil {
			return nil, ifErr
		}
		if satisfiesContract(contract, slots, skills) {
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
	capabilityIDs := make([]string, 0, len(skills))
	for capabilityID := range skills {
		capabilityIDs = append(capabilityIDs, capabilityID)
	}
	sort.Strings(capabilityIDs)
	for _, capabilityID := range capabilityIDs {
		sortedSkills = append(sortedSkills, withDispatch(skills[capabilityID], id))
	}

	resolved := &ResolvedAgent{
		ID:                  id,
		DisplayName:         displayName,
		Description:         current.Description,
		Emit:                current.Emit,
		Embeds:              append([]string{}, current.Embeds...),
		Satisfies:           satisfies,
		Slots:               slots,
		SlotKeys:            slotKeys,
		WorkingNorms:        norms,
		ContextFiles:        contexts,
		Context:             contextRefs,
		AllowedContextTypes: allowedContextTypes,
		Skills:              sortedSkills,
		Provenance:          provenance,
	}
	r.slotDepths[id] = slotDepths
	r.skillDepths[id] = skillDepths
	r.componentCache[id] = resolved
	return resolved, nil
}

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
			agents := make([]string, len(nearest))
			for i, c := range nearest {
				agents[i] = c.agent
			}
			return nil, nil, resource.Errorf(
				"%s: embedded capability '%s' is ambiguous between %s; bind the capability on %s to resolve the conflict",
				id, capabilityID, strings.Join(agents, ", "), id)
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
			CapabilityID:         capabilityID,
			ImplementationID:     implementation.ID,
			Description:          implementation.Description,
			Instructions:         instructions,
			Variants:             variants,
			InputSchema:          inputSchema,
			OutputSchema:         outputSchema,
			ContextFiles:         distinct(append(append([]string{}, contexts...), normalizeAll(implementation.ContextFiles)...)),
			RequiresContextTypes: aggregateContextRequirements(implementation),
			RequiresTools:        aggregateToolRequirements(implementation),
			Exposed:              capability.Visibility == "exposed",
			Sealed:               binding.Sealed,
			Required:             binding.Required,
			Provenance: []ProvenanceEntry{
				{Field: "skill.capability", Source: capabilityID},
				{Field: "skill.implementation", Source: implementation.ID},
			},
		}
		depths[capabilityID] = 0
	}
	return result, depths, nil
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
	skills := []string{}
	for _, embedID := range current.Embeds {
		embedded, embErr := r.resolveInterface(embedID, visiting)
		if embErr != nil {
			return nil, embErr
		}
		slots = append(slots, embedded.slots...)
		skills = append(skills, embedded.skills...)
	}
	for _, capability := range current.RequiresCapabilities {
		if _, capErr := r.require(capability, "capability"); capErr != nil {
			return nil, capErr
		}
	}
	contract := &interfaceContract{
		slots:  distinct(append(slots, current.RequiresSlots...)),
		skills: distinct(append(skills, current.RequiresCapabilities...)),
	}
	r.interfaceCache[id] = contract
	return contract, nil
}

func satisfiesContract(contract *interfaceContract, slots map[string]string, skills map[string]ResolvedSkill) bool {
	for _, slot := range contract.slots {
		if _, ok := slots[slot]; !ok {
			return false
		}
	}
	for _, skill := range contract.skills {
		if _, ok := skills[skill]; !ok {
			return false
		}
	}
	return true
}

// contextTypeClosure returns the set of contextType ids a context object of the
// given type satisfies: the type itself plus every type it transitively embeds
// (refinement). A governedX that embeds X satisfies both (ADR-0013).
func (r *Resolver) contextTypeClosure(id string, visiting map[string]bool) ([]string, error) {
	ct, ok := r.resources[id]
	if !ok || ct.Kind != "contextType" {
		return nil, resource.Errorf("Missing contextType: %s", id)
	}
	if visiting[id] {
		return nil, resource.Errorf("ContextType refinement cycle detected at %s", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	result := []string{id}
	for _, embedID := range ct.Embeds {
		base, err := r.contextTypeClosure(embedID, visiting)
		if err != nil {
			return nil, err
		}
		result = append(result, base...)
	}
	return distinct(result), nil
}

// providedContextTypes is the union of contextType closures over the context
// objects an agent holds by id.
func (r *Resolver) providedContextTypes(objectIDs []string) (map[string]bool, error) {
	provided := map[string]bool{}
	for _, objID := range objectIDs {
		obj, ok := r.resources[objID]
		if !ok || obj.Kind != "context" {
			return nil, resource.Errorf("Missing context: %s", objID)
		}
		closure, err := r.contextTypeClosure(obj.ContextType, map[string]bool{})
		if err != nil {
			return nil, err
		}
		for _, t := range closure {
			provided[t] = true
		}
	}
	return provided, nil
}

// validateContextFields checks a context object carries every field its
// contextType — and every type it refines — declares required (ADR-0013).
func (r *Resolver) validateContextFields(obj *resource.Document) error {
	closure, err := r.contextTypeClosure(obj.ContextType, map[string]bool{})
	if err != nil {
		return err
	}
	required := map[string]bool{}
	for _, ctID := range closure {
		for _, field := range requiredFields(r.resources[ctID].Schema) {
			required[field] = true
		}
	}
	names := make([]string, 0, len(required))
	for name := range required {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ok := obj.ContextFields[name]; !ok {
			return resource.Errorf("%s: missing required field %q declared by its contextType schema", obj.ID, name)
		}
	}
	return nil
}

// requiredFields extracts the top-level "required" array from a JSON Schema
// string. It only reads (never emits), so stdlib json is fine here.
func requiredFields(schema string) []string {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var doc struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal([]byte(schema), &doc); err != nil {
		return nil
	}
	return doc.Required
}

// validateTool checks a tool declaration's interface schemas parse (ADR-0017).
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

// aggregateContextRequirements unions a skill's context-type requirements with
// every variant's, since a multimodal skill emits all of its variants and the
// agent must satisfy each (ADR-0012 per-variant narrowing; ADR-0013).
func aggregateContextRequirements(impl *resource.Document) []string {
	reqs := append([]string{}, impl.RequiresContextTypes...)
	for _, mode := range sortedKeys(impl.Variants) {
		reqs = append(reqs, impl.Variants[mode].RequiresContextTypes...)
	}
	return distinct(reqs)
}

func aggregateToolRequirements(impl *resource.Document) []string {
	reqs := append([]string{}, impl.RequiresTools...)
	for _, mode := range sortedKeys(impl.Variants) {
		reqs = append(reqs, impl.Variants[mode].RequiresTools...)
	}
	return distinct(reqs)
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
func intersectAllowLists(embedded []*ResolvedAgent, current *resource.Document) []string {
	lists := [][]string{}
	for _, c := range embedded {
		if len(c.AllowedContextTypes) > 0 {
			lists = append(lists, c.AllowedContextTypes)
		}
	}
	if len(current.AllowedContextTypes) > 0 {
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

// checkAllowedContext verifies each held context object satisfies at least one
// allowed contextType (through its refinement closure). Unrestricted when the
// effective allow-list is empty (ADR-0013).
func (r *Resolver) checkAllowedContext(agentID string, contextRefs, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	allowSet := map[string]bool{}
	for _, a := range allowed {
		allowSet[a] = true
	}
	for _, objID := range contextRefs {
		obj, ok := r.resources[objID]
		if !ok || obj.Kind != "context" {
			continue // existence is checked in providedContextTypes
		}
		closure, err := r.contextTypeClosure(obj.ContextType, map[string]bool{})
		if err != nil {
			return err
		}
		permitted := false
		for _, t := range closure {
			if allowSet[t] {
				permitted = true
				break
			}
		}
		if !permitted {
			return resource.Errorf("%s: held context %s (type %s) is not among the allowed context types",
				agentID, objID, obj.ContextType)
		}
	}
	return nil
}

func concatContextRefs(embedded []*ResolvedAgent, current *resource.Document) []string {
	values := []string{}
	for _, component := range embedded {
		values = append(values, component.Context...)
	}
	return append(values, current.Context...)
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

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

func isPromotedProvenance(entry ProvenanceEntry) bool {
	if entry.Field == "displayName" || entry.Field == "description" {
		return false
	}
	return !strings.HasPrefix(entry.Field, "satisfies.")
}

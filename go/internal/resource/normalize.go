package resource

import (
	"strings"
)

// Normalize materializes what version 7 derives rather than asks authors to
// write, over a merged document set (root package plus locked dependencies):
//
//   - implied capabilities: a root skill that binds no capability defines one
//     identified by itself;
//   - capability schemas carried onto the skills that bind them;
//   - flattened extension chains: an extension carries its base's contract,
//     instructions, held documents, files, servers, parameters, bindings, and
//     Copilot fields (ADR-0031, ADR-0036);
//   - capability references that name a skill, rewritten to that skill's
//     capability;
//   - an agent or profile display name defaulting to its identity leaf.
func Normalize(docs map[string]*Document) error {
	ids := SortedKeys(docs)
	f := &flattener{docs: docs, visiting: map[string]bool{}}
	for _, id := range ids {
		if docs[id].Kind == "skill" {
			if err := f.flatten(id); err != nil {
				return err
			}
		}
	}
	for _, id := range ids {
		doc := docs[id]
		for i := range doc.Skills {
			if capability := doc.Skills[i].Capability; capability != nil {
				if skill, ok := docs[*capability]; ok && skill.Kind == "skill" {
					rewritten := skill.Binds
					doc.Skills[i].Capability = &rewritten
				}
			}
		}
		if (doc.Kind == "agent" || doc.Kind == "profile") && strings.TrimSpace(doc.DisplayName) == "" {
			doc.DisplayName = Leaf(doc.ID)
		}
	}
	return nil
}

type flattener struct {
	docs     map[string]*Document
	visiting map[string]bool
}

func (f *flattener) flatten(id string) error {
	doc := f.docs[id]
	if doc.Flattened {
		return nil
	}
	if f.visiting[id] {
		return Errorf("%s: skill extension cycle detected", doc.Path)
	}
	f.visiting[id] = true
	defer delete(f.visiting, id)

	if doc.Extends == "" {
		if doc.Binds == "" {
			doc.Binds = doc.ID
			doc.ImpliedCapability = true
		} else {
			capability, ok := f.docs[doc.Binds]
			if !ok || capability.Kind != "capability" {
				return Errorf("%s: binds %s, which is not a capability in this build", doc.Path, doc.Binds)
			}
			if doc.HasInputSchema || doc.HasOutputSchema {
				return Errorf("%s: a skill that binds a capability takes its schemas; declare them on %s instead", doc.Path, capability.Path)
			}
			doc.InputSchema, doc.HasInputSchema = capability.InputSchema, capability.HasInputSchema
			doc.OutputSchema, doc.HasOutputSchema = capability.OutputSchema, capability.HasOutputSchema
		}
		if err := checkFiles(doc); err != nil {
			return err
		}
		doc.Flattened = true
		return nil
	}

	base, ok := f.docs[doc.Extends]
	if !ok || base.Kind != "skill" {
		return Errorf("%s: extends %s, which is not a skill in this build", doc.Path, doc.Extends)
	}
	if err := f.flatten(base.ID); err != nil {
		return err
	}
	if doc.HasInputSchema || doc.HasOutputSchema {
		return Errorf("%s: an extension takes its base's schemas and cannot declare its own", doc.Path)
	}
	doc.Binds = base.Binds
	doc.ImpliedCapability = base.ImpliedCapability
	doc.InputSchema, doc.HasInputSchema = base.InputSchema, base.HasInputSchema
	doc.OutputSchema, doc.HasOutputSchema = base.OutputSchema, base.HasOutputSchema

	if err := mergeInstructions(doc, base); err != nil {
		return err
	}
	doc.RequiresServers = distinctStrings(append(append([]string{}, base.RequiresServers...), doc.RequiresServers...))
	doc.Context = mergeContext(base.Context, doc.Context)
	doc.Files = append(append([]PackageFile{}, base.Files...), doc.Files...)
	if err := checkFiles(doc); err != nil {
		return err
	}
	parameters := copyMap(base.Parameters)
	if parameters == nil {
		parameters = map[string]string{}
	}
	for _, name := range SortedKeys(doc.Parameters) {
		if prior, exists := parameters[name]; exists && prior != doc.Parameters[name] {
			return Errorf("%s: parameter '%s' is declared as %s here and as %s by base skill %s; one name has one context type", doc.Path, name, doc.Parameters[name], prior, base.ID)
		}
		parameters[name] = doc.Parameters[name]
	}
	doc.Parameters = parameters
	with := copyMap(base.With)
	if with == nil {
		with = map[string]string{}
	}
	for _, name := range SortedKeys(doc.With) {
		if _, exists := with[name]; exists {
			return Errorf("%s: parameter '%s' is already bound by base skill %s", doc.Path, name, base.ID)
		}
		with[name] = doc.With[name]
	}
	doc.With = with
	for _, name := range SortedKeys(doc.With) {
		if _, declared := doc.Parameters[name]; !declared {
			return Errorf("%s: 'with' binds '%s', which is not a parameter of %s", doc.Path, name, doc.Extends)
		}
	}
	if len(doc.OwnWith) > 0 {
		for _, name := range SortedKeys(doc.Parameters) {
			if _, bound := doc.With[name]; !bound {
				return Errorf("%s: 'with' must bind every parameter of its template; '%s' is unbound", doc.Path, name)
			}
		}
	}
	doc.Copilot = doc.Copilot.Overlay(base.Copilot)
	doc.Flattened = true
	return nil
}

func checkFiles(doc *Document) error {
	var paths SkillPaths
	for _, file := range doc.Files {
		if err := paths.Claim(file.As, file.Package+":"+file.Source); err != nil {
			return Errorf("%s: %s", doc.Path, err)
		}
	}
	return nil
}

func mergeContext(base, own []ContextRef) []ContextRef {
	out := append([]ContextRef{}, base...)
	seen := map[string]bool{}
	for _, ref := range base {
		seen[ref.ID] = true
	}
	for _, ref := range own {
		if !seen[ref.ID] {
			seen[ref.ID] = true
			out = append(out, ref)
		}
	}
	return out
}

// mergeInstructions applies the additive extension rule per mode: the base's
// instructions verbatim, then the extension's, separated by one blank line.
func mergeInstructions(doc, base *Document) error {
	baseUnimodal := len(base.Variants) == 0
	extUnimodal := len(doc.Variants) == 0
	if extUnimodal && strings.TrimSpace(doc.Instructions) == "" {
		doc.Instructions = base.Instructions
		doc.Variants = cloneVariants(base.Variants)
		return nil
	}
	switch {
	case baseUnimodal && extUnimodal:
		doc.Instructions = joinInstructions(base.Instructions, doc.Instructions)
	case baseUnimodal && !extUnimodal:
		merged := map[string]Variant{}
		for mode, v := range doc.Variants {
			v.Instructions = joinInstructions(base.Instructions, v.Instructions)
			merged[mode] = v
		}
		doc.Variants = merged
	case !baseUnimodal && extUnimodal:
		merged := map[string]Variant{}
		for mode, v := range base.Variants {
			merged[mode] = Variant{
				Instructions:    joinInstructions(v.Instructions, doc.Instructions),
				RequiresServers: append([]string{}, v.RequiresServers...),
			}
		}
		doc.Variants = merged
		doc.Instructions = ""
	default:
		merged := cloneVariants(base.Variants)
		for mode, v := range doc.Variants {
			baseVariant, ok := base.Variants[mode]
			if !ok {
				return Errorf("%s: variant '%s' does not exist on base skill %s; an extension can only add to its base's modes", doc.Path, mode, base.ID)
			}
			merged[mode] = Variant{
				Instructions:    joinInstructions(baseVariant.Instructions, v.Instructions),
				RequiresServers: distinctStrings(append(append([]string{}, baseVariant.RequiresServers...), v.RequiresServers...)),
			}
		}
		doc.Variants = merged
	}
	return nil
}

func joinInstructions(base, addition string) string {
	return strings.TrimRight(base, "\n") + "\n\n" + strings.TrimLeft(addition, "\n")
}

func cloneVariants(variants map[string]Variant) map[string]Variant {
	if variants == nil {
		return nil
	}
	out := make(map[string]Variant, len(variants))
	for mode, v := range variants {
		out[mode] = Variant{
			Instructions:    v.Instructions,
			RequiresServers: append([]string{}, v.RequiresServers...),
		}
	}
	return out
}

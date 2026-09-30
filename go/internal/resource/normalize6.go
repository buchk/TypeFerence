package resource

import (
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
)

// NormalizeV6 materializes the structure version 6 derives rather than asks
// authors to write, over a merged document set (root package plus locked
// dependencies):
//
//   - the built-in text context type, and it as the type of any context
//     document that declares none (ADR-0030);
//   - implied capabilities: a root skill that binds no capability defines one
//     identified by itself (ADR-0030);
//   - flattened extension chains: an extension carries its base's contract
//     and accumulated requirements, and its instructions are the base's
//     followed by its own (ADR-0031);
//   - capability references that name a skill, rewritten to that skill's
//     capability;
//   - an agent or profile display name defaulting to its identity leaf.
//
// Legacy documents (schemaVersion < 6) are left untouched.
func NormalizeV6(docs map[string]*Document) error {
	if _, exists := docs[BuiltinTextContextType]; !exists {
		docs[BuiltinTextContextType] = builtinTextType()
	}
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		doc := docs[id]
		if doc.SchemaVersion == 6 && doc.Kind == "context" && doc.ContextType == "" {
			doc.ContextType = BuiltinTextContextType
		}
	}
	f := &flattener{docs: docs, visiting: map[string]bool{}}
	for _, id := range ids {
		if doc := docs[id]; doc.Kind == "skill" && doc.SchemaVersion == 6 {
			if err := f.flatten(id); err != nil {
				return err
			}
		}
	}
	for _, id := range ids {
		doc := docs[id]
		if doc.SchemaVersion != 6 {
			continue
		}
		for i := range doc.Skills {
			if capability := doc.Skills[i].Capability; capability != nil {
				if skill, ok := docs[*capability]; ok && skill.Kind == "skill" {
					rewritten := skill.Binds
					doc.Skills[i].Capability = &rewritten
				}
			}
		}
		for i, capability := range doc.RequiresCapabilities {
			if skill, ok := docs[capability]; ok && skill.Kind == "skill" {
				doc.RequiresCapabilities[i] = skill.Binds
			}
		}
		if (doc.Kind == "agent" || doc.Kind == "profile") && strings.TrimSpace(doc.DisplayName) == "" {
			doc.DisplayName = Leaf(doc.ID)
		}
	}
	return nil
}

// Leaf extracts the unversioned last segment of a resource identity.
func Leaf(id string) string {
	last := id[strings.LastIndex(id, "/")+1:]
	return strings.SplitN(last, "@", 2)[0]
}

func builtinTextType() *Document {
	doc := NewDocument()
	doc.SchemaVersion = 6
	doc.Kind = "contextType"
	doc.ID = BuiltinTextContextType
	doc.DisplayName = "Text"
	doc.Package = ReservedPackagePrefix
	doc.ContextBody = &ContextBody{Type: "text", Required: true}
	return doc
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
		} else if capability, ok := f.docs[doc.Binds]; ok && capability.Kind == "capability" {
			if !doc.HasInputSchema {
				doc.InputSchema = capability.InputSchema
			}
			if !doc.HasOutputSchema {
				doc.OutputSchema = capability.OutputSchema
			}
		}
		doc.Flattened = true
		return nil
	}

	base, ok := f.docs[doc.Extends]
	if !ok || base.Kind != "skill" {
		return Errorf("%s: extends %s, which is not a skill in this build", doc.Path, doc.Extends)
	}
	if base.SchemaVersion != 6 {
		return Errorf("%s: extends %s, which is not a version 6 skill", doc.Path, doc.Extends)
	}
	if base.Sealed {
		return Errorf("%s: extends sealed skill %s; a sealed skill cannot be extended", doc.Path, base.ID)
	}
	if err := f.flatten(base.ID); err != nil {
		return err
	}

	doc.Binds = base.Binds
	doc.ImpliedCapability = base.ImpliedCapability
	if doc.HasInputSchema && !sameJSON(doc.InputSchema, base.InputSchema) {
		return Errorf("%s: an extension cannot change its base's inputSchema", doc.Path)
	}
	if doc.HasOutputSchema && !sameJSON(doc.OutputSchema, base.OutputSchema) {
		return Errorf("%s: an extension cannot change its base's outputSchema", doc.Path)
	}
	doc.InputSchema, doc.OutputSchema = base.InputSchema, base.OutputSchema
	doc.HasInputSchema, doc.HasOutputSchema = base.HasInputSchema, base.HasOutputSchema

	if err := mergeInstructions(doc, base); err != nil {
		return err
	}
	doc.RequiresTools = distinctStrings(append(append([]string{}, base.RequiresTools...), doc.RequiresTools...))
	doc.RequiresContextTypes = distinctStrings(append(append([]string{}, base.RequiresContextTypes...), doc.RequiresContextTypes...))
	doc.Context = distinctStrings(append(append([]string{}, base.Context...), doc.Context...))
	doc.Flattened = true
	return nil
}

// mergeInstructions applies ADR-0031's additive rule per mode: the base's
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
				Instructions:         joinInstructions(v.Instructions, doc.Instructions),
				RequiresContextTypes: append([]string{}, v.RequiresContextTypes...),
				RequiresTools:        append([]string{}, v.RequiresTools...),
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
				Instructions:         joinInstructions(baseVariant.Instructions, v.Instructions),
				RequiresContextTypes: distinctStrings(append(append([]string{}, baseVariant.RequiresContextTypes...), v.RequiresContextTypes...)),
				RequiresTools:        distinctStrings(append(append([]string{}, baseVariant.RequiresTools...), v.RequiresTools...)),
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
			Instructions:         v.Instructions,
			RequiresContextTypes: append([]string{}, v.RequiresContextTypes...),
			RequiresTools:        append([]string{}, v.RequiresTools...),
		}
	}
	return out
}

func sameJSON(a, b string) bool {
	left, err := jsonx.Parse(a)
	if err != nil {
		return false
	}
	right, err := jsonx.Parse(b)
	if err != nil {
		return false
	}
	return jsonx.Compact(left) == jsonx.Compact(right)
}

func distinctStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

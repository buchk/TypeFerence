package compile

import (
	"sort"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
)

// BundleJSON renders the canonical indented bundle JSON for a resolved
// agent. Member order is the canonical bundle field order from the spec.
func BundleJSON(agent *resolve.ResolvedAgent) string {
	return jsonx.Indented(bundleValue(agent))
}

func bundleJSON(agent *resolve.ResolvedAgent) string { return BundleJSON(agent) }

func bundleValue(agent *resolve.ResolvedAgent) jsonx.Value {
	slots := jsonx.Obj{}
	for _, key := range agent.SlotKeys {
		slots = append(slots, jsonx.Member{K: key, V: jsonx.Str(agent.Slots[key])})
	}
	skills := jsonx.Arr{}
	for _, skill := range agent.Skills {
		skills = append(skills, skillValue(skill))
	}
	obj := jsonx.Obj{
		{K: "id", V: jsonx.Str(agent.ID)},
		{K: "displayName", V: jsonx.Str(agent.DisplayName)},
		{K: "description", V: jsonx.Str(agent.Description)},
		{K: "emit", V: jsonx.Bool(agent.Emit)},
		{K: "embeds", V: stringArr(agent.Embeds)},
		{K: "satisfies", V: stringArr(agent.Satisfies)},
		{K: "slots", V: slots},
		{K: "workingNorms", V: stringArr(agent.WorkingNorms)},
	}
	if len(agent.ContextFiles) > 0 {
		obj = append(obj, jsonx.Member{K: "contextFiles", V: stringArr(agent.ContextFiles)})
	}
	// Typed context held by id. Absent when the agent holds none, so bundles
	// that predate reference-by-id context are unchanged (ADR-0013).
	if len(agent.ContextObjects) > 0 {
		context := jsonx.Arr{}
		for _, ref := range agent.ContextObjects {
			values, err := jsonx.Parse(ref.ValuesJSON)
			if err != nil {
				values = jsonx.Obj{}
			}
			item := jsonx.Obj{
				{K: "id", V: jsonx.Str(ref.ID)},
				{K: "contextType", V: jsonx.Str(ref.ContextType)},
				{K: "satisfies", V: stringArr(ref.Satisfies)},
			}
			if ref.ValuesJSON != "{}" {
				item = append(item, jsonx.Member{K: "values", V: values})
			}
			item = append(item, jsonx.Member{K: "content", V: jsonx.Str(ref.Content)})
			context = append(context, item)
		}
		obj = append(obj, jsonx.Member{K: "context", V: context})
	}
	obj = append(obj, jsonx.Member{K: "skills", V: skills})
	obj = append(obj, jsonx.Member{K: "provenance", V: provenanceValue(agent.Provenance)})
	return obj
}

func skillValue(skill resolve.ResolvedSkill) jsonx.Value {
	obj := jsonx.Obj{
		{K: "dispatchName", V: jsonx.Str(skill.DispatchName)},
		{K: "capabilityId", V: jsonx.Str(skill.CapabilityID)},
		{K: "implementationId", V: jsonx.Str(skill.ImplementationID)},
		{K: "description", V: jsonx.Str(skill.Description)},
		{K: "instructions", V: jsonx.Str(skill.Instructions)},
		{K: "inputSchema", V: jsonx.Str(skill.InputSchema)},
		{K: "outputSchema", V: jsonx.Str(skill.OutputSchema)},
	}
	if len(skill.ContextFiles) > 0 {
		obj = append(obj, jsonx.Member{K: "contextFiles", V: stringArr(skill.ContextFiles)})
	}
	// A multimodal skill also emits its per-mode renderings. This member is
	// absent for unimodal skills, so their bundle output is unchanged (ADR-0012).
	if len(skill.Variants) > 0 {
		modes := make([]string, 0, len(skill.Variants))
		for mode := range skill.Variants {
			modes = append(modes, mode)
		}
		sort.Strings(modes)
		variants := jsonx.Obj{}
		for _, mode := range modes {
			variant := jsonx.Obj{
				{K: "instructions", V: jsonx.Str(skill.Variants[mode])},
				{K: "requiresContextTypes", V: stringArr(skill.VariantContextRequirements[mode])},
				{K: "requiresTools", V: stringArr(skill.VariantToolRequirements[mode])},
			}
			variants = append(variants, jsonx.Member{K: mode, V: variant})
		}
		obj = append(obj, jsonx.Member{K: "variants", V: variants})
	}
	if len(skill.RequiresContextTypes) > 0 {
		obj = append(obj, jsonx.Member{K: "requiresContextTypes", V: stringArr(skill.RequiresContextTypes)})
	}
	if len(skill.RequiresTools) > 0 {
		obj = append(obj, jsonx.Member{K: "requiresTools", V: stringArr(skill.RequiresTools)})
	}
	if skill.Exposed {
		obj = append(obj, jsonx.Member{K: "exposed", V: jsonx.Bool(true)})
	}
	// Modifier state is emitted only when a lock is engaged, so permissive
	// bindings are unchanged. Absent means the axis is at its default — open,
	// optional — which is what lets diff flag open -> sealed (ADR-0016).
	if skill.Sealed {
		obj = append(obj, jsonx.Member{K: "sealed", V: jsonx.Bool(true)})
	}
	if skill.Required {
		obj = append(obj, jsonx.Member{K: "required", V: jsonx.Bool(true)})
	}
	obj = append(obj, jsonx.Member{K: "provenance", V: provenanceValue(skill.Provenance)})
	return obj
}

// provenanceJSON renders the canonical indented provenance.json.
func provenanceJSON(entries []resolve.ProvenanceEntry) string {
	return jsonx.Indented(provenanceValue(entries))
}

func provenanceValue(entries []resolve.ProvenanceEntry) jsonx.Value {
	arr := jsonx.Arr{}
	for _, entry := range entries {
		arr = append(arr, jsonx.Obj{
			{K: "field", V: jsonx.Str(entry.Field)},
			{K: "source", V: jsonx.Str(entry.Source)},
		})
	}
	return arr
}

func stringArr(values []string) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, v := range values {
		arr = append(arr, jsonx.Str(v))
	}
	return arr
}

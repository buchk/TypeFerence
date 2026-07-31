// Context-type refinement, validation, materialization, and allow-listing.
package resolve

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

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

// validateContextFields checks a context object against its contextType schema
// (and every type it refines): required fields are present, and each declared
// field's structural type matches (ADR-0013).
func (r *Resolver) validateContextFields(obj *resource.Document) error {
	if obj.SchemaVersion == 4 {
		return r.validateNativeContext(obj)
	}
	closure, err := r.contextTypeClosure(obj.ContextType, map[string]bool{})
	if err != nil {
		return err
	}
	required := map[string]bool{}
	propTypes := map[string]string{}
	for _, ctID := range closure {
		req, types, err := parseContextSchema(r.resources[ctID].Schema)
		if err != nil {
			return resource.Errorf("%s: contextType %s has an invalid schema: %s", obj.ID, ctID, err)
		}
		for _, field := range req {
			required[field] = true
		}
		for field, t := range types {
			propTypes[field] = t
		}
	}
	for _, name := range sortedStringSet(required) {
		if _, ok := obj.ContextFields[name]; !ok {
			return resource.Errorf("%s: missing required field %q declared by its contextType schema", obj.ID, name)
		}
	}
	for _, name := range sortedStringMapKeys(propTypes) {
		field, ok := obj.ContextFields[name]
		if !ok {
			continue // absent optional field; presence is handled above
		}
		if !kindMatchesType(field.Kind, propTypes[name]) {
			return resource.Errorf("%s: field %q must be %s per its contextType schema, got a %s",
				obj.ID, name, propTypes[name], field.Kind)
		}
	}
	return nil
}

// parseContextSchema reads a JSON Schema's top-level "required" names and
// property types. A malformed "required" or "properties" is an error rather
// than a silently-empty result.
func parseContextSchema(schema string) ([]string, map[string]string, error) {
	if strings.TrimSpace(schema) == "" {
		return nil, nil, nil
	}
	var doc struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schema), &doc); err != nil {
		return nil, nil, resource.Errorf("\"required\" must be an array of field names and \"properties\" a type map")
	}
	types := map[string]string{}
	for name, prop := range doc.Properties {
		if prop.Type != "" {
			types[name] = prop.Type
		}
	}
	return doc.Required, types, nil
}

// kindMatchesType reports whether a field's structural kind satisfies a JSON
// Schema type. Unknown/absent types are not enforced.
func kindMatchesType(kind, schemaType string) bool {
	switch schemaType {
	case "array":
		return kind == "sequence"
	case "object":
		return kind == "mapping"
	case "string", "number", "integer", "boolean":
		return kind == "scalar"
	default:
		return true
	}
}

func sortedStringSet(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// validateTool checks a tool declaration's interface schemas parse (ADR-0017).
func (r *Resolver) validateNativeContext(obj *resource.Document) error {
	fields, bodyAllowed, bodyRequired, err := r.nativeContextShape(obj.ContextType)
	if err != nil {
		return resource.Errorf("%s: %s", obj.ID, err)
	}
	if obj.ContextFields == nil {
		obj.ContextFields = map[string]resource.FieldValue{}
	}
	for name := range obj.ContextFields {
		if _, declared := fields[name]; !declared {
			return resource.Errorf("%s: unknown context field %q", obj.ID, name)
		}
	}
	for _, name := range sortedContextFieldKeys(fields) {
		field := fields[name]
		value, present := obj.ContextFields[name]
		if !present && field.HasDefault {
			value = cloneFieldValue(field.Default)
			obj.ContextFields[name] = value
			present = true
		}
		if !present {
			if field.Required {
				return resource.Errorf("%s: missing required context field %q", obj.ID, name)
			}
			continue
		}
		materialized, err := r.materializeNativeValue(value, field.Type, name)
		if err != nil {
			return resource.Errorf("%s: %s", obj.ID, err)
		}
		obj.ContextFields[name] = materialized
	}
	if bodyRequired && strings.TrimSpace(obj.Content) == "" {
		return resource.Errorf("%s: its contextType requires a text body", obj.ID)
	}
	if !bodyAllowed && strings.TrimSpace(obj.Content) != "" {
		return resource.Errorf("%s: its contextType does not declare a text body", obj.ID)
	}
	return nil
}

func (r *Resolver) nativeContextShape(id string) (map[string]resource.ContextField, bool, bool, error) {
	return r.nativeContextShapeFrom(id, map[string]bool{})
}

type contextFieldConflict struct {
	required bool
	defaults bool
}

func (r *Resolver) nativeContextShapeFrom(id string, visiting map[string]bool) (map[string]resource.ContextField, bool, bool, error) {
	ct, ok := r.resources[id]
	if !ok || ct.Kind != "contextType" {
		return nil, false, false, resource.Errorf("Missing contextType: %s", id)
	}
	if visiting[id] {
		return nil, false, false, resource.Errorf("ContextType refinement cycle detected at %s", id)
	}
	if ct.SchemaVersion != 4 {
		return nil, false, false, resource.Errorf("native contextType %s cannot use legacy schemaVersion %d type %s", id, ct.SchemaVersion, ct.ID)
	}
	visiting[id] = true
	defer delete(visiting, id)

	fields := map[string]resource.ContextField{}
	bodyAllowed := false
	bodyRequired := false
	conflicts := map[string]contextFieldConflict{}
	for _, baseID := range ct.Embeds {
		baseFields, baseBodyAllowed, baseBodyRequired, err := r.nativeContextShapeFrom(baseID, visiting)
		if err != nil {
			return nil, false, false, err
		}
		if baseBodyAllowed {
			bodyAllowed = true
		}
		if baseBodyRequired {
			bodyRequired = true
		}
		for _, name := range sortedContextFieldKeys(baseFields) {
			next := baseFields[name]
			if prior, exists := fields[name]; exists {
				if !sameTypeExpr(prior.Type, next.Type) {
					return nil, false, false, resource.Errorf("contextType %s inherits incompatible types for field %q", ct.ID, name)
				}
				conflict := conflicts[name]
				if prior.Required != next.Required {
					conflict.required = true
				}
				if prior.HasDefault != next.HasDefault ||
					(prior.HasDefault && !reflect.DeepEqual(prior.Default, next.Default)) {
					conflict.defaults = true
				}
				if conflict.required || conflict.defaults {
					conflicts[name] = conflict
				} else {
					delete(conflicts, name)
				}
				next.Required = prior.Required || next.Required
				if prior.HasDefault {
					next.HasDefault = true
					next.Default = cloneFieldValue(prior.Default)
				}
			}
			fields[name] = next
		}
	}
	if ct.ContextBody != nil {
		bodyAllowed = true
		if ct.ContextBody.Required {
			bodyRequired = true
		}
	}
	for _, name := range sortedContextFieldKeys(ct.ContextTypeFields) {
		next := ct.ContextTypeFields[name]
		if prior, exists := fields[name]; exists {
			if !sameTypeExpr(prior.Type, next.Type) {
				return nil, false, false, resource.Errorf("contextType %s changes inherited field %q's type", ct.ID, name)
			}
			if prior.Required && !next.Required {
				return nil, false, false, resource.Errorf("contextType %s weakens inherited required field %q", ct.ID, name)
			}
			if conflict := conflicts[name]; conflict.defaults && !next.HasDefault {
				return nil, false, false, resource.Errorf("contextType %s must resolve inherited default ambiguity for field %q", ct.ID, name)
			}
			if prior.HasDefault && !next.HasDefault {
				next.HasDefault = true
				next.Default = cloneFieldValue(prior.Default)
			}
		}
		fields[name] = next
		delete(conflicts, name)
	}
	if len(conflicts) > 0 {
		names := make([]string, 0, len(conflicts))
		for name := range conflicts {
			names = append(names, name)
		}
		sort.Strings(names)
		return nil, false, false, resource.Errorf("contextType %s inherits ambiguous field %q; redeclare it locally", ct.ID, names[0])
	}
	return fields, bodyAllowed, bodyRequired, nil
}

func (r *Resolver) validateNativeValue(value resource.FieldValue, typ resource.TypeExpr, field string) error {
	_, err := r.materializeNativeValue(value, typ, field)
	return err
}

func (r *Resolver) materializeNativeValue(value resource.FieldValue, typ resource.TypeExpr, field string) (resource.FieldValue, error) {
	switch typ.Kind {
	case "string", "text":
		if value.Kind != "scalar" {
			return value, resource.Errorf("field %q must be %s, got %s", field, typ.Kind, value.Kind)
		}
	case "boolean":
		if value.Kind != "scalar" || (value.Scalar != "true" && value.Scalar != "false") {
			return value, resource.Errorf("field %q must be boolean true or false", field)
		}
	case "integer":
		if value.Kind != "scalar" {
			return value, resource.Errorf("field %q must be an integer", field)
		}
		if _, err := strconv.ParseInt(value.Scalar, 10, 64); err != nil {
			return value, resource.Errorf("field %q must be an integer", field)
		}
		if parsed, err := jsonx.Parse(value.Scalar); err != nil {
			return value, resource.Errorf("field %q must use a canonical JSON integer token", field)
		} else if _, ok := parsed.(jsonx.Num); !ok || strings.ContainsAny(value.Scalar, ".eE") {
			return value, resource.Errorf("field %q must be an integer", field)
		}
	case "number":
		if value.Kind != "scalar" {
			return value, resource.Errorf("field %q must be a number", field)
		}
		if _, err := strconv.ParseFloat(value.Scalar, 64); err != nil {
			return value, resource.Errorf("field %q must be a number", field)
		}
		if parsed, err := jsonx.Parse(value.Scalar); err != nil {
			return value, resource.Errorf("field %q must use a canonical JSON number token", field)
		} else if _, ok := parsed.(jsonx.Num); !ok {
			return value, resource.Errorf("field %q must be a number", field)
		}
	case "list":
		if value.Kind != "list" {
			return value, resource.Errorf("field %q must be a list", field)
		}
		for i, item := range value.List {
			materialized, err := r.materializeNativeValue(item, *typ.Elem, field+"["+strconv.Itoa(i)+"]")
			if err != nil {
				return value, err
			}
			value.List[i] = materialized
		}
	case "map":
		if value.Kind != "map" {
			return value, resource.Errorf("field %q must be a map", field)
		}
		for _, key := range sortedFieldValueKeys(value.Map) {
			materialized, err := r.materializeNativeValue(value.Map[key], *typ.Elem, field+"."+key)
			if err != nil {
				return value, err
			}
			value.Map[key] = materialized
		}
	case "ref":
		if value.Kind != "map" {
			return value, resource.Errorf("field %q must be a value of named context type %s", field, typ.Ref)
		}
		fields, _, bodyRequired, err := r.nativeContextShape(typ.Ref)
		if err != nil {
			return value, err
		}
		if bodyRequired {
			return value, resource.Errorf("field %q cannot embed named context type %s because it requires a text body", field, typ.Ref)
		}
		for key := range value.Map {
			if _, ok := fields[key]; !ok {
				return value, resource.Errorf("field %q has unknown member %q for %s", field, key, typ.Ref)
			}
		}
		for _, key := range sortedContextFieldKeys(fields) {
			member, present := value.Map[key]
			if !present && fields[key].HasDefault {
				member = cloneFieldValue(fields[key].Default)
				present = true
			}
			if !present {
				if fields[key].Required {
					return value, resource.Errorf("field %q is missing required member %q", field, key)
				}
				continue
			}
			materialized, err := r.materializeNativeValue(member, fields[key].Type, field+"."+key)
			if err != nil {
				return value, err
			}
			value.Map[key] = materialized
		}
	default:
		return value, resource.Errorf("field %q uses unsupported type %q", field, typ.Kind)
	}
	return value, nil
}

func (r *Resolver) validateNativeDefaults(id string) error {
	fields, _, _, err := r.nativeContextShape(id)
	if err != nil {
		return err
	}
	for _, name := range sortedContextFieldKeys(fields) {
		field := fields[name]
		if !field.HasDefault {
			continue
		}
		if _, err := r.materializeNativeValue(cloneFieldValue(field.Default), field.Type, name+" default"); err != nil {
			return resource.Errorf("contextType %s: %s", id, err)
		}
	}
	return nil
}

func (r *Resolver) validateContextValueTypeGraph(id string, visiting map[string]bool) error {
	if visiting[id] {
		return resource.Errorf("named context value type cycle detected at %s", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	fields, _, _, err := r.nativeContextShape(id)
	if err != nil {
		return err
	}
	for _, name := range sortedContextFieldKeys(fields) {
		for _, ref := range typeExprRefs(fields[name].Type) {
			if err := r.validateContextValueTypeGraph(ref, visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

func typeExprRefs(typ resource.TypeExpr) []string {
	if typ.Kind == "ref" {
		return []string{typ.Ref}
	}
	if typ.Elem != nil {
		return typeExprRefs(*typ.Elem)
	}
	return nil
}

func sameTypeExpr(a, b resource.TypeExpr) bool {
	if a.Kind != b.Kind || a.Ref != b.Ref || (a.Elem == nil) != (b.Elem == nil) {
		return false
	}
	return a.Elem == nil || sameTypeExpr(*a.Elem, *b.Elem)
}

func sortedContextFieldKeys(values map[string]resource.ContextField) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFieldValueKeys(values map[string]resource.FieldValue) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r *Resolver) checkAllowedContext(agentID string, contextRefs, allowed []string, constrained bool) error {
	if !constrained {
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

// resolveContextObjects turns held context ids into (id, contextType) refs,
// sorted by id for deterministic emission. Ids that do not resolve to a context
// object are skipped (agent-level checks surface missing context elsewhere).
func (r *Resolver) resolveContextObjects(contextRefs []string) []ResolvedContextRef {
	refs := []ResolvedContextRef{}
	for _, id := range contextRefs {
		if obj, ok := r.resources[id]; ok && obj.Kind == "context" {
			satisfies, _ := r.contextTypeClosure(obj.ContextType, map[string]bool{})
			refs = append(refs, ResolvedContextRef{
				ID:          id,
				DisplayName: obj.DisplayName,
				ContextType: obj.ContextType,
				Satisfies:   satisfies,
				Content:     obj.Content,
				Values:      cloneFieldValues(obj.ContextFields),
				ValuesJSON:  r.contextValuesJSON(obj),
			})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs
}

func (r *Resolver) contextValuesJSON(obj *resource.Document) string {
	if obj.SchemaVersion != 4 {
		return "{}"
	}
	fields, _, _, err := r.nativeContextShape(obj.ContextType)
	if err != nil {
		return "{}"
	}
	out := jsonx.Obj{}
	for _, name := range sortedContextFieldKeys(fields) {
		value, present := obj.ContextFields[name]
		if !present {
			continue
		}
		out = append(out, jsonx.Member{K: name, V: r.nativeValueJSON(value, fields[name].Type)})
	}
	return jsonx.Compact(out)
}

func (r *Resolver) nativeValueJSON(value resource.FieldValue, typ resource.TypeExpr) jsonx.Value {
	switch typ.Kind {
	case "boolean":
		return jsonx.Bool(value.Scalar == "true")
	case "integer", "number":
		parsed, err := jsonx.Parse(value.Scalar)
		if err == nil {
			return parsed
		}
		return jsonx.Str(value.Scalar)
	case "list":
		arr := jsonx.Arr{}
		for _, item := range value.List {
			arr = append(arr, r.nativeValueJSON(item, *typ.Elem))
		}
		return arr
	case "map":
		obj := jsonx.Obj{}
		for _, key := range sortedFieldValueKeys(value.Map) {
			obj = append(obj, jsonx.Member{K: key, V: r.nativeValueJSON(value.Map[key], *typ.Elem)})
		}
		return obj
	case "ref":
		fields, _, _, err := r.nativeContextShape(typ.Ref)
		if err != nil {
			return jsonx.Obj{}
		}
		obj := jsonx.Obj{}
		for _, key := range sortedFieldValueKeys(value.Map) {
			obj = append(obj, jsonx.Member{K: key, V: r.nativeValueJSON(value.Map[key], fields[key].Type)})
		}
		return obj
	default:
		return jsonx.Str(value.Scalar)
	}
}

func cloneFieldValues(values map[string]resource.FieldValue) map[string]resource.FieldValue {
	if values == nil {
		return nil
	}
	out := make(map[string]resource.FieldValue, len(values))
	for key, value := range values {
		out[key] = cloneFieldValue(value)
	}
	return out
}

func cloneFieldValue(value resource.FieldValue) resource.FieldValue {
	cloned := resource.FieldValue{Kind: value.Kind, Scalar: value.Scalar}
	if value.List != nil {
		cloned.List = make([]resource.FieldValue, len(value.List))
		for i, item := range value.List {
			cloned.List[i] = cloneFieldValue(item)
		}
	}
	if value.Map != nil {
		cloned.Map = cloneFieldValues(value.Map)
	}
	return cloned
}

func concatContextRefs(embedded []*ResolvedAgent, current *resource.Document) []string {
	values := []string{}
	for _, component := range embedded {
		values = append(values, component.Context...)
	}
	return append(values, current.Context...)
}

func slotContextRefs(slots map[string]string, keys []string) []string {
	refs := []string{}
	for _, key := range keys {
		if resource.IsResourceID(slots[key]) {
			refs = append(refs, slots[key])
		}
	}
	return refs
}

package resource

import (
	"regexp"
	"strconv"
)

var integerToken = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)$`)

// Value is a typed data value: a field value interpreted against its declared
// field type.
type Value struct {
	// Type is the declared field type.
	Type string
	// Text holds string and text values verbatim, integer lexemes as
	// written, and booleans as true or false.
	Text string
	// List holds list<string> values.
	List []string
}

// Check interprets an authored value against the field's declared type
// (schema-directed typing) and enforces choices.
func (f ContextField) Check(v FieldValue) (Value, error) {
	switch f.Type {
	case "string", "text":
		if v.Kind != "scalar" {
			return Value{}, Errorf("must be a %s", f.Type)
		}
		if len(f.Choices) > 0 {
			allowed := false
			for _, choice := range f.Choices {
				if choice == v.Scalar {
					allowed = true
				}
			}
			if !allowed {
				return Value{}, Errorf("'%s' is not one of the choices %v", v.Scalar, f.Choices)
			}
		}
		return Value{Type: f.Type, Text: v.Scalar}, nil
	case "boolean":
		if v.Kind != "scalar" || v.Quoted || (v.Scalar != "true" && v.Scalar != "false") {
			return Value{}, Errorf("must be the unquoted boolean true or false")
		}
		return Value{Type: f.Type, Text: v.Scalar}, nil
	case "integer":
		if v.Kind != "scalar" || v.Quoted || !integerToken.MatchString(v.Scalar) {
			return Value{}, Errorf("must be an unquoted integer")
		}
		if _, err := strconv.ParseInt(v.Scalar, 10, 64); err != nil {
			return Value{}, Errorf("is outside the signed 64-bit integer range")
		}
		return Value{Type: f.Type, Text: v.Scalar}, nil
	case "list<string>":
		if v.Kind == "null" {
			return Value{Type: f.Type, List: []string{}}, nil
		}
		if v.Kind != "list" {
			return Value{}, Errorf("must be a list of strings")
		}
		items := make([]string, 0, len(v.List))
		for _, item := range v.List {
			if item.Kind != "scalar" {
				return Value{}, Errorf("must be a list of strings")
			}
			items = append(items, item.Scalar)
		}
		return Value{Type: f.Type, List: items}, nil
	}
	return Value{}, Errorf("has unknown type '%s'", f.Type)
}

// DataValues checks a data document's values against its context type and
// returns every field's value with defaults materialized. Optional fields
// with neither a value nor a default are absent from the result.
func DataValues(data, contextType *Document) (map[string]Value, error) {
	declared := map[string]ContextField{}
	for _, field := range contextType.Fields {
		declared[field.Name] = field
	}
	for _, name := range SortedKeys(data.Values) {
		if _, ok := declared[name]; !ok {
			return nil, Errorf("%s: field '%s' is not declared by context type %s", data.Path, name, contextType.ID)
		}
	}
	values := map[string]Value{}
	for _, field := range contextType.Fields {
		raw, present := data.Values[field.Name]
		if present && raw.Kind == "null" && field.Type != "list<string>" {
			present = false
		}
		if !present {
			if field.Required {
				return nil, Errorf("%s: required field '%s' of context type %s has no value", data.Path, field.Name, contextType.ID)
			}
			if !field.HasDefault {
				continue
			}
			raw = field.Default
		}
		value, err := field.Check(raw)
		if err != nil {
			return nil, Errorf("%s: field '%s' %s", data.Path, field.Name, err.(*Error).Message)
		}
		if field.Name == contextType.InstanceName && !IsHostName(value.Text) {
			return nil, Errorf("%s: field '%s' names this data's instances, so it must be lowercase letters, digits, and single hyphens, got '%s'", data.Path, field.Name, value.Text)
		}
		values[field.Name] = value
	}
	return values, nil
}

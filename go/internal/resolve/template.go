// Package resolve composes version 8 documents into resolved plugins, agents,
// and skills: promoted bindings, held documents, bound parameters, and
// rendered field references (docs/specification.md).
package resolve

import (
	"regexp"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// Binding is one parameter bound to a data document.
type Binding struct {
	Name        string
	DataID      string
	ContextType string
	Values      map[string]resource.Value
}

var fieldReference = regexp.MustCompile(`^\{\{([a-z][A-Za-z0-9]*)\.([A-Za-z][A-Za-z0-9_]*)\}\}`)

// scanReferences walks text for field references. In template mode, `\{{`
// writes a literal `{{` and each reference is replaced by what replace
// returns; outside template mode, text is returned unchanged.
func scanReferences(text string, template bool, replace func(name, field string) (string, error)) (string, error) {
	if !template || !strings.Contains(text, "{{") {
		return text, nil
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], `\{{`) {
			b.WriteString("{{")
			i += 3
			continue
		}
		if strings.HasPrefix(text[i:], "{{") {
			if m := fieldReference.FindStringSubmatch(text[i:]); m != nil {
				value, err := replace(m[1], m[2])
				if err != nil {
					return "", err
				}
				b.WriteString(value)
				i += len(m[0])
				continue
			}
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String(), nil
}

// checkReferences validates a template's references against its declared
// parameters without substituting anything.
func (r *Resolver) checkReferences(text, where string, parameters map[string]string) error {
	_, err := scanReferences(text, len(parameters) > 0, func(name, field string) (string, error) {
		typeID, declared := parameters[name]
		if !declared {
			return "", resource.Errorf("%s: {{%s.%s}} references '%s', which is not a declared parameter; write \\{{ for a literal", where, name, field, name)
		}
		_, err := r.fieldOf(typeID, field, where, name)
		return "", err
	})
	return err
}

func (r *Resolver) fieldOf(typeID, field, where, parameter string) (resource.ContextField, error) {
	contextType := r.docs[typeID]
	if contextType == nil || contextType.Kind != "contextType" {
		return resource.ContextField{}, resource.Errorf("%s: parameter '%s' names %s, which is not a context type in this build", where, parameter, typeID)
	}
	for _, f := range contextType.Fields {
		if f.Name == field {
			if f.Type == "list<string>" {
				return f, resource.Errorf("%s: {{%s.%s}} references a list<string> field, which cannot be inserted into text", where, parameter, field)
			}
			return f, nil
		}
	}
	return resource.ContextField{}, resource.Errorf("%s: {{%s.%s}}: context type %s has no field '%s'", where, parameter, field, typeID, field)
}

// render substitutes field references from bound parameters.
func (r *Resolver) render(text, where string, template bool, bindings map[string]*Binding) (string, error) {
	return scanReferences(text, template, func(name, field string) (string, error) {
		binding, bound := bindings[name]
		if !bound {
			return "", resource.Errorf("%s: {{%s.%s}} references '%s', which nothing binds", where, name, field, name)
		}
		if _, err := r.fieldOf(binding.ContextType, field, where, name); err != nil {
			return "", err
		}
		value, present := binding.Values[field]
		if !present {
			return "", resource.Errorf("%s: {{%s.%s}}: %s has no value for optional field '%s' and the field has no default", where, name, field, binding.DataID, field)
		}
		return value.Text, nil
	})
}

// referencedNames returns the parameter names text references, as written.
func referencedNames(text string) []string {
	names := []string{}
	_, _ = scanReferences(text, true, func(name, field string) (string, error) {
		names = append(names, name)
		return "", nil
	})
	return names
}

package resource

import (
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/tferlex"
)

// fieldDecoder6 applies schema-directed scalar typing to tferlex nodes: every
// field declares its type, and the scalar's spelling never decides it
// (ADR-0032). A quoted or block scalar is always a string.
type fieldDecoder6 struct {
	file string
}

func (d *fieldDecoder6) errorf(n *tferlex.Node, format string, args ...any) error {
	line := 0
	if n != nil {
		line = n.KeyLine
		if line == 0 {
			line = n.Line
		}
	}
	message := Errorf(format, args...).Message
	if line > 0 {
		return Errorf("%s:%d: %s", d.file, line, message)
	}
	return Errorf("%s: %s", d.file, message)
}

func child(n *tferlex.Node, key string) *tferlex.Node {
	if n == nil {
		return nil
	}
	for _, item := range n.Items {
		if item.Key == key {
			return item
		}
	}
	return nil
}

// decode applies a closed field table to a mapping node. An unknown key is an
// error that names the field.
func (d *fieldDecoder6) decode(n *tferlex.Node, fields map[string]func(*tferlex.Node) error) error {
	if n == nil {
		return nil
	}
	if !n.IsMap {
		return d.errorf(n, "expected a mapping")
	}
	for _, item := range n.Items {
		decode, known := fields[item.Key]
		if !known {
			return d.errorf(item, "unknown field '%s'", item.Key)
		}
		if err := decode(item); err != nil {
			return err
		}
	}
	return nil
}

// text returns a scalar's string value. Null is reported so optional fields
// can treat it as absent.
func (d *fieldDecoder6) text(n *tferlex.Node) (string, bool, error) {
	if !n.IsScalar {
		return "", false, d.errorf(n, "'%s' must be a string", n.Key)
	}
	if n.Value.Kind == tferlex.KindNull {
		return "", true, nil
	}
	return n.Value.Text, false, nil
}

func (d *fieldDecoder6) stringInto(target *string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, _, err := d.text(n)
		*target = value
		return err
	}
}

func (d *fieldDecoder6) boolean(n *tferlex.Node) (bool, error) {
	if !n.IsScalar {
		return false, d.errorf(n, "'%s' must be true or false", n.Key)
	}
	if n.Value.Kind != tferlex.KindPlain {
		return false, d.errorf(n, "'%s' must be the unquoted boolean true or false", n.Key)
	}
	switch n.Value.Text {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, d.errorf(n, "'%s' must be true or false, got '%s'", n.Key, n.Value.Text)
}

func (d *fieldDecoder6) boolInto(target *bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, err := d.boolean(n)
		*target = value
		return err
	}
}

// stringList decodes a sequence of string scalars. Null and [] are the empty
// list; presence is observable through the caller.
func (d *fieldDecoder6) stringList(n *tferlex.Node) ([]string, error) {
	if n.IsScalar && n.Value.Kind == tferlex.KindNull {
		return []string{}, nil
	}
	if !n.IsSeq {
		return nil, d.errorf(n, "'%s' must be a list", n.Key)
	}
	items := make([]string, 0, len(n.Items))
	for _, item := range n.Items {
		if !item.IsScalar || item.Value.Kind == tferlex.KindNull {
			return nil, d.errorf(item, "'%s' items must be strings", n.Key)
		}
		items = append(items, item.Value.Text)
	}
	return items, nil
}

func (d *fieldDecoder6) stringListInto(target *[]string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		items, err := d.stringList(n)
		*target = items
		return err
	}
}

func (d *fieldDecoder6) stringMap(n *tferlex.Node) (map[string]string, error) {
	if n.IsScalar && n.Value.Kind == tferlex.KindNull {
		return map[string]string{}, nil
	}
	if !n.IsMap {
		return nil, d.errorf(n, "'%s' must be a mapping", n.Key)
	}
	values := map[string]string{}
	for _, item := range n.Items {
		value, isNull, err := d.text(item)
		if err != nil {
			return nil, err
		}
		if isNull {
			return nil, d.errorf(item, "'%s' needs a value", item.Key)
		}
		values[item.Key] = value
	}
	return values, nil
}

var fieldValueKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// fieldValue decodes a context value without interpreting its scalars: the
// declared contextType field types decide what each scalar means during
// resolution. Quoted records whether the author wrote a quoted string.
func (d *fieldDecoder6) fieldValue(n *tferlex.Node) (FieldValue, error) {
	switch {
	case n.IsSeq:
		items := make([]FieldValue, 0, len(n.Items))
		for _, item := range n.Items {
			value, err := d.fieldValue(item)
			if err != nil {
				return FieldValue{}, err
			}
			items = append(items, value)
		}
		return FieldValue{Kind: "list", List: items}, nil
	case n.IsMap:
		values := map[string]FieldValue{}
		for _, item := range n.Items {
			if !fieldValueKey.MatchString(item.Key) {
				return FieldValue{}, d.errorf(item, "context value key '%s' must be an ASCII identifier", item.Key)
			}
			value, err := d.fieldValue(item)
			if err != nil {
				return FieldValue{}, err
			}
			values[item.Key] = value
		}
		return FieldValue{Kind: "map", Map: values}, nil
	}
	switch n.Value.Kind {
	case tferlex.KindNull:
		return FieldValue{Kind: "null"}, nil
	case tferlex.KindPlain:
		return FieldValue{Kind: "scalar", Scalar: n.Value.Text}, nil
	default:
		return FieldValue{Kind: "scalar", Scalar: n.Value.Text, Quoted: true}, nil
	}
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// singleLine rejects control characters and line breaks in routing and
// display strings, which are emitted into host frontmatter.
func singleLine(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return !strings.ContainsRune(value, '\u2028') && !strings.ContainsRune(value, '\u2029')
}

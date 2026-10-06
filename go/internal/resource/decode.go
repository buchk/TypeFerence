package resource

import (
	"github.com/buchk/TypeFerence/go/internal/tferlex"
)

// fieldDecoder applies schema-directed scalar typing to tferlex nodes: every
// field declares its type, and a scalar's spelling never decides it
// (ADR-0032). A quoted or block scalar is always a string.
type fieldDecoder struct {
	file string
}

func (d *fieldDecoder) errorf(n *tferlex.Node, format string, args ...any) error {
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

func isNull(n *tferlex.Node) bool {
	return n != nil && n.IsScalar && n.Value.Kind == tferlex.KindNull
}

// decode applies a closed field table to a mapping node. An unknown key is an
// error that names the field.
func (d *fieldDecoder) decode(n *tferlex.Node, fields map[string]func(*tferlex.Node) error) error {
	if n == nil || isNull(n) {
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
func (d *fieldDecoder) text(n *tferlex.Node) (string, bool, error) {
	if !n.IsScalar {
		return "", false, d.errorf(n, "'%s' must be a string", n.Key)
	}
	if n.Value.Kind == tferlex.KindNull {
		return "", true, nil
	}
	return n.Value.Text, false, nil
}

func (d *fieldDecoder) stringInto(target *string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, _, err := d.text(n)
		*target = value
		return err
	}
}

// optionalString decodes a string field whose presence matters.
func (d *fieldDecoder) optionalString(target **string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, null, err := d.text(n)
		if err != nil {
			return err
		}
		if null {
			return d.errorf(n, "'%s' needs a value", n.Key)
		}
		*target = &value
		return nil
	}
}

func (d *fieldDecoder) boolean(n *tferlex.Node) (bool, error) {
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

func (d *fieldDecoder) boolInto(target *bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, err := d.boolean(n)
		*target = value
		return err
	}
}

// optionalBool decodes a boolean field whose presence matters.
func (d *fieldDecoder) optionalBool(target **bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, err := d.boolean(n)
		if err != nil {
			return err
		}
		*target = &value
		return nil
	}
}

// stringList decodes a sequence of string scalars. Null and [] are the empty
// list.
func (d *fieldDecoder) stringList(n *tferlex.Node) ([]string, error) {
	if isNull(n) {
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

func (d *fieldDecoder) stringListInto(target *[]string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		items, err := d.stringList(n)
		*target = items
		return err
	}
}

func (d *fieldDecoder) stringMap(n *tferlex.Node) (map[string]string, error) {
	if isNull(n) {
		return map[string]string{}, nil
	}
	if !n.IsMap {
		return nil, d.errorf(n, "'%s' must be a mapping", n.Key)
	}
	values := map[string]string{}
	for _, item := range n.Items {
		value, null, err := d.text(item)
		if err != nil {
			return nil, err
		}
		if null {
			return nil, d.errorf(item, "'%s' needs a value", item.Key)
		}
		values[item.Key] = value
	}
	return values, nil
}

// fieldValue decodes a data value without interpreting its scalars: the
// declared context type field decides what each scalar means.
func (d *fieldDecoder) fieldValue(n *tferlex.Node) (FieldValue, error) {
	line := n.KeyLine
	if line == 0 {
		line = n.Line
	}
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
		return FieldValue{Kind: "list", List: items, Line: line}, nil
	case n.IsMap:
		return FieldValue{Kind: "map", Line: line}, nil
	}
	switch n.Value.Kind {
	case tferlex.KindNull:
		return FieldValue{Kind: "null", Line: line}, nil
	case tferlex.KindPlain:
		return FieldValue{Kind: "scalar", Scalar: n.Value.Text, Line: line}, nil
	default:
		return FieldValue{Kind: "scalar", Scalar: n.Value.Text, Quoted: true, Line: line}, nil
	}
}

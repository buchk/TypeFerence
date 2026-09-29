// Package tferlex implements the closed TypeFerence frontmatter grammar
// specified in docs/specification.md ("Frontmatter grammar", ADR-0026).
//
// The grammar is deliberately tiny: two-space indentation nesting, flow-free
// sequences, single- or double-quoted strings, literal block scalars with
// YAML 1.2 chomping/indentation semantics, inert comments, and syntactic
// scalar typing with no implicit resolution. Anchors, aliases, tags, flow
// collections, and multi-document streams are hard errors.
package tferlex

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind classifies a parsed scalar per the v5 syntactic rules.
type Kind int

const (
	KindString Kind = iota // quoted
	KindInteger
	KindDecimal
	KindBoolean
	KindNull
	KindBare // unquoted bare word: an error wherever a value is required
)

// Value is one typed scalar token.
type Value struct {
	Text string // the canonical content (quotes stripped, block scalar folded)
	Kind Kind
	Line int // 1-based line for diagnostics
}

// Node is a mapping, sequence, or scalar in the parsed tree.
type Node struct {
	Key      string
	KeyLine  int
	Value    Value   // valid when IsScalar
	Items    []*Node // sequence items or mapping entries (entries carry Key)
	IsMap    bool
	IsSeq    bool
	IsScalar bool
}

type line struct {
	indent  int
	text    string // comment-stripped, right-trimmed; empty for blank/comment lines
	num     int    // 1-based source line number
	rawText string // text after indent, before comment stripping (for block scalars)
}

// Parse parses frontmatter text into a mapping node.
func Parse(text string) (*Node, error) {
	lines, err := scanLines(text)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}
	pos := 0
	root, err := parseBlock(lines, &pos, lines[0].indent)
	if err != nil {
		return nil, err
	}
	if pos != len(lines) {
		return nil, fmt.Errorf("line %d: unexpected indentation (deeper than any enclosing mapping)", lines[pos].num)
	}
	if !root.IsMap {
		return nil, fmt.Errorf("frontmatter must be a mapping")
	}
	return root, nil
}

// scanLines strips comments (respecting quotes), records indents, and drops
// blank/comment-only lines. Tab indentation is an error; two-space increments
// are not enforced here but inconsistent dedents are caught by parseBlock.
func scanLines(text string) ([]line, error) {
	var out []line
	for i, raw := range strings.Split(text, "\n") {
		if i > 0 && i == len(strings.Split(text, "\n"))-1 && raw == "" {
			break // trailing newline of the document, not a blank body line
		}
		raw = strings.TrimRight(raw, "\r")
		indent := 0
		for indent < len(raw) && raw[indent] == ' ' {
			indent++
		}
		body := raw[indent:]
		if strings.HasPrefix(body, "\t") {
			return nil, fmt.Errorf("line %d: tab indentation is not allowed; use spaces", i+1)
		}
		stripped := stripComment(body)
		if stripped == "" {
			// Keep blank lines: readBlockScalar needs them to preserve
			// paragraph breaks; mapping parsers skip indent<0 entries.
			out = append(out, line{indent: -1, text: "", num: i + 1, rawText: body})
			continue
		}
		out = append(out, line{indent: indent, text: stripped, num: i + 1, rawText: body})
	}
	return out, nil
}

// stripComment removes an inline comment: a '#' at line start or preceded by
// whitespace, outside quotes.
func stripComment(s string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (i == 0 || s[i-1] == ' ') {
				return strings.TrimRight(s[:i], " ")
			}
		}
	}
	return strings.TrimRight(s, " ")
}

func isBlockScalar(l line) bool {
	return strings.HasSuffix(l.text, "|") || strings.HasSuffix(l.text, "|-") ||
		strings.HasPrefix(l.text, "|2 ") || l.text == "|2" ||
		strings.HasSuffix(l.text, ">") || strings.HasSuffix(l.text, ">-")
}

func parseBlock(lines []line, pos *int, indent int) (*Node, error) {
	if *pos >= len(lines) {
		return nil, fmt.Errorf("unexpected end of frontmatter")
	}
	if strings.HasPrefix(lines[*pos].text, "- ") || lines[*pos].text == "-" {
		return parseSequence(lines, pos, indent)
	}
	return parseMapping(lines, pos, indent)
}

func parseMapping(lines []line, pos *int, indent int) (*Node, error) {
	node := &Node{IsMap: true}
	seen := map[string]bool{}
	for *pos < len(lines) {
		l := lines[*pos]
		if l.indent < 0 {
			*pos++
			continue
		}
		if l.indent < indent {
			return node, nil
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: bad indentation (expected %d spaces)", l.num, indent)
		}
		if strings.HasPrefix(l.text, "- ") || l.text == "-" {
			return nil, fmt.Errorf("line %d: sequence item inside a mapping", l.num)
		}
		key, rest, err := splitKey(l)
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("line %d: duplicate property '%s'", l.num, key)
		}
		seen[key] = true
		*pos++

		if rest != "" {
			// Block scalar: consume following more-indented lines verbatim.
			if rest == "|" || rest == "|-" || rest == "|2" || rest == ">" || rest == ">-" {
				content, err := readBlockScalar(lines, pos, indent, rest)
				if err != nil {
					return nil, err
				}
				node.Items = append(node.Items, &Node{Key: key, KeyLine: l.num,
					Value: Value{Text: content, Kind: KindString, Line: l.num}, IsScalar: true})
				continue
			}
			v, err := classifyScalar(rest, l.num)
			if err != nil {
				return nil, err
			}
			node.Items = append(node.Items, &Node{Key: key, KeyLine: l.num, Value: v, IsScalar: true})
			continue
		}
		// Nested block value on following lines.
		if *pos < len(lines) && lines[*pos].indent > indent {
			child, err := parseBlock(lines, pos, lines[*pos].indent)
			if err != nil {
				return nil, err
			}
			child.Key = key
			child.KeyLine = l.num
			node.Items = append(node.Items, child)
		} else if *pos < len(lines) && lines[*pos].indent == indent &&
			(strings.HasPrefix(lines[*pos].text, "- ") || lines[*pos].text == "-") {
			// A sequence may sit at the same indent as its key.
			child, err := parseSequence(lines, pos, indent)
			if err != nil {
				return nil, err
			}
			child.Key = key
			child.KeyLine = l.num
			node.Items = append(node.Items, child)
		} else {
			// Empty value: null.
			node.Items = append(node.Items, &Node{Key: key, KeyLine: l.num,
				Value: Value{Kind: KindNull, Line: l.num}, IsScalar: true})
		}
	}
	return node, nil
}

func parseSequence(lines []line, pos *int, indent int) (*Node, error) {
	node := &Node{IsSeq: true}
	for *pos < len(lines) {
		l := lines[*pos]
		if l.indent < 0 {
			*pos++
			continue
		}
		if l.indent < indent || !(strings.HasPrefix(l.text, "- ") || l.text == "-") {
			return node, nil
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: bad indentation in sequence", l.num)
		}
		item := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		*pos++
		if item == "" {
			// Nested structure under the dash.
			if *pos < len(lines) && lines[*pos].indent > indent {
				child, err := parseBlock(lines, pos, lines[*pos].indent)
				if err != nil {
					return nil, err
				}
				node.Items = append(node.Items, child)
				continue
			}
			node.Items = append(node.Items, &Node{Value: Value{Kind: KindNull, Line: l.num}, IsScalar: true})
			continue
		}
		// Inline "key: value" under a dash starts a nested mapping whose
		// members continue on subsequent lines at indent+2.
		if k, rest, err := splitKeyText(item, l.num); err == nil {
			entry := &Node{IsMap: true, Key: "", KeyLine: l.num}
			seen := map[string]bool{}
			if err := addMapEntry(entry, seen, k, rest, lines, pos, indent+2, l); err != nil {
				return nil, err
			}
			// Continue collecting sibling keys of this map entry.
			for *pos < len(lines) && lines[*pos].indent == indent+2 &&
				!strings.HasPrefix(lines[*pos].text, "- ") {
				k2, rest2, err := splitKey(lines[*pos])
				if err != nil {
					return nil, err
				}
				if seen[k2] {
					return nil, fmt.Errorf("line %d: duplicate property '%s'", lines[*pos].num, k2)
				}
				seen[k2] = true
				cur := lines[*pos]
				*pos++
				if err := addMapEntry(entry, seen, k2, rest2, lines, pos, indent+2, cur); err != nil {
					return nil, err
				}
			}
			node.Items = append(node.Items, entry)
			continue
		}
		v, err := classifyScalar(item, l.num)
		if err != nil {
			return nil, err
		}
		node.Items = append(node.Items, &Node{Value: v, IsScalar: true})
	}
	return node, nil
}

// addMapEntry appends one key/rest pair to a mapping embedded in a sequence
// item, handling nested blocks that follow.
func addMapEntry(entry *Node, seen map[string]bool, key, rest string, lines []line, pos *int, childIndent int, cur line) error {
	if rest != "" {
		if rest == "|" || rest == "|-" || rest == "|2" || rest == ">" || rest == ">-" {
			content, err := readBlockScalar(lines, pos, childIndent-2, rest)
			if err != nil {
				return err
			}
			entry.Items = append(entry.Items, &Node{Key: key, KeyLine: cur.num,
				Value: Value{Text: content, Kind: KindString, Line: cur.num}, IsScalar: true})
			return nil
		}
		v, err := classifyScalar(rest, cur.num)
		if err != nil {
			return err
		}
		entry.Items = append(entry.Items, &Node{Key: key, KeyLine: cur.num, Value: v, IsScalar: true})
		return nil
	}
	if *pos < len(lines) && lines[*pos].indent > childIndent {
		child, err := parseBlock(lines, pos, lines[*pos].indent)
		if err != nil {
			return err
		}
		child.Key = key
		child.KeyLine = cur.num
		entry.Items = append(entry.Items, child)
	} else if *pos < len(lines) && lines[*pos].indent == childIndent &&
		(strings.HasPrefix(lines[*pos].text, "- ") || lines[*pos].text == "-") {
		child, err := parseSequence(lines, pos, childIndent)
		if err != nil {
			return err
		}
		child.Key = key
		child.KeyLine = cur.num
		entry.Items = append(entry.Items, child)
	} else {
		entry.Items = append(entry.Items, &Node{Key: key, KeyLine: cur.num,
			Value: Value{Kind: KindNull, Line: cur.num}, IsScalar: true})
	}
	return nil
}

// splitKey splits "key: rest" honoring quotes and requires exactly one colon.
func splitKey(l line) (key, rest string, err error) {
	return splitKeyText(l.text, l.num)
}

func splitKeyText(text string, num int) (string, string, error) {
	inSingle, inDouble := false, false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case ':':
			if !inSingle && !inDouble {
				if i+1 < len(text) && text[i+1] != ' ' {
					return "", "", fmt.Errorf("line %d: expected a space after ':'", num)
				}
				key := strings.TrimSpace(text[:i])
				rest := strings.TrimSpace(text[i+1:])
				if key == "" {
					return "", "", fmt.Errorf("line %d: empty key", num)
				}
				if strings.ContainsAny(key, ":{}[],&*!>") {
					return "", "", fmt.Errorf("line %d: invalid character in key '%s'", num, key)
				}
				return key, rest, nil
			}
		case '{', '[', '&', '*', '!':
			if !inSingle && !inDouble {
				return "", "", fmt.Errorf("line %d: flow collections, anchors, aliases, and tags do not exist in the TypeFerence grammar", num)
			}
		}
	}
	return "", "", fmt.Errorf("line %d: expected 'key: value'", num)
}

// readBlockScalar consumes the more-indented body lines of a block scalar and
// applies YAML 1.2 clip (|) / strip (-) chomping with the explicit |2
// indentation indicator.
func readBlockScalar(lines []line, pos *int, keyIndent int, header string) (string, error) {
	chompStrip := strings.Contains(header, "-")
	explicitIndent := 0
	if strings.HasPrefix(header, "|2") {
		explicitIndent = 2
	}
	var body []string
	contentIndent := -1 // absolute column where scalar content starts
	for *pos < len(lines) {
		l := lines[*pos]
		if l.indent < 0 {
			// Blank line inside the block scalar: preserved as empty.
			body = append(body, "")
			*pos++
			continue
		}
		if l.indent <= keyIndent {
			break
		}
		if contentIndent < 0 {
			if explicitIndent > 0 {
				contentIndent = keyIndent + explicitIndent
				if l.indent < contentIndent {
					return "", fmt.Errorf("line %d: block scalar content less indented than the explicit indicator", l.num)
				}
			} else {
				contentIndent = l.indent
			}
		}
		// l.text is comment-stripped text at the line's own indent; re-add
		// any indentation deeper than the scalar's base indent so internal
		// structure is preserved.
		extra := l.indent - contentIndent
		if extra > 0 {
			body = append(body, strings.Repeat(" ", extra)+l.text)
		} else {
			body = append(body, l.text)
		}
		*pos++
	}
	// YAML 1.2 clip: single trailing newline; strip: none. Leading/trailing
	// blank lines within the body are preserved by construction above.
	content := strings.Join(body, "\n")
	if !chompStrip && content != "" {
		content += "\n"
	}
	return content, nil
}

var (
	intRe    = regexp.MustCompile(`^[+-]?[0-9]+$`)
	decRe    = regexp.MustCompile(`^[+-]?([0-9]+\.[0-9]*|\.[0-9]+|[0-9]+)([eE][+-]?[0-9]+)?$`)
	boolRe   = regexp.MustCompile(`^(true|false)$`)
	floatExp = regexp.MustCompile(`[eE.]`)
)

// classifyScalar applies the v5 syntactic typing rules to one scalar token.
func classifyScalar(raw string, num int) (Value, error) {
	if raw == "" {
		return Value{Kind: KindNull, Line: num}, nil
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		unq := raw[1 : len(raw)-1]
		if strings.Contains(unq, "\"\"") {
			unq = strings.ReplaceAll(unq, "\"\"", "\"")
		}
		return Value{Text: unq, Kind: KindString, Line: num}, nil
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		unq := raw[1 : len(raw)-1]
		unq = strings.ReplaceAll(unq, "''", "'")
		return Value{Text: unq, Kind: KindString, Line: num}, nil
	}
	if raw == "null" || raw == "~" {
		return Value{Kind: KindNull, Line: num}, nil
	}
	if boolRe.MatchString(raw) {
		return Value{Text: raw, Kind: KindBoolean, Line: num}, nil
	}
	if intRe.MatchString(raw) {
		return Value{Text: raw, Kind: KindInteger, Line: num}, nil
	}
	if decRe.MatchString(raw) && floatExp.MatchString(raw) {
		return Value{Text: raw, Kind: KindDecimal, Line: num}, nil
	}
	return Value{Text: raw, Kind: KindBare, Line: num},
		fmt.Errorf("line %d: bare word '%s' is not allowed; quote it to make it a string", num, raw)
}

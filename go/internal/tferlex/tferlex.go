// Package tferlex implements the closed TypeFerence frontmatter grammar
// specified in docs/specification.md ("Frontmatter grammar", ADR-0032).
//
// The grammar is deliberately tiny: indentation-nested mappings and
// sequences, single- and double-quoted strings, literal block scalars with
// YAML 1.2 clip/strip chomping and an optional explicit indentation
// indicator, inert comments, and the two empty-collection tokens `[]` and
// `{}`. Anchors, aliases, tags, flow collections, folded scalars, quoted keys,
// and multi-document streams are hard errors.
//
// Scalar typing is schema-directed and happens in exactly one layer: this
// package never guesses a type. An unquoted scalar is returned as KindPlain
// with its exact text, and the consumer's declared field type decides what it
// means (a string field keeps the text verbatim; a boolean field accepts only
// `true` or `false`; and so on). A quoted scalar or block scalar is always a
// string. There is no implicit resolution anywhere.
package tferlex

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind classifies a scalar token.
type Kind int

const (
	// KindPlain is an unquoted scalar. Its meaning is decided by the field it
	// is assigned to, never by its spelling.
	KindPlain Kind = iota
	// KindQuoted is a single- or double-quoted scalar: always a string.
	KindQuoted
	// KindBlock is a literal block scalar: always a string.
	KindBlock
	// KindNull is an empty value, `null`, or `~`.
	KindNull
)

// Value is one scalar token.
type Value struct {
	Text string // content: quotes removed and escapes decoded
	Kind Kind
	Line int // 1-based line for diagnostics
}

// Node is a mapping, sequence, or scalar in the parsed tree. Mapping entries
// and sequence items are both stored in Items; mapping entries carry Key.
type Node struct {
	Key      string
	KeyLine  int
	Line     int
	Value    Value // valid when IsScalar
	Items    []*Node
	IsMap    bool
	IsSeq    bool
	IsScalar bool
}

// Error is a positioned grammar error.
type Error struct {
	Line    int
	Message string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

func errorf(line int, format string, args ...any) error {
	return &Error{Line: line, Message: fmt.Sprintf(format, args...)}
}

// KeyPattern is the closed key grammar: identifiers, slot and field names,
// mode names, and package names (which contain '/').
var KeyPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]*$`)

type line struct {
	num     int
	raw     string // full line without its line terminator
	indent  int    // leading spaces; -1 for blank or comment-only lines
	content string // text after the indent with any comment removed, right-trimmed
}

// Parse parses frontmatter text into its root mapping. Empty or
// comment-only input returns (nil, nil).
func Parse(text string) (*Node, error) {
	if !utf8.ValidString(text) {
		return nil, errorf(0, "frontmatter is not valid UTF-8")
	}
	p := &parser{lines: scanLines(text)}
	p.skipBlank()
	if p.pos >= len(p.lines) {
		return nil, nil
	}
	first := p.lines[p.pos]
	if err := structural(first); err != nil {
		return nil, err
	}
	if first.indent != 0 {
		return nil, errorf(first.num, "the frontmatter mapping must start at column 1")
	}
	if isSequenceItem(first.content) {
		return nil, errorf(first.num, "frontmatter must be a mapping, not a sequence")
	}
	root, err := p.mapping(0)
	if err != nil {
		return nil, err
	}
	p.skipBlank()
	if p.pos < len(p.lines) {
		return nil, errorf(p.lines[p.pos].num, "unexpected indentation")
	}
	return root, nil
}

func scanLines(text string) []line {
	parts := strings.Split(text, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	out := make([]line, 0, len(parts))
	for i, raw := range parts {
		raw = strings.TrimSuffix(raw, "\r")
		indent := 0
		for indent < len(raw) && raw[indent] == ' ' {
			indent++
		}
		rest := raw[indent:]
		l := line{num: i + 1, raw: raw, indent: indent}
		if strings.TrimSpace(rest) == "" || strings.HasPrefix(rest, "#") {
			l.indent = -1
		} else {
			l.content = stripComment(rest)
		}
		out = append(out, l)
	}
	return out
}

// stripComment removes an inline comment: a '#' at the start of the text or
// preceded by whitespace, outside quotes. Double-quoted text honors backslash
// escapes; in single-quoted text, two single quotes are a literal quote.
func stripComment(s string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inDouble:
			if c == '\\' {
				i++
			} else if c == '"' {
				inDouble = false
			}
		case inSingle:
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
				} else {
					inSingle = false
				}
			}
		default:
			switch c {
			case '"':
				if opensScalar(s, i) {
					inDouble = true
				}
			case '\'':
				if opensScalar(s, i) {
					inSingle = true
				}
			case '#':
				if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
					return strings.TrimRight(s[:i], " \t")
				}
			}
		}
	}
	return strings.TrimRight(s, " \t")
}

// opensScalar reports whether a quote at position i opens a quoted scalar: it
// must begin a value (after "key: ", after "- ", or at the start of the
// text). A quote inside a plain scalar (don't, say "x") is ordinary text.
func opensScalar(s string, i int) bool {
	prefix := strings.TrimRight(s[:i], " ")
	return prefix == "" || prefix == "-" || strings.HasSuffix(prefix, ":")
}

type parser struct {
	lines []line
	pos   int
}

func (p *parser) skipBlank() {
	for p.pos < len(p.lines) && p.lines[p.pos].indent < 0 {
		p.pos++
	}
}

func isSequenceItem(content string) bool {
	return content == "-" || strings.HasPrefix(content, "- ")
}

// structural rejects tab indentation on lines that carry structure. Tabs
// inside block-scalar content are ordinary characters and never reach here.
func structural(l line) error {
	if strings.HasPrefix(l.content, "\t") {
		return errorf(l.num, "tab indentation is not allowed; indent with spaces")
	}
	return nil
}

// mapping parses consecutive "key: value" entries at exactly indent.
func (p *parser) mapping(indent int) (*Node, error) {
	node := &Node{IsMap: true}
	if p.pos < len(p.lines) {
		node.Line = p.lines[p.pos].num
	}
	seen := map[string]bool{}
	for {
		p.skipBlank()
		if p.pos >= len(p.lines) {
			return node, nil
		}
		l := p.lines[p.pos]
		if l.indent < indent {
			return node, nil
		}
		if err := structural(l); err != nil {
			return nil, err
		}
		if l.indent > indent {
			return nil, errorf(l.num, "unexpected indentation")
		}
		if isSequenceItem(l.content) {
			return node, nil
		}
		key, rest, err := splitKey(l.content, l.num)
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, errorf(l.num, "duplicate property '%s'", key)
		}
		seen[key] = true
		p.pos++
		entry, err := p.value(key, rest, l.num, indent)
		if err != nil {
			return nil, err
		}
		node.Items = append(node.Items, entry)
	}
}

// value parses the value of a mapping entry whose key sits at keyIndent.
func (p *parser) value(key, rest string, num, keyIndent int) (*Node, error) {
	if rest != "" {
		if isBlockHeader(rest) {
			text, err := p.blockScalar(rest, keyIndent, num)
			if err != nil {
				return nil, err
			}
			return &Node{Key: key, KeyLine: num, Line: num, IsScalar: true,
				Value: Value{Text: text, Kind: KindBlock, Line: num}}, nil
		}
		child, err := inlineValue(rest, num)
		if err != nil {
			return nil, err
		}
		child.Key, child.KeyLine = key, num
		return child, nil
	}
	// No inline value: a nested block follows, or the value is null.
	p.skipBlank()
	if p.pos < len(p.lines) {
		next := p.lines[p.pos]
		if next.indent > keyIndent {
			if err := structural(next); err != nil {
				return nil, err
			}
			var child *Node
			var err error
			if isSequenceItem(next.content) {
				child, err = p.sequence(next.indent)
			} else {
				child, err = p.mapping(next.indent)
			}
			if err != nil {
				return nil, err
			}
			child.Key, child.KeyLine = key, num
			return child, nil
		}
		if next.indent == keyIndent && isSequenceItem(next.content) {
			// A sequence may sit at the same indentation as its key.
			child, err := p.sequence(keyIndent)
			if err != nil {
				return nil, err
			}
			child.Key, child.KeyLine = key, num
			return child, nil
		}
	}
	return &Node{Key: key, KeyLine: num, Line: num, IsScalar: true,
		Value: Value{Kind: KindNull, Line: num}}, nil
}

// sequence parses consecutive "- item" lines at exactly indent.
func (p *parser) sequence(indent int) (*Node, error) {
	node := &Node{IsSeq: true, Line: p.lines[p.pos].num}
	for {
		p.skipBlank()
		if p.pos >= len(p.lines) {
			return node, nil
		}
		l := p.lines[p.pos]
		if l.indent < indent || (l.indent == indent && !isSequenceItem(l.content)) {
			return node, nil
		}
		if err := structural(l); err != nil {
			return nil, err
		}
		if l.indent > indent {
			return nil, errorf(l.num, "unexpected indentation in sequence")
		}
		p.pos++
		afterDash := strings.TrimPrefix(l.content, "-")
		item := strings.TrimLeft(afterDash, " ")
		if item == "" {
			p.skipBlank()
			if p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
				next := p.lines[p.pos]
				if err := structural(next); err != nil {
					return nil, err
				}
				var child *Node
				var err error
				if isSequenceItem(next.content) {
					child, err = p.sequence(next.indent)
				} else {
					child, err = p.mapping(next.indent)
				}
				if err != nil {
					return nil, err
				}
				child.Line = l.num
				node.Items = append(node.Items, child)
				continue
			}
			node.Items = append(node.Items, &Node{Line: l.num, IsScalar: true,
				Value: Value{Kind: KindNull, Line: l.num}})
			continue
		}
		if key, rest, ok := inlineKey(item); ok {
			// "- key: value" opens a mapping whose further keys align with key.
			entryIndent := indent + 1 + (len(afterDash) - len(item))
			mapping := &Node{IsMap: true, Line: l.num}
			first, err := p.value(key, rest, l.num, entryIndent)
			if err != nil {
				return nil, err
			}
			mapping.Items = append(mapping.Items, first)
			seen := map[string]bool{key: true}
			for {
				p.skipBlank()
				if p.pos >= len(p.lines) {
					break
				}
				next := p.lines[p.pos]
				if next.indent < entryIndent {
					break
				}
				if err := structural(next); err != nil {
					return nil, err
				}
				if next.indent > entryIndent || isSequenceItem(next.content) {
					return nil, errorf(next.num, "unexpected indentation")
				}
				k, r, err := splitKey(next.content, next.num)
				if err != nil {
					return nil, err
				}
				if seen[k] {
					return nil, errorf(next.num, "duplicate property '%s'", k)
				}
				seen[k] = true
				p.pos++
				entry, err := p.value(k, r, next.num, entryIndent)
				if err != nil {
					return nil, err
				}
				mapping.Items = append(mapping.Items, entry)
			}
			node.Items = append(node.Items, mapping)
			continue
		}
		if isBlockHeader(item) {
			return nil, errorf(l.num, "a block scalar cannot be a sequence item; use a quoted string")
		}
		child, err := inlineValue(item, l.num)
		if err != nil {
			return nil, err
		}
		node.Items = append(node.Items, child)
	}
}

// inlineKey reports whether a sequence item's text is "key: value" or "key:".
func inlineKey(item string) (string, string, bool) {
	if item[0] == '"' || item[0] == '\'' {
		return "", "", false
	}
	idx := strings.Index(item, ":")
	if idx <= 0 || (idx+1 < len(item) && item[idx+1] != ' ') {
		return "", "", false
	}
	key := item[:idx]
	if !KeyPattern.MatchString(key) {
		return "", "", false
	}
	return key, strings.TrimSpace(item[idx+1:]), true
}

// splitKey splits "key: rest" (or "key:") at the first colon.
func splitKey(content string, num int) (string, string, error) {
	switch content[0] {
	case '"', '\'':
		return "", "", errorf(num, "quoted keys do not exist; write the key unquoted")
	case '?', '&', '*', '!', '%', '@', '`', '[', '{', '|', '>':
		return "", "", errorf(num, "expected 'key: value'")
	}
	idx := strings.Index(content, ":")
	if idx < 0 {
		return "", "", errorf(num, "expected 'key: value'")
	}
	if idx+1 < len(content) && content[idx+1] != ' ' {
		return "", "", errorf(num, "expected a space after ':' in '%s'", content)
	}
	key := content[:idx]
	if !KeyPattern.MatchString(key) {
		return "", "", errorf(num, "invalid key '%s': keys use letters, digits, '_', '.', '/', and '-'", key)
	}
	return key, strings.TrimSpace(content[idx+1:]), nil
}

func isBlockHeader(rest string) bool {
	return rest[0] == '|' || rest[0] == '>'
}

// blockScalar consumes the raw lines of a literal block scalar. Content is
// every following line indented deeper than the key; comment characters
// inside the block are ordinary text.
func (p *parser) blockScalar(header string, keyIndent, num int) (string, error) {
	if header[0] == '>' {
		return "", errorf(num, "folded block scalars ('>') do not exist; use a literal block ('|')")
	}
	strip := false
	body := header[1:]
	if strings.HasSuffix(body, "-") {
		strip = true
		body = strings.TrimSuffix(body, "-")
	}
	contentIndent := -1
	if body != "" {
		n, err := strconv.Atoi(body)
		if err != nil || n < 1 || n > 9 {
			return "", errorf(num, "invalid block scalar header '%s'; use |, |-, |N, or |N-", header)
		}
		contentIndent = keyIndent + n
	}
	var lines []string
	for p.pos < len(p.lines) {
		raw := p.lines[p.pos].raw
		indent := 0
		for indent < len(raw) && raw[indent] == ' ' {
			indent++
		}
		if strings.TrimSpace(raw) == "" {
			lines = append(lines, "")
			p.pos++
			continue
		}
		if indent <= keyIndent {
			break
		}
		if contentIndent < 0 {
			contentIndent = indent
		}
		if indent < contentIndent {
			return "", errorf(p.lines[p.pos].num, "block scalar line is less indented than the block's content")
		}
		lines = append(lines, raw[contentIndent:])
		p.pos++
	}
	// Trailing blank lines are not content under clip or strip chomping.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	text := strings.Join(lines, "\n")
	if !strip && text != "" {
		text += "\n"
	}
	return text, nil
}

// inlineValue parses a scalar or empty-collection token that follows "key: "
// or "- ".
func inlineValue(rest string, num int) (*Node, error) {
	switch rest {
	case "[]":
		return &Node{IsSeq: true, Line: num}, nil
	case "{}":
		return &Node{IsMap: true, Line: num}, nil
	case "null", "~":
		return &Node{Line: num, IsScalar: true, Value: Value{Kind: KindNull, Line: num}}, nil
	}
	switch rest[0] {
	case '[', '{':
		return nil, errorf(num, "flow collections do not exist in the TypeFerence grammar; only the empty tokens [] and {} are allowed")
	case '&', '*':
		return nil, errorf(num, "anchors and aliases do not exist in the TypeFerence grammar")
	case '!':
		return nil, errorf(num, "tags do not exist in the TypeFerence grammar")
	case '"':
		text, err := decodeDouble(rest, num)
		if err != nil {
			return nil, err
		}
		return &Node{Line: num, IsScalar: true, Value: Value{Text: text, Kind: KindQuoted, Line: num}}, nil
	case '\'':
		text, err := decodeSingle(rest, num)
		if err != nil {
			return nil, err
		}
		return &Node{Line: num, IsScalar: true, Value: Value{Text: text, Kind: KindQuoted, Line: num}}, nil
	}
	return &Node{Line: num, IsScalar: true, Value: Value{Text: rest, Kind: KindPlain, Line: num}}, nil
}

// decodeDouble decodes a double-quoted scalar using JSON string escapes.
func decodeDouble(token string, num int) (string, error) {
	var b strings.Builder
	for i := 1; i < len(token); i++ {
		c := token[i]
		if c == '"' {
			if strings.TrimSpace(token[i+1:]) != "" {
				return "", errorf(num, "unexpected text after a quoted string")
			}
			return b.String(), nil
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(token) {
			break
		}
		switch token[i] {
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case '/':
			b.WriteByte('/')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			r, consumed, err := decodeUnicodeEscape(token, i, num)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
			i += consumed
		default:
			return "", errorf(num, "invalid escape '\\%c' in a quoted string", token[i])
		}
	}
	return "", errorf(num, "unterminated quoted string")
}

// decodeUnicodeEscape decodes \uXXXX (and a following low surrogate) where
// token[i] is the 'u'. It returns the rune and how many bytes after the 'u'
// were consumed.
func decodeUnicodeEscape(token string, i, num int) (rune, int, error) {
	hex := func(at int) (rune, bool) {
		if at+4 > len(token) {
			return 0, false
		}
		v, err := strconv.ParseUint(token[at:at+4], 16, 32)
		return rune(v), err == nil
	}
	r, ok := hex(i + 1)
	if !ok {
		return 0, 0, errorf(num, "invalid \\u escape in a quoted string")
	}
	consumed := 4
	if r >= 0xD800 && r <= 0xDBFF && i+6 < len(token) && token[i+5] == '\\' && token[i+6] == 'u' {
		if low, lowOK := hex(i + 7); lowOK && low >= 0xDC00 && low <= 0xDFFF {
			r = (r-0xD800)<<10 + (low - 0xDC00) + 0x10000
			consumed = 10
		}
	}
	if r >= 0xD800 && r <= 0xDFFF {
		return 0, 0, errorf(num, "unpaired surrogate escape in a quoted string")
	}
	return r, consumed, nil
}

// decodeSingle decodes a single-quoted scalar, where two single quotes are a
// literal quote.
func decodeSingle(token string, num int) (string, error) {
	var b strings.Builder
	for i := 1; i < len(token); i++ {
		if token[i] != '\'' {
			b.WriteByte(token[i])
			continue
		}
		if i+1 < len(token) && token[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		if strings.TrimSpace(token[i+1:]) != "" {
			return "", errorf(num, "unexpected text after a quoted string")
		}
		return b.String(), nil
	}
	return "", errorf(num, "unterminated quoted string")
}

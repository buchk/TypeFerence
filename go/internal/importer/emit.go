// Package importer turns existing GitHub Copilot customizations (custom
// agents, Agent Skills, and Agent Plugins) into version 7 TypeFerence sources
// (ADR-0008). It writes only constructs a person could hand-author.
package importer

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// frontmatter builds version 7 frontmatter text deterministically: keys in
// the order written, strings plain where the grammar allows and quoted
// otherwise, multi-line strings as literal blocks.
type frontmatter struct {
	b strings.Builder
}

func (f *frontmatter) String() string { return f.b.String() }

func pad(indent int) string { return strings.Repeat(" ", indent) }

// scalar writes key: value, skipping empty values.
func (f *frontmatter) scalar(indent int, key, value string) {
	if value == "" {
		return
	}
	f.raw(indent, key, value)
}

// raw writes key: value even when value is empty.
func (f *frontmatter) raw(indent int, key, value string) {
	if strings.Contains(value, "\n") {
		f.block(indent, key, value)
		return
	}
	f.b.WriteString(pad(indent) + key + ": " + scalarText(value) + "\n")
}

// token writes key: value verbatim (booleans, integers, empty collections).
func (f *frontmatter) token(indent int, key, value string) {
	f.b.WriteString(pad(indent) + key + ": " + value + "\n")
}

func (f *frontmatter) key(indent int, key string) {
	f.b.WriteString(pad(indent) + key + ":\n")
}

// list writes a sequence of plain or quoted strings.
func (f *frontmatter) list(indent int, key string, values []string) {
	if len(values) == 0 {
		return
	}
	f.key(indent, key)
	for _, value := range values {
		f.b.WriteString(pad(indent+2) + "- " + scalarText(value) + "\n")
	}
}

// block writes a literal block scalar, preserving the text up to a trailing
// newline (clip) or its absence (strip).
func (f *frontmatter) block(indent int, key, value string) {
	header := "|"
	content := strings.TrimRight(value, "\n")
	if !strings.HasSuffix(value, "\n") {
		header = "|-"
	}
	firstLine := strings.SplitN(content, "\n", 2)[0]
	if strings.HasPrefix(firstLine, " ") {
		header = "|2" + strings.TrimPrefix(header, "|")
	}
	f.b.WriteString(pad(indent) + key + ": " + header + "\n")
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			f.b.WriteString("\n")
			continue
		}
		f.b.WriteString(pad(indent+2) + line + "\n")
	}
}

var plainStart = regexp.MustCompile(`^(?:[A-Za-z0-9_./(]|-[0-9.])`)

// plainSafe reports whether a string survives as an unquoted scalar with the
// same text in every position the emitter writes it (mapping value or
// sequence item).
func plainSafe(s string) bool {
	if s == "" || s != strings.TrimSpace(s) || !plainStart.MatchString(s) {
		return false
	}
	switch s {
	case "null", "~", "[]", "{}":
		return false
	}
	if hasControl(s) {
		return false
	}
	return !strings.Contains(s, " #") && !strings.Contains(s, ": ") && !strings.HasSuffix(s, ":")
}

// scalarText renders a single-line string as a plain scalar when safe, a
// single-quoted string when that needs no escapes, and a double-quoted
// string with JSON escapes otherwise.
func scalarText(s string) string {
	if plainSafe(s) {
		return s
	}
	if !strings.Contains(s, "'") && utf8.ValidString(s) && !hasControl(s) {
		return "'" + s + "'"
	}
	return doubleQuoted(s)
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func doubleQuoted(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				hex := strconv.FormatInt(int64(r), 16)
				b.WriteString(`\u` + strings.Repeat("0", 4-len(hex)) + hex)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// document assembles a version 7 document: fenced frontmatter and a body.
func document(fm *frontmatter, body string) string {
	text := "---\n" + fm.String() + "---\n"
	if strings.TrimSpace(body) != "" {
		text += body
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
	}
	return text
}

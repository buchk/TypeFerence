package jsonx

import (
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Compact serializes v with no whitespace, matching the reference
// implementation's compact form (used for canonical JSON schemas).
func Compact(v Value) string {
	return string(appendValue(nil, v, -1))
}

// Indented serializes v with two-space indentation, ": " after keys, and LF
// line endings, matching the reference implementation's indented form after
// newline normalization.
func Indented(v Value) string {
	return string(appendValue(nil, v, 0))
}

// appendValue appends v to dst. level < 0 means compact; otherwise level is
// the current indentation depth.
func appendValue(dst []byte, v Value, level int) []byte {
	switch t := v.(type) {
	case Str:
		return AppendString(dst, string(t))
	case Num:
		return append(dst, t...)
	case Bool:
		if t {
			return append(dst, "true"...)
		}
		return append(dst, "false"...)
	case Null:
		return append(dst, "null"...)
	case Arr:
		if len(t) == 0 {
			return append(dst, "[]"...)
		}
		dst = append(dst, '[')
		for i, item := range t {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendNewlineIndent(dst, childLevel(level))
			dst = appendValue(dst, item, childLevel(level))
		}
		dst = appendNewlineIndent(dst, level)
		return append(dst, ']')
	case Obj:
		if len(t) == 0 {
			return append(dst, "{}"...)
		}
		dst = append(dst, '{')
		for i, member := range t {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendNewlineIndent(dst, childLevel(level))
			dst = AppendString(dst, member.K)
			dst = append(dst, ':')
			if level >= 0 {
				dst = append(dst, ' ')
			}
			dst = appendValue(dst, member.V, childLevel(level))
		}
		dst = appendNewlineIndent(dst, level)
		return append(dst, '}')
	}
	panic("jsonx: unknown value type")
}

func childLevel(level int) int {
	if level < 0 {
		return -1
	}
	return level + 1
}

func appendNewlineIndent(dst []byte, level int) []byte {
	if level < 0 {
		return dst
	}
	dst = append(dst, '\n')
	for range level {
		dst = append(dst, "  "...)
	}
	return dst
}

// AppendString appends s as a quoted JSON string using the reference
// implementation's default escape policy: ASCII 0x20..0x7E stay literal except
// `"` `&` `'` `+` `<` `>` `\` and backtick; \b \t \n \f \r use short escapes;
// backslash doubles; everything else (including all non-ASCII) becomes
// uppercase \uXXXX, with surrogate pairs for supplementary-plane runes.
// Invalid UTF-8 bytes are replaced with U+FFFD, matching how the reference
// implementation decodes text before serializing it.
func AppendString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for _, r := range s {
		switch {
		case r == '\\':
			dst = append(dst, '\\', '\\')
		case r == '\b':
			dst = append(dst, '\\', 'b')
		case r == '\t':
			dst = append(dst, '\\', 't')
		case r == '\n':
			dst = append(dst, '\\', 'n')
		case r == '\f':
			dst = append(dst, '\\', 'f')
		case r == '\r':
			dst = append(dst, '\\', 'r')
		case r >= 0x20 && r <= 0x7E && !escapedASCII(r):
			dst = append(dst, byte(r))
		case r > 0xFFFF:
			hi, lo := utf16.EncodeRune(r)
			dst = appendUnicodeEscape(dst, hi)
			dst = appendUnicodeEscape(dst, lo)
		default:
			if r == utf8.RuneError {
				// Range over string yields RuneError for invalid bytes; the
				// replacement character itself is also escaped this way.
				r = 0xFFFD
			}
			dst = appendUnicodeEscape(dst, r)
		}
	}
	return append(dst, '"')
}

func escapedASCII(r rune) bool {
	switch r {
	case '"', '&', '\'', '+', '<', '>', '`':
		return true
	}
	return false
}

func appendUnicodeEscape(dst []byte, r rune) []byte {
	const hex = "0123456789ABCDEF"
	dst = append(dst, '\\', 'u')
	for shift := 12; shift >= 0; shift -= 4 {
		dst = append(dst, hex[(r>>shift)&0xF])
	}
	return dst
}

// Int returns a Num for a Go integer.
func Int(v int64) Num {
	return Num(strconv.FormatInt(v, 10))
}

package jsonx

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const maxDepth = 64

// Parse reads a single strict RFC 8259 JSON document. Number tokens are kept
// raw, object member order and duplicate keys are preserved, so
// Compact(Parse(x)) is the canonical form of x.
func Parse(input string) (Value, error) {
	p := &parser{s: input}
	p.skipSpace()
	v, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.i != len(p.s) {
		return nil, p.errorf("unexpected content after JSON document")
	}
	return v, nil
}

type parser struct {
	s string
	i int
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("invalid JSON at offset %d: %s", p.i, fmt.Sprintf(format, args...))
}

func (p *parser) skipSpace() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (Value, error) {
	if depth > maxDepth {
		return nil, p.errorf("maximum depth exceeded")
	}
	if p.i >= len(p.s) {
		return nil, p.errorf("unexpected end of input")
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, err := p.string()
		if err != nil {
			return nil, err
		}
		return Str(s), nil
	case c == 't':
		return p.literal("true", Bool(true))
	case c == 'f':
		return p.literal("false", Bool(false))
	case c == 'n':
		return p.literal("null", Null{})
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	default:
		return nil, p.errorf("unexpected character %q", c)
	}
}

func (p *parser) literal(text string, v Value) (Value, error) {
	if !strings.HasPrefix(p.s[p.i:], text) {
		return nil, p.errorf("invalid literal")
	}
	p.i += len(text)
	return v, nil
}

func (p *parser) object(depth int) (Value, error) {
	p.i++ // '{'
	obj := Obj{}
	p.skipSpace()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return obj, nil
	}
	for {
		p.skipSpace()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, p.errorf("expected object key")
		}
		key, err := p.string()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, p.errorf("expected ':' after object key")
		}
		p.i++
		p.skipSpace()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		obj = append(obj, Member{K: key, V: v})
		p.skipSpace()
		if p.i >= len(p.s) {
			return nil, p.errorf("unterminated object")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return obj, nil
		default:
			return nil, p.errorf("expected ',' or '}' in object")
		}
	}
}

func (p *parser) array(depth int) (Value, error) {
	p.i++ // '['
	arr := Arr{}
	p.skipSpace()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return arr, nil
	}
	for {
		p.skipSpace()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		p.skipSpace()
		if p.i >= len(p.s) {
			return nil, p.errorf("unterminated array")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return arr, nil
		default:
			return nil, p.errorf("expected ',' or ']' in array")
		}
	}
}

func (p *parser) string() (string, error) {
	p.i++ // '"'
	var b strings.Builder
	for {
		if p.i >= len(p.s) {
			return "", p.errorf("unterminated string")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), nil
		case c == '\\':
			p.i++
			if p.i >= len(p.s) {
				return "", p.errorf("unterminated escape")
			}
			switch e := p.s[p.i]; e {
			case '"', '\\', '/':
				b.WriteByte(e)
				p.i++
			case 'b':
				b.WriteByte('\b')
				p.i++
			case 'f':
				b.WriteByte('\f')
				p.i++
			case 'n':
				b.WriteByte('\n')
				p.i++
			case 'r':
				b.WriteByte('\r')
				p.i++
			case 't':
				b.WriteByte('\t')
				p.i++
			case 'u':
				p.i++
				r, err := p.hex4()
				if err != nil {
					return "", err
				}
				if utf16.IsSurrogate(r) {
					if r >= 0xD800 && r <= 0xDBFF && p.i+1 < len(p.s) && p.s[p.i] == '\\' && p.s[p.i+1] == 'u' {
						p.i += 2
						lo, err := p.hex4()
						if err != nil {
							return "", err
						}
						if combined := utf16.DecodeRune(r, lo); combined != utf8.RuneError {
							b.WriteRune(combined)
							break
						}
						// Not a valid pair: both decode to replacement chars,
						// matching the reference implementation's string
						// round-trip behavior.
						b.WriteRune(utf8.RuneError)
						b.WriteRune(utf8.RuneError)
						break
					}
					b.WriteRune(utf8.RuneError)
					break
				}
				b.WriteRune(r)
			default:
				return "", p.errorf("invalid escape character %q", e)
			}
		case c < 0x20:
			return "", p.errorf("unescaped control character in string")
		default:
			r, size := utf8.DecodeRuneInString(p.s[p.i:])
			if r == utf8.RuneError && size == 1 {
				b.WriteRune(utf8.RuneError)
			} else {
				b.WriteString(p.s[p.i : p.i+size])
			}
			p.i += size
		}
	}
}

func (p *parser) hex4() (rune, error) {
	if p.i+4 > len(p.s) {
		return 0, p.errorf("invalid \\u escape")
	}
	var r rune
	for _, c := range []byte(p.s[p.i : p.i+4]) {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, p.errorf("invalid \\u escape")
		}
	}
	p.i += 4
	return r, nil
}

func (p *parser) number() (Value, error) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	switch {
	case p.i < len(p.s) && p.s[p.i] == '0':
		p.i++
	case p.i < len(p.s) && p.s[p.i] >= '1' && p.s[p.i] <= '9':
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
	default:
		return nil, p.errorf("invalid number")
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		if p.i >= len(p.s) || p.s[p.i] < '0' || p.s[p.i] > '9' {
			return nil, p.errorf("invalid number")
		}
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if p.i >= len(p.s) || p.s[p.i] < '0' || p.s[p.i] > '9' {
			return nil, p.errorf("invalid number")
		}
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
	}
	return Num(p.s[start:p.i]), nil
}

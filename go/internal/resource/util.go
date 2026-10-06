package resource

import (
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
)

var resourceID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+@[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

var (
	// fieldName is a context type field name.
	fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	// parameterName is a template parameter name.
	parameterName = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	// hostName is the Agent Skills name grammar, shared by skills, agents,
	// and instance names.
	hostName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	// serverName requires at least two hyphen-separated segments so every
	// emitted MCP server name is namespaced (ADR-0037).
	serverName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)+$`)
)

// IsResourceID reports whether id is a well-formed resource identity.
func IsResourceID(id string) bool { return resourceID.MatchString(id) }

// IsHostName reports whether name satisfies the Agent Skills name grammar in
// at most 64 characters.
func IsHostName(name string) bool { return len(name) <= 64 && hostName.MatchString(name) }

// IsParameterName reports whether name is a valid template parameter name.
func IsParameterName(name string) bool { return parameterName.MatchString(name) }

// IsFieldName reports whether name is a valid context type field name.
func IsFieldName(name string) bool { return fieldName.MatchString(name) }

func stripBOM(s string) string {
	return strings.TrimPrefix(s, string(rune(0xFEFF)))
}

// NormalizeText strips a byte order mark and normalizes CRLF to LF.
func NormalizeText(s string) string {
	return stripBOM(strings.ReplaceAll(s, "\r\n", "\n"))
}

// splitFrontmatter separates a document's frontmatter from its body.
func splitFrontmatter(text string) (frontmatter, body string, err error) {
	nl := strings.IndexByte(text, '\n')
	if nl < 0 || strings.TrimRight(text[:nl], "\r") != "---" {
		return "", "", Errorf("a .tfer file must begin with a '---' frontmatter fence")
	}
	rest := text[nl+1:]
	for idx := 0; ; {
		lineEnd := strings.IndexByte(rest[idx:], '\n')
		if lineEnd < 0 {
			if strings.TrimRight(rest[idx:], "\r") == "---" {
				return rest[:idx], "", nil
			}
			return "", "", Errorf("a .tfer file is missing its closing '---' frontmatter fence")
		}
		line := rest[idx : idx+lineEnd]
		next := idx + lineEnd + 1
		if strings.TrimRight(line, "\r") == "---" {
			return rest[:idx], rest[next:], nil
		}
		idx = next
	}
}

func validateJSON(value, file, field string) error {
	if _, err := jsonx.Parse(value); err != nil {
		return Errorf("%s: invalid %s: %s", file, field, err)
	}
	return nil
}

// SortedKeys returns map keys in canonical (code point) order.
func SortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Leaf extracts the unversioned last segment of a resource identity.
func Leaf(id string) string {
	last := id[strings.LastIndex(id, "/")+1:]
	return strings.SplitN(last, "@", 2)[0]
}

// VersionOf extracts the version of a resource identity.
func VersionOf(id string) string {
	return id[strings.LastIndex(id, "@")+1:]
}

// singleLine rejects control characters and line breaks in routing and
// display strings, which are emitted into host frontmatter.
func singleLine(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return !strings.ContainsRune(value, ' ') && !strings.ContainsRune(value, ' ')
}

// cleanRelative reports whether p is a clean, relative, forward-slash path.
func cleanRelative(p string) bool {
	return p != "" && !strings.Contains(p, "\\") && !strings.HasPrefix(p, "/") &&
		!filepath.IsAbs(p) && path.Clean(p) == p && p != "." && !strings.HasPrefix(p, "../") && p != ".."
}

func distinctStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

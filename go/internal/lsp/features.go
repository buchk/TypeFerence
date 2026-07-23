package lsp

import (
	"regexp"
	"strings"
)

// topLevelFields are the resource fields offered as completions at column 0.
var topLevelFields = []string{
	"schemaVersion", "kind", "id", "displayName", "description", "binds",
	"emit", "embeds", "requiresSlots", "requiresCapabilities", "slots",
	"workingNorms", "contextFiles", "context", "skills", "instructions",
	"inputSchema", "outputSchema", "contextType", "schema",
	"requiresContextTypes", "requiresTools", "visibility", "variants",
}

// kinds are the resource kinds offered after `kind:`.
var kinds = []string{"agent", "profile", "interface", "capability", "skill", "context", "contextType", "tool"}

// idToken matches a resource identifier (namespace/name@semver).
var idToken = regexp.MustCompile(`[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+@[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?`)

// completions returns completion labels for the cursor: kind values after
// `kind:`, otherwise the top-level field names when the cursor is in a line's
// leading token (no colon yet).
func completions(text string, line, char int) []string {
	cur := lineAt(text, line)
	prefix := cur
	if char >= 0 && char <= len(cur) {
		prefix = cur[:char]
	}
	if strings.HasPrefix(strings.TrimSpace(prefix), "kind:") {
		return kinds
	}
	if !strings.Contains(prefix, ":") {
		return topLevelFields
	}
	return nil
}

func lineAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return strings.TrimRight(lines[line], "\r")
}

// tokenAt returns the resource-id token spanning the cursor, or "".
func tokenAt(text string, line, char int) string {
	cur := lineAt(text, line)
	for _, m := range idToken.FindAllStringIndex(cur, -1) {
		if char >= m[0] && char <= m[1] {
			return cur[m[0]:m[1]]
		}
	}
	return ""
}

// symbolOf extracts (id, kind) from a resource's text for a document symbol.
func symbolOf(text string) (id, kind string) {
	for _, raw := range strings.Split(text, "\n") {
		if v, ok := scalarField(raw, "id"); ok {
			id = v
		}
		if v, ok := scalarField(raw, "kind"); ok {
			kind = v
		}
	}
	return id, kind
}

func scalarField(line, name string) (string, bool) {
	t := strings.TrimSpace(strings.TrimRight(line, "\r"))
	if strings.HasPrefix(t, name+":") {
		return strings.TrimSpace(strings.TrimPrefix(t, name+":")), true
	}
	return "", false
}

func isSource(path string) bool {
	return strings.HasSuffix(path, ".tfer") || strings.HasSuffix(path, ".yaml")
}

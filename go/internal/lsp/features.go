package lsp

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// manifestKind is the completion vocabulary of the package manifest.
const manifestKind = "manifest"

// kindFields lists the fields each version 7 document kind accepts, offered
// as completions in a key position.
var kindFields = map[string][]string{
	"agent":       {"displayName", "description", "embeds", "context", "skills", "with", "copilot", "rules", "commands", "hooks", "servers"},
	"profile":     {"displayName", "description", "embeds", "parameters", "context", "skills", "rules", "commands", "hooks"},
	"capability":  {"displayName", "description", "inputSchema", "outputSchema"},
	"skill":       {"displayName", "description", "binds", "extends", "parameters", "with", "inputSchema", "outputSchema", "context", "files", "requiresServers", "variants", "copilot"},
	"server":      {"displayName", "description", "transport", "command", "args", "env", "cwd", "url", "headers"},
	"contextType": {"displayName", "description", "instanceName", "fields"},
	"context":     {"displayName", "description", "parameters", "contextType", "values"},
	"plugin":      {"description", "agents", "profiles", "skills", "modes", "rules", "commands", "hooks", "lspServers", "author", "homepage", "repository", "license", "keywords", "category", "tags", "files"},
	"rule":        {"displayName", "description", "parameters", "paths"},
	"command":     {"displayName", "description", "parameters", "argumentHint", "allowedTools", "disableModelInvocation"},
	"hook":        {"displayName", "description", "event", "matcher", "type", "bash", "powershell", "command", "exec", "args", "cwd", "env", "timeoutSec", "url", "headers", "allowedEnvVars", "prompt"},
	"lsp":         {"displayName", "description", "command", "bash", "powershell", "args", "env", "cwd", "fileExtensions", "rootUri", "initializationOptions"},
	manifestKind:  {"schemaVersion", "name", "version", "marketplace", "dependencies", "plugins", "exports"},
}

// referenceKinds maps a reference field to the document kinds it accepts.
func referenceKinds(docKind, field string) []string {
	switch field {
	case "embeds":
		if docKind == "profile" {
			return []string{"profile"}
		}
		return []string{"profile", "agent"}
	case "skills", "extends", "skill":
		return []string{"skill"}
	case "binds":
		return []string{"capability"}
	case "capability":
		return []string{"capability", "skill"}
	case "context":
		return []string{"context"}
	case "contextType":
		return []string{"contextType"}
	case "requiresServers", "servers":
		return []string{"server"}
	case "rules":
		return []string{"rule"}
	case "commands":
		return []string{"command"}
	case "hooks":
		return []string{"hook"}
	case "lspServers":
		return []string{"lsp"}
	case "agents":
		return []string{"agent"}
	case "profiles":
		return []string{"profile"}
	case "plugins":
		return []string{"plugin"}
	case "exports":
		return []string{"agent", "profile", "capability", "skill", "server", "contextType", "context", "rule", "command", "hook", "lsp"}
	}
	return nil
}

var enumValues = map[string][]string{
	"modes":                  {"manual", "pipeline"},
	"required":               {"true", "false"},
	"render":                 {"inline", "file"},
	"transport":              {"stdio", "streamable-http"},
	"event":                  {"agentStop", "errorOccurred", "notification", "permissionRequest", "postToolUse", "postToolUseFailure", "preCompact", "preToolUse", "sessionEnd", "sessionStart", "subagentStart", "subagentStop", "userPromptSubmitted", "userPromptTransformed"},
	"userInvocable":          {"true", "false"},
	"disableModelInvocation": {"true", "false"},
	"type":                   {"string", "text", "boolean", "integer", "list<string>"},
}

// documentKind derives a document's kind from its file name.
func documentKind(path string) string {
	if filepath.Base(path) == resource.ManifestFile {
		return manifestKind
	}
	kind, _, ok := resource.KindFromPath(filepath.ToSlash(filepath.Base(path)))
	if !ok {
		return ""
	}
	return kind
}

var (
	keyLine      = regexp.MustCompile(`^(\s*)([A-Za-z0-9_./-]+):(\s.*)?$`)
	sequenceLine = regexp.MustCompile(`^(\s*)-(\s.*)?$`)
)

// frontmatterLine reports whether a zero-based line lies between a
// document's opening and closing fences.
func frontmatterLine(text string, line int) bool {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" || line == 0 {
		return false
	}
	for i := 1; i < len(lines) && i <= line; i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return false
		}
	}
	return true
}

// fieldAt names the field whose value the cursor is writing: the key on the
// cursor's own line, or for a sequence item the nearest less-indented key.
func fieldAt(text string, line int, prefix string) (field string, keyPosition bool) {
	if m := keyLine.FindStringSubmatch(prefix); m != nil {
		return m[2], false
	}
	trimmed := strings.TrimSpace(prefix)
	if !sequenceLine.MatchString(prefix) {
		return "", !strings.Contains(trimmed, ":")
	}
	indent := len(prefix) - len(strings.TrimLeft(prefix, " "))
	lines := strings.Split(text, "\n")
	for i := line - 1; i >= 0; i-- {
		candidate := strings.TrimRight(lines[i], "\r")
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		candidateIndent := len(candidate) - len(strings.TrimLeft(candidate, " "))
		if m := keyLine.FindStringSubmatch(candidate); m != nil && candidateIndent <= indent && strings.TrimSpace(m[3]) == "" {
			return m[2], false
		}
		if candidateIndent < indent {
			break
		}
	}
	return "", false
}

// completions returns labels for the cursor: field names in a key position,
// enumerated values, or package-relative paths of the kinds a reference
// field accepts.
func completions(text, path, root string, line, char int) []string {
	if !frontmatterLine(text, line) {
		return nil
	}
	kind := documentKind(path)
	current := lineAt(text, line)
	prefix := current
	if char >= 0 && char <= len(current) {
		prefix = current[:char]
	}
	field, keyPosition := fieldAt(text, line, prefix)
	if keyPosition {
		if strings.HasPrefix(prefix, " ") {
			return nil
		}
		return kindFields[kind]
	}
	if values, ok := enumValues[field]; ok {
		if field == "type" {
			return append(append([]string{}, values...), documentPaths(root, []string{"contextType"})...)
		}
		return values
	}
	if kinds := referenceKinds(kind, field); kinds != nil && root != "" {
		return documentPaths(root, kinds)
	}
	return nil
}

// documentPaths lists a package's documents of the given kinds as
// package-relative paths, in canonical order.
func documentPaths(root string, kinds []string) []string {
	want := map[string]bool{}
	for _, kind := range kinds {
		want[kind] = true
	}
	paths := []string{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "dist" || name == "bin" || name == "obj" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if kind, _, ok := resource.KindFromPath(rel); ok && want[kind] {
			paths = append(paths, rel)
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func lineAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return strings.TrimRight(lines[line], "\r")
}

var pathToken = regexp.MustCompile(`[A-Za-z0-9_./:-]+\.tfer`)

// tokenAt returns the document reference spanning the cursor, or "".
func tokenAt(text string, line, char int) string {
	current := lineAt(text, line)
	for _, m := range pathToken.FindAllStringIndex(current, -1) {
		if char >= m[0] && char <= m[1] {
			return current[m[0]:m[1]]
		}
	}
	return ""
}

// packageRoot finds the version 7 package containing a file: the nearest
// ancestor directory whose typeference.tfer declares schemaVersion 7.
func packageRoot(path string) (string, *resource.Project) {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, resource.ManifestFile)); err == nil {
			if project, err := resource.LoadProject(dir); err == nil && project.IsCurrent() {
				return dir, project
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// relativeTo returns a file's package-relative slash path.
func relativeTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func isSource(path string) bool {
	return strings.HasSuffix(path, ".tfer")
}

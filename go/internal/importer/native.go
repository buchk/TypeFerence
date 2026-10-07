package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// nativeImport collects an Agent Plugin's Copilot components (ADR-0007):
// commands, rules, hooks, and language servers, as version 8 documents.
type nativeImport struct {
	files    []File
	commands []string
	rules    []string
	hooks    []string
	lsp      []string
}

func (n *nativeImport) add(path, content string, list *[]string) {
	n.files = append(n.files, File{Path: path, Content: content})
	*list = append(*list, path)
}

// copilotComponents imports the com.github.copilot components of an Agent
// Plugins 1.0 plugin.
func (im *importer) copilotComponents() error {
	base := filepath.Join(im.root, "com.github.copilot")
	if err := im.markdownComponents(filepath.Join(base, "commands"), "command"); err != nil {
		return err
	}
	if err := im.markdownComponents(filepath.Join(base, "rules"), "rule"); err != nil {
		return err
	}
	if exists(filepath.Join(base, "hooks", "hooks.json")) {
		if err := im.hookComponents(filepath.Join(base, "hooks", "hooks.json")); err != nil {
			return err
		}
	}
	if exists(filepath.Join(base, "lsp.json")) {
		if err := im.lspComponents(filepath.Join(base, "lsp.json")); err != nil {
			return err
		}
	}
	return nil
}

// markdownComponents imports command or rule Markdown files.
func (im *importer) markdownComponents(dir, kind string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			if entry.IsDir() {
				im.unsupported = append(im.unsupported, im.rel(filepath.Join(dir, entry.Name()))+": nested "+kind+" directories")
			}
			continue
		}
		path := filepath.Join(dir, entry.Name())
		values, keys, body, err := frontmatterOf(path)
		if err != nil {
			return err
		}
		source := im.rel(path)
		name := strings.TrimSuffix(entry.Name(), ".md")
		if !hostName.MatchString(name) || len(name) > 64 {
			return resource.Errorf("%s: %s name %q must use lowercase letters, digits, and single hyphens", source, kind, name)
		}
		if strings.TrimSpace(body) == "" {
			return resource.Errorf("%s: the %s has no body", source, kind)
		}
		fm := &frontmatter{}
		description := text(values, "description")
		if kind == "command" && description == "" {
			return resource.Errorf("%s: a command requires a description", source)
		}
		fm.scalar(0, "description", description)
		for _, key := range keys {
			switch {
			case key == "description":
			case kind == "rule" && (key == "paths" || key == "applyTo"):
				paths, ok := values[key].(string)
				if !ok {
					im.unsupported = append(im.unsupported, source+": '"+key+"' must be one glob string")
					continue
				}
				fm.raw(0, "paths", paths)
			case kind == "command" && key == "argument-hint":
				fm.raw(0, "argumentHint", text(values, key))
			case kind == "command" && key == "disable-model-invocation":
				if value, ok := values[key].(bool); ok {
					fm.token(0, "disableModelInvocation", strconv.FormatBool(value))
				} else {
					im.unsupported = append(im.unsupported, source+": 'disable-model-invocation' must be true or false")
				}
			case kind == "command" && key == "allowed-tools":
				tools := toolList(values[key])
				if len(tools) > 0 {
					fm.list(0, "allowedTools", tools)
				}
			default:
				im.unsupported = append(im.unsupported, source+": frontmatter field '"+key+"'")
			}
		}
		target := "commands/" + name + ".command.tfer"
		list := &im.native.commands
		if kind == "rule" {
			target = "rules/" + name + ".rule.tfer"
			list = &im.native.rules
		}
		im.native.add(target, document(fm, body), list)
	}
	return nil
}

func toolList(value any) []string {
	switch v := value.(type) {
	case string:
		return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	case []any:
		out := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// kebab turns a camelCase hook event into a document name segment.
func kebab(event string) string {
	var b strings.Builder
	for i, r := range event {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// hookComponents imports a hooks.json into one hook document per entry.
func (im *importer) hookComponents(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return resource.Errorf("Cannot read %s", path)
	}
	source := im.rel(path)
	var config struct {
		Version int                         `json:"version"`
		Hooks   map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return resource.Errorf("%s: invalid JSON: %s", source, err)
	}
	if config.Version != 1 {
		return resource.Errorf("%s: only hooks configuration version 1 can be imported", source)
	}
	events := make([]string, 0, len(config.Hooks))
	for event := range config.Hooks {
		events = append(events, event)
	}
	sort.Strings(events)
	for _, event := range events {
		for i, entry := range config.Hooks[event] {
			fm := &frontmatter{}
			fm.raw(0, "event", event)
			ok := true
			for _, key := range sortedKeys(entry) {
				switch value := entry[key].(type) {
				case string:
					switch key {
					case "matcher", "type", "bash", "powershell", "command", "exec", "cwd", "url", "prompt":
						fm.raw(0, key, value)
					default:
						ok = false
					}
				case float64:
					if key == "timeoutSec" || key == "timeout" {
						fm.token(0, "timeoutSec", strconv.FormatInt(int64(value), 10))
					} else {
						ok = false
					}
				case []any:
					if key == "args" || key == "allowedEnvVars" {
						fm.list(0, key, toolList(value))
					} else {
						ok = false
					}
				case map[string]any:
					if key == "env" || key == "headers" {
						fm.key(0, key)
						for _, k := range sortedKeys(value) {
							if s, isString := value[k].(string); isString {
								fm.raw(2, k, s)
							}
						}
					} else {
						ok = false
					}
				default:
					ok = false
				}
				if !ok {
					im.unsupported = append(im.unsupported, source+": hook "+event+"["+strconv.Itoa(i)+"] field '"+key+"'")
					break
				}
			}
			if !ok {
				continue
			}
			name := "hooks/" + kebab(event) + "-" + strconv.Itoa(i+1) + ".hook.tfer"
			im.native.add(name, document(fm, ""), &im.native.hooks)
		}
	}
	return nil
}

// lspComponents imports an lsp.json into one LSP document per server.
func (im *importer) lspComponents(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return resource.Errorf("Cannot read %s", path)
	}
	source := im.rel(path)
	var config struct {
		LSPServers map[string]map[string]json.RawMessage `json:"lspServers"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return resource.Errorf("%s: invalid JSON: %s", source, err)
	}
	for _, name := range sortedKeys(config.LSPServers) {
		if !hostName.MatchString(name) || len(name) > 64 {
			return resource.Errorf("%s: LSP server name %q must use lowercase letters, digits, and single hyphens", source, name)
		}
		entry := config.LSPServers[name]
		fm := &frontmatter{}
		for _, key := range sortedKeys(entry) {
			value := entry[key]
			switch key {
			case "command", "bash", "powershell", "cwd", "rootUri":
				var s string
				if json.Unmarshal(value, &s) != nil {
					im.unsupported = append(im.unsupported, source+": "+name+" '"+key+"' must be a string")
					continue
				}
				fm.raw(0, key, s)
			case "args":
				var items []string
				if json.Unmarshal(value, &items) != nil {
					im.unsupported = append(im.unsupported, source+": "+name+" 'args' must list strings")
					continue
				}
				fm.list(0, "args", items)
			case "env", "fileExtensions":
				var values map[string]string
				if json.Unmarshal(value, &values) != nil {
					im.unsupported = append(im.unsupported, source+": "+name+" '"+key+"' must map strings to strings")
					continue
				}
				fm.key(0, key)
				for _, k := range sortedKeys(values) {
					written := k
					if key == "fileExtensions" {
						written = strings.TrimPrefix(k, ".")
					}
					fm.raw(2, written, values[k])
				}
			case "initializationOptions":
				fm.raw(0, key, string(value))
			default:
				im.unsupported = append(im.unsupported, source+": "+name+" field '"+key+"'")
			}
		}
		im.native.add("lsp/"+name+".lsp.tfer", document(fm, ""), &im.native.lsp)
	}
	return nil
}

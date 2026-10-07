package compile

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// Native Copilot components live under the client namespace directory
// (ADR-0007).
const copilotDir = "com.github.copilot"

// nativeSet collects the rules, commands, hooks, and language servers one
// plugin ships, one per emitted name.
type nativeSet struct {
	where    string
	rules    map[string]resolve.ResolvedRule
	commands map[string]resolve.ResolvedCommand
	hooks    map[string]bool
	lsp      map[string]bool
}

func newNativeSet(where string) *nativeSet {
	return &nativeSet{
		where: where, rules: map[string]resolve.ResolvedRule{}, commands: map[string]resolve.ResolvedCommand{},
		hooks: map[string]bool{}, lsp: map[string]bool{},
	}
}

func (n *nativeSet) addRule(rule resolve.ResolvedRule) error {
	if prior, exists := n.rules[rule.Name]; exists && prior.Key() != rule.Key() {
		return resource.Errorf("%s: %s and %s both emit the rule name '%s'", n.where, prior.Key(), rule.Key(), rule.Name)
	}
	n.rules[rule.Name] = rule
	return nil
}

func (n *nativeSet) addCommand(command resolve.ResolvedCommand) error {
	if prior, exists := n.commands[command.Name]; exists && prior.Key() != command.Key() {
		return resource.Errorf("%s: %s and %s both emit the command name '%s'", n.where, prior.Key(), command.Key(), command.Name)
	}
	n.commands[command.Name] = command
	return nil
}

func (n *nativeSet) addHooks(ids []string) {
	for _, id := range ids {
		n.hooks[id] = true
	}
}

// planNative collects the native components a plugin's composition holds,
// one per emitted name.
func planNative(doc *resource.Document, plan *pluginPlan, resolved *resolve.ResolvedPlugin) error {
	set := newNativeSet(doc.Path)
	for _, rule := range resolved.Rules {
		if err := set.addRule(rule); err != nil {
			return err
		}
	}
	for _, command := range resolved.Commands {
		if err := set.addCommand(command); err != nil {
			return err
		}
	}
	set.addHooks(resolved.Hooks)
	for _, id := range resolved.LSP {
		set.lsp[id] = true
	}
	skillNames := map[string]bool{}
	for _, skill := range plan.Skills {
		skillNames[skill.Name] = true
	}
	for _, name := range resource.SortedKeys(set.commands) {
		if skillNames[name] {
			return resource.Errorf("%s: command '%s' has the name of a skill the plugin ships; Copilot lets the skill hide the command, so rename one", doc.Path, name)
		}
		plan.Commands = append(plan.Commands, set.commands[name])
	}
	for _, name := range resource.SortedKeys(set.rules) {
		plan.Rules = append(plan.Rules, set.rules[name])
	}
	plan.Hooks = resource.SortedKeys(set.hooks)
	plan.LSP = resource.SortedKeys(set.lsp)
	return nil
}

// writeNative emits an artifact's rules, commands, hooks, and language
// servers.
func (c *compilation) writeNative(dir string, artifact pluginArtifact, written *[]string) error {
	plan := artifact.plan
	for _, rule := range plan.Rules {
		if err := writeFile(filepath.Join(dir, copilotDir, "rules", rule.Name+".md"), renderRule(rule), written); err != nil {
			return err
		}
	}
	for _, command := range plan.Commands {
		if err := writeFile(filepath.Join(dir, copilotDir, "commands", command.Name+".md"), renderCommand(command), written); err != nil {
			return err
		}
	}
	if len(plan.Hooks) > 0 {
		if err := writeFile(filepath.Join(dir, copilotDir, "hooks", "hooks.json"), hooksJSON(c.docs, plan.Hooks)+"\n", written); err != nil {
			return err
		}
	}
	if len(plan.LSP) > 0 {
		value, err := lspJSON(c.docs, plan.LSP)
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, copilotDir, "lsp.json"), value+"\n", written); err != nil {
			return err
		}
	}
	return nil
}

// renderRule renders a Copilot rule: frontmatter only when the rule declares
// a description or a path scope, then its Markdown.
func renderRule(rule resolve.ResolvedRule) string {
	var b strings.Builder
	if rule.Description != "" || rule.Paths != "" {
		b.WriteString("---\n")
		if rule.Paths != "" {
			b.WriteString("paths: " + escapeYAML(rule.Paths) + "\n")
		}
		if rule.Description != "" {
			b.WriteString("description: " + escapeYAML(rule.Description) + "\n")
		}
		b.WriteString("---\n\n")
	}
	b.WriteString(strings.TrimSpace(rule.Body) + "\n")
	return b.String()
}

// renderCommand renders a Copilot slash command.
func renderCommand(command resolve.ResolvedCommand) string {
	var b strings.Builder
	b.WriteString("---\ndescription: " + escapeYAML(command.Description) + "\n")
	if command.ArgumentHint != nil {
		b.WriteString("argument-hint: " + escapeYAML(*command.ArgumentHint) + "\n")
	}
	if command.AllowedTools != nil {
		b.WriteString("allowed-tools:\n")
		for _, tool := range command.AllowedTools {
			b.WriteString("  - " + escapeYAML(tool) + "\n")
		}
	}
	if command.DisableModelInvocation != nil {
		value := "false"
		if *command.DisableModelInvocation {
			value = "true"
		}
		b.WriteString("disable-model-invocation: " + value + "\n")
	}
	b.WriteString("---\n\n" + strings.TrimSpace(command.Body) + "\n")
	return b.String()
}

func stringMapValue(values map[string]string) jsonx.Obj {
	obj := jsonx.Obj{}
	for _, key := range resource.SortedKeys(values) {
		obj = append(obj, jsonx.Member{K: key, V: jsonx.Str(values[key])})
	}
	return obj
}

// hooksJSON renders the Copilot hooks configuration: events in canonical
// order, and each event's entries in hook identity order.
func hooksJSON(docs map[string]*resource.Document, ids []string) string {
	byEvent := map[string][]string{}
	for _, id := range ids {
		event := docs[id].Hook.Event
		byEvent[event] = append(byEvent[event], id)
	}
	events := jsonx.Obj{}
	for _, event := range resource.SortedKeys(byEvent) {
		entries := jsonx.Arr{}
		hookIDs := byEvent[event]
		sort.Strings(hookIDs)
		for _, id := range hookIDs {
			entries = append(entries, hookValue(docs[id].Hook))
		}
		events = append(events, jsonx.Member{K: event, V: entries})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "version", V: jsonx.Num("1")},
		{K: "hooks", V: events},
	})
}

func hookValue(h *resource.HookConfig) jsonx.Obj {
	obj := jsonx.Obj{{K: "type", V: jsonx.Str(h.Type)}}
	str := func(key, value string) {
		if value != "" {
			obj = append(obj, jsonx.Member{K: key, V: jsonx.Str(value)})
		}
	}
	str("matcher", h.Matcher)
	switch h.Type {
	case "command":
		str("bash", h.Bash)
		str("powershell", h.PowerShell)
		str("command", h.Command)
		str("exec", h.Exec)
		if h.Args != nil {
			obj = append(obj, jsonx.Member{K: "args", V: stringArr(h.Args)})
		}
		str("cwd", h.Cwd)
		if h.Env != nil {
			obj = append(obj, jsonx.Member{K: "env", V: stringMapValue(h.Env)})
		}
	case "http":
		str("url", h.URL)
		if h.Headers != nil {
			obj = append(obj, jsonx.Member{K: "headers", V: stringMapValue(h.Headers)})
		}
		if h.AllowedEnvVars != nil {
			obj = append(obj, jsonx.Member{K: "allowedEnvVars", V: stringArr(h.AllowedEnvVars)})
		}
	case "prompt":
		str("prompt", h.Prompt)
	}
	if h.TimeoutSec != "" {
		obj = append(obj, jsonx.Member{K: "timeoutSec", V: jsonx.Num(h.TimeoutSec)})
	}
	return obj
}

// lspJSON renders the Copilot language server configuration, keyed by
// server name.
func lspJSON(docs map[string]*resource.Document, ids []string) (string, error) {
	byName := map[string]*resource.Document{}
	for _, id := range ids {
		byName[resource.Leaf(id)] = docs[id]
	}
	servers := jsonx.Obj{}
	for _, name := range resource.SortedKeys(byName) {
		l := byName[name].LSP
		obj := jsonx.Obj{}
		str := func(key, value string) {
			if value != "" {
				obj = append(obj, jsonx.Member{K: key, V: jsonx.Str(value)})
			}
		}
		str("command", l.Command)
		str("bash", l.Bash)
		str("powershell", l.PowerShell)
		if l.Args != nil {
			obj = append(obj, jsonx.Member{K: "args", V: stringArr(l.Args)})
		}
		str("cwd", l.Cwd)
		if l.Env != nil {
			obj = append(obj, jsonx.Member{K: "env", V: stringMapValue(l.Env)})
		}
		extensions := jsonx.Obj{}
		for _, ext := range resource.SortedKeys(l.FileExtensions) {
			extensions = append(extensions, jsonx.Member{K: "." + ext, V: jsonx.Str(l.FileExtensions[ext])})
		}
		obj = append(obj, jsonx.Member{K: "fileExtensions", V: extensions})
		str("rootUri", l.RootURI)
		if l.InitializationOptions != "" {
			value, err := jsonx.Parse(l.InitializationOptions)
			if err != nil {
				return "", resource.Errorf("%s: initializationOptions must be a JSON document", byName[name].Path)
			}
			obj = append(obj, jsonx.Member{K: "initializationOptions", V: value})
		}
		servers = append(servers, jsonx.Member{K: name, V: obj})
	}
	return jsonx.Indented(jsonx.Obj{{K: "lspServers", V: servers}}), nil
}

// agentServersFrontmatter renders an agent's own mcp-servers block, which
// scopes those servers to the agent (ADR-0007).
func agentServersFrontmatter(b *strings.Builder, docs map[string]*resource.Document, ids []string) {
	if len(ids) == 0 {
		return
	}
	byName := map[string]*resource.Document{}
	for _, id := range ids {
		byName[resource.Leaf(id)] = docs[id]
	}
	b.WriteString("mcp-servers:\n")
	for _, name := range resource.SortedKeys(byName) {
		s := byName[name].Server
		b.WriteString("  " + name + ":\n")
		list := func(key string, values []string) {
			b.WriteString("    " + key + ":\n")
			for _, value := range values {
				b.WriteString("      - " + escapeYAML(value) + "\n")
			}
		}
		mapping := func(key string, values map[string]string) {
			b.WriteString("    " + key + ":\n")
			for _, k := range resource.SortedKeys(values) {
				b.WriteString("      " + escapeYAML(k) + ": " + escapeYAML(values[k]) + "\n")
			}
		}
		if s.Transport == "stdio" {
			b.WriteString("    type: \"stdio\"\n")
			b.WriteString("    command: " + escapeYAML(s.Command) + "\n")
			if len(s.Args) > 0 {
				list("args", s.Args)
			}
			if len(s.Env) > 0 {
				mapping("env", s.Env)
			}
			if s.Cwd != "" {
				b.WriteString("    cwd: " + escapeYAML(s.Cwd) + "\n")
			}
		} else {
			b.WriteString("    type: \"http\"\n")
			b.WriteString("    url: " + escapeYAML(s.URL) + "\n")
			if len(s.Headers) > 0 {
				mapping("headers", s.Headers)
			}
		}
		list("tools", []string{"*"})
	}
}

func rulesValue(rules []resolve.ResolvedRule) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, rule := range rules {
		arr = append(arr, jsonx.Obj{
			{K: "name", V: jsonx.Str(rule.Name)},
			{K: "id", V: jsonx.Str(rule.ID)},
			{K: "templateId", V: jsonx.Str(rule.TemplateID)},
			{K: "bindings", V: bindingsValue(rule.Bindings)},
		})
	}
	return arr
}

func commandsValue(commands []resolve.ResolvedCommand) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, command := range commands {
		arr = append(arr, jsonx.Obj{
			{K: "name", V: jsonx.Str(command.Name)},
			{K: "id", V: jsonx.Str(command.ID)},
			{K: "templateId", V: jsonx.Str(command.TemplateID)},
			{K: "bindings", V: bindingsValue(command.Bindings)},
		})
	}
	return arr
}

package resolve

import (
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// ResolvedRule is a Copilot rule as it ships: always-on guidance that applies
// wherever the plugin is active (ADR-0007).
type ResolvedRule struct {
	Name        string
	ID          string
	TemplateID  string
	Description string
	Paths       string
	Body        string
	Bindings    []*Binding
}

// Key identifies a rule's emitted content.
func (r ResolvedRule) Key() string { return contentKey(r.ID, r.Bindings) }

// ResolvedCommand is a Copilot slash command as it ships.
type ResolvedCommand struct {
	Name                   string
	ID                     string
	TemplateID             string
	Description            string
	Body                   string
	ArgumentHint           *string
	AllowedTools           []string
	DisableModelInvocation *bool
	Bindings               []*Binding
}

// Key identifies a command's emitted content.
func (c ResolvedCommand) Key() string { return contentKey(c.ID, c.Bindings) }

func contentKey(id string, bindings []*Binding) string {
	parts := []string{id}
	for _, b := range bindings {
		parts = append(parts, b.Name+"="+b.DataID)
	}
	return strings.Join(parts, "|")
}

// componentBindings selects the bindings a rule or command needs from the
// bindings available to it, failing on any it lacks.
func (r *Resolver) componentBindings(doc *resource.Document, available map[string]*Binding) (map[string]*Binding, error) {
	bindings := map[string]*Binding{}
	for _, name := range resource.SortedKeys(doc.Parameters) {
		b, ok := available[name]
		if !ok {
			return nil, resource.Errorf("%s: parameter '%s' is unbound; a %s with parameters ships only as an instance, through a plugin that binds them", doc.Path, name, doc.Kind)
		}
		bindings[name] = b
	}
	return bindings, nil
}

func componentName(doc *resource.Document, instanceName string) (string, string, error) {
	name := resource.Leaf(doc.ID)
	if len(doc.Parameters) == 0 {
		return name, "", nil
	}
	if instanceName == "" {
		return "", "", resource.Errorf("%s: a template %s needs an instance name from its plugin's data", doc.Path, doc.Kind)
	}
	return instanceName + "-" + name, doc.ID, nil
}

func sortedBindings(bindings map[string]*Binding) []*Binding {
	out := []*Binding{}
	for _, name := range resource.SortedKeys(bindings) {
		out = append(out, bindings[name])
	}
	return out
}

// resolveRule renders a rule with the bindings available to it.
func (r *Resolver) resolveRule(id string, available map[string]*Binding, instanceName string) (ResolvedRule, error) {
	doc := r.docs[id]
	bindings, err := r.componentBindings(doc, available)
	if err != nil {
		return ResolvedRule{}, err
	}
	name, templateID, err := componentName(doc, instanceName)
	if err != nil {
		return ResolvedRule{}, err
	}
	template := len(doc.Parameters) > 0
	description, err := r.render(doc.Description, doc.Path+" description", template, bindings)
	if err != nil {
		return ResolvedRule{}, err
	}
	if !singleLine(description) {
		return ResolvedRule{}, resource.Errorf("%s: description must stay a single line after field references are resolved", doc.Path)
	}
	body, err := r.render(doc.Body, doc.Path, template, bindings)
	if err != nil {
		return ResolvedRule{}, err
	}
	return ResolvedRule{
		Name: name, ID: id, TemplateID: templateID, Description: description,
		Paths: doc.RulePaths, Body: body, Bindings: sortedBindings(bindings),
	}, nil
}

// resolveCommand renders a command with the bindings available to it.
func (r *Resolver) resolveCommand(id string, available map[string]*Binding, instanceName string) (ResolvedCommand, error) {
	doc := r.docs[id]
	bindings, err := r.componentBindings(doc, available)
	if err != nil {
		return ResolvedCommand{}, err
	}
	name, templateID, err := componentName(doc, instanceName)
	if err != nil {
		return ResolvedCommand{}, err
	}
	template := len(doc.Parameters) > 0
	description, err := r.render(doc.Description, doc.Path+" description", template, bindings)
	if err != nil {
		return ResolvedCommand{}, err
	}
	if len([]rune(description)) > 1024 || !singleLine(description) {
		return ResolvedCommand{}, resource.Errorf("%s: description must stay a single line of at most 1024 characters after field references are resolved", doc.Path)
	}
	body, err := r.render(doc.Body, doc.Path, template, bindings)
	if err != nil {
		return ResolvedCommand{}, err
	}
	return ResolvedCommand{
		Name: name, ID: id, TemplateID: templateID, Description: description, Body: body,
		ArgumentHint: doc.ArgumentHint, AllowedTools: doc.AllowedTools,
		DisableModelInvocation: doc.DisableModelInvocation, Bindings: sortedBindings(bindings),
	}, nil
}

// validateNative checks the native component references of one document.
func (r *Resolver) validateNative(doc *resource.Document) error {
	check := func(ids []string, kind string) error {
		for _, id := range ids {
			if r.kindOf(id) != kind {
				return resource.Errorf("%s: references %s, which is not a %s in this build", doc.Path, id, kind)
			}
		}
		return nil
	}
	if err := check(doc.Rules, "rule"); err != nil {
		return err
	}
	if err := check(doc.Commands, "command"); err != nil {
		return err
	}
	if err := check(doc.Hooks, "hook"); err != nil {
		return err
	}
	if err := check(doc.LSPServers, "lsp"); err != nil {
		return err
	}
	if err := check(doc.Servers, "server"); err != nil {
		return err
	}
	// An agent-scoped server's frontmatter expands only ${PLUGIN_ROOT}.
	for _, id := range doc.Servers {
		s := r.docs[id].Server
		values := append([]string{s.Command, s.Cwd}, s.Args...)
		for _, key := range resource.SortedKeys(s.Env) {
			values = append(values, s.Env[key])
		}
		for _, value := range values {
			if strings.Contains(value, "${PLUGIN_DATA}") {
				return resource.Errorf("%s: server %s uses ${PLUGIN_DATA}, which Copilot does not expand in an agent's own mcp-servers; ship it in the plugin's mcp.json through a skill's requiresServers instead", doc.Path, id)
			}
		}
	}
	if doc.Kind == "rule" || doc.Kind == "command" {
		if err := r.checkReferences(doc.Description, doc.Path+" description", doc.Parameters); err != nil {
			return err
		}
		if err := r.checkReferences(doc.Body, doc.Path, doc.Parameters); err != nil {
			return err
		}
	}
	return nil
}

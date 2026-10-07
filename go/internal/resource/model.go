// Package resource loads and validates version 7 TypeFerence source packages
// (docs/specification.md).
package resource

import "fmt"

// Document is one version 7 source document as authored, after reference
// paths have been converted to identities.
type Document struct {
	Kind        string
	ID          string
	Path        string
	Package     string
	DisplayName string
	Description string

	// Agents and profiles.
	Embeds []string
	Skills []SkillBinding
	// Objectives is an agent's body.
	Objectives string

	// Agents, profiles, and skills hold documents.
	Context []ContextRef

	// Profiles, skills, and documents declare parameters: name to context
	// type identity (ADR-0006).
	Parameters map[string]string
	// OwnParameters records the parameters a skill declared itself, before
	// its extension chain was flattened.
	OwnParameters map[string]string
	// Agents and skills bind parameters: name to data identity.
	With map[string]string
	// OwnWith records the bindings a skill declared itself.
	OwnWith map[string]string

	// Capabilities and skills.
	Binds           string
	Extends         string
	InputSchema     string
	OutputSchema    string
	HasInputSchema  bool
	HasOutputSchema bool
	// ImpliedCapability marks a skill whose capability is the skill itself
	// (or the root of its extension chain).
	ImpliedCapability bool
	Flattened         bool

	// Skills.
	Instructions    string
	Variants        map[string]Variant
	Files           []PackageFile
	RequiresServers []string
	Copilot         CopilotFields

	// Servers (ADR-0007).
	Server *ServerConfig

	// Context types (ADR-0006).
	Fields       []ContextField
	InstanceName string

	// Contexts: a document (Content) or data (ContextType and Values).
	ContextType string
	Values      map[string]FieldValue
	Content     string

	// Plugins link; they do not compose.
	PluginAgents   []string
	PluginProfiles []string
	PluginSkills   []string
	PluginModes    []string
	PluginLSP      []string
	// PluginMetadata is the plugin's descriptive manifest members
	// (ADR-0007). A plugin's Files ship at the root of each artifact (ADR-0004).
	PluginMetadata PluginMetadata

	// Native Copilot components (ADR-0007). Agents, profiles, and plugins
	// hold rules, commands, and hooks; agents hold agent-scoped servers.
	Rules    []string
	Commands []string
	Hooks    []string
	Servers  []string

	// Rules: an optional path glob scoping the rule to matching files.
	RulePaths string

	// Commands: Copilot command frontmatter.
	ArgumentHint           *string
	AllowedTools           []string
	DisableModelInvocation *bool
	// Body is a rule's or command's Markdown body.
	Body string

	// Hooks.
	Hook *HookConfig

	// LSP servers.
	LSP *LSPConfig
}

// HookConfig is one Copilot hook entry and the event it handles.
type HookConfig struct {
	Event          string
	Matcher        string
	Type           string
	Bash           string
	PowerShell     string
	Command        string
	Exec           string
	Args           []string
	Cwd            string
	Env            map[string]string
	TimeoutSec     string
	URL            string
	Headers        map[string]string
	AllowedEnvVars []string
	Prompt         string
}

// LSPConfig is one Copilot language server.
type LSPConfig struct {
	Command               string
	Bash                  string
	PowerShell            string
	Args                  []string
	Env                   map[string]string
	Cwd                   string
	FileExtensions        map[string]string
	RootURI               string
	InitializationOptions string
}

// IsData reports whether a context document is typed data rather than a
// Markdown document.
func (d *Document) IsData() bool { return d.Kind == "context" && d.ContextType != "" }

// IsTemplate reports whether a skill or document still has unbound
// parameters after its extension chain is flattened.
func (d *Document) IsTemplate() bool {
	for name := range d.Parameters {
		if _, bound := d.With[name]; !bound {
			return true
		}
	}
	return false
}

// SkillBinding attaches a skill implementation, or an abstract capability
// requirement, to an agent or profile.
type SkillBinding struct {
	Ref        string
	Capability *string
	Required   bool
}

// ContextRef is a held document and how a skill renders it.
type ContextRef struct {
	ID string
	// Render is "inline" (the default) or "file".
	Render string
}

// PluginMetadata is the optional descriptive part of a plugin's Agent
// Plugins manifest. Empty strings and nil values are absent members.
type PluginMetadata struct {
	Author     *PluginAuthor
	Homepage   string
	Repository string
	License    string
	Keywords   []string
}

// PluginAuthor is a plugin manifest's author.
type PluginAuthor struct {
	Name  string
	Email string
	URL   string
}

// PackageFile is a package file that ships as authored: in a skill
// directory, at a plugin artifact's root, or at the target root (ADR-0004).
type PackageFile struct {
	// Source is the package-relative path of the file.
	Source string
	// Package is the package that contains the file.
	Package string
	// As is the destination relative to the skill directory, the artifact
	// directory, or the target root.
	As string
	// Data holds the file's bytes: normalized text when Text is set, the
	// exact bytes otherwise.
	Data []byte
	Text bool
}

// Variant is a mode-specific rendering of a multimodal skill.
type Variant struct {
	Instructions    string
	RequiresServers []string
}

// ServerConfig is one MCP server declaration.
type ServerConfig struct {
	Transport string
	Command   string
	Args      []string
	Env       map[string]string
	Cwd       string
	URL       string
	Headers   map[string]string
}

// CopilotFields are the opt-in Copilot-only frontmatter fields of a skill or
// agent (ADR-0007). Nil pointers and nil slices are absent fields.
type CopilotFields struct {
	ArgumentHint           *string
	UserInvocable          *bool
	DisableModelInvocation *bool
	AllowedTools           []string
	Model                  *string
	Tools                  []string
}

// Empty reports whether no Copilot field is set.
func (c CopilotFields) Empty() bool {
	return c.ArgumentHint == nil && c.UserInvocable == nil && c.DisableModelInvocation == nil &&
		c.AllowedTools == nil && c.Model == nil && c.Tools == nil
}

// Overlay returns base with every field the receiver sets replacing base's.
func (c CopilotFields) Overlay(base CopilotFields) CopilotFields {
	out := base
	if c.ArgumentHint != nil {
		out.ArgumentHint = c.ArgumentHint
	}
	if c.UserInvocable != nil {
		out.UserInvocable = c.UserInvocable
	}
	if c.DisableModelInvocation != nil {
		out.DisableModelInvocation = c.DisableModelInvocation
	}
	if c.AllowedTools != nil {
		out.AllowedTools = append([]string{}, c.AllowedTools...)
	}
	if c.Model != nil {
		out.Model = c.Model
	}
	if c.Tools != nil {
		out.Tools = append([]string{}, c.Tools...)
	}
	return out
}

// ContextField declares one field of a context type, in authored order.
type ContextField struct {
	Name        string
	Type        string
	Required    bool
	HasDefault  bool
	Default     FieldValue
	DisplayName string
	Description string
	Choices     []string
}

// FieldValue is a data value as authored, before its declared type decides
// what its scalars mean (schema-directed typing).
type FieldValue struct {
	// Kind is "scalar", "list", "map", or "null".
	Kind   string
	Scalar string
	List   []FieldValue
	// Quoted records a quoted or block scalar, which is always a string.
	Quoted bool
	Line   int
}

// Error is a diagnostic failure: one human-readable message describing what
// to fix.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// Errorf builds an *Error.
func Errorf(format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...)}
}

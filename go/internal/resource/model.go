// Package resource loads and validates TypeFerence typed resources
// (docs/specification.md, schemaVersion 3) from a source directory.
package resource

import "fmt"

// Document is one typed YAML resource as authored.
type Document struct {
	SchemaVersion        int
	Kind                 string
	ID                   string
	DisplayName          string
	Description          string
	Binds                string
	Emit                 bool
	Embeds               []string
	RequiresSlots        []string
	RequiresCapabilities []string
	Slots                map[string]string
	WorkingNorms         []string
	ContextFiles         []string
	Skills               []SkillBinding
	Instructions         string
	InputSchema          string
	OutputSchema         string
	// ContextType is the id of the contextType a `kind: context` object
	// instantiates (ADR-0013).
	ContextType string
	// Schema is an optional JSON Schema over a `kind: contextType`'s
	// frontmatter (ADR-0013).
	Schema string
	// Content is a `.tfer` markdown body materialized onto a resource: a
	// skill's instructions or a context object's content (ADR-0013 format).
	Content string
	// RequiresContextTypes are contextType ids a skill needs; the holding
	// agent must supply context satisfying each (ADR-0013).
	RequiresContextTypes []string
	// Context are context-object ids an agent or profile holds by reference
	// rather than by path (ADR-0013 reference-by-id).
	Context []string
	// RequiresTools are tool ids a skill depends on; each must be declared and
	// its interface shape-checked (ADR-0017).
	RequiresTools []string
	// Visibility is "internal" (default) or "exposed" for a capability
	// (ADR-0015). Empty means internal.
	Visibility string
}

// SkillBinding attaches a skill implementation (and optionally the capability
// it must satisfy) to an agent or profile.
type SkillBinding struct {
	Ref        string
	Capability *string
}

// NewDocument returns a Document carrying the spec-defined defaults.
func NewDocument() *Document {
	return &Document{
		Emit:         true,
		Slots:        map[string]string{},
		InputSchema:  `{"type":"object","additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`,
	}
}

// Error is a diagnostic failure. It mirrors the reference implementation's
// TypeFerenceException: one human-readable message describing what to fix.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// Errorf builds an *Error.
func Errorf(format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...)}
}

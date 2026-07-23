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
	// ContextFields holds a `kind: context` object's schema-typed frontmatter
	// fields (those beyond the standard keys). Scalars keep their string value;
	// sequences and mappings are recorded as present with an empty value. The
	// declaring contextType's schema validates these (ADR-0013).
	ContextFields map[string]string
	// RequiresContextTypes are contextType ids a skill needs; the holding
	// agent must supply context satisfying each (ADR-0013).
	RequiresContextTypes []string
	// Context are context-object ids an agent or profile holds by reference
	// rather than by path (ADR-0013 reference-by-id).
	Context []string
	// AllowedContextTypes whitelists the contextType ids a component (and
	// anything embedding it) may hold; empty means unrestricted. Intersects
	// through embeds — the most restrictive ancestor wins (ADR-0013).
	AllowedContextTypes []string
	// RequiresTools are tool ids a skill depends on; each must be declared and
	// its interface shape-checked (ADR-0017).
	RequiresTools []string
	// Visibility is "internal" (default) or "exposed" for a capability
	// (ADR-0015). Empty means internal.
	Visibility string
	// Variants holds mode-specific renderings for a multimodal skill
	// (ADR-0012): mode name -> variant. A skill declares either Instructions
	// or Variants, never both.
	Variants map[string]Variant
}

// SkillBinding attaches a skill implementation (and optionally the capability
// it must satisfy) to an agent or profile. Sealed marks the binding as
// non-overridable by embedders; Required marks it mandatory (ADR-0016).
type SkillBinding struct {
	Ref        string
	Capability *string
	Sealed     bool
	Required   bool
}

// Variant is a mode-specific rendering of a multimodal skill (ADR-0012). It
// varies instructions and may narrow requirements; the capability contract
// (schemas) is invariant.
type Variant struct {
	Instructions         string
	RequiresContextTypes []string
	RequiresTools        []string
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

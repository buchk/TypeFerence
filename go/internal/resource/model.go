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

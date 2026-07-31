package resolve

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

func TestContextMissingRequiredFieldRejected(t *testing.T) {
	ct := doc("contextType", "t/ct/cast@1.0.0", func(d *resource.Document) {
		d.Schema = `{"type":"object","required":["role"]}`
	})
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/cast@1.0.0" })
	_, err := New(docSet(ct, obj)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "missing required field") {
		t.Fatalf("expected a missing-required-field error, got %v", err)
	}
}

func TestContextRequiredFieldPresent(t *testing.T) {
	ct := doc("contextType", "t/ct/cast@1.0.0", func(d *resource.Document) {
		d.Schema = `{"type":"object","required":["role"]}`
	})
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) {
		d.ContextType = "t/ct/cast@1.0.0"
		d.ContextFields = map[string]resource.FieldValue{"role": {Kind: "scalar", Scalar: "owner"}}
	})
	if _, err := New(docSet(ct, obj)).ResolveAll(); err != nil {
		t.Fatalf("a present required field should validate: %v", err)
	}
}

func TestContextFieldTypeMismatchRejected(t *testing.T) {
	ct := doc("contextType", "t/ct/cast@1.0.0", func(d *resource.Document) {
		d.Schema = `{"type":"object","properties":{"tags":{"type":"array"}}}`
	})
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) {
		d.ContextType = "t/ct/cast@1.0.0"
		d.ContextFields = map[string]resource.FieldValue{"tags": {Kind: "scalar", Scalar: "not-an-array"}}
	})
	_, err := New(docSet(ct, obj)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "must be array") {
		t.Fatalf("a scalar in an array-typed field must be rejected, got %v", err)
	}
}

func TestContextFieldTypeMatchAccepted(t *testing.T) {
	ct := doc("contextType", "t/ct/cast@1.0.0", func(d *resource.Document) {
		d.Schema = `{"type":"object","properties":{"tags":{"type":"array"},"owner":{"type":"string"}}}`
	})
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) {
		d.ContextType = "t/ct/cast@1.0.0"
		d.ContextFields = map[string]resource.FieldValue{
			"tags":  {Kind: "sequence"},
			"owner": {Kind: "scalar", Scalar: "Dana"},
		}
	})
	if _, err := New(docSet(ct, obj)).ResolveAll(); err != nil {
		t.Fatalf("matching field types should validate: %v", err)
	}
}

func TestRequiredFieldFromRefinedTypeEnforced(t *testing.T) {
	base := doc("contextType", "t/ct/cast@1.0.0", func(d *resource.Document) {
		d.Schema = `{"type":"object","required":["role"]}`
	})
	gov := doc("contextType", "t/ct/gov@1.0.0", func(d *resource.Document) { d.Embeds = []string{"t/ct/cast@1.0.0"} })
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/gov@1.0.0" })
	_, err := New(docSet(base, gov, obj)).ResolveAll()
	if err == nil || !strings.Contains(err.Error(), "missing required field") {
		t.Fatalf("a required field from a refined type must be enforced, got %v", err)
	}
}

func TestSchemalessContextTypeImposesNoFields(t *testing.T) {
	ct := doc("contextType", "t/ct/cast@1.0.0", nil) // no schema
	obj := doc("context", "t/notes/n@1.0.0", func(d *resource.Document) { d.ContextType = "t/ct/cast@1.0.0" })
	if _, err := New(docSet(ct, obj)).ResolveAll(); err != nil {
		t.Fatalf("a schemaless contextType should impose no field requirements: %v", err)
	}
}

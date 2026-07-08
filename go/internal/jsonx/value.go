// Package jsonx serializes JSON with the exact byte layout of the reference
// implementation (System.Text.Json with the default encoder), as required by
// the canonical serialization rules in docs/specification.md. The Go standard
// library encoder cannot produce this layout (different escape set, no control
// over member order), so compiled artifacts are built from the Value model in
// this package instead.
package jsonx

// Value is a JSON value that preserves everything the canonical form cares
// about: member order, duplicate keys, and raw number tokens.
type Value interface{ isValue() }

// Str is a JSON string.
type Str string

// Num is a JSON number kept as its raw source token. Canonical serialization
// preserves number tokens byte-for-byte (1.0 stays 1.0, 1e5 stays 1e5).
type Num string

// Bool is a JSON boolean.
type Bool bool

// Null is the JSON null literal.
type Null struct{}

// Arr is a JSON array.
type Arr []Value

// Member is a single object member. Objects are ordered member lists, not
// maps: canonical output is defined by member order, and parsed documents may
// legally contain duplicate keys that must round-trip.
type Member struct {
	K string
	V Value
}

// Obj is a JSON object as an ordered member list.
type Obj []Member

func (Str) isValue()  {}
func (Num) isValue()  {}
func (Bool) isValue() {}
func (Null) isValue() {}
func (Arr) isValue()  {}
func (Obj) isValue()  {}

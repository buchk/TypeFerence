package tferlex

import (
	"strings"
	"testing"
)

func parse(t *testing.T, src string) *Node {
	t.Helper()
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return n
}

func field(n *Node, key string) *Node {
	for _, item := range n.Items {
		if item.Key == key {
			return item
		}
	}
	return nil
}

func TestBasicMappingAndScalars(t *testing.T) {
	n := parse(t, "kind: \"skill\"\nid: \"a/b@1.0.0\"\ncount: 42\nratio: 0.50\nok: true\nnothing: null\n")
	if got := field(n, "kind").Value; got.Kind != KindString || got.Text != "skill" {
		t.Errorf("quoted kind is a string: %+v", got)
	}
	if got := field(n, "id").Value; got.Text != "a/b@1.0.0" || got.Kind != KindString {
		t.Errorf("quoted id is a string: %+v", got)
	}
	if got := field(n, "count").Value; got.Kind != KindInteger || got.Text != "42" {
		t.Errorf("integer: %+v", got)
	}
	if got := field(n, "ratio").Value; got.Kind != KindDecimal || got.Text != "0.50" {
		t.Errorf("decimal lexeme must be preserved verbatim, got %+v", got)
	}
	if got := field(n, "ok").Value; got.Kind != KindBoolean {
		t.Errorf("boolean: %+v", got)
	}
	if got := field(n, "nothing").Value; got.Kind != KindNull {
		t.Errorf("null: %+v", got)
	}
}

func TestQuotedIsAlwaysString(t *testing.T) {
	n := parse(t, "a: \"42\"\nb: 'true'\n")
	if got := field(n, "a").Value; got.Kind != KindString || got.Text != "42" {
		t.Errorf("quoted digits are strings: %+v", got)
	}
	if got := field(n, "b").Value; got.Kind != KindString || got.Text != "true" {
		t.Errorf("quoted true is a string: %+v", got)
	}
}

func TestBareWordRejected(t *testing.T) {
	if _, err := Parse("kind: skill\n"); err == nil || !strings.Contains(err.Error(), "bare word") {
		t.Fatalf("expected bare-word error, got %v", err)
	}
}

func testCommentsInert(t *testing.T) {
	n := parse(t, "# leading comment\nkind: agent # trailing\n# tail\n")
	if len(n.Items) != 1 || n.Items[0].Key != "kind" {
		t.Fatalf("comments must be inert: %+v", n.Items)
	}
}

func TestDuplicateKeysError(t *testing.T) {
	_, err := Parse("a: \"x\"\na: \"y\"\n")
	if err == nil || !strings.Contains(err.Error(), "duplicate property 'a'") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestFlowAnchorAliasTagRejected(t *testing.T) {
	cases := map[string]string{
		"flow":    "a: [1, 2]\n",
		"anchor":  "a: &x \"y\"\nb: *x\n",
		"tag":     "a: !!str hello\n",
		"flowmap": "a: {b: \"c\"}\n",
	}
	for name, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestSequenceOfMaps(t *testing.T) {
	src := "skills:\n  - ref: \"a@1.0.0\"\n    sealed: true\n  - ref: \"b@1.0.0\"\n"
	n := parse(t, src)
	seq := field(n, "skills")
	if !seq.IsSeq || len(seq.Items) != 2 {
		t.Fatalf("expected sequence of 2, got %+v", seq)
	}
	first := seq.Items[0]
	ref := first.Items[0]
	if ref.Key != "ref" || ref.Value.Text != "a@1.0.0" {
		t.Errorf("first item ref: %+v", ref)
	}
	if sealed := first.Items[1]; sealed.Key != "sealed" || sealed.Value.Kind != KindBoolean {
		t.Errorf("first item sealed: %+v", sealed)
	}
}

func TestBlockScalarClipChomping(t *testing.T) {
	src := "body: |\n  line one\n  line two\nkind: \"x\"\n"
	n := parse(t, src)
	got := field(n, "body").Value.Text
	want := "line one\nline two\n"
	if got != want {
		t.Errorf("clip chomp: got %q want %q", got, want)
	}
}

func TestBlockScalarStripChomping(t *testing.T) {
	src := "body: |-\n  no trailing\nkind: \"x\"\n"
	n := parse(t, src)
	if got := field(n, "body").Value.Text; got != "no trailing" {
		t.Errorf("strip chomp: got %q", got)
	}
}

func TestBlockScalarPreservesBlankLines(t *testing.T) {
	src := "body: |\n  para one\n\n  para two\n"
	n := parse(t, src)
	got := field(n, "body").Value.Text
	if got != "para one\n\npara two\n" {
		t.Errorf("blank lines preserved: %q", got)
	}
}

func TestTabIndentationRejected(t *testing.T) {
	if _, err := Parse("a:\n\tb: \"x\"\n"); err == nil || !strings.Contains(err.Error(), "tab") {
		t.Fatalf("tab indentation must be rejected, got %v", err)
	}
}

func TestNestedMappingTwoSpaceIndent(t *testing.T) {
	src := "fields:\n  owner:\n    type: \"string\"\n    required: true\n"
	n := parse(t, src)
	fields := field(n, "fields")
	owner := fields.Items[0]
	if owner.Key != "owner" {
		t.Fatalf("owner: %+v", owner)
	}
	typ := owner.Items[0]
	if typ.Value.Text != "string" || typ.Value.Kind != KindString {
		t.Errorf("type value: %+v", typ.Value)
	}
	req := owner.Items[1]
	if req.Value.Kind != KindBoolean {
		t.Errorf("required: %+v", req.Value)
	}
}

func TestEmptyValueIsNull(t *testing.T) {
	n := parse(t, "key:\nother: \"v\"\n")
	if got := field(n, "key").Value; got.Kind != KindNull {
		t.Errorf("empty value: %+v", got)
	}
}

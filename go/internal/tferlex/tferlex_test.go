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

func TestPlainScalarsAreUntypedText(t *testing.T) {
	n := parse(t, "kind: skill\ncount: 42\nratio: 0.50\nok: true\nsays: Note: colons are fine\n")
	for key, want := range map[string]string{
		"kind": "skill", "count": "42", "ratio": "0.50", "ok": "true", "says": "Note: colons are fine",
	} {
		got := field(n, key).Value
		if got.Kind != KindPlain || got.Text != want {
			t.Errorf("%s: want plain %q, got %+v", key, want, got)
		}
	}
}

func TestQuotedIsAlwaysAString(t *testing.T) {
	n := parse(t, "a: \"42\"\nb: 'true'\nc: \"x \\\"y\\\" \\\\ \\n\\u00e9\\ud83d\\ude00\"\nd: 'it''s'\n")
	cases := map[string]string{"a": "42", "b": "true", "c": "x \"y\" \\ \né😀", "d": "it's"}
	for key, want := range cases {
		got := field(n, key).Value
		if got.Kind != KindQuoted || got.Text != want {
			t.Errorf("%s: want quoted %q, got %+v", key, want, got)
		}
	}
}

func TestNullAndEmptyCollections(t *testing.T) {
	n := parse(t, "a:\nb: null\nc: ~\nd: []\ne: {}\n")
	for _, key := range []string{"a", "b", "c"} {
		if got := field(n, key).Value; got.Kind != KindNull {
			t.Errorf("%s must be null, got %+v", key, got)
		}
	}
	if d := field(n, "d"); !d.IsSeq || len(d.Items) != 0 {
		t.Errorf("[] must be the empty sequence: %+v", d)
	}
	if e := field(n, "e"); !e.IsMap || len(e.Items) != 0 {
		t.Errorf("{} must be the empty mapping: %+v", e)
	}
}

func TestCommentsAreInert(t *testing.T) {
	n := parse(t, "# leading\nkind: agent # trailing\nq: \"keep # this\"\nr: C#sharp\n# tail\n")
	if got := field(n, "kind").Value.Text; got != "agent" {
		t.Errorf("trailing comment must be stripped: %q", got)
	}
	if got := field(n, "q").Value.Text; got != "keep # this" {
		t.Errorf("a # inside quotes is text: %q", got)
	}
	if got := field(n, "r").Value.Text; got != "C#sharp" {
		t.Errorf("a # not preceded by whitespace is text: %q", got)
	}
}

func TestRejectedSyntax(t *testing.T) {
	cases := map[string]string{
		"flow":       "a: [1, 2]\n",
		"flowmap":    "a: {b: c}\n",
		"anchor":     "a: &x y\n",
		"alias":      "a: *x\n",
		"tag":        "a: !!str hello\n",
		"folded":     "a: >\n  folded\n",
		"quotedKey":  "\"a\": b\n",
		"tab":        "a:\n\tb: c\n",
		"noSpace":    "a:b\n",
		"duplicate":  "a: x\na: y\n",
		"badKey":     "a b: c\n",
		"badHeader":  "a: |x\n  y\n",
		"unterminat": "a: \"open\n",
		"badEscape":  "a: \"\\q\"\n",
		"trailing":   "a: \"x\" y\n",
		"indent":     "a: x\n  b: y\n",
		"seqRoot":    "- a\n",
	}
	for name, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestBlockScalars(t *testing.T) {
	n := parse(t, "clip: |\n  one\n  # not a comment\n\n  two\n\n\nstrip: |-\n  no newline\nexplicit: |4\n      deeper\n    base\nafter: x\n")
	if got := field(n, "clip").Value; got.Kind != KindBlock || got.Text != "one\n# not a comment\n\ntwo\n" {
		t.Errorf("clip: %+v", got)
	}
	if got := field(n, "strip").Value.Text; got != "no newline" {
		t.Errorf("strip: %q", got)
	}
	if got := field(n, "explicit").Value.Text; got != "  deeper\nbase\n" {
		t.Errorf("explicit indentation: %q", got)
	}
	if got := field(n, "after").Value.Text; got != "x" {
		t.Errorf("parsing resumes after a block: %q", got)
	}
}

func TestSequences(t *testing.T) {
	src := "skills:\n  - skills/a.skill.tfer\n  - skill: skills/b.skill.tfer\n    sealed: true\n    tags:\n      - x\n  - dep/pkg:skills/c.skill.tfer\nsame:\n- one\n- two\nnested:\n  -\n    k: v\n"
	n := parse(t, src)
	skills := field(n, "skills")
	if !skills.IsSeq || len(skills.Items) != 3 {
		t.Fatalf("skills: %+v", skills)
	}
	if got := skills.Items[0].Value; got.Kind != KindPlain || got.Text != "skills/a.skill.tfer" {
		t.Errorf("path item: %+v", got)
	}
	mapping := skills.Items[1]
	if !mapping.IsMap || len(mapping.Items) != 3 {
		t.Fatalf("inline mapping item: %+v", mapping)
	}
	if field(mapping, "sealed").Value.Text != "true" || len(field(mapping, "tags").Items) != 1 {
		t.Errorf("inline mapping members: %+v", mapping)
	}
	if got := skills.Items[2].Value.Text; got != "dep/pkg:skills/c.skill.tfer" {
		t.Errorf("a package-qualified path is one scalar: %q", got)
	}
	if same := field(n, "same"); !same.IsSeq || len(same.Items) != 2 {
		t.Errorf("a sequence may align with its key: %+v", same)
	}
	if nested := field(n, "nested"); len(nested.Items) != 1 || field(nested.Items[0], "k").Value.Text != "v" {
		t.Errorf("a bare dash introduces a nested block: %+v", nested)
	}
}

func TestNestedMappings(t *testing.T) {
	n := parse(t, "fields:\n  owner:\n    type: string\n    required: true\n  count:\n    type: integer\n")
	fields := field(n, "fields")
	if !fields.IsMap || len(fields.Items) != 2 {
		t.Fatalf("fields: %+v", fields)
	}
	owner := field(fields, "owner")
	if field(owner, "type").Value.Text != "string" || field(owner, "required").Value.Text != "true" {
		t.Errorf("owner: %+v", owner)
	}
}

func TestErrorsCarryLines(t *testing.T) {
	_, err := Parse("a: x\nb: [1]\n")
	if err == nil || !strings.HasPrefix(err.Error(), "line 2:") {
		t.Fatalf("expected a line-2 error, got %v", err)
	}
}

func TestEmptyInput(t *testing.T) {
	n, err := Parse("\n# only a comment\n")
	if err != nil || n != nil {
		t.Fatalf("empty frontmatter parses to nil, got %+v %v", n, err)
	}
}

package jsonx

import (
	"strings"
	"testing"
)

// The expected values in this file were produced by probing the reference
// implementation (System.Text.Json, default encoder) on .NET 10.

func TestEscapeTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x00", "\"\\u0000\""},
		{"\x01", "\"\\u0001\""},
		{"\b", "\"\\b\""},
		{"\t", "\"\\t\""},
		{"\n", "\"\\n\""},
		{"\v", "\"\\u000B\""},
		{"\f", "\"\\f\""},
		{"\r", "\"\\r\""},
		{"\x1f", "\"\\u001F\""},
		{" ", "\" \""},
		{"!", "\"!\""},
		{"\"", "\"\\u0022\""},
		{"#$%", "\"#$%\""},
		{"&", "\"\\u0026\""},
		{"'", "\"\\u0027\""},
		{"()*,-./", "\"()*,-./\""},
		{"+", "\"\\u002B\""},
		{"09:;=?@", "\"09:;=?@\""},
		{"<", "\"\\u003C\""},
		{">", "\"\\u003E\""},
		{"AZ[]^_", "\"AZ[]^_\""},
		{"\\", "\"\\\\\""},
		{"`", "\"\\u0060\""},
		{"az{|}~", "\"az{|}~\""},
		{"\x7f", "\"\\u007F\""},
		{"\u0080", "\"\\u0080\""},
		{"\u00A0", "\"\\u00A0\""},
		{"\u00E9", "\"\\u00E9\""},
		{"\u2028", "\"\\u2028\""},
		{"\u4F60", "\"\\u4F60\""},
		{"\uFFFD", "\"\\uFFFD\""},
		{"\U0001F600", "\"\\uD83D\\uDE00\""},
		{"\U0010FFFF", "\"\\uDBFF\\uDFFF\""},
	}
	for _, c := range cases {
		if got := string(AppendString(nil, c.in)); got != c.want {
			t.Errorf("AppendString(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestInvalidUTF8BecomesReplacement(t *testing.T) {
	if got := string(AppendString(nil, "a\xffb")); got != "\"a\\uFFFDb\"" {
		t.Errorf("got %s", got)
	}
}

func TestNumberTokensPreserved(t *testing.T) {
	for _, token := range []string{"1.0", "1e5", "-0", "1E+2", "0.10", "123456789012345678901234567890", "1e-07", "2E10"} {
		v, err := Parse(token)
		if err != nil {
			t.Fatalf("Parse(%s): %v", token, err)
		}
		if got := Compact(v); got != token {
			t.Errorf("Compact(Parse(%s)) = %s", token, got)
		}
	}
}

func TestCanonicalCompact(t *testing.T) {
	// Matches the reference probe: whitespace collapses, an escaped A
	// re-encodes as the literal A, the 1.50 number token survives.
	in := "{\"b\":1,\"a\":{\"x\":[1.50,true,null,\"q\\u0041\"] }}"
	v, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Compact(v), "{\"b\":1,\"a\":{\"x\":[1.50,true,null,\"qA\"]}}"; got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestIndentedLayout(t *testing.T) {
	v := Obj{
		{K: "alpha", V: Str("x")},
		{K: "emptyArr", V: Arr{}},
		{K: "emptyObj", V: Obj{}},
		{K: "arr", V: Arr{Num("1"), Str("two"), Obj{{K: "inner", V: Bool(true)}}}},
		{K: "nested", V: Obj{
			{K: "deepList", V: Arr{Str("a")}},
			{K: "nullVal", V: Null{}},
		}},
	}
	want := strings.Join([]string{
		"{",
		"  \"alpha\": \"x\",",
		"  \"emptyArr\": [],",
		"  \"emptyObj\": {},",
		"  \"arr\": [",
		"    1,",
		"    \"two\",",
		"    {",
		"      \"inner\": true",
		"    }",
		"  ],",
		"  \"nested\": {",
		"    \"deepList\": [",
		"      \"a\"",
		"    ],",
		"    \"nullVal\": null",
		"  }",
		"}",
	}, "\n")
	if got := Indented(v); got != want {
		t.Errorf("indented layout mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{"", "{", "[1,]", "{\"a\":}", "01", "1.", "1e", "\"\x01\"", "tru", "{\"a\":1} extra", "-"}
	for _, b := range bad {
		if _, err := Parse(b); err == nil {
			t.Errorf("Parse(%q) should fail", b)
		}
	}
}

func TestParsePreservesDuplicateKeysAndOrder(t *testing.T) {
	v, err := Parse("{\"z\":1,\"a\":2,\"z\":3}")
	if err != nil {
		t.Fatal(err)
	}
	if got := Compact(v); got != "{\"z\":1,\"a\":2,\"z\":3}" {
		t.Errorf("got %s", got)
	}
}

func TestSurrogatePairEscapes(t *testing.T) {
	v, err := Parse("\"\\uD83D\\uDE00\"")
	if err != nil {
		t.Fatal(err)
	}
	if got := Compact(v); got != "\"\\uD83D\\uDE00\"" {
		t.Errorf("round-trip got %s", got)
	}
	// A lone surrogate escape decodes to U+FFFD, matching the reference
	// implementation's string round-trip.
	v, err = Parse("\"\\uD800x\"")
	if err != nil {
		t.Fatal(err)
	}
	if got := Compact(v); got != "\"\\uFFFDx\"" {
		t.Errorf("lone surrogate got %s", got)
	}
}

func TestDeepNestingRejected(t *testing.T) {
	deep := strings.Repeat("[", 80) + strings.Repeat("]", 80)
	if _, err := Parse(deep); err == nil {
		t.Error("expected depth error")
	}
}

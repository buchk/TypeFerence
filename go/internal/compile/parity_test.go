package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoPath resolves a path relative to the repository root.
func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

// TestHelioParityWithCommittedOutput compiles the shared example and
// byte-compares every target tree against the committed dist/ reference. This is
// the determinism contract: same input, byte-identical artifacts (ADR-0014).
func TestHelioParityWithCommittedOutput(t *testing.T) {
	source := repoPath(t, "examples", "helio")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("examples/helio not available: %v", err)
	}
	out := t.TempDir()
	if _, err := Build(source, out, []Target{Neutral, Codex, Copilot, Cursor},
		&ArdPublicationOptions{PublisherDomain: "helio.example"}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"neutral", "codex", "copilot", "cursor", "ard"} {
		expected := repoPath(t, "dist", target)
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("committed reference output missing: %s", expected)
		}
		result, err := CompareDirs(expected, filepath.Join(out, target))
		if err != nil {
			t.Fatal(err)
		}
		if result.Different {
			t.Errorf("%s differs from reference output:\n added: %v\n removed: %v\n changed: %v",
				target, result.Added, result.Removed, result.Changed)
		}
	}
}

// TestDeterministicRebuild verifies byte-identical output across repeated
// builds.
func TestDeterministicRebuild(t *testing.T) {
	source := repoPath(t, "examples", "helio")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("examples/helio not available: %v", err)
	}
	first := t.TempDir()
	second := t.TempDir()
	for _, out := range []string{first, second} {
		if _, err := Build(source, out, []Target{Neutral, Codex, Copilot, Cursor},
			&ArdPublicationOptions{PublisherDomain: "helio.example"}); err != nil {
			t.Fatal(err)
		}
	}
	hashFirst, err := HashDirectory(first)
	if err != nil {
		t.Fatal(err)
	}
	hashSecond, err := HashDirectory(second)
	if err != nil {
		t.Fatal(err)
	}
	if hashFirst != hashSecond {
		t.Errorf("rebuild produced different digest: %s vs %s", hashFirst, hashSecond)
	}
}

func TestEscapeYAML(t *testing.T) {
	if got := escapeYAML(`say "hi" \ bye`); got != `"say \"hi\" \\ bye"` {
		t.Errorf("got %s", got)
	}
}

func TestUrnSegment(t *testing.T) {
	cases := map[string]string{
		"Helio Source!": "helio-source",
		"---":           "package",
		"already-fine":  "already-fine",
	}
	for in, want := range cases {
		if got := urnSegment(in); got != want {
			t.Errorf("urnSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTargets(t *testing.T) {
	all, err := ParseTargets("ALL")
	if err != nil || len(all) != 4 {
		t.Fatalf("ParseTargets(all) = %v, %v", all, err)
	}
	if _, err := ParseTargets("bogus"); err == nil || !strings.Contains(err.Error(), "Unknown target") {
		t.Errorf("expected unknown target error, got %v", err)
	}
}

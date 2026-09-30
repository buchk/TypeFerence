package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

func sample() *AnswerSet {
	as, err := ParseAnswerSet([]byte(validJSON()))
	if err != nil {
		panic(err)
	}
	return as
}

func TestScaffoldProducesDeterministicTree(t *testing.T) {
	a, ma, err := Scaffold(sample())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := Scaffold(sample())
	if err != nil {
		t.Fatal(err)
	}
	if ma.GeneratorVersion != GeneratorVersion || ma.SchemaVersion != SchemaVersion {
		t.Fatalf("manifest: %+v", ma)
	}
	if len(a.Files) != len(b.Files) {
		t.Fatalf("file count differs: %d vs %d", len(a.Files), len(b.Files))
	}
	for i := range a.Files {
		if a.Files[i].Path != b.Files[i].Path || string(a.Files[i].Bytes) != string(b.Files[i].Bytes) {
			t.Fatalf("tree differs at %d: %s vs %s", i, a.Files[i].Path, b.Files[i].Path)
		}
	}
}

func TestGeneratedTreeCompilesWithOrdinaryCompiler(t *testing.T) {
	tree, _, err := Scaffold(sample())
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for _, f := range tree.Files {
		full := filepath.Join(src, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, f.Bytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := resource.LoadV6(src, resource.V6Options{})
	if err != nil {
		t.Fatalf("generated tree must load as an ordinary version 6 package: %v", err)
	}
	if len(loaded.Documents) < 5 {
		t.Fatalf("expected at least 5 documents, got %d", len(loaded.Documents))
	}
	out := t.TempDir()
	targets, _ := compile.ParseTargets("all")
	if _, err := compile.Build(src, out, targets, nil); err != nil {
		t.Fatalf("generated tree must compile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "agent-plugin", "ticket-bot", "plugin.json")); err != nil {
		t.Fatalf("the generated plugin set must emit the agent's plugin: %v", err)
	}
}

func TestMultiWordLevelNamesBecomePathSegments(t *testing.T) {
	as := sample()
	as.Levels = []TeamLevel{{Name: "Platform Team"}}
	tree, _, err := Scaffold(as)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range tree.Files {
		if f.Path == "profiles/platform-team.profile.tfer" {
			found = true
		}
	}
	if !found {
		t.Fatal("a multi-word level name must become a valid path segment")
	}
}

func TestRetiredHostNamesStayAccepted(t *testing.T) {
	raw := strings.Replace(validJSON(), `"hosts": ["neutral"]`, `"hosts": ["codex", "cursor"]`, 1)
	if _, err := ParseAnswerSet([]byte(raw)); err != nil {
		t.Fatalf("answer sets written before ADR-0029 must stay valid: %v", err)
	}
}

func TestNoWizardOnlyKeysInOutput(t *testing.T) {
	tree, _, err := Scaffold(sample())
	if err != nil {
		t.Fatal(err)
	}
	reserved := []string{"wizard", "scaffoldAnswer", "generatorHint"}
	for _, f := range tree.Files {
		text := strings.ToLower(string(f.Bytes))
		for _, r := range reserved {
			if strings.Contains(text, r) {
				t.Errorf("%s contains wizard-only key %q", f.Path, r)
			}
		}
	}
}

func TestMultilevelChainEmbedded(t *testing.T) {
	as := sample()
	as.Levels = []TeamLevel{{Name: "Platform"}, {Name: "Payments"}}
	as2 := &AnswerSet{SchemaVersion: as.SchemaVersion, Organization: as.Organization,
		Norms: as.Norms, Levels: []TeamLevel{{Name: "Platform"}, {Name: "Payments"}},
		Agent: as.Agent, Targets: as.Targets}
	tree, _, err := Scaffold(as2)
	if err != nil {
		t.Fatal(err)
	}
	var profile string
	for _, f := range tree.Files {
		if strings.Contains(f.Path, "payments.profile.tfer") {
			profile = string(f.Bytes)
		}
	}
	if profile == "" || !strings.Contains(profile, "profiles/platform.profile.tfer") {
		t.Fatalf("second level must embed the first; got:\n%s", profile)
	}
}

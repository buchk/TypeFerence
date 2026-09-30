package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

func digestPackage(t *testing.T) string {
	t.Helper()
	return writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":                 "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"context-types/settings.contexttype.tfer": "---\nfields:\n  enabled:\n    type: boolean\n    default: true\n---\n",
		"context/settings.context.tfer":           "---\ncontextType: context-types/settings.contexttype.tfer\n---\n",
		"agents/a.agent.tfer":                     "---\ndescription: A.\ncontext:\n  - context/settings.context.tfer\n---\n",
	})
}

func TestSourceDigestIgnoresGeneratedAndUnreferencedFiles(t *testing.T) {
	source := digestPackage(t)
	before, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	writeSrc(t, source, "dist/generated.json", "{}")
	writeSrc(t, source, "notes.txt", "not a resource")
	writeSrc(t, source, "drafts/unreferenced.skill.tfer", "---\ndescription: Unreferenced.\n---\nUnused.\n")
	after, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("generated or unreferenced files changed source identity: %s != %s", before, after)
	}
	writeSrc(t, source, "agents/a.agent.tfer", "---\ndescription: A, edited.\ncontext:\n  - context/settings.context.tfer\n---\n")
	edited, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if edited == before {
		t.Fatal("editing a member document must change source identity")
	}
}

func TestSourceDigestUnaffectedByResolverNormalization(t *testing.T) {
	source := digestPackage(t)
	before, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := resource.LoadV6(source, resource.V6Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.NormalizeV6(loaded.Documents); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve.New(loaded.Documents).ResolveAll(); err != nil {
		t.Fatal(err)
	}
	value, ok := loaded.Documents["acme/test/context/settings@1.0.0"].ContextFields["enabled"]
	if !ok || value.Kind != "scalar" || value.Scalar != "true" {
		t.Fatalf("resolver did not exercise in-place default materialization: %#v", value)
	}
	after, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("resolver normalization changed file-derived source identity: %s != %s", before, after)
	}
}

func TestBuildRejectsSourceRootAsOutput(t *testing.T) {
	source := digestPackage(t)
	if _, err := Build(source, source, []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "must not be the source root") {
		t.Fatalf("expected source-root output rejection, got %v", err)
	}
	if _, err := Build(source, filepath.Join(source, "agents", "out"), []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "under dist, bin, or obj") {
		t.Fatalf("expected nested non-output rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "agents", "out")); !os.IsNotExist(err) {
		t.Fatal("a rejected output directory must not be created")
	}
}

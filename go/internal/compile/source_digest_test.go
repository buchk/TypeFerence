package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

func TestSourceDigestIgnoresGeneratedAndUnreferencedFiles(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "agent.tfer", "schemaVersion: 5\nkind: agent\nid: acme/agents/a@1.0.0\n")
	before, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSrc(t, filepath.Join(source, "dist"), "generated.json", "{}")
	writeSrc(t, source, "notes.txt", "not a resource")
	after, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("generated/unreferenced files changed source identity: %s != %s", before, after)
	}
}

func TestSourceDigestUnaffectedByResolverNormalization(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "context-type.tfer", "schemaVersion: 5\nkind: contextType\nid: acme/context-types/settings@1.0.0\nfields:\n  enabled:\n    type: boolean\n    default: true\n")
	writeSrc(t, source, "context.tfer", "schemaVersion: 5\nkind: context\nid: acme/context/settings@1.0.0\ncontextType: acme/context-types/settings@1.0.0\n")
	writeSrc(t, source, "agent.tfer", "schemaVersion: 5\nkind: agent\nid: acme/agents/a@1.0.0\ncontext:\n  - acme/context/settings@1.0.0\n")

	before, err := HashSource(source)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := resource.Load(source, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolve.New(resources).ResolveAll(); err != nil {
		t.Fatal(err)
	}
	value, ok := resources["acme/context/settings@1.0.0"].ContextFields["enabled"]
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
	source := t.TempDir()
	writeSrc(t, source, "agent.tfer", "schemaVersion: 5\nkind: agent\nid: acme/agents/a@1.0.0\n")
	if _, err := Build(source, source, []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "must not be the source root") {
		t.Fatalf("expected source-root output rejection, got %v", err)
	}
}

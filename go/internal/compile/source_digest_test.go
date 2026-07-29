package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceDigestIgnoresGeneratedAndUnreferencedFiles(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
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

func TestBuildRejectsSourceRootAsOutput(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	if _, err := Build(source, source, []Target{Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "must not be the source root") {
		t.Fatalf("expected source-root output rejection, got %v", err)
	}
}

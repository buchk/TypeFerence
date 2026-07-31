package packages_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/packages"
)

func TestRestoreMaterializesTransitiveGraphForOfflineBuild(t *testing.T) {
	feed := t.TempDir()
	base := t.TempDir()
	write(t, base, "typeference.yaml", "schemaVersion: 2\nname: acme/base\nversion: 1.0.0\n")
	write(t, base, "profile.yaml", "schemaVersion: 4\nkind: profile\nid: acme/profiles/base@1.0.0\nworkingNorms: [preserve evidence]\n")
	packToFeed(t, base, feed, "acme/base", "1.0.0", "base")

	foundations := t.TempDir()
	write(t, foundations, "typeference.yaml", "schemaVersion: 2\nname: acme/foundations\nversion: 2.0.0\ndependencies:\n  acme/base: 1.0.0\n")
	write(t, foundations, "profile.yaml", "schemaVersion: 4\nkind: profile\nid: acme/profiles/foundations@2.0.0\nembeds: [acme/profiles/base@1.0.0]\n")
	restoreProject(t, foundations, feed)
	packToFeed(t, foundations, feed, "acme/foundations", "2.0.0", "foundations")

	root := t.TempDir()
	write(t, root, "typeference.yaml", "schemaVersion: 2\nname: acme/agents\nversion: 1.0.0\ndependencies:\n  acme/foundations: 2.0.0\n")
	write(t, root, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/payments@1.0.0\nembeds: [acme/profiles/foundations@2.0.0]\n")
	packagesDir := filepath.Join(root, "obj", "typeference", "packages")
	restorer := packages.Restorer{
		Source: root, PackagesDir: packagesDir,
		Config: &packages.FeedConfig{SchemaVersion: 1, Routes: map[string]packages.Route{
			"acme": {Kind: "filesystem", Path: feed},
		}},
	}
	lock, err := restorer.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Packages) != 2 {
		t.Fatalf("expected complete transitive graph, got %+v", lock.Packages)
	}
	if _, err := compile.BuildWithOptions(root, t.TempDir(), []compile.Target{compile.Neutral}, nil,
		compile.BuildOptions{PackagesDir: packagesDir}); err != nil {
		t.Fatalf("offline build could not consume restored graph: %v", err)
	}
}

func restoreProject(t *testing.T, source, feed string) {
	t.Helper()
	restorer := packages.Restorer{
		Source: source,
		Config: &packages.FeedConfig{SchemaVersion: 1, Routes: map[string]packages.Route{
			"acme": {Kind: "filesystem", Path: feed},
		}},
	}
	if _, err := restorer.Restore(); err != nil {
		t.Fatal(err)
	}
}

func TestPackIsByteDeterministic(t *testing.T) {
	source := t.TempDir()
	write(t, source, "typeference.yaml", "schemaVersion: 2\nname: acme/package\nversion: 1.0.0\n")
	write(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	first := filepath.Join(t.TempDir(), "a.tferpkg")
	second := filepath.Join(t.TempDir(), "b.tferpkg")
	digestA, err := packages.Pack(source, first)
	if err != nil {
		t.Fatal(err)
	}
	digestB, err := packages.Pack(source, second)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if digestA != digestB || string(a) != string(b) {
		t.Fatal("repeated packs were not byte-identical")
	}
}

func TestBuildAndPackRejectStaleLockForEmptyDependencyGraph(t *testing.T) {
	source := t.TempDir()
	write(t, source, "typeference.yaml", "schemaVersion: 2\nname: acme/package\nversion: 1.0.0\n")
	write(t, source, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agents/a@1.0.0\n")
	lock := packages.Lock{
		SchemaVersion: 1,
		Root:          "acme/package",
		RootVersion:   "1.0.0",
		Packages: []packages.LockedPackage{{
			Name: "acme/stale", Version: "1.0.0",
			Digest: "sha256:" + strings.Repeat("0", 64),
		}},
	}
	write(t, source, packages.LockFile, string(packages.EncodeLock(lock)))

	if _, err := compile.Build(source, t.TempDir(), []compile.Target{compile.Neutral}, nil); err == nil ||
		!strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("build must reject an undeclared locked package, got %v", err)
	}
	if _, err := packages.Pack(source, filepath.Join(t.TempDir(), "stale.tferpkg")); err == nil ||
		!strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("pack must reject an undeclared locked package, got %v", err)
	}
}

func packToFeed(t *testing.T, source, feed, name, version, leaf string) {
	t.Helper()
	output := filepath.Join(feed, filepath.FromSlash(name), version, leaf+"-"+version+".tferpkg")
	if _, err := packages.Pack(source, output); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

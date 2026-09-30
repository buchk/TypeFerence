package packages_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/packages"
)

const reviewSkill = "---\ndescription: Review a change for correctness.\n---\nReview the change for correctness.\n"

// feedPackages writes acme/base and acme/foundations to a filesystem
// feed: foundations depends on base and exports a profile and a skill.
func feedPackages(t *testing.T) string {
	t.Helper()
	feed := t.TempDir()
	base := t.TempDir()
	write(t, base, "typeference.tfer", "---\nschemaVersion: 6\nname: acme/base\nversion: 1.0.0\nexports:\n  - profiles/base.profile.tfer\n---\n")
	write(t, base, "profiles/base.profile.tfer", "---\ncontext:\n  - context/norm.context.tfer\n---\n")
	write(t, base, "context/norm.context.tfer", "---\ndisplayName: Base norm\n---\nCite evidence.\n")
	packToFeed(t, base, feed, "acme/base", "1.0.0", "base")

	foundations := t.TempDir()
	write(t, foundations, "typeference.tfer", "---\nschemaVersion: 6\nname: acme/foundations\nversion: 2.0.0\ndependencies:\n  acme/base: 1.0.0\nexports:\n  - profiles/foundations.profile.tfer\n  - skills/review.skill.tfer\n---\n")
	write(t, foundations, "profiles/foundations.profile.tfer", "---\nembeds:\n  - acme/base:profiles/base.profile.tfer\nskills:\n  - skills/review.skill.tfer\n---\n")
	write(t, foundations, "skills/review.skill.tfer", reviewSkill)
	write(t, foundations, "skills/internal.skill.tfer", "---\ndescription: Not exported.\n---\nInternal.\n")
	restoreProject(t, foundations, feed)
	packToFeed(t, foundations, feed, "acme/foundations", "2.0.0", "foundations")
	return feed
}

func TestRestoreMaterializesTransitiveGraphForOfflineBuild(t *testing.T) {
	feed := feedPackages(t)
	root := t.TempDir()
	write(t, root, "typeference.tfer", "---\nschemaVersion: 6\nname: acme/agents\nversion: 1.0.0\ndependencies:\n  acme/foundations: 2.0.0\nplugins:\n  - plugins/payments.plugin.tfer\n---\n")
	write(t, root, "plugins/payments.plugin.tfer", "---\ndescription: Payments kit.\nagents:\n  - agents/payments.agent.tfer\n---\n")
	write(t, root, "agents/payments.agent.tfer", "---\ndescription: Payments agent.\nembeds:\n  - acme/foundations:profiles/foundations.profile.tfer\nskills:\n  - skills/payments-review.skill.tfer\n---\n")
	// A skill may extend an exported skill of a dependency (ADR-0031).
	write(t, root, "skills/payments-review.skill.tfer", "---\ndescription: Review a payments change.\nextends: acme/foundations:skills/review.skill.tfer\n---\nAlso check reconciliation.\n")
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
	out := t.TempDir()
	if _, err := compile.BuildWithOptions(root, out, []compile.Target{compile.Neutral, compile.AgentPlugin}, nil,
		compile.BuildOptions{PackagesDir: packagesDir}); err != nil {
		t.Fatalf("offline build could not consume restored graph: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(out, "agent-plugin", "payments", "skills", "payments-review", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "Review the change for correctness.") || !strings.Contains(string(skill), "Also check reconciliation.") {
		t.Fatalf("a cross-package extension must carry its base's instructions then its own:\n%s", skill)
	}
	agent, err := os.ReadFile(filepath.Join(out, "agent-plugin", "payments", "com.github.copilot", "agents", "payments.agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agent), "Cite evidence.") {
		t.Fatalf("context inherited from a transitive dependency must render into the agent:\n%s", agent)
	}
}

func TestReferenceToUnexportedDependencyResourceFails(t *testing.T) {
	feed := feedPackages(t)
	root := t.TempDir()
	write(t, root, "typeference.tfer", "---\nschemaVersion: 6\nname: acme/agents\nversion: 1.0.0\ndependencies:\n  acme/foundations: 2.0.0\nplugins:\n  - plugins/kit.plugin.tfer\n---\n")
	write(t, root, "plugins/kit.plugin.tfer", "---\ndescription: Kit.\nskills:\n  - acme/foundations:skills/internal.skill.tfer\n---\n")
	restoreProject(t, root, feed)
	if _, err := compile.Build(root, t.TempDir(), []compile.Target{compile.AgentPlugin}, nil); err == nil ||
		!strings.Contains(err.Error(), "does not export") {
		t.Fatalf("a reference to an unexported dependency resource must fail, got %v", err)
	}
}

func TestReferenceToUndeclaredPackageFails(t *testing.T) {
	root := t.TempDir()
	write(t, root, "typeference.tfer", "---\nschemaVersion: 6\nname: acme/agents\nversion: 1.0.0\nplugins:\n  - plugins/kit.plugin.tfer\n---\n")
	write(t, root, "plugins/kit.plugin.tfer", "---\ndescription: Kit.\nskills:\n  - acme/other:skills/review.skill.tfer\n---\n")
	if _, err := compile.Build(root, t.TempDir(), []compile.Target{compile.AgentPlugin}, nil); err == nil ||
		!strings.Contains(err.Error(), "does not declare as a dependency") {
		t.Fatalf("a reference to an undeclared package must fail, got %v", err)
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

func minimalPackage(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	write(t, source, "typeference.tfer", "---\nschemaVersion: 6\nname: acme/package\nversion: 1.0.0\nplugins:\n  - plugins/kit.plugin.tfer\nexports:\n  - skills/review.skill.tfer\n---\n")
	write(t, source, "plugins/kit.plugin.tfer", "---\ndescription: Kit.\nskills:\n  - skills/review.skill.tfer\n---\n")
	write(t, source, "skills/review.skill.tfer", reviewSkill)
	return source
}

func TestPackIsByteDeterministic(t *testing.T) {
	source := minimalPackage(t)
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
	archive, err := packages.DecodeArchive(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Exports) != 1 || archive.Exports[0] != "acme/package/skills/review@1.0.0" {
		t.Fatalf("a package exports exactly its manifest's exports, got %v", archive.Exports)
	}
}

func TestSourceMembershipIsTheManifestClosure(t *testing.T) {
	source := minimalPackage(t)
	write(t, source, "drafts/unused.skill.tfer", "---\nnot: [valid\n---\n")
	write(t, source, "NOTES.md", "not source\n")
	files, err := packages.SourceFiles(source)
	if err != nil {
		t.Fatalf("unreferenced files must not affect membership: %v", err)
	}
	paths := []string{}
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	want := "plugins/kit.plugin.tfer,skills/review.skill.tfer,typeference.tfer"
	if strings.Join(paths, ",") != want {
		t.Fatalf("source members = %v, want %s", paths, want)
	}
}

func TestBuildAndPackRejectStaleLockForEmptyDependencyGraph(t *testing.T) {
	source := minimalPackage(t)
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

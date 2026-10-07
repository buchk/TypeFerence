package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

// marketplaceFixture copies the marketplace fixture so a test can change it, applying
// edits (path relative to the fixture, old text, new text) to the copy.
func marketplaceFixture(t *testing.T, edits ...[3]string) (string, manifest) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "fixture")
	copyTree(t, filepath.Join(fixturesRoot(t), "001-marketplace-from-packages"), dir)
	for _, edit := range edits {
		path := filepath.Join(dir, filepath.FromSlash(edit[0]))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), edit[1]) {
			t.Fatalf("%s does not contain %q", edit[0], edit[1])
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(data), edit[1], edit[2], 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, readManifest(t, dir)
}

func buildMarketplace(t *testing.T, dir string, m manifest) string {
	t.Helper()
	source, err := stagePackages(t, dir, m)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := compile.Build(source, out, compile.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(out, "agent-plugin")
}

func artifactDigest(t *testing.T, root, artifact string) string {
	t.Helper()
	digest, err := compile.HashDirectory(filepath.Join(root, artifact))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// A plugin artifact is a function of its owning package and that package's
// locked closure (ADR-0004): releasing one team's package rewrites only that
// team's plugins.
func TestMarketplaceBumpRewritesOnlyTheOwnersPlugins(t *testing.T) {
	baseDir, baseManifest := marketplaceFixture(t)
	before := buildMarketplace(t, baseDir, baseManifest)
	dir, m := marketplaceFixture(t,
		[3]string{"packages/data/typeference.tfer", "version: 1.1.0", "version: 1.2.0"},
		[3]string{"packages/data/agents/data-agent.agent.tfer", "keep its repositories healthy.", "keep its pipelines healthy."},
		[3]string{"source/typeference.tfer", "conformance/data: 1.1.0", "conformance/data: 1.2.0"},
	)
	after := buildMarketplace(t, dir, m)
	for _, artifact := range []string{"engineering-kit", "payments", "payments-pipeline"} {
		if artifactDigest(t, before, artifact) != artifactDigest(t, after, artifact) {
			t.Errorf("releasing conformance/data changed the %s plugin, which conformance/data does not own", artifact)
		}
	}
	if artifactDigest(t, before, "data") == artifactDigest(t, after, "data") {
		t.Error("releasing conformance/data must change its own plugin")
	}
	index, err := os.ReadFile(filepath.Join(after, ".github", "plugin", "marketplace.json"))
	if err != nil || !strings.Contains(string(index), `"version": "1.2.0"`) {
		t.Fatalf("the marketplace index lists the owning package's new version: %v\n%s", err, index)
	}
}

// A server shipped by plugins of two packages is one configuration, and a
// marketplace whose packages declare two different servers under one name
// fails rather than letting Copilot pick whichever loaded last.
func TestMarketplaceServerNamesDenoteOneConfiguration(t *testing.T) {
	dir, m := marketplaceFixture(t,
		[3]string{"packages/payments/skills/payments-status.skill.tfer", "extends: conformance/core:skills/status.skill.tfer", "extends: conformance/core:skills/status.skill.tfer\nrequiresServers:\n  - servers/conformance-tickets.server.tfer"},
	)
	server := "---\ntransport: streamable-http\nurl: https://payments.example/mcp\n---\n"
	path := filepath.Join(dir, "packages", "payments", "servers", "conformance-tickets.server.tfer")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(server), 0o644); err != nil {
		t.Fatal(err)
	}
	built := buildMarketplace(t, dir, m)
	config, err := os.ReadFile(filepath.Join(built, "payments", "mcp.json"))
	if err != nil || !strings.Contains(string(config), `"conformance-tickets"`) {
		t.Fatalf("the payments plugin ships the server its skill requires: %v\n%s", err, config)
	}
	if _, err := os.Stat(filepath.Join(built, "data", "mcp.json")); err == nil {
		t.Fatal("a plugin whose skills require no server ships no mcp.json")
	}
}

// candidate stages the marketplace fixture and writes a candidate package
// beside it.
func candidate(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	dir, m := marketplaceFixture(t)
	source, err := stagePackages(t, dir, m)
	if err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "candidate")
	for name, content := range files {
		full := filepath.Join(pkg, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return source, pkg
}

const candidateManifest = "---\nschemaVersion: 7\nname: conformance/data\nversion: 1.2.0\ndependencies:\n  conformance/core: 1.0.0\nplugins:\n  - plugins/data.plugin.tfer\n---\n"

func TestCandidateThatFitsValidates(t *testing.T) {
	source, pkg := candidate(t, map[string]string{
		"typeference.tfer":             candidateManifest,
		"plugins/data.plugin.tfer":     "---\ndescription: The data platform agent.\nagents:\n  - agents/data-agent.agent.tfer\n---\n",
		"agents/data-agent.agent.tfer": "---\ndescription: Data agent.\nembeds:\n  - conformance/core:profiles/engineering.profile.tfer\n---\nHelp the data team.\n",
	})
	summary, err := compile.Summarize(source, compile.BuildOptions{Candidate: pkg})
	if err != nil {
		t.Fatalf("a candidate that fits must validate: %v", err)
	}
	if strings.Join(summary.Plugins, ",") != "data,engineering-kit,payments,payments-pipeline" {
		t.Fatalf("the candidate's plugins replace the ones the marketplace ships from it: %v", summary.Plugins)
	}
	if _, err := compile.Build(source, t.TempDir(), compile.BuildOptions{Candidate: pkg}); err == nil {
		t.Fatal("a candidate must never be built")
	}
}

func TestCandidateCollisionIsReported(t *testing.T) {
	// The candidate ships its own skill named `status`, colliding with the
	// shared core skill every other plugin ships.
	source, pkg := candidate(t, map[string]string{
		"typeference.tfer":         candidateManifest,
		"plugins/data.plugin.tfer": "---\ndescription: The data kit.\nskills:\n  - skills/status.skill.tfer\n---\n",
		"skills/status.skill.tfer": "---\ndescription: Data pipeline status.\n---\nReport pipeline status.\n",
	})
	_, err := compile.Summarize(source, compile.BuildOptions{Candidate: pkg})
	if err == nil || !strings.Contains(err.Error(), "both emit the skill name 'status'") {
		t.Fatalf("the preflight must report the collision, got %v", err)
	}
}

func TestCandidateVersionSkewIsReported(t *testing.T) {
	// Releasing a new core while payments and data still pin the old one.
	source, pkg := candidate(t, map[string]string{
		"typeference.tfer":         "---\nschemaVersion: 7\nname: conformance/core\nversion: 1.1.0\nexports:\n  - skills/status.skill.tfer\n---\n",
		"skills/status.skill.tfer": "---\ndescription: Status.\n---\nReport status.\n",
	})
	_, err := compile.Summarize(source, compile.BuildOptions{Candidate: pkg})
	if err == nil || !strings.Contains(err.Error(), "must move with it") {
		t.Fatalf("the preflight must name the packages that have to move with a core release, got %v", err)
	}
}

func TestCandidateDependenciesMustBeLocked(t *testing.T) {
	source, pkg := candidate(t, map[string]string{
		"typeference.tfer":                  "---\nschemaVersion: 7\nname: conformance/data\nversion: 1.2.0\ndependencies:\n  conformance/core: 2.0.0\nplugins:\n  - plugins/data.plugin.tfer\n---\n",
		"plugins/data.plugin.tfer":          "---\ndescription: The data kit.\nskills:\n  - skills/pipeline-status.skill.tfer\n---\n",
		"skills/pipeline-status.skill.tfer": "---\ndescription: Pipeline status.\n---\nReport pipeline status.\n",
	})
	_, err := compile.Summarize(source, compile.BuildOptions{Candidate: pkg})
	if err == nil || !strings.Contains(err.Error(), "but conformance/marketplace locks conformance/core@1.0.0") {
		t.Fatalf("a candidate must use the locked versions of its dependencies, got %v", err)
	}
}

// Package conformance runs the current-language (version 6) golden
// conformance corpus and the separately identified legacy-v5 and legacy-v3
// archival regression corpora.
package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"github.com/buchk/TypeFerence/go/internal/scaffold"
)

// -update regenerates the digests in every success manifest from this
// implementation's output; never hand-edit a digest.
var update = flag.Bool("update", false, "rewrite fixture manifests with computed digests")

type manifest struct {
	Description        string `json:"description"`
	Language           string `json:"language,omitempty"`
	Expect             string `json:"expect"`
	EmitArd            string `json:"emitArd,omitempty"`
	TrustSignatures    string `json:"trustSignatures,omitempty"`
	AllowUnsignedTrust bool   `json:"allowUnsignedTrust,omitempty"`
	// Packages names a fixture directory of dependency package sources,
	// published to a temporary feed and restored before the build (ADR-0034).
	Packages string `json:"packages,omitempty"`
	// AnswerSet names the wizard answer file that produced the tree
	// (ADR-0028). GeneratorVersion names the generator that produced it; when
	// it is the current generator, the runner regenerates the tree from the
	// answers and requires the committed bytes.
	AnswerSet        string            `json:"answerSet,omitempty"`
	GeneratorVersion string            `json:"generatorVersion,omitempty"`
	Digests          map[string]string `json:"digests,omitempty"`
}

func fixturesRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "conformance", "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("conformance fixtures not found at %s: %v", root, err)
	}
	return root
}

func TestConformance(t *testing.T) {
	runCorpus(t, compile.LanguageCurrent)
}

// TestLegacyV5Golden reproduces the retired version 5 corpus. Its neutral
// bytes are archival evidence and must never change; the retired host
// adapters' digests left it with ADR-0029.
func TestLegacyV5Golden(t *testing.T) {
	runCorpus(t, compile.LanguageLegacyV5)
}

func TestLegacyV3Golden(t *testing.T) {
	runCorpus(t, compile.LanguageLegacyV3)
}

func runCorpus(t *testing.T, language string) {
	t.Helper()
	root := fixturesRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	count := 0
	for _, name := range names {
		dir := filepath.Join(root, name)
		m := readManifest(t, dir)
		if m.Language != language {
			continue
		}
		count++
		t.Run(name, func(t *testing.T) {
			runFixture(t, dir, m)
		})
	}
	if count == 0 {
		t.Fatalf("no fixtures found for language %q", map[bool]string{true: "v6", false: language}[language == ""])
	}
}

func readManifest(t *testing.T, dir string) manifest {
	t.Helper()
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("missing manifest: %v", err)
	}
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		t.Fatalf("invalid manifest: %v", err)
	}
	if m.Language != compile.LanguageCurrent && m.Language != compile.LanguageLegacyV5 && m.Language != compile.LanguageLegacyV3 {
		t.Fatalf("manifest language must be omitted for v6, or be legacy-v5 or legacy-v3, got %q", m.Language)
	}
	if m.Expect != "success" && m.Expect != "error" {
		t.Fatalf("manifest expect must be success or error, got %q", m.Expect)
	}
	return m
}

func runFixture(t *testing.T, dir string, m manifest) {
	manifestPath := filepath.Join(dir, "manifest.json")
	source := filepath.Join(dir, "source")
	out := t.TempDir()
	var ard *compile.ArdPublicationOptions
	if m.EmitArd != "" {
		ard = &compile.ArdPublicationOptions{
			PublisherDomain:    m.EmitArd,
			AllowUnsignedTrust: m.AllowUnsignedTrust,
		}
		if m.TrustSignatures != "" {
			ard.TrustSignaturesPath = filepath.Join(dir, m.TrustSignatures)
		}
	}
	// The current corpus builds every product target; the archival corpora
	// build the one target whose bytes they preserve.
	targets := []compile.Target{compile.Neutral}
	if m.Language == compile.LanguageCurrent {
		targets = []compile.Target{compile.Neutral, compile.AgentPlugin}
	}
	var buildErr error
	if m.Packages != "" {
		source, buildErr = stagePackages(t, dir, m)
	}
	if buildErr == nil {
		_, buildErr = compile.BuildWithOptions(source, out, targets, ard, compile.BuildOptions{Language: m.Language})
	}

	if m.Expect == "error" {
		if buildErr == nil {
			t.Fatal("expected compilation to fail, but it succeeded")
		}
		var diagnostic *resource.Error
		if !errors.As(buildErr, &diagnostic) {
			t.Fatalf("expected a diagnostic error, got %T: %v", buildErr, buildErr)
		}
		t.Logf("diagnostic: %v", buildErr)
		return
	}
	if buildErr != nil {
		t.Fatalf("expected success, got: %v", buildErr)
	}

	// A wizard golden fixture (ADR-0028) pins the SOURCE tree bytes, not the
	// compiled targets: recompute the scaffold digest exactly as `init` does
	// and compare it to the manifest's "scaffold" entry. The compile above
	// already proved the tree builds through the ordinary compiler.
	if m.AnswerSet != "" {
		want, ok := m.Digests["scaffold"]
		if !ok {
			t.Fatal("wizard fixture manifest has no scaffold digest")
		}
		got := hashSourceTree(source)
		if got != want {
			t.Errorf("scaffold digest mismatch:\n got:  %s\n want: %s\nIf this change intentionally alters generated bytes for existing answer sets, regenerate with the generator-version bump that justifies it.", got, want)
		}
		if m.GeneratorVersion == scaffold.GeneratorVersion {
			raw, err := os.ReadFile(filepath.Join(dir, m.AnswerSet))
			if err != nil {
				t.Fatal(err)
			}
			answers, err := scaffold.ParseAnswerSet(raw)
			if err != nil {
				t.Fatal(err)
			}
			tree, _, err := scaffold.Scaffold(answers)
			if err != nil {
				t.Fatal(err)
			}
			if regenerated := hashScaffold(tree); regenerated != want {
				t.Errorf("generator %s no longer reproduces the pinned tree:\n got:  %s\n want: %s", scaffold.GeneratorVersion, regenerated, want)
			}
		}
		return
	}

	computed := map[string]string{}
	outEntries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range outEntries {
		if !entry.IsDir() {
			continue
		}
		hash, hashErr := compile.HashDirectory(filepath.Join(out, entry.Name()))
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		computed[entry.Name()] = "sha256:" + hash
	}

	if *update {
		m.Digests = computed
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(m); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	if len(m.Digests) == 0 {
		t.Fatal("manifest has no digests; run `go test ./conformance -update` and review the specification/ADR change")
	}
	for target, want := range m.Digests {
		got, ok := computed[target]
		if !ok {
			t.Errorf("expected target %s was not emitted", target)
			continue
		}
		if got != want {
			t.Errorf("%s digest mismatch:\n got:  %s\n want: %s", target, got, want)
		}
	}
	for target := range computed {
		if _, ok := m.Digests[target]; !ok {
			t.Errorf("unexpected emitted target %s (not in manifest)", target)
		}
	}
}

// stagePackages publishes a fixture's dependency package sources to a
// temporary filesystem feed and restores a copy of the fixture's source
// against it (ADR-0034). Each package is packed after restoring its own
// dependencies, so the lockfiles and digests it produces are the ones restore
// and pack define. A broken package fails the test; a restore failure of the
// fixture's own source is returned as the fixture's result.
func stagePackages(t *testing.T, dir string, m manifest) (string, error) {
	t.Helper()
	work := t.TempDir()
	feed := filepath.Join(work, "feed")
	entries, err := os.ReadDir(filepath.Join(dir, m.Packages))
	if err != nil {
		t.Fatalf("fixture packages: %v", err)
	}
	type candidate struct {
		dir     string
		project *resource.Project
	}
	pending := []candidate{}
	routes := map[string]packages.Route{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		copied := filepath.Join(work, "packages", entry.Name())
		copyTree(t, filepath.Join(dir, m.Packages, entry.Name()), copied)
		project, err := resource.LoadProject(copied)
		if err != nil || project == nil {
			t.Fatalf("fixture package %s: %v", entry.Name(), err)
		}
		pending = append(pending, candidate{copied, project})
		routes[strings.SplitN(project.Name, "/", 2)[0]] = packages.Route{Kind: "filesystem", Path: feed}
	}
	config := &packages.FeedConfig{SchemaVersion: 1, Routes: routes}
	published := map[string]bool{}
	for len(pending) > 0 {
		progressed := false
		remaining := []candidate{}
		for _, c := range pending {
			ready := true
			for name, version := range c.project.Dependencies {
				if !published[name+"@"+version] {
					ready = false
				}
			}
			if !ready {
				remaining = append(remaining, c)
				continue
			}
			if len(c.project.Dependencies) > 0 {
				if _, err := (&packages.Restorer{Source: c.dir, Config: config}).Restore(); err != nil {
					t.Fatalf("fixture package %s: restore: %v", c.project.Name, err)
				}
			}
			leaf := c.project.Name[strings.LastIndex(c.project.Name, "/")+1:]
			output := filepath.Join(feed, filepath.FromSlash(c.project.Name), c.project.Version, leaf+"-"+c.project.Version+".tferpkg")
			if _, err := packages.Pack(c.dir, output); err != nil {
				t.Fatalf("fixture package %s: pack: %v", c.project.Name, err)
			}
			published[c.project.Name+"@"+c.project.Version] = true
			progressed = true
		}
		if !progressed {
			t.Fatalf("fixture packages depend on packages the fixture does not provide")
		}
		pending = remaining
	}
	if len(routes) == 0 {
		t.Fatal("fixture packages directory holds no packages")
	}
	source := filepath.Join(work, "source")
	copyTree(t, filepath.Join(dir, "source"), source)
	if project, err := resource.LoadProject(source); err == nil && project != nil {
		for name := range project.Dependencies {
			routes[strings.SplitN(name, "/", 2)[0]] = packages.Route{Kind: "filesystem", Path: feed}
		}
	}
	if _, err := (&packages.Restorer{Source: source, Config: config}).Restore(); err != nil {
		return "", err
	}
	return source, nil
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// hashScaffold computes the scaffold digest of an in-memory generated tree,
// exactly as `typeference init` does.
func hashScaffold(tree *scaffold.SourceTree) string {
	paths := make([]string, 0, len(tree.Files))
	contentOf := map[string][]byte{}
	for _, f := range tree.Files {
		paths = append(paths, f.Path)
		contentOf[f.Path] = f.Bytes
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(contentOf[p])
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// hashSourceTree computes typeference-resource-set-v1 over the files under
// root: sorted slash paths, each contributing path, NUL, content, NUL. This
// mirrors writeTree in cmd/typeference/init.go and the wasm scaffold entry
// point byte-for-byte (ADR-0028).
func hashSourceTree(root string) string {
	var paths []string
	contentOf := map[string][]byte{}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel = filepath.ToSlash(rel)
		paths = append(paths, rel)
		contentOf[rel] = data
		return nil
	})
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(contentOf[p])
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

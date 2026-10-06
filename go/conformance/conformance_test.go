// Package conformance runs the version 7 golden conformance corpus: each
// fixture either compiles to recorded agent-plugin digests or fails with a
// diagnostic.
package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// -update regenerates the digests in every success manifest from this
// implementation's output; never hand-edit a digest.
var update = flag.Bool("update", false, "rewrite fixture manifests with computed digests")

type manifest struct {
	Description string `json:"description"`
	Expect      string `json:"expect"`
	// Packages names a fixture directory of dependency package sources,
	// published to a temporary feed and restored before the build.
	Packages string            `json:"packages,omitempty"`
	Digests  map[string]string `json:"digests,omitempty"`
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
	if len(names) == 0 {
		t.Fatal("no conformance fixtures found")
	}
	for _, name := range names {
		dir := filepath.Join(root, name)
		m := readManifest(t, dir)
		t.Run(name, func(t *testing.T) {
			runFixture(t, dir, m)
		})
	}
}

func readManifest(t *testing.T, dir string) manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("missing manifest: %v", err)
	}
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		t.Fatalf("invalid manifest: %v", err)
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
	var buildErr error
	if m.Packages != "" {
		source, buildErr = stagePackages(t, dir, m)
	}
	if buildErr == nil {
		_, buildErr = compile.Build(source, out, compile.BuildOptions{})
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
	// TF_OUTPUT_DIR keeps each success fixture's output so CI can validate it
	// against the host schemas (docs/output-contract.md).
	if keep := os.Getenv("TF_OUTPUT_DIR"); keep != "" {
		copyTree(t, out, filepath.Join(keep, filepath.Base(dir)))
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
// against it. A broken package fails the test; a restore failure of the
// fixture's own source is returned as the fixture's result.
func stagePackages(t *testing.T, dir string, m manifest) (string, error) {
	t.Helper()
	work := t.TempDir()
	entries, err := os.ReadDir(filepath.Join(dir, m.Packages))
	if err != nil {
		t.Fatalf("fixture packages: %v", err)
	}
	dirs := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		copied := filepath.Join(work, "packages", entry.Name())
		copyTree(t, filepath.Join(dir, m.Packages, entry.Name()), copied)
		dirs = append(dirs, copied)
	}
	if len(dirs) == 0 {
		t.Fatal("fixture packages directory holds no packages")
	}
	config, err := packages.StageLocal(dirs, filepath.Join(work, "feed"))
	if err != nil {
		t.Fatalf("fixture packages: %v", err)
	}
	source := filepath.Join(work, "source")
	copyTree(t, filepath.Join(dir, "source"), source)
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

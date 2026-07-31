// Package conformance runs the current-language golden conformance corpus and
// the separately identified legacy-v3 archival regression corpus.
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
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// -update regenerates the digests in every success manifest from this
// implementation's output; never hand-edit a digest.
var update = flag.Bool("update", false, "rewrite fixture manifests with computed digests")

type manifest struct {
	Description        string            `json:"description"`
	Language           string            `json:"language,omitempty"`
	Expect             string            `json:"expect"`
	EmitArd            string            `json:"emitArd,omitempty"`
	TrustSignatures    string            `json:"trustSignatures,omitempty"`
	AllowUnsignedTrust bool              `json:"allowUnsignedTrust,omitempty"`
	Digests            map[string]string `json:"digests,omitempty"`
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
	runCorpus(t, "")
}

func TestLegacyV3Golden(t *testing.T) {
	runCorpus(t, "legacy-v3")
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
			runFixture(t, dir, m, language == "legacy-v3")
		})
	}
	if count == 0 {
		t.Fatalf("no fixtures found for language %q", map[bool]string{true: "v4", false: language}[language == ""])
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
	if m.Language != "" && m.Language != "legacy-v3" {
		t.Fatalf("manifest language must be omitted for v4 or legacy-v3, got %q", m.Language)
	}
	if m.Expect != "success" && m.Expect != "error" {
		t.Fatalf("manifest expect must be success or error, got %q", m.Expect)
	}
	return m
}

func runFixture(t *testing.T, dir string, m manifest, legacyV3 bool) {
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
	_, buildErr := compile.BuildWithOptions(source, out,
		[]compile.Target{compile.Neutral, compile.Codex, compile.Copilot, compile.Cursor},
		ard, compile.BuildOptions{LegacyV3: legacyV3})

	if m.Expect == "error" {
		if buildErr == nil {
			t.Fatal("expected compilation to fail, but it succeeded")
		}
		var diagnostic *resource.Error
		if !errors.As(buildErr, &diagnostic) {
			t.Fatalf("expected a diagnostic error, got %T: %v", buildErr, buildErr)
		}
		return
	}
	if buildErr != nil {
		t.Fatalf("expected success, got: %v", buildErr)
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

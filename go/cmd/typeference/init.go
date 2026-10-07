package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/buchk/TypeFerence/go/internal/scaffold"
)

// initCommand scaffolds a starter suite from a versioned answer set
// (ADR-0008). It is the CLI front door of the same generator the browser
// wizard uses; identical answers must produce identical bytes.
//
//	typeference init --answers answers.json [--out DIR] [--verify sha256:...]
func initCommand(args []string) (int, error) {
	answersPath := ""
	out := "."
	verify := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--answers", "--out", "--verify":
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a value", args[i])
			}
			switch args[i] {
			case "--answers":
				answersPath = args[i+1]
			case "--out":
				out = args[i+1]
			case "--verify":
				verify = args[i+1]
			}
			i++
		default:
			return 0, fmt.Errorf("unknown init flag: %s (usage: typeference init --answers <file> [--out DIR] [--verify sha256:...])", args[i])
		}
	}
	if answersPath == "" {
		return 0, fmt.Errorf("typeference init requires --answers <file>")
	}

	raw, err := os.ReadFile(answersPath)
	if err != nil {
		return 0, fmt.Errorf("cannot read answer set: %v", err)
	}
	as, err := scaffold.ParseAnswerSet(raw)
	if err != nil {
		return 0, err
	}
	tree, manifest, err := scaffold.Scaffold(as)
	if err != nil {
		return 0, err
	}

	digest, err := writeTree(tree, out)
	if err != nil {
		return 0, err
	}

	manifestJSON, _ := json.MarshalIndent(map[string]interface{}{
		"generatorVersion": manifest.GeneratorVersion,
		"schemaVersion":    manifest.SchemaVersion,
		"scaffoldDigest":   digest,
	}, "", "  ")
	dotDir := filepath.Join(out, ".typeference")
	if mkErr := os.MkdirAll(dotDir, 0o755); mkErr == nil {
		_ = os.WriteFile(filepath.Join(dotDir, "scaffold.json"), append(manifestJSON, '\n'), 0o644)
	}

	fmt.Printf("Scaffolded %d files at %s\n", len(tree.Files), out)
	fmt.Printf("SHA-256 %s\n", digest)
	fmt.Printf("Generator %s / answer schema %d\n", manifest.GeneratorVersion, manifest.SchemaVersion)

	if verify != "" {
		if verify != digest {
			return 2, fmt.Errorf("scaffold verification failed: expected %s, computed %s.\nIf the versions differ, download matching answers from your wizard session or upgrade the CLI.", verify, digest)
		}
		fmt.Println("Verified: generated suite matches the browser session byte-for-byte.")
	}
	return 0, nil
}

func writeTree(tree *scaffold.SourceTree, out string) (string, error) {
	abs, err := filepath.Abs(out)
	if err != nil {
		abs = out
	}
	if entries, readErr := os.ReadDir(abs); readErr == nil && len(entries) > 0 {
		// Allow re-init into a directory that only contains our own marker.
		owned := false
		if _, statErr := os.Stat(filepath.Join(abs, ".typeference", "scaffold.json")); statErr == nil {
			owned = true
		}
		if !owned {
			return "", fmt.Errorf("output directory %s is not empty and was not created by typeference init; choose another --out", abs)
		}
	}
	for _, f := range tree.Files {
		full := filepath.Join(out, filepath.FromSlash(f.Path))
		if mkErr := os.MkdirAll(filepath.Dir(full), 0o755); mkErr != nil {
			return "", mkErr
		}
		if wErr := os.WriteFile(full, f.Bytes, 0o644); wErr != nil {
			return "", wErr
		}
	}
	paths := make([]string, 0, len(tree.Files))
	contentOf := map[string]string{}
	for _, f := range tree.Files {
		paths = append(paths, f.Path)
		contentOf[f.Path] = string(f.Bytes)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write([]byte(contentOf[p]))
		h.Write([]byte{0})
	}
	return "sha256:" + fmt.Sprintf("%x", h.Sum(nil)), nil
}

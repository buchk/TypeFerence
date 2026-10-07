//go:build js && wasm

package main

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"syscall/js"

	"github.com/buchk/TypeFerence/go/internal/scaffold"
)

// scaffoldFunc implements TypeFerence.scaffold(request): the same generator
// `typeference init` runs locally (ADR-0008). The request carries a raw
// answerSet JSON string; the result carries ok, error, files (path -> bytes),
// digest, and manifest. The digest is computed identically to the CLI so the
// wizard's --verify exit ramp holds byte-for-byte.
func scaffoldFunc(_ js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = map[string]any{"ok": false, "error": fmt.Sprintf("internal error: %v", r)}
		}
	}()
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return map[string]any{"ok": false, "error": "scaffold requires a request object"}
	}
	raw := args[0].Get("answerSet")
	if raw.Type() != js.TypeString {
		return map[string]any{"ok": false, "error": "answerSet must be a JSON string"}
	}
	as, err := scaffold.ParseAnswerSet([]byte(raw.String()))
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	tree, manifest, err := scaffold.Scaffold(as)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}

	files := map[string]any{}
	paths := make([]string, 0, len(tree.Files))
	contentOf := map[string]string{}
	for _, f := range tree.Files {
		files[f.Path] = string(f.Bytes)
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
	digest := "sha256:" + fmt.Sprintf("%x", h.Sum(nil))

	return map[string]any{
		"ok":     true,
		"files":  files,
		"digest": digest,
		"manifest": map[string]any{
			"generatorVersion": manifest.GeneratorVersion,
			"schemaVersion":    manifest.SchemaVersion,
		},
	}
}

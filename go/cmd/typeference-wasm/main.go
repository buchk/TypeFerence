//go:build js && wasm

// Command typeference-wasm exposes the unmodified Go compiler to the browser
// playground (web/playground). It registers a global `TypeFerence.compile`
// function that writes the caller's sources into the in-memory filesystem
// provided by memfs.js, runs the same compile.Validate / compile.Build /
// compile.HashDirectory code paths as the CLI, and returns artifacts,
// diagnostics, and the embedding graph as a plain JavaScript object.
//
// The compiler internals are untouched: the playground's determinism digest
// is produced by the identical code that produces it on disk.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall/js"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// version is stamped by the Pages workflow via
// -ldflags "-X main.version=<ref>"; development builds report "dev".
var version = "dev"

const (
	workRoot   = "/work"
	outputRoot = "/out"
)

func main() {
	js.Global().Set("TypeFerence", js.ValueOf(map[string]any{
		"version": version,
		"compile": js.FuncOf(compileFunc),
	}))
	select {}
}

// compileFunc implements TypeFerence.compile(request). The request object
// carries files (a path-to-content map), target, emitArd, publisherDomain,
// and sourceName; the result object carries ok, error, agents (id,
// displayName, emit, satisfies, bundle), files, hash, and graph.
//
// sourceName becomes the in-memory source directory's basename. The ARD
// source-package URN is derived from that name, so using the same directory
// name as a CLI invocation reproduces its digest exactly.
func compileFunc(_ js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = map[string]any{"ok": false, "error": fmt.Sprintf("internal error: %v", r)}
		}
	}()
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return map[string]any{"ok": false, "error": "compile requires a request object"}
	}
	request := args[0]

	sourceName := "src"
	if n := request.Get("sourceName"); n.Type() == js.TypeString {
		if clean := sanitizeName(n.String()); clean != "" {
			sourceName = clean
		}
	}
	sourceRoot := path.Join(workRoot, sourceName)

	if err := writeSources(sourceRoot, request.Get("files")); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}

	// The embedding graph comes straight from the loaded documents so it can
	// be shown even when resolution fails (cycles, ambiguity, missing refs).
	graph := map[string]any{"nodes": []any{}, "edges": []any{}}
	if resources, err := resource.Load(sourceRoot, ""); err == nil {
		graph = buildGraph(resources)
	}

	targetValue := "all"
	if t := request.Get("target"); t.Type() == js.TypeString && t.String() != "" {
		targetValue = t.String()
	}
	targets, err := compile.ParseTargets(targetValue)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error(), "graph": graph}
	}
	var ard *compile.ArdPublicationOptions
	if b := request.Get("emitArd"); b.Type() == js.TypeBoolean && b.Bool() {
		domain := "playground.example"
		if d := request.Get("publisherDomain"); d.Type() == js.TypeString && d.String() != "" {
			domain = d.String()
		}
		ard = &compile.ArdPublicationOptions{PublisherDomain: domain}
	}

	agents, err := compile.Validate(sourceRoot, "")
	if err != nil {
		return map[string]any{"ok": false, "error": relativizeError(err.Error(), sourceRoot), "graph": graph}
	}
	if _, err := compile.Build(sourceRoot, outputRoot, targets, ard); err != nil {
		return map[string]any{"ok": false, "error": relativizeError(err.Error(), sourceRoot), "graph": graph}
	}
	hash, err := compile.HashDirectory(outputRoot)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error(), "graph": graph}
	}
	artifacts, err := readTree(outputRoot)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error(), "graph": graph}
	}

	agentList := make([]any, 0, len(agents))
	for _, agent := range agents {
		satisfies := make([]any, 0, len(agent.Satisfies))
		for _, s := range agent.Satisfies {
			satisfies = append(satisfies, s)
		}
		agentList = append(agentList, map[string]any{
			"id":          agent.ID,
			"displayName": agent.DisplayName,
			"emit":        agent.Emit,
			"satisfies":   satisfies,
			"bundle":      compile.BundleJSON(agent),
		})
	}

	return map[string]any{
		"ok":     true,
		"agents": agentList,
		"files":  artifacts,
		"hash":   hash,
		"graph":  graph,
	}
}

// sanitizeName reduces a requested source-directory name to a safe single
// path segment.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".")
}

// writeSources resets the work tree and materializes the request's file map
// beneath sourceRoot.
func writeSources(sourceRoot string, files js.Value) error {
	if files.Type() != js.TypeObject {
		return resource.Errorf("compile request needs a files object")
	}
	if err := os.RemoveAll(workRoot); err != nil {
		return resource.Errorf("cannot reset source root: %s", err)
	}
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		return resource.Errorf("cannot create source root: %s", err)
	}
	keys := js.Global().Get("Object").Call("keys", files)
	for i := 0; i < keys.Length(); i++ {
		name := keys.Index(i).String()
		clean := path.Clean("/" + name)[1:]
		if clean == "" || clean == "." || strings.HasPrefix(clean, "..") {
			return resource.Errorf("invalid source path: %s", name)
		}
		full := filepath.Join(sourceRoot, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return resource.Errorf("cannot create directory for %s", name)
		}
		content := files.Get(name).String()
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return resource.Errorf("cannot write %s: %s", name, err)
		}
	}
	return nil
}

// readTree returns every file beneath root keyed by slash-separated path
// relative to root.
func readTree(root string) (map[string]any, error) {
	files := map[string]any{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		files[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("cannot read compiled output: %s", err)
	}
	return files, nil
}

// buildGraph extracts the declared composition edges from loaded documents:
// embeds, skill attachments, and capability bindings.
func buildGraph(resources map[string]*resource.Document) map[string]any {
	ids := make([]string, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	nodes := make([]any, 0, len(ids))
	edges := []any{}
	edge := func(from, to, kind string) {
		edges = append(edges, map[string]any{"from": from, "to": to, "kind": kind})
	}
	for _, id := range ids {
		doc := resources[id]
		nodes = append(nodes, map[string]any{
			"id":          doc.ID,
			"kind":        doc.Kind,
			"displayName": doc.DisplayName,
			"emit":        doc.Emit,
		})
		for _, embedded := range doc.Embeds {
			edge(doc.ID, embedded, "embeds")
		}
		for _, binding := range doc.Skills {
			edge(doc.ID, binding.Ref, "skill")
			if binding.Capability != nil {
				edge(binding.Ref, *binding.Capability, "capability")
			}
		}
		if doc.Kind == "skill" && doc.Binds != "" {
			edge(doc.ID, doc.Binds, "binds")
		}
		for _, capability := range doc.RequiresCapabilities {
			edge(doc.ID, capability, "requires")
		}
	}
	return map[string]any{"nodes": nodes, "edges": edges}
}

// relativizeError strips the in-memory source prefix from diagnostics so the
// playground shows the same paths the user is editing.
func relativizeError(message, sourceRoot string) string {
	return strings.ReplaceAll(message, sourceRoot+"/", "")
}

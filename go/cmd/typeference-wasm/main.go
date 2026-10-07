//go:build js && wasm

// Command typeference-wasm exposes the unmodified Go compiler to the browser
// playground (web/playground). It registers a global `TypeFerence` object
// whose compile function writes the caller's sources into the in-memory
// filesystem provided by memfs.js, stages any dependency packages into an
// in-memory feed, and runs the same restore, compile.Validate, compile.Build,
// and compile.HashDirectory code paths as the CLI. It returns artifacts,
// diagnostics, the composition graph, and every context type's form shape.
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
	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// version is stamped by the Pages workflow via
// -ldflags "-X main.version=<ref>"; development builds report "dev".
var version = "dev"

const (
	workRoot    = "/work"
	outputRoot  = "/out"
	packageRoot = "/work/packages"
	feedRoot    = "/work/feed"
)

func main() {
	// Restore stages packages through os.MkdirTemp.
	if err := os.MkdirAll(os.TempDir(), 0o755); err != nil {
		panic(err)
	}
	js.Global().Set("TypeFerence", js.ValueOf(map[string]any{
		"version":  version,
		"compile":  js.FuncOf(compileFunc),
		"scaffold": js.FuncOf(scaffoldFunc),
	}))
	select {}
}

// compileFunc implements TypeFerence.compile(request). The request object
// carries files (a path-to-content map for the package being built),
// sourceName, and optionally packages (a map from package directory name to
// a path-to-content map) for the package's dependencies. The result object
// carries ok, error, agents (id, displayName, bundle), files, hash, graph,
// and contextTypes.
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
	if err := stagePackages(sourceRoot, request.Get("packages")); err != nil {
		return map[string]any{"ok": false, "error": relativizeError(err.Error(), workRoot)}
	}

	// The composition graph and form shapes come straight from the loaded
	// documents so they can be shown even when resolution fails.
	graph := map[string]any{"nodes": []any{}, "edges": []any{}}
	contextTypes := []any{}
	data := []any{}
	if loaded, err := resource.LoadPackage(sourceRoot, resource.PackageOptions{}); err == nil {
		docs := loaded.Documents
		if set, setErr := packages.LoadDependencySet(sourceRoot, ""); setErr == nil {
			for id, doc := range set.Documents {
				docs[id] = doc
			}
		}
		graph = buildGraph(docs)
		contextTypes = formShapes(docs)
		data = dataEntries(docs)
	}

	fail := func(message string) map[string]any {
		return map[string]any{"ok": false, "error": message, "graph": graph, "contextTypes": contextTypes, "data": data}
	}
	agents, err := compile.Validate(sourceRoot)
	if err != nil {
		return fail(relativizeError(err.Error(), sourceRoot))
	}
	if _, err := compile.Build(sourceRoot, outputRoot, compile.BuildOptions{}); err != nil {
		return fail(relativizeError(err.Error(), sourceRoot))
	}
	hash, err := compile.HashDirectory(outputRoot)
	if err != nil {
		return fail(err.Error())
	}
	artifacts, err := readTree(outputRoot)
	if err != nil {
		return fail(err.Error())
	}
	agentList := make([]any, 0, len(agents))
	for _, agent := range agents {
		agentList = append(agentList, map[string]any{
			"id":          agent.ID,
			"displayName": agent.DisplayName,
			"bundle":      compile.BundleJSON(agent),
		})
	}
	return map[string]any{
		"ok":           true,
		"agents":       agentList,
		"files":        artifacts,
		"hash":         hash,
		"graph":        graph,
		"contextTypes": contextTypes,
		"data":         data,
	}
}

// dataEntries returns every data document's authored values as the
// compiler parsed them, keyed by package and path, so the Instantiate form
// starts from the compiler's reading of the file rather than its own parse.
func dataEntries(docs map[string]*resource.Document) []any {
	entries := []any{}
	for _, id := range resource.SortedKeys(docs) {
		doc := docs[id]
		if !doc.IsData() {
			continue
		}
		values := map[string]any{}
		for _, name := range resource.SortedKeys(doc.Values) {
			value := doc.Values[name]
			switch value.Kind {
			case "scalar":
				values[name] = value.Scalar
			case "list":
				items := make([]any, 0, len(value.List))
				for _, item := range value.List {
					items = append(items, item.Scalar)
				}
				values[name] = items
			}
		}
		entries = append(entries, map[string]any{
			"package":     doc.Package,
			"path":        doc.Path,
			"contextType": doc.ContextType,
			"values":      values,
		})
	}
	return entries
}

// stagePackages writes each dependency package beneath the work tree, packs
// them into an in-memory filesystem feed, and restores the source package
// against it.
func stagePackages(sourceRoot string, request js.Value) error {
	if request.Type() != js.TypeObject {
		return nil
	}
	keys := js.Global().Get("Object").Call("keys", request)
	if keys.Length() == 0 {
		return nil
	}
	dirs := []string{}
	for i := 0; i < keys.Length(); i++ {
		key := keys.Index(i).String()
		name := sanitizeName(key)
		if name == "" {
			return resource.Errorf("invalid package directory name: %s", key)
		}
		dir := path.Join(packageRoot, name)
		if err := materializeTree(dir, request.Get(key)); err != nil {
			return err
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	config, err := packages.StageLocal(dirs, feedRoot)
	if err != nil {
		return err
	}
	restorer := packages.Restorer{Source: sourceRoot, Config: config}
	_, err = restorer.Restore()
	return err
}

// formShapes describes every context type as the fields of a form whose
// result is a data document (ADR-0006).
func formShapes(docs map[string]*resource.Document) []any {
	shapes := []any{}
	for _, id := range resource.SortedKeys(docs) {
		doc := docs[id]
		if doc.Kind != "contextType" {
			continue
		}
		fields := make([]any, 0, len(doc.Fields))
		for _, field := range doc.Fields {
			choices := make([]any, 0, len(field.Choices))
			for _, choice := range field.Choices {
				choices = append(choices, choice)
			}
			entry := map[string]any{
				"name":        field.Name,
				"type":        field.Type,
				"required":    field.Required,
				"displayName": field.DisplayName,
				"description": field.Description,
				"choices":     choices,
			}
			if field.HasDefault {
				if field.Default.Kind == "list" {
					items := make([]any, 0, len(field.Default.List))
					for _, item := range field.Default.List {
						items = append(items, item.Scalar)
					}
					entry["default"] = items
				} else {
					entry["default"] = field.Default.Scalar
				}
			}
			fields = append(fields, entry)
		}
		shapes = append(shapes, map[string]any{
			"id":           doc.ID,
			"path":         doc.Path,
			"displayName":  doc.DisplayName,
			"description":  doc.Description,
			"instanceName": doc.InstanceName,
			"fields":       fields,
		})
	}
	return shapes
}

// sanitizeName reduces a requested directory name to a safe single path
// segment.
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
	if err := os.RemoveAll(workRoot); err != nil {
		return resource.Errorf("cannot reset source root: %s", err)
	}
	return materializeTree(sourceRoot, files)
}

// materializeTree writes a JS path-to-content object beneath root, resetting
// root first.
func materializeTree(root string, files js.Value) error {
	if files.Type() != js.TypeObject {
		return resource.Errorf("request needs a files object")
	}
	if err := os.RemoveAll(root); err != nil {
		return resource.Errorf("cannot reset %s: %s", root, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return resource.Errorf("cannot create %s: %s", root, err)
	}
	keys := js.Global().Get("Object").Call("keys", files)
	for i := 0; i < keys.Length(); i++ {
		name := keys.Index(i).String()
		clean := path.Clean("/" + name)[1:]
		if clean == "" || clean == "." || strings.HasPrefix(clean, "..") {
			return resource.Errorf("invalid source path: %s", name)
		}
		full := filepath.Join(root, filepath.FromSlash(clean))
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

// buildGraph extracts the declared composition edges from loaded documents.
func buildGraph(resources map[string]*resource.Document) map[string]any {
	nodes := []any{}
	edges := []any{}
	edge := func(from, to, kind string) {
		edges = append(edges, map[string]any{"from": from, "to": to, "kind": kind})
	}
	for _, id := range resource.SortedKeys(resources) {
		doc := resources[id]
		nodes = append(nodes, map[string]any{
			"id":          doc.ID,
			"kind":        doc.Kind,
			"displayName": doc.DisplayName,
		})
		for _, embedded := range doc.Embeds {
			edge(doc.ID, embedded, "embeds")
		}
		for _, binding := range doc.Skills {
			if binding.Ref != "" {
				edge(doc.ID, binding.Ref, "skill")
			}
			if binding.Capability != nil {
				edge(doc.ID, *binding.Capability, "capability")
			}
		}
		if doc.Kind == "skill" && doc.Binds != "" && doc.Binds != doc.ID {
			edge(doc.ID, doc.Binds, "binds")
		}
		if doc.Kind == "skill" && doc.Extends != "" {
			edge(doc.ID, doc.Extends, "extends")
		}
		for _, name := range resource.SortedKeys(doc.Parameters) {
			edge(doc.ID, doc.Parameters[name], "parameter")
		}
		for _, name := range resource.SortedKeys(doc.With) {
			edge(doc.ID, doc.With[name], "with")
		}
		for _, ref := range doc.Context {
			edge(doc.ID, ref.ID, "context")
		}
		for _, server := range doc.RequiresServers {
			edge(doc.ID, server, "server")
		}
		if doc.ContextType != "" {
			edge(doc.ID, doc.ContextType, "contextType")
		}
		for _, shipped := range append(append(append([]string{}, doc.PluginAgents...), doc.PluginProfiles...), doc.PluginSkills...) {
			edge(doc.ID, shipped, "ships")
		}
		for _, id := range doc.Rules {
			edge(doc.ID, id, "rule")
		}
		for _, id := range doc.Commands {
			edge(doc.ID, id, "command")
		}
		for _, id := range doc.Hooks {
			edge(doc.ID, id, "hook")
		}
		for _, id := range append(append([]string{}, doc.Servers...), doc.PluginLSP...) {
			edge(doc.ID, id, "server")
		}
	}
	return map[string]any{"nodes": nodes, "edges": edges}
}

// relativizeError strips the in-memory source prefix from diagnostics so the
// playground shows the same paths the user is editing.
func relativizeError(message, sourceRoot string) string {
	return strings.ReplaceAll(message, sourceRoot+"/", "")
}

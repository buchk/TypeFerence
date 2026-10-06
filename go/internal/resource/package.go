package resource

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/buchk/TypeFerence/go/internal/tferlex"
)

// kindSuffixes maps each document suffix to its kind. A document's kind is
// decided by its file name, never by a field.
var kindSuffixes = []struct {
	suffix string
	kind   string
}{
	{".agent.tfer", "agent"},
	{".profile.tfer", "profile"},
	{".capability.tfer", "capability"},
	{".skill.tfer", "skill"},
	{".server.tfer", "server"},
	{".contexttype.tfer", "contextType"},
	{".context.tfer", "context"},
	{".plugin.tfer", "plugin"},
}

// removedSuffixes name the document kinds version 7 removed, so a stray file
// fails with a pointer rather than as an unknown suffix.
var removedSuffixes = map[string]string{
	".interface.tfer": "interfaces were removed in version 7 (ADR-0035)",
	".tool.tfer":      "tools were replaced by .server.tfer documents in version 7 (ADR-0037)",
}

// KindSuffix returns the file suffix for a document kind.
func KindSuffix(kind string) string {
	for _, entry := range kindSuffixes {
		if entry.kind == kind {
			return entry.suffix
		}
	}
	return ""
}

var pathSegment = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// KindFromPath derives a document's kind and identity stem from its
// source-relative path: skills/core/review.skill.tfer is a skill whose stem is
// skills/core/review.
func KindFromPath(relative string) (kind, stem string, ok bool) {
	for _, entry := range kindSuffixes {
		if strings.HasSuffix(relative, entry.suffix) {
			return entry.kind, strings.TrimSuffix(relative, entry.suffix), true
		}
	}
	return "", "", false
}

// ValidSourcePath reports whether a source-relative path is clean, relative,
// and names a document whose stem segments are identity segments.
func ValidSourcePath(relative string) error {
	if relative == "" {
		return Errorf("empty path")
	}
	if strings.Contains(relative, "\\") {
		return Errorf("path '%s' must use forward slashes", relative)
	}
	if strings.HasPrefix(relative, "/") || filepath.IsAbs(relative) || path.Clean(relative) != relative {
		return Errorf("path '%s' must be a clean path relative to the package root", relative)
	}
	for suffix, reason := range removedSuffixes {
		if strings.HasSuffix(relative, suffix) {
			return Errorf("path '%s': %s", relative, reason)
		}
	}
	kind, stem, ok := KindFromPath(relative)
	if !ok || kind == "" {
		return Errorf("path '%s' does not name a document kind (.agent.tfer, .skill.tfer, ...)", relative)
	}
	for _, segment := range strings.Split(stem, "/") {
		if segment == ".." || !pathSegment.MatchString(segment) {
			return Errorf("path '%s' has segment '%s'; identity segments use lowercase letters, digits, '.', and '-'", relative, segment)
		}
	}
	return nil
}

// DeriveID derives the identity of a document from its package and
// source-relative path.
func DeriveID(packageName, version, relative string) string {
	_, stem, _ := KindFromPath(relative)
	return packageName + "/" + stem + "@" + version
}

// PackageOptions supplies what the loader needs to resolve package-qualified
// references: the exact version of each dependency the package declares.
type PackageOptions struct {
	Dependencies map[string]string
}

// Package is a loaded source package: its manifest, the documents in the
// closure of its plugins and exports, and the references it makes into
// dependencies.
type Package struct {
	Project   *Project
	Root      string
	Documents map[string]*Document
	// Files are the source-relative document paths in the closure, sorted.
	Files []string
	// SkillFiles are the source-relative paths of the files member skills
	// ship, sorted.
	SkillFiles []string
	// Qualified maps a dependency package name to the identities this
	// package references in it; each must be exported by that package.
	Qualified map[string][]string
	// Exports are the identities of the manifest's exports, sorted.
	Exports []string
	// OwnPlugins are the identities of the plugins the package contains,
	// sorted.
	OwnPlugins []string
	// DependencyPlugins are the plugins the manifest ships from direct
	// dependencies, sorted by identity.
	DependencyPlugins []DependencyPlugin
}

// DependencyPlugin is a manifest entry that ships a dependency's plugin.
type DependencyPlugin struct {
	Package string
	ID      string
}

type pendingRef struct {
	path string
	from string
}

type packageLoader struct {
	root       string
	project    *Project
	deps       map[string]string
	queue      []pendingRef
	seen       map[string]bool
	qualified  map[string]map[string]bool
	skillFiles map[string]bool
}

// LoadPackage loads the closure of a package: every document reachable from
// the manifest's plugins and exports, and the files its skills ship.
// Unreferenced files are not source members.
func LoadPackage(sourceDir string, options PackageOptions) (*Package, error) {
	root, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, Errorf("Source directory not found: %s", sourceDir)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		return nil, Errorf("Source directory not found: %s", root)
	}
	project, err := LoadProject(root)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, Errorf("%s has no %s; a package root needs a manifest with schemaVersion 7", root, ManifestFile)
	}
	l := &packageLoader{
		root:       root,
		project:    project,
		deps:       options.Dependencies,
		seen:       map[string]bool{},
		qualified:  map[string]map[string]bool{},
		skillFiles: map[string]bool{},
	}
	if l.deps == nil {
		l.deps = project.Dependencies
	}
	ownPlugins := []string{}
	dependencyPlugins := []DependencyPlugin{}
	for _, entry := range project.Plugins {
		pkg, p := SplitPluginEntry(entry)
		if pkg == "" {
			l.enqueue(p, ManifestFile)
			ownPlugins = append(ownPlugins, DeriveID(project.Name, project.Version, p))
			continue
		}
		version, declared := l.deps[pkg]
		if !declared {
			return nil, Errorf("%s: plugin '%s' names package %s, which is not a dependency", ManifestFile, entry, pkg)
		}
		dependencyPlugins = append(dependencyPlugins, DependencyPlugin{Package: pkg, ID: DeriveID(pkg, version, p)})
	}
	sort.Strings(ownPlugins)
	sort.Slice(dependencyPlugins, func(i, j int) bool { return dependencyPlugins[i].ID < dependencyPlugins[j].ID })
	exports := []string{}
	for _, p := range project.Exports {
		l.enqueue(p, ManifestFile)
		exports = append(exports, DeriveID(project.Name, project.Version, p))
	}
	sort.Strings(exports)
	documents := map[string]*Document{}
	files := []string{}
	for len(l.queue) > 0 {
		next := l.queue[0]
		l.queue = l.queue[1:]
		doc, err := l.load(next)
		if err != nil {
			return nil, err
		}
		if prior, duplicate := documents[doc.ID]; duplicate {
			return nil, Errorf("%s and %s derive the same identity %s; rename one", prior.Path, doc.Path, doc.ID)
		}
		documents[doc.ID] = doc
		files = append(files, next.path)
	}
	sort.Strings(files)
	qualified := map[string][]string{}
	for pkg, ids := range l.qualified {
		qualified[pkg] = SortedKeys(ids)
	}
	return &Package{
		Project: project, Root: root, Documents: documents, Files: files,
		SkillFiles: SortedKeys(l.skillFiles), Qualified: qualified, Exports: exports,
		OwnPlugins: ownPlugins, DependencyPlugins: dependencyPlugins,
	}, nil
}

func (l *packageLoader) enqueue(relative, from string) {
	if l.seen[relative] {
		return
	}
	l.seen[relative] = true
	l.queue = append(l.queue, pendingRef{path: relative, from: from})
}

// ReadDocument reads one document file from a package root, applying the
// canonical text normalization (BOM stripped, CRLF normalized to LF).
func ReadDocument(root, relative string) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", Errorf("referenced file does not exist or is not a regular file: %s", relative)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", Errorf("%s: %s", relative, err)
	}
	return NormalizeText(string(raw)), nil
}

// ReadSkillFile reads a skill file: normalized text when its bytes are valid
// UTF-8, the exact bytes otherwise.
func ReadSkillFile(root, relative string) ([]byte, bool, error) {
	full := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, Errorf("skill file does not exist or is not a regular file: %s", relative)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, false, Errorf("%s: %s", relative, err)
	}
	if utf8.Valid(raw) {
		return []byte(NormalizeText(string(raw))), true, nil
	}
	return raw, false, nil
}

func (l *packageLoader) load(ref pendingRef) (*Document, error) {
	if err := ValidSourcePath(ref.path); err != nil {
		return nil, Errorf("%s: %s", ref.from, err.(*Error).Message)
	}
	text, err := ReadDocument(l.root, ref.path)
	if err != nil {
		return nil, Errorf("%s: %s", ref.from, err.(*Error).Message)
	}
	return ParseDocument(l.project.Name, l.project.Version, ref.path, text, l)
}

// refSink receives the references a document makes so a loader can follow
// them; a nil sink (single-document checks) only validates their syntax.
type refSink interface {
	local(relative, from string)
	qualifiedRef(pkg, id string)
	dependencyVersion(pkg string) (string, bool)
	skillFile(relative string) ([]byte, bool, error)
}

func (l *packageLoader) local(relative, from string) { l.enqueue(relative, from) }

func (l *packageLoader) qualifiedRef(pkg, id string) {
	if l.qualified[pkg] == nil {
		l.qualified[pkg] = map[string]bool{}
	}
	l.qualified[pkg][id] = true
}

func (l *packageLoader) dependencyVersion(pkg string) (string, bool) {
	version, ok := l.deps[pkg]
	return version, ok
}

func (l *packageLoader) skillFile(relative string) ([]byte, bool, error) {
	data, text, err := ReadSkillFile(l.root, relative)
	if err != nil {
		return nil, false, err
	}
	l.skillFiles[relative] = true
	return data, text, nil
}

// ParseDocument parses and validates one document in isolation. References
// are converted to identities; a non-nil sink is told about each one so a
// loader can follow local references and read skill files.
func ParseDocument(packageName, version, relative, text string, sink refSink) (*Document, error) {
	kind, _, ok := KindFromPath(relative)
	if !ok {
		return nil, Errorf("%s: unknown document kind", relative)
	}
	frontmatter, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, Errorf("%s: %s", relative, err.(*Error).Message)
	}
	node, err := tferlex.Parse(frontmatter)
	if err != nil {
		if grammar, ok := err.(*tferlex.Error); ok && grammar.Line > 0 {
			// Frontmatter line N is file line N+1 (after the opening fence).
			return nil, Errorf("%s:%d: %s", relative, grammar.Line+1, grammar.Message)
		}
		return nil, Errorf("%s: %s", relative, err)
	}
	doc := &Document{
		Kind: kind, Path: relative, Package: packageName,
		ID: DeriveID(packageName, version, relative),
	}
	d := &documentDecoder{
		fieldDecoder: fieldDecoder{file: relative},
		doc:          doc,
		sink:         sink,
		packageName:  packageName,
		version:      version,
	}
	if node != nil {
		shift(node, 1)
	}
	if err := d.decodeKind(node); err != nil {
		return nil, err
	}
	if err := d.applyBody(body); err != nil {
		return nil, err
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// shift converts frontmatter-relative line numbers to file line numbers.
func shift(n *tferlex.Node, offset int) {
	if n.Line > 0 {
		n.Line += offset
	}
	if n.KeyLine > 0 {
		n.KeyLine += offset
	}
	for _, item := range n.Items {
		shift(item, offset)
	}
}

type documentDecoder struct {
	fieldDecoder
	doc         *Document
	sink        refSink
	packageName string
	version     string
}

// ref converts one reference to an identity. A reference is a
// package-relative document path, or `<package>:<path>` into a declared
// dependency. allowed lists the kinds the field accepts.
func (d *documentDecoder) ref(n *tferlex.Node, raw string, allowed ...string) (string, error) {
	target := raw
	pkg := ""
	if idx := strings.Index(raw, ":"); idx >= 0 {
		pkg, target = raw[:idx], raw[idx+1:]
		if !packageName.MatchString(pkg) {
			return "", d.errorf(n, "reference '%s' names an invalid package", raw)
		}
	}
	if err := ValidSourcePath(target); err != nil {
		return "", d.errorf(n, "%s", err.(*Error).Message)
	}
	kind, _, _ := KindFromPath(target)
	permitted := false
	for _, k := range allowed {
		if k == kind {
			permitted = true
			break
		}
	}
	if !permitted {
		names := make([]string, len(allowed))
		for i, k := range allowed {
			names[i] = KindSuffix(k)
		}
		return "", d.errorf(n, "'%s' must reference a %s document, got '%s'", n.Key, strings.Join(names, " or "), raw)
	}
	if kind == "plugin" {
		return "", d.errorf(n, "plugins are listed by the manifest and cannot be referenced")
	}
	if pkg == "" {
		if d.sink != nil {
			d.sink.local(target, d.doc.Path)
		}
		return DeriveID(d.packageName, d.version, target), nil
	}
	if pkg == d.packageName {
		return "", d.errorf(n, "reference '%s' names this package; write the path without a package prefix", raw)
	}
	version := "0.0.0"
	if d.sink != nil {
		v, ok := d.sink.dependencyVersion(pkg)
		if !ok {
			return "", d.errorf(n, "reference '%s' names package %s, which the manifest does not declare as a dependency", raw, pkg)
		}
		version = v
	}
	id := DeriveID(pkg, version, target)
	if d.sink != nil {
		d.sink.qualifiedRef(pkg, id)
	}
	return id, nil
}

func (d *documentDecoder) refInto(target *string, allowed ...string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		raw, null, err := d.text(n)
		if err != nil {
			return err
		}
		if null {
			return d.errorf(n, "'%s' needs a reference", n.Key)
		}
		id, err := d.ref(n, raw, allowed...)
		*target = id
		return err
	}
}

func (d *documentDecoder) refList(n *tferlex.Node, allowed ...string) ([]string, error) {
	items, err := d.stringList(n)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	seen := map[string]bool{}
	for i, raw := range items {
		at := n
		if i < len(n.Items) {
			at = n.Items[i]
			at.Key = n.Key
		}
		id, err := d.ref(at, raw, allowed...)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, d.errorf(at, "'%s' lists %s more than once", n.Key, raw)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func (d *documentDecoder) refListInto(target *[]string, allowed ...string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		ids, err := d.refList(n, allowed...)
		*target = ids
		return err
	}
}

// refMap decodes a mapping from names to references, such as parameters
// (name to context type) or with (name to data document).
func (d *documentDecoder) refMap(target *map[string]string, nameRule func(string) bool, nameMessage string, allowed ...string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		values, err := d.stringMap(n)
		if err != nil {
			return err
		}
		out := map[string]string{}
		for _, key := range SortedKeys(values) {
			if !nameRule(key) {
				return d.errorf(child(n, key), "'%s' %s", key, nameMessage)
			}
			id, err := d.ref(child(n, key), values[key], allowed...)
			if err != nil {
				return err
			}
			out[key] = id
		}
		*target = out
		return nil
	}
}

func (d *documentDecoder) jsonSchema(target *string, declared *bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, null, err := d.text(n)
		if err != nil {
			return err
		}
		if null {
			return d.errorf(n, "'%s' needs a JSON Schema document", n.Key)
		}
		if err := validateJSON(value, d.file, n.Key); err != nil {
			return d.errorf(n, "invalid %s JSON: %s", n.Key, strings.TrimPrefix(err.(*Error).Message, d.file+": invalid "+n.Key+": "))
		}
		*target = value
		if declared != nil {
			*declared = true
		}
		return nil
	}
}

func (d *documentDecoder) parametersField() func(*tferlex.Node) error {
	return d.refMap(&d.doc.Parameters, IsParameterName,
		"is not a valid parameter name; use a lowercase letter followed by letters and digits", "contextType")
}

func (d *documentDecoder) withField() func(*tferlex.Node) error {
	return d.refMap(&d.doc.With, IsParameterName,
		"is not a valid parameter name; use a lowercase letter followed by letters and digits", "context")
}

func (d *documentDecoder) decodeKind(n *tferlex.Node) error {
	doc := d.doc
	common := map[string]func(*tferlex.Node) error{
		"displayName": d.stringInto(&doc.DisplayName),
		"description": d.stringInto(&doc.Description),
	}
	with := func(extra map[string]func(*tferlex.Node) error) map[string]func(*tferlex.Node) error {
		fields := map[string]func(*tferlex.Node) error{}
		for k, v := range common {
			fields[k] = v
		}
		for k, v := range extra {
			fields[k] = v
		}
		return fields
	}
	switch doc.Kind {
	case "agent":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"embeds":  d.refListInto(&doc.Embeds, "profile", "agent"),
			"context": d.contextEntries(false),
			"skills":  d.bindings,
			"with":    d.withField(),
			"copilot": d.copilot(false),
		}))
	case "profile":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"embeds":     d.refListInto(&doc.Embeds, "profile"),
			"parameters": d.parametersField(),
			"context":    d.contextEntries(false),
			"skills":     d.bindings,
		}))
	case "capability":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"inputSchema":  d.jsonSchema(&doc.InputSchema, &doc.HasInputSchema),
			"outputSchema": d.jsonSchema(&doc.OutputSchema, &doc.HasOutputSchema),
		}))
	case "skill":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"binds":           d.refInto(&doc.Binds, "capability"),
			"extends":         d.refInto(&doc.Extends, "skill"),
			"parameters":      d.parametersField(),
			"with":            d.withField(),
			"inputSchema":     d.jsonSchema(&doc.InputSchema, &doc.HasInputSchema),
			"outputSchema":    d.jsonSchema(&doc.OutputSchema, &doc.HasOutputSchema),
			"context":         d.contextEntries(true),
			"files":           d.files,
			"requiresServers": d.refListInto(&doc.RequiresServers, "server"),
			"variants":        d.variants,
			"copilot":         d.copilot(true),
		}))
	case "server":
		server := &ServerConfig{}
		doc.Server = server
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"transport": d.stringInto(&server.Transport),
			"command":   d.stringInto(&server.Command),
			"args": func(n *tferlex.Node) error {
				items, err := d.stringList(n)
				server.Args = items
				return err
			},
			"env": func(n *tferlex.Node) error {
				values, err := d.stringMap(n)
				server.Env = values
				return err
			},
			"cwd": d.stringInto(&server.Cwd),
			"url": d.stringInto(&server.URL),
			"headers": func(n *tferlex.Node) error {
				values, err := d.stringMap(n)
				server.Headers = values
				return err
			},
		}))
	case "contextType":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"instanceName": d.stringInto(&doc.InstanceName),
			"fields":       d.contextTypeFields,
		}))
	case "context":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"parameters":  d.parametersField(),
			"contextType": d.refInto(&doc.ContextType, "contextType"),
			"values": func(n *tferlex.Node) error {
				if isNull(n) {
					doc.Values = map[string]FieldValue{}
					return nil
				}
				if !n.IsMap {
					return d.errorf(n, "'values' must be a mapping from field name to value")
				}
				values := map[string]FieldValue{}
				for _, item := range n.Items {
					if !IsFieldName(item.Key) {
						return d.errorf(item, "field name '%s' must start with a letter and use letters, digits, and '_'", item.Key)
					}
					value, err := d.fieldValue(item)
					if err != nil {
						return err
					}
					values[item.Key] = value
				}
				doc.Values = values
				return nil
			},
		}))
	case "plugin":
		return d.decode(n, map[string]func(*tferlex.Node) error{
			"description": d.stringInto(&doc.Description),
			"agents":      d.refListInto(&doc.PluginAgents, "agent"),
			"profiles":    d.refListInto(&doc.PluginProfiles, "profile"),
			"skills":      d.refListInto(&doc.PluginSkills, "skill"),
			"modes":       d.stringListInto(&doc.PluginModes),
		})
	}
	return Errorf("%s: unknown document kind", d.file)
}

// contextEntries decodes a held-document list. Skills may give an entry as a
// mapping {context, render}; agents and profiles list paths only.
func (d *documentDecoder) contextEntries(allowRender bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		if isNull(n) {
			return nil
		}
		if !n.IsSeq {
			return d.errorf(n, "'context' must be a list")
		}
		seen := map[string]bool{}
		for _, item := range n.Items {
			item.Key = "context"
			ref := ContextRef{Render: "inline"}
			switch {
			case item.IsScalar && item.Value.Kind != tferlex.KindNull:
				id, err := d.ref(item, item.Value.Text, "context")
				if err != nil {
					return err
				}
				ref.ID = id
			case item.IsMap && allowRender:
				err := d.decode(item, map[string]func(*tferlex.Node) error{
					"context": d.refInto(&ref.ID, "context"),
					"render":  d.stringInto(&ref.Render),
				})
				if err != nil {
					return err
				}
				if ref.ID == "" {
					return d.errorf(item, "a context entry mapping must name a context document")
				}
				if ref.Render != "inline" && ref.Render != "file" {
					return d.errorf(item, "render must be inline or file, got '%s'", ref.Render)
				}
			case item.IsMap:
				return d.errorf(item, "only a skill may choose how a document renders; list the document's path")
			default:
				return d.errorf(item, "a context entry is a document path")
			}
			if seen[ref.ID] {
				return d.errorf(item, "'context' lists %s more than once", ref.ID)
			}
			seen[ref.ID] = true
			d.doc.Context = append(d.doc.Context, ref)
		}
		return nil
	}
}

// bindings decodes an agent or profile skill list. An entry is a skill path,
// or a mapping {skill, capability, required}; a capability reference may
// name a capability document or a skill (meaning that skill's capability).
func (d *documentDecoder) bindings(n *tferlex.Node) error {
	if isNull(n) {
		return nil
	}
	if !n.IsSeq {
		return d.errorf(n, "'skills' must be a list")
	}
	for _, item := range n.Items {
		item.Key = "skills"
		var binding SkillBinding
		switch {
		case item.IsScalar && item.Value.Kind != tferlex.KindNull:
			id, err := d.ref(item, item.Value.Text, "skill")
			if err != nil {
				return err
			}
			binding.Ref = id
		case item.IsMap:
			var capability string
			hasCapability := false
			err := d.decode(item, map[string]func(*tferlex.Node) error{
				"skill": d.refInto(&binding.Ref, "skill"),
				"capability": func(c *tferlex.Node) error {
					hasCapability = true
					return d.refInto(&capability, "capability", "skill")(c)
				},
				"required": d.boolInto(&binding.Required),
			})
			if err != nil {
				return err
			}
			if hasCapability {
				binding.Capability = &capability
			}
		default:
			return d.errorf(item, "a skills entry is a skill path or a mapping")
		}
		d.doc.Skills = append(d.doc.Skills, binding)
	}
	return nil
}

var skillFileRoots = map[string]bool{"references": true, "scripts": true, "assets": true}

// files decodes a skill's plain files. An entry is a package path, or a
// mapping {path, as}.
func (d *documentDecoder) files(n *tferlex.Node) error {
	if isNull(n) {
		return nil
	}
	if !n.IsSeq {
		return d.errorf(n, "'files' must be a list")
	}
	for _, item := range n.Items {
		item.Key = "files"
		file := SkillFile{Package: d.packageName}
		switch {
		case item.IsScalar && item.Value.Kind != tferlex.KindNull:
			file.Source = item.Value.Text
		case item.IsMap:
			err := d.decode(item, map[string]func(*tferlex.Node) error{
				"path": d.stringInto(&file.Source),
				"as":   d.stringInto(&file.As),
			})
			if err != nil {
				return err
			}
		default:
			return d.errorf(item, "a files entry is a path or a mapping {path, as}")
		}
		if !cleanRelative(file.Source) {
			return d.errorf(item, "file '%s' must be a clean path relative to the package root", file.Source)
		}
		if strings.HasSuffix(file.Source, ".tfer") {
			return d.errorf(item, "file '%s' is a .tfer document; skill files are plain files", file.Source)
		}
		switch strings.SplitN(file.Source, "/", 2)[0] {
		case "dist", "bin", "obj", ".git":
			return d.errorf(item, "file '%s' is beneath a generated or excluded directory", file.Source)
		}
		if file.As == "" {
			file.As = "references/" + path.Base(file.Source)
		}
		if !cleanRelative(file.As) || !strings.Contains(file.As, "/") || !skillFileRoots[strings.SplitN(file.As, "/", 2)[0]] {
			return d.errorf(item, "destination '%s' must be a clean path beneath references/, scripts/, or assets/", file.As)
		}
		if d.sink != nil {
			data, text, err := d.sink.skillFile(file.Source)
			if err != nil {
				return d.errorf(item, "%s", err.(*Error).Message)
			}
			file.Data, file.Text = data, text
		}
		d.doc.Files = append(d.doc.Files, file)
	}
	return nil
}

func (d *documentDecoder) copilot(skill bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		c := &d.doc.Copilot
		tools := func(target *[]string) func(*tferlex.Node) error {
			return func(n *tferlex.Node) error {
				items, err := d.stringList(n)
				if err != nil {
					return err
				}
				if len(items) == 0 {
					return d.errorf(n, "'%s' must list at least one tool; omit it otherwise", n.Key)
				}
				*target = items
				return nil
			}
		}
		fields := map[string]func(*tferlex.Node) error{
			"userInvocable":          d.optionalBool(&c.UserInvocable),
			"disableModelInvocation": d.optionalBool(&c.DisableModelInvocation),
		}
		if skill {
			fields["argumentHint"] = d.optionalString(&c.ArgumentHint)
			fields["allowedTools"] = tools(&c.AllowedTools)
		} else {
			fields["model"] = d.optionalString(&c.Model)
			fields["tools"] = tools(&c.Tools)
		}
		return d.decode(n, fields)
	}
}

func (d *documentDecoder) variants(n *tferlex.Node) error {
	if !n.IsMap {
		return d.errorf(n, "'variants' must be a mapping from mode name to variant")
	}
	variants := map[string]Variant{}
	for _, item := range n.Items {
		if item.Key != "manual" && item.Key != "pipeline" {
			return d.errorf(item, "variant mode '%s' is not supported; modes are manual and pipeline", item.Key)
		}
		var v Variant
		err := d.decode(item, map[string]func(*tferlex.Node) error{
			"instructions":    d.stringInto(&v.Instructions),
			"requiresServers": d.refListInto(&v.RequiresServers, "server"),
		})
		if err != nil {
			return err
		}
		variants[item.Key] = v
	}
	d.doc.Variants = variants
	return nil
}

var fieldTypes = map[string]bool{"string": true, "text": true, "boolean": true, "integer": true, "list<string>": true}

func (d *documentDecoder) contextTypeFields(n *tferlex.Node) error {
	if isNull(n) {
		return nil
	}
	if !n.IsMap {
		return d.errorf(n, "'fields' must be a mapping")
	}
	for _, item := range n.Items {
		if !IsFieldName(item.Key) {
			return d.errorf(item, "field name '%s' must start with a letter and use letters, digits, and '_'", item.Key)
		}
		field := ContextField{Name: item.Key}
		var choicesNode *tferlex.Node
		err := d.decode(item, map[string]func(*tferlex.Node) error{
			"type":        d.stringInto(&field.Type),
			"required":    d.boolInto(&field.Required),
			"displayName": d.stringInto(&field.DisplayName),
			"description": d.stringInto(&field.Description),
			"choices": func(c *tferlex.Node) error {
				choicesNode = c
				return d.stringListInto(&field.Choices)(c)
			},
			"default": func(v *tferlex.Node) error {
				value, err := d.fieldValue(v)
				if err != nil {
					return err
				}
				field.HasDefault = true
				field.Default = value
				return nil
			},
		})
		if err != nil {
			return err
		}
		switch {
		case field.Type == "":
			return d.errorf(item, "field '%s' must declare a type", item.Key)
		case field.Type == "number" || field.Type == "decimal":
			return d.errorf(item, "field '%s': type '%s' does not exist in version 7; use integer or string", item.Key, field.Type)
		case strings.HasPrefix(field.Type, "map<"):
			return d.errorf(item, "field '%s': map types were removed in version 7", item.Key)
		case !fieldTypes[field.Type]:
			return d.errorf(item, "field '%s': unknown type '%s'; use string, text, boolean, integer, or list<string>", item.Key, field.Type)
		}
		if !singleLine(field.DisplayName) || !singleLine(field.Description) {
			return d.errorf(item, "field '%s': displayName and description must be single lines", item.Key)
		}
		if choicesNode != nil {
			if field.Type != "string" {
				return d.errorf(choicesNode, "field '%s': choices apply only to string fields", item.Key)
			}
			if len(field.Choices) == 0 {
				return d.errorf(choicesNode, "field '%s': choices must list at least one value", item.Key)
			}
			seen := map[string]bool{}
			for _, choice := range field.Choices {
				if seen[choice] {
					return d.errorf(choicesNode, "field '%s': choice '%s' is listed more than once", item.Key, choice)
				}
				seen[choice] = true
			}
		}
		if field.Required && field.HasDefault {
			return d.errorf(item, "field '%s' is required, so it cannot declare a default", item.Key)
		}
		if field.HasDefault {
			if _, err := field.Check(field.Default); err != nil {
				return d.errorf(item, "field '%s': default %s", item.Key, err.(*Error).Message)
			}
		}
		d.doc.Fields = append(d.doc.Fields, field)
	}
	return nil
}

func (d *documentDecoder) applyBody(body string) error {
	doc := d.doc
	hasBody := strings.TrimSpace(body) != ""
	switch doc.Kind {
	case "skill":
		if hasBody {
			if len(doc.Variants) != 0 {
				return Errorf("%s: a multimodal skill has no single body; put instructions inside each variant", d.file)
			}
			doc.Instructions = body
		}
	case "context":
		if doc.ContextType != "" {
			if hasBody {
				return Errorf("%s: a data document (one with a contextType) has no body; its values reach output through field references", d.file)
			}
			return nil
		}
		if !hasBody {
			return Errorf("%s: a document needs Markdown in its body; to hold typed values, declare a contextType", d.file)
		}
		doc.Content = body
	case "agent":
		if hasBody {
			doc.Objectives = body
		}
	default:
		if hasBody {
			return Errorf("%s: a %s document has no body", d.file, doc.Kind)
		}
	}
	return nil
}

var (
	commandName   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._+-]*$`)
	pluginPathVar = regexp.MustCompile(`\$\{PLUGIN_(?:ROOT|DATA)\}`)
)

func (d *documentDecoder) validate() error {
	doc := d.doc
	file := d.file
	if !singleLine(doc.DisplayName) {
		return Errorf("%s: displayName must be a single line", file)
	}
	if !singleLine(doc.Description) {
		return Errorf("%s: description must be a single line without control characters", file)
	}
	switch doc.Kind {
	case "agent", "skill", "plugin":
		if strings.TrimSpace(doc.Description) == "" {
			return Errorf("%s: a %s requires a description; hosts use it to decide when to use the %s", file, doc.Kind, doc.Kind)
		}
	}
	for _, binding := range doc.Skills {
		if strings.TrimSpace(binding.Ref) != "" {
			continue
		}
		if binding.Capability == nil {
			return Errorf("%s: a skills entry must name a skill, or a capability to declare an abstract requirement", file)
		}
		if !binding.Required {
			return Errorf("%s: an abstract requirement must set required: true; an unbound optional capability has no effect", file)
		}
	}
	switch doc.Kind {
	case "skill":
		if doc.Binds != "" && doc.Extends != "" {
			return Errorf("%s: a skill binds a capability or extends a skill, not both; an extension inherits its base's capability", file)
		}
		if doc.Extends == "" && strings.TrimSpace(doc.Instructions) == "" && len(doc.Variants) == 0 {
			return Errorf("%s: a skill needs instructions: write them as the body, or declare variants", file)
		}
		for mode, v := range doc.Variants {
			if strings.TrimSpace(v.Instructions) == "" {
				return Errorf("%s: variant '%s' must set instructions", file, mode)
			}
		}
		if len(doc.With) > 0 {
			if doc.Extends == "" {
				return Errorf("%s: 'with' instantiates a template, so the skill must extend one", file)
			}
			if len(doc.Parameters) > 0 {
				return Errorf("%s: a skill that binds its template's parameters with 'with' cannot declare parameters of its own", file)
			}
		}
		doc.OwnParameters = copyMap(doc.Parameters)
		doc.OwnWith = copyMap(doc.With)
	case "server":
		if err := validateServer(doc); err != nil {
			return err
		}
	case "contextType":
		if doc.InstanceName != "" {
			var named *ContextField
			for i := range doc.Fields {
				if doc.Fields[i].Name == doc.InstanceName {
					named = &doc.Fields[i]
				}
			}
			if named == nil {
				return Errorf("%s: instanceName names '%s', which is not a field", file, doc.InstanceName)
			}
			if named.Type != "string" {
				return Errorf("%s: instanceName field '%s' must be a string", file, named.Name)
			}
			if !named.Required && !named.HasDefault {
				return Errorf("%s: instanceName field '%s' must be required or have a default", file, named.Name)
			}
			if named.HasDefault && !IsHostName(named.Default.Scalar) {
				return Errorf("%s: the default of instanceName field '%s' must be lowercase letters, digits, and single hyphens", file, named.Name)
			}
			for _, choice := range named.Choices {
				if !IsHostName(choice) {
					return Errorf("%s: choice '%s' of instanceName field '%s' must be lowercase letters, digits, and single hyphens", file, choice, named.Name)
				}
			}
		}
	case "context":
		if doc.ContextType != "" && len(doc.Parameters) > 0 {
			return Errorf("%s: a data document declares values, not parameters", file)
		}
		if doc.ContextType == "" && doc.Values != nil {
			return Errorf("%s: 'values' needs a contextType that declares the fields", file)
		}
		if doc.ContextType != "" && doc.Values == nil {
			doc.Values = map[string]FieldValue{}
		}
	case "plugin":
		if len(doc.PluginAgents)+len(doc.PluginProfiles)+len(doc.PluginSkills) == 0 {
			return Errorf("%s: a plugin must link at least one agent, profile, or skill", file)
		}
		seen := map[string]bool{}
		for _, mode := range doc.PluginModes {
			if mode != "manual" && mode != "pipeline" {
				return Errorf("%s: plugin mode '%s' is not supported; use manual or pipeline", file, mode)
			}
			if seen[mode] {
				return Errorf("%s: plugin mode '%s' is listed more than once", file, mode)
			}
			seen[mode] = true
		}
		if len(doc.PluginModes) == 0 {
			doc.PluginModes = []string{"manual"}
		}
		sort.Strings(doc.PluginModes)
	}
	return nil
}

func validateServer(doc *Document) error {
	file := doc.Path
	s := doc.Server
	name := Leaf(doc.ID)
	if !serverName.MatchString(name) || len(name) > 64 {
		return Errorf("%s: server name '%s' must be at least two hyphen-separated lowercase segments, such as 'acme-tickets', so it cannot replace a user's own server", file, name)
	}
	hasPlaceholder := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(pluginPathVar.ReplaceAllString(value, ""), "${") {
				return true
			}
		}
		return false
	}
	switch s.Transport {
	case "stdio":
		if s.URL != "" || s.Headers != nil {
			return Errorf("%s: a stdio server takes command, args, env, and cwd, not url or headers", file)
		}
		if s.Command == "" {
			return Errorf("%s: a stdio server requires a command", file)
		}
		if strings.HasPrefix(s.Command, "./") {
			if !cleanRelative(strings.TrimPrefix(s.Command, "./")) {
				return Errorf("%s: command '%s' must be a clean ./ path without '..'", file, s.Command)
			}
		} else if !commandName.MatchString(s.Command) {
			return Errorf("%s: command '%s' must be a bare executable name or a ./ path", file, s.Command)
		}
		if s.Cwd != "" {
			cwd := s.Cwd
			switch {
			case strings.HasPrefix(cwd, "./"):
				if !cleanRelative(strings.TrimPrefix(cwd, "./")) {
					return Errorf("%s: cwd '%s' must be a clean ./ path", file, cwd)
				}
			case cwd == "${PLUGIN_ROOT}" || cwd == "${PLUGIN_DATA}":
			case strings.HasPrefix(cwd, "${PLUGIN_ROOT}/") || strings.HasPrefix(cwd, "${PLUGIN_DATA}/"):
				rest := cwd[strings.Index(cwd, "}/")+2:]
				if !cleanRelative(rest) {
					return Errorf("%s: cwd '%s' must continue with a clean path", file, cwd)
				}
			default:
				return Errorf("%s: cwd must be a ./ path, ${PLUGIN_ROOT}, or ${PLUGIN_DATA}", file)
			}
		}
		values := append([]string{s.Command}, s.Args...)
		for _, key := range SortedKeys(s.Env) {
			if key == "PLUGIN_ROOT" || key == "PLUGIN_DATA" {
				return Errorf("%s: env must not set %s; the host provides it", file, key)
			}
			values = append(values, s.Env[key])
		}
		if hasPlaceholder(append(values, s.Cwd)...) {
			return Errorf("%s: only ${PLUGIN_ROOT} and ${PLUGIN_DATA} may appear in server values; plugins cannot depend on environment variables", file)
		}
	case "streamable-http":
		if s.Command != "" || s.Args != nil || s.Env != nil || s.Cwd != "" {
			return Errorf("%s: a streamable-http server takes url and headers, not command, args, env, or cwd", file)
		}
		if !strings.HasPrefix(s.URL, "https://") || strings.Contains(strings.SplitN(strings.TrimPrefix(s.URL, "https://"), "/", 2)[0], "@") || len(s.URL) <= len("https://") {
			return Errorf("%s: url must be an absolute HTTPS URL without credentials", file)
		}
		values := []string{s.URL}
		for _, key := range SortedKeys(s.Headers) {
			values = append(values, s.Headers[key])
		}
		if strings.Contains(strings.Join(values, "\n"), "${") {
			return Errorf("%s: remote server values are passed to the host literally; they cannot contain ${...}", file)
		}
	case "":
		return Errorf("%s: a server requires a transport: stdio or streamable-http", file)
	default:
		return Errorf("%s: transport '%s' is not supported; use stdio or streamable-http", file, s.Transport)
	}
	return nil
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// CheckDocument validates one document in isolation, without following
// references. It is the language server's per-buffer diagnostic.
func CheckDocument(relative, text string) error {
	if relative == ManifestFile {
		_, err := ParseProjectManifest(text)
		return err
	}
	if err := ValidSourcePath(relative); err != nil {
		return err
	}
	_, err := ParseDocument("check/package", "0.0.0", relative, NormalizeText(text), nil)
	return err
}

package resource

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/tferlex"
)

// BuiltinTextContextType is the built-in context type of a version 6 context
// document that declares no contextType: a required text body and no fields
// (ADR-0030).
const BuiltinTextContextType = ReservedPackagePrefix + "/text@1.0.0"

// kindSuffixes maps each version 6 document suffix to its kind. The kind of a
// document is decided by its file name, never by a field.
var kindSuffixes = []struct {
	suffix string
	kind   string
}{
	{".agent.tfer", "agent"},
	{".profile.tfer", "profile"},
	{".interface.tfer", "interface"},
	{".capability.tfer", "capability"},
	{".skill.tfer", "skill"},
	{".tool.tfer", "tool"},
	{".contexttype.tfer", "contextType"},
	{".context.tfer", "context"},
	{".plugin.tfer", "plugin"},
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
// and names a version 6 document whose stem segments are identity segments.
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

// DeriveID derives the identity of a version 6 document from its package and
// source-relative path.
func DeriveID(packageName, version, relative string) string {
	_, stem, _ := KindFromPath(relative)
	return packageName + "/" + stem + "@" + version
}

// V6Options supplies what the loader needs to resolve package-qualified
// references: the exact version of each dependency the package declares.
type V6Options struct {
	Dependencies map[string]string
}

// V6Source is a loaded version 6 package: its manifest, the documents in the
// closure of its plugins and exports, and the qualified references it makes
// into dependencies.
type V6Source struct {
	Project   *Project
	Documents map[string]*Document
	// Files are the source-relative document paths in the closure, sorted.
	Files []string
	// Qualified maps a dependency package name to the identities this package
	// references in it; each must be exported by that package.
	Qualified map[string][]string
	// Exports are the identities of the manifest's exports, sorted.
	Exports []string
	// OwnPlugins are the identities of the plugins the package itself
	// contains, sorted.
	OwnPlugins []string
	// DependencyPlugins are the plugins the manifest ships from direct
	// dependencies (ADR-0034), sorted by identity.
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

type loader6 struct {
	root      string
	project   *Project
	deps      map[string]string
	queue     []pendingRef
	seen      map[string]bool
	qualified map[string]map[string]bool
}

// LoadV6 loads the closure of a version 6 package: every document reachable
// from the manifest's plugins and exports. Unreferenced files are not source
// members (ADR-0030).
func LoadV6(sourceDir string, options V6Options) (*V6Source, error) {
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
	if !project.IsV6() {
		return nil, Errorf("%s: a version 6 package requires a %s manifest with schemaVersion 6", root, ManifestFile)
	}
	l := &loader6{
		root:      root,
		project:   project,
		deps:      options.Dependencies,
		seen:      map[string]bool{},
		qualified: map[string]map[string]bool{},
	}
	if l.deps == nil {
		l.deps = project.Dependencies
	}
	ownPlugins := []string{}
	dependencyPlugins := []DependencyPlugin{}
	for _, entry := range project.Plugins {
		pkg, path := SplitPluginEntry(entry)
		if pkg == "" {
			l.enqueue(path, ManifestFile)
			ownPlugins = append(ownPlugins, DeriveID(project.Name, project.Version, path))
			continue
		}
		version, declared := l.deps[pkg]
		if !declared {
			return nil, Errorf("%s: plugin '%s' names package %s, which is not a dependency", ManifestFile, entry, pkg)
		}
		dependencyPlugins = append(dependencyPlugins, DependencyPlugin{Package: pkg, ID: DeriveID(pkg, version, path)})
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
		list := make([]string, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		sort.Strings(list)
		qualified[pkg] = list
	}
	return &V6Source{
		Project: project, Documents: documents, Files: files, Qualified: qualified, Exports: exports,
		OwnPlugins: ownPlugins, DependencyPlugins: dependencyPlugins,
	}, nil
}

func (l *loader6) enqueue(relative, from string) {
	if l.seen[relative] {
		return
	}
	l.seen[relative] = true
	l.queue = append(l.queue, pendingRef{path: relative, from: from})
}

// ReadV6Document reads one document file from a package root, applying the
// canonical text normalization (BOM stripped, CRLF normalized to LF).
func ReadV6Document(root, relative string) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", Errorf("referenced file does not exist or is not a regular file: %s", relative)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", Errorf("%s: %s", relative, err)
	}
	return stripBOM(strings.ReplaceAll(string(raw), "\r\n", "\n")), nil
}

func (l *loader6) load(ref pendingRef) (*Document, error) {
	if err := ValidSourcePath(ref.path); err != nil {
		return nil, Errorf("%s: %s", ref.from, err.(*Error).Message)
	}
	text, err := ReadV6Document(l.root, ref.path)
	if err != nil {
		return nil, Errorf("%s: %s", ref.from, err.(*Error).Message)
	}
	return ParseV6Document(l.project.Name, l.project.Version, ref.path, text, l)
}

// refSink receives the references a document makes so a loader can follow
// them; a nil sink (single-document checks) only validates their syntax.
type refSink interface {
	local(relative, from string)
	qualifiedRef(pkg, id string)
	dependencyVersion(pkg string) (string, bool)
}

func (l *loader6) local(relative, from string) { l.enqueue(relative, from) }

func (l *loader6) qualifiedRef(pkg, id string) {
	if l.qualified[pkg] == nil {
		l.qualified[pkg] = map[string]bool{}
	}
	l.qualified[pkg][id] = true
}

func (l *loader6) dependencyVersion(pkg string) (string, bool) {
	version, ok := l.deps[pkg]
	return version, ok
}

// ParseV6Document parses and validates one version 6 document in isolation.
// References are converted to identities; a non-nil sink is told about each
// one so a loader can follow local references.
func ParseV6Document(packageName, version, relative, text string, sink refSink) (*Document, error) {
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
	doc := NewDocument()
	doc.SchemaVersion = 6
	doc.Kind = kind
	doc.Path = relative
	doc.Package = packageName
	doc.ID = DeriveID(packageName, version, relative)
	d := &document6{
		fieldDecoder6: fieldDecoder6{file: relative},
		doc:           doc,
		sink:          sink,
		packageName:   packageName,
		version:       version,
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

type document6 struct {
	fieldDecoder6
	doc         *Document
	sink        refSink
	packageName string
	version     string
}

// ref converts one reference to an identity. A reference is a
// package-relative document path, or `<package>:<path>` into a declared
// dependency. allowed lists the kinds the field accepts.
func (d *document6) ref(n *tferlex.Node, raw string, allowed ...string) (string, error) {
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
	version := ""
	if d.sink != nil {
		v, ok := d.sink.dependencyVersion(pkg)
		if !ok {
			return "", d.errorf(n, "reference '%s' names package %s, which the manifest does not declare as a dependency", raw, pkg)
		}
		version = v
	} else {
		version = "0.0.0"
	}
	id := DeriveID(pkg, version, target)
	if d.sink != nil {
		d.sink.qualifiedRef(pkg, id)
	}
	return id, nil
}

func (d *document6) refInto(target *string, allowed ...string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		raw, isNull, err := d.text(n)
		if err != nil || isNull {
			if isNull {
				return d.errorf(n, "'%s' needs a reference", n.Key)
			}
			return err
		}
		id, err := d.ref(n, raw, allowed...)
		*target = id
		return err
	}
}

func (d *document6) refList(n *tferlex.Node, allowed ...string) ([]string, error) {
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

func (d *document6) refListInto(target *[]string, allowed ...string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		ids, err := d.refList(n, allowed...)
		*target = ids
		return err
	}
}

func (d *document6) jsonSchema(target *string, declared *bool) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, isNull, err := d.text(n)
		if err != nil {
			return err
		}
		if isNull {
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

func (d *document6) decodeKind(n *tferlex.Node) error {
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
	case "agent", "profile":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"embeds":  d.refListInto(&doc.Embeds, "profile", "agent"),
			"context": d.refListInto(&doc.Context, "context"),
			"slots": func(n *tferlex.Node) error {
				values, err := d.stringMap(n)
				if err != nil {
					return err
				}
				for _, key := range sortedMapKeys(values) {
					if !slotName.MatchString(key) {
						return d.errorf(n, "slot name '%s' must be an ASCII identifier", key)
					}
					id, err := d.ref(child(n, key), values[key], "context")
					if err != nil {
						return err
					}
					doc.Slots[key] = id
				}
				return nil
			},
			"allowedContextTypes": func(n *tferlex.Node) error {
				doc.HasAllowedContextTypes = true
				return d.refListInto(&doc.AllowedContextTypes, "contextType")(n)
			},
			"skills": d.bindings,
		}))
	case "interface":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"embeds": d.refListInto(&doc.Embeds, "interface"),
			"requiresSlots": func(n *tferlex.Node) error {
				values, err := d.stringMap(n)
				if err != nil {
					return err
				}
				for _, key := range sortedMapKeys(values) {
					if !slotName.MatchString(key) {
						return d.errorf(n, "required slot name '%s' must be an ASCII identifier", key)
					}
					id, err := d.ref(child(n, key), values[key], "contextType")
					if err != nil {
						return err
					}
					doc.RequiredSlotTypes[key] = id
					doc.RequiresSlots = append(doc.RequiresSlots, key)
				}
				sort.Strings(doc.RequiresSlots)
				return nil
			},
			"requiresCapabilities": d.refListInto(&doc.RequiresCapabilities, "capability", "skill"),
		}))
	case "capability":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"visibility":   d.stringInto(&doc.Visibility),
			"inputSchema":  d.jsonSchema(&doc.InputSchema, nil),
			"outputSchema": d.jsonSchema(&doc.OutputSchema, nil),
		}))
	case "tool":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"inputSchema":  d.jsonSchema(&doc.InputSchema, nil),
			"outputSchema": d.jsonSchema(&doc.OutputSchema, nil),
		}))
	case "skill":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"binds":                d.refInto(&doc.Binds, "capability"),
			"extends":              d.refInto(&doc.Extends, "skill"),
			"sealed":               d.boolInto(&doc.Sealed),
			"inputSchema":          d.jsonSchema(&doc.InputSchema, &doc.HasInputSchema),
			"outputSchema":         d.jsonSchema(&doc.OutputSchema, &doc.HasOutputSchema),
			"requiresContextTypes": d.refListInto(&doc.RequiresContextTypes, "contextType"),
			"requiresTools":        d.refListInto(&doc.RequiresTools, "tool"),
			"context":              d.refListInto(&doc.Context, "context"),
			"variants":             d.variants,
		}))
	case "contextType":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"embeds": d.refListInto(&doc.Embeds, "contextType"),
			"fields": d.contextTypeFields,
			"body": func(n *tferlex.Node) error {
				body := &ContextBody{Type: "text"}
				if err := d.decode(n, map[string]func(*tferlex.Node) error{
					"type":     d.stringInto(&body.Type),
					"required": d.boolInto(&body.Required),
				}); err != nil {
					return err
				}
				if body.Type != "text" {
					return d.errorf(n, "a context body type must be text")
				}
				doc.ContextBody = body
				return nil
			},
		}))
	case "context":
		return d.decode(n, with(map[string]func(*tferlex.Node) error{
			"contextType": d.refInto(&doc.ContextType, "contextType"),
			"values": func(n *tferlex.Node) error {
				if n.IsScalar && n.Value.Kind == tferlex.KindNull {
					return nil
				}
				if !n.IsMap {
					return d.errorf(n, "'values' must be a mapping")
				}
				values := map[string]FieldValue{}
				for _, item := range n.Items {
					if !slotName.MatchString(item.Key) {
						return d.errorf(item, "context field name '%s' must be an ASCII identifier", item.Key)
					}
					value, err := d.fieldValue(item)
					if err != nil {
						return err
					}
					values[item.Key] = value
				}
				doc.ContextFields = values
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

// bindings decodes an agent or profile skill list. An entry is a skill path,
// or a mapping {skill, capability, sealed, required}; a capability reference
// may name a capability document or a skill (meaning that skill's
// capability).
func (d *document6) bindings(n *tferlex.Node) error {
	if n.IsScalar && n.Value.Kind == tferlex.KindNull {
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
				"sealed":   d.boolInto(&binding.Sealed),
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

var modeName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func (d *document6) variants(n *tferlex.Node) error {
	if !n.IsMap {
		return d.errorf(n, "'variants' must be a mapping from mode name to variant")
	}
	variants := map[string]Variant{}
	for _, item := range n.Items {
		if !modeName.MatchString(item.Key) {
			return d.errorf(item, "variant mode '%s' must be a lowercase identifier", item.Key)
		}
		var v Variant
		err := d.decode(item, map[string]func(*tferlex.Node) error{
			"instructions":         d.stringInto(&v.Instructions),
			"requiresContextTypes": d.refListInto(&v.RequiresContextTypes, "contextType"),
			"requiresTools":        d.refListInto(&v.RequiresTools, "tool"),
		})
		if err != nil {
			return err
		}
		variants[item.Key] = v
	}
	d.doc.Variants = variants
	return nil
}

// typeExpression parses a native context type expression: a scalar
// constructor, list<T>, map<T>, or a path to a named context type.
func (d *document6) typeExpression(n *tferlex.Node, raw string) (TypeExpr, error) {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "string", "text", "boolean", "integer", "decimal":
		return TypeExpr{Kind: raw}, nil
	case "number":
		return TypeExpr{}, d.errorf(n, "type 'number' does not exist; declare integer or decimal")
	}
	for _, constructor := range []string{"list", "map"} {
		if strings.HasPrefix(raw, constructor+"<") && strings.HasSuffix(raw, ">") {
			elem, err := d.typeExpression(n, raw[len(constructor)+1:len(raw)-1])
			if err != nil {
				return TypeExpr{}, err
			}
			return TypeExpr{Kind: constructor, Elem: &elem}, nil
		}
	}
	id, err := d.ref(n, raw, "contextType")
	if err != nil {
		return TypeExpr{}, d.errorf(n, "unknown type expression '%s'", raw)
	}
	return TypeExpr{Kind: "ref", Ref: id}, nil
}

func (d *document6) contextTypeFields(n *tferlex.Node) error {
	if !n.IsMap {
		return d.errorf(n, "'fields' must be a mapping")
	}
	fields := map[string]ContextField{}
	for _, item := range n.Items {
		if !slotName.MatchString(item.Key) {
			return d.errorf(item, "context field name '%s' must be an ASCII identifier", item.Key)
		}
		var field ContextField
		typed := false
		err := d.decode(item, map[string]func(*tferlex.Node) error{
			"type": func(t *tferlex.Node) error {
				raw, isNull, err := d.text(t)
				if err != nil {
					return err
				}
				if isNull {
					return d.errorf(t, "'type' needs a type expression")
				}
				expr, err := d.typeExpression(t, raw)
				field.Type = expr
				typed = true
				return err
			},
			"required": d.boolInto(&field.Required),
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
		if !typed {
			return d.errorf(item, "context field '%s' must declare a type", item.Key)
		}
		fields[item.Key] = field
	}
	d.doc.ContextTypeFields = fields
	return nil
}

func (d *document6) applyBody(body string) error {
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

func (d *document6) validate() error {
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
		if len([]rune(doc.Description)) > 1024 {
			return Errorf("%s: description exceeds 1024 characters", file)
		}
	}
	if doc.Visibility != "" && doc.Visibility != "internal" && doc.Visibility != "exposed" {
		return Errorf("%s: visibility must be internal or exposed", file)
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
		if binding.Sealed {
			return Errorf("%s: an abstract requirement cannot be sealed; sealing protects a concrete implementation", file)
		}
	}
	if doc.Kind == "skill" {
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
	}
	if doc.Kind == "plugin" {
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

// CheckV6Document validates one version 6 document in isolation, without
// following references. It is the language server's per-buffer diagnostic.
func CheckV6Document(relative, text string) error {
	if relative == ManifestFile {
		_, isV6, err := parseV6Manifest(stripBOM(strings.ReplaceAll(text, "\r\n", "\n")))
		if !isV6 {
			return Errorf("%s: schemaVersion must be 6", ManifestFile)
		}
		return err
	}
	if err := ValidSourcePath(relative); err != nil {
		return err
	}
	_, err := ParseV6Document("check/package", "0.0.0", relative, stripBOM(strings.ReplaceAll(text, "\r\n", "\n")), nil)
	return err
}

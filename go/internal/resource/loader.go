package resource

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"gopkg.in/yaml.v3"
)

var resourceID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+@[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// Spec ("Canonical text and ordering"): slot names are a canonical key space
// and must be ASCII so every conforming implementation orders them
// identically.
var slotName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// IsResourceID reports whether id is a well-formed resource identifier.
func IsResourceID(id string) bool { return resourceID.MatchString(id) }

// Load reads every *.yaml resource beneath sourceDir (excluding the trust
// configuration), validates document shape, and returns documents keyed by
// resource id.
func Load(sourceDir string, trustConfigPath string) (map[string]*Document, error) {
	root, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, Errorf("Source directory not found: %s", sourceDir)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		return nil, Errorf("Source directory not found: %s", root)
	}
	excluded := []string{
		filepath.Join(root, "typeference.trust.yaml"),
		filepath.Join(root, ProjectManifestFile),
	}
	if trustConfigPath != "" {
		if full, absErr := filepath.Abs(trustConfigPath); absErr == nil {
			excluded = append(excluded, full)
		}
	}
	var files []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".tfer")) {
			return nil
		}
		for _, ex := range excluded {
			if samePath(path, ex) {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	if walkErr != nil {
		return nil, Errorf("Source directory not found: %s", root)
	}
	sort.Strings(files)

	result := map[string]*Document{}
	for _, file := range files {
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			return nil, Errorf("%s: %s", file, readErr)
		}
		text := stripBOM(string(raw))
		isTfer := strings.HasSuffix(file, ".tfer")
		var body string
		if isTfer {
			frontmatter, tferBody, splitErr := splitFrontmatter(text)
			if splitErr != nil {
				return nil, Errorf("%s: %s", file, splitErr)
			}
			text, body = frontmatter, tferBody
		}
		doc, parseErr := parseDocument(text)
		if parseErr != nil {
			return nil, Errorf("%s: invalid YAML resource: %s", file, parseErr)
		}
		if doc == nil {
			return nil, Errorf("Empty resource: %s", file)
		}
		if isTfer {
			if err := applyBody(doc, body, file); err != nil {
				return nil, err
			}
		}
		if err := validateShape(doc, file, root); err != nil {
			return nil, err
		}
		if _, exists := result[doc.ID]; exists {
			return nil, Errorf("Duplicate resource id: %s", doc.ID)
		}
		result[doc.ID] = doc
	}
	if len(result) == 0 {
		return nil, Errorf("No YAML resources found under %s", root)
	}
	return result, nil
}

// samePath compares absolute paths using the platform's case rules, matching
// how the reference implementation excludes the trust configuration.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func stripBOM(s string) string {
	return strings.TrimPrefix(s, string(rune(0xFEFF)))
}

// parseDocument decodes a single-document YAML resource into a Document,
// rejecting unknown fields, aliases-as-fields mismatches, and extra documents,
// mirroring the reference implementation's strict deserialization.
func parseDocument(text string) (*Document, error) {
	decoder := yaml.NewDecoder(strings.NewReader(text))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil // empty resource
		}
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, Errorf("expected a single YAML document")
	}
	doc := NewDocument()
	// A context object may carry schema-typed frontmatter fields beyond the
	// standard keys; every other kind stays strict (unknown key => error).
	kind := scanKind(&node)
	extraFields := map[string]*yaml.Node{}
	unknown := func(key string, value *yaml.Node) error {
		if kind == "context" {
			extraFields[key] = value
			return nil
		}
		return Errorf("property '%s' not found", key)
	}
	if err := decodeMapping(&node, map[string]fieldDecoder{
		"schemaVersion":        intField(&doc.SchemaVersion),
		"kind":                 stringField(&doc.Kind),
		"id":                   stringField(&doc.ID),
		"displayName":          stringField(&doc.DisplayName),
		"description":          stringField(&doc.Description),
		"binds":                stringField(&doc.Binds),
		"emit":                 boolField(&doc.Emit),
		"embeds":               stringListField(&doc.Embeds),
		"requiresSlots":        stringListField(&doc.RequiresSlots),
		"requiresCapabilities": stringListField(&doc.RequiresCapabilities),
		"slots":                stringMapField(&doc.Slots),
		"workingNorms":         stringListField(&doc.WorkingNorms),
		"contextFiles":         stringListField(&doc.ContextFiles),
		"skills":               skillsField(&doc.Skills),
		"instructions":         stringField(&doc.Instructions),
		"inputSchema":          stringField(&doc.InputSchema),
		"outputSchema":         stringField(&doc.OutputSchema),
		"contextType":          stringField(&doc.ContextType),
		"schema":               stringField(&doc.Schema),
		"requiresContextTypes": stringListField(&doc.RequiresContextTypes),
		"context":              stringListField(&doc.Context),
		"requiresTools":        stringListField(&doc.RequiresTools),
		"visibility":           stringField(&doc.Visibility),
		"variants":             variantsField(&doc.Variants),
		"allowedContextTypes":  stringListField(&doc.AllowedContextTypes),
	}, unknown); err != nil {
		return nil, err
	}
	if len(extraFields) > 0 {
		doc.ContextFields = map[string]FieldValue{}
		for key, value := range extraFields {
			switch value.Kind {
			case yaml.SequenceNode:
				doc.ContextFields[key] = FieldValue{Kind: "sequence"}
			case yaml.MappingNode:
				doc.ContextFields[key] = FieldValue{Kind: "mapping"}
			default:
				doc.ContextFields[key] = FieldValue{Kind: "scalar", Scalar: value.Value}
			}
		}
	}
	return doc, nil
}

// scanKind reads the top-level `kind` scalar without a full decode, so the
// decoder can decide whether unknown frontmatter keys are context fields or an
// error.
func scanKind(node *yaml.Node) string {
	n := resolveAlias(node)
	if n.Kind == yaml.DocumentNode && len(n.Content) == 1 {
		n = resolveAlias(n.Content[0])
	}
	if n.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := resolveAlias(n.Content[i])
		if key.Kind == yaml.ScalarNode && key.Value == "kind" {
			if v := resolveAlias(n.Content[i+1]); v.Kind == yaml.ScalarNode {
				return v.Value
			}
		}
	}
	return ""
}

// splitFrontmatter separates a .tfer file into its YAML frontmatter and its
// verbatim markdown body. The file MUST begin with a `---` fence line; the
// frontmatter runs to the next `---` fence line and the body is everything
// after it, preserved byte-for-byte (ADR-0013 format: typed head, prose tail).
func splitFrontmatter(text string) (frontmatter, body string, err error) {
	nl := strings.IndexByte(text, '\n')
	if nl < 0 || strings.TrimRight(text[:nl], "\r") != "---" {
		return "", "", Errorf("a .tfer file must begin with a '---' frontmatter fence")
	}
	rest := text[nl+1:]
	for idx := 0; ; {
		lineEnd := strings.IndexByte(rest[idx:], '\n')
		if lineEnd < 0 {
			if strings.TrimRight(rest[idx:], "\r") == "---" {
				return rest[:idx], "", nil
			}
			return "", "", Errorf("a .tfer file is missing its closing '---' frontmatter fence")
		}
		line := rest[idx : idx+lineEnd]
		next := idx + lineEnd + 1
		if strings.TrimRight(line, "\r") == "---" {
			return rest[:idx], rest[next:], nil
		}
		idx = next
	}
}

// applyBody materializes a .tfer markdown body onto the resource: a skill's
// instructions or a context object's content. Kinds without a body field
// reject a non-empty body (ADR-0013 format; ADR-0012 for the multimodal note).
func applyBody(doc *Document, body, file string) error {
	switch doc.Kind {
	case "skill":
		if strings.TrimSpace(body) == "" {
			return nil
		}
		if len(doc.Variants) != 0 {
			return Errorf("%s: a multimodal skill has no single body; put instructions inside each variant", file)
		}
		if strings.TrimSpace(doc.Instructions) != "" {
			return Errorf("%s: a .tfer skill sets instructions in both the body and frontmatter; use one", file)
		}
		doc.Instructions = body
	case "context":
		doc.Content = body
	default:
		if strings.TrimSpace(body) != "" {
			return Errorf("%s: a %s resource has no body field; put content in frontmatter", file, doc.Kind)
		}
	}
	return nil
}

type fieldDecoder func(*yaml.Node) error

func resolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// decodeMapping decodes a mapping node against a field table. Unknown keys go
// to the optional unknown handler; when it is nil an unknown key is an error.
func decodeMapping(node *yaml.Node, fields map[string]fieldDecoder, unknown func(string, *yaml.Node) error) error {
	node = resolveAlias(node)
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = resolveAlias(node.Content[0])
	}
	if node.Kind != yaml.MappingNode {
		return Errorf("expected a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := resolveAlias(node.Content[i])
		if key.Kind != yaml.ScalarNode {
			return Errorf("mapping keys must be scalars")
		}
		decoder, known := fields[key.Value]
		if !known {
			if unknown != nil {
				if err := unknown(key.Value, resolveAlias(node.Content[i+1])); err != nil {
					return err
				}
				continue
			}
			return Errorf("property '%s' not found", key.Value)
		}
		if err := decoder(resolveAlias(node.Content[i+1])); err != nil {
			return err
		}
	}
	return nil
}

func scalarString(node *yaml.Node, out *string) error {
	if node.Kind != yaml.ScalarNode {
		return Errorf("expected a scalar value")
	}
	if node.Tag == "!!null" {
		*out = ""
		return nil
	}
	*out = node.Value
	return nil
}

func stringField(target *string) fieldDecoder {
	return func(node *yaml.Node) error { return scalarString(node, target) }
}

func intField(target *int) fieldDecoder {
	return func(node *yaml.Node) error {
		var raw string
		if err := scalarString(node, &raw); err != nil {
			return err
		}
		v, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return Errorf("expected an integer, got '%s'", raw)
		}
		*target = v
		return nil
	}
}

func boolField(target *bool) fieldDecoder {
	return func(node *yaml.Node) error {
		var raw string
		if err := scalarString(node, &raw); err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true":
			*target = true
		case "false":
			*target = false
		default:
			return Errorf("expected true or false, got '%s'", raw)
		}
		return nil
	}
}

func stringListField(target *[]string) fieldDecoder {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.SequenceNode {
			return Errorf("expected a sequence")
		}
		items := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			var v string
			if err := scalarString(resolveAlias(item), &v); err != nil {
				return err
			}
			items = append(items, v)
		}
		*target = items
		return nil
	}
}

func stringMapField(target *map[string]string) fieldDecoder {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.MappingNode {
			return Errorf("expected a mapping")
		}
		m := map[string]string{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := resolveAlias(node.Content[i])
			if key.Kind != yaml.ScalarNode {
				return Errorf("mapping keys must be scalars")
			}
			var v string
			if err := scalarString(resolveAlias(node.Content[i+1]), &v); err != nil {
				return err
			}
			m[key.Value] = v
		}
		*target = m
		return nil
	}
}

func variantsField(target *map[string]Variant) fieldDecoder {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.MappingNode {
			return Errorf("expected a mapping")
		}
		m := map[string]Variant{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := resolveAlias(node.Content[i])
			if key.Kind != yaml.ScalarNode {
				return Errorf("mapping keys must be scalars")
			}
			var v Variant
			if err := decodeMapping(resolveAlias(node.Content[i+1]), map[string]fieldDecoder{
				"instructions":         stringField(&v.Instructions),
				"requiresContextTypes": stringListField(&v.RequiresContextTypes),
				"requiresTools":        stringListField(&v.RequiresTools),
			}, nil); err != nil {
				return err
			}
			m[key.Value] = v
		}
		*target = m
		return nil
	}
}

func skillsField(target *[]SkillBinding) fieldDecoder {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.SequenceNode {
			return Errorf("expected a sequence")
		}
		bindings := make([]SkillBinding, 0, len(node.Content))
		for _, item := range node.Content {
			var binding SkillBinding
			var capability string
			hasCapability := false
			err := decodeMapping(resolveAlias(item), map[string]fieldDecoder{
				"ref": stringField(&binding.Ref),
				"capability": func(n *yaml.Node) error {
					hasCapability = true
					return scalarString(n, &capability)
				},
				"sealed":   boolField(&binding.Sealed),
				"required": boolField(&binding.Required),
			}, nil)
			if err != nil {
				return err
			}
			if hasCapability {
				binding.Capability = &capability
			}
			bindings = append(bindings, binding)
		}
		*target = bindings
		return nil
	}
}

func validateShape(doc *Document, file, root string) error {
	if err := validateDocumentShape(doc, file); err != nil {
		return err
	}
	var referenced []string
	referenced = append(referenced, doc.ContextFiles...)
	for _, key := range SortedKeys(doc.Slots) {
		referenced = append(referenced, doc.Slots[key])
	}
	for _, relative := range referenced {
		full := filepath.Join(root, filepath.FromSlash(relative))
		if !strings.HasPrefix(full, root+string(filepath.Separator)) {
			return Errorf("%s: path escapes source root: %s", file, relative)
		}
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			return Errorf("%s: referenced file does not exist: %s", file, relative)
		}
	}
	return nil
}

// validateDocumentShape validates a single resource in isolation: every rule
// except the composition-level checks that require the whole source tree
// (referenced-file existence). CheckDocument reuses it for the language server.
func validateDocumentShape(doc *Document, file string) error {
	if doc.SchemaVersion != 3 {
		return Errorf("%s: schemaVersion must be 3", file)
	}
	switch doc.Kind {
	case "agent", "profile", "interface", "capability", "skill", "context", "contextType", "tool":
	default:
		return Errorf("%s: unknown kind '%s'", file, doc.Kind)
	}
	if !resourceID.MatchString(doc.ID) {
		return Errorf("%s: id must use lowercase namespace/name@semantic-version", file)
	}
	// capabilities, skills, context objects, and tools do not embed; contextTypes
	// may embed to refine other contextTypes (ADR-0013), and agents/profiles/
	// interfaces embed by design.
	switch doc.Kind {
	case "capability", "skill", "context", "tool":
		if len(doc.Embeds) != 0 {
			return Errorf("%s: %ss cannot embed resources", file, doc.Kind)
		}
	}
	if doc.Kind == "context" {
		if strings.TrimSpace(doc.ContextType) == "" {
			return Errorf("%s: a context resource must declare a contextType", file)
		}
		if !resourceID.MatchString(doc.ContextType) {
			return Errorf("%s: contextType must reference a contextType id", file)
		}
	} else if strings.TrimSpace(doc.ContextType) != "" {
		return Errorf("%s: only context resources declare a contextType", file)
	}
	if doc.Kind == "contextType" {
		if strings.TrimSpace(doc.Schema) != "" {
			if err := validateJSON(doc.Schema, file, "schema"); err != nil {
				return err
			}
		}
	} else if strings.TrimSpace(doc.Schema) != "" {
		return Errorf("%s: only contextType resources declare a schema", file)
	}
	// requiresContextTypes / requiresTools are skill-only; context (holding by
	// id) is agent/profile-only; visibility is capability-only.
	if len(doc.RequiresContextTypes) != 0 && doc.Kind != "skill" {
		return Errorf("%s: only skills declare requiresContextTypes", file)
	}
	if len(doc.RequiresTools) != 0 && doc.Kind != "skill" {
		return Errorf("%s: only skills declare requiresTools", file)
	}
	if len(doc.Context) != 0 && doc.Kind != "agent" && doc.Kind != "profile" {
		return Errorf("%s: only agents and profiles hold context by id", file)
	}
	if len(doc.AllowedContextTypes) != 0 && doc.Kind != "agent" && doc.Kind != "profile" {
		return Errorf("%s: only agents and profiles declare allowedContextTypes", file)
	}
	refs := append([]string{}, doc.RequiresContextTypes...)
	refs = append(refs, doc.RequiresTools...)
	refs = append(refs, doc.Context...)
	refs = append(refs, doc.AllowedContextTypes...)
	for _, v := range doc.Variants {
		refs = append(refs, v.RequiresContextTypes...)
		refs = append(refs, v.RequiresTools...)
	}
	for _, ref := range refs {
		if !resourceID.MatchString(ref) {
			return Errorf("%s: reference '%s' must be a namespace/name@semantic-version id", file, ref)
		}
	}
	if strings.TrimSpace(doc.Visibility) != "" {
		if doc.Kind != "capability" {
			return Errorf("%s: only capabilities declare visibility", file)
		}
		if doc.Visibility != "internal" && doc.Visibility != "exposed" {
			return Errorf("%s: visibility must be 'internal' or 'exposed'", file)
		}
	}
	if len(doc.Variants) != 0 {
		if doc.Kind != "skill" {
			return Errorf("%s: only skills declare variants", file)
		}
		if strings.TrimSpace(doc.Instructions) != "" {
			return Errorf("%s: a skill declares either instructions or variants, not both", file)
		}
		for mode, v := range doc.Variants {
			if !slotName.MatchString(mode) {
				return Errorf("%s: variant mode '%s' must be an ASCII identifier", file, mode)
			}
			if strings.TrimSpace(v.Instructions) == "" {
				return Errorf("%s: variant '%s' must set instructions", file, mode)
			}
		}
	}
	if doc.Kind == "skill" && strings.TrimSpace(doc.Binds) == "" {
		return Errorf("%s: skills must bind a capability", file)
	}
	if doc.Kind == "skill" && !resourceID.MatchString(doc.Binds) {
		return Errorf("%s: binds must reference a capability id", file)
	}
	if doc.Kind != "skill" && strings.TrimSpace(doc.Binds) != "" {
		return Errorf("%s: only skills can bind capabilities", file)
	}
	slotNames := append(SortedKeys(doc.Slots), doc.RequiresSlots...)
	for _, name := range slotNames {
		if !slotName.MatchString(name) {
			return Errorf("%s: slot name '%s' must be an ASCII identifier matching [A-Za-z0-9][A-Za-z0-9._-]*", file, name)
		}
	}
	if err := validateJSON(doc.InputSchema, file, "inputSchema"); err != nil {
		return err
	}
	return validateJSON(doc.OutputSchema, file, "outputSchema")
}

// CheckDocument parses one resource from its path and content and validates its
// shape in isolation, without composition-level checks that need the whole
// source tree. It is the diagnostic entrypoint used by the language server.
func CheckDocument(path, content string) error {
	text := stripBOM(content)
	isTfer := strings.HasSuffix(path, ".tfer")
	var body string
	if isTfer {
		frontmatter, tferBody, err := splitFrontmatter(text)
		if err != nil {
			return err
		}
		text, body = frontmatter, tferBody
	}
	doc, err := parseDocument(text)
	if err != nil {
		return Errorf("invalid YAML resource: %s", err)
	}
	if doc == nil {
		return Errorf("empty resource")
	}
	if isTfer {
		if err := applyBody(doc, body, filepath.Base(path)); err != nil {
			return err
		}
	}
	return validateDocumentShape(doc, filepath.Base(path))
}

func validateJSON(value, file, field string) error {
	if _, err := jsonx.Parse(value); err != nil {
		return Errorf("%s: invalid %s: %s", file, field, err)
	}
	return nil
}

// SortedKeys returns map keys in canonical (code point) order.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

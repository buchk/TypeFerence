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
	excluded := []string{filepath.Join(root, "typeference.trust.yaml")}
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
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
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
		doc, parseErr := parseDocument(stripBOM(string(raw)))
		if parseErr != nil {
			return nil, Errorf("%s: invalid YAML resource: %s", file, parseErr)
		}
		if doc == nil {
			return nil, Errorf("Empty resource: %s", file)
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
	}); err != nil {
		return nil, err
	}
	return doc, nil
}

type fieldDecoder func(*yaml.Node) error

func resolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

func decodeMapping(node *yaml.Node, fields map[string]fieldDecoder) error {
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
			})
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
	if doc.SchemaVersion != 3 {
		return Errorf("%s: schemaVersion must be 3", file)
	}
	switch doc.Kind {
	case "agent", "profile", "interface", "capability", "skill":
	default:
		return Errorf("%s: unknown kind '%s'", file, doc.Kind)
	}
	if !resourceID.MatchString(doc.ID) {
		return Errorf("%s: id must use lowercase namespace/name@semantic-version", file)
	}
	if (doc.Kind == "capability" || doc.Kind == "skill") && len(doc.Embeds) != 0 {
		return Errorf("%s: %ss cannot embed resources", file, doc.Kind)
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
	if err := validateJSON(doc.InputSchema, file, "inputSchema"); err != nil {
		return err
	}
	return validateJSON(doc.OutputSchema, file, "outputSchema")
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

package resource

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/tferlex"
	"gopkg.in/yaml.v3"
)

var packageName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+$`)
var semanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// marketplaceName follows the Copilot marketplace grammar: kebab-case, at
// most 64 characters, dots accepted.
var marketplaceName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// Marketplace is the optional publication identity a version 6 manifest
// declares for its generated marketplace index (ADR-0029).
type Marketplace struct {
	Name  string
	Owner string
}

// Project is the source-root manifest. Version 6 manifests
// (`typeference.tfer`, schemaVersion 6) additionally list the plugins that
// define the build and the resources the package exports. Legacy manifests
// (schemaVersion 2) carry identity and dependencies only.
type Project struct {
	SchemaVersion int
	File          string
	Name          string
	Version       string
	Publisher     string
	Dependencies  map[string]string
	Marketplace   *Marketplace
	Plugins       []string
	Exports       []string
}

// ProjectManifestFile is the retired schemaVersion 2 manifest name.
const ProjectManifestFile = "typeference.yaml"

// ManifestFileNameV5 is the fenced manifest name. Version 6 manifests use it
// exclusively.
const ManifestFileNameV5 = "typeference.tfer"

// ManifestFile is the version 6 project manifest name.
const ManifestFile = ManifestFileNameV5

// ReservedPackagePrefix names the namespace TypeFerence uses for built-in
// resources. User packages cannot claim it.
const ReservedPackagePrefix = "typeference/builtin"

// IsV6 reports whether the manifest declares the version 6 language.
func (p *Project) IsV6() bool { return p != nil && p.SchemaVersion == 6 }

// SplitPluginEntry splits a manifest plugins entry into the dependency that
// owns the plugin and the plugin's path in it. A package's own plugin has no
// package prefix (ADR-0034).
func SplitPluginEntry(entry string) (pkg, path string) {
	if idx := strings.Index(entry, ":"); idx >= 0 {
		return entry[:idx], entry[idx+1:]
	}
	return "", entry
}

// OwnPlugins returns the paths of the plugin documents the package itself
// contains, in manifest order.
func (p *Project) OwnPlugins() []string {
	own := []string{}
	for _, entry := range p.Plugins {
		if pkg, path := SplitPluginEntry(entry); pkg == "" {
			own = append(own, path)
		}
	}
	return own
}

func LoadProject(sourceDir string) (*Project, error) {
	tferPath := filepath.Join(sourceDir, ManifestFileNameV5)
	yamlPath := filepath.Join(sourceDir, ProjectManifestFile)
	tferRaw, tferErr := os.ReadFile(tferPath)
	yamlRaw, yamlErr := os.ReadFile(yamlPath)
	if tferErr != nil && !os.IsNotExist(tferErr) {
		return nil, Errorf("%s: %s", ManifestFileNameV5, tferErr)
	}
	if yamlErr != nil && !os.IsNotExist(yamlErr) {
		return nil, Errorf("%s: %s", ProjectManifestFile, yamlErr)
	}
	if tferErr == nil {
		text := stripBOM(strings.ReplaceAll(string(tferRaw), "\r\n", "\n"))
		if project, isV6, err := parseV6Manifest(text); isV6 {
			if err != nil {
				return nil, err
			}
			if yamlErr == nil {
				return nil, Errorf("%s: a version 6 project cannot also contain the retired %s", ManifestFile, ProjectManifestFile)
			}
			return project, nil
		}
	}
	// Legacy schemaVersion 2 manifests: typeference.yaml takes precedence, as
	// it always has, so archived corpora keep their exact identity.
	if yamlErr == nil {
		return parseLegacyManifest(string(yamlRaw), ProjectManifestFile)
	}
	if tferErr == nil {
		return parseLegacyManifest(string(tferRaw), ManifestFileNameV5)
	}
	return nil, nil
}

// ParseProjectManifest parses manifest text by file name: a version 6
// manifest, or an archival schemaVersion 2 manifest.
func ParseProjectManifest(fileName, text string) (*Project, error) {
	text = stripBOM(strings.ReplaceAll(text, "\r\n", "\n"))
	if fileName == ManifestFileNameV5 {
		if project, isV6, err := parseV6Manifest(text); isV6 {
			return project, err
		}
	}
	return parseLegacyManifest(text, fileName)
}

// parseV6Manifest parses a fenced manifest with the closed grammar. isV6
// reports whether the document declares schemaVersion 6; when it does not,
// the caller falls back to the legacy parser.
func parseV6Manifest(text string) (*Project, bool, error) {
	frontmatter, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, false, nil
	}
	root, err := tferlex.Parse(frontmatter)
	if err != nil || root == nil {
		if err != nil && declaresSchemaVersion6(frontmatter) {
			return nil, true, Errorf("%s: %s", ManifestFile, err)
		}
		return nil, false, nil
	}
	versionNode := child(root, "schemaVersion")
	if versionNode == nil || !versionNode.IsScalar || versionNode.Value.Kind != tferlex.KindPlain || versionNode.Value.Text != "6" {
		return nil, false, nil
	}
	if strings.TrimSpace(body) != "" {
		return nil, true, Errorf("%s: the project manifest does not take a body", ManifestFile)
	}
	project := &Project{SchemaVersion: 6, File: ManifestFile, Dependencies: map[string]string{}}
	d := &fieldDecoder6{file: ManifestFile}
	err = d.decode(root, map[string]func(*tferlex.Node) error{
		"schemaVersion": func(*tferlex.Node) error { return nil },
		"name":          d.stringInto(&project.Name),
		"version":       d.stringInto(&project.Version),
		"publisher":     d.stringInto(&project.Publisher),
		"marketplace": func(n *tferlex.Node) error {
			m := &Marketplace{}
			if err := d.decode(n, map[string]func(*tferlex.Node) error{
				"name":  d.stringInto(&m.Name),
				"owner": d.stringInto(&m.Owner),
			}); err != nil {
				return err
			}
			project.Marketplace = m
			return nil
		},
		"dependencies": func(n *tferlex.Node) error {
			values, err := d.stringMap(n)
			if err != nil {
				return err
			}
			project.Dependencies = values
			return nil
		},
		"plugins": d.stringListInto(&project.Plugins),
		"exports": d.stringListInto(&project.Exports),
	})
	if err != nil {
		return nil, true, err
	}
	if !packageName.MatchString(project.Name) {
		return nil, true, Errorf("%s: name must use a lowercase namespace/name", ManifestFile)
	}
	if project.Name == ReservedPackagePrefix || strings.HasPrefix(project.Name, ReservedPackagePrefix+"/") {
		return nil, true, Errorf("%s: the %s namespace is reserved for built-in resources", ManifestFile, ReservedPackagePrefix)
	}
	if !semanticVersion.MatchString(project.Version) {
		return nil, true, Errorf("%s: version must be an exact semantic version", ManifestFile)
	}
	if strings.TrimSpace(project.Publisher) != project.Publisher {
		return nil, true, Errorf("%s: publisher must not contain surrounding whitespace", ManifestFile)
	}
	for name, version := range project.Dependencies {
		if !packageName.MatchString(name) {
			return nil, true, Errorf("%s: dependency '%s' must use a lowercase namespace/name", ManifestFile, name)
		}
		if !semanticVersion.MatchString(version) {
			return nil, true, Errorf("%s: dependency '%s' must use an exact semantic version", ManifestFile, name)
		}
		if name == project.Name {
			return nil, true, Errorf("%s: a package cannot depend on itself", ManifestFile)
		}
	}
	if m := project.Marketplace; m != nil {
		if len(m.Name) > 64 || !marketplaceName.MatchString(m.Name) {
			return nil, true, Errorf("%s: marketplace.name must be kebab-case, at most 64 characters", ManifestFile)
		}
		if strings.TrimSpace(m.Owner) == "" || strings.TrimSpace(m.Owner) != m.Owner {
			return nil, true, Errorf("%s: marketplace.owner is required and must not contain surrounding whitespace", ManifestFile)
		}
	}
	if len(project.Plugins) == 0 && len(project.Exports) == 0 {
		return nil, true, Errorf("%s: a version 6 package must list at least one plugin or export", ManifestFile)
	}
	seen := map[string]bool{}
	for _, entry := range project.Plugins {
		pkg, path := SplitPluginEntry(entry)
		if kind, _, ok := KindFromPath(path); !ok || kind != "plugin" {
			return nil, true, Errorf("%s: plugins must list .plugin.tfer paths, got '%s'", ManifestFile, entry)
		}
		if pkg != "" {
			if !packageName.MatchString(pkg) {
				return nil, true, Errorf("%s: plugin '%s' names an invalid package", ManifestFile, entry)
			}
			if pkg == project.Name {
				return nil, true, Errorf("%s: plugin '%s' names this package; write the path without a package prefix", ManifestFile, entry)
			}
			if _, declared := project.Dependencies[pkg]; !declared {
				return nil, true, Errorf("%s: plugin '%s' names package %s, which the manifest does not declare as a dependency", ManifestFile, entry, pkg)
			}
			if err := ValidSourcePath(path); err != nil {
				return nil, true, Errorf("%s: plugin '%s': %s", ManifestFile, entry, err.(*Error).Message)
			}
		}
		if seen[entry] {
			return nil, true, Errorf("%s: plugin '%s' is listed more than once", ManifestFile, entry)
		}
		seen[entry] = true
	}
	seen = map[string]bool{}
	for _, path := range project.Exports {
		if strings.Contains(path, ":") {
			return nil, true, Errorf("%s: exports list this package's own documents, got '%s'", ManifestFile, path)
		}
		kind, _, ok := KindFromPath(path)
		if !ok || kind == "plugin" {
			return nil, true, Errorf("%s: exports must list resource paths other than plugins, got '%s'", ManifestFile, path)
		}
		if seen[path] {
			return nil, true, Errorf("%s: export '%s' is listed more than once", ManifestFile, path)
		}
		seen[path] = true
	}
	return project, true, nil
}

func declaresSchemaVersion6(frontmatter string) bool {
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.TrimSpace(line) == "schemaVersion: 6" {
			return true
		}
	}
	return false
}

func parseLegacyManifest(raw, manifestFile string) (*Project, error) {
	// The manifest may be fenced; strip an optional `---` pair (ADR-0026).
	text := strings.TrimPrefix(raw, "---\n")
	if closing := strings.LastIndex(text, "\n---\n"); closing >= 0 {
		if strings.TrimSpace(text[closing+len("\n---\n"):]) != "" {
			return nil, Errorf("%s: the project manifest does not take a body", manifestFile)
		}
		text = text[:closing]
	}
	var doc struct {
		SchemaVersion int               `yaml:"schemaVersion"`
		Name          string            `yaml:"name"`
		Version       string            `yaml:"version"`
		Publisher     string            `yaml:"publisher"`
		Dependencies  map[string]string `yaml:"dependencies"`
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, Errorf("%s: invalid manifest: %s", manifestFile, err)
	}
	if doc.SchemaVersion != 2 {
		return nil, Errorf("%s: schemaVersion must be 6", manifestFile)
	}
	if !packageName.MatchString(doc.Name) {
		return nil, Errorf("%s: name must use a lowercase namespace/name", manifestFile)
	}
	if !semanticVersion.MatchString(doc.Version) {
		return nil, Errorf("%s: version must be an exact semantic version", manifestFile)
	}
	if strings.TrimSpace(doc.Publisher) != doc.Publisher {
		return nil, Errorf("%s: publisher must not contain surrounding whitespace", manifestFile)
	}
	dependencies := map[string]string{}
	for name, version := range doc.Dependencies {
		if !packageName.MatchString(name) {
			return nil, Errorf("%s: dependency '%s' must use a lowercase namespace/name", manifestFile, name)
		}
		if !semanticVersion.MatchString(version) {
			return nil, Errorf("%s: dependency '%s' must use an exact semantic version", manifestFile, name)
		}
		dependencies[name] = version
	}
	return &Project{
		SchemaVersion: 2,
		File:          manifestFile,
		Name:          doc.Name,
		Version:       doc.Version,
		Publisher:     doc.Publisher,
		Dependencies:  dependencies,
	}, nil
}

// SortedDependencies returns dependency names in canonical order.
func (p *Project) SortedDependencies() []string {
	names := make([]string, 0, len(p.Dependencies))
	for name := range p.Dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func IsPackageName(value string) bool     { return packageName.MatchString(value) }
func IsSemanticVersion(value string) bool { return semanticVersion.MatchString(value) }

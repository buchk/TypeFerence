package resource

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/tferlex"
)

var packageName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+$`)
var semanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// marketplaceName follows the Copilot marketplace grammar: kebab-case, at
// most 64 characters, dots accepted.
var marketplaceName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// SchemaVersion is the source language version this implementation reads.
const SchemaVersion = 7

// Marketplace is the optional publication identity a manifest declares for
// its generated marketplace index.
type Marketplace struct {
	Name  string
	Owner string
	// Files ship at the target root (ADR-0041). LoadPackage reads their
	// bytes; LoadProject alone leaves Data empty.
	Files []PackageFile
}

// Project is the source-root manifest: package identity, dependencies, the
// plugins the package ships, and the documents it exports.
type Project struct {
	SchemaVersion int
	File          string
	Name          string
	Version       string
	Dependencies  map[string]string
	Marketplace   *Marketplace
	Plugins       []string
	Exports       []string
}

// ManifestFile is the project manifest name.
const ManifestFile = "typeference.tfer"

// ReservedPackagePrefix names the namespace TypeFerence reserves. User
// packages cannot claim it.
const ReservedPackagePrefix = "typeference/builtin"

// IsCurrent reports whether the manifest declares the version this
// implementation reads.
func (p *Project) IsCurrent() bool { return p != nil && p.SchemaVersion == SchemaVersion }

// SplitPluginEntry splits a manifest plugins entry into the dependency that
// owns the plugin and the plugin's path in it. A package's own plugin has no
// package prefix.
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

// LoadProject reads a source root's manifest. It returns nil, nil when the
// directory has no manifest.
func LoadProject(sourceDir string) (*Project, error) {
	raw, err := os.ReadFile(filepath.Join(sourceDir, ManifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, Errorf("%s: %s", ManifestFile, err)
	}
	return ParseProjectManifest(string(raw))
}

// ParseProjectManifest parses manifest text.
func ParseProjectManifest(text string) (*Project, error) {
	text = NormalizeText(text)
	frontmatter, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, Errorf("%s: %s", ManifestFile, err.(*Error).Message)
	}
	root, err := tferlex.Parse(frontmatter)
	if err != nil {
		return nil, Errorf("%s: %s", ManifestFile, err)
	}
	versionNode := child(root, "schemaVersion")
	if versionNode == nil || !versionNode.IsScalar || versionNode.Value.Kind != tferlex.KindPlain {
		return nil, Errorf("%s: schemaVersion must be 7 (docs/specification.md)", ManifestFile)
	}
	if versionNode.Value.Text != "7" {
		return nil, Errorf("%s: schemaVersion %s is not supported; TypeFerence reads version 7 sources (docs/specification.md, ADR-0035)", ManifestFile, versionNode.Value.Text)
	}
	if strings.TrimSpace(body) != "" {
		return nil, Errorf("%s: the project manifest does not take a body", ManifestFile)
	}
	project := &Project{SchemaVersion: SchemaVersion, File: ManifestFile, Dependencies: map[string]string{}}
	d := &fieldDecoder{file: ManifestFile}
	err = d.decode(root, map[string]func(*tferlex.Node) error{
		"schemaVersion": func(*tferlex.Node) error { return nil },
		"name":          d.stringInto(&project.Name),
		"version":       d.stringInto(&project.Version),
		"marketplace": func(n *tferlex.Node) error {
			m := &Marketplace{}
			if err := d.decode(n, map[string]func(*tferlex.Node) error{
				"name":  d.stringInto(&m.Name),
				"owner": d.stringInto(&m.Owner),
				"files": func(n *tferlex.Node) error {
					files, err := d.fileEntries(n, project.Name, marketplaceFiles)
					m.Files = files
					return err
				},
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
		return nil, err
	}
	if !packageName.MatchString(project.Name) {
		return nil, Errorf("%s: name must use a lowercase namespace/name", ManifestFile)
	}
	if project.Name == ReservedPackagePrefix || strings.HasPrefix(project.Name, ReservedPackagePrefix+"/") {
		return nil, Errorf("%s: the %s namespace is reserved", ManifestFile, ReservedPackagePrefix)
	}
	if !semanticVersion.MatchString(project.Version) {
		return nil, Errorf("%s: version must be an exact semantic version", ManifestFile)
	}
	for name, version := range project.Dependencies {
		if !packageName.MatchString(name) {
			return nil, Errorf("%s: dependency '%s' must use a lowercase namespace/name", ManifestFile, name)
		}
		if !semanticVersion.MatchString(version) {
			return nil, Errorf("%s: dependency '%s' must use an exact semantic version", ManifestFile, name)
		}
		if name == project.Name {
			return nil, Errorf("%s: a package cannot depend on itself", ManifestFile)
		}
	}
	if m := project.Marketplace; m != nil {
		if len(m.Name) > 64 || !marketplaceName.MatchString(m.Name) {
			return nil, Errorf("%s: marketplace.name must be kebab-case, at most 64 characters", ManifestFile)
		}
		if strings.TrimSpace(m.Owner) == "" || strings.TrimSpace(m.Owner) != m.Owner {
			return nil, Errorf("%s: marketplace.owner is required and must not contain surrounding whitespace", ManifestFile)
		}
	}
	if len(project.Plugins) == 0 && len(project.Exports) == 0 {
		return nil, Errorf("%s: a package must list at least one plugin or export", ManifestFile)
	}
	seen := map[string]bool{}
	for _, entry := range project.Plugins {
		pkg, path := SplitPluginEntry(entry)
		if kind, _, ok := KindFromPath(path); !ok || kind != "plugin" {
			return nil, Errorf("%s: plugins must list .plugin.tfer paths, got '%s'", ManifestFile, entry)
		}
		if pkg != "" {
			if !packageName.MatchString(pkg) {
				return nil, Errorf("%s: plugin '%s' names an invalid package", ManifestFile, entry)
			}
			if pkg == project.Name {
				return nil, Errorf("%s: plugin '%s' names this package; write the path without a package prefix", ManifestFile, entry)
			}
			if _, declared := project.Dependencies[pkg]; !declared {
				return nil, Errorf("%s: plugin '%s' names package %s, which the manifest does not declare as a dependency", ManifestFile, entry, pkg)
			}
			if err := ValidSourcePath(path); err != nil {
				return nil, Errorf("%s: plugin '%s': %s", ManifestFile, entry, err.(*Error).Message)
			}
		}
		if seen[entry] {
			return nil, Errorf("%s: plugin '%s' is listed more than once", ManifestFile, entry)
		}
		seen[entry] = true
	}
	seen = map[string]bool{}
	for _, path := range project.Exports {
		if strings.Contains(path, ":") {
			return nil, Errorf("%s: exports list this package's own documents, got '%s'", ManifestFile, path)
		}
		kind, _, ok := KindFromPath(path)
		if !ok || kind == "plugin" {
			return nil, Errorf("%s: exports must list document paths other than plugins, got '%s'", ManifestFile, path)
		}
		if seen[path] {
			return nil, Errorf("%s: export '%s' is listed more than once", ManifestFile, path)
		}
		seen[path] = true
	}
	return project, nil
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

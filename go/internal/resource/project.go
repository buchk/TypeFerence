package resource

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var packageName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+$`)
var semanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// Project is the optional schemaVersion 2 source-root manifest. It contains
// stable source identity only; feeds and deployment are external.
type Project struct {
	Name         string
	Version      string
	Publisher    string
	Dependencies map[string]string
}

const ProjectManifestFile = "typeference.yaml"

func LoadProject(sourceDir string) (*Project, error) {
	raw, err := os.ReadFile(filepath.Join(sourceDir, ProjectManifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, Errorf("%s: %s", ProjectManifestFile, err)
	}
	var doc struct {
		SchemaVersion int               `yaml:"schemaVersion"`
		Name          string            `yaml:"name"`
		Version       string            `yaml:"version"`
		Publisher     string            `yaml:"publisher"`
		Dependencies  map[string]string `yaml:"dependencies"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, Errorf("%s: invalid manifest: %s", ProjectManifestFile, err)
	}
	if doc.SchemaVersion != 2 {
		return nil, Errorf("%s: schemaVersion must be 2", ProjectManifestFile)
	}
	if !packageName.MatchString(doc.Name) {
		return nil, Errorf("%s: name must use a lowercase namespace/name", ProjectManifestFile)
	}
	if !semanticVersion.MatchString(doc.Version) {
		return nil, Errorf("%s: version must be an exact semantic version", ProjectManifestFile)
	}
	if strings.TrimSpace(doc.Publisher) != doc.Publisher {
		return nil, Errorf("%s: publisher must not contain surrounding whitespace", ProjectManifestFile)
	}
	dependencies := map[string]string{}
	for name, version := range doc.Dependencies {
		if !packageName.MatchString(name) {
			return nil, Errorf("%s: dependency '%s' must use a lowercase namespace/name", ProjectManifestFile, name)
		}
		if !semanticVersion.MatchString(version) {
			return nil, Errorf("%s: dependency '%s' must use an exact semantic version", ProjectManifestFile, name)
		}
		dependencies[name] = version
	}
	return &Project{
		Name:         doc.Name,
		Version:      doc.Version,
		Publisher:    doc.Publisher,
		Dependencies: dependencies,
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

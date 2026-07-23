package resource

import (
	"bytes"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Project is the optional source-root manifest (`typeference.yaml`): the
// project's declared identity and publisher. It is the home for settings that
// were otherwise passed on the CLI every build or derived from the folder name
// — analogous to go.mod / Cargo.toml / package.json.
type Project struct {
	Name      string
	Version   string
	Publisher string
}

// ProjectManifestFile is the source-root manifest filename.
const ProjectManifestFile = "typeference.yaml"

// LoadProject reads the project manifest from a source directory. It returns
// (nil, nil) when no manifest is present — the manifest is optional.
func LoadProject(sourceDir string) (*Project, error) {
	raw, err := os.ReadFile(filepath.Join(sourceDir, ProjectManifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, Errorf("%s: %s", ProjectManifestFile, err)
	}
	var doc struct {
		SchemaVersion int    `yaml:"schemaVersion"`
		Name          string `yaml:"name"`
		Version       string `yaml:"version"`
		Publisher     string `yaml:"publisher"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, Errorf("%s: invalid manifest: %s", ProjectManifestFile, err)
	}
	if doc.SchemaVersion != 1 {
		return nil, Errorf("%s: schemaVersion must be 1", ProjectManifestFile)
	}
	return &Project{Name: doc.Name, Version: doc.Version, Publisher: doc.Publisher}, nil
}

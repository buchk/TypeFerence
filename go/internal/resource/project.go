package resource

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Project is the optional source-root manifest (`typeference.yaml`): the
// project's declared identity and publisher. It is the home for settings that
// were otherwise passed on the CLI every build or derived from the folder name
// — analogous to go.mod / Cargo.toml / package.json.
type Project struct {
	Name       string
	Version    string
	Publisher  string
	Deployment Deployment
}

// Deployment declares where this project's agents actually run. TypeFerence
// compiles definitions; it does not deploy them, so an endpoint is authored
// here rather than invented by the compiler. Absent values stay unbound and are
// emitted as named holes for whoever deploys to fill — the same compile-time
// declaration / runtime body split tools use (ADR-0017).
type Deployment struct {
	// A2ABaseURL is the base URL serving this project's A2A agent cards. A card
	// is a live discovery claim once published, so one is only emitted when this
	// is declared.
	A2ABaseURL string
	// MCPCommand is the command that serves a compiled agent's tool manifest.
	// When empty, target configuration emits a substitution token instead.
	MCPCommand string
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
		Deployment    struct {
			A2ABaseURL string `yaml:"a2aBaseUrl"`
			MCPCommand string `yaml:"mcpCommand"`
		} `yaml:"deployment"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, Errorf("%s: invalid manifest: %s", ProjectManifestFile, err)
	}
	if doc.SchemaVersion != 1 {
		return nil, Errorf("%s: schemaVersion must be 1", ProjectManifestFile)
	}
	return &Project{
		Name:      doc.Name,
		Version:   doc.Version,
		Publisher: doc.Publisher,
		Deployment: Deployment{
			A2ABaseURL: strings.TrimRight(strings.TrimSpace(doc.Deployment.A2ABaseURL), "/"),
			MCPCommand: strings.TrimSpace(doc.Deployment.MCPCommand),
		},
	}, nil
}

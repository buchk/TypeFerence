// Package deploy validates external deployment bindings and materializes
// runnable target-native configuration from deterministic unlinked builds.
package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"gopkg.in/yaml.v3"
)

var deploymentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// pluginCommand is what an Agent Plugins stdio server may run: a bare
// executable token or a plugin-relative ./path.
var (
	bareCommand     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._+-]*$`)
	relativeCommand = regexp.MustCompile(`^\./[A-Za-z0-9._+/-]+$`)
)

// mcpSchemaURI identifies the Agent Plugins 1.0 MCP configuration schema.
const mcpSchemaURI = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

type ArtifactBinding struct {
	Modes []string `yaml:"modes"`
}

type EnvironmentReference struct {
	FromEnvironment string `yaml:"fromEnvironment"`
}

type Provider struct {
	Kind                   string                          `yaml:"kind"`
	Transport              string                          `yaml:"transport"`
	Command                string                          `yaml:"command"`
	Args                   []string                        `yaml:"args"`
	URL                    string                          `yaml:"url"`
	BearerTokenEnvironment string                          `yaml:"bearerTokenEnvironment"`
	Environment            map[string]EnvironmentReference `yaml:"environment"`
}

type ToolBinding struct {
	Provider   string `yaml:"provider"`
	RemoteName string `yaml:"remoteName"`
}

type AgentEndpoint struct {
	A2AURL string `yaml:"a2aUrl"`
}

type File struct {
	SchemaVersion  int                        `yaml:"schemaVersion"`
	Environment    string                     `yaml:"environment"`
	Artifacts      map[string]ArtifactBinding `yaml:"artifacts"`
	Providers      map[string]Provider        `yaml:"providers"`
	ToolBindings   map[string]ToolBinding     `yaml:"toolBindings"`
	AgentEndpoints map[string]AgentEndpoint   `yaml:"agentEndpoints"`
}

type dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

type toolImport struct {
	ToolID  string `json:"toolId"`
	SkillID string `json:"skillId"`
	Mode    string `json:"mode"`
}

// requirementsV1 is a neutral artifact's link manifest.
type requirementsV1 struct {
	SchemaVersion int          `json:"schemaVersion"`
	AgentID       string       `json:"agentId"`
	Target        string       `json:"target"`
	DefaultMode   string       `json:"defaultMode"`
	SourceDigest  string       `json:"sourceDigest"`
	Dependencies  []dependency `json:"dependencies"`
	Modes         []string     `json:"modes"`
	ToolImports   []toolImport `json:"toolImports"`
}

// requirementsV2 is a plugin artifact's link manifest (ADR-0029).
type requirementsV2 struct {
	SchemaVersion int          `json:"schemaVersion"`
	ID            string       `json:"id"`
	Kind          string       `json:"kind"`
	Target        string       `json:"target"`
	Mode          string       `json:"mode"`
	SourceDigest  string       `json:"sourceDigest"`
	Dependencies  []dependency `json:"dependencies"`
	Modes         []string     `json:"modes"`
	ToolImports   []toolImport `json:"toolImports"`
}

// requirements is either manifest version, normalized.
type requirements struct {
	schemaVersion int
	ID            string
	Target        string
	// Mode is the one mode a fixed-mode artifact materializes; empty for the
	// neutral artifact, which carries every mode.
	Mode         string
	SourceDigest string
	Modes        []string
	ToolImports  []toolImport
	path         string
	slug         string
}

func (r requirements) plugin() bool { return r.schemaVersion == 2 }

type indexArtifact struct {
	AgentID string `json:"agentId,omitempty"`
	ID      string `json:"id,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Path    string `json:"path"`
	// SourceDigest is a plugin artifact's owning package source digest
	// (build index schemaVersion 2, ADR-0034).
	SourceDigest string `json:"sourceDigest,omitempty"`
	Digest       string `json:"digest"`
}

type buildIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	Target        string          `json:"target"`
	SourceDigest  string          `json:"sourceDigest"`
	Artifacts     []indexArtifact `json:"artifacts"`
}

type linkProvenance struct {
	SchemaVersion    int             `json:"schemaVersion"`
	Environment      string          `json:"environment"`
	UnlinkedDigest   string          `json:"unlinkedDigest"`
	DeploymentDigest string          `json:"deploymentDigest"`
	Artifacts        []indexArtifact `json:"artifacts"`
}

func Load(path string) (*File, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, resource.Errorf("Cannot read deployment file: %s", path)
	}
	var deployment File
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&deployment); err != nil {
		return nil, nil, resource.Errorf("Invalid deployment file: %s", err)
	}
	if deployment.SchemaVersion != 1 {
		return nil, nil, resource.Errorf("deployment schemaVersion must be 1")
	}
	if strings.TrimSpace(deployment.Environment) == "" {
		return nil, nil, resource.Errorf("deployment must name its environment")
	}
	for artifactID, artifact := range deployment.Artifacts {
		if !resource.IsResourceID(artifactID) {
			return nil, nil, resource.Errorf("invalid deployment artifact id: %s", artifactID)
		}
		seenModes := map[string]bool{}
		for _, mode := range artifact.Modes {
			if !deploymentName.MatchString(mode) {
				return nil, nil, resource.Errorf("artifact %s has invalid mode %q", artifactID, mode)
			}
			if seenModes[mode] {
				return nil, nil, resource.Errorf("artifact %s selects mode %s more than once", artifactID, mode)
			}
			seenModes[mode] = true
		}
	}
	for name, provider := range deployment.Providers {
		if !deploymentName.MatchString(name) || provider.Kind != "mcp" {
			return nil, nil, resource.Errorf("provider %q must be a named MCP provider", name)
		}
		switch provider.Transport {
		case "stdio":
			if strings.TrimSpace(provider.Command) == "" || provider.URL != "" || provider.BearerTokenEnvironment != "" {
				return nil, nil, resource.Errorf("stdio provider %s requires command and cannot set url", name)
			}
		case "http":
			if provider.Command != "" || len(provider.Args) != 0 || len(provider.Environment) != 0 {
				return nil, nil, resource.Errorf("http provider %s uses url, not command/args", name)
			}
			if err := validateHTTPS(provider.URL, "provider "+name+" url"); err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, resource.Errorf("provider %s transport must be stdio or http", name)
		}
		for variable, reference := range provider.Environment {
			if strings.TrimSpace(variable) == "" || strings.TrimSpace(reference.FromEnvironment) == "" {
				return nil, nil, resource.Errorf("provider %s environment bindings must reference a host environment variable", name)
			}
			if variable != reference.FromEnvironment {
				return nil, nil, resource.Errorf("provider %s cannot rename environment variable %s to %s in deployment schemaVersion 1",
					name, reference.FromEnvironment, variable)
			}
		}
	}
	for toolID, binding := range deployment.ToolBindings {
		if !resource.IsResourceID(toolID) {
			return nil, nil, resource.Errorf("invalid tool binding id: %s", toolID)
		}
		if !deploymentName.MatchString(binding.Provider) || strings.TrimSpace(binding.RemoteName) == "" {
			return nil, nil, resource.Errorf("tool %s binding must name a provider and remote tool", toolID)
		}
	}
	for agent, endpoint := range deployment.AgentEndpoints {
		if !resource.IsResourceID(agent) {
			return nil, nil, resource.Errorf("invalid agent endpoint id: %s", agent)
		}
		if endpoint.A2AURL == "" {
			return nil, nil, resource.Errorf("agent endpoint %s must declare a2aUrl", agent)
		}
		if err := validateHTTPS(endpoint.A2AURL, "agent endpoint "+agent); err != nil {
			return nil, nil, err
		}
	}
	return &deployment, data, nil
}

func Link(input, deploymentPath, output string) ([]string, error) {
	deployment, deploymentBytes, err := Load(deploymentPath)
	if err != nil {
		return nil, err
	}
	inputAbs, err := filepath.Abs(input)
	if err != nil {
		return nil, resource.Errorf("Invalid built target path: %s", input)
	}
	outputAbs, err := filepath.Abs(output)
	if err != nil || sameOrWithin(inputAbs, outputAbs) || sameOrWithin(outputAbs, inputAbs) {
		return nil, resource.Errorf("linked output must not contain or be contained by the unlinked input")
	}
	info, err := os.Stat(inputAbs)
	if err != nil || !info.IsDir() {
		return nil, resource.Errorf("Built target directory not found: %s", input)
	}
	reqs, err := loadRequirements(inputAbs)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, resource.Errorf("No .typeference/link.json manifests found under %s", input)
	}
	if err := verifyBuildIndex(inputAbs, reqs); err != nil {
		return nil, err
	}
	if err := validatePluginModeSelection(reqs, deployment); err != nil {
		return nil, err
	}
	for _, req := range reqs {
		if err := validateBindings(req, deployment); err != nil {
			return nil, err
		}
	}
	mcpConfigs := map[string]string{}
	for _, req := range reqs {
		if req.plugin() {
			config, err := mcpConfig(req, deployment)
			if err != nil {
				return nil, err
			}
			mcpConfigs[req.slug] = config
		}
	}
	inputDigest, err := compile.HashDirectory(inputAbs)
	if err != nil {
		return nil, err
	}
	if err := prepareLinkedOutput(outputAbs); err != nil {
		return nil, err
	}
	written, err := copyTree(inputAbs, outputAbs)
	if err != nil {
		return nil, err
	}
	buildIndexSource := filepath.Join(inputAbs, ".typeference", "build.json")
	buildIndexBytes, err := os.ReadFile(buildIndexSource)
	if err != nil {
		return nil, resource.Errorf("Cannot preserve unlinked build index: %s", buildIndexSource)
	}
	copiedBuildIndex := filepath.Join(outputAbs, ".typeference", "build.json")
	if err := os.Remove(copiedBuildIndex); err != nil {
		return nil, resource.Errorf("Cannot replace copied build index: %s", copiedBuildIndex)
	}
	written = withoutPath(written, copiedBuildIndex)
	unlinkedIndexPath := filepath.Join(outputAbs, ".typeference", "unlinked-build.json")
	if err := write(unlinkedIndexPath, string(buildIndexBytes)); err != nil {
		return nil, err
	}
	written = append(written, unlinkedIndexPath)
	for _, req := range reqs {
		bindings := toolBindingsJSON(req, deployment)
		bindingsPath := filepath.Join(outputAbs, filepath.FromSlash(req.slug), ".typeference", "tool-bindings.json")
		if err := write(bindingsPath, bindings); err != nil {
			return nil, err
		}
		written = append(written, bindingsPath)
		if config := mcpConfigs[req.slug]; config != "" {
			path := filepath.Join(outputAbs, filepath.FromSlash(req.slug), "mcp.json")
			if err := write(path, config); err != nil {
				return nil, err
			}
			written = append(written, path)
		}
		if !req.plugin() {
			if endpoint := deployment.AgentEndpoints[req.ID].A2AURL; endpoint != "" {
				card, err := a2aCard(inputAbs, req, endpoint)
				if err != nil {
					return nil, err
				}
				path := filepath.Join(outputAbs, req.slug, ".typeference", "a2a-agent-card.json")
				if err := write(path, card); err != nil {
					return nil, err
				}
				written = append(written, path)
			}
		}
	}
	deploymentDigest := sha256.Sum256(bytes.ReplaceAll(deploymentBytes, []byte("\r\n"), []byte("\n")))
	schemaVersion := "1"
	artifacts := jsonx.Arr{}
	for _, req := range reqs {
		digest, err := compile.HashDirectory(filepath.Join(outputAbs, filepath.FromSlash(req.slug)))
		if err != nil {
			return nil, err
		}
		if req.plugin() {
			schemaVersion = "2"
			artifacts = append(artifacts, jsonx.Obj{
				{K: "id", V: jsonx.Str(req.ID)},
				{K: "mode", V: jsonx.Str(req.Mode)},
				{K: "path", V: jsonx.Str(req.slug)},
				{K: "digest", V: jsonx.Str("sha256:" + digest)},
			})
			continue
		}
		artifacts = append(artifacts, jsonx.Obj{
			{K: "agentId", V: jsonx.Str(req.ID)},
			{K: "path", V: jsonx.Str(req.slug)},
			{K: "digest", V: jsonx.Str("sha256:" + digest)},
		})
	}
	provenance := jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num(schemaVersion)},
		{K: "environment", V: jsonx.Str(deployment.Environment)},
		{K: "unlinkedDigest", V: jsonx.Str("sha256:" + inputDigest)},
		{K: "deploymentDigest", V: jsonx.Str("sha256:" + hex.EncodeToString(deploymentDigest[:]))},
		{K: "artifacts", V: artifacts},
	}) + "\n"
	provenancePath := filepath.Join(outputAbs, ".typeference", "link-provenance.json")
	if err := write(provenancePath, provenance); err != nil {
		return nil, err
	}
	written = append(written, provenancePath)
	sort.Strings(written)
	return written, nil
}

func withoutPath(paths []string, removed string) []string {
	result := paths[:0]
	for _, path := range paths {
		if path != removed {
			result = append(result, path)
		}
	}
	return result
}

func sameOrWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && (relative == "." ||
		(!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

// prepareLinkedOutput permits recursive replacement only for a completed output
// whose root provenance proves that TypeFerence owns the directory layout.
func prepareLinkedOutput(output string) error {
	if filepath.Dir(output) == output {
		return resource.Errorf("linked output must not be a filesystem root: %s", output)
	}
	info, err := os.Lstat(output)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return resource.Errorf("Cannot inspect linked output: %s", output)
	}
	if !info.IsDir() {
		return resource.Errorf("linked output must be a directory: %s", output)
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return resource.Errorf("Cannot inspect linked output: %s", output)
	}
	if len(entries) == 0 {
		if err := os.Remove(output); err != nil {
			return resource.Errorf("Cannot reset empty linked output: %s", output)
		}
		return nil
	}
	marker := filepath.Join(output, ".typeference", "link-provenance.json")
	if !validLinkProvenance(marker) {
		return resource.Errorf("linked output is non-empty and not a TypeFerence-owned linked output: %s", output)
	}
	if err := os.RemoveAll(output); err != nil {
		return resource.Errorf("Cannot reset linked output: %s", output)
	}
	return nil
}

func validLinkProvenance(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var provenance linkProvenance
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provenance); err != nil {
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return false
	}
	if (provenance.SchemaVersion != 1 && provenance.SchemaVersion != 2) || strings.TrimSpace(provenance.Environment) == "" ||
		!validSHA256Digest(provenance.UnlinkedDigest) || !validSHA256Digest(provenance.DeploymentDigest) ||
		len(provenance.Artifacts) == 0 {
		return false
	}
	seenPaths := map[string]bool{}
	for _, artifact := range provenance.Artifacts {
		id := artifact.AgentID
		if provenance.SchemaVersion == 2 {
			id = artifact.ID
		}
		if !resource.IsResourceID(id) || !validArtifactPath(artifact.Path) || !validSHA256Digest(artifact.Digest) ||
			seenPaths[artifact.Path] {
			return false
		}
		seenPaths[artifact.Path] = true
	}
	return true
}

func validArtifactPath(path string) bool {
	return path != "" && filepath.Base(filepath.Clean(path)) == path && path != "." && path != ".."
}

func validSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size
}

func verifyBuildIndex(root string, reqs []requirements) error {
	path := filepath.Join(root, ".typeference", "build.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return resource.Errorf("Built target is missing its integrity index: %s", path)
	}
	var index buildIndex
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil || (index.SchemaVersion != 1 && index.SchemaVersion != 2) {
		return resource.Errorf("Invalid build integrity index: %s", path)
	}
	expected := map[string]indexArtifact{}
	for _, artifact := range index.Artifacts {
		id := artifact.AgentID
		if index.SchemaVersion == 2 {
			id = artifact.ID
		}
		if !validArtifactPath(artifact.Path) || !resource.IsResourceID(id) || !validSHA256Digest(artifact.Digest) {
			return resource.Errorf("Invalid artifact entry in build integrity index: %s", path)
		}
		if index.SchemaVersion == 2 && !validSHA256Digest(artifact.SourceDigest) {
			return resource.Errorf("Invalid artifact entry in build integrity index: %s", path)
		}
		if index.SchemaVersion == 1 && artifact.SourceDigest != "" {
			return resource.Errorf("Invalid artifact entry in build integrity index: %s", path)
		}
		if _, exists := expected[artifact.Path]; exists {
			return resource.Errorf("Duplicate artifact %s in build integrity index", artifact.Path)
		}
		expected[artifact.Path] = artifact
	}
	if len(expected) != len(reqs) {
		return resource.Errorf("Build integrity index does not match its link manifests")
	}
	for _, req := range reqs {
		if req.schemaVersion != index.SchemaVersion || req.Target != index.Target {
			return resource.Errorf("Link requirements for %s do not match the build integrity index", req.ID)
		}
		want, ok := expected[req.slug]
		if !ok {
			return resource.Errorf("Build integrity index does not include %s", req.slug)
		}
		// A neutral artifact records the build's source digest; a plugin
		// artifact records its owning package's, which its entry repeats.
		expectedSource := index.SourceDigest
		if index.SchemaVersion == 2 {
			expectedSource = want.SourceDigest
		}
		if req.SourceDigest != expectedSource {
			return resource.Errorf("Link requirements for %s do not match the build integrity index", req.ID)
		}
		id := want.AgentID
		if index.SchemaVersion == 2 {
			id = want.ID
		}
		if id != req.ID || want.Mode != req.Mode {
			return resource.Errorf("Build integrity index entry %s does not match %s", req.slug, req.ID)
		}
		digest, err := compile.HashDirectory(filepath.Join(root, filepath.FromSlash(req.slug)))
		if err != nil {
			return err
		}
		if want.Digest != "sha256:"+digest {
			return resource.Errorf("Unlinked artifact digest mismatch for %s: expected %s, got sha256:%s",
				req.slug, want.Digest, digest)
		}
	}
	return nil
}

// selectedModes returns the modes an artifact materializes under a
// deployment: a plugin artifact's one fixed mode, or a neutral artifact's
// selection.
func selectedModes(req requirements, deployment *File) map[string]bool {
	selected := map[string]bool{}
	if req.plugin() {
		selected[req.Mode] = true
		return selected
	}
	for _, mode := range deployment.Artifacts[req.ID].Modes {
		selected[mode] = true
	}
	return selected
}

func toolBindingsJSON(req requirements, deployment *File) string {
	selected := selectedModes(req, deployment)
	modes := make([]string, 0, len(selected))
	for mode := range selected {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	imports := append([]toolImport{}, req.ToolImports...)
	sort.Slice(imports, func(i, j int) bool {
		if imports[i].ToolID != imports[j].ToolID {
			return imports[i].ToolID < imports[j].ToolID
		}
		if imports[i].SkillID != imports[j].SkillID {
			return imports[i].SkillID < imports[j].SkillID
		}
		return imports[i].Mode < imports[j].Mode
	})
	bindings := jsonx.Arr{}
	for _, imported := range imports {
		if imported.Mode != "*" && !selected[imported.Mode] {
			continue
		}
		binding := deployment.ToolBindings[imported.ToolID]
		bindings = append(bindings, jsonx.Obj{
			{K: "toolId", V: jsonx.Str(imported.ToolID)},
			{K: "skillId", V: jsonx.Str(imported.SkillID)},
			{K: "mode", V: jsonx.Str(imported.Mode)},
			{K: "provider", V: jsonx.Str(binding.Provider)},
			{K: "remoteName", V: jsonx.Str(binding.RemoteName)},
		})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "environment", V: jsonx.Str(deployment.Environment)},
		{K: "selectedModes", V: stringArr(modes)},
		{K: "bindings", V: bindings},
	}) + "\n"
}

func stringArr(values []string) jsonx.Arr {
	result := jsonx.Arr{}
	for _, value := range values {
		result = append(result, jsonx.Str(value))
	}
	return result
}

func a2aCard(inputRoot string, req requirements, endpoint string) (string, error) {
	bundlePath := filepath.Join(inputRoot, req.slug, "bundle.json")
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return "", resource.Errorf("Cannot read bundle for A2A card: %s", req.ID)
	}
	var bundle struct {
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		Skills      []struct {
			DispatchName string                     `json:"dispatchName"`
			CapabilityID string                     `json:"capabilityId"`
			Description  string                     `json:"description"`
			Exposed      bool                       `json:"exposed"`
			Variants     map[string]json.RawMessage `json:"variants"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		return "", resource.Errorf("Invalid bundle for A2A card: %s", req.ID)
	}
	skills := jsonx.Arr{}
	for _, skill := range bundle.Skills {
		if !skill.Exposed {
			continue
		}
		if len(skill.Variants) > 0 {
			if _, supportsA2A := skill.Variants["a2a"]; !supportsA2A {
				continue
			}
		}
		skills = append(skills, jsonx.Obj{
			{K: "id", V: jsonx.Str(skill.DispatchName)},
			{K: "name", V: jsonx.Str(resourceName(skill.CapabilityID))},
			{K: "description", V: jsonx.Str(skill.Description)},
			{K: "tags", V: jsonx.Arr{jsonx.Str("typeference")}},
		})
	}
	if len(skills) == 0 {
		return "", resource.Errorf("deployment declares an A2A endpoint for %s, but it exposes no capabilities", req.ID)
	}
	version := req.ID[strings.LastIndex(req.ID, "@")+1:]
	card := jsonx.Obj{
		{K: "protocolVersion", V: jsonx.Str("0.3.0")},
		{K: "name", V: jsonx.Str(bundle.DisplayName)},
		{K: "description", V: jsonx.Str(bundle.Description)},
		{K: "version", V: jsonx.Str(version)},
		{K: "url", V: jsonx.Str(endpoint)},
		{K: "preferredTransport", V: jsonx.Str("JSONRPC")},
		{K: "capabilities", V: jsonx.Obj{
			{K: "streaming", V: jsonx.Bool(false)},
			{K: "pushNotifications", V: jsonx.Bool(false)},
		}},
		{K: "defaultInputModes", V: jsonx.Arr{jsonx.Str("application/json")}},
		{K: "defaultOutputModes", V: jsonx.Arr{jsonx.Str("application/json")}},
		{K: "skills", V: skills},
	}
	return jsonx.Indented(card) + "\n", nil
}

func resourceName(id string) string {
	leaf := id[strings.LastIndex(id, "/")+1:]
	return strings.SplitN(leaf, "@", 2)[0]
}

func loadRequirements(root string) ([]requirements, error) {
	result := []requirements{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "link.json" || filepath.Base(filepath.Dir(path)) != ".typeference" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		req, err := decodeRequirements(data)
		if err != nil {
			return resource.Errorf("Invalid link requirements: %s", path)
		}
		artifactRoot := filepath.Dir(filepath.Dir(path))
		rel, err := filepath.Rel(root, artifactRoot)
		if err != nil {
			return err
		}
		req.path = path
		req.slug = filepath.ToSlash(rel)
		result = append(result, req)
		return nil
	})
	if err != nil {
		if typed, ok := err.(*resource.Error); ok {
			return nil, typed
		}
		return nil, resource.Errorf("Cannot inspect built target: %s", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].slug < result[j].slug })
	return result, nil
}

func decodeRequirements(data []byte) (requirements, error) {
	var peek struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return requirements{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	switch peek.SchemaVersion {
	case 1:
		var v1 requirementsV1
		if err := decoder.Decode(&v1); err != nil || !resource.IsResourceID(v1.AgentID) {
			return requirements{}, resource.Errorf("invalid link requirements")
		}
		return requirements{schemaVersion: 1, ID: v1.AgentID, Target: v1.Target, Mode: v1.DefaultMode,
			SourceDigest: v1.SourceDigest, Modes: v1.Modes, ToolImports: v1.ToolImports}, nil
	case 2:
		var v2 requirementsV2
		if err := decoder.Decode(&v2); err != nil || !resource.IsResourceID(v2.ID) || v2.Kind != "plugin" || v2.Mode == "" {
			return requirements{}, resource.Errorf("invalid link requirements")
		}
		return requirements{schemaVersion: 2, ID: v2.ID, Target: v2.Target, Mode: v2.Mode,
			SourceDigest: v2.SourceDigest, Modes: v2.Modes, ToolImports: v2.ToolImports}, nil
	}
	return requirements{}, resource.Errorf("unsupported link requirements schemaVersion")
}

// validatePluginModeSelection requires a deployment to select, for every
// plugin, exactly the modes that plugin was built in: a plugin's artifacts
// link together, so no published marketplace lists an unlinked artifact.
func validatePluginModeSelection(reqs []requirements, deployment *File) error {
	built := map[string]map[string]bool{}
	for _, req := range reqs {
		if !req.plugin() {
			continue
		}
		if built[req.ID] == nil {
			built[req.ID] = map[string]bool{}
		}
		built[req.ID][req.Mode] = true
	}
	for id, modes := range built {
		artifact, ok := deployment.Artifacts[id]
		if !ok {
			return resource.Errorf("deployment does not select plugin %s", id)
		}
		selected := map[string]bool{}
		for _, mode := range artifact.Modes {
			if !modes[mode] {
				return resource.Errorf("plugin %s was not built in mode %s", id, mode)
			}
			selected[mode] = true
		}
		for mode := range modes {
			if !selected[mode] {
				return resource.Errorf("plugin %s was built in mode %s; the deployment must select every mode the plugin ships", id, mode)
			}
		}
	}
	return nil
}

func validateBindings(req requirements, deployment *File) error {
	artifact, ok := deployment.Artifacts[req.ID]
	if !ok {
		return resource.Errorf("deployment does not select artifact %s", req.ID)
	}
	if !req.plugin() {
		available := map[string]bool{}
		for _, mode := range req.Modes {
			available[mode] = true
		}
		for _, mode := range artifact.Modes {
			if !available[mode] {
				return resource.Errorf("artifact %s does not define selected mode %s", req.ID, mode)
			}
		}
		if len(req.Modes) > 0 && req.Mode != "" {
			if len(artifact.Modes) != 1 || artifact.Modes[0] != req.Mode {
				return resource.Errorf("target %s materializes mode %s for %s; deployment must select exactly that mode",
					req.Target, req.Mode, req.ID)
			}
		}
		if len(req.Modes) > 0 && req.Mode == "" && len(artifact.Modes) == 0 {
			return resource.Errorf("deployment must select at least one mode for multimodal artifact %s", req.ID)
		}
	}
	selected := selectedModes(req, deployment)
	if endpoint := deployment.AgentEndpoints[req.ID].A2AURL; endpoint != "" {
		if req.plugin() || req.Target != "neutral" {
			return resource.Errorf("A2A endpoint for %s requires a neutral artifact, not %s", req.ID, req.Target)
		}
		if len(req.Modes) > 0 && !selected["a2a"] {
			return resource.Errorf("A2A endpoint for %s requires the a2a mode to be selected", req.ID)
		}
	}
	for _, imported := range req.ToolImports {
		if imported.Mode != "*" && !selected[imported.Mode] {
			continue
		}
		binding, ok := deployment.ToolBindings[imported.ToolID]
		if !ok {
			return resource.Errorf("artifact %s mode %s requires unbound tool %s", req.ID, imported.Mode, imported.ToolID)
		}
		if _, ok := deployment.Providers[binding.Provider]; !ok {
			return resource.Errorf("tool %s refers to missing provider %s", imported.ToolID, binding.Provider)
		}
		if strings.TrimSpace(binding.RemoteName) == "" {
			return resource.Errorf("tool %s binding must name the provider's remote tool", imported.ToolID)
		}
	}
	return nil
}

// mcpConfig materializes an Agent Plugins mcp.json for a plugin artifact from
// the providers its selected tool imports use. The file holds no secret and
// references no inherited environment variable: Agent Plugins forbids a plugin
// from depending on one, so a deployment that forwards credentials through
// the environment or a bearer-token variable fails closed (ADR-0029).
func mcpConfig(req requirements, deployment *File) (string, error) {
	selected := selectedModes(req, deployment)
	names := map[string]bool{}
	for _, imported := range req.ToolImports {
		if imported.Mode != "*" && !selected[imported.Mode] {
			continue
		}
		names[deployment.ToolBindings[imported.ToolID].Provider] = true
	}
	if len(names) == 0 {
		return "", nil
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	servers := jsonx.Obj{}
	for _, name := range sorted {
		provider := deployment.Providers[name]
		switch provider.Transport {
		case "stdio":
			if len(provider.Environment) > 0 {
				return "", resource.Errorf("provider %s forwards environment variables, which a plugin's mcp.json cannot reference (Agent Plugins forbids depending on inherited environment); have the server obtain its own credentials, or configure it outside the plugin", name)
			}
			if !bareCommand.MatchString(provider.Command) && !(relativeCommand.MatchString(provider.Command) && !strings.Contains(provider.Command, "..")) {
				return "", resource.Errorf("provider %s command %q must be a bare executable name or a plugin-relative ./path to be a plugin stdio server", name, provider.Command)
			}
			server := jsonx.Obj{
				{K: "type", V: jsonx.Str("stdio")},
				{K: "command", V: jsonx.Str(provider.Command)},
			}
			if len(provider.Args) > 0 {
				args := jsonx.Arr{}
				for _, arg := range provider.Args {
					args = append(args, jsonx.Str(strings.ReplaceAll(arg, "{bundle}", "${PLUGIN_ROOT}/.typeference/bundle.json")))
				}
				server = append(server, jsonx.Member{K: "args", V: args})
			}
			servers = append(servers, jsonx.Member{K: name, V: server})
		case "http":
			if provider.BearerTokenEnvironment != "" {
				return "", resource.Errorf("provider %s authenticates with a bearer token from the environment, which a plugin's mcp.json cannot express without embedding the secret; use a server whose authorization the client manages (OAuth)", name)
			}
			servers = append(servers, jsonx.Member{K: name, V: jsonx.Obj{
				{K: "type", V: jsonx.Str("streamable-http")},
				{K: "url", V: jsonx.Str(provider.URL)},
			}})
		}
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "$schema", V: jsonx.Str(mcpSchemaURI)},
		{K: "mcpServers", V: servers},
	}) + "\n", nil
}

func validateHTTPS(raw, label string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return resource.Errorf("%s must be an absolute HTTPS URL without embedded credentials", label)
	}
	return nil
}

func copyTree(source, destination string) ([]string, error) {
	written := []string{}
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		written = append(written, target)
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("Cannot copy unlinked target: %s", err)
	}
	return written, nil
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return resource.Errorf("Cannot create linked artifact directory: %s", filepath.Dir(path))
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(content, "\r\n", "\n")), 0o644); err != nil {
		return resource.Errorf("Cannot write linked artifact: %s", path)
	}
	return nil
}

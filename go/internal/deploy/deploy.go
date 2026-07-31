// Package deploy validates external deployment bindings and materializes
// runnable target-native configuration from deterministic unlinked builds.
package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"gopkg.in/yaml.v3"
)

var deploymentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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

type requirements struct {
	SchemaVersion int    `json:"schemaVersion"`
	AgentID       string `json:"agentId"`
	Target        string `json:"target"`
	DefaultMode   string `json:"defaultMode"`
	SourceDigest  string `json:"sourceDigest"`
	Dependencies  []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Digest  string `json:"digest"`
	} `json:"dependencies"`
	Modes       []string `json:"modes"`
	ToolImports []struct {
		ToolID  string `json:"toolId"`
		SkillID string `json:"skillId"`
		Mode    string `json:"mode"`
	} `json:"toolImports"`
	path string
	slug string
}

type buildIndex struct {
	SchemaVersion int    `json:"schemaVersion"`
	Target        string `json:"target"`
	SourceDigest  string `json:"sourceDigest"`
	Artifacts     []struct {
		AgentID string `json:"agentId"`
		Path    string `json:"path"`
		Digest  string `json:"digest"`
	} `json:"artifacts"`
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
	for agent, artifact := range deployment.Artifacts {
		if !resource.IsResourceID(agent) {
			return nil, nil, resource.Errorf("invalid deployment artifact id: %s", agent)
		}
		seenModes := map[string]bool{}
		for _, mode := range artifact.Modes {
			if !deploymentName.MatchString(mode) {
				return nil, nil, resource.Errorf("artifact %s has invalid mode %q", agent, mode)
			}
			if seenModes[mode] {
				return nil, nil, resource.Errorf("artifact %s selects mode %s more than once", agent, mode)
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
		return nil, resource.Errorf("linked output must be a separate sibling of the unlinked input")
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
	for _, req := range reqs {
		if err := validateBindings(req, deployment); err != nil {
			return nil, err
		}
	}
	inputDigest, err := compile.HashDirectory(inputAbs)
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(outputAbs); err != nil {
		return nil, resource.Errorf("Cannot reset linked output: %s", outputAbs)
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
		bindingsPath := filepath.Join(outputAbs, req.slug, ".typeference", "tool-bindings.json")
		if err := write(bindingsPath, bindings); err != nil {
			return nil, err
		}
		written = append(written, bindingsPath)
		if req.Target == "codex" {
			config, err := codexConfig(req, deployment)
			if err != nil {
				return nil, err
			}
			if config != "" {
				path := filepath.Join(outputAbs, req.slug, ".codex", "config.toml")
				if err := write(path, config); err != nil {
					return nil, err
				}
				written = append(written, path)
			}
		}
		if endpoint := deployment.AgentEndpoints[req.AgentID].A2AURL; endpoint != "" {
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
	deploymentDigest := sha256.Sum256(bytes.ReplaceAll(deploymentBytes, []byte("\r\n"), []byte("\n")))
	artifacts := jsonx.Arr{}
	for _, req := range reqs {
		digest, err := compile.HashDirectory(filepath.Join(outputAbs, req.slug))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, jsonx.Obj{
			{K: "agentId", V: jsonx.Str(req.AgentID)},
			{K: "path", V: jsonx.Str(req.slug)},
			{K: "digest", V: jsonx.Str("sha256:" + digest)},
		})
	}
	provenance := jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
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

func verifyBuildIndex(root string, reqs []requirements) error {
	path := filepath.Join(root, ".typeference", "build.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return resource.Errorf("Built target is missing its integrity index: %s", path)
	}
	var index buildIndex
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil || index.SchemaVersion != 1 {
		return resource.Errorf("Invalid build integrity index: %s", path)
	}
	type expectedArtifact struct {
		path   string
		digest string
	}
	expected := map[string]expectedArtifact{}
	for _, artifact := range index.Artifacts {
		if artifact.Path == "" || filepath.Base(filepath.Clean(artifact.Path)) != artifact.Path ||
			!resource.IsResourceID(artifact.AgentID) || len(artifact.Digest) != 71 ||
			!strings.HasPrefix(artifact.Digest, "sha256:") {
			return resource.Errorf("Invalid artifact entry in build integrity index: %s", path)
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(artifact.Digest, "sha256:")); err != nil {
			return resource.Errorf("Invalid artifact digest in build integrity index: %s", path)
		}
		if _, exists := expected[artifact.AgentID]; exists {
			return resource.Errorf("Duplicate artifact %s in build integrity index", artifact.AgentID)
		}
		expected[artifact.AgentID] = expectedArtifact{path: artifact.Path, digest: artifact.Digest}
	}
	if len(expected) != len(reqs) {
		return resource.Errorf("Build integrity index does not match its link manifests")
	}
	for _, req := range reqs {
		if req.Target != index.Target || req.SourceDigest != index.SourceDigest {
			return resource.Errorf("Link requirements for %s do not match the build integrity index", req.AgentID)
		}
		want, ok := expected[req.AgentID]
		if !ok {
			return resource.Errorf("Build integrity index does not include %s", req.AgentID)
		}
		if want.path != req.slug {
			return resource.Errorf("Build integrity index path does not match %s", req.AgentID)
		}
		digest, err := compile.HashDirectory(filepath.Join(root, req.slug))
		if err != nil {
			return err
		}
		if want.digest != "sha256:"+digest {
			return resource.Errorf("Unlinked artifact digest mismatch for %s: expected %s, got sha256:%s",
				req.AgentID, want.digest, digest)
		}
	}
	return nil
}

func toolBindingsJSON(req requirements, deployment *File) string {
	selected := map[string]bool{}
	selectedModes := append([]string{}, deployment.Artifacts[req.AgentID].Modes...)
	sort.Strings(selectedModes)
	for _, mode := range selectedModes {
		selected[mode] = true
	}
	imports := append([]struct {
		ToolID  string `json:"toolId"`
		SkillID string `json:"skillId"`
		Mode    string `json:"mode"`
	}{}, req.ToolImports...)
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
		{K: "selectedModes", V: stringArr(selectedModes)},
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
	bundlePath := filepath.Join(inputRoot, req.slug, ".typeference", "bundle.json")
	if req.Target == "neutral" {
		bundlePath = filepath.Join(inputRoot, req.slug, "bundle.json")
	}
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return "", resource.Errorf("Cannot read bundle for A2A card: %s", req.AgentID)
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
		return "", resource.Errorf("Invalid bundle for A2A card: %s", req.AgentID)
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
		return "", resource.Errorf("deployment declares an A2A endpoint for %s, but it exposes no capabilities", req.AgentID)
	}
	version := req.AgentID[strings.LastIndex(req.AgentID, "@")+1:]
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
		var req requirements
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || req.SchemaVersion != 1 || !resource.IsResourceID(req.AgentID) {
			return resource.Errorf("Invalid link requirements: %s", path)
		}
		agentRoot := filepath.Dir(filepath.Dir(path))
		req.path = path
		req.slug = filepath.Base(agentRoot)
		result = append(result, req)
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("Cannot inspect built target: %s", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AgentID < result[j].AgentID })
	return result, nil
}

func validateBindings(req requirements, deployment *File) error {
	artifact, ok := deployment.Artifacts[req.AgentID]
	if !ok {
		return resource.Errorf("deployment does not select artifact %s", req.AgentID)
	}
	available := map[string]bool{}
	for _, mode := range req.Modes {
		available[mode] = true
	}
	selected := map[string]bool{}
	for _, mode := range artifact.Modes {
		if !available[mode] {
			return resource.Errorf("artifact %s does not define selected mode %s", req.AgentID, mode)
		}
		selected[mode] = true
	}
	if len(req.Modes) > 0 && req.DefaultMode != "" {
		if len(artifact.Modes) != 1 || artifact.Modes[0] != req.DefaultMode {
			return resource.Errorf("target %s materializes mode %s for %s; deployment must select exactly that mode",
				req.Target, req.DefaultMode, req.AgentID)
		}
	}
	if len(req.Modes) > 0 && req.DefaultMode == "" && len(artifact.Modes) == 0 {
		return resource.Errorf("deployment must select at least one mode for multimodal artifact %s", req.AgentID)
	}
	if endpoint := deployment.AgentEndpoints[req.AgentID].A2AURL; endpoint != "" {
		if req.Target != "neutral" {
			return resource.Errorf("A2A endpoint for %s requires a neutral artifact, not %s", req.AgentID, req.Target)
		}
		if len(req.Modes) > 0 && !selected["a2a"] {
			return resource.Errorf("A2A endpoint for %s requires the a2a mode to be selected", req.AgentID)
		}
	}
	for _, imported := range req.ToolImports {
		if imported.Mode != "*" && !selected[imported.Mode] {
			continue
		}
		binding, ok := deployment.ToolBindings[imported.ToolID]
		if !ok {
			return resource.Errorf("artifact %s mode %s requires unbound tool %s", req.AgentID, imported.Mode, imported.ToolID)
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

func codexConfig(req requirements, deployment *File) (string, error) {
	providerNames := map[string]bool{}
	artifact := deployment.Artifacts[req.AgentID]
	selected := map[string]bool{}
	for _, mode := range artifact.Modes {
		selected[mode] = true
	}
	for _, imported := range req.ToolImports {
		if imported.Mode != "*" && !selected[imported.Mode] {
			continue
		}
		providerNames[deployment.ToolBindings[imported.ToolID].Provider] = true
	}
	names := make([]string, 0, len(providerNames))
	for name := range providerNames {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		provider := deployment.Providers[name]
		builder.WriteString("[mcp_servers." + tomlKey(name) + "]\n")
		if provider.Transport == "stdio" {
			builder.WriteString("command = " + tomlString(provider.Command) + "\n")
			args := make([]string, len(provider.Args))
			for i, arg := range provider.Args {
				args[i] = tomlString(strings.ReplaceAll(arg, "{bundle}", ".typeference/bundle.json"))
			}
			builder.WriteString("args = [" + strings.Join(args, ", ") + "]\n")
			if len(provider.Environment) > 0 {
				variables := make([]string, 0, len(provider.Environment))
				for _, reference := range provider.Environment {
					variables = append(variables, reference.FromEnvironment)
				}
				sort.Strings(variables)
				quoted := make([]string, len(variables))
				for i, variable := range variables {
					quoted[i] = tomlString(variable)
				}
				builder.WriteString("env_vars = [" + strings.Join(quoted, ", ") + "]\n")
			}
		} else {
			builder.WriteString("url = " + tomlString(provider.URL) + "\n")
			if provider.BearerTokenEnvironment != "" {
				builder.WriteString("bearer_token_env_var = " + tomlString(provider.BearerTokenEnvironment) + "\n")
			}
		}
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func tomlKey(value string) string {
	safe := true
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			safe = false
			break
		}
	}
	if safe && value != "" {
		return value
	}
	return tomlString(value)
}

func tomlString(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\b':
			builder.WriteString(`\b`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\f':
			builder.WriteString(`\f`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				hexValue := strings.ToUpper(strconv.FormatInt(int64(r), 16))
				builder.WriteString(`\u`)
				builder.WriteString(strings.Repeat("0", 4-len(hexValue)))
				builder.WriteString(hexValue)
			} else {
				builder.WriteRune(r)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
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

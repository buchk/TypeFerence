package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
	"gopkg.in/yaml.v3"
)

// File is one generated source file.
type File struct {
	Path    string
	Content string
}

// Result is a generated version 6 source tree and what a person should know
// about it.
type Result struct {
	Files []File
	Notes []string
}

// Options configures an import.
type Options struct {
	// Name and Version are the new package's identity. Defaults:
	// local/<source directory name> and 0.1.0.
	Name    string
	Version string
	// Plugin names the generated plugin; defaults to the imported plugin's
	// name or the source directory's name.
	Plugin string
	// Lossy drops what version 6 cannot represent, listing each item in the
	// notes, instead of failing (ADR-0033).
	Lossy bool
}

var hostName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type importedSkill struct {
	name        string
	description string
	body        string
	source      string
}

type importedAgent struct {
	name        string
	displayName string
	description string
	body        string
	source      string
}

type importer struct {
	root        string
	lossy       bool
	skills      map[string]importedSkill
	agents      map[string]importedAgent
	unsupported []string
	notes       []string
}

// Import reads GitHub Copilot customizations under source (an Agent Plugins
// 1.0 plugin, a Copilot CLI plugin, a repository's .github/agents and skill
// directories, or a single skill directory) and returns an equivalent
// version 6 package: one plugin linking the imported agents and skills.
// Anything version 6 cannot represent fails the import unless Lossy is set.
func Import(source string, options Options) (*Result, error) {
	root, err := filepath.Abs(source)
	if err != nil {
		return nil, resource.Errorf("Import source not found: %s", source)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		return nil, resource.Errorf("Import source not found: %s", source)
	}
	im := &importer{root: root, lossy: options.Lossy, skills: map[string]importedSkill{}, agents: map[string]importedAgent{}}
	pluginName, pluginDescription := "", ""
	switch {
	case exists(filepath.Join(root, "SKILL.md")):
		if err := im.skill(root); err != nil {
			return nil, err
		}
	case exists(filepath.Join(root, "plugin.json")) || exists(filepath.Join(root, ".claude-plugin", "plugin.json")):
		name, description, err := im.plugin()
		if err != nil {
			return nil, err
		}
		pluginName, pluginDescription = name, description
	default:
		if err := im.repository(); err != nil {
			return nil, err
		}
	}
	if len(im.skills)+len(im.agents) == 0 {
		return nil, resource.Errorf("%s contains no custom agents or Agent Skills to import", source)
	}
	if len(im.unsupported) > 0 && !im.lossy {
		sort.Strings(im.unsupported)
		return nil, resource.Errorf("version 6 cannot represent:\n  %s\nrerun with --lossy to import without them", strings.Join(im.unsupported, "\n  "))
	}
	for _, item := range im.unsupported {
		im.notes = append(im.notes, "dropped: "+item)
	}

	if options.Plugin != "" {
		pluginName = options.Plugin
	}
	if pluginName == "" {
		pluginName = strings.ToLower(filepath.Base(root))
	}
	if !hostName.MatchString(pluginName) || len(pluginName) > 64 {
		return nil, resource.Errorf("plugin name %q must use lowercase letters, digits, and single hyphens; pass --plugin <name>", pluginName)
	}
	name, version := options.Name, options.Version
	if name == "" {
		name = "local/" + pluginName
	}
	if version == "" {
		version = "0.1.0"
	}
	if !resource.IsPackageName(name) || !resource.IsSemanticVersion(version) {
		return nil, resource.Errorf("package identity must be a lowercase namespace/name and an exact semantic version")
	}

	result := &Result{}
	agentNames := sortedKeys(im.agents)
	skillNames := sortedKeys(im.skills)
	agentPaths := []string{}
	for _, agentName := range agentNames {
		agent := im.agents[agentName]
		fm := &frontmatter{}
		if agent.displayName != "" && agent.displayName != agent.name {
			fm.raw(0, "displayName", agent.displayName)
		}
		fm.raw(0, "description", agent.description)
		path := "agents/" + agent.name + ".agent.tfer"
		result.Files = append(result.Files, File{Path: path, Content: document(fm, agent.body)})
		agentPaths = append(agentPaths, path)
	}
	skillPaths := []string{}
	for _, skillName := range skillNames {
		skill := im.skills[skillName]
		fm := &frontmatter{}
		fm.raw(0, "description", skill.description)
		path := "skills/" + skill.name + ".skill.tfer"
		result.Files = append(result.Files, File{Path: path, Content: document(fm, skill.body)})
		skillPaths = append(skillPaths, path)
	}
	if pluginDescription == "" {
		if len(agentNames) == 1 {
			pluginDescription = im.agents[agentNames[0]].description
		} else if len(agentNames) == 0 && len(skillNames) == 1 {
			pluginDescription = im.skills[skillNames[0]].description
		} else {
			pluginDescription = "Agents and skills imported from " + pluginName + "."
		}
	}
	plugin := &frontmatter{}
	plugin.raw(0, "description", pluginDescription)
	plugin.list(0, "agents", agentPaths)
	plugin.list(0, "skills", skillPaths)
	pluginPath := "plugins/" + pluginName + ".plugin.tfer"
	result.Files = append(result.Files, File{Path: pluginPath, Content: document(plugin, "")})
	manifest := &frontmatter{}
	manifest.token(0, "schemaVersion", "6")
	manifest.raw(0, "name", name)
	manifest.raw(0, "version", version)
	manifest.list(0, "plugins", []string{pluginPath})
	result.Files = append(result.Files, File{Path: resource.ManifestFile, Content: document(manifest, "")})
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	sort.Strings(im.notes)
	result.Notes = im.notes
	return result, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (im *importer) rel(path string) string {
	rel, err := filepath.Rel(im.root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// frontmatterOf splits a Markdown file with YAML frontmatter. Copilot agent
// profiles and Agent Skills use YAML frontmatter, so YAML reads them here.
func frontmatterOf(path string) (map[string]any, []string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", resource.Errorf("Cannot read %s", path)
	}
	text := strings.TrimPrefix(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\uFEFF")
	if !strings.HasPrefix(text, "---\n") {
		return map[string]any{}, nil, text, nil
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	body := ""
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - 4
		} else {
			return nil, nil, "", resource.Errorf("%s: unterminated frontmatter", path)
		}
	} else {
		body = rest[end+5:]
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &node); err != nil {
		return nil, nil, "", resource.Errorf("%s: invalid frontmatter: %s", path, err)
	}
	values := map[string]any{}
	keys := []string{}
	if len(node.Content) == 1 && node.Content[0].Kind == yaml.MappingNode {
		mapping := node.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key := mapping.Content[i].Value
			var value any
			if err := mapping.Content[i+1].Decode(&value); err != nil {
				return nil, nil, "", resource.Errorf("%s: invalid frontmatter value for %s", path, key)
			}
			values[key] = value
			keys = append(keys, key)
		}
	}
	return values, keys, body, nil
}

func text(values map[string]any, key string) string {
	if s, ok := values[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func (im *importer) skill(dir string) error {
	values, keys, body, err := frontmatterOf(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return err
	}
	source := im.rel(filepath.Join(dir, "SKILL.md"))
	name := filepath.Base(dir)
	if declared := text(values, "name"); declared != "" && declared != name {
		return resource.Errorf("%s: name %q does not match its directory %q (Agent Skills requires them to match)", source, declared, name)
	}
	if !hostName.MatchString(name) || len(name) > 64 {
		return resource.Errorf("%s: skill name %q must use lowercase letters, digits, and single hyphens", source, name)
	}
	description := text(values, "description")
	if description == "" {
		return resource.Errorf("%s: an Agent Skill requires a description", source)
	}
	if strings.TrimSpace(body) == "" {
		return resource.Errorf("%s: the skill has no instructions", source)
	}
	for _, key := range keys {
		if key != "name" && key != "description" {
			im.unsupported = append(im.unsupported, source+": frontmatter field '"+key+"'")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return resource.Errorf("Cannot read %s", dir)
	}
	for _, entry := range entries {
		if entry.Name() != "SKILL.md" {
			im.unsupported = append(im.unsupported, im.rel(filepath.Join(dir, entry.Name()))+": skill resource file (version 6 skills are SKILL.md text only)")
		}
	}
	if prior, taken := im.skills[name]; taken {
		return resource.Errorf("skills %s and %s share the name %q", prior.source, source, name)
	}
	im.skills[name] = importedSkill{name: name, description: description, body: body, source: source}
	return nil
}

func (im *importer) agent(path string) error {
	values, keys, body, err := frontmatterOf(path)
	if err != nil {
		return err
	}
	source := im.rel(path)
	name := strings.TrimSuffix(filepath.Base(path), ".agent.md")
	if !hostName.MatchString(name) || len(name) > 64 {
		return resource.Errorf("%s: agent name %q must use lowercase letters, digits, and single hyphens", source, name)
	}
	description := text(values, "description")
	if description == "" {
		return resource.Errorf("%s: a custom agent requires a description", source)
	}
	for _, key := range keys {
		if key != "name" && key != "description" {
			im.unsupported = append(im.unsupported, source+": frontmatter field '"+key+"'")
		}
	}
	if prior, taken := im.agents[name]; taken {
		return resource.Errorf("agents %s and %s share the name %q", prior.source, source, name)
	}
	im.agents[name] = importedAgent{name: name, displayName: text(values, "name"), description: description, body: body, source: source}
	return nil
}

func (im *importer) skillsIn(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() && exists(filepath.Join(dir, entry.Name(), "SKILL.md")) {
			if err := im.skill(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (im *importer) agentsIn(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".agent.md") {
			if err := im.agent(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// repository imports a repository's custom agents and project skills.
func (im *importer) repository() error {
	if err := im.agentsIn(filepath.Join(im.root, ".github", "agents")); err != nil {
		return err
	}
	for _, dir := range []string{".github/skills", ".agents/skills", ".claude/skills"} {
		if err := im.skillsIn(filepath.Join(im.root, filepath.FromSlash(dir))); err != nil {
			return err
		}
	}
	if exists(filepath.Join(im.root, ".github", "copilot-instructions.md")) {
		im.notes = append(im.notes, ".github/copilot-instructions.md is repository-wide instructions, not an agent or skill; it was not imported")
	}
	return nil
}

// plugin imports an Agent Plugins 1.0 plugin (plugin.json declaring the 1.0
// $schema) or a Copilot CLI / Claude-format plugin.
func (im *importer) plugin() (string, string, error) {
	manifestPath := filepath.Join(im.root, "plugin.json")
	if !exists(manifestPath) {
		manifestPath = filepath.Join(im.root, ".claude-plugin", "plugin.json")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", "", resource.Errorf("Cannot read %s", manifestPath)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", "", resource.Errorf("%s: invalid JSON: %s", im.rel(manifestPath), err)
	}
	name, _ := manifest["name"].(string)
	description, _ := manifest["description"].(string)
	_, isV1 := manifest["$schema"]
	if isV1 {
		for _, key := range sortedKeys(manifest) {
			switch key {
			case "$schema", "name", "description", "version":
			default:
				im.notes = append(im.notes, im.rel(manifestPath)+": plugin metadata '"+key+"' is not carried into the source (the build derives plugin.json)")
			}
		}
		if err := im.skillsIn(filepath.Join(im.root, "skills")); err != nil {
			return "", "", err
		}
		if err := im.agentsIn(filepath.Join(im.root, "com.github.copilot", "agents")); err != nil {
			return "", "", err
		}
		for _, unsupported := range []string{"mcp.json", "com.github.copilot/hooks", "com.github.copilot/commands", "com.github.copilot/rules", "com.github.copilot/lsp.json"} {
			if exists(filepath.Join(im.root, filepath.FromSlash(unsupported))) {
				im.unsupported = append(im.unsupported, unsupported+": not representable as version 6 source (MCP servers are deployment bindings; hooks, commands, rules, and LSP servers have no source kind)")
			}
		}
		return name, description, nil
	}
	agentsDir := "agents"
	if value, ok := manifest["agents"].(string); ok && value != "" {
		agentsDir = value
	}
	if err := im.agentsIn(filepath.Join(im.root, filepath.FromSlash(strings.TrimPrefix(agentsDir, "./")))); err != nil {
		return "", "", err
	}
	skillDirs := []string{"skills"}
	if values, ok := manifest["skills"].([]any); ok {
		skillDirs = nil
		for _, value := range values {
			if s, isString := value.(string); isString {
				skillDirs = append(skillDirs, strings.TrimPrefix(s, "./"))
			}
		}
	} else if value, ok := manifest["skills"].(string); ok {
		skillDirs = []string{strings.TrimPrefix(value, "./")}
	}
	for _, dir := range skillDirs {
		full := filepath.Join(im.root, filepath.FromSlash(dir))
		if exists(filepath.Join(full, "SKILL.md")) {
			if err := im.skill(full); err != nil {
				return "", "", err
			}
			continue
		}
		if err := im.skillsIn(full); err != nil {
			return "", "", err
		}
	}
	for _, key := range []string{"hooks", "mcpServers", "commands", "lspServers"} {
		if _, ok := manifest[key]; ok {
			im.unsupported = append(im.unsupported, im.rel(manifestPath)+": '"+key+"' has no version 6 source kind")
		}
	}
	for _, unsupported := range []string{"hooks", "hooks.json", ".mcp.json", "commands"} {
		if exists(filepath.Join(im.root, unsupported)) {
			im.unsupported = append(im.unsupported, unsupported+": has no version 6 source kind")
		}
	}
	return name, description, nil
}

// Write writes a generated tree beneath out, which must not exist or must be
// empty, so nothing a person wrote is ever overwritten.
func Write(files []File, out string) error {
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		return resource.Errorf("output directory %s is not empty; choose a new directory", out)
	}
	for _, file := range files {
		full := filepath.Join(out, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return resource.Errorf("Cannot create directory: %s", filepath.Dir(full))
		}
		if err := os.WriteFile(full, []byte(file.Content), 0o644); err != nil {
			return resource.Errorf("Cannot write file: %s", full)
		}
	}
	return nil
}

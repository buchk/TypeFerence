package compile

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

const (
	// pluginSchemaURI identifies the Agent Plugins 1.0 manifest schema.
	pluginSchemaURI = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	// mcpSchemaURI identifies the Agent Plugins 1.0 MCP configuration schema.
	mcpSchemaURI = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
)

// pluginNamePattern is the Agent Plugins 1.0 plugin name grammar, without the
// consecutive "--" and ".." exclusions, which are checked separately.
var pluginNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// pluginPlan is one plugin document resolved into what it ships.
type pluginPlan struct {
	ID          string
	Name        string
	Description string
	// Version is the owning package's version.
	Version string
	// Provenance is the owning package's: its source digest and the locked
	// packages in its dependency closure.
	Provenance buildProvenance
	Modes      []string
	Agents     []*resolve.ResolvedAgent
	// Skills are every linked agent's resolved skills, every linked
	// profile's skills, and every directly linked skill, one per emitted
	// name, ordered by name.
	Skills []resolve.ResolvedSkill
}

// pluginArtifact is one emitted directory: a plugin rendered in one mode.
type pluginArtifact struct {
	plan *pluginPlan
	mode string
	dir  string
}

func artifactsOf(plans []*pluginPlan) []pluginArtifact {
	artifacts := []pluginArtifact{}
	for _, plan := range plans {
		for _, mode := range plan.Modes {
			dir := plan.Name
			if mode != "manual" {
				dir += "-" + mode
			}
			artifacts = append(artifacts, pluginArtifact{plan: plan, mode: mode, dir: dir})
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].dir < artifacts[j].dir })
	return artifacts
}

func validPluginName(name string) bool {
	return len(name) <= 64 && pluginNamePattern.MatchString(name) &&
		!strings.Contains(name, "--") && !strings.Contains(name, "..")
}

// planPlugins resolves every plugin the build ships (its own and the
// dependency plugins its manifest lists, sorted by identity).
func planPlugins(c *compilation, agents map[string]*resolve.ResolvedAgent, ids []string, owners map[string]buildProvenance) ([]*pluginPlan, error) {
	plans := []*pluginPlan{}
	for _, id := range ids {
		doc, ok := c.docs[id]
		if !ok || doc.Kind != "plugin" {
			return nil, resource.Errorf("%s ships %s, which is not a plugin in this build", c.project.Name, id)
		}
		plan, err := planPlugin(c, doc, agents, owners[doc.Package])
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func planPlugin(c *compilation, doc *resource.Document, agents map[string]*resolve.ResolvedAgent, owner buildProvenance) (*pluginPlan, error) {
	plan := &pluginPlan{
		ID:          doc.ID,
		Name:        resource.Leaf(doc.ID),
		Description: doc.Description,
		Version:     resource.VersionOf(doc.ID),
		Provenance:  owner,
		Modes:       append([]string{}, doc.PluginModes...),
	}
	if !validPluginName(plan.Name) {
		return nil, resource.Errorf("%s: plugin name '%s' is not a valid Agent Plugins name (lowercase letters, digits, '.', '-'; no '--' or '..'; at most 64 characters)", doc.Path, plan.Name)
	}
	for _, mode := range plan.Modes {
		if mode != "manual" && !validPluginName(plan.Name+"-"+mode) {
			return nil, resource.Errorf("%s: plugin name '%s-%s' for the %s artifact exceeds the Agent Plugins name limit", doc.Path, plan.Name, mode, mode)
		}
	}
	byName := map[string]resolve.ResolvedSkill{}
	add := func(skill resolve.ResolvedSkill) error {
		if prior, exists := byName[skill.Name]; exists {
			if prior.Key() != skill.Key() {
				return resource.Errorf("%s: %s and %s both emit the skill name '%s'", doc.Path, prior.Key(), skill.Key(), skill.Name)
			}
			return nil
		}
		byName[skill.Name] = skill
		return nil
	}
	for _, agentID := range doc.PluginAgents {
		agent, ok := agents[agentID]
		if !ok {
			return nil, resource.Errorf("%s: links agent %s, which is not an agent in this build", doc.Path, agentID)
		}
		if name := resource.Leaf(agent.ID); !resource.IsHostName(name) {
			return nil, resource.Errorf("%s: agent name '%s' must use lowercase letters, digits, and single hyphens (at most 64 characters) to be a Copilot custom agent", doc.Path, name)
		}
		plan.Agents = append(plan.Agents, agent)
		for _, skill := range agent.Skills {
			if err := add(skill); err != nil {
				return nil, err
			}
		}
	}
	for _, profileID := range doc.PluginProfiles {
		profile, err := c.resolver.ResolveProfile(profileID)
		if err != nil {
			return nil, err
		}
		for _, skill := range profile.Skills {
			if err := add(skill); err != nil {
				return nil, err
			}
		}
	}
	for _, skillID := range doc.PluginSkills {
		skill, err := c.resolver.ResolveSkill(skillID)
		if err != nil {
			return nil, err
		}
		if err := add(skill); err != nil {
			return nil, err
		}
	}
	for _, name := range resource.SortedKeys(byName) {
		skill := byName[name]
		if !resource.IsHostName(name) {
			return nil, resource.Errorf("%s: skill name '%s' (from %s) must use lowercase letters, digits, and single hyphens, at most 64 characters (Agent Skills)", doc.Path, name, skill.ImplementationID)
		}
		for _, mode := range plan.Modes {
			if len(skill.Variants) > 0 {
				if _, ok := skill.Variants[mode]; !ok {
					return nil, resource.Errorf("%s: the %s artifact needs a %s variant, but multimodal skill %s has none", doc.Path, mode, mode, skill.ImplementationID)
				}
			}
		}
		if _, err := skillDirectory(skill); err != nil {
			return nil, err
		}
		plan.Skills = append(plan.Skills, skill)
	}
	return plan, nil
}

// validateLibraryNames enforces the names a GitHub Copilot install pools:
// every emitted skill name maps to one skill or instance across the whole
// build, every custom agent name to one agent, every plugin artifact name to
// one plugin and mode, and every server name to one server document.
func validateLibraryNames(c *compilation) error {
	skills := map[string]string{}
	agentNames := map[string]string{}
	artifactNames := map[string]string{}
	servers := map[string]string{}
	for _, plan := range c.plugins {
		for _, skill := range plan.Skills {
			if prior, exists := skills[skill.Name]; exists && prior != skill.Key() {
				return resource.Errorf("%s and %s both emit the skill name '%s'; installed skills share one namespace, so rename one", prior, skill.Key(), skill.Name)
			}
			skills[skill.Name] = skill.Key()
			for _, mode := range plan.Modes {
				for _, serverID := range skill.ServersFor(mode) {
					name := resource.Leaf(serverID)
					if prior, exists := servers[name]; exists && prior != serverID {
						return resource.Errorf("servers %s and %s both emit the MCP server name '%s'; one server name denotes one configuration", prior, serverID, name)
					}
					servers[name] = serverID
				}
			}
		}
		for _, agent := range plan.Agents {
			name := resource.Leaf(agent.ID)
			if prior, exists := agentNames[name]; exists && prior != agent.ID {
				return resource.Errorf("agents %s and %s both emit the custom agent name '%s'", prior, agent.ID, name)
			}
			agentNames[name] = agent.ID
		}
	}
	for _, artifact := range artifactsOf(c.plugins) {
		if prior, exists := artifactNames[artifact.dir]; exists {
			return resource.Errorf("plugins %s and %s both emit the plugin name '%s'", prior, artifact.plan.ID, artifact.dir)
		}
		artifactNames[artifact.dir] = artifact.plan.ID
	}
	return nil
}

// skillEntry is one file in a skill directory besides SKILL.md.
type skillEntry struct {
	path string
	data []byte
}

// skillDirectory lists the files a skill ships beside SKILL.md: rendered
// documents, plain files, and schemas, failing on any collision.
func skillDirectory(skill resolve.ResolvedSkill) ([]skillEntry, error) {
	entries := []skillEntry{}
	owners := map[string]string{}
	claim := func(path, owner string, data []byte) error {
		if prior, exists := owners[path]; exists {
			return resource.Errorf("skill %s: %s and %s both ship as %s", skill.Name, prior, owner, path)
		}
		owners[path] = owner
		entries = append(entries, skillEntry{path: path, data: data})
		return nil
	}
	for _, document := range skill.Documents {
		if document.Render != "file" {
			continue
		}
		content := strings.TrimRight(document.Content, "\n") + "\n"
		if err := claim("references/"+resource.Leaf(document.ID)+".md", document.ID, []byte(content)); err != nil {
			return nil, err
		}
	}
	for _, file := range skill.Files {
		if err := claim(file.As, file.Package+":"+file.Source, file.Data); err != nil {
			return nil, err
		}
	}
	if skill.HasInputSchema {
		if err := claim("references/input.schema.json", "inputSchema", []byte(canonicalSchema(skill.InputSchema))); err != nil {
			return nil, err
		}
	}
	if skill.HasOutputSchema {
		if err := claim("references/output.schema.json", "outputSchema", []byte(canonicalSchema(skill.OutputSchema))); err != nil {
			return nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	return entries, nil
}

func canonicalSchema(schema string) string {
	value, err := jsonx.Parse(schema)
	if err != nil {
		return schema + "\n"
	}
	return jsonx.Indented(value) + "\n"
}

// writeAgentPlugins emits the agent-plugin target: a marketplace repository
// root holding one Agent Plugins 1.0 package per plugin and mode.
func (c *compilation) writeAgentPlugins(root string, written *[]string) error {
	artifacts := artifactsOf(c.plugins)
	for _, artifact := range artifacts {
		if err := c.writePluginArtifact(root, artifact, written); err != nil {
			return err
		}
	}
	if marketplace := c.project.Marketplace; marketplace != nil {
		if err := writeFile(filepath.Join(root, ".github", "plugin", "marketplace.json"), marketplaceJSON(marketplace, c.project.Version, artifacts)+"\n", written); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(root, ".typeference", "compatibility.json"), compatibilityJSON(artifacts)+"\n", written); err != nil {
		return err
	}
	return writeBuildIndex(root, artifacts, c.provenance, written)
}

// artifactServers returns the servers an artifact's skills require in its
// mode, sorted by server name.
func (c *compilation) artifactServers(artifact pluginArtifact) []*resource.Document {
	byName := map[string]*resource.Document{}
	for _, skill := range artifact.plan.Skills {
		for _, id := range skill.ServersFor(artifact.mode) {
			byName[resource.Leaf(id)] = c.docs[id]
		}
	}
	servers := []*resource.Document{}
	for _, name := range resource.SortedKeys(byName) {
		servers = append(servers, byName[name])
	}
	return servers
}

func (c *compilation) writePluginArtifact(root string, artifact pluginArtifact, written *[]string) error {
	dir := filepath.Join(root, artifact.dir)
	manifest := jsonx.Obj{
		{K: "$schema", V: jsonx.Str(pluginSchemaURI)},
		{K: "name", V: jsonx.Str(artifact.dir)},
		{K: "version", V: jsonx.Str(artifact.plan.Version)},
		{K: "description", V: jsonx.Str(artifact.plan.Description)},
	}
	if err := writeFile(filepath.Join(dir, "plugin.json"), jsonx.Indented(manifest)+"\n", written); err != nil {
		return err
	}
	servers := c.artifactServers(artifact)
	if len(servers) > 0 {
		if err := writeFile(filepath.Join(dir, "mcp.json"), mcpJSON(servers)+"\n", written); err != nil {
			return err
		}
	}
	for _, agent := range artifact.plan.Agents {
		path := filepath.Join(dir, "com.github.copilot", "agents", resource.Leaf(agent.ID)+".agent.md")
		if err := writeFile(path, renderAgent(agent), written); err != nil {
			return err
		}
	}
	for _, skill := range artifact.plan.Skills {
		skillDir := filepath.Join(dir, "skills", skill.Name)
		if err := writeFile(filepath.Join(skillDir, "SKILL.md"), renderSkill(skill, artifact.mode), written); err != nil {
			return err
		}
		entries, err := skillDirectory(skill)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := writeBytes(filepath.Join(skillDir, filepath.FromSlash(entry.path)), entry.data, written); err != nil {
				return err
			}
		}
	}
	return writeFile(filepath.Join(dir, ".typeference", "bundle.json"), bundleJSON(artifact, servers)+"\n", written)
}

// mcpJSON renders an artifact's Agent Plugins 1.0 MCP configuration.
func mcpJSON(servers []*resource.Document) string {
	entries := jsonx.Obj{}
	for _, doc := range servers {
		entries = append(entries, jsonx.Member{K: resource.Leaf(doc.ID), V: serverValue(doc.Server)})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "$schema", V: jsonx.Str(mcpSchemaURI)},
		{K: "mcpServers", V: entries},
	})
}

func serverValue(s *resource.ServerConfig) jsonx.Obj {
	stringMap := func(values map[string]string) jsonx.Obj {
		obj := jsonx.Obj{}
		for _, key := range resource.SortedKeys(values) {
			obj = append(obj, jsonx.Member{K: key, V: jsonx.Str(values[key])})
		}
		return obj
	}
	if s.Transport == "stdio" {
		obj := jsonx.Obj{
			{K: "type", V: jsonx.Str("stdio")},
			{K: "command", V: jsonx.Str(s.Command)},
		}
		if s.Args != nil {
			obj = append(obj, jsonx.Member{K: "args", V: stringArr(s.Args)})
		}
		if s.Env != nil {
			obj = append(obj, jsonx.Member{K: "env", V: stringMap(s.Env)})
		}
		if s.Cwd != "" {
			obj = append(obj, jsonx.Member{K: "cwd", V: jsonx.Str(s.Cwd)})
		}
		return obj
	}
	obj := jsonx.Obj{
		{K: "type", V: jsonx.Str("streamable-http")},
		{K: "url", V: jsonx.Str(s.URL)},
	}
	if s.Headers != nil {
		obj = append(obj, jsonx.Member{K: "headers", V: stringMap(s.Headers)})
	}
	return obj
}

// frontmatterFields renders opt-in Copilot fields in table order.
func frontmatterFields(b *strings.Builder, c resource.CopilotFields, skill bool) {
	boolean := func(key string, value *bool) {
		if value == nil {
			return
		}
		text := "false"
		if *value {
			text = "true"
		}
		b.WriteString(key + ": " + text + "\n")
	}
	list := func(key string, values []string) {
		if values == nil {
			return
		}
		b.WriteString(key + ":\n")
		for _, value := range values {
			b.WriteString("  - " + escapeYAML(value) + "\n")
		}
	}
	if skill {
		if c.ArgumentHint != nil {
			b.WriteString("argument-hint: " + escapeYAML(*c.ArgumentHint) + "\n")
		}
		boolean("user-invocable", c.UserInvocable)
		boolean("disable-model-invocation", c.DisableModelInvocation)
		list("allowed-tools", c.AllowedTools)
		return
	}
	if c.Model != nil {
		b.WriteString("model: " + escapeYAML(*c.Model) + "\n")
	}
	list("tools", c.Tools)
	boolean("user-invocable", c.UserInvocable)
	boolean("disable-model-invocation", c.DisableModelInvocation)
}

// renderAgent renders a Copilot custom agent profile: identity and
// objectives, held documents, and the names of the skills it uses.
func renderAgent(agent *resolve.ResolvedAgent) string {
	var b strings.Builder
	b.WriteString("---\nname: " + resource.Leaf(agent.ID) + "\ndescription: " + escapeYAML(agent.Description) + "\n")
	frontmatterFields(&b, agent.Copilot, false)
	b.WriteString("---\n\n# " + agent.DisplayName + "\n\n")
	for _, objective := range agent.Objectives {
		if content := strings.TrimSpace(objective.Content); content != "" {
			b.WriteString(content + "\n\n")
		}
	}
	writeDocuments(&b, agent.Documents)
	if len(agent.Skills) > 0 {
		b.WriteString("## Skills\n\n")
		for _, skill := range agent.Skills {
			b.WriteString("- " + skill.Name + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// renderSkill renders a SKILL.md: Agent Skills frontmatter with opt-in
// Copilot fields, the instructions for one mode, and inline documents.
func renderSkill(skill resolve.ResolvedSkill, mode string) string {
	var b strings.Builder
	b.WriteString("---\nname: " + skill.Name + "\ndescription: " + escapeYAML(skill.Description) + "\n")
	frontmatterFields(&b, skill.Copilot, true)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(skill.InstructionsFor(mode)) + "\n\n")
	inline := []resolve.ResolvedDocument{}
	for _, document := range skill.Documents {
		if document.Render == "inline" {
			inline = append(inline, document)
		}
	}
	writeDocuments(&b, inline)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeDocuments(b *strings.Builder, documents []resolve.ResolvedDocument) {
	if len(documents) == 0 {
		return
	}
	b.WriteString("## Context\n\n")
	for _, document := range documents {
		b.WriteString("### " + document.Title + "\n\n")
		if content := strings.TrimSpace(document.Content); content != "" {
			b.WriteString(content + "\n\n")
		}
	}
}

// BundleJSON renders a resolved agent as canonical JSON, for inspection.
func BundleJSON(agent *resolve.ResolvedAgent) string {
	return jsonx.Indented(agentValue(agent))
}

func agentValue(agent *resolve.ResolvedAgent) jsonx.Obj {
	objectives := jsonx.Arr{}
	for _, objective := range agent.Objectives {
		objectives = append(objectives, jsonx.Obj{
			{K: "source", V: jsonx.Str(objective.Source)},
			{K: "content", V: jsonx.Str(objective.Content)},
		})
	}
	skills := jsonx.Arr{}
	for _, skill := range agent.Skills {
		skills = append(skills, jsonx.Str(skill.Name))
	}
	return jsonx.Obj{
		{K: "id", V: jsonx.Str(agent.ID)},
		{K: "name", V: jsonx.Str(resource.Leaf(agent.ID))},
		{K: "displayName", V: jsonx.Str(agent.DisplayName)},
		{K: "description", V: jsonx.Str(agent.Description)},
		{K: "embeds", V: stringArr(agent.Embeds)},
		{K: "bindings", V: bindingsValue(agent.Bindings)},
		{K: "objectives", V: objectives},
		{K: "documents", V: documentsValue(agent.Documents)},
		{K: "skills", V: skills},
		{K: "provenance", V: provenanceValue(agent.Provenance)},
	}
}

func bindingsValue(bindings []*resolve.Binding) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, b := range bindings {
		values := jsonx.Obj{}
		for _, field := range resource.SortedKeys(b.Values) {
			values = append(values, jsonx.Member{K: field, V: typedValue(b.Values[field])})
		}
		arr = append(arr, jsonx.Obj{
			{K: "parameter", V: jsonx.Str(b.Name)},
			{K: "data", V: jsonx.Str(b.DataID)},
			{K: "contextType", V: jsonx.Str(b.ContextType)},
			{K: "values", V: values},
		})
	}
	return arr
}

func typedValue(v resource.Value) jsonx.Value {
	switch v.Type {
	case "integer":
		return jsonx.Num(v.Text)
	case "boolean":
		return jsonx.Bool(v.Text == "true")
	case "list<string>":
		return stringArr(v.List)
	}
	return jsonx.Str(v.Text)
}

func documentsValue(documents []resolve.ResolvedDocument) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, document := range documents {
		arr = append(arr, jsonx.Obj{
			{K: "id", V: jsonx.Str(document.ID)},
			{K: "render", V: jsonx.Str(document.Render)},
		})
	}
	return arr
}

func skillValue(skill resolve.ResolvedSkill, mode string) jsonx.Obj {
	files := jsonx.Arr{}
	for _, file := range skill.Files {
		files = append(files, jsonx.Obj{
			{K: "source", V: jsonx.Str(file.Package + ":" + file.Source)},
			{K: "as", V: jsonx.Str(file.As)},
		})
	}
	return jsonx.Obj{
		{K: "name", V: jsonx.Str(skill.Name)},
		{K: "implementationId", V: jsonx.Str(skill.ImplementationID)},
		{K: "templateId", V: jsonx.Str(skill.TemplateID)},
		{K: "capabilityId", V: jsonx.Str(skill.CapabilityID)},
		{K: "description", V: jsonx.Str(skill.Description)},
		{K: "bindings", V: bindingsValue(skill.Bindings)},
		{K: "servers", V: stringArr(skill.ServersFor(mode))},
		{K: "documents", V: documentsValue(skill.Documents)},
		{K: "files", V: files},
		{K: "provenance", V: provenanceValue(skill.Provenance)},
	}
}

func provenanceValue(entries []resolve.ProvenanceEntry) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, entry := range entries {
		arr = append(arr, jsonx.Obj{
			{K: "field", V: jsonx.Str(entry.Field)},
			{K: "source", V: jsonx.Str(entry.Source)},
		})
	}
	return arr
}

// bundleJSON records what an artifact ships and where it came from.
func bundleJSON(artifact pluginArtifact, servers []*resource.Document) string {
	agents := jsonx.Arr{}
	for _, agent := range artifact.plan.Agents {
		agents = append(agents, agentValue(agent))
	}
	skills := jsonx.Arr{}
	for _, skill := range artifact.plan.Skills {
		skills = append(skills, skillValue(skill, artifact.mode))
	}
	serverIDs := []string{}
	for _, doc := range servers {
		serverIDs = append(serverIDs, doc.ID)
	}
	provenance := artifact.plan.Provenance
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("2")},
		{K: "id", V: jsonx.Str(artifact.plan.ID)},
		{K: "name", V: jsonx.Str(artifact.dir)},
		{K: "mode", V: jsonx.Str(artifact.mode)},
		{K: "version", V: jsonx.Str(artifact.plan.Version)},
		{K: "description", V: jsonx.Str(artifact.plan.Description)},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "dependencies", V: dependenciesJSON(provenance)},
		{K: "agents", V: agents},
		{K: "skills", V: skills},
		{K: "servers", V: stringArr(serverIDs)},
	})
}

// marketplaceJSON renders the Copilot marketplace index. Each artifact is
// listed by relative source so the target directory can be published as the
// marketplace repository root.
func marketplaceJSON(marketplace *resource.Marketplace, version string, artifacts []pluginArtifact) string {
	plugins := jsonx.Arr{}
	for _, artifact := range artifacts {
		plugins = append(plugins, jsonx.Obj{
			{K: "name", V: jsonx.Str(artifact.dir)},
			{K: "source", V: jsonx.Str("./" + artifact.dir)},
			{K: "description", V: jsonx.Str(artifact.plan.Description)},
			{K: "version", V: jsonx.Str(artifact.plan.Version)},
		})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "name", V: jsonx.Str(marketplace.Name)},
		{K: "owner", V: jsonx.Obj{{K: "name", V: jsonx.Str(marketplace.Owner)}}},
		{K: "metadata", V: jsonx.Obj{{K: "version", V: jsonx.Str(version)}}},
		{K: "plugins", V: plugins},
	})
}

// writeBuildIndex writes the target's integrity index: one entry per plugin
// and mode, each with its owning package's source digest.
func writeBuildIndex(root string, artifacts []pluginArtifact, provenance buildProvenance, written *[]string) error {
	entries := jsonx.Arr{}
	for _, artifact := range artifacts {
		digest, err := HashDirectory(filepath.Join(root, artifact.dir))
		if err != nil {
			return err
		}
		entries = append(entries, jsonx.Obj{
			{K: "id", V: jsonx.Str(artifact.plan.ID)},
			{K: "mode", V: jsonx.Str(artifact.mode)},
			{K: "path", V: jsonx.Str(artifact.dir)},
			{K: "sourceDigest", V: jsonx.Str(artifact.plan.Provenance.SourceDigest)},
			{K: "digest", V: jsonx.Str("sha256:" + digest)},
		})
	}
	index := jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("2")},
		{K: "target", V: jsonx.Str(TargetName)},
		{K: "sourceDigest", V: jsonx.Str(provenance.SourceDigest)},
		{K: "artifacts", V: entries},
	}) + "\n"
	return writeFile(filepath.Join(root, ".typeference", "build.json"), index, written)
}

func dependenciesJSON(provenance buildProvenance) jsonx.Arr {
	dependencies := jsonx.Arr{}
	for _, dependency := range provenance.Dependencies {
		dependencies = append(dependencies, jsonx.Obj{
			{K: "name", V: jsonx.Str(dependency.Name)},
			{K: "version", V: jsonx.Str(dependency.Version)},
			{K: "digest", V: jsonx.Str(dependency.Digest)},
		})
	}
	return dependencies
}

func stringArr(values []string) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, v := range values {
		arr = append(arr, jsonx.Str(v))
	}
	return arr
}

// Conflict is one capability that more than one distinct emitted skill
// implements in a mode: plugins that ship different members of that family
// compete for the same requests when installed together.
type Conflict struct {
	Mode         string
	CapabilityID string
	Members      []ConflictMember
}

// ConflictMember is one plugin artifact's skill in a conflicting family.
type ConflictMember struct {
	Plugin string
	Skill  string
}

// compatibilityConflicts computes the compatibility report: per mode, every
// capability implemented by skills from more than one implementation,
// members sorted by plugin artifact and skill. Instances of one template are
// one implementation: their data distinguishes them by design.
func compatibilityConflicts(artifacts []pluginArtifact) []Conflict {
	conflicts := []Conflict{}
	for _, mode := range []string{"manual", "pipeline"} {
		families := map[string][]ConflictMember{}
		implementations := map[string]map[string]bool{}
		for _, artifact := range artifacts {
			if artifact.mode != mode {
				continue
			}
			for _, skill := range artifact.plan.Skills {
				families[skill.CapabilityID] = append(families[skill.CapabilityID], ConflictMember{artifact.dir, skill.Name})
				if implementations[skill.CapabilityID] == nil {
					implementations[skill.CapabilityID] = map[string]bool{}
				}
				key := skill.ImplementationID
				if skill.TemplateID != "" {
					key = skill.TemplateID
				}
				implementations[skill.CapabilityID][key] = true
			}
		}
		for _, capability := range resource.SortedKeys(families) {
			if len(implementations[capability]) < 2 {
				continue
			}
			members := families[capability]
			sort.Slice(members, func(i, j int) bool {
				if members[i].Plugin != members[j].Plugin {
					return members[i].Plugin < members[j].Plugin
				}
				return members[i].Skill < members[j].Skill
			})
			conflicts = append(conflicts, Conflict{Mode: mode, CapabilityID: capability, Members: members})
		}
	}
	return conflicts
}

// compatibilityJSON renders the compatibility report.
func compatibilityJSON(artifacts []pluginArtifact) string {
	conflicts := jsonx.Arr{}
	for _, conflict := range compatibilityConflicts(artifacts) {
		entries := jsonx.Arr{}
		for _, m := range conflict.Members {
			entries = append(entries, jsonx.Obj{
				{K: "plugin", V: jsonx.Str(m.Plugin)},
				{K: "skill", V: jsonx.Str(m.Skill)},
			})
		}
		conflicts = append(conflicts, jsonx.Obj{
			{K: "mode", V: jsonx.Str(conflict.Mode)},
			{K: "capabilityId", V: jsonx.Str(conflict.CapabilityID)},
			{K: "members", V: entries},
		})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "conflicts", V: conflicts},
	})
}

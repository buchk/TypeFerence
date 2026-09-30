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

// Agent Plugins 1.0 identifies its manifest schema by this URI; declaring it
// opts a plugin into the 1.0 format (ADR-0029).
const pluginSchemaURI = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

var (
	// pluginNamePattern is the Agent Plugins 1.0 plugin name grammar, without
	// the consecutive "--" and ".." exclusions, which are checked separately.
	pluginNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
	// hostNamePattern is the Agent Skills name grammar, also used for custom
	// agent names so each is a clean `--agent` and slash-command token.
	hostNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// pluginPlan is one plugin document resolved into what it ships.
type pluginPlan struct {
	ID          string
	Name        string
	Description string
	// Version is the owning package's version (ADR-0034).
	Version string
	// Provenance is the owning package's: its source digest and the locked
	// packages in its dependency closure (ADR-0034).
	Provenance buildProvenance
	Modes      []string
	Agents     []*resolve.ResolvedAgent
	// Skills are the union of every linked agent's resolved skills, every
	// linked profile's skills, and every directly linked skill, one per
	// implementation, ordered by emitted name.
	Skills []resolve.ResolvedSkill
	// direct marks skills that ship without an agent to supply their context.
	direct map[string]bool
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

func validHostName(name string) bool {
	return len(name) <= 64 && hostNamePattern.MatchString(name)
}

// planPlugins resolves every plugin the build ships (its own and the
// dependency plugins its manifest lists, sorted by identity) and enforces the
// rules a plugin must satisfy to ship (ADR-0029, ADR-0030, ADR-0034).
func planPlugins(c *compilation, docs map[string]*resource.Document, agents map[string]*resolve.ResolvedAgent, ids []string, owners map[string]buildProvenance) ([]*pluginPlan, error) {
	plans := []*pluginPlan{}
	for _, id := range ids {
		doc, ok := docs[id]
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
		Version:     doc.ID[strings.LastIndex(doc.ID, "@")+1:],
		Provenance:  owner,
		Modes:       append([]string{}, doc.PluginModes...),
		direct:      map[string]bool{},
	}
	if !validPluginName(plan.Name) {
		return nil, resource.Errorf("%s: plugin name '%s' is not a valid Agent Plugins name (lowercase letters, digits, '.', '-'; no '--' or '..'; at most 64 characters)", doc.Path, plan.Name)
	}
	for _, mode := range plan.Modes {
		if mode != "manual" && !validPluginName(plan.Name+"-"+mode) {
			return nil, resource.Errorf("%s: plugin name '%s-%s' for the %s artifact exceeds the Agent Plugins name limit", doc.Path, plan.Name, mode, mode)
		}
	}
	byImplementation := map[string]resolve.ResolvedSkill{}
	add := func(skill resolve.ResolvedSkill, direct bool) {
		if _, exists := byImplementation[skill.ImplementationID]; !exists {
			// The shipped skill is the skill itself, not one agent's binding of it.
			skill.DispatchName, skill.Sealed, skill.Required = "", false, false
			byImplementation[skill.ImplementationID] = skill
		}
		if direct {
			plan.direct[skill.ImplementationID] = true
		}
	}
	for _, agentID := range doc.PluginAgents {
		agent, ok := agents[agentID]
		if !ok {
			return nil, resource.Errorf("%s: links agent %s, which is not an agent in this build", doc.Path, agentID)
		}
		name := resource.Leaf(agent.ID)
		if !validHostName(name) {
			return nil, resource.Errorf("%s: agent name '%s' must use lowercase letters, digits, and single hyphens (at most 64 characters) to be a Copilot custom agent", doc.Path, name)
		}
		for _, mode := range plan.Modes {
			if err := validateModeContext(agent, doc.Path+" mode "+mode, []string{mode}); err != nil {
				return nil, err
			}
		}
		plan.Agents = append(plan.Agents, agent)
		for _, skill := range agent.Skills {
			add(skill, false)
		}
	}
	for _, profileID := range doc.PluginProfiles {
		profile, err := c.resolver.ResolveProfile(profileID)
		if err != nil {
			return nil, err
		}
		bound := map[string]bool{}
		for _, skill := range profile.Skills {
			bound[skill.CapabilityID] = true
		}
		for _, capability := range profile.RequiredCapabilities {
			if !bound[capability] {
				return nil, resource.Errorf("%s: ships profile %s, which leaves required capability %s unbound; only a complete profile ships without an agent", doc.Path, profileID, capability)
			}
		}
		if len(profile.Context) > 0 {
			return nil, resource.Errorf("%s: ships profile %s without an agent, but the profile holds context %s; team context belongs to an agent, so link an agent that embeds the profile instead", doc.Path, profileID, profile.Context[0])
		}
		for _, skill := range profile.Skills {
			add(skill, true)
		}
	}
	for _, skillID := range doc.PluginSkills {
		skill, err := c.resolver.ResolveSkill(skillID)
		if err != nil {
			return nil, err
		}
		add(skill, true)
	}
	names := make([]string, 0, len(byImplementation))
	byName := map[string]string{}
	for _, skill := range byImplementation {
		name := skillName(skill)
		if prior, exists := byName[name]; exists {
			return nil, resource.Errorf("%s: skills %s and %s both emit the skill name '%s'", doc.Path, prior, skill.ImplementationID, name)
		}
		byName[name] = skill.ImplementationID
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		skill := byImplementation[byName[name]]
		if !validHostName(name) {
			return nil, resource.Errorf("%s: skill name '%s' (from %s) must use lowercase letters, digits, and single hyphens, at most 64 characters (Agent Skills)", doc.Path, name, skill.ImplementationID)
		}
		for _, mode := range plan.Modes {
			if len(skill.Variants) > 0 {
				if _, ok := skill.Variants[mode]; !ok {
					return nil, resource.Errorf("%s: the %s artifact needs a %s variant, but multimodal skill %s has none", doc.Path, mode, mode, skill.ImplementationID)
				}
			}
			if plan.direct[skill.ImplementationID] {
				independent, err := skillIndependentIn(skill, mode)
				if err != nil {
					return nil, err
				}
				if !independent {
					return nil, resource.Errorf("%s: skill %s requires context that only an agent can provide, so it ships only through an agent that holds that context (ADR-0030)", doc.Path, skill.ImplementationID)
				}
			}
		}
		plan.Skills = append(plan.Skills, skill)
	}
	return plan, nil
}

// skillIndependentIn reports whether a skill's own context satisfies every
// context type it requires in one mode.
func skillIndependentIn(skill resolve.ResolvedSkill, mode string) (bool, error) {
	available := map[string]bool{}
	for _, context := range skill.ContextObjects {
		for _, contextType := range context.Satisfies {
			available[contextType] = true
		}
	}
	required := append([]string{}, skill.RequiresContextTypes...)
	required = append(required, skill.VariantContextRequirements[mode]...)
	for _, contextType := range required {
		if !available[contextType] {
			return false, nil
		}
	}
	return true, nil
}

// validateLibraryNames enforces the names a GitHub Copilot install pools:
// every emitted skill name maps to one skill across the whole build, every
// custom agent name to one agent, and every plugin artifact name to one
// plugin and mode (ADR-0029, ADR-0031).
func validateLibraryNames(c *compilation) error {
	skills := map[string]string{}
	claim := func(skill resolve.ResolvedSkill) error {
		name := skillName(skill)
		if prior, exists := skills[name]; exists && prior != skill.ImplementationID {
			return resource.Errorf("skills %s and %s both emit the skill name '%s'; installed skills share one namespace, so rename one", prior, skill.ImplementationID, name)
		}
		skills[name] = skill.ImplementationID
		return nil
	}
	for _, agent := range c.agents {
		for _, skill := range agent.Skills {
			if err := claim(skill); err != nil {
				return err
			}
		}
	}
	agentNames := map[string]string{}
	artifactNames := map[string]string{}
	for _, plan := range c.plugins {
		for _, skill := range plan.Skills {
			if err := claim(skill); err != nil {
				return err
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
	return writePluginBuildIndex(root, artifacts, c.provenance, written)
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
	for _, agent := range artifact.plan.Agents {
		name := resource.Leaf(agent.ID)
		path := filepath.Join(dir, "com.github.copilot", "agents", name+".agent.md")
		if err := writeFile(path, renderPluginAgent(agent), written); err != nil {
			return err
		}
	}
	for _, skill := range artifact.plan.Skills {
		path := filepath.Join(dir, "skills", skillName(skill), "SKILL.md")
		if err := writeFile(path, renderSkill(skill, skill.InstructionsFor(artifact.mode)), written); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(dir, ".typeference", "bundle.json"), pluginBundleJSON(artifact)+"\n", written); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, ".typeference", "link.json"), pluginLinkRequirementsJSON(artifact)+"\n", written)
}

// renderPluginAgent renders a Copilot custom agent profile. The agent is
// thin: identity and objectives, held context, and the names of the skills it
// uses, which ship beside it as SKILL.md files (ADR-0030). Its description is
// routing metadata and appears only in the frontmatter (ADR-0029).
func renderPluginAgent(agent *resolve.ResolvedAgent) string {
	var b strings.Builder
	b.WriteString("---\nname: " + resource.Leaf(agent.ID) + "\ndescription: " + escapeYAML(agent.Description) + "\n---\n\n")
	b.WriteString("# " + agent.DisplayName + "\n\n")
	writeObjectives(&b, agent)
	writeContext(&b, "## Context", agent.ContextObjects)
	if len(agent.Skills) > 0 {
		names := make([]string, 0, len(agent.Skills))
		for _, skill := range agent.Skills {
			names = append(names, skillName(skill))
		}
		sort.Strings(names)
		b.WriteString("## Skills\n\n")
		for _, name := range names {
			b.WriteString("- `" + name + "`\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func pluginBundleJSON(artifact pluginArtifact) string {
	agents := jsonx.Arr{}
	for _, agent := range artifact.plan.Agents {
		agents = append(agents, bundleValue(agent))
	}
	skills := jsonx.Arr{}
	for _, skill := range artifact.plan.Skills {
		skills = append(skills, pluginSkillValue(skill, artifact.mode))
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "id", V: jsonx.Str(artifact.plan.ID)},
		{K: "name", V: jsonx.Str(artifact.dir)},
		{K: "mode", V: jsonx.Str(artifact.mode)},
		{K: "version", V: jsonx.Str(artifact.plan.Version)},
		{K: "description", V: jsonx.Str(artifact.plan.Description)},
		{K: "agents", V: agents},
		{K: "skills", V: skills},
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

// Conflict is one capability that more than one distinct emitted skill
// implements in a mode: plugins that ship different members of that family
// compete for the same requests when installed together (ADR-0031).
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
// capability with more than one distinct emitted skill, members sorted by
// plugin artifact and skill.
func compatibilityConflicts(artifacts []pluginArtifact) []Conflict {
	conflicts := []Conflict{}
	for _, mode := range []string{"manual", "pipeline"} {
		families := map[string][]ConflictMember{}
		for _, artifact := range artifacts {
			if artifact.mode != mode {
				continue
			}
			for _, skill := range artifact.plan.Skills {
				families[skill.CapabilityID] = append(families[skill.CapabilityID], ConflictMember{artifact.dir, skillName(skill)})
			}
		}
		capabilities := make([]string, 0, len(families))
		for capability := range families {
			capabilities = append(capabilities, capability)
		}
		sort.Strings(capabilities)
		for _, capability := range capabilities {
			members := families[capability]
			distinct := map[string]bool{}
			for _, m := range members {
				distinct[m.Skill] = true
			}
			if len(distinct) < 2 {
				continue
			}
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

// compatibilityJSON renders the compatibility report (ADR-0031).
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

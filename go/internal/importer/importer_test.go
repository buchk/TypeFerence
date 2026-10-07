package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const skillBody = "\nRead the diff. Flag correctness bugs first, then style.\n"

func repository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	write(t, repo, ".github/agents/reviewer.agent.md", "---\nname: Reviewer\ndescription: Reviews pull requests for the team.\n---\n\nYou review pull requests.\n")
	write(t, repo, ".github/skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff. Use when asked to review a change.\n---\n"+skillBody)
	write(t, repo, ".github/copilot-instructions.md", "Repository-wide instructions.\n")
	return repo
}

func TestImportRepositoryRoundTrips(t *testing.T) {
	result, err := Import(repository(t), Options{Name: "acme/team", Version: "1.0.0", Plugin: "team"})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "copilot-instructions.md") {
		t.Errorf("repository-wide instructions are noted, not imported: %v", result.Notes)
	}
	built := t.TempDir()
	if _, err := compile.Build(out, built, compile.BuildOptions{}); err != nil {
		t.Fatalf("imported sources must build: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(built, "agent-plugin", "team", "skills", "code-review", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), strings.TrimSpace(skillBody)) ||
		!strings.Contains(string(skill), `description: "Review a diff. Use when asked to review a change."`) {
		t.Fatalf("a built skill keeps the imported name, description, and instructions:\n%s", skill)
	}
	agent, err := os.ReadFile(filepath.Join(built, "agent-plugin", "team", "com.github.copilot", "agents", "reviewer.agent.md"))
	if err != nil || !strings.Contains(string(agent), "You review pull requests.") {
		t.Fatalf("a built agent keeps the imported instructions as its objectives: %v\n%s", err, agent)
	}
}

func TestImportFailsClosedOnUnrepresentableContent(t *testing.T) {
	repo := repository(t)
	write(t, repo, ".github/agents/reviewer.agent.md", "---\ndescription: Reviews pull requests.\nhandoffs: ['planner']\n---\nReview.\n")
	write(t, repo, ".github/skills/code-review/notes.txt", "loose file\n")
	_, err := Import(repo, Options{Plugin: "team"})
	if err == nil || !strings.Contains(err.Error(), "frontmatter field 'handoffs'") || !strings.Contains(err.Error(), "references/, scripts/, or assets/") {
		t.Fatalf("unrepresentable content must fail and be listed, got %v", err)
	}
	result, err := Import(repo, Options{Plugin: "team", Lossy: true})
	if err != nil {
		t.Fatalf("--lossy imports without the listed content: %v", err)
	}
	dropped := strings.Join(result.Notes, "\n")
	if !strings.Contains(dropped, "dropped: .github/agents/reviewer.agent.md: frontmatter field 'handoffs'") {
		t.Fatalf("a lossy import lists what it dropped:\n%s", dropped)
	}
}

func TestImportCarriesFilesServersAndCopilotFields(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit."}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\nargument-hint: \"[pr]\"\nuser-invocable: false\n---\n"+skillBody)
	write(t, plugin, "skills/code-review/references/checklist.md", "- Correctness first.\n")
	write(t, plugin, "skills/code-review/scripts/diff.sh", "git diff\n")
	write(t, plugin, "com.github.copilot/agents/reviewer.agent.md", "---\ndescription: Reviews pull requests.\ntools: ['read', 'search', 'team-tickets/*']\n---\nReview.\n")
	write(t, plugin, "mcp.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"team-tickets":{"type":"streamable-http","url":"https://tickets.example/mcp"}}}`)
	result, err := Import(plugin, Options{Name: "acme/team", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if notes := strings.Join(result.Notes, "\n"); strings.Contains(notes, "tools list does not name") {
		t.Fatalf("an agent whose tools name every imported server needs no note:\n%s", notes)
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	built := t.TempDir()
	if _, err := compile.Build(out, built, compile.BuildOptions{}); err != nil {
		t.Fatalf("imported sources must build: %v", err)
	}
	root := filepath.Join(built, "agent-plugin", "team-kit")
	skill, err := os.ReadFile(filepath.Join(root, "skills", "code-review", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), `argument-hint: "[pr]"`) || !strings.Contains(string(skill), "user-invocable: false") {
		t.Fatalf("Copilot skill fields survive the round trip:\n%s", skill)
	}
	for _, rel := range []string{"skills/code-review/references/checklist.md", "skills/code-review/scripts/diff.sh", "mcp.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s must survive the round trip: %v", rel, err)
		}
	}
	agent, err := os.ReadFile(filepath.Join(root, "com.github.copilot", "agents", "reviewer.agent.md"))
	if err != nil || !strings.Contains(string(agent), "tools:\n  - \"read\"\n  - \"search\"\n  - \"team-tickets/*\"") {
		t.Fatalf("Copilot agent fields survive the round trip: %v\n%s", err, agent)
	}
}

// Every skill a plugin ships sits beside every agent it ships, and import
// assumes every skill requires every server, so an agent whose tools list
// omits a server cannot validate as imported; import says so.
func TestImportNotesAgentToolsThatMissAServer(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit."}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	write(t, plugin, "com.github.copilot/agents/reviewer.agent.md", "---\ndescription: Reviews pull requests.\ntools: ['read']\n---\nReview.\n")
	write(t, plugin, "mcp.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"team-tickets":{"type":"streamable-http","url":"https://tickets.example/mcp"}}}`)
	result, err := Import(plugin, Options{Name: "acme/team", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if notes := strings.Join(result.Notes, "\n"); !strings.Contains(notes, "com.github.copilot/agents/reviewer.agent.md: its tools list does not name team-tickets") {
		t.Fatalf("import names the agent and the server its tools miss:\n%s", notes)
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	if _, err := compile.Build(out, t.TempDir(), compile.BuildOptions{}); err == nil || !strings.Contains(err.Error(), "team-tickets") {
		t.Fatalf("the imported package fails closed on the missing grant, got %v", err)
	}
}

func TestImportCarriesCopilotComponents(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit."}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	write(t, plugin, "com.github.copilot/commands/changelog.md", "---\ndescription: Draft a changelog entry.\nargument-hint: \"[range]\"\ndisable-model-invocation: true\n---\nDraft the changelog.\n")
	write(t, plugin, "com.github.copilot/rules/api.md", "---\napplyTo: \"src/api/**\"\ndescription: API conventions\n---\nUse ApiError.\n")
	write(t, plugin, "com.github.copilot/hooks/hooks.json", `{"version":1,"hooks":{"preToolUse":[{"type":"command","bash":"./guard.sh","matcher":"bash","timeoutSec":10}]}}`)
	write(t, plugin, "com.github.copilot/lsp.json", `{"lspServers":{"team-lsp":{"command":"team-lsp","fileExtensions":{".team":"team"}}}}`)
	result, err := Import(plugin, Options{Name: "acme/team", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	built := t.TempDir()
	if _, err := compile.Build(out, built, compile.BuildOptions{}); err != nil {
		t.Fatalf("imported sources must build: %v", err)
	}
	root := filepath.Join(built, "agent-plugin", "team-kit", "com.github.copilot")
	for _, rel := range []string{"commands/changelog.md", "rules/api.md", "hooks/hooks.json", "lsp.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s must survive the round trip: %v", rel, err)
		}
	}
	rule, _ := os.ReadFile(filepath.Join(root, "rules", "api.md"))
	if !strings.Contains(string(rule), `paths: "src/api/**"`) {
		t.Fatalf("a rule's applyTo scope becomes paths:\n%s", rule)
	}
	lsp, _ := os.ReadFile(filepath.Join(root, "lsp.json"))
	if !strings.Contains(string(lsp), `".team": "team"`) {
		t.Fatalf("an LSP server's extensions survive the round trip:\n%s", lsp)
	}
}

func TestImportKeepsAnAgentWithNoTools(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit."}`)
	write(t, plugin, "com.github.copilot/agents/quiet.agent.md", "---\ndescription: Answers with no tools.\ntools: []\n---\nAnswer only from the conversation.\n")
	result, err := Import(plugin, Options{Name: "acme/team", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	built := t.TempDir()
	if _, err := compile.Build(out, built, compile.BuildOptions{}); err != nil {
		t.Fatalf("imported sources must build: %v", err)
	}
	agent, err := os.ReadFile(filepath.Join(built, "agent-plugin", "team-kit", "com.github.copilot", "agents", "quiet.agent.md"))
	if err != nil || !strings.Contains(string(agent), "\ntools: []\n") {
		t.Fatalf("an agent with no tools must not gain Copilot's default tools on import: %v\n%s", err, agent)
	}
}

func TestImportRejectsGenericServerNames(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit."}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	write(t, plugin, "mcp.json", `{"mcpServers":{"github":{"type":"stdio","command":"github-mcp"}}}`)
	if _, err := Import(plugin, Options{}); err == nil || !strings.Contains(err.Error(), "two hyphen-separated") {
		t.Fatalf("a generic server name must fail rather than be rewritten, got %v", err)
	}
}

func TestImportAgentPluginFormat(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit."}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	write(t, plugin, "com.github.copilot/agents/reviewer.agent.md", "---\ndescription: Reviews pull requests.\n---\nReview.\n")
	result, err := Import(plugin, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var pluginDoc string
	for _, file := range result.Files {
		if file.Path == "plugins/team-kit.plugin.tfer" {
			pluginDoc = file.Content
		}
	}
	if !strings.Contains(pluginDoc, "description: Our team kit.") || !strings.Contains(pluginDoc, "agents/reviewer.agent.tfer") || !strings.Contains(pluginDoc, "skills/code-review.skill.tfer") {
		t.Fatalf("the imported plugin links its agents and skills:\n%s", pluginDoc)
	}
}

func TestImportRejectsInvalidNamesRatherThanRewriting(t *testing.T) {
	repo := t.TempDir()
	write(t, repo, ".github/skills/Code_Review/SKILL.md", "---\nname: Code_Review\ndescription: Review.\n---\nReview.\n")
	if _, err := Import(repo, Options{Plugin: "team"}); err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("an invalid skill name must fail, got %v", err)
	}
}

func TestImportCarriesPluginMetadataAndFiles(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","version":"2.3.0","description":"Our team kit.","author":{"name":"Team Platform","email":"platform@example.com"},"repository":"https://example.com/git/team-kit","license":"MIT","keywords":["review","incidents"]}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	write(t, plugin, "README.md", "# Team kit\n")
	write(t, plugin, "templates/settings.json", "{\"enabledPlugins\":{}}\n")
	write(t, plugin, ".typeference/bundle.json", "{}\n")
	result, err := Import(plugin, Options{Name: "acme/team"})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	built := t.TempDir()
	if _, err := compile.Build(out, built, compile.BuildOptions{}); err != nil {
		t.Fatalf("imported sources must build: %v", err)
	}
	root := filepath.Join(built, "agent-plugin", "team-kit")
	manifest, err := os.ReadFile(filepath.Join(root, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": "2.3.0"`, `"name": "Team Platform"`, `"email": "platform@example.com"`, `"repository": "https://example.com/git/team-kit"`, `"license": "MIT"`, "\"keywords\": [\n    \"review\",\n    \"incidents\"\n  ]"} {
		if !strings.Contains(string(manifest), want) {
			t.Fatalf("plugin.json must carry %s:\n%s", want, manifest)
		}
	}
	for _, rel := range []string{"README.md", "templates/settings.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("carried file %s must survive the round trip: %v", rel, err)
		}
	}
	if !strings.Contains(strings.Join(result.Notes, "\n"), ".typeference/") {
		t.Fatalf("skipped build provenance is noted: %v", result.Notes)
	}
}

func TestImportCarriesCatalogFields(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"name":"team-kit","description":"Our team kit.","category":"developer-tools","tags":["review","incidents"]}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	result, err := Import(plugin, Options{Name: "acme/team"})
	if err != nil {
		t.Fatal(err)
	}
	var doc string
	for _, file := range result.Files {
		if file.Path == "plugins/team-kit.plugin.tfer" {
			doc = file.Content
		}
	}
	for _, want := range []string{"category:", "developer-tools", "tags:", "review", "incidents"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("the plugin document must carry %s:\n%s", want, doc)
		}
	}
	out := t.TempDir()
	if err := Write(result.Files, out); err != nil {
		t.Fatal(err)
	}
	built := t.TempDir()
	if _, err := compile.Build(out, built, compile.BuildOptions{}); err != nil {
		t.Fatalf("imported sources must build: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(built, "agent-plugin", "team-kit", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "category") || strings.Contains(string(manifest), "tags") {
		t.Fatalf("catalog fields belong only to the marketplace entry:\n%s", manifest)
	}

	write(t, plugin, "plugin.json", `{"name":"team-kit","description":"Our team kit.","tags":["review","review"]}`)
	if _, err := Import(plugin, Options{}); err == nil || !strings.Contains(err.Error(), "plugin metadata 'tags'") {
		t.Fatalf("a repeated tag must fail the import, got %v", err)
	}
}

func TestImportFailsClosedOnPluginManifestAndStrayComponentFiles(t *testing.T) {
	plugin := t.TempDir()
	write(t, plugin, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"team-kit","description":"Our team kit.","extensions":{"com.example":{}},"keywords":[]}`)
	write(t, plugin, "skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff.\n---\n"+skillBody)
	write(t, plugin, "skills/notes.md", "not a skill\n")
	write(t, plugin, "com.github.copilot/settings.json", "{}\n")
	_, err := Import(plugin, Options{})
	if err == nil {
		t.Fatal("unrepresentable plugin content must fail the import")
	}
	for _, want := range []string{"manifest member 'extensions'", "plugin metadata 'keywords'", "skills/notes.md", "com.github.copilot/settings.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the failure must list %s:\n%v", want, err)
		}
	}
	result, err := Import(plugin, Options{Lossy: true})
	if err != nil {
		t.Fatalf("--lossy imports without the listed content: %v", err)
	}
	if notes := strings.Join(result.Notes, "\n"); !strings.Contains(notes, "dropped: plugin.json: manifest member 'extensions'") {
		t.Fatalf("a lossy import lists what it dropped:\n%s", notes)
	}
}

func TestImportRejectsAMarketplaceRepository(t *testing.T) {
	repo := t.TempDir()
	write(t, repo, ".github/plugin/marketplace.json", `{"name":"team-agents","owner":{"name":"Team"},"plugins":[{"name":"team-kit","source":"./plugins/team-kit"}]}`)
	write(t, repo, "README.md", "# Team agents\n")
	_, err := Import(repo, Options{})
	if err == nil || !strings.Contains(err.Error(), "marketplace repository") || !strings.Contains(err.Error(), "team-kit: plugins/team-kit") {
		t.Fatalf("a marketplace must fail and list its plugin directories, got %v", err)
	}
}

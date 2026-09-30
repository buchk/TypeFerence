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
	if _, err := compile.Build(out, built, []compile.Target{compile.AgentPlugin}, nil); err != nil {
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
	write(t, repo, ".github/agents/reviewer.agent.md", "---\ndescription: Reviews pull requests.\ntools: ['read']\n---\nReview.\n")
	write(t, repo, ".github/skills/code-review/scripts/run.sh", "echo hi\n")
	_, err := Import(repo, Options{Plugin: "team"})
	if err == nil || !strings.Contains(err.Error(), "frontmatter field 'tools'") || !strings.Contains(err.Error(), "skill resource file") {
		t.Fatalf("unrepresentable content must fail and be listed, got %v", err)
	}
	result, err := Import(repo, Options{Plugin: "team", Lossy: true})
	if err != nil {
		t.Fatalf("--lossy imports without the listed content: %v", err)
	}
	dropped := strings.Join(result.Notes, "\n")
	if !strings.Contains(dropped, "dropped: .github/agents/reviewer.agent.md: frontmatter field 'tools'") {
		t.Fatalf("a lossy import lists what it dropped:\n%s", dropped)
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

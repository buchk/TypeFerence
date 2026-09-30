package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// writePackage writes a version 6 package: a manifest listing the given
// plugins plus every file in files (paths are package-relative).
func writePackage(t *testing.T, plugins []string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	manifest := "---\nschemaVersion: 6\nname: acme/test\nversion: 1.0.0\n"
	if len(plugins) > 0 {
		manifest += "plugins:\n"
		for _, plugin := range plugins {
			manifest += "  - " + plugin + "\n"
		}
	}
	manifest += "---\n"
	if _, overridden := files["typeference.tfer"]; !overridden {
		writeSrc(t, root, "typeference.tfer", manifest)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeSrc(t, root, name, files[name])
	}
	return root
}

func writeSrc(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readOut(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const toolSkillPackage = "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n"

func TestBuildIsUnlinked(t *testing.T) {
	source := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": toolSkillPackage,
		"agents/a.agent.tfer":     "---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n---\n",
		"skills/s.skill.tfer":     "---\ndescription: S.\nrequiresTools:\n  - tools/runtime.tool.tfer\n---\nDo it.\n",
		"tools/runtime.tool.tfer": "---\ndescription: A runtime import.\n---\n",
	})
	output := t.TempDir()
	if _, err := Build(source, output, []Target{Neutral, AgentPlugin}, nil); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(output, "agent-plugin", "kit")
	if _, err := os.Stat(filepath.Join(plugin, "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("build must not emit a plugin mcp.json; link does")
	}
	link := readOut(t, plugin, ".typeference", "link.json")
	if !strings.Contains(link, `"toolId": "acme/test/tools/runtime@1.0.0"`) || !strings.Contains(link, `"mode": "manual"`) {
		t.Fatalf("build must emit typed link requirements for the plugin artifact:\n%s", link)
	}
	if _, err := os.Stat(filepath.Join(output, "neutral", "a", ".typeference", "link.json")); err != nil {
		t.Fatal("build must emit neutral link requirements")
	}
}

func TestProjectManifestRejectsUnknownFields(t *testing.T) {
	source := writePackage(t, nil, map[string]string{
		"typeference.tfer":        "---\nschemaVersion: 6\nname: acme/test\nversion: 1.0.0\nplugins:\n  - plugins/kit.plugin.tfer\ndeployment:\n  mcpCommand: nope\n---\n",
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/s.skill.tfer\n---\n",
		"skills/s.skill.tfer":     "---\ndescription: S.\n---\nS.\n",
	})
	_, err := Build(source, t.TempDir(), []Target{AgentPlugin}, nil)
	if err == nil || !strings.Contains(err.Error(), "deployment") {
		t.Fatalf("deployment metadata in the source manifest must fail, got %v", err)
	}
}

func TestARDDoesNotInventCallableCards(t *testing.T) {
	source := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":        "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"capabilities/c.capability.tfer": "---\nvisibility: exposed\n---\n",
		"skills/s.skill.tfer":            "---\ndescription: S.\nbinds: capabilities/c.capability.tfer\n---\nDo it.\n",
		"agents/a.agent.tfer":            "---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n---\n",
	})
	output := t.TempDir()
	if _, err := Build(source, output, []Target{Neutral, AgentPlugin}, &ArdPublicationOptions{PublisherDomain: "acme.example"}); err != nil {
		t.Fatal(err)
	}
	catalog := readOut(t, output, "ard", "ai-catalog.json")
	if strings.Contains(catalog, "a2a-agent-card") || strings.Contains(catalog, "mcp-server") {
		t.Fatal("unlinked build must not publish callable endpoint/provider claims")
	}
	if !strings.Contains(catalog, "urn:air:acme.example:typeference:agent-plugin:kit") {
		t.Fatalf("the catalog must list each plugin artifact:\n%s", catalog)
	}
}

func TestRetiredTargetsAreNamed(t *testing.T) {
	for _, retired := range []string{"codex", "copilot", "cursor"} {
		if _, err := ParseTargets(retired); err == nil || !strings.Contains(err.Error(), "retired") {
			t.Errorf("%s must be reported as retired, got %v", retired, err)
		}
	}
}

func TestLegacySourcesRequireTheArchivalLanguage(t *testing.T) {
	source := t.TempDir()
	writeSrc(t, source, "agent.tfer", "---\nschemaVersion: 5\nkind: agent\nid: acme/agents/a@1.0.0\ndescription: A.\n---\n")
	if _, err := Build(source, t.TempDir(), []Target{Neutral}, nil); err == nil || !strings.Contains(err.Error(), "version 6") {
		t.Fatalf("a version 5 tree must not build as the current language, got %v", err)
	}
	out := t.TempDir()
	if _, err := BuildWithOptions(source, out, []Target{Neutral}, nil, BuildOptions{Language: LanguageLegacyV5}); err != nil {
		t.Fatalf("the archival language must still reproduce version 5 output: %v", err)
	}
	if !strings.Contains(readOut(t, out, "neutral", "a", "AGENTS.md"), "A.") {
		t.Fatal("archival neutral output keeps the version 5 rendering, description included")
	}
	if _, err := BuildWithOptions(source, t.TempDir(), []Target{AgentPlugin}, nil, BuildOptions{Language: LanguageLegacyV5}); err == nil {
		t.Fatal("agent-plugin requires a version 6 package")
	}
}

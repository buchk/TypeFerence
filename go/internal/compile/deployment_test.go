package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildWithManifest compiles a minimal exposed agent, optionally with a project
// manifest declaring a deployment endpoint, and returns the ard output dir.
func buildWithManifest(t *testing.T, manifest string) (ardDir, codexDir string) {
	t.Helper()
	src := t.TempDir()
	writeSrc(t, src, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\nvisibility: exposed\n")
	writeSrc(t, src, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/c@1.0.0\ninstructions: do it\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	if manifest != "" {
		writeSrc(t, src, "typeference.yaml", manifest)
	}
	out := t.TempDir()
	targets, err := ParseTargets("codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(src, out, targets, &ArdPublicationOptions{PublisherDomain: "acme.example"}); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(out, "ard"), filepath.Join(out, "codex", "agent")
}

func TestNoA2ACardWithoutDeclaredEndpoint(t *testing.T) {
	// The compiler must not invent a service endpoint: an A2A card is a live
	// routing claim that a registry indexes and a picker follows.
	ard, _ := buildWithManifest(t, "")
	if _, err := os.Stat(filepath.Join(ard, "agent.agent-card.json")); !os.IsNotExist(err) {
		t.Error("an A2A card must not be emitted without a declared deployment endpoint")
	}
	catalog, err := os.ReadFile(filepath.Join(ard, "ai-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(catalog), "a2a-agent-card") {
		t.Error("the catalog must not carry an A2A entry without a declared endpoint")
	}
	// The MCP manifest asserts no endpoint, so it is still emitted.
	if _, err := os.Stat(filepath.Join(ard, "agent.mcp.json")); err != nil {
		t.Error("the MCP tool manifest claims no endpoint and should still be emitted")
	}
}

func TestDeclaredEndpointIsUsedVerbatim(t *testing.T) {
	ard, _ := buildWithManifest(t,
		"schemaVersion: 1\nname: acme\nversion: 1.0.0\ndeployment:\n  a2aBaseUrl: https://runtime.acme.example/agents/\n")
	raw, err := os.ReadFile(filepath.Join(ard, "agent.agent-card.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The trailing slash is normalized; the authored host is otherwise untouched
	// rather than derived from the publisher domain.
	if !strings.Contains(string(raw), `"url": "https://runtime.acme.example/agents/agent"`) {
		t.Errorf("the card should route to the authored endpoint:\n%s", raw)
	}
}

func TestCodexConfigEmitsSubstitutionTokenWhenUnbound(t *testing.T) {
	_, codex := buildWithManifest(t, "")
	raw, err := os.ReadFile(filepath.Join(codex, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(raw)
	if !strings.Contains(config, MCPCommandToken) {
		t.Errorf("an unbound MCP command should emit a substitution token:\n%s", config)
	}
	// The retired runtime must not be invoked: there is no `serve` verb.
	if strings.Contains(config, `"serve"`) {
		t.Errorf("the config must not invoke the retired runtime:\n%s", config)
	}
}

func TestCodexConfigBakesDeclaredCommand(t *testing.T) {
	_, codex := buildWithManifest(t,
		"schemaVersion: 1\nname: acme\nversion: 1.0.0\ndeployment:\n  mcpCommand: acme-mcp\n")
	raw, err := os.ReadFile(filepath.Join(codex, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(raw)
	if !strings.Contains(config, `command = "acme-mcp"`) {
		t.Errorf("a declared command should be baked in:\n%s", config)
	}
	if strings.Contains(config, MCPCommandToken) {
		t.Errorf("a declared command leaves no substitution token:\n%s", config)
	}
}

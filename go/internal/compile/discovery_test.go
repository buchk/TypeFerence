package compile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func buildDiscovery(t *testing.T, exposed bool) string {
	t.Helper()
	src := t.TempDir()
	vis := ""
	if exposed {
		vis = "visibility: exposed\n"
	}
	writeSrc(t, src, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/status@1.0.0\ndisplayName: Status\ninputSchema: '{\"type\":\"object\",\"properties\":{\"focus\":{\"type\":\"string\"}},\"additionalProperties\":false}'\n"+vis)
	writeSrc(t, src, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/status@1.0.0\ninstructions: do it\ninputSchema: '{\"type\":\"object\",\"properties\":{\"focus\":{\"type\":\"string\"}},\"additionalProperties\":false}'\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\ndisplayName: Acme Agent\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	out := t.TempDir()
	targets, _ := ParseTargets("neutral")
	if _, err := Build(src, out, targets, &ArdPublicationOptions{PublisherDomain: "acme.example"}); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(out, "ard")
}

func TestA2ACardAndMCPManifestEmitted(t *testing.T) {
	ard := buildDiscovery(t, true)

	var card map[string]any
	raw, err := os.ReadFile(filepath.Join(ard, "agent.agent-card.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &card); err != nil {
		t.Fatalf("agent card is not valid JSON: %v", err)
	}
	for _, k := range []string{"protocolVersion", "name", "url", "skills"} {
		if _, ok := card[k]; !ok {
			t.Errorf("A2A agent card missing required field %q", k)
		}
	}
	if len(card["skills"].([]any)) != 1 {
		t.Errorf("expected one A2A skill for the exposed capability")
	}

	var mcp struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	raw, err = os.ReadFile(filepath.Join(ard, "agent.mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &mcp); err != nil {
		t.Fatalf("mcp manifest is not valid JSON: %v", err)
	}
	if len(mcp.Tools) != 1 || mcp.Tools[0].Name != "agent.status" {
		t.Fatalf("expected one MCP tool named agent.status, got %+v", mcp.Tools)
	}
	// inputSchema must be a JSON Schema object, not a string.
	if mcp.Tools[0].InputSchema["type"] != "object" {
		t.Errorf("MCP tool inputSchema must be a JSON Schema object")
	}
}

func TestCatalogHasOfficialTypedEntries(t *testing.T) {
	ard := buildDiscovery(t, true)
	var catalog struct {
		Entries []struct {
			Type string `json:"type"`
		} `json:"entries"`
	}
	raw, err := os.ReadFile(filepath.Join(ard, "ai-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, e := range catalog.Entries {
		types[e.Type] = true
	}
	for _, want := range []string{"application/a2a-agent-card+json", "application/mcp-server+json"} {
		if !types[want] {
			t.Errorf("ai-catalog.json is missing an official %q entry a registry would index", want)
		}
	}
}

func TestNoDiscoveryCardsWithoutExposure(t *testing.T) {
	ard := buildDiscovery(t, false)
	if _, err := os.Stat(filepath.Join(ard, "agent.agent-card.json")); err == nil {
		t.Error("an agent exposing nothing must not emit an A2A card")
	}
	if _, err := os.Stat(filepath.Join(ard, "agent.mcp.json")); err == nil {
		t.Error("an agent exposing nothing must not emit an MCP manifest")
	}
}

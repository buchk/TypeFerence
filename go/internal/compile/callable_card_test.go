package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSrc(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildCatalog(t *testing.T, exposed bool) string {
	t.Helper()
	src := t.TempDir()
	vis := ""
	if exposed {
		vis = "visibility: exposed\n"
	}
	writeSrc(t, src, "cap.yaml", "schemaVersion: 3\nkind: capability\nid: acme/cap/c@1.0.0\n"+vis)
	writeSrc(t, src, "skill.yaml", "schemaVersion: 3\nkind: skill\nid: acme/skills/s@1.0.0\nbinds: acme/cap/c@1.0.0\ninstructions: do it\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\nskills:\n  - ref: acme/skills/s@1.0.0\n")
	out := t.TempDir()
	targets, err := ParseTargets("neutral")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(src, out, targets, &ArdPublicationOptions{PublisherDomain: "acme.example"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "ard", "ai-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCallableCardEmittedForExposedCapability(t *testing.T) {
	catalog := buildCatalog(t, true)
	if !strings.Contains(catalog, "callable-card") {
		t.Error("expected a callable-resource card for an exposed capability")
	}
	if !strings.Contains(catalog, "typeference:callable:agent") {
		t.Error("expected the callable card identifier")
	}
	if !strings.Contains(catalog, "instructionsTemplate") {
		t.Error("expected the card to carry the instruction-package template")
	}
}

func TestNoCallableCardWithoutExposure(t *testing.T) {
	catalog := buildCatalog(t, false)
	if strings.Contains(catalog, "callable-card") {
		t.Error("an internal-only capability must not emit a callable card")
	}
}

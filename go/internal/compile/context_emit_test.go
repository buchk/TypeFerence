package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleEmitsHeldContext(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "ct.yaml", "schemaVersion: 3\nkind: contextType\nid: acme/ct/cast@1.0.0\n")
	writeSrc(t, src, "note.yaml", "schemaVersion: 3\nkind: context\nid: acme/notes/n@1.0.0\ncontextType: acme/ct/cast@1.0.0\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\ncontext:\n  - acme/notes/n@1.0.0\n")
	out := t.TempDir()
	targets, _ := ParseTargets("neutral")
	if _, err := Build(src, out, targets, nil); err != nil {
		t.Fatal(err)
	}
	bundle, err := os.ReadFile(filepath.Join(out, "neutral", "agent", "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), `"context"`) || !strings.Contains(string(bundle), "acme/notes/n@1.0.0") {
		t.Errorf("bundle should list held context objects:\n%s", bundle)
	}
	if !strings.Contains(string(bundle), "acme/ct/cast@1.0.0") {
		t.Errorf("held context should carry its contextType")
	}
}

func TestBundleOmitsContextWhenNoneHeld(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "agent.yaml", "schemaVersion: 3\nkind: agent\nid: acme/agent@1.0.0\n")
	out := t.TempDir()
	targets, _ := ParseTargets("neutral")
	if _, err := Build(src, out, targets, nil); err != nil {
		t.Fatal(err)
	}
	bundle, _ := os.ReadFile(filepath.Join(out, "neutral", "agent", "bundle.json"))
	if strings.Contains(string(bundle), `"context"`) {
		t.Errorf("an agent holding no context must not emit a context member:\n%s", bundle)
	}
}

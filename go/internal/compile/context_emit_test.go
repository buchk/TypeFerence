package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleEmitsHeldContext(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "ct.yaml", "schemaVersion: 4\nkind: contextType\nid: acme/ct/cast@1.0.0\nfields:\n  owner:\n    type: string\n    required: true\n  governed:\n    type: boolean\n    default: false\n")
	writeSrc(t, src, "note.yaml", "schemaVersion: 4\nkind: context\nid: acme/notes/n@1.0.0\ncontextType: acme/ct/cast@1.0.0\nvalues:\n  owner: Dana\n")
	writeSrc(t, src, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agent@1.0.0\ncontext:\n  - acme/notes/n@1.0.0\n")
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
	if !strings.Contains(string(bundle), `"owner": "Dana"`) || !strings.Contains(string(bundle), `"governed": false`) {
		t.Errorf("bundle should preserve values and materialized defaults:\n%s", bundle)
	}
}

func TestBundleOmitsContextWhenNoneHeld(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "agent.yaml", "schemaVersion: 4\nkind: agent\nid: acme/agent@1.0.0\n")
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

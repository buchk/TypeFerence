package resource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeV6(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const v6Manifest = "---\nschemaVersion: 6\nname: acme/test\nversion: 1.2.3\nplugins:\n  - plugins/kit.plugin.tfer\n---\n"

func TestKindAndIdentityComeFromThePath(t *testing.T) {
	kind, stem, ok := KindFromPath("skills/core/review.skill.tfer")
	if !ok || kind != "skill" || stem != "skills/core/review" {
		t.Fatalf("KindFromPath = %q %q %v", kind, stem, ok)
	}
	if kind, _, _ := KindFromPath("types/team.contexttype.tfer"); kind != "contextType" {
		t.Fatalf("contexttype suffix must win over context, got %q", kind)
	}
	if got := DeriveID("acme/test", "1.2.3", "skills/core/review.skill.tfer"); got != "acme/test/skills/core/review@1.2.3" {
		t.Fatalf("DeriveID = %q", got)
	}
	for _, bad := range []string{"/abs.skill.tfer", "../up.skill.tfer", "skills/Upper.skill.tfer", "skills\\win.skill.tfer", "notes.md", "a//b.skill.tfer"} {
		if err := ValidSourcePath(bad); err == nil {
			t.Errorf("%q must be rejected as a source path", bad)
		}
	}
}

func TestScalarsAreTypedByTheirField(t *testing.T) {
	doc, err := ParseV6Document("acme/test", "1.0.0", "skills/s.skill.tfer",
		"---\ndescription: no\nsealed: true\n---\nBody.\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Description != "no" || !doc.Sealed {
		t.Fatalf("a string field keeps plain text verbatim and a boolean field reads true: %+v", doc)
	}
	if _, err := ParseV6Document("acme/test", "1.0.0", "skills/s.skill.tfer",
		"---\ndescription: S.\nsealed: \"true\"\n---\nBody.\n", nil); err == nil || !strings.Contains(err.Error(), "unquoted boolean") {
		t.Fatalf("a quoted boolean is a string, got %v", err)
	}
	if _, err := ParseV6Document("acme/test", "1.0.0", "skills/s.skill.tfer",
		"---\ndescription: S.\nbogus: x\n---\nBody.\n", nil); err == nil || !strings.Contains(err.Error(), "skills/s.skill.tfer:3: unknown field 'bogus'") {
		t.Fatalf("an unknown field names its file line, got %v", err)
	}
}

func TestReferencesAreKindCheckedPaths(t *testing.T) {
	if _, err := ParseV6Document("acme/test", "1.0.0", "skills/s.skill.tfer",
		"---\ndescription: S.\nextends: context/c.context.tfer\n---\nBody.\n", nil); err == nil || !strings.Contains(err.Error(), ".skill.tfer") {
		t.Fatalf("extends must name a skill document, got %v", err)
	}
	doc, err := ParseV6Document("acme/test", "1.0.0", "agents/a.agent.tfer",
		"---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n  - skill: skills/t.skill.tfer\n    sealed: true\n---\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Skills) != 2 || doc.Skills[0].Ref != "acme/test/skills/s@1.0.0" || !doc.Skills[1].Sealed {
		t.Fatalf("bindings accept a path or a mapping: %+v", doc.Skills)
	}
}

func TestLoadIsTheManifestClosure(t *testing.T) {
	root := t.TempDir()
	writeV6(t, root, map[string]string{
		"typeference.tfer":           v6Manifest,
		"plugins/kit.plugin.tfer":    "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"agents/a.agent.tfer":        "---\ndescription: A.\ncontext:\n  - context/norm.context.tfer\n---\nObjectives.\n",
		"context/norm.context.tfer":  "---\ndisplayName: Norm\n---\nA norm.\n",
		"drafts/broken.skill.tfer":   "---\nthis is: [not valid\n---\n",
		"drafts/unused.profile.tfer": "---\n---\n",
	})
	loaded, err := LoadV6(root, V6Options{})
	if err != nil {
		t.Fatalf("unreferenced documents must not be loaded: %v", err)
	}
	want := "agents/a.agent.tfer,context/norm.context.tfer,plugins/kit.plugin.tfer"
	if strings.Join(loaded.Files, ",") != want {
		t.Fatalf("closure = %v, want %s", loaded.Files, want)
	}
	agent := loaded.Documents["acme/test/agents/a@1.2.3"]
	if agent == nil || strings.TrimSpace(agent.Objectives) != "Objectives." {
		t.Fatalf("an agent's body is its objectives: %+v", agent)
	}
}

func TestMissingReferencedDocumentFails(t *testing.T) {
	root := t.TempDir()
	writeV6(t, root, map[string]string{
		"typeference.tfer":        v6Manifest,
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/missing.skill.tfer\n---\n",
	})
	if _, err := LoadV6(root, V6Options{}); err == nil || !strings.Contains(err.Error(), "skills/missing.skill.tfer") {
		t.Fatalf("a missing reference must fail and name the path, got %v", err)
	}
}

func loadNormalized(t *testing.T, files map[string]string) map[string]*Document {
	t.Helper()
	root := t.TempDir()
	files["typeference.tfer"] = v6Manifest
	writeV6(t, root, files)
	loaded, err := LoadV6(root, V6Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := NormalizeV6(loaded.Documents); err != nil {
		t.Fatal(err)
	}
	return loaded.Documents
}

func TestNormalizationDerivesWhatAuthorsDoNotWrite(t *testing.T) {
	docs := loadNormalized(t, map[string]string{
		"plugins/kit.plugin.tfer":        "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"capabilities/c.capability.tfer": "---\ninputSchema: '{\"type\":\"object\",\"properties\":{\"x\":{\"type\":\"string\"}}}'\n---\n",
		"skills/bound.skill.tfer":        "---\ndescription: Bound.\nbinds: capabilities/c.capability.tfer\n---\nBound.\n",
		"skills/root.skill.tfer":         "---\ndescription: Root.\n---\nRoot.\n",
		"skills/ext.skill.tfer":          "---\ndescription: Ext.\nextends: skills/root.skill.tfer\n---\nMore.\n",
		"context/norm.context.tfer":      "---\n---\nA norm.\n",
		"agents/a.agent.tfer":            "---\ndescription: A.\ncontext:\n  - context/norm.context.tfer\nskills:\n  - skills/bound.skill.tfer\n  - skill: skills/ext.skill.tfer\n    capability: skills/root.skill.tfer\n---\n",
	})
	root := docs["acme/test/skills/root@1.2.3"]
	if !root.ImpliedCapability || root.Binds != root.ID {
		t.Fatalf("a root skill without binds defines its own capability: %+v", root)
	}
	ext := docs["acme/test/skills/ext@1.2.3"]
	if ext.Binds != root.ID || ext.Instructions != "Root.\n\nMore.\n" {
		t.Fatalf("an extension inherits its base's capability and appends its instructions: %q %q", ext.Binds, ext.Instructions)
	}
	bound := docs["acme/test/skills/bound@1.2.3"]
	if !strings.Contains(bound.InputSchema, `"x"`) {
		t.Fatalf("a skill that binds a capability inherits its undeclared schemas: %s", bound.InputSchema)
	}
	agent := docs["acme/test/agents/a@1.2.3"]
	if *agent.Skills[1].Capability != root.ID {
		t.Fatalf("a capability reference naming a skill resolves to that skill's capability: %s", *agent.Skills[1].Capability)
	}
	if agent.DisplayName != "a" {
		t.Fatalf("an agent's display name defaults to its identity leaf: %q", agent.DisplayName)
	}
	if norm := docs["acme/test/context/norm@1.2.3"]; norm.ContextType != BuiltinTextContextType {
		t.Fatalf("a context without a contextType is built-in text: %q", norm.ContextType)
	}
}

func TestExtensionRulesFailClosed(t *testing.T) {
	cases := map[string]map[string]string{
		"sealed": {
			"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/ext.skill.tfer\n---\n",
			"skills/base.skill.tfer":  "---\ndescription: Base.\nsealed: true\n---\nBase.\n",
			"skills/ext.skill.tfer":   "---\ndescription: Ext.\nextends: skills/base.skill.tfer\n---\nExt.\n",
		},
		"cycle": {
			"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/a.skill.tfer\n---\n",
			"skills/a.skill.tfer":     "---\ndescription: A.\nextends: skills/b.skill.tfer\n---\nA.\n",
			"skills/b.skill.tfer":     "---\ndescription: B.\nextends: skills/a.skill.tfer\n---\nB.\n",
		},
		"contract": {
			"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/ext.skill.tfer\n---\n",
			"skills/base.skill.tfer":  "---\ndescription: Base.\n---\nBase.\n",
			"skills/ext.skill.tfer":   "---\ndescription: Ext.\nextends: skills/base.skill.tfer\ninputSchema: '{\"type\":\"string\"}'\n---\nExt.\n",
		},
		"new-mode": {
			"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/ext.skill.tfer\n---\n",
			"skills/base.skill.tfer":  "---\ndescription: Base.\nvariants:\n  manual:\n    instructions: Manual.\n---\n",
			"skills/ext.skill.tfer":   "---\ndescription: Ext.\nextends: skills/base.skill.tfer\nvariants:\n  pipeline:\n    instructions: New.\n---\n",
		},
	}
	for name, files := range cases {
		root := t.TempDir()
		files["typeference.tfer"] = v6Manifest
		writeV6(t, root, files)
		loaded, err := LoadV6(root, V6Options{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := NormalizeV6(loaded.Documents); err == nil {
			t.Errorf("%s: normalization must fail", name)
		}
	}
}

func TestManifestIsClosedAndV6Only(t *testing.T) {
	for name, text := range map[string]string{
		"unknown field": "---\nschemaVersion: 6\nname: acme/test\nversion: 1.0.0\nplugins:\n  - plugins/kit.plugin.tfer\nfeeds: x\n---\n",
		"reserved name": "---\nschemaVersion: 6\nname: typeference/builtin\nversion: 1.0.0\nplugins:\n  - plugins/kit.plugin.tfer\n---\n",
		"empty":         "---\nschemaVersion: 6\nname: acme/test\nversion: 1.0.0\n---\n",
		"not a plugin":  "---\nschemaVersion: 6\nname: acme/test\nversion: 1.0.0\nplugins:\n  - skills/s.skill.tfer\n---\n",
	} {
		root := t.TempDir()
		writeV6(t, root, map[string]string{"typeference.tfer": text})
		if _, err := LoadProject(root); err == nil {
			t.Errorf("%s: manifest must be rejected", name)
		}
	}
}

func TestManifestShipsDeclaredDependencyPlugins(t *testing.T) {
	root := t.TempDir()
	writeV6(t, root, map[string]string{"typeference.tfer": "---\nschemaVersion: 6\nname: acme/marketplace\nversion: 1.0.0\ndependencies:\n  acme/team: 2.0.0\nplugins:\n  - acme/team:plugins/kit.plugin.tfer\n---\n"})
	loaded, err := LoadV6(root, V6Options{})
	if err != nil {
		t.Fatalf("a manifest may ship a declared dependency's plugin: %v", err)
	}
	if len(loaded.OwnPlugins) != 0 || len(loaded.DependencyPlugins) != 1 ||
		loaded.DependencyPlugins[0] != (DependencyPlugin{Package: "acme/team", ID: "acme/team/plugins/kit@2.0.0"}) {
		t.Fatalf("the plugin belongs to its dependency, at the declared version: %+v %+v", loaded.OwnPlugins, loaded.DependencyPlugins)
	}
	for name, text := range map[string]string{
		"undeclared package": "---\nschemaVersion: 6\nname: acme/marketplace\nversion: 1.0.0\nplugins:\n  - acme/team:plugins/kit.plugin.tfer\n---\n",
		"own package prefix": "---\nschemaVersion: 6\nname: acme/marketplace\nversion: 1.0.0\nplugins:\n  - acme/marketplace:plugins/kit.plugin.tfer\n---\n",
		"not a plugin":       "---\nschemaVersion: 6\nname: acme/marketplace\nversion: 1.0.0\ndependencies:\n  acme/team: 2.0.0\nplugins:\n  - acme/team:skills/s.skill.tfer\n---\n",
		"qualified export":   "---\nschemaVersion: 6\nname: acme/marketplace\nversion: 1.0.0\ndependencies:\n  acme/team: 2.0.0\nexports:\n  - acme/team:skills/s.skill.tfer\n---\n",
	} {
		dir := t.TempDir()
		writeV6(t, dir, map[string]string{"typeference.tfer": text})
		if _, err := LoadProject(dir); err == nil {
			t.Errorf("%s: manifest must be rejected", name)
		}
	}
}

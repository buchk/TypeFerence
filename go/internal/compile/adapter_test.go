package compile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

func TestDefaultCopilotAdapterIsByteCompatible(t *testing.T) {
	source := t.TempDir()
	write(t, source, templatePackage())
	defaultOut, explicitOut := t.TempDir(), t.TempDir()
	if _, err := compile.Build(source, defaultOut, compile.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := compile.Build(source, explicitOut, compile.BuildOptions{Adapter: "copilot"}); err != nil {
		t.Fatal(err)
	}
	diff, err := compile.CompareDirs(defaultOut, explicitOut)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Different {
		t.Fatalf("explicit Copilot adapter changes the default output: %+v", diff)
	}
}

func TestPortableAdapterEmitsOnlyCorePluginComponents(t *testing.T) {
	files := templatePackage()
	files["skills/plain.skill.tfer"] = "---\ndescription: Plain skill.\ncopilot:\n  userInvocable: false\n  argumentHint: \"[target]\"\n---\nPortable instructions.\n"
	source := t.TempDir()
	write(t, source, files)
	out := t.TempDir()
	if _, err := compile.Build(source, out, compile.BuildOptions{Adapter: "portable"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(out, compile.TargetName)
	manifest := read(t, root, "kit/plugin.json")
	if !strings.Contains(manifest, "agent-plugins.org/schemas/1.0.0/plugin.schema.json") {
		t.Fatalf("missing Agent Plugins 1.0 manifest: %s", manifest)
	}
	skill := read(t, root, "kit/skills/plain/SKILL.md")
	if !strings.Contains(skill, "Portable instructions.") || strings.Contains(skill, "argument-hint:") || strings.Contains(skill, "user-invocable:") {
		t.Fatalf("portable skill must retain instructions but omit Copilot frontmatter: %s", skill)
	}
	if _, err := os.Stat(filepath.Join(root, "kit", "com.github.copilot")); !os.IsNotExist(err) {
		t.Fatalf("portable build must not ship Copilot extension: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "plugin", "marketplace.json")); !os.IsNotExist(err) {
		t.Fatalf("portable build must not ship a Copilot marketplace: %v", err)
	}
	for _, rel := range []string{"kit/.typeference/bundle.json", ".typeference/build.json", ".typeference/compatibility.json"} {
		read(t, root, rel)
	}
}

func TestPortableAdapterOmitsNativeComponents(t *testing.T) {
	source := filepath.Join("..", "..", "..", "conformance", "fixtures", "014-native-components", "source")
	out := t.TempDir()
	if _, err := compile.Build(source, out, compile.BuildOptions{Adapter: "portable"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(out, compile.TargetName, "kit")
	read(t, root, "plugin.json")
	if _, err := os.Stat(filepath.Join(root, "com.github.copilot")); !os.IsNotExist(err) {
		t.Fatalf("portable output unexpectedly contains Copilot hooks, agents, rules or commands: %v", err)
	}
}

func TestUnknownOutputAdapterFails(t *testing.T) {
	_, err := compile.Build(t.TempDir(), t.TempDir(), compile.BuildOptions{Adapter: "claude"})
	if err == nil || !strings.Contains(err.Error(), "Unknown output adapter") {
		t.Fatalf("an unimplemented client must not silently produce portable output: %v", err)
	}
}

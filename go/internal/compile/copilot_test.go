package compile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

func fixtureSource(name string) string {
	return filepath.Join("..", "..", "..", "conformance", "fixtures", name, "source")
}

// A plugin with no Copilot components builds for every client: Agent Plugins
// 1.0 for Copilot and Codex, Claude Code's own layout for Claude Code, and
// Copilot skill fields only in the Copilot build.
func TestPluginBuildsForEveryClient(t *testing.T) {
	out := t.TempDir()
	if _, err := compile.Build(fixtureSource("023-plugin-for-every-client"), out, compile.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(out, compile.TargetName)
	for _, client := range []string{"copilot", "codex"} {
		root := filepath.Join(target, client, "kit")
		for _, rel := range []string{"plugin.json", "mcp.json", "skills/tickets/SKILL.md", ".typeference/bundle.json"} {
			read(t, root, rel)
		}
		if _, err := os.Stat(filepath.Join(root, "com.github.copilot")); !os.IsNotExist(err) {
			t.Fatalf("%s: a plugin without Copilot components must not emit com.github.copilot: %v", client, err)
		}
	}
	if !strings.Contains(read(t, target, "copilot/kit/skills/signals/SKILL.md"), "argument-hint:") {
		t.Fatal("the Copilot build carries the skill's Copilot fields")
	}
	for _, client := range []string{"claude", "codex"} {
		if strings.Contains(read(t, target, client+"/kit/skills/signals/SKILL.md"), "argument-hint:") {
			t.Fatalf("the %s build must not carry Copilot skill fields", client)
		}
	}
	claude := filepath.Join(target, "claude", "kit")
	read(t, claude, ".claude-plugin/plugin.json")
	if _, err := os.Stat(filepath.Join(claude, "plugin.json")); !os.IsNotExist(err) {
		t.Fatalf("the Claude Code build has no Agent Plugins manifest: %v", err)
	}
	mcp := read(t, claude, ".mcp.json")
	for _, want := range []string{
		`"command": "${CLAUDE_PLUGIN_ROOT}/bin/build-signals"`,
		`"${CLAUDE_PLUGIN_DATA}/cache"`,
		`"SIGNALS_HOME": "${CLAUDE_PLUGIN_ROOT}/share"`,
		`"type": "http"`,
	} {
		if !strings.Contains(mcp, want) {
			t.Fatalf("Claude Code's .mcp.json must contain %s:\n%s", want, mcp)
		}
	}
	if strings.Contains(mcp, "${PLUGIN_") {
		t.Fatalf("Claude Code's .mcp.json must use Claude Code's variable names:\n%s", mcp)
	}
}

// An adapter that cannot build a plugin as written does not build it, and
// build.json and the client's index say so; every client's index lists only
// what its adapter built.
func TestAdaptersSkipWhatTheirClientCannotRun(t *testing.T) {
	out := t.TempDir()
	if _, err := compile.Build(fixtureSource("024-marketplace-indexes"), out, compile.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(out, compile.TargetName)
	build := read(t, target, ".typeference/build.json")
	for _, want := range []string{`"reason": "copilot-components"`, `"reason": "server-cwd"`} {
		if !strings.Contains(build, want) {
			t.Fatalf("build.json must record %s:\n%s", want, build)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "claude", "ops")); !os.IsNotExist(err) {
		t.Fatalf("a plugin with Copilot components must not build for Claude Code: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "claude", "signals")); !os.IsNotExist(err) {
		t.Fatalf("a plugin whose server sets cwd must not build for Claude Code: %v", err)
	}
	read(t, target, "codex/signals/plugin.json")
	claudeIndex := read(t, target, ".claude-plugin/marketplace.json")
	codexIndex := read(t, target, ".agents/plugins/marketplace.json")
	if strings.Contains(claudeIndex, `"ops"`) || strings.Contains(claudeIndex, `"signals"`) || !strings.Contains(claudeIndex, `"./claude/tickets"`) {
		t.Fatalf("Claude Code's index lists exactly what its adapter built:\n%s", claudeIndex)
	}
	if strings.Contains(codexIndex, `"ops"`) || !strings.Contains(codexIndex, `"./codex/signals"`) || !strings.Contains(codexIndex, `"category": "support"`) || !strings.Contains(codexIndex, `"category": "Productivity"`) {
		t.Fatalf("Codex's index lists what its adapter built, with each plugin's category or Productivity:\n%s", codexIndex)
	}
}

// A Copilot component written beside the copilot block fails and names the
// block, so nothing is dropped silently.
func TestComponentOutsideCopilotBlockNamesTheBlock(t *testing.T) {
	for _, fixture := range []string{"167-agents-outside-copilot", "168-profile-hooks-outside-copilot"} {
		_, err := compile.Build(fixtureSource(fixture), t.TempDir(), compile.BuildOptions{})
		if err == nil || !strings.Contains(err.Error(), "list them under 'copilot'") {
			t.Fatalf("%s: want an error naming the copilot block, got %v", fixture, err)
		}
	}
}

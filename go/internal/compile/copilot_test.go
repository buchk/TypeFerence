package compile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

// A plugin without a copilot block compiles to the portable Agent Plugins
// core: Copilot skill fields still need their own opt-in, and no
// com.github.copilot directory exists.
func TestPluginWithoutCopilotBlockIsPortable(t *testing.T) {
	source := filepath.Join("..", "..", "..", "conformance", "fixtures", "023-portable-core", "source")
	out := t.TempDir()
	if _, err := compile.Build(source, out, compile.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(out, compile.TargetName, "kit")
	for _, rel := range []string{"plugin.json", "mcp.json", "skills/tickets/SKILL.md", ".typeference/bundle.json"} {
		read(t, root, rel)
	}
	if _, err := os.Stat(filepath.Join(root, "com.github.copilot")); !os.IsNotExist(err) {
		t.Fatalf("a plugin without Copilot components must not emit com.github.copilot: %v", err)
	}
}

// A Copilot component written beside the copilot block fails and names the
// block, so nothing is dropped silently.
func TestComponentOutsideCopilotBlockNamesTheBlock(t *testing.T) {
	for _, fixture := range []string{"167-agents-outside-copilot", "168-profile-hooks-outside-copilot"} {
		source := filepath.Join("..", "..", "..", "conformance", "fixtures", fixture, "source")
		_, err := compile.Build(source, t.TempDir(), compile.BuildOptions{})
		if err == nil || !strings.Contains(err.Error(), "list them under 'copilot'") {
			t.Fatalf("%s: want an error naming the copilot block, got %v", fixture, err)
		}
	}
}

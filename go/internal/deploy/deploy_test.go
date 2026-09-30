package deploy

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

// writePackage writes a version 6 package whose manifest lists the plugins.
func writePackage(t *testing.T, plugins []string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	manifest := "---\nschemaVersion: 6\nname: acme/test\nversion: 1.0.0\nplugins:\n"
	for _, plugin := range plugins {
		manifest += "  - " + plugin + "\n"
	}
	writeTestFile(t, filepath.Join(root, "typeference.tfer"), manifest+"---\n")
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(name)), files[name])
	}
	return root
}

// toolPackage ships agent a, whose skill imports a runtime tool, in a plugin
// built for the given modes.
func toolPackage(t *testing.T, modes string) string {
	t.Helper()
	return writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer":        "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n" + modes + "---\n",
		"capabilities/c.capability.tfer": "---\nvisibility: exposed\n---\n",
		"tools/runtime.tool.tfer":        "---\ndescription: Runtime import.\n---\n",
		"skills/s.skill.tfer":            "---\ndescription: S.\nbinds: capabilities/c.capability.tfer\nrequiresTools:\n  - tools/runtime.tool.tfer\nvariants:\n  manual:\n    instructions: talk\n  pipeline:\n    instructions: emit\n  a2a:\n    instructions: call\n---\n",
		"agents/a.agent.tfer":            "---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n---\n",
	})
}

func build(t *testing.T, source string, targets ...compile.Target) string {
	t.Helper()
	out := t.TempDir()
	if _, err := compile.Build(source, out, targets, nil); err != nil {
		t.Fatal(err)
	}
	return out
}

const pluginDeployment = "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/test/plugins/kit@1.0.0:\n    modes: [manual, pipeline]\nproviders:\n  runtime:\n    kind: mcp\n    transport: stdio\n    command: runtime-server\n    args: ['--bundle', '{bundle}', 'literal={unknown}']\ntoolBindings:\n  acme/test/tools/runtime@1.0.0:\n    provider: runtime\n    remoteName: run\n"

func TestPluginMCPConfigurationIsMaterializedOnlyAtLink(t *testing.T) {
	built := build(t, toolPackage(t, "modes:\n  - manual\n  - pipeline\n"), compile.AgentPlugin)
	if _, err := os.Stat(filepath.Join(built, "agent-plugin", "kit", "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("build must not emit mcp.json")
	}
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, pluginDeployment)
	linked := filepath.Join(t.TempDir(), "linked")
	if _, err := Link(filepath.Join(built, "agent-plugin"), deployment, linked); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{"kit", "kit-pipeline"} {
		config, err := os.ReadFile(filepath.Join(linked, artifact, "mcp.json"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(config)
		for _, want := range []string{
			`"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"`,
			`"type": "stdio"`,
			`"command": "runtime-server"`,
			`"${PLUGIN_ROOT}/.typeference/bundle.json"`,
			`"literal={unknown}"`,
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s mcp.json is missing %s:\n%s", artifact, want, text)
			}
		}
		if strings.Contains(text, `"env"`) {
			t.Fatalf("a plugin mcp.json never carries environment values:\n%s", text)
		}
	}
	bindings, err := os.ReadFile(filepath.Join(linked, "kit-pipeline", ".typeference", "tool-bindings.json"))
	if err != nil || !strings.Contains(string(bindings), `"remoteName": "run"`) || !strings.Contains(string(bindings), "\"selectedModes\": [\n    \"pipeline\"\n  ]") {
		t.Fatalf("each artifact records its one materialized mode and bindings: %v\n%s", err, bindings)
	}
	if _, err := os.Stat(filepath.Join(linked, ".typeference", "build.json")); !os.IsNotExist(err) {
		t.Fatalf("linked output must not retain a stale active build index, got %v", err)
	}
	provenance, err := os.ReadFile(filepath.Join(linked, ".typeference", "link-provenance.json"))
	if err != nil || !strings.Contains(string(provenance), `"schemaVersion": 2`) || !strings.Contains(string(provenance), `"path": "kit-pipeline"`) {
		t.Fatalf("linked plugin digests were not recorded: %v\n%s", err, provenance)
	}
	// A completed plugin link may be replaced by a relink.
	if _, err := Link(filepath.Join(built, "agent-plugin"), deployment, linked); err != nil {
		t.Fatalf("relinking over a completed plugin link must succeed: %v", err)
	}
}

func TestPluginLinkRequiresEveryBuiltMode(t *testing.T) {
	built := build(t, toolPackage(t, "modes:\n  - manual\n  - pipeline\n"), compile.AgentPlugin)
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, strings.Replace(pluginDeployment, "modes: [manual, pipeline]", "modes: [pipeline]", 1))
	if _, err := Link(filepath.Join(built, "agent-plugin"), deployment, filepath.Join(t.TempDir(), "linked")); err == nil ||
		!strings.Contains(err.Error(), "must select every mode") {
		t.Fatalf("a plugin's artifacts link together, got %v", err)
	}
}

func TestPluginLinkRefusesCredentialForwarding(t *testing.T) {
	built := build(t, toolPackage(t, ""), compile.AgentPlugin)
	cases := map[string]string{
		"environment": strings.Replace(pluginDeployment, "    args: ['--bundle', '{bundle}', 'literal={unknown}']\n",
			"    args: ['--bundle', '{bundle}']\n    environment:\n      ACME_TOKEN:\n        fromEnvironment: ACME_TOKEN\n", 1),
		"bearer": strings.Replace(pluginDeployment,
			"    transport: stdio\n    command: runtime-server\n    args: ['--bundle', '{bundle}', 'literal={unknown}']\n",
			"    transport: http\n    url: https://tools.example/mcp\n    bearerTokenEnvironment: ACME_TOKEN\n", 1),
		"command": strings.Replace(pluginDeployment, "command: runtime-server", "command: /usr/local/bin/runtime-server", 1),
	}
	for name, text := range cases {
		text = strings.Replace(text, "modes: [manual, pipeline]", "modes: [manual]", 1)
		deployment := filepath.Join(t.TempDir(), "deploy.yaml")
		writeTestFile(t, deployment, text)
		output := filepath.Join(t.TempDir(), "linked")
		if _, err := Link(filepath.Join(built, "agent-plugin"), deployment, output); err == nil {
			t.Errorf("%s: link must fail closed", name)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Errorf("%s: a refused link must not write output", name)
		}
	}
}

func TestA2ACardRequiresNeutralArtifactAndA2AMode(t *testing.T) {
	built := build(t, toolPackage(t, ""), compile.Neutral, compile.AgentPlugin)
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/test/agents/a@1.0.0:\n    modes: [a2a]\n  acme/test/plugins/kit@1.0.0:\n    modes: [manual]\nproviders:\n  runtime:\n    kind: mcp\n    transport: http\n    url: https://tools.example/mcp\ntoolBindings:\n  acme/test/tools/runtime@1.0.0:\n    provider: runtime\n    remoteName: run\nagentEndpoints:\n  acme/test/agents/a@1.0.0:\n    a2aUrl: https://agents.example/a\n")
	linked := filepath.Join(t.TempDir(), "neutral-linked")
	if _, err := Link(filepath.Join(built, "neutral"), deployment, linked); err != nil {
		t.Fatal(err)
	}
	card, err := os.ReadFile(filepath.Join(linked, "a", ".typeference", "a2a-agent-card.json"))
	if err != nil || !strings.Contains(string(card), `"url": "https://agents.example/a"`) {
		t.Fatalf("A2A card did not preserve the authored endpoint: %v\n%s", err, card)
	}
	pluginDeploymentWithEndpoint := strings.Replace(pluginDeployment, "toolBindings:", "agentEndpoints:\n  acme/test/plugins/kit@1.0.0:\n    a2aUrl: https://agents.example/kit\ntoolBindings:", 1)
	pluginDeploymentWithEndpoint = strings.Replace(pluginDeploymentWithEndpoint, "modes: [manual, pipeline]", "modes: [manual]", 1)
	writeTestFile(t, deployment, pluginDeploymentWithEndpoint)
	if _, err := Link(filepath.Join(built, "agent-plugin"), deployment, filepath.Join(t.TempDir(), "plugin-linked")); err == nil ||
		!strings.Contains(err.Error(), "requires a neutral artifact") {
		t.Fatalf("an A2A endpoint must reject a plugin artifact, got %v", err)
	}
}

func TestLinkRejectsTamperedUnlinkedArtifact(t *testing.T) {
	input, deployment := buildLinkFixture(t)
	writeTestFile(t, filepath.Join(input, "a", "AGENTS.md"), "tampered\n")
	if _, err := Link(input, deployment, filepath.Join(t.TempDir(), "linked")); err == nil ||
		!strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected tampering to fail integrity verification, got %v", err)
	}
}

func TestLinkRejectsOutputThatContainsItsInput(t *testing.T) {
	input, deployment := buildLinkFixture(t)
	container := filepath.Dir(input)
	if _, err := Link(input, deployment, container); err == nil ||
		!strings.Contains(err.Error(), "must not contain or be contained") {
		t.Fatalf("expected containing output to be rejected before reset, got %v", err)
	}
}

func TestLinkRefusesNonEmptyUnownedOutputWithoutDeletingIt(t *testing.T) {
	input, deployment := buildLinkFixture(t)
	output := filepath.Join(t.TempDir(), "examples")
	sentinel := filepath.Join(output, "keep-me.txt")
	writeTestFile(t, sentinel, "user data\n")

	if _, err := Link(input, deployment, output); err == nil ||
		!strings.Contains(err.Error(), "not a TypeFerence-owned linked output") {
		t.Fatalf("expected an unowned non-empty output to fail closed, got %v", err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "user data\n" {
		t.Fatalf("link changed or removed the unrelated sentinel: %v, %q", err, data)
	}
}

func TestLinkReplacesOnlyAValidPriorLinkedOutput(t *testing.T) {
	input, deployment := buildLinkFixture(t)
	output := t.TempDir()
	if _, err := Link(input, deployment, output); err != nil {
		t.Fatalf("link should accept an empty output directory: %v", err)
	}
	stale := filepath.Join(output, "stale-generated-file.txt")
	writeTestFile(t, stale, "stale\n")
	if _, err := Link(input, deployment, output); err != nil {
		t.Fatalf("link should replace its prior completed output: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("relink must remove stale generated files, got %v", err)
	}

	writeTestFile(t, filepath.Join(output, ".typeference", "link-provenance.json"), "not json\n")
	sentinel := filepath.Join(output, "keep-after-corruption.txt")
	writeTestFile(t, sentinel, "keep\n")
	if _, err := Link(input, deployment, output); err == nil ||
		!strings.Contains(err.Error(), "not a TypeFerence-owned linked output") {
		t.Fatalf("malformed provenance must not authorize recursive replacement, got %v", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep\n" {
		t.Fatalf("failed relink changed the existing output: %v, %q", err, data)
	}
}

func TestInvalidEndpointFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, path, "schemaVersion: 1\nenvironment: test\nagentEndpoints:\n  acme/agents/a@1.0.0:\n    a2aUrl: http://insecure.example/a\n")
	if _, _, err := Load(path); err == nil {
		t.Fatal("an insecure A2A endpoint must be rejected")
	}
}

func TestToolFreeLinkedArtifactRecordsSelectedModes(t *testing.T) {
	source := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"skills/s.skill.tfer":     "---\ndescription: S.\nvariants:\n  manual:\n    instructions: talk\n  pipeline:\n    instructions: emit\n---\n",
		"agents/a.agent.tfer":     "---\ndescription: A.\nskills:\n  - skills/s.skill.tfer\n---\n",
	})
	built := build(t, source, compile.Neutral)
	deployment := filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/test/agents/a@1.0.0:\n    modes: [pipeline]\n")
	linked := filepath.Join(t.TempDir(), "linked")
	if _, err := Link(filepath.Join(built, "neutral"), deployment, linked); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(linked, "a", ".typeference", "tool-bindings.json"))
	if err != nil || !strings.Contains(string(manifest), "\"selectedModes\": [\n    \"pipeline\"\n  ]") {
		t.Fatalf("tool-free link did not preserve selected modes: %v\n%s", err, manifest)
	}
}

func buildLinkFixture(t *testing.T) (input, deployment string) {
	t.Helper()
	source := writePackage(t, []string{"plugins/kit.plugin.tfer"}, map[string]string{
		"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nagents:\n  - agents/a.agent.tfer\n---\n",
		"agents/a.agent.tfer":     "---\ndescription: A.\n---\n",
	})
	container := t.TempDir()
	built := filepath.Join(container, "built")
	if _, err := compile.Build(source, built, []compile.Target{compile.Neutral}, nil); err != nil {
		t.Fatal(err)
	}
	deployment = filepath.Join(t.TempDir(), "deploy.yaml")
	writeTestFile(t, deployment, "schemaVersion: 1\nenvironment: test\nartifacts:\n  acme/test/agents/a@1.0.0:\n    modes: []\n")
	return filepath.Join(built, "neutral"), deployment
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

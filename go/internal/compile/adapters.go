package compile

import (
	"path/filepath"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// The build adapters, one per supported client (ADR-0001). Every build runs
// all of them over the same resolved plugins; each writes its client's
// layout beneath <target>/<client>/ and its client's marketplace index.
const (
	clientCopilot = "copilot"
	clientClaude  = "claude"
	clientCodex   = "codex"
)

var clients = []string{clientCopilot, clientClaude, clientCodex}

// skipReason says why a client's adapter does not build an artifact, or ""
// when it does. An adapter never builds a plugin without part of its
// composition.
func (c *compilation) skipReason(client string, artifact pluginArtifact) string {
	if client == clientCopilot {
		return ""
	}
	plan := artifact.plan
	if len(plan.Agents)+len(plan.Rules)+len(plan.Commands)+len(plan.Hooks)+len(plan.LSP) > 0 {
		return "copilot-components"
	}
	if client == clientClaude {
		for _, doc := range c.artifactServers(artifact) {
			if doc.Server.Cwd != "" {
				return "server-cwd"
			}
		}
	}
	return ""
}

// skippedArtifact records an artifact a client's adapter does not build.
type skippedArtifact struct {
	client   string
	artifact pluginArtifact
	reason   string
}

// pluginManifest is the shared manifest body: name, version, description,
// and the plugin's metadata members.
func pluginManifest(artifact pluginArtifact) jsonx.Obj {
	manifest := jsonx.Obj{
		{K: "name", V: jsonx.Str(artifact.dir)},
		{K: "version", V: jsonx.Str(artifact.plan.Version)},
		{K: "description", V: jsonx.Str(artifact.plan.Description)},
	}
	return append(manifest, metadataMembers(artifact.plan.Metadata)...)
}

// writeClientConfig writes an artifact's manifest and MCP configuration in
// its client's format: Agent Plugins 1.0 for Copilot and Codex, and Claude
// Code's .claude-plugin/plugin.json and .mcp.json.
func writeClientConfig(client, dir string, artifact pluginArtifact, servers []*resource.Document, written *[]string) error {
	if client == clientClaude {
		if err := writeFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), jsonx.Indented(pluginManifest(artifact))+"\n", written); err != nil {
			return err
		}
		if len(servers) == 0 {
			return nil
		}
		return writeFile(filepath.Join(dir, ".mcp.json"), claudeMCPJSON(servers)+"\n", written)
	}
	manifest := append(jsonx.Obj{{K: "$schema", V: jsonx.Str(pluginSchemaURI)}}, pluginManifest(artifact)...)
	if err := writeFile(filepath.Join(dir, "plugin.json"), jsonx.Indented(manifest)+"\n", written); err != nil {
		return err
	}
	if len(servers) == 0 {
		return nil
	}
	return writeFile(filepath.Join(dir, "mcp.json"), mcpJSON(servers)+"\n", written)
}

// claudeMCPJSON renders Claude Code's MCP configuration, spelling the plugin
// root and data directory as Claude Code does.
func claudeMCPJSON(servers []*resource.Document) string {
	entries := jsonx.Obj{}
	for _, doc := range servers {
		entries = append(entries, jsonx.Member{K: resource.Leaf(doc.ID), V: claudeServerValue(doc.Server)})
	}
	return jsonx.Indented(jsonx.Obj{{K: "mcpServers", V: entries}})
}

func claudeServerValue(s *resource.ServerConfig) jsonx.Obj {
	if s.Transport != "stdio" {
		obj := jsonx.Obj{
			{K: "type", V: jsonx.Str("http")},
			{K: "url", V: jsonx.Str(s.URL)},
		}
		if s.Headers != nil {
			obj = append(obj, jsonx.Member{K: "headers", V: stringMapValue(s.Headers)})
		}
		return obj
	}
	command := s.Command
	if rest, relative := strings.CutPrefix(command, "./"); relative {
		command = "${CLAUDE_PLUGIN_ROOT}/" + rest
	}
	obj := jsonx.Obj{{K: "command", V: jsonx.Str(command)}}
	if s.Args != nil {
		args := make([]string, len(s.Args))
		for i, arg := range s.Args {
			args[i] = claudeSpelling(arg)
		}
		obj = append(obj, jsonx.Member{K: "args", V: stringArr(args)})
	}
	if s.Env != nil {
		env := map[string]string{}
		for key, value := range s.Env {
			env[key] = claudeSpelling(value)
		}
		obj = append(obj, jsonx.Member{K: "env", V: stringMapValue(env)})
	}
	return obj
}

// claudeSpelling rewrites the Agent Plugins variables to Claude Code's names
// for the same directories; server values contain no other ${.
func claudeSpelling(value string) string {
	value = strings.ReplaceAll(value, "${PLUGIN_ROOT}", "${CLAUDE_PLUGIN_ROOT}")
	return strings.ReplaceAll(value, "${PLUGIN_DATA}", "${CLAUDE_PLUGIN_DATA}")
}

// marketplaceJSON renders the Copilot or Claude Code marketplace index: the
// same members, each source pointing into the client's directory.
func marketplaceJSON(client string, marketplace *resource.Marketplace, version string, artifacts []pluginArtifact) string {
	plugins := jsonx.Arr{}
	for _, artifact := range artifacts {
		entry := jsonx.Obj{
			{K: "name", V: jsonx.Str(artifact.dir)},
			{K: "source", V: jsonx.Str("./" + client + "/" + artifact.dir)},
			{K: "description", V: jsonx.Str(artifact.plan.Description)},
			{K: "version", V: jsonx.Str(artifact.plan.Version)},
		}
		entry = append(entry, metadataMembers(artifact.plan.Metadata)...)
		plugins = append(plugins, append(entry, catalogMembers(artifact.plan.Metadata)...))
	}
	metadata := jsonx.Obj{}
	if marketplace.Description != "" {
		metadata = append(metadata, jsonx.Member{K: "description", V: jsonx.Str(marketplace.Description)})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "name", V: jsonx.Str(marketplace.Name)},
		{K: "owner", V: jsonx.Obj{{K: "name", V: jsonx.Str(marketplace.Owner)}}},
		{K: "metadata", V: append(metadata, jsonx.Member{K: "version", V: jsonx.Str(version)})},
		{K: "plugins", V: plugins},
	})
}

// codexMarketplaceJSON renders Codex's marketplace index. Codex requires a
// policy and a category on every entry; the policy is fixed and the category
// is the plugin's, or Productivity.
func codexMarketplaceJSON(marketplace *resource.Marketplace, artifacts []pluginArtifact) string {
	plugins := jsonx.Arr{}
	for _, artifact := range artifacts {
		category := "Productivity"
		if artifact.plan.Metadata.Category != "" {
			category = artifact.plan.Metadata.Category
		}
		plugins = append(plugins, jsonx.Obj{
			{K: "name", V: jsonx.Str(artifact.dir)},
			{K: "source", V: jsonx.Obj{
				{K: "source", V: jsonx.Str("local")},
				{K: "path", V: jsonx.Str("./" + clientCodex + "/" + artifact.dir)},
			}},
			{K: "policy", V: jsonx.Obj{
				{K: "installation", V: jsonx.Str("AVAILABLE")},
				{K: "authentication", V: jsonx.Str("ON_INSTALL")},
			}},
			{K: "category", V: jsonx.Str(category)},
		})
	}
	return jsonx.Indented(jsonx.Obj{
		{K: "name", V: jsonx.Str(marketplace.Name)},
		{K: "plugins", V: plugins},
	})
}

// writeMarketplaces writes each client's index at the path that client reads,
// for every client whose adapter built at least one artifact.
func writeMarketplaces(root string, marketplace *resource.Marketplace, version string, built map[string][]pluginArtifact, written *[]string) error {
	if artifacts := built[clientCopilot]; len(artifacts) > 0 {
		if err := writeFile(filepath.Join(root, ".github", "plugin", "marketplace.json"), marketplaceJSON(clientCopilot, marketplace, version, artifacts)+"\n", written); err != nil {
			return err
		}
	}
	if artifacts := built[clientClaude]; len(artifacts) > 0 {
		if err := writeFile(filepath.Join(root, ".claude-plugin", "marketplace.json"), marketplaceJSON(clientClaude, marketplace, version, artifacts)+"\n", written); err != nil {
			return err
		}
	}
	if artifacts := built[clientCodex]; len(artifacts) > 0 {
		if err := writeFile(filepath.Join(root, ".agents", "plugins", "marketplace.json"), codexMarketplaceJSON(marketplace, artifacts)+"\n", written); err != nil {
			return err
		}
	}
	return nil
}

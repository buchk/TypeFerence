// Package mcpserver serves the typeference CLI as Model Context Protocol
// tools over stdio (ADR-0008). Each tool call becomes one CLI invocation with
// the arguments a person would type; the server owns no compiler semantics.
package mcpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Runner runs the typeference CLI with args and reports what it printed and
// its exit code. An error means the CLI could not be run at all.
type Runner func(args []string) (stdout, stderr string, code int, err error)

// supportedVersions are the MCP protocol revisions this server speaks, newest
// first. A client that asks for another revision is answered with the newest.
var supportedVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type param struct {
	name        string
	description string
	flag        string // "" for a positional argument
	boolean     bool
	required    bool
}

type tool struct {
	name        string
	command     string
	description string
	params      []param
	// differences marks a command whose exit code 1 is a result, not a
	// failure: diff exits 1 when the directories differ.
	differences bool
}

var (
	sourceParam   = param{name: "source", description: "Path to the package root, the directory holding typeference.tfer.", required: true}
	packagesParam = param{name: "packagesDir", description: "Materialized package directory restore wrote (default obj/typeference/packages under the source).", flag: "--packages-dir"}
	feedsParam    = param{name: "feeds", description: "Path to the external feed configuration (never part of source).", flag: "--feeds", required: true}
)

var tools = []tool{
	{name: "typeference_validate", command: "validate",
		description: "Resolve every plugin and agent in a package without writing anything, and report compatibility conflicts. With candidate, check an unpublished package against a marketplace package's locked graph.",
		params: []param{sourceParam, packagesParam,
			{name: "candidate", description: "Path to an unpublished package to check against this marketplace package.", flag: "--candidate"}}},
	{name: "typeference_build", command: "build",
		description: "Compile a package offline and deterministically into <out>/agent-plugin: Agent Plugins 1.0 packages, Copilot components where the source lists them under copilot, and the marketplace index when the manifest declares one.",
		params: []param{sourceParam,
			{name: "out", description: "Output directory (default dist). It must not be inside the source root except under dist, bin, or obj.", flag: "--out"},
			packagesParam}},
	{name: "typeference_diff", command: "diff", differences: true,
		description: "Rebuild a package and byte-compare it with a committed output directory. Lists added (+), removed (-), and changed (~) files; reports No differences. when they match.",
		params: []param{sourceParam,
			{name: "against", description: "The committed output directory to compare with, such as dist.", flag: "--against", required: true},
			packagesParam,
			{name: "json", description: "Report the comparison as JSON.", flag: "--json", boolean: true}}},
	{name: "typeference_inspect", command: "inspect",
		description: "Print one resolved agent as JSON: its extension chain, objectives by source, documents, and the skills its plugin ships beside it.",
		params: []param{
			{name: "agentId", description: "The agent's identity, <package>/<path stem>@<version>, such as acme/team/agents/ops@1.2.0.", required: true},
			{name: "source", description: "Path to the package root (default the working directory).", flag: "--source"},
			packagesParam}},
	{name: "typeference_import", command: "import",
		description: "Convert existing Copilot customizations (a repository's agents and skills, a skill directory, or an Agent Plugin) into a new package. Fails, listing every item, on anything the language cannot represent unless lossy is set.",
		params: []param{
			{name: "source", description: "The directory to import from.", required: true},
			{name: "out", description: "A new or empty directory to write the package into.", flag: "--out", required: true},
			{name: "name", description: "Package name, such as acme/payments (default local/<plugin>).", flag: "--name"},
			{name: "version", description: "Exact package version (default the plugin's, else 0.1.0).", flag: "--version"},
			{name: "plugin", description: "Plugin name (default the source plugin's or directory's name).", flag: "--plugin"},
			{name: "lossy", description: "Import without what the language cannot represent, listing each dropped item.", flag: "--lossy", boolean: true}}},
	{name: "typeference_init", command: "init",
		description: "Scaffold a starter package from a versioned setup answer set (the same generator as the browser wizard).",
		params: []param{
			{name: "answers", description: "Path to the answer set JSON file.", flag: "--answers", required: true},
			{name: "out", description: "Directory to write the package into (default the working directory).", flag: "--out"},
			{name: "verify", description: "Expected sha256: digest of the generated tree.", flag: "--verify"}}},
	{name: "typeference_restore", command: "restore",
		description: "Resolve the package's exact dependencies from configured feeds, verify every digest, write typeference.lock, and materialize the graph. The only command that contacts feeds; it honors an existing lockfile and never rewrites it.",
		params: []param{sourceParam, feedsParam,
			{name: "locked", description: "Require an existing lockfile.", flag: "--locked", boolean: true},
			packagesParam}},
	{name: "typeference_update", command: "update",
		description: "Re-resolve and rewrite typeference.lock after the manifest's exact dependency versions change.",
		params:      []param{sourceParam, feedsParam, packagesParam}},
	{name: "typeference_pack", command: "pack",
		description: "Write one canonical source package (.tferpkg) for publishing to a feed.",
		params: []param{sourceParam,
			{name: "out", description: "Package file to write (default package.tferpkg).", flag: "--out"}}},
}

// Serve answers JSON-RPC requests read line by line from r, writing one
// response per line to w, until r ends.
func Serve(r io.Reader, w io.Writer, run Runner, version string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(w)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if response := handle([]byte(line), run, version); response != nil {
			data, err := json.Marshal(response)
			if err != nil {
				return err
			}
			if _, err := out.Write(append(data, '\n')); err != nil {
				return err
			}
			if err := out.Flush(); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func handle(line []byte, run Runner, version string) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error"}}
	}
	if len(req.ID) == 0 {
		return nil // a notification, such as notifications/initialized
	}
	reply := func(result any) *response { return &response{JSONRPC: "2.0", ID: req.ID, Result: result} }
	fail := func(code int, message string) *response {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: message}}
	}
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		chosen := supportedVersions[0]
		for _, v := range supportedVersions {
			if v == params.ProtocolVersion {
				chosen = v
			}
		}
		return reply(map[string]any{
			"protocolVersion": chosen,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "typeference", "version": version},
			"instructions":    "Each tool runs the typeference CLI once with the arguments it describes and returns the CLI's output and exit code. Relative paths resolve against the server's working directory. Exit code 2 is a validation or execution failure; read the diagnostic, fix the source, and rerun.",
		})
	case "ping":
		return reply(map[string]any{})
	case "tools/list":
		list := make([]any, 0, len(tools))
		for _, t := range tools {
			list = append(list, t.schema())
		}
		return reply(map[string]any{"tools": list})
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return fail(-32602, "invalid tools/call params")
		}
		t, ok := find(params.Name)
		if !ok {
			return fail(-32602, "unknown tool: "+params.Name)
		}
		args, err := t.arguments(params.Arguments)
		if err != nil {
			return reply(toolResult(err.Error(), true))
		}
		stdout, stderr, code, err := run(args)
		if err != nil {
			return reply(toolResult("typeference could not run: "+err.Error(), true))
		}
		failed := code != 0 && !(t.differences && code == 1)
		return reply(toolResult(report(args, stdout, stderr, code), failed))
	}
	return fail(-32601, "method not found: "+req.Method)
}

func find(name string) (tool, bool) {
	for _, t := range tools {
		if t.name == name {
			return t, true
		}
	}
	return tool{}, false
}

func (t tool) schema() map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, p := range t.params {
		kind := "string"
		if p.boolean {
			kind = "boolean"
		}
		properties[p.name] = map[string]any{"type": kind, "description": p.description}
		if p.required {
			required = append(required, p.name)
		}
	}
	return map[string]any{
		"name":        t.name,
		"description": t.description,
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		},
	}
}

// arguments maps tool arguments to the CLI argument list: the command, its
// positional arguments in table order, then its flags in table order.
func (t tool) arguments(values map[string]any) ([]string, error) {
	known := map[string]bool{}
	for _, p := range t.params {
		known[p.name] = true
	}
	for name := range values {
		if !known[name] {
			return nil, fmt.Errorf("%s does not take '%s'", t.name, name)
		}
	}
	args := []string{t.command}
	var flags []string
	for _, p := range t.params {
		value, present := values[p.name]
		if p.boolean {
			if !present {
				continue
			}
			b, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("'%s' must be a boolean", p.name)
			}
			if b {
				flags = append(flags, p.flag)
			}
			continue
		}
		text := ""
		if present {
			s, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("'%s' must be a string", p.name)
			}
			text = s
		}
		if text == "" {
			if p.required {
				return nil, fmt.Errorf("'%s' is required", p.name)
			}
			continue
		}
		if strings.HasPrefix(text, "--") {
			return nil, fmt.Errorf("'%s' must not start with --", p.name)
		}
		if p.flag == "" {
			args = append(args, text)
		} else {
			flags = append(flags, p.flag, text)
		}
	}
	return append(args, flags...), nil
}

func report(args []string, stdout, stderr string, code int) string {
	var b strings.Builder
	b.WriteString("$ typeference " + strings.Join(args, " ") + "\n")
	if s := strings.TrimRight(stdout, "\n"); s != "" {
		b.WriteString(s + "\n")
	}
	if s := strings.TrimRight(stderr, "\n"); s != "" {
		b.WriteString(s + "\n")
	}
	fmt.Fprintf(&b, "exit code %d", code)
	return b.String()
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": isError,
	}
}

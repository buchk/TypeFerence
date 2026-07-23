// Command typeference is the Go implementation of the TypeFerence CLI. It
// implements the same command surface as the C# reference implementation and
// produces byte-identical artifacts (verified by the shared conformance
// suite under conformance/).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/eval"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// version is stamped by the release workflow via
// -ldflags "-X main.version=<semver>"; development builds report "dev".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		return help()
	}
	var code int
	var err error
	switch args[0] {
	case "validate":
		code, err = validate(args)
	case "build":
		code, err = build(args)
	case "inspect":
		code, err = inspect(args)
	case "diff":
		code, err = diff(args)
	case "publish":
		code, err = publish(args)
	case "eval":
		code, err = evalCommand(args)
	case "equivalence":
		code, err = equivalenceCommand(args)
	case "version", "--version":
		fmt.Printf("typeference %s\n", version)
		return 0
	default:
		return fail(fmt.Sprintf("Unknown command: %s", args[0]))
	}
	if err != nil {
		return fail(err.Error())
	}
	return code
}

func validate(args []string) (int, error) {
	source, err := requiredArg(args, 1, "source")
	if err != nil {
		return 0, err
	}
	trustConfig, err := option(args, "--trust-config")
	if err != nil {
		return 0, err
	}
	agents, err := compile.Validate(source, trustConfig)
	if err != nil {
		return 0, err
	}
	fmt.Printf("Valid: %d agents resolved.\n", len(agents))
	return 0, nil
}

func build(args []string) (int, error) {
	source, err := requiredArg(args, 1, "source")
	if err != nil {
		return 0, err
	}
	output := "dist"
	if v, err := option(args, "--out"); err != nil {
		return 0, err
	} else if v != "" {
		output = v
	}
	target := "all"
	if v, err := option(args, "--target"); err != nil {
		return 0, err
	} else if v != "" {
		target = v
	}
	ard, err := ardOptions(args, source)
	if err != nil {
		return 0, err
	}
	targets, err := compile.ParseTargets(target)
	if err != nil {
		return 0, err
	}
	files, err := compile.Build(source, output, targets, ard)
	if err != nil {
		return 0, err
	}
	full, absErr := filepath.Abs(output)
	if absErr != nil {
		full = output
	}
	hash, err := compile.HashDirectory(output)
	if err != nil {
		return 0, err
	}
	fmt.Printf("Built %d files at %s\n", len(files), full)
	fmt.Printf("SHA-256 %s\n", hash)
	return 0, nil
}

func inspect(args []string) (int, error) {
	source := "."
	if v, err := option(args, "--source"); err != nil {
		return 0, err
	} else if v != "" {
		source = v
	}
	id, err := requiredArg(args, 1, "agent id")
	if err != nil {
		return 0, err
	}
	agents, err := compile.Validate(source, "")
	if err != nil {
		return 0, err
	}
	for _, agent := range agents {
		if agent.ID == id {
			fmt.Println(compile.BundleJSON(agent))
			return 0, nil
		}
	}
	return 0, resource.Errorf("Agent not found: %s", id)
}

func diff(args []string) (int, error) {
	source, err := requiredArg(args, 1, "source")
	if err != nil {
		return 0, err
	}
	against, err := option(args, "--against")
	if err != nil {
		return 0, err
	}
	if against == "" {
		return 0, resource.Errorf("--against is required")
	}
	temp, err := os.MkdirTemp("", "typeference-diff-")
	if err != nil {
		return 0, resource.Errorf("Cannot create temporary directory")
	}
	defer os.RemoveAll(temp)

	ard, err := ardOptions(args, source)
	if err != nil {
		return 0, err
	}
	target := "all"
	if v, err := option(args, "--target"); err != nil {
		return 0, err
	} else if v != "" {
		target = v
	}
	targets, err := compile.ParseTargets(target)
	if err != nil {
		return 0, err
	}
	if _, err := compile.Build(source, temp, targets, ard); err != nil {
		return 0, err
	}
	result, err := compile.CompareDirs(against, temp)
	if err != nil {
		return 0, err
	}
	if slices.Contains(args, "--json") {
		fmt.Println(jsonx.Indented(jsonx.Obj{
			{K: "Different", V: jsonx.Bool(result.Different)},
			{K: "Added", V: stringArr(result.Added)},
			{K: "Removed", V: stringArr(result.Removed)},
			{K: "Changed", V: stringArr(result.Changed)},
		}))
	} else {
		for _, x := range result.Added {
			fmt.Printf("+ %s\n", x)
		}
		for _, x := range result.Removed {
			fmt.Printf("- %s\n", x)
		}
		for _, x := range result.Changed {
			fmt.Printf("~ %s\n", x)
		}
		if !result.Different {
			fmt.Println("No differences.")
		}
	}
	if result.Different {
		return 1, nil
	}
	return 0, nil
}

// publish registers a compiled ARD catalog with a registry. It is the
// deployment edge, not the deterministic core (ADR-0018): registry lifecycle is
// disclaimed by core semantics, so this is an optional, side-effecting verb.
// Without --registry it is a dry run that only summarizes the catalog.
func publish(args []string) (int, error) {
	dir, err := requiredArg(args, 1, "ard directory")
	if err != nil {
		return 0, err
	}
	catalogPath := filepath.Join(dir, "ai-catalog.json")
	data, readErr := os.ReadFile(catalogPath)
	if readErr != nil {
		return 0, resource.Errorf("No ai-catalog.json under %s; build with --emit-ard first", dir)
	}
	var catalog struct {
		Entries []struct {
			Identifier string `json:"identifier"`
			Type       string `json:"type"`
		} `json:"entries"`
	}
	if json.Unmarshal(data, &catalog) != nil {
		return 0, resource.Errorf("Invalid ai-catalog.json: %s", catalogPath)
	}
	registry, err := option(args, "--registry")
	if err != nil {
		return 0, err
	}
	if registry == "" {
		fmt.Printf("Dry run: %d catalog entries in %s (pass --registry <url> to publish)\n", len(catalog.Entries), catalogPath)
		for _, e := range catalog.Entries {
			fmt.Printf("  %s  %s\n", entryKind(e.Type), e.Identifier)
		}
		return 0, nil
	}
	resp, postErr := http.Post(registry, "application/json", bytes.NewReader(data))
	if postErr != nil {
		return 0, resource.Errorf("Publish failed: %s", postErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 1, resource.Errorf("Registry returned %s", resp.Status)
	}
	fmt.Printf("Published %d entries to %s (%s)\n", len(catalog.Entries), registry, resp.Status)
	return 0, nil
}

// entryKind maps an ARD entry media type to a short label.
func entryKind(mediaType string) string {
	switch {
	case strings.Contains(mediaType, "a2a-agent-card"):
		return "a2a    "
	case strings.Contains(mediaType, "mcp-server"):
		return "mcp    "
	case strings.Contains(mediaType, "source-package"):
		return "source "
	case strings.Contains(mediaType, "target-bundle"):
		return "bundle "
	default:
		return "entry  "
	}
}

func evalCommand(args []string) (int, error) {
	source, err := requiredArg(args, 1, "source")
	if err != nil {
		return 0, err
	}
	scenarios, err := option(args, "--scenarios")
	if err != nil {
		return 0, err
	}
	if scenarios == "" {
		return 0, resource.Errorf("--scenarios is required")
	}
	model, err := option(args, "--model")
	if err != nil {
		return 0, err
	}
	outDir, err := option(args, "--out")
	if err != nil {
		return 0, err
	}
	live := slices.Contains(args, "--live")

	opts := eval.Options{Model: model, Live: live, OutDir: outDir}
	if live {
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return 0, resource.Errorf("--live requires ANTHROPIC_API_KEY in the environment; run without --live for a dry run")
		}
		opts.Backend = &eval.AnthropicBackend{APIKey: apiKey}
	}
	return eval.Run(source, scenarios, opts)
}

func equivalenceCommand(args []string) (int, error) {
	subcommand, err := requiredArg(args, 1, "subcommand (pack or score)")
	if err != nil {
		return 0, err
	}
	switch subcommand {
	case "pack":
		source, err := requiredArg(args, 2, "source")
		if err != nil {
			return 0, err
		}
		scenarios, err := option(args, "--scenarios")
		if err != nil {
			return 0, err
		}
		if scenarios == "" {
			return 0, resource.Errorf("--scenarios is required")
		}
		outDir, err := option(args, "--out")
		if err != nil {
			return 0, err
		}
		if outDir == "" {
			return 0, resource.Errorf("--out is required")
		}
		target := "all"
		if v, err := option(args, "--target"); err != nil {
			return 0, err
		} else if v != "" {
			target = v
		}
		targets, err := eval.ParseTargetList(target)
		if err != nil {
			return 0, err
		}
		return eval.Pack(source, scenarios, outDir, eval.PackOptions{Targets: targets})
	case "score":
		runDir, err := requiredArg(args, 2, "run directory")
		if err != nil {
			return 0, err
		}
		model, err := option(args, "--model")
		if err != nil {
			return 0, err
		}
		live := slices.Contains(args, "--live")
		opts := eval.ScoreOptions{Model: model, Live: live}
		if live {
			apiKey := os.Getenv("ANTHROPIC_API_KEY")
			if apiKey == "" {
				return 0, resource.Errorf("--live requires ANTHROPIC_API_KEY in the environment; run without --live to emit judge payloads")
			}
			opts.Backend = &eval.AnthropicBackend{APIKey: apiKey}
		}
		return eval.Score(runDir, opts)
	default:
		return 0, resource.Errorf("Unknown equivalence subcommand: %s (expected pack or score)", subcommand)
	}
}

func stringArr(values []string) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, v := range values {
		arr = append(arr, jsonx.Str(v))
	}
	return arr
}

func ardOptions(args []string, source string) (*compile.ArdPublicationOptions, error) {
	publisherDomain, err := option(args, "--publisher-domain")
	if err != nil {
		return nil, err
	}
	trustConfig, err := option(args, "--trust-config")
	if err != nil {
		return nil, err
	}
	trustSignatures, err := option(args, "--trust-signatures")
	if err != nil {
		return nil, err
	}
	allowUnsignedTrust := slices.Contains(args, "--allow-unsigned-trust")
	emitArd := slices.Contains(args, "--emit-ard")
	// A project manifest (typeference.yaml) with a publisher makes ARD emission
	// the default and supplies the publisher domain, so `compile -> pushable
	// ai-catalog.json` needs no flags (ADR-0018).
	project, projErr := resource.LoadProject(source)
	if projErr != nil {
		return nil, projErr
	}
	if project != nil && strings.TrimSpace(project.Publisher) != "" {
		emitArd = true
		if publisherDomain == "" {
			publisherDomain = project.Publisher
		}
	}
	if emitArd && publisherDomain == "" {
		return nil, resource.Errorf("--emit-ard requires --publisher-domain (or a `publisher` in typeference.yaml)")
	}
	if !emitArd && publisherDomain != "" {
		return nil, resource.Errorf("--publisher-domain requires --emit-ard")
	}
	if !emitArd && trustConfig != "" {
		return nil, resource.Errorf("--trust-config requires --emit-ard")
	}
	if !emitArd && trustSignatures != "" {
		return nil, resource.Errorf("--trust-signatures requires --emit-ard")
	}
	if !emitArd && allowUnsignedTrust {
		return nil, resource.Errorf("--allow-unsigned-trust requires --emit-ard")
	}
	if !emitArd {
		return nil, nil
	}
	return &compile.ArdPublicationOptions{
		PublisherDomain:     publisherDomain,
		TrustConfigPath:     trustConfig,
		TrustSignaturesPath: trustSignatures,
		AllowUnsignedTrust:  allowUnsignedTrust,
	}, nil
}

func requiredArg(args []string, index int, name string) (string, error) {
	if len(args) > index {
		return args[index], nil
	}
	return "", resource.Errorf("Missing %s", name)
}

func option(args []string, name string) (string, error) {
	for i, arg := range args {
		if arg == name {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", resource.Errorf("%s requires a value", name)
			}
			return args[i+1], nil
		}
	}
	return "", nil
}

func fail(message string) int {
	fmt.Fprintf(os.Stderr, "typeference: %s\n", message)
	return 2
}

func help() int {
	fmt.Print(`TypeFerence - typed coherence for AI agents (Go implementation)

Commands:
  typeference validate <source> [--trust-config path]
  typeference build <source> [--target all|neutral|codex|copilot|cursor] [--out dist]
      [--emit-ard --publisher-domain example.com] [--trust-config path]
      [--trust-signatures signatures.json]
      [--allow-unsigned-trust]
  typeference inspect <agent-id> [--source path]
  typeference diff <source> --against <compiled-dir> [--target all]
      [--emit-ard --publisher-domain example.com] [--trust-config path]
      [--trust-signatures signatures.json] [--json]
      [--allow-unsigned-trust]
  typeference publish <ard-dir> [--registry <url>]
      (deployment edge, not core: summarizes the compiled ARD catalog; with
       --registry POSTs it to a registry. A dry run without --registry.)
  typeference eval <source> --scenarios <file-or-dir> [--live] [--model id] [--out dir]
      (dry run by default: validates scenarios and emits exact request
       payloads without calling any API; --live reads ANTHROPIC_API_KEY)
  typeference equivalence pack <source> --scenarios <file-or-dir> --out <run-dir>
      [--target all|<name>[,<name>...]]
      (lays out one cell per scenario x surface: compiled bundle, context,
       and prompt; an operator collects one host response per cell)
  typeference equivalence score <run-dir> [--live] [--model id]
      (judges collected responses and writes the equivalence scorecard;
       without --live it emits judge payloads and stays offline)
  typeference version
`)
	return 0
}

// Command typeference is the Go implementation of the TypeFerence CLI. It
// implements the same command surface as the C# reference implementation and
// produces byte-identical artifacts (verified by the shared conformance
// suite under conformance/).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

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
	ard, err := ardOptions(args)
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

	ard, err := ardOptions(args)
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

func stringArr(values []string) jsonx.Arr {
	arr := jsonx.Arr{}
	for _, v := range values {
		arr = append(arr, jsonx.Str(v))
	}
	return arr
}

func ardOptions(args []string) (*compile.ArdPublicationOptions, error) {
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
	if emitArd && publisherDomain == "" {
		return nil, resource.Errorf("--emit-ard requires --publisher-domain")
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
`)
	return 0
}

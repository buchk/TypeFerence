package main

import (
	"fmt"
	"slices"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/importer"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// importCommand turns existing GitHub Copilot customizations into a version 6
// package (ADR-0033) and validates the result through the ordinary compiler.
//
//	typeference import <source> --out <dir> [--name ns/name] [--version x.y.z]
//	    [--plugin name] [--lossy]
func importCommand(args []string) (int, error) {
	source, err := requiredArg(args, 1, "import source")
	if err != nil {
		return 0, err
	}
	out, err := option(args, "--out")
	if err != nil {
		return 0, err
	}
	if out == "" {
		return 0, resource.Errorf("--out is required")
	}
	name, err := option(args, "--name")
	if err != nil {
		return 0, err
	}
	version, err := option(args, "--version")
	if err != nil {
		return 0, err
	}
	plugin, err := option(args, "--plugin")
	if err != nil {
		return 0, err
	}
	result, err := importer.Import(source, importer.Options{
		Name: name, Version: version, Plugin: plugin, Lossy: slices.Contains(args, "--lossy"),
	})
	if err != nil {
		return 0, err
	}
	if err := importer.Write(result.Files, out); err != nil {
		return 0, err
	}
	for _, note := range result.Notes {
		fmt.Printf("note: %s\n", note)
	}
	summary, err := compile.Summarize(out, "", "")
	if err != nil {
		return 0, resource.Errorf("imported sources do not validate: %s", err)
	}
	fmt.Printf("Imported %d files at %s: %d agents, plugins %v.\n", len(result.Files), out, len(summary.Agents), summary.Plugins)
	return 0, nil
}

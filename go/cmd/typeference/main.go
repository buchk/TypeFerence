// Command typeference is the reference CLI. It produces deterministic Copilot
// plugin artifacts verified by the conformance corpus under conformance/.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/compile"
	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/packages"
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
	case "init":
		code, err = initCommand(args)
	case "import":
		code, err = importCommand(args)
	case "validate":
		code, err = validate(args)
	case "build":
		code, err = build(args)
	case "pack":
		code, err = pack(args)
	case "restore":
		code, err = restore(args, false)
	case "update":
		code, err = restore(args, true)
	case "inspect":
		code, err = inspect(args)
	case "diff":
		code, err = diff(args)
	case "link", "publish", "eval", "equivalence":
		return fail(fmt.Sprintf("the %s command was removed in version 7 (ADR-0035)", args[0]))
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
	packagesDir, err := option(args, "--packages-dir")
	if err != nil {
		return 0, err
	}
	candidate, err := option(args, "--candidate")
	if err != nil {
		return 0, err
	}
	summary, err := compile.Summarize(source, compile.BuildOptions{PackagesDir: packagesDir, Candidate: candidate})
	if err != nil {
		return 0, err
	}
	verdict := "Valid"
	if candidate != "" {
		verdict = "Candidate fits"
	}
	fmt.Printf("%s: %d agents resolved; %d plugin artifacts.\n", verdict, len(summary.Agents), len(summary.Plugins))
	printConflicts(summary.Conflicts)
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
	if target, err := option(args, "--target"); err != nil {
		return 0, err
	} else if err := compile.CheckTarget(target); err != nil {
		return 0, err
	}
	packagesDir, err := option(args, "--packages-dir")
	if err != nil {
		return 0, err
	}
	files, err := compile.Build(source, output, compile.BuildOptions{PackagesDir: packagesDir})
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
	reportPluginConflicts(filepath.Join(output, compile.TargetName, ".typeference", "compatibility.json"))
	return 0, nil
}

// reportPluginConflicts prints the build's compatibility report: plugins
// that ship different skills for one capability compete for the same
// requests when installed together (ADR-0031). It informs; it never fails a
// build, because a team may ship competing packs on purpose.
func reportPluginConflicts(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var report struct {
		Conflicts []struct {
			Mode         string `json:"mode"`
			CapabilityID string `json:"capabilityId"`
			Members      []struct {
				Plugin string `json:"plugin"`
				Skill  string `json:"skill"`
			} `json:"members"`
		} `json:"conflicts"`
	}
	if json.Unmarshal(data, &report) != nil {
		return
	}
	conflicts := []compile.Conflict{}
	for _, conflict := range report.Conflicts {
		members := []compile.ConflictMember{}
		for _, member := range conflict.Members {
			members = append(members, compile.ConflictMember{Plugin: member.Plugin, Skill: member.Skill})
		}
		conflicts = append(conflicts, compile.Conflict{Mode: conflict.Mode, CapabilityID: conflict.CapabilityID, Members: members})
	}
	printConflicts(conflicts)
}

// printConflicts reports plugins that compete when installed together.
func printConflicts(conflicts []compile.Conflict) {
	for _, conflict := range conflicts {
		members := make([]string, 0, len(conflict.Members))
		for _, member := range conflict.Members {
			members = append(members, member.Plugin+"/"+member.Skill)
		}
		fmt.Printf("note: %s plugins compete for %s when installed together: %s\n",
			conflict.Mode, conflict.CapabilityID, strings.Join(members, ", "))
	}
}

func pack(args []string) (int, error) {
	source, err := requiredArg(args, 1, "source")
	if err != nil {
		return 0, err
	}
	output, err := option(args, "--out")
	if err != nil {
		return 0, err
	}
	digest, err := packages.Pack(source, output)
	if err != nil {
		return 0, err
	}
	fmt.Printf("Packed %s\n", digest)
	return 0, nil
}

func restore(args []string, update bool) (int, error) {
	source, err := requiredArg(args, 1, "source")
	if err != nil {
		return 0, err
	}
	feeds, err := option(args, "--feeds")
	if err != nil {
		return 0, err
	}
	config, err := packages.LoadFeedConfig(feeds)
	if err != nil {
		return 0, err
	}
	packagesDir, err := option(args, "--packages-dir")
	if err != nil {
		return 0, err
	}
	locked := slices.Contains(args, "--locked")
	if update && locked {
		return 0, resource.Errorf("update cannot be combined with --locked")
	}
	if !update && !locked {
		existing, err := packages.LoadLock(source)
		if err != nil {
			return 0, err
		}
		locked = existing != nil
	}
	restorer := packages.Restorer{
		Source: source, PackagesDir: packagesDir, Config: config, Locked: locked,
	}
	lock, err := restorer.Restore()
	if err != nil {
		return 0, err
	}
	fmt.Printf("Restored %d packages%s.\n", len(lock.Packages), map[bool]string{true: " and updated the lockfile", false: ""}[update])
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
	packagesDir, err := option(args, "--packages-dir")
	if err != nil {
		return 0, err
	}
	agents, err := compile.ValidateWithPackages(source, packagesDir)
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

	packagesDir, err := option(args, "--packages-dir")
	if err != nil {
		return 0, err
	}
	if _, err := compile.Build(source, temp, compile.BuildOptions{PackagesDir: packagesDir}); err != nil {
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
	fmt.Print(`TypeFerence - authoring and reuse for GitHub Copilot plugins (Go implementation)

Commands:
  typeference init --answers <answers.json> [--out DIR] [--verify sha256:...]
      (scaffolds a starter package from a versioned answer set)
  typeference import <copilot-source> --out <dir> [--name ns/name]
      [--version x.y.z] [--plugin name] [--lossy]
      (turns existing Copilot custom agents, Agent Skills, or an Agent Plugin
       into a version 7 package; fails on anything it cannot represent
       unless --lossy)
  typeference validate <source> [--packages-dir obj/typeference/packages]
      [--candidate <package-dir>]
      (--candidate checks an unpublished package against a marketplace
       package's locked graph without writing anything)
  typeference pack <source> [--out package.tferpkg]
  typeference restore <source> --feeds <external-config> [--locked]
      [--packages-dir obj/typeference/packages]
  typeference update <source> --feeds <external-config>
      [--packages-dir obj/typeference/packages]
  typeference build <source> [--out dist] [--packages-dir obj/typeference/packages]
      (writes <out>/agent-plugin: a GitHub Copilot plugin marketplace root)
  typeference inspect <agent-id> [--source path] [--packages-dir dir]
  typeference diff <source> --against <compiled-dir> [--json]
      [--packages-dir obj/typeference/packages]
  typeference version
`)
	return 0
}

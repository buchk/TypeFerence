package packages

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// DependencySet is the verified locked dependency graph of a
// package: every locked package's documents, its lock entries, and each
// package's exported identities and own plugins.
type DependencySet struct {
	Documents map[string]*resource.Document
	Locked    []LockedPackage
	Exports   map[string][]string
	// Plugins are the identities of each package's own plugins: the plugins
	// a dependent may ship (ADR-0034).
	Plugins map[string][]string
	// Candidate is the unpublished package the set was loaded with, if any.
	Candidate *resource.Project
}

// LoadDependencySet verifies and loads a package's committed locked graph from
// the materialized package directory. It performs no network access.
func LoadDependencySet(source, packagesDir string) (*DependencySet, error) {
	return loadDependencySet(source, packagesDir, "")
}

// LoadDependencySetWithCandidate loads a package's locked graph with an
// unpublished candidate package, read from its source directory, in place of
// the locked package of the same name, or added beside the graph when the
// package does not depend on it yet (ADR-0034). The candidate's dependencies
// must be locked at the versions it declares, and every locked package that
// depends on the candidate's name must declare the candidate's version. The
// result is for validation only: the candidate has no published digest.
func LoadDependencySetWithCandidate(source, packagesDir, candidateDir string) (*DependencySet, error) {
	if strings.TrimSpace(candidateDir) == "" {
		return nil, resource.Errorf("a candidate package directory is required")
	}
	return loadDependencySet(source, packagesDir, candidateDir)
}

func loadDependencySet(source, packagesDir, candidateDir string) (*DependencySet, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return nil, err
	}
	lock, err := LoadLock(source)
	if err != nil {
		return nil, err
	}
	set := &DependencySet{
		Documents: map[string]*resource.Document{},
		Exports:   map[string][]string{},
		Plugins:   map[string][]string{},
	}
	if project == nil {
		if lock != nil {
			return nil, resource.Errorf("%s requires %s", LockFile, resource.ManifestFile)
		}
		if candidateDir != "" {
			return nil, resource.Errorf("%s has no %s to validate a candidate against", source, resource.ManifestFile)
		}
		return set, nil
	}
	if err := validateProjectLock(project, lock, len(project.Dependencies) > 0); err != nil {
		return nil, err
	}
	lockedPackages := []LockedPackage{}
	if lock != nil {
		lockedPackages = lock.Packages
	}
	var candidate *resource.Project
	if candidateDir != "" {
		candidate, err = checkCandidate(project, lockedPackages, candidateDir)
		if err != nil {
			return nil, err
		}
		set.Candidate = candidate
	}
	if packagesDir == "" {
		packagesDir = filepath.Join(source, "obj", "typeference", "packages")
	}
	seenPackages := map[string]bool{}
	for _, item := range lockedPackages {
		if seenPackages[item.Name] {
			return nil, resource.Errorf("%s contains duplicate package %s", LockFile, item.Name)
		}
		seenPackages[item.Name] = true
		if candidate != nil && item.Name == candidate.Name {
			continue
		}
		root := filepath.Join(packagesDir, filepath.FromSlash(item.Name), item.Version,
			strings.TrimPrefix(item.Digest, "sha256:"), "source")
		files, err := SourceFiles(root)
		if err != nil {
			return nil, resource.Errorf("locked package %s is not materialized; run typeference restore --locked", item.Name)
		}
		archive := Archive{
			SchemaVersion: 1, Name: item.Name, Version: item.Version,
			Dependencies: item.Dependencies, Exports: item.Exports, Files: files,
		}
		if Digest(EncodeArchive(archive)) != item.Digest {
			return nil, resource.Errorf("materialized package %s does not match locked digest", item.Name)
		}
		loaded, err := resource.LoadPackage(root, resource.PackageOptions{Dependencies: item.Dependencies})
		if err != nil {
			return nil, resource.Errorf("locked package %s: %s", item.Name, err)
		}
		if strings.Join(loaded.Exports, "\x00") != strings.Join(item.Exports, "\x00") {
			return nil, resource.Errorf("materialized package %s exports differ from the lockfile", item.Name)
		}
		if err := set.add(loaded.Documents); err != nil {
			return nil, err
		}
		set.Exports[item.Name] = append([]string{}, item.Exports...)
		set.Plugins[item.Name] = loaded.OwnPlugins
		set.Locked = append(set.Locked, item)
	}
	if candidate != nil {
		loaded, err := resource.LoadPackage(candidateDir, resource.PackageOptions{Dependencies: candidate.Dependencies})
		if err != nil {
			return nil, resource.Errorf("candidate %s: %s", candidate.Name, err)
		}
		for _, pkg := range sortedMapKeys(candidate.Dependencies) {
			exported := map[string]bool{}
			for _, id := range set.Exports[pkg] {
				exported[id] = true
			}
			for _, id := range loaded.Qualified[pkg] {
				if !exported[id] {
					return nil, resource.Errorf("candidate %s references %s, which package %s does not export", candidate.Name, id, pkg)
				}
			}
		}
		if err := set.add(loaded.Documents); err != nil {
			return nil, err
		}
		set.Exports[candidate.Name] = loaded.Exports
		set.Plugins[candidate.Name] = loaded.OwnPlugins
		set.Locked = append(set.Locked, LockedPackage{
			Name: candidate.Name, Version: candidate.Version,
			Dependencies: candidate.Dependencies, Exports: loaded.Exports,
		})
		sort.Slice(set.Locked, func(i, j int) bool { return set.Locked[i].Name < set.Locked[j].Name })
	}
	return set, nil
}

func (set *DependencySet) add(documents map[string]*resource.Document) error {
	for id, document := range documents {
		if _, duplicate := set.Documents[id]; duplicate {
			return resource.Errorf("dependency resource id conflict: %s", id)
		}
		set.Documents[id] = document
	}
	return nil
}

// checkCandidate reads a candidate package's manifest and checks that it fits
// the locked graph: every dependency it declares is locked at that version,
// and every locked package that depends on it declares its version.
func checkCandidate(project *resource.Project, locked []LockedPackage, candidateDir string) (*resource.Project, error) {
	candidate, err := resource.LoadProject(candidateDir)
	if err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, resource.Errorf("candidate %s has no %s", candidateDir, resource.ManifestFile)
	}
	if candidate.Name == project.Name {
		return nil, resource.Errorf("candidate %s is the package being validated, not one of its dependencies", candidate.Name)
	}
	byName := map[string]LockedPackage{}
	for _, item := range locked {
		byName[item.Name] = item
	}
	for _, name := range candidate.SortedDependencies() {
		version := candidate.Dependencies[name]
		item, ok := byName[name]
		if !ok {
			return nil, resource.Errorf("candidate %s depends on %s@%s, which %s does not lock; publish it and add it to %s first",
				candidate.Name, name, version, project.Name, project.Name)
		}
		if item.Version != version {
			return nil, resource.Errorf("candidate %s depends on %s@%s, but %s locks %s@%s",
				candidate.Name, name, version, project.Name, name, item.Version)
		}
	}
	for _, item := range locked {
		if item.Name == candidate.Name {
			continue
		}
		if version, ok := item.Dependencies[candidate.Name]; ok && version != candidate.Version {
			return nil, resource.Errorf("%s@%s depends on %s@%s; the candidate is %s@%s, so %s must move with it",
				item.Name, item.Version, candidate.Name, version, candidate.Name, candidate.Version, item.Name)
		}
	}
	return candidate, nil
}

func validateProjectLock(project *resource.Project, lock *Lock, required bool) error {
	if lock == nil {
		if required {
			return resource.Errorf("dependencies are declared but %s is missing; run typeference restore", LockFile)
		}
		return nil
	}
	if lock.Root != project.Name || lock.RootVersion != project.Version ||
		!sameDependencies(project.Dependencies, directDependencies(lock.Packages, project.Dependencies)) {
		return resource.Errorf("%s does not match %s; run typeference restore", LockFile, project.File)
	}
	if err := validateLockedGraph(lock, project.Dependencies); err != nil {
		return err
	}
	return nil
}

func validateLockedGraph(lock *Lock, roots map[string]string) error {
	items := map[string]LockedPackage{}
	for _, item := range lock.Packages {
		items[item.Name] = item
	}
	visited := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string, string) error
	visit = func(name, version string) error {
		item, exists := items[name]
		if !exists || item.Version != version {
			return resource.Errorf("%s does not contain required package %s@%s", LockFile, name, version)
		}
		if visiting[name] {
			return resource.Errorf("%s contains a dependency cycle at %s", LockFile, name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for dependency, requiredVersion := range item.Dependencies {
			if err := visit(dependency, requiredVersion); err != nil {
				return err
			}
		}
		delete(visiting, name)
		visited[name] = true
		return nil
	}
	for name, version := range roots {
		if err := visit(name, version); err != nil {
			return err
		}
	}
	if len(visited) != len(items) {
		return resource.Errorf("%s contains packages unreachable from the project dependencies", LockFile)
	}
	return nil
}

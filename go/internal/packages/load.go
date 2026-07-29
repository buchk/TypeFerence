package packages

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// LoadDependencies verifies and loads the committed locked graph from the
// materialized package directory. It performs no network access.
func LoadDependencies(source, packagesDir string) (map[string]*resource.Document, []LockedPackage, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return nil, nil, err
	}
	if project == nil || len(project.Dependencies) == 0 {
		return map[string]*resource.Document{}, nil, nil
	}
	lock, err := LoadLock(source)
	if err != nil {
		return nil, nil, err
	}
	if lock == nil {
		return nil, nil, resource.Errorf("dependencies are declared but %s is missing; run typeference restore", LockFile)
	}
	if lock.Root != project.Name || lock.RootVersion != project.Version ||
		!sameDependencies(project.Dependencies, directDependencies(lock.Packages, project.Dependencies)) {
		return nil, nil, resource.Errorf("%s does not match %s; run typeference restore", LockFile, resource.ProjectManifestFile)
	}
	if err := validateLockedGraph(lock, project.Dependencies); err != nil {
		return nil, nil, err
	}
	if packagesDir == "" {
		packagesDir = filepath.Join(source, "obj", "typeference", "packages")
	}
	all := map[string]*resource.Document{}
	seenPackages := map[string]bool{}
	for _, item := range lock.Packages {
		if seenPackages[item.Name] {
			return nil, nil, resource.Errorf("%s contains duplicate package %s", LockFile, item.Name)
		}
		seenPackages[item.Name] = true
		root := filepath.Join(packagesDir, filepath.FromSlash(item.Name), item.Version,
			strings.TrimPrefix(item.Digest, "sha256:"), "source")
		files, err := SourceFiles(root)
		if err != nil {
			return nil, nil, resource.Errorf("locked package %s is not materialized; run typeference restore --locked", item.Name)
		}
		archive := Archive{
			SchemaVersion: 1, Name: item.Name, Version: item.Version,
			Dependencies: item.Dependencies, Exports: item.Exports, Files: files,
		}
		if Digest(EncodeArchive(archive)) != item.Digest {
			return nil, nil, resource.Errorf("materialized package %s does not match locked digest", item.Name)
		}
		documents, err := resource.Load(root, "")
		if err != nil {
			return nil, nil, err
		}
		actualExports := make([]string, 0, len(documents))
		for id := range documents {
			actualExports = append(actualExports, id)
		}
		sort.Strings(actualExports)
		if strings.Join(actualExports, "\x00") != strings.Join(item.Exports, "\x00") {
			return nil, nil, resource.Errorf("materialized package %s exports differ from the lockfile", item.Name)
		}
		for id, document := range documents {
			if _, duplicate := all[id]; duplicate {
				return nil, nil, resource.Errorf("dependency resource id conflict: %s", id)
			}
			all[id] = document
		}
	}
	return all, append([]LockedPackage{}, lock.Packages...), nil
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

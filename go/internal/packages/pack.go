package packages

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// SourceFiles returns a package's explicit source resource set. It is shared
// by pack and source digesting so output and cache directories cannot
// contaminate identity.
//
// A package's members are its manifest, its lockfile when present, the
// documents in the closure of its plugins and exports, and the files its
// member skills ship. Unreferenced files are not members.
func SourceFiles(source string) ([]File, error) {
	root, err := filepath.Abs(source)
	if err != nil {
		return nil, resource.Errorf("Source directory not found: %s", source)
	}
	project, err := resource.LoadProject(root)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, resource.Errorf("%s has no %s", root, resource.ManifestFile)
	}
	loaded, err := resource.LoadPackage(root, resource.PackageOptions{Dependencies: project.Dependencies})
	if err != nil {
		return nil, err
	}
	memberFiles := map[string]bool{}
	for _, rel := range loaded.MemberFiles {
		memberFiles[rel] = true
	}
	paths := append(append([]string{}, loaded.Files...), loaded.MemberFiles...)
	paths = append(paths, resource.ManifestFile)
	if info, statErr := os.Stat(filepath.Join(root, LockFile)); statErr == nil && info.Mode().IsRegular() {
		paths = append(paths, LockFile)
	}
	sort.Strings(paths)
	files := make([]File, 0, len(paths))
	for i, rel := range paths {
		if i > 0 && paths[i-1] == rel {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if readErr != nil {
			return nil, resource.Errorf("Cannot read source file: %s", rel)
		}
		if memberFiles[rel] && !utf8.Valid(data) {
			files = append(files, File{Path: rel, Content: base64.StdEncoding.EncodeToString(data), Encoding: "base64"})
			continue
		}
		files = append(files, File{Path: rel, Content: resource.NormalizeText(string(data))})
	}
	return files, nil
}

// PackBytes builds a package's canonical source package in memory.
func PackBytes(source string) ([]byte, *resource.Project, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return nil, nil, err
	}
	if project == nil {
		return nil, nil, resource.Errorf("typeference pack requires a %s with schemaVersion 8", resource.ManifestFile)
	}
	lock, err := LoadLock(source)
	if err != nil {
		return nil, nil, err
	}
	if err := validateProjectLock(project, lock, len(project.Dependencies) > 0); err != nil {
		return nil, nil, resource.Errorf("typeference pack requires a lockfile matching declared dependencies: %s", err)
	}
	loaded, err := resource.LoadPackage(source, resource.PackageOptions{Dependencies: project.Dependencies})
	if err != nil {
		return nil, nil, err
	}
	files, err := SourceFiles(source)
	if err != nil {
		return nil, nil, err
	}
	archive := Archive{
		SchemaVersion: 1,
		Name:          project.Name, Version: project.Version,
		Dependencies: project.Dependencies, Exports: loaded.Exports, Files: files,
	}
	return EncodeArchive(archive), project, nil
}

// Pack writes a package's canonical source package. Its exports are exactly
// the documents its manifest exports.
func Pack(source, output string) (string, error) {
	data, project, err := PackBytes(source)
	if err != nil {
		return "", err
	}
	if output == "" {
		output = filepath.Join(source, project.Name[strings.LastIndex(project.Name, "/")+1:]+"-"+project.Version+".tferpkg")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return "", resource.Errorf("Cannot create package directory: %s", filepath.Dir(output))
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		return "", resource.Errorf("Cannot write package: %s", output)
	}
	return Digest(data), nil
}

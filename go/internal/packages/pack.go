package packages

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

var excludedSourceDirs = map[string]bool{
	".git": true, "dist": true, "bin": true, "obj": true,
}

// SourceFiles returns a package's explicit source resource set. It is shared
// by pack and source digesting so output and cache directories cannot
// contaminate identity.
//
// A version 6 package's members are its manifest, its lockfile and trust
// configuration when present, and exactly the documents in the closure of
// its plugins and exports (ADR-0030); unreferenced files are not members.
// Archival packages keep their original walk-based membership.
func SourceFiles(source string) ([]File, error) {
	root, err := filepath.Abs(source)
	if err != nil {
		return nil, resource.Errorf("Source directory not found: %s", source)
	}
	project, err := resource.LoadProject(root)
	if err != nil {
		return nil, err
	}
	if project.IsV6() {
		return sourceFilesV6(root, project)
	}
	return legacySourceFiles(root)
}

func sourceFilesV6(root string, project *resource.Project) ([]File, error) {
	loaded, err := resource.LoadV6(root, resource.V6Options{Dependencies: project.Dependencies})
	if err != nil {
		return nil, err
	}
	paths := append([]string{}, loaded.Files...)
	paths = append(paths, resource.ManifestFile)
	for _, optional := range []string{LockFile, "typeference.trust.tfer"} {
		if info, statErr := os.Stat(filepath.Join(root, optional)); statErr == nil && info.Mode().IsRegular() {
			paths = append(paths, optional)
		}
	}
	sort.Strings(paths)
	files := make([]File, 0, len(paths))
	for _, rel := range paths {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if readErr != nil {
			return nil, resource.Errorf("Cannot read source file: %s", rel)
		}
		content := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\uFEFF")
		files = append(files, File{Path: rel, Content: content})
	}
	return files, nil
}

func legacySourceFiles(root string) ([]File, error) {
	files := []File{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filepath.Dir(path) == root && excludedSourceDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		name := entry.Name()
		if name != resource.ProjectManifestFile && name != resource.ManifestFileNameV5 && name != LockFile && name != "typeference.trust.tfer" &&
			name != "typeference.yaml" && !strings.HasSuffix(name, ".tfer") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		content := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\uFEFF")
		files = append(files, File{Path: rel, Content: content})
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("Cannot enumerate source package: %s", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Pack writes a version 6 package's canonical source package. Its exports are
// exactly the resources its manifest exports.
func Pack(source, output string) (string, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return "", err
	}
	if project == nil || !project.IsV6() {
		return "", resource.Errorf("typeference pack requires a version 6 %s", resource.ManifestFile)
	}
	lock, err := LoadLock(source)
	if err != nil {
		return "", err
	}
	if err := validateProjectLock(project, lock, len(project.Dependencies) > 0); err != nil {
		return "", resource.Errorf("typeference pack requires a lockfile matching declared dependencies: %s", err)
	}
	loaded, err := resource.LoadV6(source, resource.V6Options{Dependencies: project.Dependencies})
	if err != nil {
		return "", err
	}
	files, err := SourceFiles(source)
	if err != nil {
		return "", err
	}
	archive := Archive{
		SchemaVersion: 1,
		Name:          project.Name, Version: project.Version,
		Dependencies: project.Dependencies, Exports: loaded.Exports, Files: files,
	}
	data := EncodeArchive(archive)
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

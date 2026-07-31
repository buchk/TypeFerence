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

// SourceFiles returns the explicit version 4 source resource set. It is shared
// by pack and source digesting so output/cache directories cannot contaminate
// identity.
func SourceFiles(source string) ([]File, error) {
	root, err := filepath.Abs(source)
	if err != nil {
		return nil, resource.Errorf("Source directory not found: %s", source)
	}
	files := []File{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
		if name != resource.ProjectManifestFile && name != LockFile && name != "typeference.trust.yaml" &&
			!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".tfer") {
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

func Pack(source, output string) (string, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return "", err
	}
	if project == nil {
		return "", resource.Errorf("typeference pack requires %s", resource.ProjectManifestFile)
	}
	lock, err := LoadLock(source)
	if err != nil {
		return "", err
	}
	if err := validateProjectLock(project, lock, len(project.Dependencies) > 0); err != nil {
		return "", resource.Errorf("typeference pack requires a lockfile matching declared dependencies: %s", err)
	}
	documents, err := resource.Load(source, "")
	if err != nil {
		return "", err
	}
	exports := make([]string, 0, len(documents))
	for id := range documents {
		exports = append(exports, id)
	}
	sort.Strings(exports)
	files, err := SourceFiles(source)
	if err != nil {
		return "", err
	}
	archive := Archive{
		SchemaVersion: 1,
		Name:          project.Name, Version: project.Version,
		Dependencies: project.Dependencies, Exports: exports, Files: files,
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

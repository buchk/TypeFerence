// Package packages implements deterministic TypeFerence source packages and
// locked, offline dependency materialization.
package packages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"gopkg.in/yaml.v3"
)

const LockFile = "typeference.lock"

var packageDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Archive struct {
	SchemaVersion int               `json:"schemaVersion"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Dependencies  map[string]string `json:"dependencies"`
	Exports       []string          `json:"exports"`
	Files         []File            `json:"files"`
}

type LockedPackage struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Digest       string            `json:"digest"`
	Dependencies map[string]string `json:"dependencies"`
	Exports      []string          `json:"exports"`
}

type Lock struct {
	SchemaVersion int             `json:"schemaVersion"`
	Root          string          `json:"root"`
	RootVersion   string          `json:"rootVersion"`
	Packages      []LockedPackage `json:"packages"`
}

func EncodeArchive(archive Archive) []byte {
	dependencies := jsonx.Obj{}
	for _, name := range sortedMapKeys(archive.Dependencies) {
		dependencies = append(dependencies, jsonx.Member{K: name, V: jsonx.Str(archive.Dependencies[name])})
	}
	exports := jsonx.Arr{}
	for _, id := range archive.Exports {
		exports = append(exports, jsonx.Str(id))
	}
	files := jsonx.Arr{}
	for _, file := range archive.Files {
		files = append(files, jsonx.Obj{
			{K: "path", V: jsonx.Str(file.Path)},
			{K: "content", V: jsonx.Str(file.Content)},
		})
	}
	return []byte(jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "name", V: jsonx.Str(archive.Name)},
		{K: "version", V: jsonx.Str(archive.Version)},
		{K: "dependencies", V: dependencies},
		{K: "exports", V: exports},
		{K: "files", V: files},
	}) + "\n")
}

func DecodeArchive(data []byte) (*Archive, error) {
	var archive Archive
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&archive); err != nil {
		return nil, resource.Errorf("invalid TypeFerence package: %s", err)
	}
	if archive.SchemaVersion != 1 || !resource.IsPackageName(archive.Name) || !resource.IsSemanticVersion(archive.Version) {
		return nil, resource.Errorf("invalid TypeFerence package identity")
	}
	if err := validateDependencies(archive.Dependencies); err != nil {
		return nil, err
	}
	if err := validateExports(archive.Exports); err != nil {
		return nil, err
	}
	last := ""
	for _, file := range archive.Files {
		clean := filepath.ToSlash(filepath.Clean(file.Path))
		if clean != file.Path || clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(file.Path) {
			return nil, resource.Errorf("package contains unsafe path: %s", file.Path)
		}
		if last != "" && file.Path <= last {
			return nil, resource.Errorf("package files are not in unique canonical order")
		}
		last = file.Path
	}
	if err := validateArchiveManifest(&archive); err != nil {
		return nil, err
	}
	if string(EncodeArchive(archive)) != string(data) {
		return nil, resource.Errorf("package is not canonically serialized")
	}
	return &archive, nil
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func EncodeLock(lock Lock) []byte {
	packages := jsonx.Arr{}
	for _, item := range lock.Packages {
		dependencies := jsonx.Obj{}
		for _, name := range sortedMapKeys(item.Dependencies) {
			dependencies = append(dependencies, jsonx.Member{K: name, V: jsonx.Str(item.Dependencies[name])})
		}
		exports := jsonx.Arr{}
		for _, id := range item.Exports {
			exports = append(exports, jsonx.Str(id))
		}
		packages = append(packages, jsonx.Obj{
			{K: "name", V: jsonx.Str(item.Name)},
			{K: "version", V: jsonx.Str(item.Version)},
			{K: "digest", V: jsonx.Str(item.Digest)},
			{K: "dependencies", V: dependencies},
			{K: "exports", V: exports},
		})
	}
	return []byte(jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "root", V: jsonx.Str(lock.Root)},
		{K: "rootVersion", V: jsonx.Str(lock.RootVersion)},
		{K: "packages", V: packages},
	}) + "\n")
}

func LoadLock(source string) (*Lock, error) {
	data, err := os.ReadFile(filepath.Join(source, LockFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, resource.Errorf("%s: %s", LockFile, err)
	}
	var lock Lock
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lock); err != nil {
		return nil, resource.Errorf("%s: invalid lockfile: %s", LockFile, err)
	}
	if lock.SchemaVersion != 1 || string(EncodeLock(lock)) != string(data) {
		return nil, resource.Errorf("%s: lockfile is not canonically serialized", LockFile)
	}
	if !resource.IsPackageName(lock.Root) || !resource.IsSemanticVersion(lock.RootVersion) {
		return nil, resource.Errorf("%s: invalid root package identity", LockFile)
	}
	lastName := ""
	for _, item := range lock.Packages {
		if !resource.IsPackageName(item.Name) || !resource.IsSemanticVersion(item.Version) ||
			!packageDigest.MatchString(item.Digest) {
			return nil, resource.Errorf("%s: invalid locked package identity for %s", LockFile, item.Name)
		}
		if lastName != "" && item.Name <= lastName {
			return nil, resource.Errorf("%s: packages must be in unique canonical order", LockFile)
		}
		lastName = item.Name
		if err := validateDependencies(item.Dependencies); err != nil {
			return nil, resource.Errorf("%s: package %s: %s", LockFile, item.Name, err)
		}
		if err := validateExports(item.Exports); err != nil {
			return nil, resource.Errorf("%s: package %s: %s", LockFile, item.Name, err)
		}
	}
	return &lock, nil
}

func validateDependencies(dependencies map[string]string) error {
	for name, version := range dependencies {
		if !resource.IsPackageName(name) {
			return resource.Errorf("invalid dependency package name: %s", name)
		}
		if !resource.IsSemanticVersion(version) {
			return resource.Errorf("dependency %s must use an exact semantic version", name)
		}
	}
	return nil
}

func validateExports(exports []string) error {
	if !sort.StringsAreSorted(exports) {
		return resource.Errorf("package exports are not in canonical order")
	}
	last := ""
	for _, id := range exports {
		if !resource.IsResourceID(id) {
			return resource.Errorf("invalid exported resource id: %s", id)
		}
		if id == last {
			return resource.Errorf("package exports are not unique")
		}
		last = id
	}
	return nil
}

func validateArchiveManifest(archive *Archive) error {
	var manifest *File
	for i := range archive.Files {
		if archive.Files[i].Path == resource.ProjectManifestFile {
			manifest = &archive.Files[i]
			break
		}
	}
	if manifest == nil {
		return resource.Errorf("package is missing %s", resource.ProjectManifestFile)
	}
	var project struct {
		SchemaVersion int               `yaml:"schemaVersion"`
		Name          string            `yaml:"name"`
		Version       string            `yaml:"version"`
		Publisher     string            `yaml:"publisher"`
		Dependencies  map[string]string `yaml:"dependencies"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(manifest.Content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&project); err != nil || project.SchemaVersion != 2 {
		return resource.Errorf("package contains an invalid %s", resource.ProjectManifestFile)
	}
	if project.Name != archive.Name || project.Version != archive.Version ||
		!sameDependencies(project.Dependencies, archive.Dependencies) {
		return resource.Errorf("package envelope does not match %s", resource.ProjectManifestFile)
	}
	return nil
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

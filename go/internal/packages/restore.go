package packages

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"gopkg.in/yaml.v3"
)

type Route struct {
	Kind                  string `yaml:"kind"`
	Path                  string `yaml:"path"`
	BaseURL               string `yaml:"baseUrl"`
	CredentialEnvironment string `yaml:"credentialEnvironment"`
	Organization          string `yaml:"organization"`
	Project               string `yaml:"project"`
	Feed                  string `yaml:"feed"`
	// Git routes: an HTTPS repository URL, an optional package root within
	// the repository, and the prefix of version tags.
	URL       string `yaml:"url"`
	Root      string `yaml:"root"`
	TagPrefix string `yaml:"tagPrefix"`
}

type FeedConfig struct {
	SchemaVersion int              `yaml:"schemaVersion"`
	Routes        map[string]Route `yaml:"routes"`
}

type Restorer struct {
	Source      string
	PackagesDir string
	Config      *FeedConfig
	Locked      bool
	client      *http.Client
}

func LoadFeedConfig(path string) (*FeedConfig, error) {
	if strings.TrimSpace(path) == "" {
		return nil, resource.Errorf("restore requires --feeds <external-feed-config>")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, resource.Errorf("Cannot read feed configuration: %s", path)
	}
	var config FeedConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, resource.Errorf("Invalid feed configuration: %s", err)
	}
	if config.SchemaVersion != 1 || len(config.Routes) == 0 {
		return nil, resource.Errorf("feed configuration must use schemaVersion 1 and declare routes")
	}
	for prefix, route := range config.Routes {
		if !resource.IsPackageName(prefix + "/placeholder") {
			return nil, resource.Errorf("invalid feed route prefix: %s", prefix)
		}
		gitFields := route.URL != "" || route.Root != "" || route.TagPrefix != ""
		if route.Kind != "git" && gitFields {
			return nil, resource.Errorf("route %s: url, root, and tagPrefix apply only to git routes", prefix)
		}
		switch route.Kind {
		case "git":
			parsed, err := url.Parse(route.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
				route.Path != "" || route.BaseURL != "" || route.CredentialEnvironment != "" ||
				route.Organization != "" || route.Project != "" || route.Feed != "" {
				return nil, resource.Errorf("git route %s requires an HTTPS url without credentials", prefix)
			}
			if route.Root != "" && (strings.HasPrefix(route.Root, "/") || strings.Contains(route.Root, "..") || strings.Contains(route.Root, "\\")) {
				return nil, resource.Errorf("git route %s: root must be a clean path within the repository", prefix)
			}
		case "filesystem":
			if route.Path == "" || route.BaseURL != "" || route.CredentialEnvironment != "" ||
				route.Organization != "" || route.Project != "" || route.Feed != "" {
				return nil, resource.Errorf("filesystem route %s requires path", prefix)
			}
		case "http", "jfrog":
			parsed, err := url.Parse(route.BaseURL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
				route.Path != "" || route.Organization != "" || route.Project != "" || route.Feed != "" {
				return nil, resource.Errorf("%s route %s requires an absolute HTTPS baseUrl", route.Kind, prefix)
			}
		case "azureArtifacts":
			parsed, err := url.Parse(route.Organization)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || route.Feed == "" ||
				route.Path != "" || route.BaseURL != "" || route.CredentialEnvironment != "" {
				return nil, resource.Errorf("azureArtifacts route %s requires HTTPS organization and feed", prefix)
			}
		default:
			return nil, resource.Errorf("unsupported feed route kind %q", route.Kind)
		}
	}
	return &config, nil
}

func (r *Restorer) Restore() (*Lock, error) {
	project, err := resource.LoadProject(r.Source)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, resource.Errorf("restore requires %s", resource.ManifestFile)
	}
	if r.PackagesDir == "" {
		r.PackagesDir = filepath.Join(r.Source, "obj", "typeference", "packages")
	}
	if r.client == nil {
		r.client = http.DefaultClient
	}
	var existing *Lock
	if r.Locked {
		existing, err = LoadLock(r.Source)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, resource.Errorf("--locked requires %s", LockFile)
		}
		if existing.Root != project.Name || existing.RootVersion != project.Version {
			return nil, resource.Errorf("%s root identity does not match %s", LockFile, resource.ManifestFile)
		}
		if !sameDependencies(project.Dependencies, directDependencies(existing.Packages, project.Dependencies)) {
			return nil, resource.Errorf("%s does not match manifest dependencies", LockFile)
		}
		if err := validateLockedGraph(existing, project.Dependencies); err != nil {
			return nil, err
		}
	}

	resolved := map[string]LockedPackage{}
	visiting := map[string]bool{}
	var visit func(string, string) error
	visit = func(name, version string) error {
		if prior, exists := resolved[name]; exists {
			if prior.Version != version {
				return resource.Errorf("dependency conflict: %s requested as both %s and %s", name, prior.Version, version)
			}
			return nil
		}
		if visiting[name] {
			return resource.Errorf("dependency cycle detected at %s", name)
		}
		visiting[name] = true
		defer delete(visiting, name)

		data, err := r.fetch(name, version)
		if err != nil {
			return err
		}
		archive, err := DecodeArchive(data)
		if err != nil {
			return resource.Errorf("%s@%s: %s", name, version, err)
		}
		if archive.Name != name || archive.Version != version {
			return resource.Errorf("feed returned %s@%s while resolving %s@%s", archive.Name, archive.Version, name, version)
		}
		digest := Digest(data)
		if r.Locked {
			expected := findLocked(existing.Packages, name)
			if expected == nil || expected.Version != version || expected.Digest != digest {
				return resource.Errorf("%s@%s does not match its locked digest", name, version)
			}
		}
		for _, dependency := range sortedMapKeys(archive.Dependencies) {
			if err := visit(dependency, archive.Dependencies[dependency]); err != nil {
				return err
			}
		}
		item := LockedPackage{
			Name: name, Version: version, Digest: digest,
			Dependencies: archive.Dependencies, Exports: append([]string{}, archive.Exports...),
		}
		resolved[name] = item
		return r.materialize(archive, digest)
	}
	for _, name := range project.SortedDependencies() {
		if err := visit(name, project.Dependencies[name]); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)
	lock := &Lock{SchemaVersion: 1, Root: project.Name, RootVersion: project.Version}
	for _, name := range names {
		lock.Packages = append(lock.Packages, resolved[name])
	}
	if r.Locked {
		if string(EncodeLock(*lock)) != string(EncodeLock(*existing)) {
			return nil, resource.Errorf("restored graph differs from %s", LockFile)
		}
		if err := r.writeResolution(lock); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if err := os.WriteFile(filepath.Join(r.Source, LockFile), EncodeLock(*lock), 0o644); err != nil {
		return nil, resource.Errorf("Cannot write %s", LockFile)
	}
	if err := r.writeResolution(lock); err != nil {
		return nil, err
	}
	return lock, nil
}

func (r *Restorer) writeResolution(lock *Lock) error {
	items := jsonx.Arr{}
	for _, item := range lock.Packages {
		sourcePath := filepath.ToSlash(filepath.Join(filepath.FromSlash(item.Name), item.Version,
			strings.TrimPrefix(item.Digest, "sha256:"), "source"))
		items = append(items, jsonx.Obj{
			{K: "name", V: jsonx.Str(item.Name)},
			{K: "version", V: jsonx.Str(item.Version)},
			{K: "digest", V: jsonx.Str(item.Digest)},
			{K: "source", V: jsonx.Str(sourcePath)},
		})
	}
	content := jsonx.Indented(jsonx.Obj{
		{K: "schemaVersion", V: jsonx.Num("1")},
		{K: "packages", V: items},
	}) + "\n"
	path := filepath.Join(r.PackagesDir, "resolution.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return resource.Errorf("Cannot create package resolution directory")
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return resource.Errorf("Cannot write package resolution manifest")
	}
	return nil
}

func (r *Restorer) routeFor(name string) (Route, error) {
	best := ""
	var selected Route
	for prefix, route := range r.Config.Routes {
		if (name == prefix || strings.HasPrefix(name, prefix+"/")) && len(prefix) > len(best) {
			best, selected = prefix, route
		}
	}
	if best == "" {
		return Route{}, resource.Errorf("no feed route for package %s", name)
	}
	return selected, nil
}

func (r *Restorer) fetch(name, version string) ([]byte, error) {
	route, err := r.routeFor(name)
	if err != nil {
		return nil, err
	}
	leaf := name[strings.LastIndex(name, "/")+1:] + "-" + version + ".tferpkg"
	switch route.Kind {
	case "filesystem":
		return os.ReadFile(filepath.Join(route.Path, filepath.FromSlash(name), version, leaf))
	case "http", "jfrog":
		address := strings.TrimRight(route.BaseURL, "/") + "/" + name + "/" + version + "/" + leaf
		request, _ := http.NewRequest(http.MethodGet, address, nil)
		if route.CredentialEnvironment != "" {
			token := os.Getenv(route.CredentialEnvironment)
			if token == "" {
				return nil, resource.Errorf("credential environment variable %s is not set", route.CredentialEnvironment)
			}
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := r.client.Do(request)
		if err != nil {
			return nil, resource.Errorf("cannot restore %s@%s: %s", name, version, err)
		}
		defer response.Body.Close()
		if response.StatusCode >= 300 {
			return nil, resource.Errorf("cannot restore %s@%s: feed returned %s", name, version, response.Status)
		}
		return io.ReadAll(response.Body)
	case "azureArtifacts":
		return fetchAzureUniversal(route, name, version)
	case "git":
		return fetchGit(route, name, version)
	default:
		return nil, resource.Errorf("unsupported feed kind: %s", route.Kind)
	}
}

func fetchAzureUniversal(route Route, name, version string) ([]byte, error) {
	temp, err := os.MkdirTemp("", "typeference-azure-")
	if err != nil {
		return nil, resource.Errorf("cannot create Azure Artifacts staging directory")
	}
	defer os.RemoveAll(temp)
	packageName := strings.ReplaceAll(name, "/", "--")
	args := []string{"artifacts", "universal", "download", "--organization", route.Organization,
		"--feed", route.Feed, "--name", packageName, "--version", version, "--path", temp}
	if route.Project != "" {
		args = append(args, "--project", route.Project, "--scope", "project")
	}
	command := exec.Command("az", args...)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, resource.Errorf("Azure Artifacts download failed: %s", strings.TrimSpace(string(output)))
	}
	var packagePath string
	_ = filepath.WalkDir(temp, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tferpkg") {
			if packagePath == "" {
				packagePath = path
			} else {
				packagePath = "\x00"
			}
		}
		return nil
	})
	if packagePath == "" || packagePath == "\x00" {
		return nil, resource.Errorf("Azure Universal Package must contain exactly one .tferpkg file")
	}
	return os.ReadFile(packagePath)
}

// fetchGit resolves a package from a Git repository tag and packs it exactly
// as typeference pack would. Credentials come from Git's own configuration.
func fetchGit(route Route, name, version string) ([]byte, error) {
	temp, err := os.MkdirTemp("", "typeference-git-")
	if err != nil {
		return nil, resource.Errorf("cannot create Git staging directory")
	}
	defer os.RemoveAll(temp)
	tag := route.TagPrefix + version
	command := exec.Command("git", "clone", "--quiet", "--depth", "1", "--branch", tag, "--", route.URL, temp)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		return nil, resource.Errorf("cannot restore %s@%s from %s tag %s: %s", name, version, route.URL, tag, strings.TrimSpace(string(output)))
	}
	root := temp
	if route.Root != "" {
		root = filepath.Join(temp, filepath.FromSlash(route.Root))
	}
	data, project, err := PackBytes(root)
	if err != nil {
		return nil, resource.Errorf("%s@%s from %s tag %s: %s", name, version, route.URL, tag, err)
	}
	if project.Name != name || project.Version != version {
		return nil, resource.Errorf("%s tag %s holds %s@%s, not %s@%s", route.URL, tag, project.Name, project.Version, name, version)
	}
	return data, nil
}

func (r *Restorer) materialize(archive *Archive, digest string) error {
	hexDigest := strings.TrimPrefix(digest, "sha256:")
	root := filepath.Join(r.PackagesDir, filepath.FromSlash(archive.Name), archive.Version, hexDigest, "source")
	packagesRootAbs, err := filepath.Abs(r.PackagesDir)
	if err != nil {
		return resource.Errorf("invalid package materialization directory")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return resource.Errorf("invalid package materialization path")
	}
	relative, err := filepath.Rel(packagesRootAbs, rootAbs)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return resource.Errorf("package materialization path escapes package directory")
	}
	// The digest-qualified directory is derived output. Recreate it exactly so
	// an interrupted prior restore cannot leave undeclared source files behind.
	if err := os.RemoveAll(rootAbs); err != nil {
		return resource.Errorf("cannot reset package materialization directory")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return resource.Errorf("cannot create package materialization directory")
	}
	for _, file := range archive.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		full, _ := filepath.Abs(target)
		rootAbs, _ := filepath.Abs(root)
		if !strings.HasPrefix(full, rootAbs+string(filepath.Separator)) {
			return resource.Errorf("package path escapes materialization root: %s", file.Path)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return resource.Errorf("cannot materialize package path: %s", file.Path)
		}
		data, err := file.Bytes()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return resource.Errorf("cannot materialize package file: %s", file.Path)
		}
	}
	return nil
}

func findLocked(items []LockedPackage, name string) *LockedPackage {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func directDependencies(items []LockedPackage, declared map[string]string) map[string]string {
	result := map[string]string{}
	for name := range declared {
		if item := findLocked(items, name); item != nil {
			result[name] = item.Version
		}
	}
	return result
}

func sameDependencies(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

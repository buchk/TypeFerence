package packages

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// StageLocal packs a set of local package directories into a filesystem
// feed, in dependency order, restoring each package that has dependencies
// against the packages staged before it. It returns a feed configuration
// that routes every staged namespace to the feed, so a dependent package can
// then be restored from it. Restoring writes each package's lockfile, so
// callers stage copies, not authored sources they want left untouched.
func StageLocal(dirs []string, feed string) (*FeedConfig, error) {
	type staged struct {
		dir     string
		project *resource.Project
	}
	byName := map[string]staged{}
	for _, dir := range dirs {
		project, err := resource.LoadProject(dir)
		if err != nil {
			return nil, err
		}
		if project == nil {
			return nil, resource.Errorf("%s has no %s", dir, resource.ManifestFile)
		}
		if _, duplicate := byName[project.Name]; duplicate {
			return nil, resource.Errorf("package %s is staged more than once", project.Name)
		}
		byName[project.Name] = staged{dir: dir, project: project}
	}
	config := &FeedConfig{SchemaVersion: 1, Routes: map[string]Route{}}
	for name := range byName {
		namespace := strings.SplitN(name, "/", 2)[0]
		config.Routes[namespace] = Route{Kind: "filesystem", Path: feed}
	}
	order := []string{}
	visiting := map[string]bool{}
	done := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if done[name] {
			return nil
		}
		if visiting[name] {
			return resource.Errorf("staged packages contain a dependency cycle at %s", name)
		}
		visiting[name] = true
		item := byName[name]
		dependencies := item.project.SortedDependencies()
		for _, dependency := range dependencies {
			if _, local := byName[dependency]; local {
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		delete(visiting, name)
		done[name] = true
		order = append(order, name)
		return nil
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	for _, name := range order {
		item := byName[name]
		if len(item.project.Dependencies) > 0 {
			restorer := Restorer{Source: item.dir, Config: config}
			if _, err := restorer.Restore(); err != nil {
				return nil, resource.Errorf("staging %s: %s", name, err)
			}
		}
		leaf := name[strings.LastIndex(name, "/")+1:]
		output := filepath.Join(feed, filepath.FromSlash(name), item.project.Version, leaf+"-"+item.project.Version+".tferpkg")
		if _, err := Pack(item.dir, output); err != nil {
			return nil, resource.Errorf("staging %s: %s", name, err)
		}
	}
	return config, nil
}

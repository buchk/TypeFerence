package compile

import (
	"sort"

	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

// compilation is a loaded, resolved, and planned source package: everything
// the target writer needs, computed once.
type compilation struct {
	source     string
	project    *resource.Project
	docs       map[string]*resource.Document
	resolver   *resolve.Resolver
	resolved   []*resolve.ResolvedAgent
	plugins    []*pluginPlan
	provenance buildProvenance
}

type buildProvenance struct {
	SourceDigest string
	Dependencies []packages.LockedPackage
}

// prepare loads a package: the closure of its manifest's own plugins and
// exports, its locked dependencies, and the dependency plugins its manifest
// ships.
func prepare(source string, options BuildOptions) (*compilation, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, resource.Errorf("%s has no %s; TypeFerence builds sources whose manifest declares schemaVersion 7 (docs/specification.md)", source, resource.ManifestFile)
	}
	var dependencies *packages.DependencySet
	if options.Candidate != "" {
		dependencies, err = packages.LoadDependencySetWithCandidate(source, options.PackagesDir, options.Candidate)
	} else {
		dependencies, err = packages.LoadDependencySet(source, options.PackagesDir)
	}
	if err != nil {
		return nil, err
	}
	declared := project.Dependencies
	if candidate := dependencies.Candidate; candidate != nil {
		declared = map[string]string{}
		for name, version := range project.Dependencies {
			declared[name] = version
		}
		declared[candidate.Name] = candidate.Version
	}
	root, err := resource.LoadPackage(source, resource.PackageOptions{Dependencies: declared})
	if err != nil {
		return nil, err
	}
	shipped, err := shippedDependencyPlugins(project, root, dependencies)
	if err != nil {
		return nil, err
	}
	all := map[string]*resource.Document{}
	for id, doc := range root.Documents {
		all[id] = doc
	}
	for id, doc := range dependencies.Documents {
		if doc.Kind == "plugin" && !shipped[id] {
			continue // a dependency's plugins ship only where a manifest lists them
		}
		if _, exists := all[id]; exists {
			return nil, resource.Errorf("root document cannot shadow locked dependency document: %s", id)
		}
		all[id] = doc
	}
	for _, pkg := range resource.SortedKeys(root.Qualified) {
		exported := map[string]bool{}
		for _, id := range dependencies.Exports[pkg] {
			exported[id] = true
		}
		for _, id := range root.Qualified[pkg] {
			if !exported[id] {
				return nil, resource.Errorf("%s references %s, which package %s does not export", project.Name, id, pkg)
			}
		}
	}
	if err := resource.Normalize(all); err != nil {
		return nil, err
	}
	r, err := resolve.New(all)
	if err != nil {
		return nil, err
	}
	resolved, err := r.ResolveAll()
	if err != nil {
		return nil, err
	}
	sourceDigest, err := HashSource(source)
	if err != nil {
		return nil, err
	}
	c := &compilation{
		source:     source,
		project:    project,
		docs:       all,
		resolver:   r,
		resolved:   resolved,
		provenance: buildProvenance{SourceDigest: "sha256:" + sourceDigest, Dependencies: dependencies.Locked},
	}
	byID := map[string]*resolve.ResolvedAgent{}
	for _, agent := range resolved {
		byID[agent.ID] = agent
	}
	pluginIDs := append([]string{}, root.OwnPlugins...)
	for id := range shipped {
		pluginIDs = append(pluginIDs, id)
	}
	sort.Strings(pluginIDs)
	plugins, err := planPlugins(c, byID, pluginIDs, ownerProvenance(c, dependencies.Locked))
	if err != nil {
		return nil, err
	}
	c.plugins = plugins
	if err := validateLibraryNames(c); err != nil {
		return nil, err
	}
	if err := validateAgentTools(c); err != nil {
		return nil, err
	}
	return c, nil
}

// shippedDependencyPlugins returns the dependency plugins a package ships: the
// ones its manifest lists, each of which its owning package's manifest must
// list among its own plugins. When a candidate package is being validated,
// its plugins replace the ones the manifest lists from it.
func shippedDependencyPlugins(project *resource.Project, root *resource.Package, dependencies *packages.DependencySet) (map[string]bool, error) {
	listed := map[string]map[string]bool{}
	for pkg, ids := range dependencies.Plugins {
		listed[pkg] = map[string]bool{}
		for _, id := range ids {
			listed[pkg][id] = true
		}
	}
	shipped := map[string]bool{}
	candidate := dependencies.Candidate
	for _, plugin := range root.DependencyPlugins {
		if candidate != nil && plugin.Package == candidate.Name {
			continue
		}
		if !listed[plugin.Package][plugin.ID] {
			return nil, resource.Errorf("%s ships %s, which package %s does not list among its plugins", project.Name, plugin.ID, plugin.Package)
		}
		shipped[plugin.ID] = true
	}
	if candidate != nil {
		for _, id := range dependencies.Plugins[candidate.Name] {
			shipped[id] = true
		}
	}
	return shipped, nil
}

// ownerProvenance maps each package in the build to the provenance its plugin
// artifacts record: the building package's own, or a locked package's digest
// and the locked packages in its dependency closure.
func ownerProvenance(c *compilation, locked []packages.LockedPackage) map[string]buildProvenance {
	byName := map[string]packages.LockedPackage{}
	for _, item := range locked {
		byName[item.Name] = item
	}
	owners := map[string]buildProvenance{c.project.Name: c.provenance}
	for _, item := range locked {
		closure := map[string]bool{}
		var visit func(string)
		visit = func(name string) {
			for dependency := range byName[name].Dependencies {
				if !closure[dependency] {
					closure[dependency] = true
					visit(dependency)
				}
			}
		}
		visit(item.Name)
		dependencies := []packages.LockedPackage{}
		for _, other := range locked {
			if closure[other.Name] {
				dependencies = append(dependencies, other)
			}
		}
		owners[item.Name] = buildProvenance{SourceDigest: item.Digest, Dependencies: dependencies}
	}
	return owners
}

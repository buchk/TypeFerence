package compile

import (
	"sort"

	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"github.com/buchk/TypeFerence/go/internal/trust"
)

// compilation is a loaded, resolved, and planned source package: everything a
// target writer needs, computed once and shared by every target.
type compilation struct {
	source     string
	language   string
	project    *resource.Project
	resolver   *resolve.Resolver
	trust      *trust.Loaded
	resolved   []*resolve.ResolvedAgent // every agent in the build, by id
	agents     []*resolve.ResolvedAgent // agents the neutral target emits, by id
	plugins    []*pluginPlan            // version 6 plugins, by id
	provenance buildProvenance
}

type buildProvenance struct {
	SourceDigest string
	Dependencies []packages.LockedPackage
}

func (c *compilation) current() bool { return c.language == LanguageCurrent }

func prepare(source, trustConfigPath string, options BuildOptions) (*compilation, error) {
	loaded, err := trust.Load(source, trustConfigPath)
	if err != nil {
		return nil, err
	}
	switch options.Language {
	case LanguageCurrent:
		return prepareCurrent(source, loaded, options)
	case LanguageLegacyV5, LanguageLegacyV3:
		return prepareLegacy(source, loaded, options)
	}
	return nil, resource.Errorf("Unknown source language: %s", options.Language)
}

// prepareCurrent loads a version 6 package: the closure of its manifest's own
// plugins and exports, its locked dependencies, and the dependency plugins its
// manifest ships (ADR-0030, ADR-0034).
func prepareCurrent(source string, loaded *trust.Loaded, options BuildOptions) (*compilation, error) {
	project, err := resource.LoadProject(source)
	if err != nil {
		return nil, err
	}
	if !project.IsV6() {
		return nil, resource.Errorf("%s is not a version 6 package: TypeFerence builds sources whose %s declares schemaVersion 6 (docs/specification.md, ADR-0030)", source, resource.ManifestFile)
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
	candidate := dependencies.Candidate
	if candidate != nil {
		declared = map[string]string{}
		for name, version := range project.Dependencies {
			declared[name] = version
		}
		declared[candidate.Name] = candidate.Version
	}
	root, err := resource.LoadV6(source, resource.V6Options{Dependencies: declared})
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
			return nil, resource.Errorf("root resource cannot shadow locked dependency resource: %s", id)
		}
		all[id] = doc
	}
	for _, pkg := range sortedQualifiedPackages(root.Qualified) {
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
	if err := resource.NormalizeV6(all); err != nil {
		return nil, err
	}
	r := resolve.New(all)
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
		language:   LanguageCurrent,
		project:    project,
		resolver:   r,
		trust:      loaded,
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
	plugins, err := planPlugins(c, all, byID, pluginIDs, ownerProvenance(c, dependencies.Locked))
	if err != nil {
		return nil, err
	}
	c.plugins = plugins
	// The neutral target emits the package's own agents and every agent a
	// plugin ships, so a shipped dependency agent keeps its canonical bundle.
	emitted := map[string]bool{}
	for _, agent := range resolved {
		if all[agent.ID].Package == project.Name {
			emitted[agent.ID] = true
		}
	}
	for _, plan := range plugins {
		for _, agent := range plan.Agents {
			emitted[agent.ID] = true
		}
	}
	for _, agent := range resolved {
		if emitted[agent.ID] {
			c.agents = append(c.agents, agent)
		}
	}
	if err := validateAgentArtifactNames(c.agents); err != nil {
		return nil, err
	}
	if err := validateLibraryNames(c); err != nil {
		return nil, err
	}
	return c, nil
}

// shippedDependencyPlugins returns the dependency plugins a package ships: the
// ones its manifest lists, each of which its owning package's manifest must
// list among its own plugins (ADR-0034). When a candidate package is being
// validated, its plugins replace the ones the manifest lists from it.
func shippedDependencyPlugins(project *resource.Project, root *resource.V6Source, dependencies *packages.DependencySet) (map[string]bool, error) {
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
// and the locked packages in its dependency closure (ADR-0034).
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

// prepareLegacy reproduces an archival build exactly as its language defined
// it. Only the conformance corpora reach this path.
func prepareLegacy(source string, loaded *trust.Loaded, options BuildOptions) (*compilation, error) {
	trustPath := ""
	if loaded != nil {
		trustPath = loaded.Path
	}
	rootResources, err := resource.LoadWithOptions(source, trustPath, resource.LoadOptions{AllowLegacyV3: options.Language == LanguageLegacyV3})
	if err != nil {
		return nil, err
	}
	dependencies, locked, err := packages.LoadDependencies(source, options.PackagesDir)
	if err != nil {
		return nil, err
	}
	for id, document := range dependencies {
		if _, exists := rootResources[id]; exists {
			return nil, resource.Errorf("root resource cannot shadow locked dependency resource: %s", id)
		}
		rootResources[id] = document
	}
	if _, err := resource.LoadProject(source); err != nil {
		return nil, err
	}
	r := resolve.New(rootResources)
	resolved, err := r.ResolveAll()
	if err != nil {
		return nil, err
	}
	agents := []*resolve.ResolvedAgent{}
	for _, agent := range resolved {
		if agent.Emit {
			agents = append(agents, agent)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	if err := validateAgentArtifactNames(agents); err != nil {
		return nil, err
	}
	sourceDigest, err := HashSource(source)
	if err != nil {
		return nil, err
	}
	return &compilation{
		source:     source,
		language:   options.Language,
		resolver:   r,
		trust:      loaded,
		resolved:   resolved,
		agents:     agents,
		provenance: buildProvenance{SourceDigest: "sha256:" + sourceDigest, Dependencies: locked},
	}, nil
}

func sortedQualifiedPackages(qualified map[string][]string) []string {
	names := make([]string, 0, len(qualified))
	for name := range qualified {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

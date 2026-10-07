package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/resource"
)

// pluginMetadata is a source plugin's descriptive manifest members, each
// already checked against "Plugin metadata" (ADR-0007).
type pluginMetadata struct {
	author     [][2]string // name, then email and url when present
	homepage   string
	repository string
	license    string
	keywords   []string
}

// carriedFile is a file in the plugin directory that no component reads; it
// becomes a plugin `files` entry.
type carriedFile struct {
	as   string
	data []byte
}

// pluginFilesRoot is where import copies carried plugin files. It is not a
// valid skill name, so it cannot collide with files/<skill>/.
const pluginFilesRoot = "plugin-files/"

// metadataKeys are the manifest members import carries as plugin metadata.
var metadataKeys = map[string]bool{"author": true, "homepage": true, "repository": true, "license": true, "keywords": true}

// readMetadata carries one manifest member, or lists it as unsupported when
// its value is not one "Plugin metadata" accepts.
func (im *importer) readMetadata(source, key string, value any) {
	bad := func(why string) {
		im.unsupported = append(im.unsupported, source+": plugin metadata '"+key+"' "+why)
	}
	switch key {
	case "author":
		fields, ok := value.(map[string]any)
		if !ok {
			bad("must be an object of name, email, and url")
			return
		}
		author := [][2]string{}
		for _, field := range []string{"name", "email", "url"} {
			raw, present := fields[field]
			if !present {
				continue
			}
			text, isString := raw.(string)
			if !isString || !resource.IsMetadataValue(text) {
				bad("member '" + field + "' must be a non-empty single-line string")
				return
			}
			author = append(author, [2]string{field, text})
		}
		for field := range fields {
			if field != "name" && field != "email" && field != "url" {
				bad("has member '" + field + "', which Agent Plugins does not define")
				return
			}
		}
		if len(author) == 0 || author[0][0] != "name" {
			bad("requires a name")
			return
		}
		im.metadata.author = author
	case "homepage", "repository", "license":
		text, ok := value.(string)
		if !ok || !resource.IsMetadataValue(text) {
			bad("must be a non-empty single-line string")
			return
		}
		switch key {
		case "homepage":
			im.metadata.homepage = text
		case "repository":
			im.metadata.repository = text
		default:
			im.metadata.license = text
		}
	case "keywords":
		items, ok := value.([]any)
		if !ok || len(items) == 0 {
			bad("must be a non-empty list of strings")
			return
		}
		seen := map[string]bool{}
		keywords := []string{}
		for _, item := range items {
			text, isString := item.(string)
			if !isString || !resource.IsMetadataValue(text) || seen[text] {
				bad("must list distinct non-empty single-line strings")
				return
			}
			seen[text] = true
			keywords = append(keywords, text)
		}
		im.metadata.keywords = keywords
	}
}

// writeMetadata writes carried metadata into the plugin document.
func writeMetadata(fm *frontmatter, m pluginMetadata) {
	if len(m.author) > 0 {
		fm.key(0, "author")
		for _, field := range m.author {
			fm.raw(2, field[0], field[1])
		}
	}
	fm.scalar(0, "homepage", m.homepage)
	fm.scalar(0, "repository", m.repository)
	fm.scalar(0, "license", m.license)
	fm.list(0, "keywords", m.keywords)
}

// copilotComponent matches the files under com.github.copilot/ that the
// component import reads.
var copilotComponent = regexp.MustCompile(`^com\.github\.copilot/(?:agents/[^/]+\.agent\.md|commands/[^/]+\.md|rules/[^/]+\.md|hooks/hooks\.json|lsp\.json)$`)

// nestedComponent matches files in nested command or rule directories, which
// the component import already lists as unsupported.
var nestedComponent = regexp.MustCompile(`^com\.github\.copilot/(?:commands|rules)/[^/]+/`)

// carryPluginFiles walks the plugin directory and carries every file that is
// not a component. components reports whether a path is one; claimed lists
// the directories (with trailing slash) whose files components own, so an
// unread file there is unsupported rather than carried.
func (im *importer) carryPluginFiles(components func(string) bool, claimed []string) error {
	typeference := false
	err := filepath.WalkDir(im.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := im.rel(path)
		if entry.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case components(rel):
			return nil
		case strings.HasPrefix(rel, ".typeference/"):
			typeference = true
			return nil
		}
		for _, dir := range claimed {
			if strings.HasPrefix(rel, dir) {
				im.unsupported = append(im.unsupported, rel+": is in "+strings.TrimSuffix(dir, "/")+"/ but is not a component import reads")
				return nil
			}
		}
		if strings.HasSuffix(rel, ".tfer") {
			im.unsupported = append(im.unsupported, rel+": a .tfer file cannot be carried as a plain file")
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		im.carried = append(im.carried, carriedFile{as: rel, data: data})
		return nil
	})
	if err != nil {
		return resource.Errorf("Cannot read %s: %s", im.root, err)
	}
	if typeference {
		im.notes = append(im.notes, ".typeference/ is TypeFerence build provenance; it was not imported, and build regenerates it")
	}
	sort.Slice(im.carried, func(i, j int) bool { return im.carried[i].as < im.carried[j].as })
	return nil
}

// writeCarried adds the plugin's files entries and copies their bytes.
func writeCarried(fm *frontmatter, carried []carriedFile, result *Result) {
	if len(carried) == 0 {
		return
	}
	fm.key(0, "files")
	for _, file := range carried {
		source := pluginFilesRoot + file.as
		fm.b.WriteString(pad(2) + "- path: " + scalarText(source) + "\n")
		fm.b.WriteString(pad(4) + "as: " + scalarText(file.as) + "\n")
		result.Files = append(result.Files, File{Path: source, Data: file.data})
	}
}

// marketplaceIndexes are where Copilot reads a marketplace index.
var marketplaceIndexes = []string{".github/plugin/marketplace.json", ".claude-plugin/marketplace.json"}

// marketplaceError fails the import of a marketplace repository, naming the
// plugin directories its index lists, or returns nil when root is not one.
func marketplaceError(root string) error {
	for _, index := range marketplaceIndexes {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(index)))
		if err != nil {
			continue
		}
		var parsed struct {
			Plugins []struct {
				Name   string `json:"name"`
				Source any    `json:"source"`
			} `json:"plugins"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return resource.Errorf("%s: invalid JSON: %s", index, err)
		}
		lines := []string{}
		for _, plugin := range parsed.Plugins {
			if source, ok := plugin.Source.(string); ok && strings.HasPrefix(source, "./") {
				lines = append(lines, plugin.Name+": "+strings.TrimPrefix(source, "./"))
			} else {
				lines = append(lines, plugin.Name+": not a directory in this repository")
			}
		}
		sort.Strings(lines)
		return resource.Errorf("%s is a marketplace repository (%s); import does not convert a marketplace. Import each plugin directory into a package, then list those plugins and the repository's root files (README, CI workflows) in a marketplace package's manifest (docs/specification.md, \"Organization marketplaces\"). The index lists:\n  %s", root, index, strings.Join(lines, "\n  "))
	}
	return nil
}

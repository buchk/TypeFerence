package resource

import (
	"path"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/tferlex"
)

// fileTarget is where a files list ships: it supplies the default
// destination for an entry without `as` and rejects destinations build
// reserves for itself.
type fileTarget struct {
	defaultAs func(source string) string
	check     func(destination string) string
}

var skillFileRoots = map[string]bool{"references": true, "scripts": true, "assets": true}

// skillFiles ship in a skill directory, beneath references/, scripts/, or
// assets/.
var skillFiles = fileTarget{
	defaultAs: func(source string) string { return "references/" + path.Base(source) },
	check: func(destination string) string {
		if !strings.Contains(destination, "/") || !skillFileRoots[strings.SplitN(destination, "/", 2)[0]] {
			return "must be a clean path beneath references/, scripts/, or assets/"
		}
		return ""
	},
}

// pluginFiles ship at the root of each of a plugin's artifacts (ADR-0004).
var pluginFiles = fileTarget{
	defaultAs: path.Base,
	check: func(destination string) string {
		lower := strings.ToLower(destination)
		if lower == "plugin.json" || lower == "mcp.json" {
			return "is a file build emits"
		}
		switch strings.SplitN(lower, "/", 2)[0] {
		case "skills", "com.github.copilot", ".typeference", ".git":
			return "is beneath a directory build emits or excludes"
		}
		return ""
	},
}

// marketplaceFiles ship at the target root (ADR-0004). That no destination
// enters an artifact directory is checked by build, which knows the
// artifacts.
var marketplaceFiles = fileTarget{
	defaultAs: path.Base,
	check: func(destination string) string {
		lower := strings.ToLower(destination)
		switch strings.SplitN(lower, "/", 2)[0] {
		case ".typeference", ".git":
			return "is beneath a directory build emits or excludes"
		}
		for _, reserved := range []struct{ dir, holds string }{
			{".github/plugin", "the marketplace index build emits"},
			{".github/copilot", "repository settings, which build never emits"},
		} {
			if lower == reserved.dir || strings.HasPrefix(lower, reserved.dir+"/") {
				return "is beneath " + reserved.dir + "/, which holds " + reserved.holds
			}
		}
		return ""
	},
}

// fileEntries decodes a files list: each entry is a package path or a
// mapping {path, as}. It validates paths and destinations but reads no
// bytes, and checks collisions among the entries it decodes.
func (d *fieldDecoder) fileEntries(n *tferlex.Node, packageName string, target fileTarget) ([]PackageFile, error) {
	if isNull(n) {
		return nil, nil
	}
	if !n.IsSeq {
		return nil, d.errorf(n, "'files' must be a list")
	}
	files := []PackageFile{}
	var paths SkillPaths
	for _, item := range n.Items {
		item.Key = "files"
		file := PackageFile{Package: packageName}
		switch {
		case item.IsScalar && item.Value.Kind != tferlex.KindNull:
			file.Source = item.Value.Text
		case item.IsMap:
			err := d.decode(item, map[string]func(*tferlex.Node) error{
				"path": d.stringInto(&file.Source),
				"as":   d.stringInto(&file.As),
			})
			if err != nil {
				return nil, err
			}
		default:
			return nil, d.errorf(item, "a files entry is a path or a mapping {path, as}")
		}
		if !cleanRelative(file.Source) {
			return nil, d.errorf(item, "file '%s' must be a clean path relative to the package root", file.Source)
		}
		if strings.HasSuffix(file.Source, ".tfer") {
			return nil, d.errorf(item, "file '%s' is a .tfer document; files are plain files", file.Source)
		}
		switch strings.SplitN(file.Source, "/", 2)[0] {
		case "dist", "bin", "obj", ".git":
			return nil, d.errorf(item, "file '%s' is beneath a generated or excluded directory", file.Source)
		}
		if file.As == "" {
			file.As = target.defaultAs(file.Source)
		}
		if !cleanRelative(file.As) {
			return nil, d.errorf(item, "destination '%s' must be a clean relative path", file.As)
		}
		if problem := target.check(file.As); problem != "" {
			return nil, d.errorf(item, "destination '%s' %s", file.As, problem)
		}
		if err := paths.Claim(file.As, file.Source); err != nil {
			return nil, d.errorf(item, "%s", err)
		}
		files = append(files, file)
	}
	return files, nil
}

// metadataString decodes one plugin metadata value (ADR-0007).
func (d *fieldDecoder) metadataString(target *string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		value, null, err := d.text(n)
		if err != nil {
			return err
		}
		if null || !metadataValue(value) {
			return d.errorf(n, "'%s' must be a non-empty single line without surrounding whitespace", n.Key)
		}
		*target = value
		return nil
	}
}

func (d *fieldDecoder) pluginAuthor(target **PluginAuthor) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		if !n.IsMap {
			return d.errorf(n, "'author' must be a mapping of name, email, and url")
		}
		author := &PluginAuthor{}
		if err := d.decode(n, map[string]func(*tferlex.Node) error{
			"name":  d.metadataString(&author.Name),
			"email": d.metadataString(&author.Email),
			"url":   d.metadataString(&author.URL),
		}); err != nil {
			return err
		}
		if author.Name == "" {
			return d.errorf(n, "'author' requires a name")
		}
		*target = author
		return nil
	}
}

func (d *fieldDecoder) keywords(target *[]string) func(*tferlex.Node) error {
	return func(n *tferlex.Node) error {
		items, err := d.stringList(n)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return d.errorf(n, "'keywords' must list at least one keyword; omit it otherwise")
		}
		seen := map[string]bool{}
		for _, item := range items {
			if !metadataValue(item) {
				return d.errorf(n, "keyword '%s' must be a non-empty single line without surrounding whitespace", item)
			}
			if seen[item] {
				return d.errorf(n, "keyword '%s' is listed more than once", item)
			}
			seen[item] = true
		}
		*target = items
		return nil
	}
}

// metadataValue reports whether a plugin metadata value is a non-empty
// single line without surrounding whitespace.
func metadataValue(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && singleLine(value)
}

// IsMetadataValue is metadataValue for the importer, which checks values
// before it writes them.
func IsMetadataValue(value string) bool { return metadataValue(value) }

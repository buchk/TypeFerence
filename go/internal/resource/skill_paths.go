package resource

import "strings"

type skillPath struct {
	path      string
	source    string
	directory bool
}

// SkillPaths checks files and their directory prefixes for conflicts using
// the same rules on every platform (ADR-0003). Its zero value is ready to use.
// Destinations must already be clean, skill-relative paths.
type SkillPaths struct {
	claims map[string]skillPath
}

// Claim reserves a file and its directory prefixes. Shared directories must
// have identical spelling; files cannot share a path with files or directories.
func (p *SkillPaths) Claim(destination, source string) error {
	if p.claims == nil {
		p.claims = map[string]skillPath{}
	}
	parts := strings.Split(destination, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		key := strings.ToLower(prefix)
		directory := i < len(parts)-1
		if prior, exists := p.claims[key]; exists {
			switch {
			case prior.directory != directory:
				return Errorf("%s and %s use %s as both a file and a directory", prior.source, source, prefix)
			case !directory:
				return Errorf("%s and %s both ship as %s (destinations that differ only in letter case collide)", prior.source, source, destination)
			case prior.path != prefix:
				return Errorf("%s and %s use inconsistent directory casing: %s and %s", prior.source, source, prior.path, prefix)
			}
			continue
		}
		p.claims[key] = skillPath{path: prefix, source: source, directory: directory}
	}
	return nil
}

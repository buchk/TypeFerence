package compile_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/compile"
)

func TestSkillDirectoryPaths(t *testing.T) {
	tests := []struct {
		name      string
		paths     []string
		inherited bool
		extra     string
		wantError string
	}{
		{name: "directory case", paths: []string{"references/A/one.txt", "references/a/two.txt"}, wantError: "directory casing"},
		{name: "nested directory case", paths: []string{"assets/Team/Docs/one.txt", "assets/Team/docs/two.txt"}, wantError: "directory casing"},
		{name: "file and directory", paths: []string{"scripts/tool", "scripts/tool/run.sh"}, wantError: "both a file and a directory"},
		{name: "file and directory case", paths: []string{"references/Guide", "references/guide/two.txt"}, wantError: "both a file and a directory"},
		{name: "extension directory case", paths: []string{"references/A/one.txt", "references/a/two.txt"}, inherited: true, wantError: "directory casing"},
		{name: "rendered document", paths: []string{"references/Guide.md/one.txt"}, extra: "context:\n  - context: docs/guide.context.tfer\n    render: file\n", wantError: "both a file and a directory"},
		{name: "schema", paths: []string{"references/OUTPUT.schema.json/one.txt"}, extra: "outputSchema: '{}'\n", wantError: "both a file and a directory"},
		{name: "consistent mixed case", paths: []string{"references/Team/Docs/One.txt", "references/Team/Docs/Two.txt"}},
	}
	for _, tt := range tests {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tt.name, reverse), func(t *testing.T) {
				files := map[string]string{
					"typeference.tfer":        manifest,
					"plugins/kit.plugin.tfer": "---\ndescription: Kit.\nskills:\n  - skills/report.skill.tfer\n---\n",
					"docs/guide.context.tfer": "---\n---\nGuide.\n",
				}
				paths := append([]string{}, tt.paths...)
				if reverse {
					for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
						paths[i], paths[j] = paths[j], paths[i]
					}
				}
				var entries strings.Builder
				for i, path := range paths {
					name := fmt.Sprintf("files/%d.txt", i)
					files[name] = path + "\n"
					entry := fmt.Sprintf("  - path: %s\n    as: %s\n", name, path)
					if tt.inherited && i == 0 {
						files["skills/base.skill.tfer"] = "---\ndescription: Base.\nfiles:\n" + entry + "---\nBase.\n"
					} else {
						entries.WriteString(entry)
					}
				}
				fm := "---\ndescription: Report.\n" + tt.extra
				if tt.inherited {
					fm += "extends: skills/base.skill.tfer\n"
				}
				files["skills/report.skill.tfer"] = fm + "files:\n" + entries.String() + "---\nReport.\n"
				source, out := t.TempDir(), t.TempDir()
				write(t, source, files)
				write(t, out, map[string]string{"agent-plugin/keep.txt": "previous build\n"})
				_, err := compile.Build(source, out, compile.BuildOptions{})
				if tt.wantError == "" {
					if err != nil {
						t.Fatal(err)
					}
					for _, path := range paths {
						if got := read(t, out, "agent-plugin/kit/skills/report/"+path); got != path+"\n" {
							t.Fatalf("%s content = %q", path, got)
						}
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q diagnostic, got %v", tt.wantError, err)
				}
				if got := read(t, out, "agent-plugin/keep.txt"); got != "previous build\n" {
					t.Fatal("invalid paths changed the previous build")
				}
				remaining, err := os.ReadDir(filepath.Join(out, "agent-plugin"))
				if err != nil || len(remaining) != 1 {
					t.Fatalf("invalid paths must fail before writing output: %v, %v", remaining, err)
				}
			})
		}
	}
}

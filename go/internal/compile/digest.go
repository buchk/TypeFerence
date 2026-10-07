package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resource"
)

var runtimeCaseInsensitive = runtime.GOOS == "windows"

// HashDirectory computes the typeference-directory-v1 digest: SHA-256 over
// each file's forward-slash relative path and content, each followed by NUL,
// with files in canonical (code point) path order. Content is normalized text
// (byte order mark removed, CRLF to LF) for a file whose bytes are valid
// UTF-8, and the exact bytes of any other file (ADR-0038).
func HashDirectory(directory string) (string, error) {
	files, err := relativeFiles(directory)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, rel := range files {
		content, readErr := readTextFile(filepath.Join(directory, filepath.FromSlash(rel)))
		if readErr != nil {
			return "", readErr
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(content))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashSource computes typeference-resource-set-v1 over the explicit source
// membership used by pack. Generated output, caches, VCS data, and arbitrary
// unreferenced files therefore cannot change source identity.
func HashSource(source string) (string, error) {
	files, err := packages.SourceFiles(source)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, file := range files {
		data, err := file.Bytes()
		if err != nil {
			return "", err
		}
		h.Write([]byte(file.Path))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// relativeFiles lists every file beneath root as forward-slash relative
// paths in canonical order.
func relativeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("Cannot enumerate directory: %s", root)
	}
	sort.Strings(files)
	return files, nil
}

// readTextFile reads an artifact for comparison: normalized text when its
// bytes are valid UTF-8 (so a Windows checkout's CRLF conversion of committed
// text does not change identity), and the exact bytes otherwise, so a change
// to a binary file is never normalized away (ADR-0038).
func readTextFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", resource.Errorf("Cannot read file: %s", path)
	}
	return comparableContent(raw), nil
}

func comparableContent(raw []byte) string {
	if !utf8.Valid(raw) {
		return string(raw)
	}
	text := strings.TrimPrefix(string(raw), string(rune(0xFEFF)))
	return strings.ReplaceAll(text, "\r\n", "\n")
}

// DiffResult reports the file-level differences between two compiled trees.
type DiffResult struct {
	Different bool
	Added     []string
	Removed   []string
	Changed   []string
}

// CompareDirs compares two directories by relative path and exact content.
func CompareDirs(expected, actual string) (*DiffResult, error) {
	left, err := fileContents(expected)
	if err != nil {
		return nil, err
	}
	right, err := fileContents(actual)
	if err != nil {
		return nil, err
	}
	added := []string{}
	removed := []string{}
	changed := []string{}
	for path := range right {
		if _, ok := left[path]; !ok {
			added = append(added, path)
		}
	}
	for path, content := range left {
		rightContent, ok := right[path]
		if !ok {
			removed = append(removed, path)
		} else if content != rightContent {
			changed = append(changed, path)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)
	return &DiffResult{
		Different: len(added)+len(removed)+len(changed) > 0,
		Added:     added,
		Removed:   removed,
		Changed:   changed,
	}, nil
}

// fileContents maps relative slash paths to comparable content: text with
// its byte order mark stripped and line endings normalized when the file is
// valid UTF-8, and the exact bytes otherwise (ADR-0038).
func fileContents(root string) (map[string]string, error) {
	result := map[string]string{}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return result, nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		result[filepath.ToSlash(rel)] = comparableContent(raw)
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("Cannot enumerate directory: %s", root)
	}
	return result, nil
}

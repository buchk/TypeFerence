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

	"github.com/buchk/TypeFerence/go/internal/resource"
)

var runtimeCaseInsensitive = runtime.GOOS == "windows"

// HashDirectory computes the typeference-directory-v1 digest: SHA-256 over
// each file's forward-slash relative path and LF-normalized UTF-8 content,
// each followed by NUL, with files in canonical (code point) path order.
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

// readTextFile reads a UTF-8 text file, stripping a BOM and normalizing CRLF
// to LF, matching how the reference implementation reads artifact text.
func readTextFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", resource.Errorf("Cannot read file: %s", path)
	}
	text := strings.TrimPrefix(string(raw), string(rune(0xFEFF)))
	return strings.ReplaceAll(text, "\r\n", "\n"), nil
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

// fileContents maps relative slash paths to raw text (BOM stripped, no
// newline normalization), matching the reference diff contract.
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
		result[filepath.ToSlash(rel)] = strings.TrimPrefix(string(raw), string(rune(0xFEFF)))
		return nil
	})
	if err != nil {
		return nil, resource.Errorf("Cannot enumerate directory: %s", root)
	}
	return result, nil
}

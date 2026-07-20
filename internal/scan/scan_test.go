package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRecursiveDirectorySizes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "root.bin"), 10)
	writeFile(t, filepath.Join(root, "a", "a.bin"), 20)
	writeFile(t, filepath.Join(root, "a", "b", "b.bin"), 30)

	result, err := Scan(root, Options{Limit: 10, SizeMode: SizeModeRecursive, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}

	sizes := entriesByBase(result)
	if sizes[filepath.Base(root)] != 60 {
		t.Fatalf("root size = %d, want 60", sizes[filepath.Base(root)])
	}
	if sizes["a"] != 50 {
		t.Fatalf("a size = %d, want 50", sizes["a"])
	}
	if sizes["b"] != 30 {
		t.Fatalf("b size = %d, want 30", sizes["b"])
	}
}

func TestScanTopLevelDirectorySizes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "root.bin"), 10)
	writeFile(t, filepath.Join(root, "a", "a.bin"), 20)
	writeFile(t, filepath.Join(root, "a", "b", "b.bin"), 30)

	result, err := Scan(root, Options{Limit: 10, SizeMode: SizeModeTopLevel, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}

	sizes := entriesByBase(result)
	if sizes[filepath.Base(root)] != 10 {
		t.Fatalf("root size = %d, want 10", sizes[filepath.Base(root)])
	}
	if sizes["a"] != 20 {
		t.Fatalf("a size = %d, want 20", sizes["a"])
	}
	if sizes["b"] != 30 {
		t.Fatalf("b size = %d, want 30", sizes["b"])
	}
}

func TestScanExcludesBeforeAggregation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep", "keep.bin"), 10)
	writeFile(t, filepath.Join(root, "node_modules", "dep.bin"), 90)

	result, err := Scan(root, Options{
		Limit:           10,
		SizeMode:        SizeModeRecursive,
		ExcludePatterns: []string{"node_modules"},
		Workers:         2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 10 {
		t.Fatalf("total = %d, want 10", result.Total)
	}
	for _, entry := range result.Entries {
		if filepath.Base(entry.Path) == "node_modules" {
			t.Fatal("excluded directory was reported")
		}
	}
}

func TestScanIgnoreFileUsesRootRelativeGitStyleRules(t *testing.T) {
	root := t.TempDir()
	ignoreFile := filepath.Join(t.TempDir(), "rules")
	writeFile(t, filepath.Join(root, "keep.bin"), 10)
	writeFile(t, filepath.Join(root, "cache", "drop.bin"), 40)
	writeFile(t, filepath.Join(root, "cache", "keep.bin"), 20)
	writeFile(t, filepath.Join(root, "dist", "drop.bin"), 30)
	writeFile(t, filepath.Join(root, "nested", "old.tmp"), 50)
	writeFile(t, filepath.Join(root, "nested", "root-only"), 60)
	writeFile(t, filepath.Join(root, "root-only"), 70)
	writeText(t, ignoreFile, strings.Join([]string{
		"# backup exclusions",
		"cache/",
		"!cache/keep.bin",
		"/dist/",
		"*.tmp",
		"/root-only",
	}, "\n"))

	result, err := Scan(root, Options{
		Limit:       20,
		SizeMode:    SizeModeRecursive,
		IgnoreFiles: []string{ignoreFile},
		Workers:     2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 90 {
		t.Fatalf("total = %d, want 90", result.Total)
	}
	for _, entry := range result.Entries {
		if base := filepath.Base(entry.Path); base == "cache" || base == "dist" || entry.Path == filepath.Join(root, "root-only") {
			t.Fatalf("ignored path was reported: %s", entry.Path)
		}
	}
}

func TestScanIgnoreFileSupportsAbsoluteRules(t *testing.T) {
	root := t.TempDir()
	ignoreFile := filepath.Join(t.TempDir(), "rules")
	writeFile(t, filepath.Join(root, "keep.bin"), 10)
	writeFile(t, filepath.Join(root, "ignored", "drop.bin"), 90)
	writeText(t, ignoreFile, filepath.Join(root, "ignored")+"\n")

	result, err := Scan(root, Options{
		Limit:       10,
		SizeMode:    SizeModeRecursive,
		IgnoreFiles: []string{ignoreFile},
		Workers:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 10 {
		t.Fatalf("total = %d, want 10", result.Total)
	}
}

func TestScanIgnoreFileSupportsDoubleStarRules(t *testing.T) {
	root := t.TempDir()
	ignoreFile := filepath.Join(t.TempDir(), "rules")
	writeFile(t, filepath.Join(root, "archive", "one", "generated", "drop.bin"), 90)
	writeFile(t, filepath.Join(root, "archive", "one", "keep.bin"), 10)
	writeText(t, ignoreFile, "archive/**/generated/\n")

	result, err := Scan(root, Options{
		Limit:       10,
		SizeMode:    SizeModeRecursive,
		IgnoreFiles: []string{ignoreFile},
		Workers:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 10 {
		t.Fatalf("total = %d, want 10", result.Total)
	}
}

func TestScanIgnoreFileDoesNotOverrideExclude(t *testing.T) {
	root := t.TempDir()
	ignoreFile := filepath.Join(t.TempDir(), "rules")
	writeFile(t, filepath.Join(root, "cache", "keep.bin"), 20)
	writeText(t, ignoreFile, "!cache/keep.bin\n")

	result, err := Scan(root, Options{
		Limit:           10,
		SizeMode:        SizeModeRecursive,
		ExcludePatterns: []string{"cache"},
		IgnoreFiles:     []string{ignoreFile},
		Workers:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 {
		t.Fatalf("total = %d, want 0", result.Total)
	}
}

func TestScanRejectsUnreadableIgnoreFile(t *testing.T) {
	_, err := Scan(t.TempDir(), Options{
		IgnoreFiles: []string{filepath.Join(t.TempDir(), "missing")},
	})
	if err == nil || !strings.Contains(err.Error(), "read ignore file") {
		t.Fatalf("error = %v, want read ignore file error", err)
	}
}

func TestScanExcludesPseudoFilesystemsBeforeAggregation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep", "keep.bin"), 10)
	writeFile(t, filepath.Join(root, "proc", "synthetic.bin"), 90)
	procPath := filepath.Join(root, "proc")

	restore := stubPseudoFilesystem(t, func(path string) (bool, error) {
		return path == procPath, nil
	})
	defer restore()

	result, err := Scan(root, Options{
		Limit:           10,
		SizeMode:        SizeModeRecursive,
		CrossFilesystem: true,
		Workers:         2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 10 {
		t.Fatalf("total = %d, want 10", result.Total)
	}
	for _, entry := range result.Entries {
		if filepath.Base(entry.Path) == "proc" {
			t.Fatal("pseudo-filesystem directory was reported")
		}
	}
}

func TestScanPseudoFilesystemRootReturnsEmptyResult(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "synthetic.bin"), 90)

	restore := stubPseudoFilesystem(t, func(path string) (bool, error) {
		return path == root, nil
	})
	defer restore()

	result, err := Scan(root, Options{
		Limit:    10,
		SizeMode: SizeModeRecursive,
		Workers:  2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 0 {
		t.Fatalf("total = %d, want 0", result.Total)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Path != root || result.Entries[0].Size != 0 {
		t.Fatalf("entry = %#v, want zero-sized root", result.Entries[0])
	}
}

func TestScanRegularFilesOnlySkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.bin")
	link := filepath.Join(root, "target.link")
	writeFile(t, target, 10)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	result, err := Scan(root, Options{
		Limit:            10,
		SizeMode:         SizeModeRecursive,
		RegularFilesOnly: true,
		Workers:          2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 10 {
		t.Fatalf("total = %d, want 10", result.Total)
	}
}

func stubPseudoFilesystem(t *testing.T, fn func(string) (bool, error)) func() {
	t.Helper()
	original := isPseudoFilesystem
	isPseudoFilesystem = fn
	return func() {
		isPseudoFilesystem = original
	}
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, size)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeText(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func entriesByBase(result Result) map[string]int64 {
	out := make(map[string]int64, len(result.Entries))
	for _, entry := range result.Entries {
		out[filepath.Base(entry.Path)] = entry.Size
	}
	return out
}

package scan

import (
	"os"
	"path/filepath"
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

func entriesByBase(result Result) map[string]int64 {
	out := make(map[string]int64, len(result.Entries))
	for _, entry := range result.Entries {
		out[filepath.Base(entry.Path)] = entry.Size
	}
	return out
}

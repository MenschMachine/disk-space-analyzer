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

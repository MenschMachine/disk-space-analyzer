package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestRunVersion(t *testing.T) {
	originalVersion := version
	version = "1.2.3"
	t.Cleanup(func() {
		version = originalVersion
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	if got, want := stdout.String(), "dsa 1.2.3\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestParseArgsAllowsFlagsAfterPath(t *testing.T) {
	cfg, err := parseArgs([]string{".", "--limit", "1", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.path != "." {
		t.Fatalf("path = %q, want .", cfg.path)
	}
	if cfg.limit != 1 {
		t.Fatalf("limit = %d, want 1", cfg.limit)
	}
	if cfg.format != "json" {
		t.Fatalf("format = %q, want json", cfg.format)
	}
}

func TestParseArgsAllowsRepeatedExcludesAfterPath(t *testing.T) {
	cfg, err := parseArgs([]string{".", "--exclude", "node_modules", "--exclude=dist"})
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.excludes) != 2 {
		t.Fatalf("len(excludes) = %d, want 2", len(cfg.excludes))
	}
	if cfg.excludes[0] != "node_modules" || cfg.excludes[1] != "dist" {
		t.Fatalf("excludes = %#v, want node_modules and dist", cfg.excludes)
	}
}

func TestParseArgsAllowsCrossFSAfterPath(t *testing.T) {
	cfg, err := parseArgs([]string{".", "--cross-fs"})
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.crossFS {
		t.Fatal("crossFS = false, want true")
	}
}

func TestParseArgsAllowsNoDeviceCheckAfterPath(t *testing.T) {
	cfg, err := parseArgs([]string{".", "--no-device-check"})
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.noDeviceCheck {
		t.Fatal("noDeviceCheck = false, want true")
	}
}

func TestParseArgsAllowsRegularFilesOnlyAfterPath(t *testing.T) {
	cfg, err := parseArgs([]string{".", "--regular-files-only"})
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.regularFilesOnly {
		t.Fatal("regularFilesOnly = false, want true")
	}
}

func TestDisplayPathWithBranchGradientUsesOneHueForBranch(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	parent := filepath.Join(root, "node_modules")
	child := filepath.Join(parent, ".cache")

	base := colorForTopLevelPath("node_modules")
	got := displayPathWithBranchGradient(child, root, true)
	want := colorize("node_modules", ansiRGB(shadeForDepth(base, 0)), true) +
		string(filepath.Separator) +
		colorize(".cache", ansiRGB(shadeForDepth(base, 1)), true)
	if got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
}

func TestDisplayPathWithBranchGradientUsesSamePrefixColorsForParentAndChild(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	parent := filepath.Join(root, "node_modules")
	child := filepath.Join(parent, ".cache")

	parentDisplay := displayPathWithBranchGradient(parent, root, true)
	childDisplay := displayPathWithBranchGradient(child, root, true)

	if parentDisplay != colorize("node_modules", ansiRGB(shadeForDepth(colorForTopLevelPath("node_modules"), 0)), true) {
		t.Fatalf("parent display = %q, want top-level branch color", parentDisplay)
	}
	wantPrefix := parentDisplay + string(filepath.Separator)
	if childDisplay[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("child display = %q, want prefix %q", childDisplay, wantPrefix)
	}
}

func TestDisplayPathWithBranchGradientDarkensByDepth(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	grandparent := filepath.Join(root, "src")
	parent := filepath.Join(grandparent, "vendor")
	child := filepath.Join(parent, "cache")

	base := colorForTopLevelPath("src")
	got := displayPathWithBranchGradient(child, root, true)
	want := colorize("src", ansiRGB(shadeForDepth(base, 0)), true) +
		string(filepath.Separator) +
		colorize("vendor", ansiRGB(shadeForDepth(base, 1)), true) +
		string(filepath.Separator) +
		colorize("cache", ansiRGB(shadeForDepth(base, 2)), true)
	if got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
}

func TestDisplayPathWithBranchGradientLeavesPlainPathWhenColorDisabled(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	child := filepath.Join(root, "node_modules", ".cache")

	got := displayPathWithBranchGradient(child, root, false)
	want := filepath.Join("node_modules", ".cache")
	if got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
}

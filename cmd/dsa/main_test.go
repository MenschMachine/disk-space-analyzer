package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/MenschMachine/disk-space-analyzer/internal/scan"
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

func TestParseArgsAllowsRepeatedIgnoreFilesAfterPath(t *testing.T) {
	cfg, err := parseArgs([]string{".", "--ignore-file", "first.ignore", "--ignore-file=second.ignore"})
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.ignoreFiles) != 2 {
		t.Fatalf("len(ignoreFiles) = %d, want 2", len(cfg.ignoreFiles))
	}
	if cfg.ignoreFiles[0] != "first.ignore" || cfg.ignoreFiles[1] != "second.ignore" {
		t.Fatalf("ignoreFiles = %#v, want first.ignore and second.ignore", cfg.ignoreFiles)
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

func TestDisplayPathWithBranchColorUsesOneColorForBranch(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	parent := filepath.Join(root, "node_modules")
	child := filepath.Join(parent, ".cache")

	branchColors := branchColorsForEntries(root, []scan.Entry{{Path: parent}, {Path: child}})
	base := branchColors["node_modules"]
	got := displayPathWithBranchColor(child, root, branchColors, true)
	want := colorize("node_modules", colorForDepth(base, 0), true) +
		string(filepath.Separator) +
		colorize(".cache", colorForDepth(base, 1), true)
	if got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
}

func TestDisplayPathWithBranchColorUsesSamePrefixColorsForParentAndChild(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	parent := filepath.Join(root, "node_modules")
	child := filepath.Join(parent, ".cache")

	branchColors := branchColorsForEntries(root, []scan.Entry{{Path: parent}, {Path: child}})
	parentDisplay := displayPathWithBranchColor(parent, root, branchColors, true)
	childDisplay := displayPathWithBranchColor(child, root, branchColors, true)

	if parentDisplay != colorize("node_modules", colorForDepth(branchColors["node_modules"], 0), true) {
		t.Fatalf("parent display = %q, want top-level branch color", parentDisplay)
	}
	wantPrefix := parentDisplay + string(filepath.Separator)
	if childDisplay[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("child display = %q, want prefix %q", childDisplay, wantPrefix)
	}
}

func TestDisplayPathWithBranchColorUsesGradientForDescendants(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	grandparent := filepath.Join(root, "src")
	parent := filepath.Join(grandparent, "vendor")
	child := filepath.Join(parent, "cache")

	branchColors := branchColorsForEntries(root, []scan.Entry{{Path: grandparent}, {Path: parent}, {Path: child}})
	base := branchColors["src"]
	got := displayPathWithBranchColor(child, root, branchColors, true)
	want := colorize("src", colorForDepth(base, 0), true) +
		string(filepath.Separator) +
		colorize("vendor", colorForDepth(base, 1), true) +
		string(filepath.Separator) +
		colorize("cache", colorForDepth(base, 2), true)
	if got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
	if colorForDepth(base, 1) == colorForDepth(base, 2) {
		t.Fatal("depth 1 and depth 2 use the same color")
	}
}

func TestDisplayPathWithBranchColorLeavesPlainPathWhenColorDisabled(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	child := filepath.Join(root, "node_modules", ".cache")

	branchColors := branchColorsForEntries(root, []scan.Entry{{Path: child}})
	got := displayPathWithBranchColor(child, root, branchColors, false)
	want := filepath.Join("node_modules", ".cache")
	if got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
}

func TestBranchColorsForEntriesDoesNotReuseColorsBeforePaletteWrap(t *testing.T) {
	root := filepath.Join("tmp", "scan")
	entries := []scan.Entry{
		{Path: filepath.Join(root, "michael")},
		{Path: filepath.Join(root, "michael", "icloud")},
		{Path: filepath.Join(root, "pdfdancer-pii-detection")},
		{Path: filepath.Join(root, "font-identifier")},
	}

	colors := branchColorsForEntries(root, entries)
	if colors["michael"] == colors["pdfdancer-pii-detection"] {
		t.Fatal("michael and pdfdancer-pii-detection were assigned the same color")
	}
	if colors["michael"] == colors["font-identifier"] {
		t.Fatal("michael and font-identifier were assigned the same color")
	}
	if colors["pdfdancer-pii-detection"] == colors["font-identifier"] {
		t.Fatal("pdfdancer-pii-detection and font-identifier were assigned the same color")
	}
}

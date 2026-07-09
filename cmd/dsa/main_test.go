package main

import (
	"bytes"
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

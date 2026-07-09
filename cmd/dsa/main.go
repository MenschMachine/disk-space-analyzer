package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/mlahr/disk-space-analyzer/internal/scan"
)

const defaultLimit = 50

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type cliConfig struct {
	format   string
	limit    int
	sizeMode string
	excludes []string
	workers  int
	path     string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return 0
		}
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, "Run dsa --help for usage.")
		return 2
	}

	mode, err := scan.ParseSizeMode(cfg.sizeMode)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	result, err := scan.Scan(cfg.path, scan.Options{
		Limit:           cfg.limit,
		SizeMode:        mode,
		ExcludePatterns: cfg.excludes,
		Workers:         cfg.workers,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	switch cfg.format {
	case "table":
		writeTable(stdout, result)
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "invalid --format %q: expected table or json\n", cfg.format)
		return 2
	}

	return 0
}

func parseArgs(args []string) (cliConfig, error) {
	var excludes multiFlag
	cfg := cliConfig{
		format:   "table",
		limit:    defaultLimit,
		sizeMode: string(scan.SizeModeRecursive),
		workers:  0,
	}

	fs := flag.NewFlagSet("dsa", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.format, "format", cfg.format, "output format: table or json")
	fs.IntVar(&cfg.limit, "limit", cfg.limit, "maximum number of directories to show")
	fs.StringVar(&cfg.sizeMode, "size-mode", cfg.sizeMode, "directory size mode: recursive or top-level")
	fs.Var(&excludes, "exclude", "glob pattern to exclude; may be repeated")
	fs.IntVar(&cfg.workers, "workers", cfg.workers, "number of scanner workers; defaults to logical CPUs")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.limit < 1 {
		return cfg, fmt.Errorf("--limit must be greater than zero")
	}
	if cfg.workers < 0 {
		return cfg, fmt.Errorf("--workers must be zero or greater")
	}
	if fs.NArg() > 1 {
		return cfg, fmt.Errorf("expected at most one path argument")
	}
	if fs.NArg() == 1 {
		cfg.path = fs.Arg(0)
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return cfg, err
		}
		cfg.path = wd
	}
	cfg.excludes = excludes
	return cfg, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: dsa [flags] [path]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Find the largest directories under path. If path is omitted, dsa scans the current working directory.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --format table|json           output format (default table)")
	fmt.Fprintf(w, "  --limit N                     maximum directories to show (default %d)\n", defaultLimit)
	fmt.Fprintln(w, "  --size-mode recursive|top-level")
	fmt.Fprintln(w, "                                directory size aggregation mode (default recursive)")
	fmt.Fprintln(w, "  --exclude GLOB                exclude paths matching glob; may be repeated")
	fmt.Fprintln(w, "  --workers N                   scanner workers; defaults to logical CPUs")
	fmt.Fprintln(w, "  --help                        show this help")
}

func writeTable(w io.Writer, result scan.Result) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SIZE\tPERCENT\tPATH")
	for _, entry := range result.Entries {
		fmt.Fprintf(tw, "%s\t%.1f%%\t%s\n", humanSize(entry.Size), entry.Percent, displayPath(entry.Path, result.Root))
	}
	if len(result.Errors) > 0 {
		fmt.Fprintf(tw, "\nWARNINGS\t%d scan errors; use --format json for details\t\n", len(result.Errors))
	}
	_ = tw.Flush()
}

func displayPath(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return path
	}
	return rel
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB", "PiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f EiB", value/unit)
}

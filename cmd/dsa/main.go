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
	"sync"
	"text/tabwriter"
	"time"

	"github.com/MenschMachine/disk-space-analyzer/internal/scan"
)

const defaultLimit = 50

var version = "dev"

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type cliConfig struct {
	format           string
	limit            int
	sizeMode         string
	excludes         []string
	workers          int
	stream           bool
	crossFS          bool
	noDeviceCheck    bool
	regularFilesOnly bool
	path             string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintf(stdout, "dsa %s\n", version)
		return 0
	}

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

	options := scan.Options{
		Limit:            cfg.limit,
		SizeMode:         mode,
		ExcludePatterns:  cfg.excludes,
		Workers:          cfg.workers,
		CrossFilesystem:  cfg.crossFS,
		NoDeviceCheck:    cfg.noDeviceCheck,
		RegularFilesOnly: cfg.regularFilesOnly,
	}
	var writeMu sync.Mutex
	useColor, err := shouldUseColor(stdout)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	if cfg.stream {
		if cfg.format != "table" {
			fmt.Fprintln(stderr, "--stream requires --format table")
			return 2
		}
		options.ProgressEvery = 250 * time.Millisecond
		options.Progress = func(snapshot scan.Snapshot) {
			if snapshot.Done {
				return
			}
			writeMu.Lock()
			defer writeMu.Unlock()
			writeLiveTable(stdout, snapshot, useColor)
		}
	}

	result, err := scan.Scan(cfg.path, options)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	switch cfg.format {
	case "table":
		writeMu.Lock()
		defer writeMu.Unlock()
		if cfg.stream {
			clearScreen(stdout)
		}
		writeTable(stdout, result, useColor)
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
	fs.BoolVar(&cfg.crossFS, "cross-fs", cfg.crossFS, "descend into directories on other filesystems")
	fs.BoolVar(&cfg.noDeviceCheck, "no-device-check", cfg.noDeviceCheck, "skip directory device checks; may cross filesystem boundaries")
	fs.BoolVar(&cfg.regularFilesOnly, "regular-files-only", cfg.regularFilesOnly, "count only regular file entries")
	fs.IntVar(&cfg.workers, "workers", cfg.workers, "number of scanner workers; defaults to logical CPUs")
	fs.BoolVar(&cfg.stream, "stream", cfg.stream, "continuously refresh the current top directories while scanning")

	normalizedArgs := normalizeInterspersedFlags(args)
	if err := fs.Parse(normalizedArgs); err != nil {
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

func normalizeInterspersedFlags(args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		if flagNeedsSeparateValue(arg) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positionals...)
}

func flagNeedsSeparateValue(arg string) bool {
	name := strings.TrimLeft(arg, "-")
	if name == "help" || name == "h" || strings.Contains(name, "=") {
		return false
	}
	switch name {
	case "format", "limit", "size-mode", "exclude", "workers":
		return true
	default:
		return false
	}
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
	fmt.Fprintln(w, "  --cross-fs                    descend into directories on other filesystems")
	fmt.Fprintln(w, "  --no-device-check             skip directory device checks; may cross filesystem boundaries")
	fmt.Fprintln(w, "                                pseudo-filesystems are still excluded")
	fmt.Fprintln(w, "  --regular-files-only          count only regular file entries")
	fmt.Fprintln(w, "  --stream                      continuously refresh current top table while scanning")
	fmt.Fprintln(w, "  --workers N                   scanner workers; defaults to logical CPUs")
	fmt.Fprintln(w, "  --version                     show version")
	fmt.Fprintln(w, "  --help                        show this help")
}

func writeLiveTable(w io.Writer, snapshot scan.Snapshot, useColor bool) {
	clearScreen(w)
	state := "Scanning"
	if snapshot.Done {
		state = "Finalizing"
	}
	fmt.Fprintf(w, "%s: %s\n", colorize(state, ansiCyanBold, useColor), snapshot.Root)
	fmt.Fprintf(w, "Mode: %s  Directories seen: %s  Known size: %s  Errors: %s\n\n",
		colorize(string(snapshot.SizeMode), ansiDim, useColor),
		colorize(fmt.Sprintf("%d", snapshot.DirectoriesScanned), ansiGreen, useColor),
		colorize(humanSize(snapshot.Total), ansiGreen, useColor),
		colorize(fmt.Sprintf("%d", len(snapshot.Errors)), errorColor(len(snapshot.Errors), useColor), useColor),
	)
	writeEntriesTable(w, snapshot.Root, snapshot.Entries, useColor)
}

func clearScreen(w io.Writer) {
	fmt.Fprint(w, "\033[H\033[2J")
}

func writeTable(w io.Writer, result scan.Result, useColor bool) {
	writeEntriesTable(w, result.Root, result.Entries, useColor)
	if len(result.Errors) > 0 {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "\n%s\t%d scan errors; use --format json for details\t\n", colorize("WARNINGS", ansiYellowBold, useColor), len(result.Errors))
		_ = tw.Flush()
	}
}

func writeEntriesTable(w io.Writer, root string, entries []scan.Entry, useColor bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	branchColors := branchColorsForEntries(root, entries)
	fmt.Fprintf(tw, "%s\t%s\t%s\n",
		colorize("SIZE", ansiBold, useColor),
		colorize("PERCENT", ansiBold, useColor),
		colorize("PATH", ansiBold, useColor),
	)
	for _, entry := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			colorize(humanSize(entry.Size), sizeColor(entry.Percent), useColor),
			colorize(fmt.Sprintf("%.1f%%", entry.Percent), percentColor(entry.Percent), useColor),
			displayPathWithBranchColor(entry.Path, root, branchColors, useColor),
		)
	}
	_ = tw.Flush()
}

const (
	ansiReset       = "\033[0m"
	ansiBold        = "\033[1m"
	ansiDim         = "\033[2m"
	ansiGreen       = "\033[32m"
	ansiYellow      = "\033[33m"
	ansiRed         = "\033[31m"
	ansiBlue        = "\033[34m"
	ansiMagenta     = "\033[35m"
	ansiCyan        = "\033[36m"
	ansiGreenBold   = "\033[1;32m"
	ansiBlueBold    = "\033[1;34m"
	ansiMagentaBold = "\033[1;35m"
	ansiCyanBold    = "\033[1;36m"
	ansiYellowBold  = "\033[1;33m"
)

type branchColor struct {
	shades [4]string
}

var branchBaseColors = []branchColor{
	ansi256BranchRamp(82, 76, 70, 64),
	ansi256BranchRamp(51, 45, 39, 33),
	ansi256BranchRamp(219, 213, 207, 201),
	ansi256BranchRamp(220, 214, 208, 202),
	ansi256BranchRamp(117, 111, 105, 99),
	ansi256BranchRamp(215, 209, 203, 197),
	ansi256BranchRamp(183, 177, 171, 165),
	ansi256BranchRamp(86, 80, 74, 68),
	ansi256BranchRamp(154, 148, 142, 136),
	ansi256BranchRamp(81, 75, 69, 63),
	ansi256BranchRamp(203, 167, 131, 95),
	ansi256BranchRamp(147, 141, 135, 129),
	ansi256BranchRamp(121, 85, 49, 35),
	ansi256BranchRamp(229, 223, 217, 211),
	ansi256BranchRamp(159, 153, 147, 141),
	ansi256BranchRamp(218, 212, 206, 200),
}

func shouldUseColor(w io.Writer) (bool, error) {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false, nil
	}
	file, ok := w.(*os.File)
	if !ok {
		return false, nil
	}
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeCharDevice != 0, nil
}

func colorize(value, color string, enabled bool) string {
	if !enabled || color == "" || color == ansiReset {
		return value
	}
	return color + value + ansiReset
}

func displayPathWithBranchColor(path, root string, branchColors map[string]branchColor, useColor bool) string {
	display := displayPath(path, root)
	if !useColor {
		return display
	}
	if display == path {
		return colorize(display, ansiCyan, useColor)
	}

	segments := strings.Split(filepath.ToSlash(display), "/")
	if len(segments) == 0 || segments[0] == "" {
		return display
	}

	base, ok := branchColors[segments[0]]
	if !ok {
		base = branchBaseColors[0]
	}
	var b strings.Builder
	for i, segment := range segments {
		if i > 0 {
			b.WriteString(string(filepath.Separator))
		}
		b.WriteString(colorize(segment, colorForDepth(base, i), useColor))
	}
	return b.String()
}

func branchColorsForEntries(root string, entries []scan.Entry) map[string]branchColor {
	colors := make(map[string]branchColor)
	for _, entry := range entries {
		topLevel, ok := topLevelSegment(entry.Path, root)
		if !ok {
			continue
		}
		if _, exists := colors[topLevel]; exists {
			continue
		}
		colors[topLevel] = branchBaseColors[len(colors)%len(branchBaseColors)]
	}
	return colors
}

func topLevelSegment(path, root string) (string, bool) {
	display := displayPath(path, root)
	if display == path {
		return "", false
	}
	segments := strings.Split(filepath.ToSlash(display), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "", false
	}
	return segments[0], true
}

func colorForDepth(base branchColor, depth int) string {
	if depth == 0 {
		return "\033[1;" + strings.TrimPrefix(base.shades[0], "\033[")
	}
	if depth >= len(base.shades) {
		depth = len(base.shades) - 1
	}
	return base.shades[depth]
}

func ansi256BranchRamp(c0, c1, c2, c3 int) branchColor {
	return branchColor{shades: [4]string{
		ansi256Foreground(c0),
		ansi256Foreground(c1),
		ansi256Foreground(c2),
		ansi256Foreground(c3),
	}}
}

func ansi256Foreground(code int) string {
	return fmt.Sprintf("\033[38;5;%dm", code)
}

func sizeColor(percent float64) string {
	switch {
	case percent >= 50:
		return ansiRed
	case percent >= 20:
		return ansiYellow
	default:
		return ansiGreen
	}
}

func percentColor(percent float64) string {
	return sizeColor(percent)
}

func errorColor(errors int, useColor bool) string {
	if !useColor {
		return ""
	}
	if errors > 0 {
		return ansiYellow
	}
	return ansiGreen
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

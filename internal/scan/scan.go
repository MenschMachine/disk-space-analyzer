package scan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type SizeMode string

const (
	SizeModeRecursive SizeMode = "recursive"
	SizeModeTopLevel  SizeMode = "top-level"
)

type Options struct {
	Limit           int
	SizeMode        SizeMode
	ExcludePatterns []string
	Workers         int
}

type Result struct {
	Root     string      `json:"root"`
	SizeMode SizeMode    `json:"size_mode"`
	Total    int64       `json:"total"`
	Entries  []Entry     `json:"entries"`
	Errors   []ScanError `json:"errors"`
}

type Entry struct {
	Path    string  `json:"path"`
	Size    int64   `json:"size"`
	Percent float64 `json:"percent"`
}

type ScanError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type task struct {
	path   string
	parent string
}

type node struct {
	path       string
	parent     string
	directSize int64
	totalSize  int64
	children   []string
}

func ParseSizeMode(value string) (SizeMode, error) {
	switch SizeMode(value) {
	case SizeModeRecursive:
		return SizeModeRecursive, nil
	case SizeModeTopLevel:
		return SizeModeTopLevel, nil
	default:
		return "", fmt.Errorf("invalid --size-mode %q: expected recursive or top-level", value)
	}
}

func Scan(root string, opts Options) (Result, error) {
	mode := opts.SizeMode
	if mode == "" {
		mode = SizeModeRecursive
	}
	if _, err := ParseSizeMode(string(mode)); err != nil {
		return Result{}, err
	}
	if opts.Limit < 1 {
		opts.Limit = 50
	}
	workers := opts.Workers
	if workers == 0 {
		workers = runtime.NumCPU()
	}
	if workers < 1 {
		return Result{}, errors.New("workers must be greater than zero")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Lstat(absRoot)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("root path is not a directory: %s", absRoot)
	}

	matcher := newExcludeMatcher(absRoot, opts.ExcludePatterns)
	nodes := map[string]*node{
		absRoot: {path: absRoot},
	}
	var mu sync.Mutex
	errs := make([]ScanError, 0)
	tasks := make(chan task, workers*2)
	var wg sync.WaitGroup

	worker := func() {
		for t := range tasks {
			scanOne(t, absRoot, matcher, tasks, &wg, &mu, nodes, &errs)
			wg.Done()
		}
	}

	for i := 0; i < workers; i++ {
		go worker()
	}

	wg.Add(1)
	tasks <- task{path: absRoot}
	wg.Wait()
	close(tasks)

	total := computeTotals(nodes, absRoot)
	entries := make([]Entry, 0, len(nodes))
	for _, n := range nodes {
		size := n.totalSize
		if mode == SizeModeTopLevel {
			size = n.directSize
		}
		entries = append(entries, Entry{
			Path:    n.path,
			Size:    size,
			Percent: percent(size, total),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Size == entries[j].Size {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Size > entries[j].Size
	})
	if len(entries) > opts.Limit {
		entries = entries[:opts.Limit]
	}
	sort.Slice(errs, func(i, j int) bool {
		return errs[i].Path < errs[j].Path
	})

	return Result{
		Root:     absRoot,
		SizeMode: mode,
		Total:    total,
		Entries:  entries,
		Errors:   errs,
	}, nil
}

func scanOne(t task, root string, matcher excludeMatcher, tasks chan<- task, wg *sync.WaitGroup, mu *sync.Mutex, nodes map[string]*node, errs *[]ScanError) {
	entries, err := os.ReadDir(t.path)
	if err != nil {
		mu.Lock()
		*errs = append(*errs, ScanError{Path: t.path, Error: err.Error()})
		mu.Unlock()
		return
	}

	for _, entry := range entries {
		childPath := filepath.Join(t.path, entry.Name())
		if matcher.excluded(childPath, entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			mu.Lock()
			*errs = append(*errs, ScanError{Path: childPath, Error: err.Error()})
			mu.Unlock()
			continue
		}

		if info.IsDir() {
			mu.Lock()
			nodes[childPath] = &node{path: childPath, parent: t.path}
			nodes[t.path].children = append(nodes[t.path].children, childPath)
			mu.Unlock()
			wg.Add(1)
			enqueueTask(tasks, task{path: childPath, parent: t.path})
			continue
		}

		mu.Lock()
		nodes[t.path].directSize += info.Size()
		mu.Unlock()
	}
}

func enqueueTask(tasks chan<- task, t task) {
	select {
	case tasks <- t:
	default:
		go func() {
			tasks <- t
		}()
	}
}

func computeTotals(nodes map[string]*node, root string) int64 {
	visited := make(map[string]bool, len(nodes))
	var visit func(string) int64
	visit = func(path string) int64 {
		n := nodes[path]
		if n == nil {
			return 0
		}
		if visited[path] {
			return n.totalSize
		}
		visited[path] = true
		total := n.directSize
		for _, child := range n.children {
			total += visit(child)
		}
		n.totalSize = total
		return total
	}
	return visit(root)
}

func percent(size, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(size) / float64(total) * 100
}

type excludeMatcher struct {
	root     string
	patterns []string
}

func newExcludeMatcher(root string, patterns []string) excludeMatcher {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern != "" {
			normalized = append(normalized, pattern)
		}
	}
	return excludeMatcher{root: root, patterns: normalized}
}

func (m excludeMatcher) excluded(path, name string) bool {
	if len(m.patterns) == 0 {
		return false
	}
	rel, err := filepath.Rel(m.root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	abs := filepath.ToSlash(path)

	for _, pattern := range m.patterns {
		if matchGlob(pattern, name) || matchGlob(pattern, rel) || matchGlob(pattern, abs) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, value string) bool {
	ok, err := filepath.Match(pattern, value)
	if err == nil && ok {
		return true
	}
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, value)
	}
	return false
}

func matchDoubleStar(pattern, value string) bool {
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false
	}
	prefix, suffix := parts[0], parts[1]
	return strings.HasPrefix(value, strings.TrimSuffix(prefix, "/")) && strings.HasSuffix(value, strings.TrimPrefix(suffix, "/"))
}

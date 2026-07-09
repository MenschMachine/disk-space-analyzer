package scan

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type SizeMode string

const (
	SizeModeRecursive SizeMode = "recursive"
	SizeModeTopLevel  SizeMode = "top-level"
	readDirBatchSize           = 256
)

type Options struct {
	Limit            int
	SizeMode         SizeMode
	ExcludePatterns  []string
	Workers          int
	CrossFilesystem  bool
	NoDeviceCheck    bool
	RegularFilesOnly bool
	Progress         func(Snapshot)
	ProgressEvery    time.Duration
}

type Result struct {
	Root     string      `json:"root"`
	SizeMode SizeMode    `json:"size_mode"`
	Total    int64       `json:"total"`
	Entries  []Entry     `json:"entries"`
	Errors   []ScanError `json:"errors"`
}

type Snapshot struct {
	Root               string
	SizeMode           SizeMode
	Total              int64
	Entries            []Entry
	Errors             []ScanError
	DirectoriesScanned int
	Done               bool
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
	if isPseudo, err := isPseudoFilesystem(absRoot); err != nil {
		return Result{}, err
	} else if isPseudo {
		return emptyResult(absRoot, mode), nil
	}
	skipDeviceCheck := opts.CrossFilesystem || opts.NoDeviceCheck
	var rootDevice uint64
	if !skipDeviceCheck {
		rootDevice, err = deviceID(info)
		if err != nil {
			return Result{}, err
		}
	}

	matcher := newExcludeMatcher(absRoot, opts.ExcludePatterns)
	nodes := map[string]*node{
		absRoot: {path: absRoot},
	}
	var mu sync.Mutex
	errs := make([]ScanError, 0)
	tasks := make(chan task, workers*2)
	var wg sync.WaitGroup
	done := make(chan struct{})
	if opts.Progress != nil {
		every := opts.ProgressEvery
		if every <= 0 {
			every = 250 * time.Millisecond
		}
		go func() {
			ticker := time.NewTicker(every)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					opts.Progress(snapshot(absRoot, mode, opts.Limit, nodes, &errs, false, &mu))
				case <-done:
					return
				}
			}
		}()
	}

	worker := func() {
		for t := range tasks {
			scanOne(t, absRoot, matcher, rootDevice, skipDeviceCheck, opts.RegularFilesOnly, tasks, &wg, &mu, nodes, &errs)
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
	close(done)

	finalSnapshot := snapshot(absRoot, mode, opts.Limit, nodes, &errs, true, &mu)
	if opts.Progress != nil {
		opts.Progress(finalSnapshot)
	}

	return Result{
		Root:     finalSnapshot.Root,
		SizeMode: finalSnapshot.SizeMode,
		Total:    finalSnapshot.Total,
		Entries:  finalSnapshot.Entries,
		Errors:   finalSnapshot.Errors,
	}, nil
}

func snapshot(root string, mode SizeMode, limit int, nodes map[string]*node, errs *[]ScanError, done bool, mu *sync.Mutex) Snapshot {
	mu.Lock()
	nodeCopies := make(map[string]*node, len(nodes))
	for path, n := range nodes {
		children := append([]string(nil), n.children...)
		nodeCopies[path] = &node{
			path:       n.path,
			parent:     n.parent,
			directSize: n.directSize,
			children:   children,
		}
	}
	errCopies := make([]ScanError, len(*errs))
	copy(errCopies, *errs)
	mu.Unlock()

	total := computeTotals(nodeCopies, root)
	entries := make([]Entry, 0, len(nodeCopies))
	for _, n := range nodeCopies {
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
	if len(entries) > limit {
		entries = entries[:limit]
	}
	sort.Slice(errCopies, func(i, j int) bool {
		return errCopies[i].Path < errCopies[j].Path
	})

	return Snapshot{
		Root:               root,
		SizeMode:           mode,
		Total:              total,
		Entries:            entries,
		Errors:             errCopies,
		DirectoriesScanned: len(nodeCopies),
		Done:               done,
	}
}

func scanOne(t task, root string, matcher excludeMatcher, rootDevice uint64, skipDeviceCheck bool, regularFilesOnly bool, tasks chan<- task, wg *sync.WaitGroup, mu *sync.Mutex, nodes map[string]*node, errs *[]ScanError) {
	dir, err := os.Open(t.path)
	if err != nil {
		mu.Lock()
		*errs = append(*errs, ScanError{Path: t.path, Error: err.Error()})
		mu.Unlock()
		return
	}

	var directSize int64
	childDirs := make([]string, 0)
	localErrs := make([]ScanError, 0)

	for {
		entries, err := dir.ReadDir(readDirBatchSize)
		for _, entry := range entries {
			childPath := filepath.Join(t.path, entry.Name())
			if matcher.excluded(childPath, entry.Name()) {
				continue
			}

			entryType := entry.Type()
			if entryType.IsDir() {
				if skip, err := shouldSkipPseudoFilesystem(childPath); skip {
					continue
				} else if err != nil {
					localErrs = append(localErrs, ScanError{Path: childPath, Error: err.Error()})
					continue
				}
				if skipDeviceCheck {
					childDirs = append(childDirs, childPath)
					continue
				}

				info, err := entry.Info()
				if err != nil {
					localErrs = append(localErrs, ScanError{Path: childPath, Error: err.Error()})
					continue
				}
				skip, err := shouldSkipDevice(info, rootDevice)
				if err != nil {
					localErrs = append(localErrs, ScanError{Path: childPath, Error: err.Error()})
					continue
				}
				if skip {
					continue
				}

				childDirs = append(childDirs, childPath)
				continue
			}
			if regularFilesOnly && entryType != 0 {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				localErrs = append(localErrs, ScanError{Path: childPath, Error: err.Error()})
				continue
			}

			if info.IsDir() {
				if skip, err := shouldSkipPseudoFilesystem(childPath); skip {
					continue
				} else if err != nil {
					localErrs = append(localErrs, ScanError{Path: childPath, Error: err.Error()})
					continue
				}
				if !skipDeviceCheck {
					skip, err := shouldSkipDevice(info, rootDevice)
					if err != nil {
						localErrs = append(localErrs, ScanError{Path: childPath, Error: err.Error()})
						continue
					}
					if skip {
						continue
					}
				}

				childDirs = append(childDirs, childPath)
				continue
			}
			if regularFilesOnly && !info.Mode().IsRegular() {
				continue
			}

			directSize += info.Size()
		}

		if err != nil {
			if !errors.Is(err, io.EOF) {
				localErrs = append(localErrs, ScanError{Path: t.path, Error: err.Error()})
			}
			break
		}
	}
	if err := dir.Close(); err != nil {
		localErrs = append(localErrs, ScanError{Path: t.path, Error: err.Error()})
	}

	mu.Lock()
	current := nodes[t.path]
	if current != nil {
		current.directSize += directSize
		for _, childPath := range childDirs {
			nodes[childPath] = &node{path: childPath, parent: t.path}
			current.children = append(current.children, childPath)
		}
	}
	*errs = append(*errs, localErrs...)
	mu.Unlock()

	for _, childPath := range childDirs {
		wg.Add(1)
		enqueueTask(tasks, task{path: childPath, parent: t.path})
	}
}

func emptyResult(root string, mode SizeMode) Result {
	return Result{
		Root:     root,
		SizeMode: mode,
		Total:    0,
		Entries: []Entry{{
			Path:    root,
			Size:    0,
			Percent: 0,
		}},
		Errors: []ScanError{},
	}
}

func shouldSkipPseudoFilesystem(path string) (bool, error) {
	return isPseudoFilesystem(path)
}

func shouldSkipDevice(info os.FileInfo, rootDevice uint64) (bool, error) {
	childDevice, err := deviceID(info)
	if err != nil {
		return false, err
	}
	return childDevice != rootDevice, nil
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

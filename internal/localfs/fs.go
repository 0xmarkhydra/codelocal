package localfs

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

type FS struct {
	Root            string
	maxReadBytes    int64
	maxBatchBytes   int64
	maxListEntries  int
	readConcurrency int
	mu              sync.RWMutex
	ignore          []ignoreRule
}

type ignoreRule struct {
	pattern string
	negate  bool
	dirOnly bool
}

var indexNoiseDirs = map[string]struct{}{
	".cache": {}, ".dart_tool": {}, ".gradle": {}, ".next": {}, ".nuxt": {}, ".parcel-cache": {}, ".pytest_cache": {}, ".svelte-kit": {}, ".turbo": {},
	"__pycache__": {}, "build": {}, "coverage": {}, "deriveddata": {}, "dist": {}, "node_modules": {}, "pods": {}, "target": {}, "vendor": {}, "venv": {}, ".venv": {},
}

func codeLocalPortablePath(relative string) bool {
	value := strings.Trim(filepath.ToSlash(relative), "/")
	switch value {
	case ".codelocal", ".codelocal/project.json", ".codelocal/quality.json":
		return true
	}
	return value == ".codelocal/rules" || strings.HasPrefix(value, ".codelocal/rules/") ||
		value == ".codelocal/skills" || strings.HasPrefix(value, ".codelocal/skills/")
}

func codeLocalRuntimePath(relative string) bool {
	value := strings.Trim(filepath.ToSlash(relative), "/")
	if value == ".codelocal" || !strings.HasPrefix(value, ".codelocal/") {
		return false
	}
	return !codeLocalPortablePath(value)
}

func indexNoisePath(relative string) bool {
	for _, part := range strings.Split(strings.Trim(filepath.ToSlash(relative), "/"), "/") {
		if _, ignored := indexNoiseDirs[strings.ToLower(part)]; ignored {
			return true
		}
	}
	return false
}

func New(root string) (*FS, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	f := &FS{Root: real, maxReadBytes: int64(envInt("CODELOCAL_MAX_READ_BYTES", 2*1024*1024)), maxBatchBytes: int64(envInt("CODELOCAL_MAX_BATCH_BYTES", 8*1024*1024)), maxListEntries: envInt("CODELOCAL_MAX_LIST_ENTRIES", 10000), readConcurrency: envInt("CODELOCAL_READ_CONCURRENCY", 6)}
	_ = ensureCodeLocalGitIgnore(real)
	_ = f.ReloadIgnore()
	return f, nil
}

func envInt(name string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

const codeLocalExcludeBlock = `# BEGIN CodeLocal local-only state
.codelocal/*
!.codelocal/project.json
!.codelocal/quality.json
!.codelocal/rules/
!.codelocal/rules/**
!.codelocal/skills/
!.codelocal/skills/**
# END CodeLocal local-only state
`

func ensureCodeLocalLocalExclude(root string) error {
	path := filepath.Join(root, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), "# BEGIN CodeLocal local-only state") {
		return nil
	}
	content := strings.TrimRight(string(data), "\r\n")
	if content != "" {
		content += "\n"
	}
	return atomicWrite(path, []byte(content+codeLocalExcludeBlock), 0o644)
}

func ensureCodeLocalGitIgnore(root string) error {
	internalDir := filepath.Join(root, ".codelocal")
	if info, err := os.Stat(internalDir); err != nil || !info.IsDir() {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil
	}
	ignorePath := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(ignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	// Older CodeLocal releases ignored the entire .codelocal directory. That
	// accidentally made portable project identity/rules/skills impossible to
	// commit. Migrate the exact legacy blanket rule and keep only runtime
	// worktrees private.
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines)+1)
	hasRuntimeIgnore := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".codelocal/" || trimmed == ".codelocal" {
			continue
		}
		if trimmed == ".codelocal/worktrees/" || trimmed == ".codelocal/worktrees" {
			hasRuntimeIgnore = true
		}
		out = append(out, line)
	}
	if !hasRuntimeIgnore {
		out = append(out, ".codelocal/worktrees/")
	}
	content := strings.Trim(strings.Join(out, "\n"), "\n")
	if content != "" {
		content += "\n"
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(ignorePath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := atomicWrite(ignorePath, []byte(content), mode); err != nil {
		return err
	}
	return ensureCodeLocalLocalExclude(root)
}

func (f *FS) inside(candidate string) bool {
	if candidate == f.Root {
		return true
	}
	rel, err := filepath.Rel(f.Root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (f *FS) Rel(path string) string {
	rel, err := filepath.Rel(f.Root, path)
	if err != nil || rel == "" {
		return "."
	}
	return filepath.ToSlash(rel)
}

func (f *FS) lexical(relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("absolute paths are not allowed")
	}
	candidate := filepath.Clean(filepath.Join(f.Root, relative))
	if !f.inside(candidate) {
		return "", errors.New("path escapes PROJECT_ROOT")
	}
	return candidate, nil
}

func (f *FS) Existing(relative string) (string, error) {
	if security.IsSensitivePath(relative) {
		return "", fmt.Errorf("access blocked by sensitive-path policy: %s", relative)
	}
	candidate, err := f.lexical(relative)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !f.inside(real) {
		return "", errors.New("resolved path escapes PROJECT_ROOT (possible symlink traversal)")
	}
	return real, nil
}

func (f *FS) WritePath(relative string) (string, error) {
	if security.IsSensitivePath(relative) {
		return "", fmt.Errorf("access blocked by sensitive-path policy: %s", relative)
	}
	candidate, err := f.lexical(relative)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		return "", err
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return "", err
	}
	if !f.inside(realParent) {
		return "", errors.New("parent directory escapes PROJECT_ROOT")
	}
	if real, err := filepath.EvalSymlinks(candidate); err == nil {
		if !f.inside(real) {
			return "", errors.New("existing file escapes PROJECT_ROOT")
		}
		return real, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return filepath.Join(realParent, filepath.Base(candidate)), nil
}

func parseIgnore(content string) []ignoreRule {
	out := []ignoreRule{}
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := strings.HasPrefix(line, "!")
		if negate {
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimPrefix(filepath.ToSlash(line), "/")
		dirOnly := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		if line != "" {
			out = append(out, ignoreRule{pattern: line, negate: negate, dirOnly: dirOnly})
		}
	}
	return out
}

func (f *FS) ReloadIgnore() error {
	rules := []ignoreRule{{pattern: ".git", dirOnly: true}, {pattern: ".DS_Store"}}
	for _, path := range []string{filepath.Join(f.Root, ".gitignore"), filepath.Join(f.Root, ".git", "info", "exclude")} {
		data, err := os.ReadFile(path)
		if err == nil {
			rules = append(rules, parseIgnore(string(data))...)
		}
	}
	// Runtime worktrees are implementation details and must never become project
	// context. Portable .codelocal/project.json, rules and skills remain visible.
	rules = append(rules, ignoreRule{pattern: ".codelocal/worktrees", dirOnly: true})
	f.mu.Lock()
	f.ignore = rules
	f.mu.Unlock()
	return nil
}

func matchIgnore(pattern, path string, dirOnly, isDir bool) bool {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if dirOnly && !isDir {
		// A directory pattern still ignores children beneath the directory.
		if path != pattern && !strings.HasPrefix(path, pattern+"/") {
			return false
		}
	}
	if !strings.Contains(pattern, "/") {
		for _, part := range strings.Split(path, "/") {
			if ok, _ := filepath.Match(pattern, part); ok {
				return true
			}
		}
	}
	if ok, _ := filepath.Match(pattern, path); ok {
		return true
	}
	if strings.HasPrefix(path, pattern+"/") {
		return true
	}
	return false
}

func (f *FS) Ignored(relative string, isDir bool) bool {
	if codeLocalRuntimePath(relative) || indexNoisePath(relative) {
		return true
	}
	f.mu.RLock()
	rules := append([]ignoreRule(nil), f.ignore...)
	f.mu.RUnlock()
	ignored := false
	for _, rule := range rules {
		if matchIgnore(rule.pattern, relative, rule.dirOnly, isDir) {
			ignored = !rule.negate
		}
	}
	return ignored
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func binary(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	return bytes.IndexByte(data[:limit], 0) >= 0
}

func (f *FS) FileInfo(relative string) (map[string]any, error) {
	path, err := f.Existing(relative)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	rel := f.Rel(path)
	result := map[string]any{"path": rel, "size": stat.Size(), "mtimeMs": stat.ModTime().UnixMilli(), "ignored": f.Ignored(rel, stat.IsDir()), "isFile": stat.Mode().IsRegular(), "isDirectory": stat.IsDir()}
	if stat.Mode().IsRegular() && stat.Size() <= f.maxReadBytes*4 {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		result["hash"] = Hash(data)
	} else {
		result["hash"] = nil
	}
	return result, nil
}

func (f *FS) Read(relative string, startLine, endLine int) (map[string]any, error) {
	path, err := f.Existing(relative)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a file", relative)
	}
	if stat.Size() > f.maxReadBytes && startLine <= 0 {
		return nil, fmt.Errorf("%s: exceeds read limit; use read_file_range", relative)
	}
	meta := map[string]any{"path": f.Rel(path), "size": stat.Size(), "mtimeMs": stat.ModTime().UnixMilli(), "ignored": f.Ignored(f.Rel(path), false), "isFile": true, "isDirectory": false}
	probe, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	first := make([]byte, 8192)
	n, _ := probe.Read(first)
	_ = probe.Close()
	if binary(first[:n]) {
		meta["binary"] = true
		meta["content"] = nil
		meta["hash"] = nil
		if stat.Size() <= f.maxReadBytes*4 {
			if all, readErr := os.ReadFile(path); readErr == nil {
				meta["hash"] = Hash(all)
			}
		}
		return meta, nil
	}
	from := startLine
	if from <= 0 {
		from = 1
	}
	to := endLine
	selected := []string{}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hasher := sha256.New()
	reader := bufio.NewReader(io.TeeReader(file, hasher))
	lineNo := 0
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if lineNo >= from && (to <= 0 || lineNo <= to) {
				selected = append(selected, line)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return nil, readErr
			}
			break
		}
	}
	actualTo := lineNo
	if to > 0 && to < actualTo {
		actualTo = to
	}
	meta["binary"] = false
	meta["startLine"] = from
	meta["endLine"] = actualTo
	meta["totalLines"] = lineNo
	meta["content"] = strings.Join(selected, "\n")
	if stat.Size() <= f.maxReadBytes*4 {
		meta["hash"] = hex.EncodeToString(hasher.Sum(nil))
	} else {
		meta["hash"] = nil
	}
	return meta, nil
}

func (f *FS) ReadMany(paths []string) (map[string]any, error) {
	files := make([]map[string]any, len(paths))
	var total int64
	// Validate the whole batch before doing I/O. The old loop discovered an
	// oversized batch only after reading the file that crossed the limit.
	for _, path := range paths {
		absolute, err := f.Existing(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: not a file", path)
		}
		total += info.Size()
		if total > f.maxBatchBytes {
			return nil, errors.New("batch exceeds configured read limit")
		}
	}
	if len(paths) == 0 {
		return map[string]any{"files": files, "totalBytes": total}, nil
	}

	workers := f.readConcurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(paths) {
		workers = len(paths)
	}
	type job struct {
		index int
		path  string
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				value, err := f.Read(item.path, 0, 0)
				if err != nil {
					errOnce.Do(func() { firstErr = err })
					continue
				}
				files[item.index] = value
			}
		}()
	}
	for index, path := range paths {
		jobs <- job{index: index, path: path}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return map[string]any{"files": files, "totalBytes": total}, nil
}

func (f *FS) List(start string, maxDepth int, includeIgnored bool) (map[string]any, error) {
	path, err := f.Existing(start)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, errors.New("path is not a directory")
	}
	if maxDepth < 0 {
		maxDepth = 0
	}
	entries := []map[string]any{}
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		children, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			if len(entries) >= f.maxListEntries {
				return nil
			}
			absolute := filepath.Join(dir, child.Name())
			rel := f.Rel(absolute)
			info, infoErr := child.Info()
			if infoErr != nil {
				continue
			}
			if codeLocalRuntimePath(rel) {
				continue
			}
			sensitive := security.IsSensitivePath(rel)
			ignored := f.Ignored(rel, info.IsDir())
			if sensitive {
				typeName := "file"
				if info.IsDir() {
					typeName = "directory"
				}
				entries = append(entries, map[string]any{"path": rel, "type": typeName, "sensitive": true})
				continue
			}
			if ignored && !includeIgnored {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				entries = append(entries, map[string]any{"path": rel, "type": "symlink", "ignored": ignored})
				continue
			}
			if info.IsDir() {
				entries = append(entries, map[string]any{"path": rel + "/", "type": "directory", "ignored": ignored})
				if depth < maxDepth {
					if err := walk(absolute, depth+1); err != nil {
						return err
					}
				}
			} else {
				entries = append(entries, map[string]any{"path": rel, "type": "file", "ignored": ignored})
			}
		}
		return nil
	}
	if err := walk(path, 0); err != nil {
		return nil, err
	}
	return map[string]any{"entries": entries, "truncated": len(entries) >= f.maxListEntries, "includeIgnored": includeIgnored}, nil
}

func runSearch(cwd, command string, args []string, maxResults int, accept func(string) bool) (matches []string, truncated bool, err error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || (accept != nil && !accept(line)) {
			continue
		}
		matches = append(matches, line)
		if len(matches) > maxResults {
			truncated = true
			_ = cmd.Process.Kill()
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	if scanErr != nil && !truncated {
		return nil, false, scanErr
	}
	if waitErr != nil && !truncated {
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) || exit.ExitCode() != 1 {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = waitErr.Error()
			}
			return nil, false, errors.New(message)
		}
	}
	return matches, truncated, nil
}

func (f *FS) Search(query, start string, maxResults int, fixed, includeIgnored bool) (map[string]any, error) {
	cwd, err := f.Existing(start)
	if err != nil {
		return nil, err
	}
	if maxResults <= 0 {
		maxResults = 200
	}
	args := []string{"--line-number", "--column", "--no-heading", "--color", "never", "--hidden"}
	if includeIgnored {
		args = append(args, "--no-ignore")
	}
	if fixed {
		args = append(args, "--fixed-strings")
	}
	args = append(args, "--glob", "!.git/**", "--glob", "!.codelocal/worktrees/**", "--glob", "!.codelocal/tmp/**", "--glob", "!.codelocal/cache/**", "--glob", "!.codelocal/state/**", "--glob", "!.codelocal/logs/**")
	for name := range indexNoiseDirs {
		args = append(args, "--glob", "!**/"+name+"/**")
	}
	args = append(args, "--", query, ".")
	accept := func(line string) bool {
		filePart := strings.TrimPrefix(strings.SplitN(line, ":", 2)[0], "./")
		return !codeLocalRuntimePath(filePart) && !indexNoisePath(filePart) && !security.IsSensitivePath(filePart)
	}
	matches, truncated, runErr := runSearch(cwd, "rg", args, maxResults, accept)
	if runErr != nil {
		grepArgs := []string{"-RIn", "--exclude-dir=.git"}
		for name := range indexNoiseDirs {
			grepArgs = append(grepArgs, "--exclude-dir="+name)
		}
		if fixed {
			grepArgs = append(grepArgs, "-F")
		}
		grepArgs = append(grepArgs, "--", query, ".")
		matches, truncated, runErr = runSearch(cwd, "grep", grepArgs, maxResults, accept)
	}
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"matches": matches, "truncated": truncated, "includeIgnored": includeIgnored}, nil
}

func (f *FS) Write(relative, content, expectedHash string) (map[string]any, error) {
	path, err := f.WritePath(relative)
	if err != nil {
		return nil, err
	}
	if expectedHash != "" {
		current, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		if Hash(current) != expectedHash {
			return nil, errors.New("file changed since read; hash mismatch")
		}
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := atomicWrite(path, []byte(content), mode); err != nil {
		return nil, err
	}
	_ = f.ReloadIgnore()
	return f.FileInfo(relative)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".codelocal-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func adaptExactEditLineEndings(text, oldText, newText string) (string, string) {
	if strings.Contains(text, oldText) {
		return oldText, newText
	}
	oldLF := strings.ReplaceAll(oldText, "\r\n", "\n")
	newLF := strings.ReplaceAll(newText, "\r\n", "\n")
	if strings.Contains(text, "\r\n") && strings.Contains(oldLF, "\n") {
		candidate := strings.ReplaceAll(oldLF, "\n", "\r\n")
		if strings.Contains(text, candidate) {
			return candidate, strings.ReplaceAll(newLF, "\n", "\r\n")
		}
	}
	if !strings.Contains(text, "\r\n") && strings.Contains(oldText, "\r\n") && strings.Contains(text, oldLF) {
		return oldLF, newLF
	}
	return oldText, newText
}

func (f *FS) ExactEdit(relative, oldText, newText string, replaceAll bool, expectedHash string) (map[string]any, error) {
	path, err := f.Existing(relative)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if expectedHash != "" && Hash(data) != expectedHash {
		return nil, errors.New("file changed since read; hash mismatch")
	}
	text := string(data)
	oldText, newText = adaptExactEditLineEndings(text, oldText, newText)
	count := strings.Count(text, oldText)
	if count == 0 {
		return nil, errors.New("oldText not found")
	}
	if count > 1 && !replaceAll {
		return nil, fmt.Errorf("oldText occurs %d times; use replaceAll or a more specific edit", count)
	}
	updated := text
	if replaceAll {
		updated = strings.ReplaceAll(text, oldText, newText)
	} else {
		updated = strings.Replace(text, oldText, newText, 1)
	}
	info, _ := os.Stat(path)
	mode := os.FileMode(0o644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	if err := atomicWrite(path, []byte(updated), mode); err != nil {
		return nil, err
	}
	return map[string]any{"path": relative, "replacements": func() int {
		if replaceAll {
			return count
		}
		return 1
	}(), "beforeHash": Hash(data), "afterHash": Hash([]byte(updated)), "bytes": len([]byte(updated)), "mtimeMs": time.Now().UnixMilli()}, nil
}

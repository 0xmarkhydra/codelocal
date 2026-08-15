package editing

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

type Edit struct {
	StartOffset *int   `json:"startOffset,omitempty"`
	EndOffset   *int   `json:"endOffset,omitempty"`
	StartLine   *int   `json:"startLine,omitempty"`
	StartColumn *int   `json:"startColumn,omitempty"`
	EndLine     *int   `json:"endLine,omitempty"`
	EndColumn   *int   `json:"endColumn,omitempty"`
	Replacement string `json:"replacement"`
}

type FileEdit struct {
	Path         string `json:"path"`
	ExpectedHash string `json:"expectedHash,omitempty"`
	Edits        []Edit `json:"edits"`
}

type Engine struct{ FS *localfs.FS }

func New(fs *localfs.FS) *Engine { return &Engine{FS: fs} }

func lineOffset(text string, line, column int) (int, error) {
	if line < 1 || column < 1 {
		return 0, errors.New("line and column must be >= 1")
	}
	if line == 1 {
		if column-1 > len(text) {
			return 0, errors.New("column exceeds line")
		}
		return column - 1, nil
	}
	offset := 0
	current := 1
	for current < line {
		idx := strings.IndexByte(text[offset:], '\n')
		if idx < 0 {
			return 0, errors.New("line exceeds file")
		}
		offset += idx + 1
		current++
	}
	lineEnd := strings.IndexByte(text[offset:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text) - offset
	}
	if column-1 > lineEnd {
		return 0, errors.New("column exceeds line")
	}
	return offset + column - 1, nil
}

func normalizeEdit(text string, e Edit) (start, end int, err error) {
	if e.StartOffset != nil || e.EndOffset != nil {
		if e.StartOffset == nil || e.EndOffset == nil {
			return 0, 0, errors.New("startOffset and endOffset must be supplied together")
		}
		start, end = *e.StartOffset, *e.EndOffset
	} else {
		if e.StartLine == nil || e.StartColumn == nil || e.EndLine == nil || e.EndColumn == nil {
			return 0, 0, errors.New("edit needs offsets or line/column range")
		}
		start, err = lineOffset(text, *e.StartLine, *e.StartColumn)
		if err != nil {
			return
		}
		end, err = lineOffset(text, *e.EndLine, *e.EndColumn)
		if err != nil {
			return
		}
	}
	if start < 0 || end < start || end > len(text) {
		return 0, 0, errors.New("edit range is outside file")
	}
	return
}

func (e *Engine) Apply(files []FileEdit) (map[string]any, error) {
	if len(files) == 0 {
		return nil, errors.New("files required")
	}
	if len(files) > 100 {
		return nil, errors.New("too many files")
	}
	type prepared struct {
		input  FileEdit
		path   string
		before []byte
		after  []byte
		mode   os.FileMode
	}
	preparedFiles := []prepared{}
	seen := map[string]struct{}{}
	for _, file := range files {
		if security.IsSensitivePath(file.Path) {
			return nil, fmt.Errorf("access blocked by sensitive-path policy: %s", file.Path)
		}
		path, err := e.FS.Existing(file.Path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[path]; ok {
			return nil, fmt.Errorf("duplicate file: %s", file.Path)
		}
		seen[path] = struct{}{}
		before, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if file.ExpectedHash != "" && localfs.Hash(before) != file.ExpectedHash {
			return nil, fmt.Errorf("%s changed since read; hash mismatch", file.Path)
		}
		text := string(before)
		type normalized struct {
			start, end  int
			replacement string
		}
		edits := []normalized{}
		for _, edit := range file.Edits {
			start, end, err := normalizeEdit(text, edit)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", file.Path, err)
			}
			edits = append(edits, normalized{start: start, end: end, replacement: edit.Replacement})
		}
		sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
		for i := 1; i < len(edits); i++ {
			if edits[i].start < edits[i-1].end {
				return nil, fmt.Errorf("%s has overlapping edits", file.Path)
			}
		}
		var out bytes.Buffer
		cursor := 0
		for _, edit := range edits {
			out.WriteString(text[cursor:edit.start])
			out.WriteString(edit.replacement)
			cursor = edit.end
		}
		out.WriteString(text[cursor:])
		info, _ := os.Stat(path)
		mode := os.FileMode(0o644)
		if info != nil {
			mode = info.Mode().Perm()
		}
		preparedFiles = append(preparedFiles, prepared{input: file, path: path, before: before, after: out.Bytes(), mode: mode})
	}
	written := []prepared{}
	for _, file := range preparedFiles {
		if err := writeAtomic(file.path, file.after, file.mode); err != nil {
			for i := len(written) - 1; i >= 0; i-- {
				_ = writeAtomic(written[i].path, written[i].before, written[i].mode)
			}
			return nil, err
		}
		written = append(written, file)
	}
	results := []map[string]any{}
	for _, file := range preparedFiles {
		results = append(results, map[string]any{"path": file.input.Path, "beforeHash": localfs.Hash(file.before), "afterHash": localfs.Hash(file.after), "bytes": len(file.after), "edits": len(file.input.Edits)})
	}
	return map[string]any{"changed": results, "atomicValidation": true, "rollbackOnFailure": true}, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".codelocal-edit-*.tmp")
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

func patchPaths(patch string) ([]string, error) {
	paths := []string{}
	scanner := bufio.NewScanner(strings.NewReader(patch))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			raw := strings.TrimSpace(line[4:])
			if raw == "/dev/null" {
				continue
			}
			raw = strings.TrimPrefix(raw, "a/")
			raw = strings.TrimPrefix(raw, "b/")
			raw = strings.Split(raw, "\t")[0]
			raw = strings.Trim(raw, "\"")
			if filepath.IsAbs(raw) || raw == ".." || strings.HasPrefix(raw, "../") || strings.Contains(filepath.ToSlash(raw), "/../") {
				return nil, errors.New("unsafe patch path")
			}
			if security.IsSensitivePath(raw) {
				return nil, errors.New("patch touches sensitive path")
			}
			paths = append(paths, raw)
		}
	}
	return paths, scanner.Err()
}

func (e *Engine) ApplyPatch(patch string) (map[string]any, error) {
	if strings.TrimSpace(patch) == "" {
		return nil, errors.New("patch required")
	}
	paths, err := patchPaths(patch)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("patch contains no file paths")
	}
	cmd := exec.Command("git", "apply", "--check", "--recount", "--whitespace=nowarn", "-")
	cmd.Dir = e.FS.Root
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("patch validation failed: %s", strings.TrimSpace(stderr.String()))
	}
	cmd = exec.Command("git", "apply", "--recount", "--whitespace=nowarn", "-")
	cmd.Dir = e.FS.Root
	cmd.Stdin = strings.NewReader(patch)
	stderr.Reset()
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("patch apply failed: %s", strings.TrimSpace(stderr.String()))
	}
	unique := map[string]struct{}{}
	for _, p := range paths {
		unique[p] = struct{}{}
	}
	out := []string{}
	for p := range unique {
		out = append(out, p)
	}
	sort.Strings(out)
	return map[string]any{"applied": true, "paths": out}, nil
}

func Int(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case jsonNumber:
		if n, err := strconv.Atoi(string(v)); err == nil {
			return n
		}
	}
	return fallback
}

type jsonNumber string

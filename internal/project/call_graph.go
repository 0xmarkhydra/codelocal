package project

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (e *Engine) CallersAt(ctx context.Context, path string, line, column int, name string, limit int) ([]map[string]any, error) {
	limit = boundedCallGraphLimit(limit)
	fallbackReason := "position_unavailable"
	if strings.TrimSpace(path) != "" {
		absolute, err := e.FS.Existing(path)
		if err != nil {
			return nil, err
		}
		fallbackReason = "lsp_unavailable"
		if e.LSP != nil && e.LSP.Available(absolute) {
			values, lspErr := e.LSP.IncomingCalls(ctx, absolute, line, column)
			if lspErr == nil && len(values) > 0 {
				return e.normalizeCallGraphPaths(values, limit), nil
			}
			if lspErr != nil {
				fallbackReason = "lsp_error"
			} else {
				fallbackReason = "lsp_empty"
			}
		}
	}
	values, err := e.callerTextFallback(name, limit)
	if err != nil {
		return nil, err
	}
	markFallback(values, "text", fallbackReason, .35)
	return values, nil
}

func (e *Engine) CalleesAt(ctx context.Context, path string, line, column int, name string, limit int) ([]map[string]any, error) {
	limit = boundedCallGraphLimit(limit)
	fallbackReason := "position_unavailable"
	if strings.TrimSpace(path) != "" {
		absolute, err := e.FS.Existing(path)
		if err != nil {
			return nil, err
		}
		fallbackReason = "lsp_unavailable"
		if e.LSP != nil && e.LSP.Available(absolute) {
			values, lspErr := e.LSP.OutgoingCalls(ctx, absolute, line, column)
			if lspErr == nil && len(values) > 0 {
				return e.normalizeCallGraphPaths(values, limit), nil
			}
			if lspErr != nil {
				fallbackReason = "lsp_error"
			} else {
				fallbackReason = "lsp_empty"
			}
		}
		if strings.EqualFold(filepath.Ext(absolute), ".go") {
			values, astErr := goCallees(absolute, line, name, limit)
			if astErr == nil && len(values) > 0 {
				for _, value := range values {
					value["path"] = e.FS.Rel(absolute)
					value["fallbackReason"] = fallbackReason
					e.annotatePathMap(value)
				}
				return values, nil
			}
		}
	}
	values, err := e.regexCalleesFallback(name, limit)
	if err != nil {
		return nil, err
	}
	markFallback(values, "text", fallbackReason, .25)
	return values, nil
}

func (e *Engine) Callers(name string, limit int) ([]map[string]any, error) {
	return e.CallersAt(context.Background(), "", 1, 1, name, limit)
}

func (e *Engine) Callees(name string, limit int) ([]map[string]any, error) {
	return e.CalleesAt(context.Background(), "", 1, 1, name, limit)
}

func boundedCallGraphLimit(limit int) int {
	if limit <= 0 {
		return 300
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func (e *Engine) normalizeCallGraphPaths(values []map[string]any, limit int) []map[string]any {
	for _, value := range values {
		if raw, ok := value["path"].(string); ok && raw != "" {
			value["path"] = e.FS.Rel(raw)
		}
		e.annotatePathMap(value)
		e.annotateCanonicalSymbol(value)
	}
	if len(values) > limit {
		return values[:limit]
	}
	return values
}

func markFallback(values []map[string]any, mode, reason string, confidence float64) {
	for _, value := range values {
		value["resolutionMode"] = mode
		value["fallbackReason"] = reason
		value["confidence"] = confidence
		value["relation"] = "CALLS"
	}
}

func (e *Engine) callerTextFallback(name string, limit int) ([]map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return []map[string]any{}, nil
	}
	searchLimit := limit * 3
	if searchLimit > 1000 {
		searchLimit = 1000
	}
	values, err := e.FindByText(name+"(", searchLimit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, min(limit, len(values)))
	for _, value := range values {
		path := fmt.Sprint(value["path"])
		line := 0
		fmt.Sscan(fmt.Sprint(value["line"]), &line)
		if line > 0 && e.isDeclarationLine(path, line, name) {
			continue
		}
		value["direction"] = "incoming"
		out = append(out, e.annotatePathMap(value))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *Engine) isDeclarationLine(path string, line int, name string) bool {
	read, err := e.FS.Read(path, line, line)
	if err != nil {
		return false
	}
	content, _ := read["content"].(string)
	quoted := regexp.QuoteMeta(name)
	patterns := []string{
		`^\s*func\s+(?:\([^)]*\)\s*)?` + quoted + `\s*\(`,
		`^\s*(?:export\s+)?(?:async\s+)?(?:function|def|fn)\s+` + quoted + `\s*\(`,
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(content) {
			return true
		}
	}
	return false
}

func (e *Engine) regexCalleesFallback(name string, limit int) ([]map[string]any, error) {
	defs, err := e.Definition(name, 1)
	if err != nil || len(defs) == 0 {
		return []map[string]any{}, err
	}
	path := fmt.Sprint(defs[0]["path"])
	line := 1
	fmt.Sscan(fmt.Sprint(defs[0]["line"]), &line)
	read, err := e.FS.Read(path, line, line+80)
	if err != nil {
		return nil, err
	}
	content, _ := read["content"].(string)
	re := regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	seen := map[string]struct{}{}
	out := []map[string]any{}
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		callee := match[1]
		if callee == name {
			continue
		}
		if _, ok := seen[callee]; ok {
			continue
		}
		seen[callee] = struct{}{}
		out = append(out, e.annotatePathMap(map[string]any{
			"name": callee, "path": path, "provider": "text-callgraph", "direction": "outgoing",
		}))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func goCallees(path string, line int, name string, limit int) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	decl := selectGoFunction(fset, file, line, name)
	if decl == nil || decl.Body == nil {
		return []map[string]any{}, nil
	}
	seen := map[string]struct{}{}
	out := []map[string]any{}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := goCallName(call.Fun)
		if callee == "" || callee == name {
			return true
		}
		if _, exists := seen[callee]; exists {
			return true
		}
		seen[callee] = struct{}{}
		position := fset.Position(call.Fun.Pos())
		out = append(out, map[string]any{
			"name": callee, "line": position.Line, "column": position.Column,
			"provider": "go-ast", "resolutionMode": "ast", "confidence": .75,
			"relation": "CALLS", "direction": "outgoing",
		})
		return len(out) < limit
	})
	return out, nil
}

func selectGoFunction(fset *token.FileSet, file *ast.File, line int, name string) *ast.FuncDecl {
	var named *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		if line >= start && line <= end {
			return fn
		}
		if named == nil && name != "" && fn.Name.Name == name {
			named = fn
		}
	}
	return named
}

func goCallName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := goCallName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.IndexExpr:
		return goCallName(value.X)
	case *ast.IndexListExpr:
		return goCallName(value.X)
	case *ast.ParenExpr:
		return goCallName(value.X)
	default:
		return ""
	}
}

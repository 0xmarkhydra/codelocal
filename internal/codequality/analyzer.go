package codequality

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Language string `json:"language"`
	Path     string `json:"path"`
	Symbol   string `json:"symbol,omitempty"`
	Line     int    `json:"line,omitempty"`
	Actual   int    `json:"actual,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Message  string `json:"message"`
}

type FileMetrics struct {
	Path          string `json:"path"`
	Language      string `json:"language"`
	ExecutableLOC int    `json:"executableLoc"`
	Functions     int    `json:"functions"`
}

type Report struct {
	Status         string        `json:"status"`
	ConfigSource   string        `json:"configSource"`
	ConfigError    string        `json:"configError,omitempty"`
	FilesAnalyzed  int           `json:"filesAnalyzed"`
	FilesExempt    int           `json:"filesExempt"`
	AdvisoryCount  int           `json:"advisoryCount"`
	BlockingCount  int           `json:"blockingCount"`
	Findings       []Finding     `json:"findings"`
	FileMetrics    []FileMetrics `json:"fileMetrics,omitempty"`
	LineDefinition string        `json:"lineDefinition"`
}

type goFunctionMetrics struct {
	Name            string
	Line            int
	ExecutableLOC   int
	NestingDepth    int
	Complexity      int
	Parameters      int
	CallCount       int
	OwnedStatements int
}

func Analyze(root string, paths []string) Report {
	policy, source, err := Load(root)
	report := Report{
		Status: "ok", ConfigSource: source, Findings: []Finding{}, FileMetrics: []FileMetrics{},
		LineDefinition: "token-bearing source lines; blank and comment-only lines are excluded",
	}
	if err != nil {
		report.Status = "config_error"
		report.ConfigError = err.Error()
		return report
	}
	goPolicy, enabled := policy.Languages["go"]
	if !enabled {
		return report
	}
	for _, relative := range uniquePaths(paths) {
		if filepath.Ext(relative) != ".go" {
			continue
		}
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		data, readErr := os.ReadFile(absolute)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			report.Findings = append(report.Findings, Finding{
				Rule: "analysis", Severity: SeverityAdvisory, Language: "go", Path: relative,
				Message: "Code quality analysis could not read this file.",
			})
			continue
		}
		if exemptPath(relative, data, policy.Exemptions) {
			report.FilesExempt++
			continue
		}
		metrics, findings := analyzeGoFile(relative, data, goPolicy)
		report.FilesAnalyzed++
		report.FileMetrics = append(report.FileMetrics, metrics)
		report.Findings = append(report.Findings, findings...)
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		left, right := report.Findings[i], report.Findings[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Rule != right.Rule {
			return left.Rule < right.Rule
		}
		return left.Symbol < right.Symbol
	})
	for _, finding := range report.Findings {
		if finding.Severity == SeverityBlocking {
			report.BlockingCount++
		} else {
			report.AdvisoryCount++
		}
	}
	if len(report.Findings) > 0 {
		report.Status = "findings"
	}
	return report
}

func uniquePaths(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range values {
		value := filepath.ToSlash(strings.TrimSpace(raw))
		value = strings.TrimPrefix(value, "./")
		if value == "" || value == "." || strings.HasPrefix(value, "../") || filepath.IsAbs(value) {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isGeneratedGo(data []byte) bool {
	prefix := data
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}
	text := strings.ToLower(string(prefix))
	return strings.Contains(text, "code generated") && strings.Contains(text, "do not edit")
}

func migrationPath(relative string) bool {
	value := strings.ToLower(filepath.ToSlash(relative))
	parts := strings.Split(value, "/")
	for _, part := range parts[:max(0, len(parts)-1)] {
		if part == "migration" || part == "migrations" {
			return true
		}
	}
	base := path.Base(value)
	return strings.HasPrefix(base, "migration_") || strings.HasSuffix(strings.TrimSuffix(base, ".go"), "_migration")
}

func pathPatternMatch(pattern, relative string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	relative = filepath.ToSlash(strings.TrimPrefix(relative, "./"))
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "**")
		return strings.HasPrefix(relative, prefix)
	}
	matched, err := path.Match(pattern, relative)
	return err == nil && matched
}

func exemptPath(relative string, data []byte, exemptions Exemptions) bool {
	if exemptions.Generated && isGeneratedGo(data) {
		return true
	}
	if exemptions.Tests && strings.HasSuffix(strings.ToLower(relative), "_test.go") {
		return true
	}
	if exemptions.Migrations && migrationPath(relative) {
		return true
	}
	for _, pattern := range exemptions.Paths {
		if pathPatternMatch(pattern, relative) {
			return true
		}
	}
	return false
}

func executableLineSet(file *token.File, data []byte) map[int]struct{} {
	lines := map[int]struct{}{}
	if file == nil {
		return lines
	}
	var scan scanner.Scanner
	scan.Init(file, data, nil, scanner.ScanComments)
	for {
		position, tok, _ := scan.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT || tok == token.SEMICOLON {
			continue
		}
		line := file.Line(position)
		if line > 0 {
			lines[line] = struct{}{}
		}
	}
	return lines
}

func linesWithin(lines map[int]struct{}, start, end int) int {
	count := 0
	for line := range lines {
		if line >= start && line <= end {
			count++
		}
	}
	return count
}

func parameterCount(fn *ast.FuncDecl) int {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return 0
	}
	count := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count
}

type nestingVisitor struct {
	depth    int
	maxDepth *int
}

func (v nestingVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	depth := v.depth
	switch node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		depth++
		if depth > *v.maxDepth {
			*v.maxDepth = depth
		}
	}
	return nestingVisitor{depth: depth, maxDepth: v.maxDepth}
}

func cyclomaticComplexity(fn *ast.FuncDecl) int {
	complexity := 1
	if fn == nil || fn.Body == nil {
		return complexity
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			if len(value.List) > 0 {
				complexity++
			}
		case *ast.CommClause:
			if value.Comm != nil {
				complexity++
			}
		case *ast.BinaryExpr:
			if value.Op == token.LAND || value.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

func directCall(expr ast.Expr) bool {
	_, ok := expr.(*ast.CallExpr)
	return ok
}

func coordinationReturn(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.BasicLit, *ast.CallExpr, *ast.IndexExpr, *ast.IndexListExpr:
		return true
	default:
		return false
	}
}

func ownedStatementCount(fn *ast.FuncDecl) int {
	if fn == nil || fn.Body == nil {
		return 0
	}
	count := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.AssignStmt:
			owned := false
			for _, expr := range stmt.Rhs {
				if !directCall(expr) {
					owned = true
					break
				}
			}
			if owned {
				count++
			}
		case *ast.IncDecStmt, *ast.SendStmt:
			count++
		case *ast.DeclStmt:
			if declaration, ok := stmt.Decl.(*ast.GenDecl); ok {
				for _, spec := range declaration.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, expr := range value.Values {
						if !directCall(expr) {
							count++
							break
						}
					}
				}
			}
		case *ast.ExprStmt:
			if !directCall(stmt.X) {
				count++
			}
		case *ast.ReturnStmt:
			for _, expr := range stmt.Results {
				if !coordinationReturn(expr) {
					count++
					break
				}
			}
		}
		return true
	})
	return count
}

func callCount(fn *ast.FuncDecl) int {
	count := 0
	if fn == nil || fn.Body == nil {
		return count
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.CallExpr); ok {
			count++
		}
		return true
	})
	return count
}

func functionMetrics(fn *ast.FuncDecl, fset *token.FileSet, executableLines map[int]struct{}) goFunctionMetrics {
	metrics := goFunctionMetrics{Name: fn.Name.Name, Complexity: cyclomaticComplexity(fn), Parameters: parameterCount(fn), CallCount: callCount(fn), OwnedStatements: ownedStatementCount(fn)}
	start, end := fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line
	metrics.Line = start
	metrics.ExecutableLOC = linesWithin(executableLines, start, end)
	ast.Walk(nestingVisitor{maxDepth: &metrics.NestingDepth}, fn.Body)
	return metrics
}

func severityFinding(policy LanguagePolicy, rule, relative, symbol string, line, actual, limit int, message string) Finding {
	return Finding{Rule: rule, Severity: SeverityFor(policy, rule), Language: "go", Path: relative, Symbol: symbol, Line: line, Actual: actual, Limit: limit, Message: message}
}

func overLimit(actual, limit int) bool { return limit > 0 && actual > limit }

func thresholdPressure(actual, limit int) bool {
	if limit <= 0 {
		return false
	}
	return actual*4 >= limit*3
}

func structuralResponsibilityPressure(metrics goFunctionMetrics, policy LanguagePolicy) int {
	signals := 0
	if thresholdPressure(metrics.ExecutableLOC, policy.MaxFunctionLines) {
		signals++
	}
	if thresholdPressure(metrics.NestingDepth, policy.MaxNestingDepth) {
		signals++
	}
	if thresholdPressure(metrics.Complexity, policy.MaxCyclomaticComplexity) {
		signals++
	}
	if metrics.CallCount >= 12 {
		signals++
	}
	if metrics.OwnedStatements >= 12 {
		signals++
	}
	return signals
}

func matchesOrchestrator(name string, policy OrchestratorPolicy) bool {
	if !policy.Enabled {
		return false
	}
	for _, explicit := range policy.ExplicitFunctions {
		if name == explicit {
			return true
		}
	}
	for _, pattern := range policy.NamePatterns {
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return true
		}
	}
	return false
}

func analyzeGoFile(relative string, data []byte, policy LanguagePolicy) (FileMetrics, []Finding) {
	metrics := FileMetrics{Path: relative, Language: "go"}
	findings := []Finding{}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, relative, data, parser.SkipObjectResolution)
	if err != nil {
		findings = append(findings, Finding{Rule: "parse", Severity: SeverityAdvisory, Language: "go", Path: relative, Message: "Go AST quality analysis skipped because the file does not parse."})
		return metrics, findings
	}
	fileToken := fset.File(parsed.Pos())
	executableLines := executableLineSet(fileToken, data)
	metrics.ExecutableLOC = len(executableLines)
	if overLimit(metrics.ExecutableLOC, policy.MaxFileLines) {
		findings = append(findings, severityFinding(policy, RuleMaxFileLines, relative, "", 1, metrics.ExecutableLOC, policy.MaxFileLines, "File executable LOC exceeds the configured project limit."))
	}
	for _, declaration := range parsed.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		metrics.Functions++
		fnMetrics := functionMetrics(fn, fset, executableLines)
		if overLimit(fnMetrics.ExecutableLOC, policy.MaxFunctionLines) {
			findings = append(findings, severityFinding(policy, RuleMaxFunctionLines, relative, fnMetrics.Name, fnMetrics.Line, fnMetrics.ExecutableLOC, policy.MaxFunctionLines, "Function executable LOC exceeds the configured project limit."))
		}
		if overLimit(fnMetrics.NestingDepth, policy.MaxNestingDepth) {
			findings = append(findings, severityFinding(policy, RuleMaxNestingDepth, relative, fnMetrics.Name, fnMetrics.Line, fnMetrics.NestingDepth, policy.MaxNestingDepth, "Function nesting depth exceeds the configured project limit."))
		}
		if overLimit(fnMetrics.Complexity, policy.MaxCyclomaticComplexity) {
			findings = append(findings, severityFinding(policy, RuleMaxCyclomaticComplexity, relative, fnMetrics.Name, fnMetrics.Line, fnMetrics.Complexity, policy.MaxCyclomaticComplexity, "Function cyclomatic complexity exceeds the configured project limit."))
		}
		if overLimit(fnMetrics.Parameters, policy.MaxParameters) {
			findings = append(findings, severityFinding(policy, RuleMaxParameters, relative, fnMetrics.Name, fnMetrics.Line, fnMetrics.Parameters, policy.MaxParameters, "Function parameter count exceeds the configured project limit."))
		}
		if policy.SingleResponsibility && structuralResponsibilityPressure(fnMetrics, policy) >= 3 {
			findings = append(findings, severityFinding(policy, RuleSingleResponsibility, relative, fnMetrics.Name, fnMetrics.Line, structuralResponsibilityPressure(fnMetrics, policy), 2, "Structural proxy suggests this function may own multiple responsibilities; review before splitting. This is advisory unless explicitly configured as blocking."))
		}
		if matchesOrchestrator(fnMetrics.Name, policy.Orchestrators) && overLimit(fnMetrics.OwnedStatements, policy.Orchestrators.MaxOwnedStatements) {
			findings = append(findings, severityFinding(policy, RuleOrchestratorOwnership, relative, fnMetrics.Name, fnMetrics.Line, fnMetrics.OwnedStatements, policy.Orchestrators.MaxOwnedStatements, "Orchestrator-like function appears to own business transformations instead of primarily coordinating calls/control flow."))
		}
	}
	return metrics, findings
}

func executableLOC(data []byte) int {
	fset := token.NewFileSet()
	file := fset.AddFile("source.go", -1, len(data))
	file.SetLinesForContent(data)
	return len(executableLineSet(file, bytes.Clone(data)))
}

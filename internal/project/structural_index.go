package project

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

func structuralSymbolsAndImports(path string, data []byte) ([]indexedSymbol, []string) {
	if strings.EqualFold(filepath.Ext(path), ".go") {
		if symbols, imports, ok := goStructuralSymbolsAndImports(path, data); ok {
			return symbols, imports
		}
	}
	return regexStructuralSymbolsAndImports(data)
}

func goStructuralSymbolsAndImports(path string, data []byte) ([]indexedSymbol, []string, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, false
	}

	symbols := []indexedSymbol{}
	for _, decl := range file.Decls {
		switch value := decl.(type) {
		case *ast.FuncDecl:
			position := fset.Position(value.Name.Pos())
			symbols = append(symbols, indexedSymbol{Name: value.Name.Name, Line: position.Line})
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					position := fset.Position(typed.Name.Pos())
					symbols = append(symbols, indexedSymbol{Name: typed.Name.Name, Line: position.Line})
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						position := fset.Position(name.Pos())
						symbols = append(symbols, indexedSymbol{Name: name.Name, Line: position.Line})
					}
				}
			}
		}
	}

	imports := make([]string, 0, len(file.Imports))
	seenImports := map[string]struct{}{}
	for _, item := range file.Imports {
		value, err := strconv.Unquote(item.Path.Value)
		if err != nil || strings.TrimSpace(value) == "" {
			continue
		}
		if _, exists := seenImports[value]; exists {
			continue
		}
		seenImports[value] = struct{}{}
		imports = append(imports, value)
	}
	return symbols, imports, true
}

func regexStructuralSymbolsAndImports(data []byte) ([]indexedSymbol, []string) {
	symbols := []indexedSymbol{}
	for _, match := range symbolRE.FindAllSubmatchIndex(data, -1) {
		symbols = append(symbols, indexedSymbol{
			Name: string(data[match[2]:match[3]]),
			Line: 1 + bytes.Count(data[:match[0]], []byte("\n")),
		})
	}
	imports := []string{}
	for _, pattern := range importPatterns {
		for _, match := range pattern.FindAllSubmatch(data, -1) {
			imports = append(imports, string(match[1]))
		}
	}
	return symbols, imports
}

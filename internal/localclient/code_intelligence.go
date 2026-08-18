package localclient

import (
	"context"
	"errors"
)

type codeLocation struct {
	Path   string
	Line   int
	Column int
	Name   string
}

func (e *Engine) codeLocation(args map[string]any) codeLocation {
	location := codeLocation{
		Path: asString(args["path"]), Line: asInt(args["line"], 1), Column: asInt(args["column"], 1),
		Name: first(asString(args["name"]), asString(args["query"])),
	}
	if location.Name == "" && location.Path != "" {
		location.Name = symbolAt(e, location.Path, location.Line, location.Column)
	}
	return location
}

func (e *Engine) handleCodeIntelligence(ctx context.Context, tool string, args map[string]any) (any, bool, error) {
	switch tool {
	case "semantic_info":
		return e.Project.SemanticInfo(), true, nil
	case "workspace_symbols", "find_symbol":
		symbols, err := e.Project.WorkspaceSymbols(ctx, asString(args["query"]), asInt(args["limit"], 200))
		return map[string]any{"symbols": symbols}, true, err
	case "document_symbols":
		symbols, err := e.Project.DocumentSymbolsAt(ctx, asString(args["path"]), asInt(args["limit"], 500))
		return map[string]any{"symbols": symbols}, true, err
	case "find_definition":
		location := e.codeLocation(args)
		items, err := e.Project.DefinitionAt(ctx, location.Path, location.Line, location.Column, location.Name, asInt(args["limit"], 100))
		return map[string]any{"definitions": items}, true, err
	case "find_references":
		location := e.codeLocation(args)
		items, err := e.Project.ReferencesAt(ctx, location.Path, location.Line, location.Column, location.Name, asInt(args["limit"], 500))
		return map[string]any{"references": items}, true, err
	case "find_implementations":
		location := e.codeLocation(args)
		if location.Name == "" {
			location.Name = symbolAt(e, location.Path, location.Line, location.Column)
		}
		items, err := e.Project.ImplementationsAt(ctx, location.Path, location.Line, location.Column, location.Name, asInt(args["limit"], 200))
		return map[string]any{"implementations": items}, true, err
	case "get_hover":
		item, err := e.Project.HoverAt(ctx, asString(args["path"]), asInt(args["line"], 1), asInt(args["column"], 1))
		return item, true, err
	case "get_diagnostics":
		item, err := e.Project.Diagnostics(ctx, asString(args["path"]), asInt(args["limit"], 500))
		return item, true, err
	case "get_callers":
		location := e.codeLocation(args)
		if location.Name == "" && location.Path == "" {
			return nil, true, errors.New("get_callers requires a symbol name or path/line/column")
		}
		items, err := e.Project.CallersAt(ctx, location.Path, location.Line, location.Column, location.Name, asInt(args["limit"], 300))
		return map[string]any{"callers": items}, true, err
	case "get_callees":
		location := e.codeLocation(args)
		if location.Name == "" && location.Path == "" {
			return nil, true, errors.New("get_callees requires a symbol name or path/line/column")
		}
		items, err := e.Project.CalleesAt(ctx, location.Path, location.Line, location.Column, location.Name, asInt(args["limit"], 300))
		return map[string]any{"callees": items}, true, err
	case "get_import_graph":
		items, err := e.Project.ImportGraph(asInt(args["limit"], 2000))
		return map[string]any{"edges": items}, true, err
	case "get_code_graph":
		item, err := e.Project.CodeGraphNeighborhoodView(ctx, asString(args["query"]), asString(args["repository"]), asString(args["view"]), asInt(args["depth"], 1), asInt(args["maxNodes"], 120))
		return item, true, err
	default:
		return nil, false, nil
	}
}

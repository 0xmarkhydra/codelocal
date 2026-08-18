package project

import (
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
)

type codeGraphStructuralRelation struct {
	From           map[string]any
	To             map[string]any
	Relation       string
	Provider       string
	ResolutionMode string
	Confidence     float64
}

type checkedGoPackage struct {
	FSet *token.FileSet
	Pkg  *types.Package
}

func goNamedType(value types.Type) *types.Named {
	switch typed := value.(type) {
	case *types.Named:
		return typed
	case *types.Pointer:
		return goNamedType(typed.Elem())
	default:
		return nil
	}
}

func goTypeGraphNode(fset *token.FileSet, object types.Object, kind string) map[string]any {
	if object == nil || object.Pos() == token.NoPos {
		return nil
	}
	position := fset.Position(object.Pos())
	if position.Filename == "" || position.Line <= 0 {
		return nil
	}
	return map[string]any{
		"name": object.Name(), "path": position.Filename, "line": position.Line, "column": position.Column,
		"kind": kind, "provider": "go-types", "resolutionMode": "type-analysis", "confidence": 1.0,
	}
}

func addGoTypeRelation(out *[]codeGraphStructuralRelation, seen map[string]struct{}, from, to map[string]any, relation string) {
	if from == nil || to == nil || strings.TrimSpace(relation) == "" {
		return
	}
	key := strings.Join([]string{
		relation, graphValue(from, "path"), graphValue(from, "name"), graphValue(from, "line"),
		graphValue(to, "path"), graphValue(to, "name"), graphValue(to, "line"),
	}, "\x00")
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*out = append(*out, codeGraphStructuralRelation{
		From: from, To: to, Relation: relation, Provider: "go-types", ResolutionMode: "type-analysis", Confidence: 1.0,
	})
}

func (e *Engine) goPackageFilesForPath(path string) []string {
	path = e.workspacePath(path)
	if !strings.EqualFold(filepath.Ext(path), ".go") || e.refreshStructuralIndex() != nil {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(path)))
	e.indexMu.Lock()
	paths := make([]string, 0, 8)
	for candidate := range e.index {
		if strings.EqualFold(filepath.Ext(candidate), ".go") && !strings.HasSuffix(strings.ToLower(candidate), "_test.go") && filepath.ToSlash(filepath.Dir(filepath.FromSlash(candidate))) == dir {
			paths = append(paths, candidate)
		}
	}
	e.indexMu.Unlock()
	sort.Strings(paths)
	return paths
}

func (e *Engine) parseCheckedGoPackage(path string) *checkedGoPackage {
	paths := e.goPackageFilesForPath(path)
	if len(paths) == 0 {
		return nil
	}
	fset := token.NewFileSet()
	files, packageName := e.parseGoPackageFiles(fset, paths)
	if len(files) == 0 || packageName == "" {
		return nil
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check(packageName, fset, files, nil)
	if err != nil || pkg == nil {
		// Only a fully type-checked package may emit semantic type relations.
		return nil
	}
	return &checkedGoPackage{FSet: fset, Pkg: pkg}
}

func (e *Engine) parseGoPackageFiles(fset *token.FileSet, paths []string) ([]*ast.File, string) {
	files := make([]*ast.File, 0, len(paths))
	packageName := ""
	for _, candidate := range paths {
		absolute, err := e.FS.Existing(candidate)
		if err != nil {
			return nil, ""
		}
		match, err := build.Default.MatchFile(filepath.Dir(absolute), filepath.Base(absolute))
		if err != nil || !match {
			continue
		}
		file, err := parser.ParseFile(fset, absolute, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, ""
		}
		if packageName == "" {
			packageName = file.Name.Name
		}
		if file.Name.Name == packageName {
			files = append(files, file)
		}
	}
	return files, packageName
}

func goPackageNamedTypes(pkg *types.Package) (map[*types.Named]*types.Interface, []*types.Named) {
	interfaces := map[*types.Named]*types.Interface{}
	concretes := []*types.Named{}
	for _, name := range pkg.Scope().Names() {
		object, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := object.Type().(*types.Named)
		if !ok {
			continue
		}
		if iface, ok := named.Underlying().(*types.Interface); ok {
			iface.Complete()
			if iface.NumMethods() > 0 {
				interfaces[named] = iface
			}
			continue
		}
		concretes = append(concretes, named)
	}
	return interfaces, concretes
}

func addGoMethodRelations(out *[]codeGraphStructuralRelation, seen map[string]struct{}, checked *checkedGoPackage, named *types.Named, typeNode map[string]any) {
	for index := 0; index < named.NumMethods(); index++ {
		method := named.Method(index)
		if method.Pkg() != checked.Pkg {
			continue
		}
		methodNode := goTypeGraphNode(checked.FSet, method, "method")
		if methodNode != nil {
			methodNode["detail"] = named.Obj().Name()
		}
		addGoTypeRelation(out, seen, typeNode, methodNode, "CONTAINS")
	}
}

func addGoEmbedRelations(out *[]codeGraphStructuralRelation, seen map[string]struct{}, checked *checkedGoPackage, named *types.Named, typeNode map[string]any) {
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for index := 0; index < structure.NumFields(); index++ {
		field := structure.Field(index)
		embedded := goNamedType(field.Type())
		if !field.Embedded() || embedded == nil || embedded.Obj() == nil || embedded.Obj().Pkg() != checked.Pkg {
			continue
		}
		addGoTypeRelation(out, seen, typeNode, goTypeGraphNode(checked.FSet, embedded.Obj(), "type"), "EMBEDS")
	}
}

func addGoImplementationRelations(out *[]codeGraphStructuralRelation, seen map[string]struct{}, checked *checkedGoPackage, named *types.Named, interfaces map[*types.Named]*types.Interface, typeNode map[string]any) {
	for ifaceNamed, iface := range interfaces {
		if !types.Implements(named, iface) && !types.Implements(types.NewPointer(named), iface) {
			continue
		}
		addGoTypeRelation(out, seen, typeNode, goTypeGraphNode(checked.FSet, ifaceNamed.Obj(), "interface"), "IMPLEMENTS")
	}
}

func addGoInterfaceExtensionRelations(out *[]codeGraphStructuralRelation, seen map[string]struct{}, checked *checkedGoPackage, named *types.Named, iface *types.Interface) {
	from := goTypeGraphNode(checked.FSet, named.Obj(), "interface")
	for index := 0; index < iface.NumEmbeddeds(); index++ {
		embedded := goNamedType(iface.EmbeddedType(index))
		if embedded == nil || embedded.Obj() == nil || embedded.Obj().Pkg() != checked.Pkg {
			continue
		}
		if _, ok := embedded.Underlying().(*types.Interface); ok {
			addGoTypeRelation(out, seen, from, goTypeGraphNode(checked.FSet, embedded.Obj(), "interface"), "EXTENDS")
		}
	}
}

func (e *Engine) goTypeRelationsForPath(path string) []codeGraphStructuralRelation {
	checked := e.parseCheckedGoPackage(path)
	if checked == nil {
		return nil
	}
	interfaces, concretes := goPackageNamedTypes(checked.Pkg)
	out, seen := []codeGraphStructuralRelation{}, map[string]struct{}{}
	for _, named := range concretes {
		typeNode := goTypeGraphNode(checked.FSet, named.Obj(), "type")
		addGoMethodRelations(&out, seen, checked, named, typeNode)
		addGoEmbedRelations(&out, seen, checked, named, typeNode)
		addGoImplementationRelations(&out, seen, checked, named, interfaces, typeNode)
	}
	for named, iface := range interfaces {
		addGoInterfaceExtensionRelations(&out, seen, checked, named, iface)
	}
	return out
}

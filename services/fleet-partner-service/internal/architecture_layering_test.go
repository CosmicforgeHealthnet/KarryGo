package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Use-case boundary guard (Fleet spec FLEET-5.1a and FLEET-5.1b).
//
//   - An HTTP handler depends on a use-case interface declared in its own
//     http package, never on the concrete *usecases.Service. So an exported
//     New* constructor in internal/features/<f>/http must not take a pointer
//     to a struct declared in a usecases package.
//   - A use case depends on repository interfaces declared in its usecases
//     package, never on the concrete Postgres type. So a usecases package must
//     not declare a struct field or New* constructor parameter typed as a
//     pointer to a struct declared in a repositories package.
//
// The concrete types are wired at the composition root (internal/app).

const layeringModulePath = "cosmicforge/logistics/services/fleet-partner-service/"

// layeringPendingFeatures are flat features that are being re-layered into
// models/repositories/usecases/http in parallel work. They are skipped here
// until that work merges; the coordinator empties this list afterwards.
// Do not add a feature to this list to silence a new violation.
var layeringPendingFeatures = map[string]bool{
	"assets":       true,
	"warehouse":    true,
	"proof":        true,
	"engagement":   true,
	"finance":      true,
	"financeops":   true,
	"integrations": true,
}

// layeringStructCache maps an import path to the struct type names it declares.
var layeringStructCache = map[string]map[string]bool{}

func layeringStructsOf(t *testing.T, importPath string) map[string]bool {
	t.Helper()
	if cached, ok := layeringStructCache[importPath]; ok {
		return cached
	}
	structs := map[string]bool{}
	layeringStructCache[importPath] = structs
	if !strings.HasPrefix(importPath, layeringModulePath) {
		return structs
	}
	dir := filepath.Join(serviceRoot, filepath.FromSlash(strings.TrimPrefix(importPath, layeringModulePath)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return structs
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
					structs[typeSpec.Name.Name] = true
				}
			}
		}
	}
	return structs
}

// layeringFile is a parsed source file with its import aliases resolved.
type layeringFile struct {
	path    string
	fset    *token.FileSet
	file    *ast.File
	imports map[string]string // local name -> import path
}

func layeringParsePackage(t *testing.T, dir string) []layeringFile {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []layeringFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		imports := map[string]string{}
		for _, spec := range parsed.Imports {
			importPath, _ := strconv.Unquote(spec.Path.Value)
			local := importPath[strings.LastIndex(importPath, "/")+1:]
			if spec.Name != nil {
				local = spec.Name.Name
			}
			imports[local] = importPath
		}
		files = append(files, layeringFile{path: path, fset: fset, file: parsed, imports: imports})
	}
	return files
}

// layeringConcretePointer reports "pkg.Type" when expr is a pointer to a
// struct declared in a package whose last path element is layer.
func layeringConcretePointer(t *testing.T, file layeringFile, expr ast.Expr, layer string) (string, bool) {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return "", false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	importPath, ok := file.imports[ident.Name]
	if !ok || importPath[strings.LastIndex(importPath, "/")+1:] != layer {
		return "", false
	}
	if !layeringStructsOf(t, importPath)[selector.Sel.Name] {
		return "", false
	}
	return ident.Name + "." + selector.Sel.Name, true
}

func layeringFeatureDirs(t *testing.T, layer string) map[string]string {
	t.Helper()
	dirs, _ := filepath.Glob(filepath.Join(serviceRoot, "internal", "features", "*", layer))
	out := map[string]string{}
	for _, dir := range dirs {
		feature := filepath.Base(filepath.Dir(dir))
		if layeringPendingFeatures[feature] {
			continue
		}
		out[feature] = dir
	}
	return out
}

func layeringReport(t *testing.T, violations []string, rule string) {
	t.Helper()
	sort.Strings(violations)
	for _, violation := range violations {
		t.Errorf("%s: %s", rule, violation)
	}
}

// TestHTTPConstructorsDependOnUseCaseInterfaces enforces FLEET-5.1a.
func TestHTTPConstructorsDependOnUseCaseInterfaces(t *testing.T) {
	var violations []string
	for _, dir := range layeringFeatureDirs(t, "http") {
		for _, file := range layeringParsePackage(t, dir) {
			for _, decl := range file.file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !fn.Name.IsExported() || !strings.HasPrefix(fn.Name.Name, "New") {
					continue
				}
				for _, param := range fn.Type.Params.List {
					if typeName, bad := layeringConcretePointer(t, file, param.Type, "usecases"); bad {
						violations = append(violations, file.fset.Position(fn.Pos()).String()+
							" "+fn.Name.Name+" takes *"+typeName+"; declare a use-case interface in the http package")
					}
				}
			}
		}
	}
	layeringReport(t, violations, "FLEET-5.1a")
}

// TestUseCasesDependOnRepositoryInterfaces enforces FLEET-5.1b.
func TestUseCasesDependOnRepositoryInterfaces(t *testing.T) {
	var violations []string
	for _, dir := range layeringFeatureDirs(t, "usecases") {
		for _, file := range layeringParsePackage(t, dir) {
			ast.Inspect(file.file, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.StructType:
					for _, field := range typed.Fields.List {
						if typeName, bad := layeringConcretePointer(t, file, field.Type, "repositories"); bad {
							violations = append(violations, file.fset.Position(field.Pos()).String()+
								" struct field typed *"+typeName+"; declare a repository interface in usecases")
						}
					}
				case *ast.FuncDecl:
					if typed.Recv != nil || !strings.HasPrefix(typed.Name.Name, "New") {
						return true
					}
					for _, param := range typed.Type.Params.List {
						if typeName, bad := layeringConcretePointer(t, file, param.Type, "repositories"); bad {
							violations = append(violations, file.fset.Position(typed.Pos()).String()+
								" "+typed.Name.Name+" takes *"+typeName+"; declare a repository interface in usecases")
						}
					}
				}
				return true
			})
		}
	}
	layeringReport(t, violations, "FLEET-5.1b")
}

// TestLayeringPendingFeaturesStillExist keeps the pending list honest: an
// entry for a feature directory that no longer exists is stale.
func TestLayeringPendingFeaturesStillExist(t *testing.T) {
	for feature := range layeringPendingFeatures {
		if _, err := os.Stat(filepath.Join(serviceRoot, "internal", "features", feature)); err != nil {
			t.Errorf("pending feature %q no longer exists; remove it from layeringPendingFeatures", feature)
		}
	}
}

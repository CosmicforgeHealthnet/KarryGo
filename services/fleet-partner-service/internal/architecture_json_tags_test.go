package internal_test

// Spec Section 6 (FLEET-6h / 6.1): every Fleet public API payload uses
// lowerCamel JSON keys. A Go struct field without a json tag serializes
// under its Go name ("ID", "AssetID", "CreatedAt"), which silently breaks
// that convention and every client written against it.
//
// The guard type-checks the feature packages from source (stdlib go/types;
// packages outside this module are stubbed, so their types simply become
// leaves) and computes which struct types can reach an HTTP payload:
//
//   - the static type of the data argument of respond.OK / Created / JSON /
//     List and of gin's c.JSON-style writers (including gin.H values);
//   - the bound type of every request-body binder (bind*, ShouldBind*);
//   - the static type of every value returned through an `any`-typed result
//     of a function declared in a gin-importing file, because many handlers
//     dispatch through such a func and the respond call only sees `any`.
//
// Every struct reached from those roots (through pointers, slices, arrays,
// map values, named types and embedded structs) that is declared under
// internal/features must tag each exported, non-embedded field with
// `json:"..."` (or `json:"-"`). Types with their own MarshalJSON are leaves.
//
// Wire contracts with other services are not declared under
// internal/features (they live in internal/platformclients, the events /
// outbox packages and shared/go) and are therefore out of scope by
// construction. jsonTagExemptTypes lists the feature-declared exceptions.

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const fleetModulePath = "cosmicforge/logistics/services/fleet-partner-service"

// jsonTagExemptTypes are feature-declared structs that reach a guard root but
// are never written to an HTTP payload. Key: "<package path suffix>.<Type>".
var jsonTagExemptTypes = map[string]string{
	// Internal HMAC routes (/internal/fleet-partner/v1/...): service-to-service
	// wire contracts whose Go-name keys other services already decode
	// (shared/go/providercompat reads completedWork.ID / IngestionStatus).
	"fleetpartner.CompletedWorkRecord":              "POST /completed-work response (internal)",
	"fleetpartner.CompensationResolution":           "POST /completed-work response (internal)",
	"fleetpartner.ManagedWorkSettlementInstruction": "POST /completed-work response (internal)",
	"dispatch.TrackingEvent":                        "POST /tracking/events ingress (internal); payload hash is computed over its JSON",
	"financeops.PaymentEvent":                       "POST /finance/payment-events ingress (internal)",
	"engagement.SupportEvent":                       "POST /support/events ingress (internal)",
	"engagement.MeasurementInput":                   "POST /sla/measurements ingress (internal)",
}

type sourceImporter struct {
	fset  *token.FileSet
	pkgs  map[string]*types.Package
	infos map[string]*types.Info
	files map[string][]*ast.File
}

func (im *sourceImporter) Import(p string) (*types.Package, error) {
	if pkg, ok := im.pkgs[p]; ok {
		return pkg, nil
	}
	if p != fleetModulePath && !strings.HasPrefix(p, fleetModulePath+"/") {
		name := path.Base(p)
		if regexp.MustCompile(`^v[0-9]+$`).MatchString(name) {
			name = path.Base(path.Dir(p))
		}
		name = strings.TrimSuffix(strings.Split(name, ".")[0], "-go")
		name = strings.ReplaceAll(name, "-", "")
		stub := types.NewPackage(p, name)
		stub.MarkComplete()
		im.pkgs[p] = stub
		return stub, nil
	}
	dir := filepath.Join(serviceRoot, strings.TrimPrefix(strings.TrimPrefix(p, fleetModulePath), "/"))
	bp, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(im.fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: im, Error: func(error) {}}
	pkg, _ := conf.Check(p, im.fset, files, info)
	im.pkgs[p] = pkg
	im.infos[p] = info
	im.files[p] = files
	return pkg, nil
}

var bindCallName = regexp.MustCompile(`^(?i)(should)?bind`)

func TestFeaturePayloadStructsUseCamelCaseJSONTags(t *testing.T) {
	im := &sourceImporter{fset: token.NewFileSet(), pkgs: map[string]*types.Package{}, infos: map[string]*types.Info{}, files: map[string][]*ast.File{}}
	featuresDir := filepath.Join(serviceRoot, "internal", "features")
	var featurePkgs []string
	err := filepath.WalkDir(featuresDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if bp, err := build.Default.ImportDir(p, 0); err == nil && len(bp.GoFiles) > 0 {
			rel, _ := filepath.Rel(serviceRoot, p)
			featurePkgs = append(featurePkgs, fleetModulePath+"/"+filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(featurePkgs) < 40 {
		t.Fatalf("found only %d feature packages; the walk is broken", len(featurePkgs))
	}

	var roots []types.Type
	for _, p := range featurePkgs {
		if _, err := im.Import(p); err != nil {
			t.Fatalf("type-check %s: %v", p, err)
		}
		info := im.infos[p]
		addExpr := func(e ast.Expr) {}
		addExpr = func(e ast.Expr) {
			if lit, ok := ast.Unparen(e).(*ast.CompositeLit); ok {
				if tv, ok := info.Types[lit]; !ok || tv.Type == nil || tv.Type == types.Typ[types.Invalid] {
					// gin.H{...} and other stubbed literals: walk the values.
					for _, el := range lit.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok {
							addExpr(kv.Value)
						} else {
							addExpr(el)
						}
					}
					return
				}
			}
			if tv, ok := info.Types[e]; ok && tv.Type != nil {
				roots = append(roots, tv.Type)
			}
		}
		for _, f := range im.files[p] {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var name, recv string
				switch fn := call.Fun.(type) {
				case *ast.SelectorExpr:
					name = fn.Sel.Name
					if id, ok := fn.X.(*ast.Ident); ok {
						recv = id.Name
					}
				case *ast.Ident:
					name = fn.Name
				}
				switch {
				case recv == "respond" && (name == "OK" || name == "Created") && len(call.Args) == 2:
					addExpr(call.Args[1])
				case recv == "respond" && name == "JSON" && len(call.Args) == 3:
					addExpr(call.Args[2])
				case recv == "respond" && name == "List" && len(call.Args) == 3:
					addExpr(call.Args[1])
				case recv != "respond" && len(call.Args) == 2 && (name == "JSON" || name == "IndentedJSON" || name == "PureJSON" || name == "AbortWithStatusJSON"):
					addExpr(call.Args[1])
				case bindCallName.MatchString(name):
					for _, a := range call.Args {
						if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
							addExpr(u.X)
						}
					}
				}
				return true
			})
		}
		// Handlers that dispatch through a func returning `any` (assets,
		// manifests, bulk imports, ...) lose the static type at the
		// respond call; recover it from the return statements instead.
		for _, f := range im.files[p] {
			if !importsGin(f) {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				// `var v any; v, err = h.s.Get(...)` dispatchers.
				if as, ok := n.(*ast.AssignStmt); ok {
					isAny := func(e ast.Expr) bool {
						tv, ok := info.Types[e]
						if !ok || tv.Type == nil {
							if id, ok := e.(*ast.Ident); ok && info.Uses[id] != nil {
								tv.Type = info.Uses[id].Type()
							} else {
								return false
							}
						}
						iface, ok := types.Unalias(tv.Type).Underlying().(*types.Interface)
						return ok && iface.NumMethods() == 0
					}
					if len(as.Rhs) == 1 && len(as.Lhs) > 1 {
						if tuple, ok := info.Types[as.Rhs[0]].Type.(*types.Tuple); ok {
							for k, l := range as.Lhs {
								if k < tuple.Len() && isAny(l) {
									roots = append(roots, tuple.At(k).Type())
								}
							}
						}
					} else if len(as.Rhs) == len(as.Lhs) {
						for k, l := range as.Lhs {
							if isAny(l) {
								addExpr(as.Rhs[k])
							}
						}
					}
					return true
				}
				var ft *ast.FuncType
				var body *ast.BlockStmt
				switch fn := n.(type) {
				case *ast.FuncDecl:
					ft, body = fn.Type, fn.Body
				case *ast.FuncLit:
					ft, body = fn.Type, fn.Body
				default:
					return true
				}
				if body == nil || ft.Results == nil {
					return true
				}
				var anyPos []int
				i := 0
				for _, field := range ft.Results.List {
					count := len(field.Names)
					if count == 0 {
						count = 1
					}
					tv := info.Types[field.Type]
					var iface *types.Interface
					isIface := false
					if tv.Type != nil {
						iface, isIface = types.Unalias(tv.Type).Underlying().(*types.Interface)
					}
					if tv.Type != nil && isIface && iface.NumMethods() == 0 {
						for k := 0; k < count; k++ {
							anyPos = append(anyPos, i+k)
						}
					}
					i += count
				}
				if len(anyPos) == 0 {
					return true
				}
				ast.Inspect(body, func(m ast.Node) bool {
					if _, nested := m.(*ast.FuncLit); nested {
						return false
					}
					ret, ok := m.(*ast.ReturnStmt)
					if !ok {
						return true
					}
					if len(ret.Results) == 1 && i > 1 {
						// return f(...) forwarding a multi-value call.
						if tuple, ok := info.Types[ret.Results[0]].Type.(*types.Tuple); ok {
							for _, pos := range anyPos {
								if pos < tuple.Len() {
									roots = append(roots, tuple.At(pos).Type())
								}
							}
						}
						return true
					}
					for _, pos := range anyPos {
						if pos < len(ret.Results) && len(ret.Results) == i {
							addExpr(ret.Results[pos])
						}
					}
					return true
				})
				return true
			})
		}
	}

	featurePrefix := fleetModulePath + "/internal/features/"
	seen := map[types.Type]bool{}
	var violations []string
	var walk func(types.Type)
	walk = func(tt types.Type) {
		if tt == nil || seen[tt] {
			return
		}
		seen[tt] = true
		switch x := tt.(type) {
		case *types.Pointer:
			walk(x.Elem())
		case *types.Slice:
			walk(x.Elem())
		case *types.Array:
			walk(x.Elem())
		case *types.Map:
			walk(x.Elem())
		case *types.Alias:
			walk(types.Unalias(x))
		case *types.Named:
			obj := x.Obj()
			if obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), fleetModulePath) {
				return
			}
			if hasMethod(x, "MarshalJSON") || hasMethod(x, "MarshalText") {
				return
			}
			st, ok := x.Underlying().(*types.Struct)
			if !ok {
				walk(x.Underlying())
				return
			}
			key := strings.TrimPrefix(obj.Pkg().Path(), featurePrefix) + "." + obj.Name()
			check := strings.HasPrefix(obj.Pkg().Path(), featurePrefix) && jsonTagExemptTypes[key] == ""
			checkStruct(im.fset, st, key, check, &violations, walk)
		case *types.Struct:
			checkStruct(im.fset, x, "(anonymous struct)", true, &violations, walk)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Errorf("%d exported payload fields have no json tag; add a lowerCamel `json:\"...\"` tag (spec Section 6):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

func checkStruct(fset *token.FileSet, st *types.Struct, key string, check bool, violations *[]string, walk func(types.Type)) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag, hasTag := reflect.StructTag(st.Tag(i)).Lookup("json")
		if hasTag && strings.Split(tag, ",")[0] == "-" {
			continue
		}
		walk(f.Type())
		if !f.Exported() || f.Embedded() || hasTag || !check {
			continue
		}
		pos := fset.Position(f.Pos())
		rel := pos.Filename
		if i := strings.Index(rel, "internal/features/"); i >= 0 {
			rel = rel[i:]
		}
		*violations = append(*violations, fmt.Sprintf("  %s.%s (%s:%d)", key, f.Name(), rel, pos.Line))
	}
}

func importsGin(f *ast.File) bool {
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "github.com/gin-gonic/gin" {
			return true
		}
	}
	return false
}

func hasMethod(n *types.Named, name string) bool {
	for _, t := range []types.Type{n, types.NewPointer(n)} {
		ms := types.NewMethodSet(t)
		for i := 0; i < ms.Len(); i++ {
			if ms.At(i).Obj().Name() == name {
				return true
			}
		}
	}
	return false
}

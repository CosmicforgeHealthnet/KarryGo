// Command openapigen derives OpenAPI operations for mounted-but-undocumented
// customer routes FROM THE HANDLER SOURCE, so no DTO field is hand-invented.
//
// Input (stdin): JSON array of {service, method, path, cls, handler}
// where handler is the gin handler name ("<pkgpath>.(*T).M-fm" or "<pkg>.F").
// It parses the owning service's Go source, finds the handler, and records:
//   - the request body DTO bound via ShouldBindJSON/BindJSON/Decode/strict*JSON
//     (fields and json tags, resolved through named struct types);
//   - c.Query / c.DefaultQuery names; c.GetHeader names; path params.
// Output (stdout): JSON {service: {"METHOD path": operation}}.
package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type route struct {
	Service, Method, Path, Cls, Handler string
}

type field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Items    string `json:"items,omitempty"`
}

type operation struct {
	Handler string   `json:"handler"`
	Body    []field  `json:"body,omitempty"`
	BodyDTO string   `json:"body_dto,omitempty"`
	Query   []string `json:"query,omitempty"`
	Headers []string `json:"headers,omitempty"`
}

type pkgIndex struct {
	funcs   map[string]*ast.FuncDecl // "Func" or "Type.Method"
	structs map[string]*ast.StructType
}

var cache = map[string]*pkgIndex{}

func index(dir string) *pkgIndex {
	if p, ok := cache[dir]; ok {
		return p
	}
	p := &pkgIndex{funcs: map[string]*ast.FuncDecl{}, structs: map[string]*ast.StructType{}}
	cache[dir] = p
	fset := token.NewFileSet()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				name := decl.Name.Name
				if decl.Recv != nil && len(decl.Recv.List) == 1 {
					t := decl.Recv.List[0].Type
					if s, ok := t.(*ast.StarExpr); ok {
						t = s.X
					}
					if id, ok := t.(*ast.Ident); ok {
						name = id.Name + "." + name
					}
				}
				p.funcs[name] = decl
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							p.structs[ts.Name.Name] = st
						}
					}
				}
			}
		}
	}
	return p
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return typeName(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeName(t.Elt)
	case *ast.MapType:
		return "map"
	case *ast.InterfaceType:
		return "any"
	}
	return "object"
}

func jsonType(goType string) (string, string) {
	if strings.HasPrefix(goType, "[]") {
		inner, _ := jsonType(goType[2:])
		if goType == "[]byte" {
			return "string", ""
		}
		return "array", inner
	}
	switch goType {
	case "string", "time.Time", "uuid.UUID":
		return "string", ""
	case "int", "int32", "int64", "uint", "uint32", "uint64":
		return "integer", ""
	case "float32", "float64":
		return "number", ""
	case "bool":
		return "boolean", ""
	case "json.RawMessage", "map", "any":
		return "object", ""
	}
	return "object", ""
}

func structFields(p *pkgIndex, st *ast.StructType, depth int) []field {
	var out []field
	for _, f := range st.Fields.List {
		tag := ""
		if f.Tag != nil {
			tag = reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("json")
		}
		if len(f.Names) == 0 { // embedded
			if inner, ok := p.structs[typeName(f.Type)]; ok && depth < 3 {
				out = append(out, structFields(p, inner, depth+1)...)
			}
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "-" || !f.Names[0].IsExported() {
			continue
		}
		if name == "" {
			name = f.Names[0].Name
		}
		t, items := jsonType(typeName(f.Type))
		_, pointer := f.Type.(*ast.StarExpr)
		out = append(out, field{Name: name, Type: t, Items: items, Required: !pointer && !strings.Contains(tag, "omitempty")})
	}
	return out
}

var bindNames = regexp.MustCompile(`(?i)^(ShouldBindJSON|BindJSON|ShouldBind|Decode|decode.*|strict.*|bind.*|readJSON|parseJSON)$`)

func analyse(p *pkgIndex, fn *ast.FuncDecl, op *operation, seen map[*ast.FuncDecl]bool) {
	if fn == nil || fn.Body == nil || seen[fn] {
		return
	}
	seen[fn] = true
	vars := map[string]string{}
	anon := map[string]*ast.StructType{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			if x.Type != nil {
				for _, name := range x.Names {
					vars[name.Name] = typeName(x.Type)
					if st, ok := x.Type.(*ast.StructType); ok {
						anon[name.Name] = st
					}
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range x.Rhs {
				if cl, ok := rhs.(*ast.CompositeLit); ok && i < len(x.Lhs) {
					if id, ok := x.Lhs[i].(*ast.Ident); ok {
						vars[id.Name] = typeName(cl.Type)
					}
				}
			}
		case *ast.CallExpr:
			name := ""
			switch f := x.Fun.(type) {
			case *ast.SelectorExpr:
				name = f.Sel.Name
			case *ast.Ident:
				name = f.Name
			}
			lit := func(i int) string {
				if i < len(x.Args) {
					if b, ok := x.Args[i].(*ast.BasicLit); ok {
						return strings.Trim(b.Value, "\"")
					}
				}
				return ""
			}
			switch name {
			case "Query", "DefaultQuery", "GetQuery":
				if q := lit(0); q != "" {
					op.Query = append(op.Query, q)
				}
			case "GetHeader":
				if h := lit(0); h != "" {
					op.Headers = append(op.Headers, h)
				}
			}
			if bindNames.MatchString(name) && op.BodyDTO == "" {
				for _, arg := range x.Args {
					if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.AND {
						if id, ok := u.X.(*ast.Ident); ok {
							if st, ok := anon[id.Name]; ok {
								op.BodyDTO = "anonymous struct in " + fn.Name.Name
								op.Body = structFields(p, st, 0)
								continue
							}
							if t, ok := vars[id.Name]; ok {
								if st, ok := p.structs[t]; ok {
									op.BodyDTO = t
									op.Body = structFields(p, st, 0)
								}
							}
						}
					}
				}
			}
			// Follow same-package helper calls one level (h.helper(c, ...)).
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				for key, callee := range p.funcs {
					if strings.HasSuffix(key, "."+sel.Sel.Name) && len(seen) < 4 {
						analyse(p, callee, op, seen)
					}
				}
			}
		}
		return true
	})
}

func main() {
	var routes []route
	if err := json.NewDecoder(os.Stdin).Decode(&routes); err != nil {
		panic(err)
	}
	root := os.Args[1]
	out := map[string]map[string]operation{}
	handlerRe := regexp.MustCompile(`^cosmicforge/logistics/services/[^/]+/([^.]+)\.(?:\(\*?([A-Za-z0-9_]+)\)\.)?([A-Za-z0-9_]+)(?:-fm)?$`)
	for _, r := range routes {
		op := operation{Handler: r.Handler}
		if m := handlerRe.FindStringSubmatch(r.Handler); m != nil {
			p := index(filepath.Join(root, "services", r.Service, filepath.FromSlash(m[1])))
			key := m[3]
			if m[2] != "" {
				key = m[2] + "." + m[3]
			}
			fn := p.funcs[key]
			if lit := routeClosure(fn, r.Method, r.Path); lit != nil {
				// Inline closure handler: analyse only the closure registered
				// for THIS route, identified by its path literal.
				fn = &ast.FuncDecl{Name: ast.NewIdent("closure"), Type: lit.Type, Body: lit.Body}
			}
			analyse(p, fn, &op, map[*ast.FuncDecl]bool{})
		}
		sort.Strings(op.Query)
		sort.Strings(op.Headers)
		op.Query, op.Headers = dedupe(op.Query), dedupe(op.Headers)
		if out[r.Service] == nil {
			out[r.Service] = map[string]operation{}
		}
		out[r.Service][r.Method+" "+r.Path] = op
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	_ = enc.Encode(out)
}

func dedupe(values []string) []string {
	var out []string
	for i, v := range values {
		if i == 0 || values[i-1] != v {
			out = append(out, v)
		}
	}
	return out
}

// routeClosure finds, inside a registration function, the handler closure of
// the gin call whose method matches and whose path literal is a suffix of the
// mounted path.
func routeClosure(fn *ast.FuncDecl, method, path string) *ast.FuncLit {
	if fn == nil || fn.Body == nil {
		return nil
	}
	norm := regexp.MustCompile(`:[A-Za-z_]+`)
	full := norm.ReplaceAllString(path, ":p")
	var best *ast.FuncLit
	bestLen := -1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != method || len(call.Args) < 2 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		suffix := norm.ReplaceAllString(strings.Trim(lit.Value, "\""), ":p")
		if suffix == "" {
			suffix = "/"
		}
		if !strings.HasSuffix(full, strings.TrimSuffix(suffix, "/")) && suffix != "/" {
			return true
		}
		if f, ok := call.Args[len(call.Args)-1].(*ast.FuncLit); ok && len(suffix) > bestLen {
			best, bestLen = f, len(suffix)
		}
		return true
	})
	return best
}

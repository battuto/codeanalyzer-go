package astx

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// ExtractSymbols parses all .go files and extracts a minimal symbol table.
func ExtractSymbols(p *loader.Program) *schema.SymbolTable {
	fset := token.NewFileSet()
	pkgs := map[string]*schema.Package{}

	for _, path := range p.Files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// skip on parse error but continue
			continue
		}
		pkgName := file.Name.Name
		pkg := pkgs[pkgName]
		if pkg == nil {
			pkg = &schema.Package{
				Path:      pkgName, // placeholder; replace with module path if desired
				Files:     []schema.File{},
				Imports:   []schema.Import{},
				Types:     []schema.TypeDecl{},
				Functions: []schema.Function{},
			}
			pkgs[pkgName] = pkg
		}
		rel := path
		if rp, err := filepath.Rel(p.Root, path); err == nil {
			rel = rp
		}
		pkg.Files = append(pkg.Files, schema.File{Path: rel})

		for _, imp := range file.Imports {
			val := imp.Path.Value // quoted
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			pkg.Imports = append(pkg.Imports, schema.Import{Path: trimQuotes(val), Alias: alias})
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				fn := schema.Function{
					Name:      x.Name.Name,
					Receiver:  recvName(x.Recv),
					Signature: buildSignature(fset, x),
					Pos:       pos(fset, x.Pos()),
				}
				pkg.Functions = append(pkg.Functions, fn)
			case *ast.GenDecl:
				if x.Tok == token.TYPE {
					for _, spec := range x.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							kind := kindOfType(ts)
							td := schema.TypeDecl{
								Name: ts.Name.Name,
								Kind: kind,
								Pos:  pos(fset, ts.Pos()),
							}
							pkg.Types = append(pkg.Types, td)
						}
					}
				}
			}
			return true
		})
	}

	out := &schema.SymbolTable{
		Language: "go",
		Packages: []schema.Package{},
	}
	// ordina i pacchetti per Path per garantire stabilità
	names := make([]string, 0, len(pkgs))
	for name := range pkgs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pkg := pkgs[name]
		// ordina contenuti del pacchetto
		sort.Slice(pkg.Files, func(i, j int) bool { return pkg.Files[i].Path < pkg.Files[j].Path })
		sort.Slice(pkg.Imports, func(i, j int) bool {
			if pkg.Imports[i].Path == pkg.Imports[j].Path {
				return pkg.Imports[i].Alias < pkg.Imports[j].Alias
			}
			return pkg.Imports[i].Path < pkg.Imports[j].Path
		})
		sort.Slice(pkg.Types, func(i, j int) bool { return pkg.Types[i].Name < pkg.Types[j].Name })
		sort.Slice(pkg.Functions, func(i, j int) bool {
			if pkg.Functions[i].Name == pkg.Functions[j].Name {
				return pkg.Functions[i].Receiver < pkg.Functions[j].Receiver
			}
			return pkg.Functions[i].Name < pkg.Functions[j].Name
		})
		out.Packages = append(out.Packages, *pkg)
	}
	return out
}

func trimQuotes(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

func recvName(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	f := fl.List[0]
	// include receiver name if present
	name := ""
	if len(f.Names) > 0 {
		name = f.Names[0].Name
	}
	t := exprString(f.Type)
	if name != "" {
		return name + " " + t
	}
	return t
}

func exprString(e ast.Expr) string {
	var buf bytes.Buffer
	// Use go/printer to get a compact representation
	_ = printer.Fprint(&buf, token.NewFileSet(), e)
	return strings.TrimSpace(buf.String())
}

func pos(fset *token.FileSet, p token.Pos) schema.Position {
	pos := fset.Position(p)
	return schema.Position{File: pos.Filename, Line: pos.Line, Column: pos.Column}
}

func buildSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	// Build param types
	params := []string{}
	if fn.Type.Params != nil {
		for _, f := range fn.Type.Params.List {
			t := exprString(f.Type)
			// number of names determines arity; if no name, just type
			if len(f.Names) == 0 {
				params = append(params, t)
			} else {
				for range f.Names {
					params = append(params, t)
				}
			}
		}
	}
	res := []string{}
	if fn.Type.Results != nil {
		for _, f := range fn.Type.Results.List {
			t := exprString(f.Type)
			if len(f.Names) == 0 {
				res = append(res, t)
			} else {
				for range f.Names {
					res = append(res, t)
				}
			}
		}
	}
	recv := recvName(fn.Recv)
	sig := "func "
	if recv != "" {
		sig += "(" + recv + ") "
	}
	sig += fn.Name.Name
	sig += "(" + strings.Join(params, ", ") + ")"
	if len(res) == 1 {
		sig += " " + res[0]
	} else if len(res) > 1 {
		sig += " (" + strings.Join(res, ", ") + ")"
	}
	return sig
}

func kindOfType(ts *ast.TypeSpec) string {
	if ts.Assign.IsValid() {
		return "alias"
	}
	switch ts.Type.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return ""
	}
}

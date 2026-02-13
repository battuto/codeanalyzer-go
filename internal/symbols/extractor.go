// Package symbols fornisce l'estrazione dei simboli CLDK-compatible.
package symbols

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"

	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// cleanDoc rimuove newline e spazi extra dalla documentazione per un JSON più leggibile.
func cleanDoc(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// Collassa spazi multipli
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// ExtractConfig configura l'estrazione dei simboli.
type ExtractConfig struct {
	IncludeBody      bool   // include informazioni sul corpo delle funzioni
	EmitPositions    string // detailed|minimal
	IncludeCallSites bool   // estrai call sites nel body
}

// Extract estrae la symbol table CLDK da un LoadResult.
func Extract(result *loader.LoadResult, cfg ExtractConfig) *schema.CLDKSymbolTable {
	st := &schema.CLDKSymbolTable{
		Packages: make(map[string]*schema.CLDKPackage),
	}

	for _, pkg := range result.Packages {
		if pkg == nil {
			continue
		}

		cldkPkg := extractPackage(pkg, result.Fset, result.Root, cfg)
		st.Packages[pkg.PkgPath] = cldkPkg
	}

	return st
}

// extractPackage estrae un singolo pacchetto.
func extractPackage(pkg *packages.Package, fset *token.FileSet, root string, cfg ExtractConfig) *schema.CLDKPackage {
	cldkPkg := &schema.CLDKPackage{
		Path:                 pkg.PkgPath,
		Name:                 pkg.Name,
		Files:                make([]string, 0),
		Imports:              make([]schema.CLDKImport, 0),
		TypeDeclarations:     make(map[string]*schema.CLDKType),
		CallableDeclarations: make(map[string]*schema.CLDKCallable),
		Variables:            make(map[string]*schema.CLDKVariable),
		Constants:            make(map[string]*schema.CLDKConstant),
	}

	// Raccogli file
	for _, f := range pkg.GoFiles {
		rel := f
		if rp, err := filepath.Rel(root, f); err == nil {
			rel = filepath.ToSlash(rp)
		}
		cldkPkg.Files = append(cldkPkg.Files, rel)
	}
	sort.Strings(cldkPkg.Files)

	// Import set per deduplicazione
	importSet := make(map[string]schema.CLDKImport)

	// Processa ogni file di sintassi
	for _, file := range pkg.Syntax {
		if file == nil {
			continue
		}

		// Estrai package documentation dal primo file che ha Doc
		if cldkPkg.Documentation == "" && file.Doc != nil {
			cldkPkg.Documentation = cleanDoc(file.Doc.Text())
		}

		// Estrai imports
		for _, imp := range file.Imports {
			path := trimQuotes(imp.Path.Value)
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			key := path + ":" + alias
			if _, exists := importSet[key]; !exists {
				cldkImp := schema.CLDKImport{
					Path:  path,
					Alias: alias,
				}
				if cfg.EmitPositions == "detailed" {
					cldkImp.Position = posOf(fset, imp.Pos(), root)
				}
				importSet[key] = cldkImp
			}
		}

		// Processa dichiarazioni
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				callable := extractCallable(pkg.PkgPath, d, fset, root, cfg)
				cldkPkg.CallableDeclarations[callable.QualifiedName] = callable

			case *ast.GenDecl:
				switch d.Tok {
				case token.TYPE:
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							t := extractType(pkg.PkgPath, ts, d, fset, root, cfg)
							cldkPkg.TypeDeclarations[t.QualifiedName] = t
						}
					}

				case token.VAR:
					for _, spec := range d.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							vars := extractVariables(pkg.PkgPath, vs, d, fset, root, cfg)
							for _, v := range vars {
								cldkPkg.Variables[v.QualifiedName] = v
							}
						}
					}

				case token.CONST:
					for _, spec := range d.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							consts := extractConstants(pkg.PkgPath, vs, d, fset, root, cfg)
							for _, c := range consts {
								cldkPkg.Constants[c.QualifiedName] = c
							}
						}
					}
				}
			}
		}

		// Estrai metodi e associali ai tipi
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil {
				recvType := extractReceiverTypeName(fn.Recv)
				if recvType != "" {
					typeQN := fmt.Sprintf("%s.%s", pkg.PkgPath, recvType)
					if t, exists := cldkPkg.TypeDeclarations[typeQN]; exists {
						if t.Methods == nil {
							t.Methods = make(map[string]*schema.CLDKMethod)
						}
						method := extractMethod(pkg.PkgPath, fn, fset, root, cfg)
						t.Methods[method.QualifiedName] = method
					}
				}
			}
		}
	}

	// Converti import set a slice
	for _, imp := range importSet {
		cldkPkg.Imports = append(cldkPkg.Imports, imp)
	}
	sort.Slice(cldkPkg.Imports, func(i, j int) bool {
		return cldkPkg.Imports[i].Path < cldkPkg.Imports[j].Path
	})

	// Popola call examples se il body è incluso
	if cfg.IncludeBody && cfg.IncludeCallSites {
		populateCallExamples(cldkPkg)
	}

	return cldkPkg
}

// extractCallable estrae una funzione o metodo.
func extractCallable(pkgPath string, fn *ast.FuncDecl, fset *token.FileSet, root string, cfg ExtractConfig) *schema.CLDKCallable {
	name := fn.Name.Name
	var qualifiedName string
	var kind string
	var recvType string
	var recvPtr bool

	if fn.Recv != nil {
		kind = "method"
		recvType, recvPtr = extractReceiverInfo(fn.Recv)
		if recvPtr {
			qualifiedName = fmt.Sprintf("%s.(*%s).%s", pkgPath, recvType, name)
		} else {
			qualifiedName = fmt.Sprintf("%s.%s.%s", pkgPath, recvType, name)
		}
	} else {
		kind = "function"
		qualifiedName = fmt.Sprintf("%s.%s", pkgPath, name)
	}

	callable := &schema.CLDKCallable{
		QualifiedName: qualifiedName,
		Name:          name,
		Signature:     buildSignature(fset, fn),
		Kind:          kind,
		ReceiverType:  recvType,
		ReceiverPtr:   recvPtr,
		Parameters:    extractParameters(fn.Type.Params),
		Results:       extractParameters(fn.Type.Results),
		Exported:      isExported(name),
	}

	// Posizione
	if cfg.EmitPositions != "minimal" {
		callable.Position = posOf(fset, fn.Pos(), root)
		callable.EndPosition = posOf(fset, fn.End(), root)
	}

	// Documentazione
	if fn.Doc != nil {
		callable.Documentation = cleanDoc(fn.Doc.Text())
	}

	// Type parameters (generics)
	if fn.Type.TypeParams != nil {
		callable.TypeParameters = extractTypeParams(fn.Type.TypeParams)
	}

	// Body info
	if cfg.IncludeBody && fn.Body != nil {
		callable.Body = extractFunctionBody(fn.Body, fset, root, cfg)
	}

	return callable
}

// extractMethod estrae un metodo come CLDKMethod.
func extractMethod(pkgPath string, fn *ast.FuncDecl, fset *token.FileSet, root string, cfg ExtractConfig) *schema.CLDKMethod {
	name := fn.Name.Name
	recvType, recvPtr := extractReceiverInfo(fn.Recv)

	var qualifiedName string
	if recvPtr {
		qualifiedName = fmt.Sprintf("%s.(*%s).%s", pkgPath, recvType, name)
	} else {
		qualifiedName = fmt.Sprintf("%s.%s.%s", pkgPath, recvType, name)
	}

	method := &schema.CLDKMethod{
		QualifiedName: qualifiedName,
		Name:          name,
		Signature:     buildSignature(fset, fn),
		ReceiverType:  recvType,
		ReceiverPtr:   recvPtr,
		Parameters:    extractParameters(fn.Type.Params),
		Results:       extractParameters(fn.Type.Results),
	}

	if cfg.EmitPositions != "minimal" {
		method.Position = posOf(fset, fn.Pos(), root)
		method.EndPosition = posOf(fset, fn.End(), root)
	}

	if fn.Doc != nil {
		method.Documentation = cleanDoc(fn.Doc.Text())
	}

	if cfg.IncludeBody && fn.Body != nil {
		method.Body = extractFunctionBody(fn.Body, fset, root, cfg)
	}

	return method
}

// extractType estrae una dichiarazione di tipo.
func extractType(pkgPath string, ts *ast.TypeSpec, gen *ast.GenDecl, fset *token.FileSet, root string, cfg ExtractConfig) *schema.CLDKType {
	name := ts.Name.Name
	qualifiedName := fmt.Sprintf("%s.%s", pkgPath, name)

	t := &schema.CLDKType{
		QualifiedName: qualifiedName,
		Name:          name,
		Kind:          kindOfType(ts),
	}

	if cfg.EmitPositions != "minimal" {
		t.Position = posOf(fset, ts.Pos(), root)
	}

	// Documentazione
	if gen.Doc != nil {
		t.Documentation = cleanDoc(gen.Doc.Text())
	} else if ts.Doc != nil {
		t.Documentation = cleanDoc(ts.Doc.Text())
	}

	// Type parameters (generics)
	if ts.TypeParams != nil {
		t.TypeParameters = extractTypeParams(ts.TypeParams)
	}

	// Underlying type per alias e named types
	if ts.Assign.IsValid() {
		t.UnderlyingType = exprString(ts.Type)
	}

	// Struct fields
	if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
		t.Fields = extractFields(st.Fields, fset, root, cfg)
		t.EmbeddedTypes = extractEmbeddedTypes(st.Fields)
	}

	// Interface methods
	if it, ok := ts.Type.(*ast.InterfaceType); ok && it.Methods != nil {
		t.EmbeddedTypes = extractInterfaceEmbedded(it.Methods)
		t.InterfaceMethods = extractInterfaceMethods(it.Methods)
	}

	return t
}

// extractVariables estrae variabili package-level.
func extractVariables(pkgPath string, vs *ast.ValueSpec, gen *ast.GenDecl, fset *token.FileSet, root string, cfg ExtractConfig) []*schema.CLDKVariable {
	var vars []*schema.CLDKVariable

	typeStr := ""
	if vs.Type != nil {
		typeStr = exprString(vs.Type)
	}

	doc := ""
	if gen.Doc != nil {
		doc = cleanDoc(gen.Doc.Text())
	} else if vs.Doc != nil {
		doc = cleanDoc(vs.Doc.Text())
	}

	for _, ident := range vs.Names {
		v := &schema.CLDKVariable{
			QualifiedName: fmt.Sprintf("%s.%s", pkgPath, ident.Name),
			Name:          ident.Name,
			Type:          typeStr,
			Exported:      isExported(ident.Name),
			Documentation: doc,
		}
		if cfg.EmitPositions != "minimal" {
			v.Position = posOf(fset, ident.Pos(), root)
		}
		vars = append(vars, v)
	}

	return vars
}

// extractConstants estrae costanti package-level.
func extractConstants(pkgPath string, vs *ast.ValueSpec, gen *ast.GenDecl, fset *token.FileSet, root string, cfg ExtractConfig) []*schema.CLDKConstant {
	var consts []*schema.CLDKConstant

	typeStr := ""
	if vs.Type != nil {
		typeStr = exprString(vs.Type)
	}

	doc := ""
	if gen.Doc != nil {
		doc = gen.Doc.Text()
	} else if vs.Doc != nil {
		doc = vs.Doc.Text()
	}

	for i, ident := range vs.Names {
		c := &schema.CLDKConstant{
			QualifiedName: fmt.Sprintf("%s.%s", pkgPath, ident.Name),
			Name:          ident.Name,
			Type:          typeStr,
			Exported:      isExported(ident.Name),
			Documentation: doc,
		}
		// Valore della costante
		if i < len(vs.Values) {
			c.Value = exprString(vs.Values[i])
		}
		if cfg.EmitPositions != "minimal" {
			c.Position = posOf(fset, ident.Pos(), root)
		}
		consts = append(consts, c)
	}

	return consts
}

// extractParameters estrae i parametri da un FieldList.
func extractParameters(fl *ast.FieldList) []schema.CLDKParameter {
	if fl == nil {
		return []schema.CLDKParameter{}
	}

	var params []schema.CLDKParameter
	for _, f := range fl.List {
		typeStr := exprString(f.Type)
		variadic := false
		if _, ok := f.Type.(*ast.Ellipsis); ok {
			variadic = true
		}

		if len(f.Names) == 0 {
			params = append(params, schema.CLDKParameter{
				Type:     typeStr,
				Variadic: variadic,
			})
		} else {
			for _, name := range f.Names {
				params = append(params, schema.CLDKParameter{
					Name:     name.Name,
					Type:     typeStr,
					Variadic: variadic,
				})
			}
		}
	}
	return params
}

// extractTypeParams estrae i parametri di tipo generici.
func extractTypeParams(fl *ast.FieldList) []schema.CLDKTypeParam {
	if fl == nil {
		return nil
	}

	var params []schema.CLDKTypeParam
	for _, f := range fl.List {
		constraint := exprString(f.Type)
		for _, name := range f.Names {
			params = append(params, schema.CLDKTypeParam{
				Name:       name.Name,
				Constraint: constraint,
			})
		}
	}
	return params
}

// extractFields estrae i campi di una struct.
func extractFields(fl *ast.FieldList, fset *token.FileSet, root string, cfg ExtractConfig) []schema.CLDKField {
	if fl == nil {
		return nil
	}

	var fields []schema.CLDKField
	for _, f := range fl.List {
		typeStr := exprString(f.Type)
		tag := ""
		if f.Tag != nil {
			tag = f.Tag.Value
		}

		if len(f.Names) == 0 {
			// Embedded field
			name := typeStr
			if star, ok := f.Type.(*ast.StarExpr); ok {
				name = exprString(star.X)
			}
			field := schema.CLDKField{
				Name:     name,
				Type:     typeStr,
				Tag:      tag,
				Exported: isExported(name),
				Embedded: true,
			}
			if cfg.EmitPositions != "minimal" {
				field.Position = posOf(fset, f.Pos(), root)
			}
			fields = append(fields, field)
		} else {
			for _, ident := range f.Names {
				field := schema.CLDKField{
					Name:     ident.Name,
					Type:     typeStr,
					Tag:      tag,
					Exported: isExported(ident.Name),
					Embedded: false,
				}
				if cfg.EmitPositions != "minimal" {
					field.Position = posOf(fset, ident.Pos(), root)
				}
				fields = append(fields, field)
			}
		}
	}
	return fields
}

// extractEmbeddedTypes estrae i tipi embedded da una struct.
func extractEmbeddedTypes(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}

	var embedded []string
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			embedded = append(embedded, exprString(f.Type))
		}
	}
	return embedded
}

// extractInterfaceEmbedded estrae le interfacce embedded.
func extractInterfaceEmbedded(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}

	var embedded []string
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			embedded = append(embedded, exprString(f.Type))
		}
	}
	return embedded
}

// extractInterfaceMethods estrae i metodi dichiarati in un'interfaccia.
func extractInterfaceMethods(fl *ast.FieldList) []schema.CLDKInterfaceMethod {
	if fl == nil {
		return nil
	}

	var methods []schema.CLDKInterfaceMethod
	for _, f := range fl.List {
		if len(f.Names) > 0 {
			if ft, ok := f.Type.(*ast.FuncType); ok {
				name := f.Names[0].Name
				im := schema.CLDKInterfaceMethod{
					Name:       name,
					Signature:  buildInterfaceMethodSig(name, ft),
					Parameters: extractParameters(ft.Params),
					Results:    extractParameters(ft.Results),
				}
				if f.Doc != nil {
					im.Documentation = cleanDoc(f.Doc.Text())
				} else if f.Comment != nil {
					im.Documentation = cleanDoc(f.Comment.Text())
				}
				methods = append(methods, im)
			}
		}
	}
	return methods
}

// buildInterfaceMethodSig costruisce la signature di un metodo di interfaccia.
func buildInterfaceMethodSig(name string, ft *ast.FuncType) string {
	params := []string{}
	if ft.Params != nil {
		for _, f := range ft.Params.List {
			t := exprString(f.Type)
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
	if ft.Results != nil {
		for _, f := range ft.Results.List {
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

	sig := name + "(" + strings.Join(params, ", ") + ")"
	if len(res) == 1 {
		sig += " " + res[0]
	} else if len(res) > 1 {
		sig += " (" + strings.Join(res, ", ") + ")"
	}
	return sig
}

// populateCallExamples popola CallExamples per ogni callable analizzando i call sites.
func populateCallExamples(pkg *schema.CLDKPackage) {
	// Costruisci indice: nome funzione -> qualified name
	nameIndex := make(map[string][]string)
	for qn, cd := range pkg.CallableDeclarations {
		nameIndex[cd.Name] = append(nameIndex[cd.Name], qn)
	}

	// Per ogni callable, cerca chi lo chiama
	examples := make(map[string][]string) // qn -> examples
	for _, caller := range pkg.CallableDeclarations {
		if caller.Body == nil {
			continue
		}
		for _, cs := range caller.Body.CallSites {
			// Cerca il target tra le callable del package
			targetName := extractCallTargetName(cs.Target)
			for _, qn := range nameIndex[targetName] {
				existing := examples[qn]
				if len(existing) >= 3 {
					continue
				}
				example := fmt.Sprintf("called by %s() [%s]", caller.Name, cs.Kind)
				// Evita duplicati
				duplicate := false
				for _, e := range existing {
					if e == example {
						duplicate = true
						break
					}
				}
				if !duplicate {
					examples[qn] = append(existing, example)
				}
			}
		}
	}

	// Assegna gli esempi
	for qn, exs := range examples {
		if cd, ok := pkg.CallableDeclarations[qn]; ok {
			cd.CallExamples = exs
		}
	}
}

// extractCallTargetName estrae il nome della funzione target da una call expression.
// Gestisce pattern come "pkg.Func", "obj.Method", "Func".
func extractCallTargetName(target string) string {
	// Rimuovi prefissi come "pkg." o "obj."
	if idx := strings.LastIndex(target, "."); idx >= 0 {
		return target[idx+1:]
	}
	return target
}

// extractFunctionBody estrae informazioni sul corpo della funzione.
func extractFunctionBody(body *ast.BlockStmt, fset *token.FileSet, root string, cfg ExtractConfig) *schema.CLDKFunctionBody {
	startPos := fset.Position(body.Pos())
	endPos := fset.Position(body.End())

	fb := &schema.CLDKFunctionBody{
		StartLine: startPos.Line,
		EndLine:   endPos.Line,
		LineCount: endPos.Line - startPos.Line + 1,
	}

	// Estrai call sites se richiesto
	if cfg.IncludeCallSites {
		fb.CallSites = extractCallSites(body, fset, root)
	}

	return fb
}

// extractCallSites estrae le chiamate a funzione nel corpo.
func extractCallSites(body *ast.BlockStmt, fset *token.FileSet, root string) []schema.CLDKCallSite {
	var sites []schema.CLDKCallSite

	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			target := exprString(x.Fun)
			site := schema.CLDKCallSite{
				Target:   target,
				Position: posOf(fset, x.Pos(), root),
				Kind:     "call",
			}
			sites = append(sites, site)

		case *ast.GoStmt:
			target := exprString(x.Call.Fun)
			site := schema.CLDKCallSite{
				Target:   target,
				Position: posOf(fset, x.Pos(), root),
				Kind:     "go",
			}
			sites = append(sites, site)

		case *ast.DeferStmt:
			target := exprString(x.Call.Fun)
			site := schema.CLDKCallSite{
				Target:   target,
				Position: posOf(fset, x.Pos(), root),
				Kind:     "defer",
			}
			sites = append(sites, site)
		}
		return true
	})

	return sites
}

// extractReceiverTypeName estrae il nome del tipo receiver.
func extractReceiverTypeName(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	f := fl.List[0]
	return extractBaseTypeName(f.Type)
}

// extractBaseTypeName estrae il nome base di un tipo (rimuove * se presente).
func extractBaseTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return extractBaseTypeName(t.X)
	case *ast.IndexExpr:
		return extractBaseTypeName(t.X)
	case *ast.IndexListExpr:
		return extractBaseTypeName(t.X)
	default:
		return exprString(e)
	}
}

// extractReceiverInfo estrae nome del tipo e se è pointer.
func extractReceiverInfo(fl *ast.FieldList) (typeName string, isPtr bool) {
	if fl == nil || len(fl.List) == 0 {
		return "", false
	}
	f := fl.List[0]

	switch t := f.Type.(type) {
	case *ast.StarExpr:
		return extractBaseTypeName(t.X), true
	default:
		return extractBaseTypeName(f.Type), false
	}
}

// ============================================================================
// Helper functions
// ============================================================================

func posOf(fset *token.FileSet, p token.Pos, root string) *schema.CLDKPosition {
	pos := fset.Position(p)
	if !pos.IsValid() {
		return nil
	}

	file := pos.Filename
	if rel, err := filepath.Rel(root, file); err == nil {
		file = filepath.ToSlash(rel)
	}

	return &schema.CLDKPosition{
		File:        file,
		StartLine:   pos.Line,
		StartColumn: pos.Column,
	}
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r := []rune(name)
	return unicode.IsUpper(r[0])
}

func exprString(e ast.Expr) string {
	if e == nil {
		return ""
	}
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, token.NewFileSet(), e)
	return strings.TrimSpace(buf.String())
}

func buildSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	params := []string{}
	if fn.Type.Params != nil {
		for _, f := range fn.Type.Params.List {
			t := exprString(f.Type)
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

	sig := "func "
	if fn.Recv != nil {
		recv := ""
		if len(fn.Recv.List) > 0 {
			recv = exprString(fn.Recv.List[0].Type)
		}
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
		return "named"
	}
}

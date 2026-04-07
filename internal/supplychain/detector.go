// Package supplychain rileva potenziali vettori di attacco supply chain
// nel codice sorgente Go, ispirato alla tassonomia GoSurf.
package supplychain

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Detect analizza un package e rileva potenziali vettori di attacco supply chain.
func Detect(pkg *packages.Package, fset *token.FileSet, root string) []schema.SupplyChainVector {
	if pkg == nil {
		return nil
	}

	var vectors []schema.SupplyChainVector

	for _, file := range pkg.Syntax {
		if file == nil {
			continue
		}

		relFile := ""
		pos := fset.Position(file.Pos())
		if pos.IsValid() {
			relFile = pos.Filename
			if rel, err := filepath.Rel(root, relFile); err == nil {
				relFile = filepath.ToSlash(rel)
			}
		}

		// 1. Scansione commenti per direttive pericolose
		vectors = append(vectors, detectDirectives(file, fset, root, relFile)...)

		// 2. Scansione import per CGo e plugin
		vectors = append(vectors, detectDangerousImports(file, fset, root, relFile)...)

		// 3. Scansione per init() side-effects e global side-effects
		vectors = append(vectors, detectInitSideEffects(pkg.PkgPath, file, fset, root, relFile)...)

		// 4. Scansione per global variable init con side-effects
		vectors = append(vectors, detectGlobalSideEffects(file, fset, root, relFile)...)

		// 5. Scansione per unsafe usage
		vectors = append(vectors, detectUnsafeUsage(file, fset, root, relFile)...)
	}

	return vectors
}

// ============================================================================
// Directive Detection (//go:generate, //go:linkname)
// ============================================================================

// detectDirectives cerca direttive pericolose nei commenti del file.
func detectDirectives(file *ast.File, fset *token.FileSet, root, relFile string) []schema.SupplyChainVector {
	var vectors []schema.SupplyChainVector

	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := c.Text

			// //go:generate — può eseguire comandi shell arbitrari
			if strings.HasPrefix(text, "//go:generate ") {
				cmd := strings.TrimPrefix(text, "//go:generate ")
				severity := classifyGenerateCmd(cmd)
				vectors = append(vectors, schema.SupplyChainVector{
					Kind:     "go_generate",
					Detail:   truncate(cmd, 200),
					Severity: severity,
					File:     relFile,
					Position: posOf(fset, c.Pos(), root),
				})
			}

			// //go:linkname — accede a funzioni interne non esportate
			if strings.HasPrefix(text, "//go:linkname ") {
				detail := strings.TrimPrefix(text, "//go:linkname ")
				vectors = append(vectors, schema.SupplyChainVector{
					Kind:     "go_linkname",
					Detail:   truncate(detail, 200),
					Severity: "high",
					File:     relFile,
					Position: posOf(fset, c.Pos(), root),
				})
			}

			// //go:nosplit, //go:noescape — tecniche avanzate insolite
			if strings.HasPrefix(text, "//go:nosplit") || strings.HasPrefix(text, "//go:noescape") {
				vectors = append(vectors, schema.SupplyChainVector{
					Kind:     "compiler_directive",
					Detail:   text,
					Severity: "low",
					File:     relFile,
					Position: posOf(fset, c.Pos(), root),
				})
			}
		}
	}

	return vectors
}

// classifyGenerateCmd classifica la pericolosità di un comando go:generate.
func classifyGenerateCmd(cmd string) string {
	cmdLower := strings.ToLower(cmd)
	// Comandi shell diretti
	dangerousPatterns := []string{
		"sh ", "bash ", "cmd ", "powershell", "curl ", "wget ",
		"rm ", "del ", "net ", "nc ", "ncat", "python", "ruby",
		"perl", "node ", "exec", "eval", "/bin/",
	}
	for _, p := range dangerousPatterns {
		if strings.Contains(cmdLower, p) {
			return "critical"
		}
	}
	// go run — può eseguire codice Go arbitrario
	if strings.Contains(cmdLower, "go run") {
		return "high"
	}
	return "medium"
}

// ============================================================================
// Dangerous Imports Detection
// ============================================================================

// detectDangerousImports cerca import pericolosi (CGo, plugin, unsafe).
func detectDangerousImports(file *ast.File, fset *token.FileSet, root, relFile string) []schema.SupplyChainVector {
	var vectors []schema.SupplyChainVector

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)

		switch path {
		case "C":
			vectors = append(vectors, schema.SupplyChainVector{
				Kind:     "cgo_usage",
				Detail:   "import \"C\" — enables calling C code, bypasses Go safety",
				Severity: "high",
				File:     relFile,
				Position: posOf(fset, imp.Pos(), root),
			})

		case "plugin":
			vectors = append(vectors, schema.SupplyChainVector{
				Kind:     "plugin_load",
				Detail:   "import \"plugin\" — enables dynamic code loading at runtime",
				Severity: "high",
				File:     relFile,
				Position: posOf(fset, imp.Pos(), root),
			})

		case "unsafe":
			vectors = append(vectors, schema.SupplyChainVector{
				Kind:     "unsafe_usage",
				Detail:   "import \"unsafe\" — direct memory access, bypasses type safety",
				Severity: "medium",
				File:     relFile,
				Position: posOf(fset, imp.Pos(), root),
			})
		}
	}

	return vectors
}

// ============================================================================
// Init Side-Effects Detection
// ============================================================================

// detectInitSideEffects cerca init() con chiamate pericolose.
func detectInitSideEffects(pkgPath string, file *ast.File, fset *token.FileSet, root, relFile string) []schema.SupplyChainVector {
	var vectors []schema.SupplyChainVector

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "init" || fn.Recv != nil {
			continue
		}

		if fn.Body == nil {
			continue
		}

		// Cerca chiamate pericolose dentro init()
		dangerousCalls := findDangerousCalls(fn.Body)
		if len(dangerousCalls) > 0 {
			detail := "init() calls: " + strings.Join(dangerousCalls, ", ")
			vectors = append(vectors, schema.SupplyChainVector{
				Kind:     "init_side_effect",
				Detail:   truncate(detail, 300),
				Severity: classifyInitCalls(dangerousCalls),
				File:     relFile,
				Position: posOf(fset, fn.Pos(), root),
			})
		}
	}

	return vectors
}

// ============================================================================
// Global Variable Side-Effects Detection
// ============================================================================

// detectGlobalSideEffects cerca variabili globali con inizializzatori che hanno side-effects.
func detectGlobalSideEffects(file *ast.File, fset *token.FileSet, root, relFile string) []schema.SupplyChainVector {
	var vectors []schema.SupplyChainVector

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}

		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, val := range vs.Values {
				// Cerca call expressions nell'inizializzatore
				calls := findCallsInExpr(val)
				if len(calls) > 0 {
					dangerousCalls := filterDangerousCalls(calls)
					if len(dangerousCalls) > 0 {
						detail := "global var init calls: " + strings.Join(dangerousCalls, ", ")
						vectors = append(vectors, schema.SupplyChainVector{
							Kind:     "global_side_effect",
							Detail:   truncate(detail, 300),
							Severity: "medium",
							File:     relFile,
							Position: posOf(fset, vs.Pos(), root),
						})
					}
				}
			}
		}
	}

	return vectors
}

// ============================================================================
// Unsafe Usage Detection
// ============================================================================

// detectUnsafeUsage cerca uso di reflect per dynamic dispatch sospetto.
func detectUnsafeUsage(file *ast.File, fset *token.FileSet, root, relFile string) []schema.SupplyChainVector {
	var vectors []schema.SupplyChainVector

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		target := exprToString(call.Fun)

		// reflect.Value.Call — dynamic function invocation
		if strings.Contains(target, "reflect") && (strings.HasSuffix(target, ".Call") || strings.HasSuffix(target, ".Invoke")) {
			vectors = append(vectors, schema.SupplyChainVector{
				Kind:     "dynamic_dispatch",
				Detail:   target + " — reflection-based dynamic call",
				Severity: "medium",
				File:     relFile,
				Position: posOf(fset, call.Pos(), root),
			})
		}

		return true
	})

	return vectors
}

// ============================================================================
// Helper Functions
// ============================================================================

// dangerousCallPatterns sono pattern di chiamate pericolose in init() e global init.
var dangerousCallPatterns = []string{
	"os.Exec", "exec.Command", "exec.CommandContext",
	"syscall.Exec", "syscall.ForkExec",
	"http.Get", "http.Post", "http.Do",
	"net.Dial", "net.Listen",
	"os.WriteFile", "os.Create", "os.Remove", "os.RemoveAll",
	"os.Setenv",
	"plugin.Open",
}

// findDangerousCalls cerca chiamate pericolose in un block statement.
func findDangerousCalls(body *ast.BlockStmt) []string {
	var found []string

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		target := exprToString(call.Fun)
		for _, pattern := range dangerousCallPatterns {
			if strings.Contains(target, pattern) || strings.HasSuffix(target, lastPart(pattern)) {
				found = append(found, target)
				break
			}
		}
		return true
	})

	return dedupStrings(found)
}

// findCallsInExpr trova tutte le call expression in un'espressione.
func findCallsInExpr(expr ast.Expr) []string {
	var calls []string

	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		calls = append(calls, exprToString(call.Fun))
		return true
	})

	return calls
}

// filterDangerousCalls filtra solo le chiamate corrispondenti ai pattern pericolosi.
func filterDangerousCalls(calls []string) []string {
	var dangerous []string
	for _, c := range calls {
		for _, pattern := range dangerousCallPatterns {
			if strings.Contains(c, pattern) || strings.HasSuffix(c, lastPart(pattern)) {
				dangerous = append(dangerous, c)
				break
			}
		}
	}
	return dedupStrings(dangerous)
}

// classifyInitCalls classifica la severità dei side-effects in init().
func classifyInitCalls(calls []string) string {
	for _, c := range calls {
		lower := strings.ToLower(c)
		if strings.Contains(lower, "exec") || strings.Contains(lower, "syscall") {
			return "critical"
		}
		if strings.Contains(lower, "http") || strings.Contains(lower, "net.") || strings.Contains(lower, "dial") {
			return "critical"
		}
	}
	return "high"
}

// exprToString converte un'espressione AST in stringa.
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return exprToString(e.X)
	case *ast.ParenExpr:
		return exprToString(e.X)
	default:
		return ""
	}
}

// lastPart restituisce l'ultima parte di un path separato da punto.
func lastPart(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// posOf costruisce una CLDKPosition da un token.Pos.
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

// truncate tronca una stringa a una lunghezza massima.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// dedupStrings rimuove duplicati da uno slice di stringhe.
func dedupStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

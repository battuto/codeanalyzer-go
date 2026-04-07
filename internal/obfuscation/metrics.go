// Package obfuscation calcola metriche euristiche per rilevare
// codice potenzialmente offuscato in pacchetti Go.
package obfuscation

import (
	"go/ast"
	"go/token"
	"math"
	"regexp"
	"unicode"

	"golang.org/x/tools/go/packages"

	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Compute calcola le metriche di offuscamento per un package.
func Compute(pkg *packages.Package) *schema.ObfuscationMetrics {
	if pkg == nil {
		return nil
	}

	m := &schema.ObfuscationMetrics{}

	var funcNames []string
	var varNames []string
	var exportedFuncs int
	var documentedFuncs int

	for _, file := range pkg.Syntax {
		if file == nil {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				name := x.Name.Name
				funcNames = append(funcNames, name)

				// Conta funzioni esportate e documentate
				if isExported(name) {
					exportedFuncs++
					if x.Doc != nil && len(x.Doc.List) > 0 {
						documentedFuncs++
					}
				}

				// Conta parametri e variabili locali
				if x.Type.Params != nil {
					for _, f := range x.Type.Params.List {
						for _, ident := range f.Names {
							varNames = append(varNames, ident.Name)
						}
					}
				}

				// Scansiona il body per XOR e variabili locali
				if x.Body != nil {
					m.XorOperations += countXorOps(x.Body)
					varNames = append(varNames, collectLocalVarNames(x.Body)...)
				}

			case *ast.GenDecl:
				if x.Tok == token.VAR {
					for _, spec := range x.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, ident := range vs.Names {
								varNames = append(varNames, ident.Name)
							}
						}
					}
				}
			}
			return true
		})
	}

	// Calcola metriche
	m.AvgFuncNameLen = avgLength(funcNames)
	m.AvgVarNameLen = avgLength(varNames)
	m.ShortNamesRatio = shortNamesRatio(funcNames, varNames)

	if exportedFuncs > 0 {
		m.DocCoverage = math.Round(float64(documentedFuncs)/float64(exportedFuncs)*10000) / 100
	}

	m.HasGarblePatterns = detectGarblePatterns(funcNames)

	return m
}

// ============================================================================
// XOR Operations Counter
// ============================================================================

// countXorOps conta le operazioni XOR (^) in un body di funzione.
func countXorOps(body *ast.BlockStmt) int {
	count := 0
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if x.Op == token.XOR {
				count++
			}
		case *ast.AssignStmt:
			if x.Tok == token.XOR_ASSIGN { // ^=
				count++
			}
		}
		return true
	})
	return count
}

// ============================================================================
// Name Analysis
// ============================================================================

// collectLocalVarNames raccoglie i nomi delle variabili locali da un body.
func collectLocalVarNames(body *ast.BlockStmt) []string {
	var names []string

	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE { // :=
				for _, lhs := range x.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
						names = append(names, ident.Name)
					}
				}
			}
		case *ast.RangeStmt:
			if ident, ok := x.Key.(*ast.Ident); ok && ident.Name != "_" {
				names = append(names, ident.Name)
			}
			if x.Value != nil {
				if ident, ok := x.Value.(*ast.Ident); ok && ident.Name != "_" {
					names = append(names, ident.Name)
				}
			}
		}
		return true
	})

	return names
}

// avgLength calcola la lunghezza media delle stringhe.
func avgLength(names []string) float64 {
	if len(names) == 0 {
		return 0
	}
	total := 0
	for _, n := range names {
		total += len(n)
	}
	return math.Round(float64(total)/float64(len(names))*100) / 100
}

// shortNamesRatio calcola la percentuale di nomi ≤ 2 caratteri.
// Esclude nomi convenzionali Go come "i", "j", "k", "n", "ok", "err".
func shortNamesRatio(funcNames, varNames []string) float64 {
	conventionalShort := map[string]bool{
		"i": true, "j": true, "k": true, "n": true,
		"ok": true, "fn": true, "wg": true, "mu": true,
		"t": true, "b": true, "s": true, "r": true, "w": true,
	}

	allNames := append(funcNames, varNames...)
	if len(allNames) == 0 {
		return 0
	}

	shortCount := 0
	for _, name := range allNames {
		if len(name) <= 2 && !conventionalShort[name] {
			shortCount++
		}
	}

	return math.Round(float64(shortCount)/float64(len(allNames))*10000) / 100
}

// garblePattern matches function names that look randomly generated.
// Garble generates names like "Kk3Io5", "aB4c2D", sequences of hex-like chars.
var garblePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{3,12}$`)

// detectGarblePatterns verifica se i nomi funzione mostrano pattern tipici
// dell'offuscatore Garble (alta entropia nei nomi, mix case random).
func detectGarblePatterns(funcNames []string) bool {
	if len(funcNames) < 5 {
		return false
	}

	randomLikeCount := 0
	for _, name := range funcNames {
		if isRandomLikeName(name) {
			randomLikeCount++
		}
	}

	// Se più del 40% dei nomi sembra random, probabilmente offuscato
	return float64(randomLikeCount)/float64(len(funcNames)) > 0.4
}

// isRandomLikeName verifica se un nome sembra generato casualmente.
func isRandomLikeName(name string) bool {
	if len(name) < 4 || len(name) > 15 {
		return false
	}

	// Esclude nomi Go convenzionali
	if isCommonGoName(name) {
		return false
	}

	// Pattern: mix di maiuscole/minuscole/numeri senza parole riconoscibili
	hasUpper := false
	hasLower := false
	hasDigit := false
	transitions := 0

	runes := []rune(name)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			hasUpper = true
		} else if unicode.IsLower(r) {
			hasLower = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		}

		// Conta transizioni upper/lower/digit
		if i > 0 {
			prevCat := charCategory(runes[i-1])
			currCat := charCategory(r)
			if prevCat != currCat {
				transitions++
			}
		}
	}

	// I nomi Garble tipicamente hanno alta varianza di categoria
	if hasUpper && hasLower && hasDigit && transitions >= 3 {
		return true
	}

	return false
}

func charCategory(r rune) int {
	switch {
	case unicode.IsUpper(r):
		return 0
	case unicode.IsLower(r):
		return 1
	case unicode.IsDigit(r):
		return 2
	default:
		return 3
	}
}

func isCommonGoName(name string) bool {
	common := map[string]bool{
		"main": true, "init": true, "New": true, "String": true,
		"Error": true, "Read": true, "Write": true, "Close": true,
		"Open": true, "Start": true, "Stop": true, "Run": true,
		"Get": true, "Set": true, "Add": true, "Delete": true,
		"Handle": true, "Serve": true, "Listen": true,
		"Marshal": true, "Unmarshal": true, "Parse": true,
		"Format": true, "Print": true, "Println": true,
	}
	return common[name]
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}

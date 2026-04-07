// Package strings fornisce l'estrazione e la classificazione di string literals
// dal codice sorgente Go per analisi di sicurezza e malware detection.
package strings

import (
	"go/ast"
	"go/token"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Config configura l'estrazione delle stringhe.
type Config struct {
	MinLength     int     // lunghezza minima della stringa (default: 4)
	MaxStrings    int     // numero massimo di stringhe per package (default: 500)
	EntropyThreshold float64 // soglia minima entropia per inclusione (0 = tutte)
}

// DefaultConfig restituisce la configurazione predefinita.
func DefaultConfig() Config {
	return Config{
		MinLength:        4,
		MaxStrings:       500,
		EntropyThreshold: 0,
	}
}

// Extract estrae tutte le string literals da un package.
func Extract(pkg *packages.Package, fset *token.FileSet, root string, cfg Config) []schema.CLDKStringLiteral {
	if pkg == nil {
		return nil
	}
	if cfg.MinLength <= 0 {
		cfg.MinLength = 4
	}
	if cfg.MaxStrings <= 0 {
		cfg.MaxStrings = 500
	}

	var result []schema.CLDKStringLiteral

	for _, file := range pkg.Syntax {
		if file == nil {
			continue
		}

		// Mappa funzione scope per le stringhe in ciascuna funzione
		funcScopes := buildFuncScopes(pkg.PkgPath, file)

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			// Unquote la stringa
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				// Prova con raw string
				val = strings.Trim(lit.Value, "`")
			}

			// Filtro lunghezza minima
			if len(val) < cfg.MinLength {
				return true
			}

			// Calcola entropia
			entropy := shannonEntropy(val)

			// Filtro entropia se configurato
			if cfg.EntropyThreshold > 0 && entropy < cfg.EntropyThreshold {
				return true
			}

			// Classifica la stringa
			category := classify(val)

			// Trova lo scope (funzione contenitrice)
			scope := findScope(fset, lit.Pos(), funcScopes)

			sl := schema.CLDKStringLiteral{
				Value:    truncateString(val, 200),
				Category: category,
				Entropy:  math.Round(entropy*100) / 100,
				Scope:    scope,
			}

			// Posizione
			pos := fset.Position(lit.Pos())
			if pos.IsValid() {
				file := pos.Filename
				if rel, err := filepath.Rel(root, file); err == nil {
					file = filepath.ToSlash(rel)
				}
				sl.Position = &schema.CLDKPosition{
					File:        file,
					StartLine:   pos.Line,
					StartColumn: pos.Column,
				}
			}

			result = append(result, sl)
			return true
		})
	}

	// Ordina: categorie interessanti prima, poi per entropia decrescente
	sort.SliceStable(result, func(i, j int) bool {
		pi := categoryPriority(result[i].Category)
		pj := categoryPriority(result[j].Category)
		if pi != pj {
			return pi < pj
		}
		return result[i].Entropy > result[j].Entropy
	})

	// Tronca al massimo
	if len(result) > cfg.MaxStrings {
		result = result[:cfg.MaxStrings]
	}

	return result
}

// ============================================================================
// String Classification
// ============================================================================

var (
	reURL         = regexp.MustCompile(`^https?://[^\s]+`)
	reIP          = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?$`)
	rePathWin     = regexp.MustCompile(`^[A-Z]:\\[^\s]+`)
	rePathUnix    = regexp.MustCompile(`^/(etc|home|tmp|var|usr|bin|sbin|opt|root|proc|sys|dev)/`)
	reBase64      = regexp.MustCompile(`^[A-Za-z0-9+/]{20,}={0,2}$`)
	reCommand     = regexp.MustCompile(`(?i)(cmd\.exe|powershell|/bin/(sh|bash|zsh)|wget\s|curl\s|chmod\s|chown\s)`)
	reCryptoWallet = regexp.MustCompile(`^(bc1|[13])[a-zA-HJ-NP-Z0-9]{25,39}$|^0x[a-fA-F0-9]{40}$|^4[0-9AB][1-9A-HJ-NP-Za-km-z]{93}$`)
	reDomain      = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.[a-zA-Z]{2,}$`)
	reRegistry    = regexp.MustCompile(`(?i)^(HKEY_|HKLM\\|HKCU\\|SOFTWARE\\)`)
	reEmail       = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	reMiningPool  = regexp.MustCompile(`(?i)(stratum\+tcp://|pool\.|mining\.|xmr|monero)`)
)

// classify classifica una stringa in una categoria di sicurezza.
func classify(s string) string {
	switch {
	case reURL.MatchString(s):
		return "url"
	case reIP.MatchString(s):
		return "ip"
	case rePathWin.MatchString(s):
		return "path_win"
	case rePathUnix.MatchString(s):
		return "path_unix"
	case reCommand.MatchString(s):
		return "command"
	case reCryptoWallet.MatchString(s):
		return "crypto_wallet"
	case reMiningPool.MatchString(s):
		return "mining_pool"
	case reRegistry.MatchString(s):
		return "registry"
	case reEmail.MatchString(s):
		return "email"
	case reDomain.MatchString(s):
		return "domain"
	case reBase64.MatchString(s):
		return "base64"
	default:
		return "other"
	}
}

// categoryPriority restituisce la priorità di una categoria (0 = più importante).
func categoryPriority(cat string) int {
	switch cat {
	case "url":
		return 0
	case "ip":
		return 1
	case "command":
		return 2
	case "crypto_wallet", "mining_pool":
		return 3
	case "domain":
		return 4
	case "path_win", "path_unix":
		return 5
	case "registry":
		return 6
	case "email":
		return 7
	case "base64":
		return 8
	default:
		return 99
	}
}

// ============================================================================
// Shannon Entropy
// ============================================================================

// shannonEntropy calcola l'entropia di Shannon di una stringa.
// Valori tipici: testo normale 3.5-4.5, dati random/cifrati/base64 5.0+
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}

	freq := make(map[rune]int)
	for _, c := range s {
		freq[c]++
	}

	length := float64(len([]rune(s)))
	entropy := 0.0
	for _, count := range freq {
		p := float64(count) / length
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// ============================================================================
// Scope Resolution
// ============================================================================

// funcScope rappresenta il range di posizioni di una funzione.
type funcScope struct {
	QualifiedName string
	Start         token.Pos
	End           token.Pos
}

// buildFuncScopes costruisce una mappa di scope funzione per il file.
func buildFuncScopes(pkgPath string, file *ast.File) []funcScope {
	var scopes []funcScope

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		name := fn.Name.Name
		var qn string

		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recvType := extractRecvTypeName(fn.Recv.List[0].Type)
			// Check pointer receiver
			if _, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
				qn = pkgPath + ".(*" + recvType + ")." + name
			} else {
				qn = pkgPath + "." + recvType + "." + name
			}
		} else {
			qn = pkgPath + "." + name
		}

		scopes = append(scopes, funcScope{
			QualifiedName: qn,
			Start:         fn.Body.Pos(),
			End:           fn.Body.End(),
		})
	}

	return scopes
}

// extractRecvTypeName estrae il nome del tipo receiver.
func extractRecvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return extractRecvTypeName(t.X)
	case *ast.IndexExpr:
		return extractRecvTypeName(t.X)
	case *ast.IndexListExpr:
		return extractRecvTypeName(t.X)
	default:
		return ""
	}
}

// findScope trova la funzione che contiene una data posizione.
func findScope(fset *token.FileSet, pos token.Pos, scopes []funcScope) string {
	for _, s := range scopes {
		if pos >= s.Start && pos <= s.End {
			return s.QualifiedName
		}
	}
	return "" // package-level
}

// truncateString tronca una stringa a una lunghezza massima.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

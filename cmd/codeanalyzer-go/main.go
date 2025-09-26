package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/codellm-devkit/codeanalyzer-go/internal/astx"
	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/emit"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

type flags struct {
	root          string
	mode          string
	out           string
	cg            string
	includeTest   bool
	excludeDirs   string
	onlyPkg       string
	emitPositions string
}

func parseFlags() flags {
	var f flags
	flag.StringVar(&f.root, "root", ".", "root folder of the Go project to analyze")
	flag.StringVar(&f.mode, "mode", "full", "analysis mode: symbol-table|call-graph|full")
	flag.StringVar(&f.out, "out", "-", "output path or '-' for STDOUT")
	flag.StringVar(&f.cg, "cg", "cha", "callgraph algo: cha|rta")
	flag.BoolVar(&f.includeTest, "include-test", false, "include *_test.go files")
	flag.StringVar(&f.excludeDirs, "exclude-dirs", "", "comma-separated directory basenames to exclude (e.g., vendor,.git)")
	flag.StringVar(&f.onlyPkg, "only-pkg", "", "comma-separated package path filters to include (substring match)")
	flag.StringVar(&f.emitPositions, "emit-positions", "detailed", "positions verbosity: detailed|minimal")
	flag.Parse()
	return f
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	f := parseFlags()

	abs, err := filepath.Abs(f.root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve root: %v\n", err)
		os.Exit(2)
	}

	ldOpts := loader.Options{
		IncludeTest: f.includeTest,
		ExcludeDirs: splitCSV(f.excludeDirs),
		OnlyPkg:     splitCSV(f.onlyPkg),
	}
	prog, err := loader.LoadWithOptions(abs, ldOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(2)
	}

	if os.Getenv("LOG_LEVEL") == "debug" {
		fmt.Fprintf(os.Stderr, "[debug] root=%s mode=%s cg=%s include-test=%v emit-positions=%s\n", abs, f.mode, f.cg, f.includeTest, f.emitPositions)
		fmt.Fprintf(os.Stderr, "[debug] exclude-dirs=%v only-pkg=%v files=%d\n", ldOpts.ExcludeDirs, ldOpts.OnlyPkg, len(prog.Files))
		// Conta pacchetti via go/packages per il riepilogo
		pkgs, _ := countPackages(abs, f.includeTest, ldOpts.ExcludeDirs, ldOpts.OnlyPkg)
		fmt.Fprintf(os.Stderr, "[debug] pkgs=%d\n", pkgs)
	}

	var st *schema.SymbolTable
	var cg *schema.CallGraph

	switch f.mode {
	case "symbol-table":
		st = astx.ExtractSymbols(prog)
	case "call-graph":
		cg = buildCG(abs, f)
	case "full":
		st = astx.ExtractSymbols(prog)
		cg = buildCG(abs, f)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", f.mode)
		os.Exit(2)
	}

	out := struct {
		Language  string              `json:"language"`
		Symbols   *schema.SymbolTable `json:"symbol_table,omitempty"`
		CallGraph *schema.CallGraph   `json:"call_graph,omitempty"`
	}{
		Language:  "go",
		Symbols:   st,
		CallGraph: cg,
	}

	var w *os.File = os.Stdout
	if f.out != "-" && f.out != "" {
		fd, err := os.Create(f.out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open out: %v\n", err)
			os.Exit(2)
		}
		defer fd.Close()
		w = fd
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(2)
	}
	_ = emit.Nop() // avoid import removal
}

func buildCG(root string, f flags) *schema.CallGraph {
	cfg := astx.CallGraphConfig{
		Root:          root,
		Algo:          f.cg,
		IncludeTest:   f.includeTest,
		ExcludeDirs:   splitCSV(f.excludeDirs),
		OnlyPkg:       splitCSV(f.onlyPkg),
		EmitPositions: f.emitPositions,
	}
	cg, err := astx.BuildCallGraph(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "call-graph: %v\n", err)
		// fallback a placeholder vuoto per non rompere lo schema
		return &schema.CallGraph{Language: "go", Nodes: []schema.CGNode{}, Edges: []schema.CGEdge{}}
	}
	return cg
}

// countPackages carica pacchetti con go/packages e applica filtri base per ottenere un conteggio.
func countPackages(root string, includeTest bool, excludeDirs, onlyPkg []string) (int, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles,
		Dir:   root,
		Tests: includeTest,
		Env:   os.Environ(),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return 0, err
	}
	// filtra
	ex := map[string]struct{}{}
	for _, d := range excludeDirs {
		ex[strings.TrimSpace(d)] = struct{}{}
	}
	keep := func(p *packages.Package) bool {
		if len(onlyPkg) > 0 {
			ok := false
			for _, s := range onlyPkg {
				s = strings.TrimSpace(s)
				if s != "" && strings.Contains(p.PkgPath, s) {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
		for _, f := range p.GoFiles {
			base := filepath.Base(filepath.Dir(f))
			if _, ok := ex[base]; ok {
				return false
			}
		}
		return true
	}
	count := 0
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		if keep(p) {
			count++
		}
	}
	return count, nil
}

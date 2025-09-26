package astx

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// CallGraphConfig raccoglie le opzioni per la costruzione del call graph.
type CallGraphConfig struct {
	Root          string
	Algo          string // "cha" | "rta"
	IncludeTest   bool
	ExcludeDirs   []string // directory names (basename) o path relative da escludere
	OnlyPkg       []string // filtra a questi package path (substring match)
	EmitPositions string   // "detailed" | "minimal"
}

// BuildCallGraph costruisce un call graph usando golang.org/x/tools.
func BuildCallGraph(cfg CallGraphConfig) (*schema.CallGraph, error) {
	// Normalizza root
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}

	// Caricamento pacchetti
	pkgs, _, err := loadPackages(root, cfg.IncludeTest)
	if err != nil {
		return nil, err
	}

	// Filtri exclude-dirs e only-pkg sui pacchetti iniziali
	pkgs = filterPackages(pkgs, cfg.ExcludeDirs, cfg.OnlyPkg)

	// Colleziona la chiusura di tutti i pacchetti inclusi gli import
	all := collectAllPackages(pkgs)

	// Carica l'intera stdlib solo se necessario (RTA) per evitare panics e completare i metadati
	if strings.ToLower(cfg.Algo) == "rta" {
		if stdPkgs, _ := ensureStdlib(root); len(stdPkgs) > 0 {
			all = append(all, stdPkgs...)
		}
	}

	// Deduplica pacchetti per PkgPath
	all = dedupPackages(all)

	// Costruzione SSA su tutti i pacchetti
	prog, ssaPkgs := ssautil.AllPackages(all, ssa.InstantiateGenerics)
	// Build SSA IR (fun bodies)
	prog.Build()

	// Costruisci call graph
	var cg *callgraph.Graph
	switch strings.ToLower(cfg.Algo) {
	case "rta":
		mainPkgs := ssautil.MainPackages(ssaPkgs)
		var roots []*ssa.Function
		for _, m := range mainPkgs {
			if fn := m.Func("main"); fn != nil {
				roots = append(roots, fn)
			}
		}
		res := rta.Analyze(roots, true)
		cg = res.CallGraph
	default: // "cha"
		cg = cha.CallGraph(prog)
	}

	// Normalizza in schema
	out := &schema.CallGraph{Language: "go", Nodes: []schema.CGNode{}, Edges: []schema.CGEdge{}}

	// Deduced sets
	nodeSet := map[string]schema.CGNode{}
	edgeSet := map[string]struct{}{}

	// Funzione helper per filtrare per onlyPkg
	inAllowedPkgs := func(f *ssa.Function) bool {
		if f == nil || f.Pkg == nil || f.Pkg.Pkg == nil {
			return len(cfg.OnlyPkg) == 0
		}
		if len(cfg.OnlyPkg) == 0 {
			return true
		}
		pp := f.Pkg.Pkg.Path()
		for _, s := range cfg.OnlyPkg {
			if s == "" {
				continue
			}
			if strings.Contains(pp, s) {
				return true
			}
		}
		return false
	}

	fset := prog.Fset
	emit := cfg.EmitPositions

	// Itera su tutti gli archi del grafo
	for _, n := range cg.Nodes {
		for _, e := range n.Out {
			if e == nil || e.Caller == nil || e.Callee == nil {
				continue
			}
			src := e.Caller.Func
			dst := e.Callee.Func
			if src == nil || dst == nil {
				continue
			}
			if !inAllowedPkgs(src) && !inAllowedPkgs(dst) {
				// se only-pkg è settato, filtra archi completamente esterni
				if len(cfg.OnlyPkg) > 0 {
					continue
				}
			}

			srcID := stableFuncID(src)
			dstID := stableFuncID(dst)
			if srcID == "" || dstID == "" {
				continue
			}

			// Aggiungi nodi se non presenti
			if _, ok := nodeSet[srcID]; !ok {
				nodeSet[srcID] = schema.CGNode{ID: srcID, Pos: cgPosOf(src, fset, emit)}
			}
			if _, ok := nodeSet[dstID]; !ok {
				nodeSet[dstID] = schema.CGNode{ID: dstID, Pos: cgPosOf(dst, fset, emit)}
			}

			k := srcID + "→" + dstID
			if _, ok := edgeSet[k]; !ok {
				edgeSet[k] = struct{}{}
			}
		}
	}

	// Riporta set a slice ordinati per stabilità
	for _, n := range nodeSet {
		out.Nodes = append(out.Nodes, n)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })

	for k := range edgeSet {
		// split k into src,dst
		if i := strings.IndexRune(k, '→'); i >= 0 {
			out.Edges = append(out.Edges, schema.CGEdge{Src: k[:i], Dst: k[i+len("→"):]})
		}
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].Src == out.Edges[j].Src {
			return out.Edges[i].Dst < out.Edges[j].Dst
		}
		return out.Edges[i].Src < out.Edges[j].Src
	})

	// Debug info
	if os.Getenv("LOG_LEVEL") == "debug" {
		fmt.Fprintf(os.Stderr, "[debug] go=%s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(os.Stderr, "[debug] callgraph algo=%s pkgs=%d nodes=%d edges=%d\n", cfg.Algo, len(pkgs), len(out.Nodes), len(out.Edges))
	}

	return out, nil
}

// loadPackages carica tutti i pacchetti sotto root usando go/packages.
func loadPackages(root string, includeTests bool) ([]*packages.Package, *token.FileSet, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedImports | packages.NeedModule,
		Dir:   root,
		Tests: includeTests,
		Env:   os.Environ(),
	}
	// Carica tutti i pacchetti sotto root
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, nil, fmt.Errorf("packages.Load: %w", err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		// Non bloccare: continua, ma logga a debug
		if os.Getenv("LOG_LEVEL") == "debug" {
			fmt.Fprintf(os.Stderr, "[debug] packages.Load reported %d issues\n", n)
		}
	}
	// Recupera fset
	var fset *token.FileSet
	if len(pkgs) > 0 {
		fset = pkgs[0].Fset
	} else {
		fset = token.NewFileSet()
	}
	return pkgs, fset, nil
}

// filterPackages applica i filtri di directory e package.
func filterPackages(pkgs []*packages.Package, excludeDirs, onlyPkg []string) []*packages.Package {
	if len(excludeDirs) == 0 && len(onlyPkg) == 0 {
		return pkgs
	}
	ex := map[string]struct{}{}
	for _, d := range excludeDirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		ex[d] = struct{}{}
	}

	keep := func(p *packages.Package) bool {
		if len(onlyPkg) > 0 {
			ok := false
			for _, s := range onlyPkg {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				if strings.Contains(p.PkgPath, s) {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
		if len(ex) == 0 {
			return true
		}
		// Se uno qualsiasi dei file del pacchetto sta in una dir esclusa, escludi il pkg.
		for _, f := range append([]string{}, p.GoFiles...) {
			base := filepath.Base(filepath.Dir(f))
			if _, ok := ex[base]; ok {
				return false
			}
		}
		return true
	}

	out := make([]*packages.Package, 0, len(pkgs))
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}

// collectAllPackages visita ricorsivamente Imports per includere tutte le dipendenze.
func collectAllPackages(roots []*packages.Package) []*packages.Package {
	seen := map[*packages.Package]struct{}{}
	var out []*packages.Package
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
		for _, ip := range p.Imports {
			visit(ip)
		}
	}
	for _, r := range roots {
		visit(r)
	}
	return out
}

// cgPosOf ritorna la posizione per un *ssa.Function rispettando EmitPositions.
func cgPosOf(f *ssa.Function, fset *token.FileSet, emit string) schema.Position {
	if strings.ToLower(emit) == "minimal" {
		return schema.Position{}
	}
	if f == nil {
		return schema.Position{}
	}
	p := fset.Position(f.Pos())
	if !p.IsValid() {
		return schema.Position{}
	}
	return schema.Position{File: p.Filename, Line: p.Line, Column: p.Column}
}

// stableFuncID genera un ID stabile pkgpath.Func o recv.(*)?Type.Method.
func stableFuncID(f *ssa.Function) string {
	if f == nil {
		return ""
	}
	// Builtins
	if f.Pkg == nil || f.Pkg.Pkg == nil {
		// e.g., runtime/internal, intrinsics, builtins: usa nome così com'è
		if f.Name() != "" {
			return f.Name()
		}
		return f.String()
	}
	pkg := f.Pkg.Pkg.Path()
	name := f.Name()
	// Receiver
	if r := f.Signature.Recv(); r != nil {
		t := r.Type().String() // es: *main.T
		return fmt.Sprintf("%s.(%s).%s", pkg, t, name)
	}
	return fmt.Sprintf("%s.%s", pkg, name)
}

// ensureStdlib carica tutti i pacchetti della standard library con sintassi.
func ensureStdlib(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedImports,
		Dir:  root,
		Env:  os.Environ(),
	}
	pkgs, err := packages.Load(cfg, "std")
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// dedupPackages rimuove duplicati per PkgPath mantenendo l'ordine di prima occorrenza.
func dedupPackages(in []*packages.Package) []*packages.Package {
	seen := make(map[string]struct{}, len(in))
	out := make([]*packages.Package, 0, len(in))
	for _, p := range in {
		if p == nil {
			continue
		}
		key := p.PkgPath
		if key == "" {
			// fallback a pointer string se manca PkgPath (raro)
			key = fmt.Sprintf("%p", p)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

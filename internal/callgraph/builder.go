// Package callgraph fornisce la costruzione del call graph CLDK-compatible.
package callgraph

import (
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Config configura la costruzione del call graph.
type Config struct {
	Algorithm     string   // cha|rta (default: rta)
	EmitPositions string   // detailed|minimal
	OnlyPkg       []string // filtra a questi package path (substring match)
}

// Build costruisce un call graph CLDK da un LoadResult con SSA.
func Build(result *loader.LoadResult, cfg Config) (*schema.CLDKCallGraph, error) {
	if result.SSAProgram == nil {
		return nil, fmt.Errorf("SSAProgram is nil, call LoadWithSSA with NeedSSA=true")
	}

	prog := result.SSAProgram
	ssaPkgs := result.SSAPackages

	// Costruisci call graph
	var cg *callgraph.Graph
	algo := strings.ToLower(cfg.Algorithm)
	if algo == "" {
		algo = "rta"
	}

	switch algo {
	case "rta":
		mainPkgs := ssautil.MainPackages(ssaPkgs)
		var roots []*ssa.Function
		for _, m := range mainPkgs {
			if fn := m.Func("main"); fn != nil {
				roots = append(roots, fn)
			}
			// Aggiungi anche init se presente
			if fn := m.Func("init"); fn != nil {
				roots = append(roots, fn)
			}
		}
		if len(roots) == 0 {
			// Fallback a CHA se non ci sono main packages
			cg = cha.CallGraph(prog)
			algo = "cha-fallback"
		} else {
			res := rta.Analyze(roots, true)
			cg = res.CallGraph
		}
	default: // "cha"
		cg = cha.CallGraph(prog)
		algo = "cha"
	}

	// Converti in formato CLDK
	out := &schema.CLDKCallGraph{
		Algorithm: algo,
		Nodes:     []schema.CLDKCGNode{},
		Edges:     []schema.CLDKCGEdge{},
	}

	nodeSet := make(map[string]*schema.CLDKCGNode)
	edgeSet := make(map[string]schema.CLDKCGEdge)
	fset := prog.Fset

	// Helper per filtrare per onlyPkg
	inAllowedPkgs := func(f *ssa.Function) bool {
		if f == nil || f.Pkg == nil || f.Pkg.Pkg == nil {
			return len(cfg.OnlyPkg) == 0
		}
		if len(cfg.OnlyPkg) == 0 {
			return true
		}
		pp := f.Pkg.Pkg.Path()
		for _, s := range cfg.OnlyPkg {
			if s != "" && strings.Contains(pp, s) {
				return true
			}
		}
		return false
	}

	// Itera su tutti i nodi e archi del grafo
	for _, n := range cg.Nodes {
		if n == nil || n.Func == nil {
			continue
		}

		for _, e := range n.Out {
			if e == nil || e.Caller == nil || e.Callee == nil {
				continue
			}

			src := e.Caller.Func
			dst := e.Callee.Func
			if src == nil || dst == nil {
				continue
			}

			// Filtra archi completamente esterni
			if len(cfg.OnlyPkg) > 0 && !inAllowedPkgs(src) && !inAllowedPkgs(dst) {
				continue
			}

			srcID := stableFuncID(src)
			dstID := stableFuncID(dst)
			if srcID == "" || dstID == "" {
				continue
			}

			// Aggiungi nodi
			if _, ok := nodeSet[srcID]; !ok {
				nodeSet[srcID] = buildNode(src, fset, result.Root, cfg)
			}
			if _, ok := nodeSet[dstID]; !ok {
				nodeSet[dstID] = buildNode(dst, fset, result.Root, cfg)
			}

			// Aggiungi arco
			edgeKey := srcID + "→" + dstID
			if _, ok := edgeSet[edgeKey]; !ok {
				edge := schema.CLDKCGEdge{
					Source: srcID,
					Target: dstID,
				}
				// Posizione del call site
				if cfg.EmitPositions != "minimal" && e.Site != nil {
					pos := fset.Position(e.Site.Pos())
					if pos.IsValid() {
						file := pos.Filename
						if rel, err := filepath.Rel(result.Root, file); err == nil {
							file = filepath.ToSlash(rel)
						}
						edge.CallSite = &schema.CLDKPosition{
							File:        file,
							StartLine:   pos.Line,
							StartColumn: pos.Column,
						}
					}
				}
				// Determina il tipo di chiamata
				if e.Site != nil {
					switch e.Site.(type) {
					case *ssa.Go:
						edge.Kind = "go"
					case *ssa.Defer:
						edge.Kind = "defer"
					default:
						edge.Kind = "call"
					}
				}
				edgeSet[edgeKey] = edge
			}
		}
	}

	// Converti set a slice ordinati per stabilità
	for _, node := range nodeSet {
		out.Nodes = append(out.Nodes, *node)
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		return out.Nodes[i].ID < out.Nodes[j].ID
	})

	for _, edge := range edgeSet {
		out.Edges = append(out.Edges, edge)
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].Source == out.Edges[j].Source {
			return out.Edges[i].Target < out.Edges[j].Target
		}
		return out.Edges[i].Source < out.Edges[j].Source
	})

	return out, nil
}

// buildNode costruisce un nodo CLDK da una funzione SSA.
func buildNode(f *ssa.Function, fset *token.FileSet, root string, cfg Config) *schema.CLDKCGNode {
	id := stableFuncID(f)

	node := &schema.CLDKCGNode{
		ID:            id,
		QualifiedName: id,
		Name:          f.Name(),
	}

	// Package
	if f.Pkg != nil && f.Pkg.Pkg != nil {
		node.Package = f.Pkg.Pkg.Path()
	}

	// Kind: function o method
	if f.Signature != nil && f.Signature.Recv() != nil {
		node.Kind = "method"
	} else {
		node.Kind = "function"
	}

	// Posizione
	if cfg.EmitPositions != "minimal" && fset != nil {
		pos := fset.Position(f.Pos())
		if pos.IsValid() {
			file := pos.Filename
			if rel, err := filepath.Rel(root, file); err == nil {
				file = filepath.ToSlash(rel)
			}
			node.Position = &schema.CLDKPosition{
				File:        file,
				StartLine:   pos.Line,
				StartColumn: pos.Column,
			}
		}
	}

	return node
}

// stableFuncID genera un ID stabile per una funzione SSA.
// Formato: pkgpath.Func o pkgpath.(*Type).Method
func stableFuncID(f *ssa.Function) string {
	if f == nil {
		return ""
	}

	// Builtins e funzioni senza package
	if f.Pkg == nil || f.Pkg.Pkg == nil {
		if f.Name() != "" {
			return f.Name()
		}
		return f.String()
	}

	pkg := f.Pkg.Pkg.Path()
	name := f.Name()

	// Receiver per metodi
	if f.Signature != nil && f.Signature.Recv() != nil {
		r := f.Signature.Recv()
		t := r.Type().String()
		// Normalizza il tipo receiver
		// es: *example.com/pkg.Type → (*Type)
		t = normalizeReceiverType(t, pkg)
		return fmt.Sprintf("%s.%s.%s", pkg, t, name)
	}

	return fmt.Sprintf("%s.%s", pkg, name)
}

// normalizeReceiverType normalizza il tipo receiver per l'ID.
func normalizeReceiverType(t, pkg string) string {
	// Rimuovi il package path se presente
	if strings.HasPrefix(t, "*") {
		inner := t[1:]
		if idx := strings.LastIndex(inner, "."); idx >= 0 {
			inner = inner[idx+1:]
		}
		return "(*" + inner + ")"
	}
	if idx := strings.LastIndex(t, "."); idx >= 0 {
		return t[idx+1:]
	}
	return t
}

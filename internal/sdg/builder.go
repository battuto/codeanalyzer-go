// Package sdg costruisce il System Dependence Graph (SDG) inter-procedurale.
// L'SDG aggiunge edge tra funzioni diverse, collegando call sites ai parametri
// formali e i return values ai risultati attuali nel caller.
// Raggruppato per caller-package per facilitare l'iterazione in CLDK Python.
// Richiede che PDG e call graph siano già stati costruiti.
package sdg

import (
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Config configura la costruzione dell'SDG.
type Config struct {
	// per ora vuoto, estendibile in futuro (es. summary edges on/off)
}

// Build costruisce l'SDG a partire dal PDG e dal call graph.
// Gli edge sono raggruppati per caller-package.
func Build(pdgResult *schema.CLDKPDG, cgResult *schema.CLDKCallGraph, cfg Config) (*schema.CLDKSDG, error) {
	sdg := &schema.CLDKSDG{
		Packages: make(map[string]*schema.CLDKPackageSDG),
	}

	if pdgResult == nil || cgResult == nil {
		return sdg, nil
	}

	// Indice: qualifiedName → CLDKFunctionPDG
	fnIndex := buildFunctionIndex(pdgResult)

	// Per ogni package nel PDG, cerca i nodi call con Target e genera edge
	for callerPkgPath, pkgPDG := range pdgResult.Packages {
		for callerQN, callerPDG := range pkgPDG.Functions {
			for _, node := range callerPDG.Nodes {
				if node.Kind != "call" || node.Target == "" {
					continue
				}

				calleeQN := node.Target
				calleePDG, found := fnIndex[calleeQN]
				if !found {
					continue // callee non nel PDG (es. stdlib, fuori scope)
				}

				// Assicurati che il package caller esista nella mappa SDG
				if _, exists := sdg.Packages[callerPkgPath]; !exists {
					sdg.Packages[callerPkgPath] = &schema.CLDKPackageSDG{
						InterEdges: []schema.SDGInterEdge{},
					}
				}

				// --- Edge: call (call-site → entry della callee) ---
				calleeEntryNode := findEntryNode(calleePDG)
				sdg.Packages[callerPkgPath].InterEdges = append(
					sdg.Packages[callerPkgPath].InterEdges,
					schema.SDGInterEdge{
						Kind:       "call",
						CallerFunc: callerQN,
						CalleeFunc: calleeQN,
						CallerNode: node.ID,
						CalleeNode: calleeEntryNode,
					},
				)

				// --- Edge: param-in ---
				paramInEdges := findDataEdgesTo(callerPDG, node.ID)
				for idx, dataEdge := range paramInEdges {
					sdg.Packages[callerPkgPath].InterEdges = append(
						sdg.Packages[callerPkgPath].InterEdges,
						schema.SDGInterEdge{
							Kind:       "param-in",
							CallerFunc: callerQN,
							CalleeFunc: calleeQN,
							CallerNode: dataEdge.From,
							CalleeNode: calleeEntryNode,
							ParamIndex: idx,
							VarName:    dataEdge.VarName,
						},
					)
				}

				// --- Edge: param-out ---
				returnNodes := findReturnNodes(calleePDG)
				paramOutEdges := findDataEdgesFrom(callerPDG, node.ID)
				for _, dataEdge := range paramOutEdges {
					for _, retNode := range returnNodes {
						sdg.Packages[callerPkgPath].InterEdges = append(
							sdg.Packages[callerPkgPath].InterEdges,
							schema.SDGInterEdge{
								Kind:       "param-out",
								CallerFunc: callerQN,
								CalleeFunc: calleeQN,
								CallerNode: dataEdge.To,
								CalleeNode: retNode,
								VarName:    dataEdge.VarName,
							},
						)
					}
				}
			}
		}
	}

	return sdg, nil
}

// buildFunctionIndex costruisce un indice rapido qualifiedName → FunctionPDG.
func buildFunctionIndex(pdg *schema.CLDKPDG) map[string]*schema.CLDKFunctionPDG {
	idx := make(map[string]*schema.CLDKFunctionPDG)
	for _, pkgPDG := range pdg.Packages {
		for qn, fnPDG := range pkgPDG.Functions {
			idx[qn] = fnPDG
		}
	}
	return idx
}

// findEntryNode trova il node ID del nodo entry (kind="entry", tipicamente id=0).
func findEntryNode(fn *schema.CLDKFunctionPDG) int {
	for _, n := range fn.Nodes {
		if n.Kind == "entry" {
			return n.ID
		}
	}
	return 0 // fallback
}

// findReturnNodes trova tutti i node ID dei nodi return.
func findReturnNodes(fn *schema.CLDKFunctionPDG) []int {
	var ids []int
	for _, n := range fn.Nodes {
		if n.Kind == "return" {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

// findDataEdgesTo trova tutti i data edges che puntano a un nodo specifico.
func findDataEdgesTo(fn *schema.CLDKFunctionPDG, toID int) []schema.PDGDataEdge {
	var edges []schema.PDGDataEdge
	for _, e := range fn.DataEdges {
		if e.To == toID {
			edges = append(edges, e)
		}
	}
	return edges
}

// findDataEdgesFrom trova tutti i data edges che partono da un nodo specifico.
func findDataEdgesFrom(fn *schema.CLDKFunctionPDG, fromID int) []schema.PDGDataEdge {
	var edges []schema.PDGDataEdge
	for _, e := range fn.DataEdges {
		if e.From == fromID {
			edges = append(edges, e)
		}
	}
	return edges
}

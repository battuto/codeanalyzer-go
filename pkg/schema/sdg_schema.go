// Package schema definisce i tipi CLDK per l'output dell'analyzer Go.
package schema

// ============================================================================
// SDG (System Dependence Graph) Schema
// ============================================================================
// L'SDG estende il PDG con edge inter-procedurali: call, param-in, param-out.
// I nodi e gli edge intra-procedurali restano nel PDG; l'SDG contiene solo
// gli edge che collegano funzioni diverse.
// Strutturato per caller-package per facilitare il consumo in CLDK Python.

// CLDKSDG contiene gli edge inter-procedurali raggruppati per caller package.
type CLDKSDG struct {
	Packages map[string]*CLDKPackageSDG `json:"packages"`
}

// CLDKPackageSDG raggruppa gli edge SDG dove il caller è in questo package.
type CLDKPackageSDG struct {
	InterEdges []SDGInterEdge `json:"inter_edges"`
}

// SDGInterEdge rappresenta un edge inter-procedurale nel SDG.
type SDGInterEdge struct {
	Kind       string `json:"kind"`                    // "call"|"param-in"|"param-out"
	CallerFunc string `json:"caller_func"`             // qualified name della funzione chiamante
	CalleeFunc string `json:"callee_func"`             // qualified name della funzione chiamata
	CallerNode int    `json:"caller_node"`             // node ID nel PDG del caller
	CalleeNode int    `json:"callee_node"`             // node ID nel PDG del callee
	ParamIndex int    `json:"param_index,omitempty"`   // indice del parametro (per param-in/out)
	VarName    string `json:"var,omitempty"`            // nome della variabile
}

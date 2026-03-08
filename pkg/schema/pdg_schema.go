// Package schema definisce i tipi CLDK per l'output dell'analyzer Go.
package schema

// ============================================================================
// PDG (Program Dependence Graph) Schema
// ============================================================================
// Il PDG cattura le dipendenze di dati e di controllo per ogni funzione.
// È costruito a partire dalla rappresentazione SSA.

// CLDKPDG rappresenta il Program Dependence Graph per tutto il progetto.
// Strutturato per package per facilitare il consumo in CLDK Python.
type CLDKPDG struct {
	Packages map[string]*CLDKPackagePDG `json:"packages"`
}

// CLDKPackagePDG raggruppa i PDG delle funzioni di un singolo package.
type CLDKPackagePDG struct {
	Functions map[string]*CLDKFunctionPDG `json:"functions"`
}

// CLDKFunctionPDG rappresenta il PDG di una singola funzione.
type CLDKFunctionPDG struct {
	QualifiedName string        `json:"qualified_name"`
	Package       string        `json:"package"`
	Nodes         []PDGNode     `json:"nodes"`
	DataEdges     []PDGDataEdge `json:"data_edges"`
	ControlEdges  []PDGCtrlEdge `json:"control_edges"`
}

// PDGNode rappresenta un nodo nel PDG (un'istruzione SSA).
type PDGNode struct {
	ID       int           `json:"id"`                 // indice sequenziale
	Kind     string        `json:"kind"`               // "entry"|"assign"|"call"|"branch"|"return"|"phi"|"store"|"field"|"other"
	Instr    string        `json:"instr"`              // rappresentazione stringa dell'istruzione SSA
	Position *CLDKPosition `json:"position,omitempty"` // posizione nel sorgente
	Target   string        `json:"target,omitempty"`   // per nodi call: qualified name del target
}

// PDGDataEdge rappresenta una data dependency: il nodo To usa un valore definito dal nodo From.
type PDGDataEdge struct {
	From    int    `json:"from"`          // node ID del definer
	To      int    `json:"to"`            // node ID dell'utente
	VarName string `json:"var,omitempty"` // nome della variabile (se disponibile)
}

// PDGCtrlEdge rappresenta una control dependency: il nodo To è eseguito
// solo quando la condizione nel nodo From prende il ramo indicato.
type PDGCtrlEdge struct {
	From      int    `json:"from"`      // node ID della condizione
	To        int    `json:"to"`        // node ID dello statement controllato
	Condition string `json:"condition"` // "true"|"false" (quale ramo della condizione)
}

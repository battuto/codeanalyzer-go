// Package schema definisce i tipi CLDK per l'output dell'analyzer Go.
package schema

// ============================================================================
// Schema Compatto per LLM
// ============================================================================
// Questo schema riduce la dimensione dell'output del ~70% mantenendo
// la comprensibilità semantica per gli LLM.

// CompactAnalysis è la struttura root dell'output compatto per LLM.
type CompactAnalysis struct {
	Meta *CompactMeta           `json:"m"`
	Pkgs map[string]*CompactPkg `json:"p,omitempty"`
	CG   *CompactCallGraph      `json:"cg,omitempty"`
	PDG  *CompactPDG            `json:"pdg"` // Program Dependence Graph (compatto)
	SDG  *CompactSDG            `json:"sdg"` // System Dependence Graph (compatto)
	Iss  []CompactIssue         `json:"iss"` // issues/warnings
}

// CompactIssue rappresenta un problema rilevato durante l'analisi.
type CompactIssue struct {
	Sev string `json:"s"`           // severity: error|warning|info
	Msg string `json:"m"`           // message
	Loc string `json:"l,omitempty"` // location (file:line)
}

// CompactMeta contiene metadata minimali.
type CompactMeta struct {
	Ver  string `json:"v"` // analyzer version
	Lang string `json:"l"` // language
	Lvl  string `json:"a"` // analysis_level
	Dur  int64  `json:"d"` // duration_ms
}

// ============================================================================
// Package Structure
// ============================================================================

// CompactPkg rappresenta un package Go in formato compatto.
type CompactPkg struct {
	Name   string                  `json:"n"`            // package name
	Doc    string                  `json:"d,omitempty"`  // package documentation
	Files  []string                `json:"f,omitempty"`  // relative file paths
	Imps   []string                `json:"i,omitempty"`  // import paths (leggibili)
	Types  map[string]*CompactType `json:"t,omitempty"`  // type declarations
	Funcs  map[string]*CompactFunc `json:"fn,omitempty"` // functions/methods
	Vars   map[string]string       `json:"v,omitempty"`  // name → type
	Consts map[string]string       `json:"c,omitempty"`  // name → value

	// Package-level metadata for malware/security analysis
	Init   bool     `json:"init,omitempty"` // has init() function
	Gor    bool     `json:"gor,omitempty"`  // starts goroutines (go statements)
	Env    bool     `json:"env,omitempty"`  // reads environment variables
	BT     []string `json:"bt,omitempty"`   // build tags/constraints
	UsedBy []string `json:"ub,omitempty"`   // reverse imports: who imports this package
	Main   bool     `json:"main,omitempty"` // reachable from main()/init() flow

	// Extended security analysis
	SL  []CompactStringLit     `json:"sl,omitempty"`  // string literals (classified)
	SC  []CompactSCVector      `json:"sc,omitempty"`  // supply chain vectors
	Obf *CompactObfMetrics     `json:"obf,omitempty"` // obfuscation metrics
}

// ============================================================================
// Type Declarations
// ============================================================================

// CompactType rappresenta una dichiarazione di tipo in formato compatto.
type CompactType struct {
	Kind    string            `json:"k"`            // struct|interface|alias|named
	Fields  map[string]string `json:"f,omitempty"`  // fieldName → type
	Methods []string          `json:"m,omitempty"`  // method signatures
	IM      []string          `json:"im,omitempty"` // interface method signatures
	Embeds  []string          `json:"e,omitempty"`  // embedded types
	Doc     string            `json:"d,omitempty"`  // documentation (solo export)
}

// ============================================================================
// Functions/Methods
// ============================================================================

// CompactFunc rappresenta una funzione o metodo in formato compatto.
type CompactFunc struct {
	Sig  string   `json:"s"`            // signature completa
	Kind string   `json:"k,omitempty"`  // "m" per method, omesso per function
	Recv string   `json:"r,omitempty"`  // receiver type (solo per method)
	Doc  string   `json:"d,omitempty"`  // documentation (solo export)
	Ex   []string `json:"ex,omitempty"` // call examples
}

// ============================================================================
// Call Graph
// ============================================================================

// CompactCallGraph rappresenta il call graph in formato compatto.
type CompactCallGraph struct {
	Algo  string      `json:"a"` // algorithm (cha|rta)
	Edges [][2]string `json:"e"` // [[source, target], ...]
}

// ============================================================================
// Security Analysis Compact Types
// ============================================================================

// CompactStringLit rappresenta una stringa classificata in formato compatto.
// Solo stringhe con categoria != "other" vengono incluse.
type CompactStringLit struct {
	V string  `json:"v"`           // value (truncated)
	C string  `json:"c"`           // category
	E float64 `json:"e,omitempty"` // entropy
	S string  `json:"s,omitempty"` // scope (function qualified name)
}

// CompactSCVector rappresenta un vettore supply chain in formato compatto.
type CompactSCVector struct {
	K string `json:"k"`           // kind
	D string `json:"d"`           // detail
	S string `json:"s"`           // severity
	F string `json:"f,omitempty"` // file
}

// CompactObfMetrics rappresenta le metriche di offuscamento in formato compatto.
type CompactObfMetrics struct {
	FnLen  float64 `json:"fl"`            // avg func name length
	VrLen  float64 `json:"vl"`            // avg var name length
	Short  float64 `json:"sr"`            // short names ratio %
	Doc    float64 `json:"dc"`            // doc coverage %
	Xor    int     `json:"xor,omitempty"` // xor operations count
	Garble bool    `json:"gb,omitempty"`  // garble patterns detected
}

// ============================================================================
// PDG (Program Dependence Graph) Compact
// ============================================================================

// CompactPDG rappresenta il PDG in formato compatto per LLM, raggruppato per package.
type CompactPDG struct {
	Pkgs map[string]*CompactPkgPDG `json:"p"` // package_path → package PDG
}

// CompactPkgPDG raggruppa i PDG delle funzioni di un singolo package in formato compatto.
type CompactPkgPDG struct {
	Fns map[string]*CompactFnPDG `json:"f"` // func_name → function PDG
}

// CompactFnPDG rappresenta il PDG di una singola funzione in formato compatto.
type CompactFnPDG struct {
	Nodes []string    `json:"n"`            // ["id:kind:instr", ...]
	Data  [][3]string `json:"d,omitempty"`  // [[from_id, to_id, var], ...]
	Ctrl  [][3]string `json:"c,omitempty"`  // [[from_id, to_id, cond], ...]
}

// ============================================================================
// SDG (System Dependence Graph) Compact
// ============================================================================

// CompactSDG rappresenta l'SDG in formato compatto per LLM, raggruppato per caller-package.
type CompactSDG struct {
	Pkgs map[string]*CompactPkgSDG `json:"p"` // caller_package → package SDG
}

// CompactPkgSDG raggruppa gli edge SDG di un singolo caller-package.
type CompactPkgSDG struct {
	// [[kind, caller_func, callee_func, caller_node, callee_node, param_idx, var], ...]
	Edges [][7]string `json:"e"`
}

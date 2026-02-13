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
	PDG  interface{}            `json:"pdg"` // placeholder per future estensioni
	SDG  interface{}            `json:"sdg"` // placeholder per future estensioni
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

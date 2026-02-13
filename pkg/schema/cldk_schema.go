// Package schema definisce i tipi CLDK per l'output dell'analyzer Go.
package schema

// ============================================================================
// Struttura Principale
// ============================================================================

// CLDKAnalysis è la struttura root dell'output dell'analyzer.
type CLDKAnalysis struct {
	Metadata    Metadata         `json:"metadata"`
	SymbolTable *CLDKSymbolTable `json:"symbol_table,omitempty"`
	CallGraph   *CLDKCallGraph   `json:"call_graph,omitempty"`
	PDG         interface{}      `json:"pdg"`    // placeholder null per future estensioni
	SDG         interface{}      `json:"sdg"`    // placeholder null per future estensioni
	Issues      []Issue          `json:"issues"`
}

// Metadata contiene informazioni sull'analisi eseguita.
type Metadata struct {
	Analyzer           string `json:"analyzer"`
	Version            string `json:"version"`
	Language           string `json:"language"`
	AnalysisLevel      string `json:"analysis_level"`
	Timestamp          string `json:"timestamp"`
	ProjectPath        string `json:"project_path"`
	GoVersion          string `json:"go_version"`
	AnalysisDurationMs int64  `json:"analysis_duration_ms"`
}

// Issue rappresenta un problema rilevato durante l'analisi.
type Issue struct {
	Severity string        `json:"severity"` // error|warning|info
	Code     string        `json:"code"`
	Message  string        `json:"message"`
	Position *CLDKPosition `json:"position,omitempty"`
}

// ============================================================================
// Symbol Table
// ============================================================================

// CLDKSymbolTable rappresenta la symbol table con packages come mappa.
type CLDKSymbolTable struct {
	Packages map[string]*CLDKPackage `json:"packages"`
}

// CLDKPackage rappresenta un package Go.
type CLDKPackage struct {
	Path                 string                   `json:"path"`
	Name                 string                   `json:"name"`
	Documentation        string                   `json:"documentation,omitempty"`
	Files                []string                 `json:"files"`
	Imports              []CLDKImport             `json:"imports"`
	TypeDeclarations     map[string]*CLDKType     `json:"type_declarations"`
	CallableDeclarations map[string]*CLDKCallable `json:"callable_declarations"`
	Variables            map[string]*CLDKVariable `json:"variables"`
	Constants            map[string]*CLDKConstant `json:"constants"`
}

// CLDKImport rappresenta un import.
type CLDKImport struct {
	Path     string        `json:"path"`
	Alias    string        `json:"alias,omitempty"`
	Position *CLDKPosition `json:"position,omitempty"`
}

// ============================================================================
// Type Declarations
// ============================================================================

// CLDKType rappresenta una dichiarazione di tipo (struct, interface, alias, etc.).
type CLDKType struct {
	QualifiedName    string                 `json:"qualified_name"`
	Name             string                 `json:"name"`
	Kind             string                 `json:"kind"` // struct|interface|alias|named
	Position         *CLDKPosition          `json:"position"`
	Documentation    string                 `json:"documentation,omitempty"`
	Fields           []CLDKField            `json:"fields,omitempty"`
	Methods          map[string]*CLDKMethod `json:"methods,omitempty"`
	InterfaceMethods []CLDKInterfaceMethod   `json:"interface_methods,omitempty"`
	EmbeddedTypes    []string               `json:"embedded_types,omitempty"`
	Implements       []string               `json:"implements,omitempty"`
	UnderlyingType   string                 `json:"underlying_type,omitempty"`
	TypeParameters   []CLDKTypeParam        `json:"type_parameters,omitempty"`
}

// CLDKInterfaceMethod rappresenta un metodo dichiarato in un'interfaccia.
type CLDKInterfaceMethod struct {
	Name          string          `json:"name"`
	Signature     string          `json:"signature"`
	Parameters    []CLDKParameter `json:"parameters"`
	Results       []CLDKParameter `json:"results"`
	Documentation string          `json:"documentation,omitempty"`
}

// CLDKField rappresenta un campo di una struct.
type CLDKField struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Tag        string        `json:"tag,omitempty"`
	Position   *CLDKPosition `json:"position,omitempty"`
	Exported   bool          `json:"exported"`
	Embedded   bool          `json:"embedded"`
}

// CLDKMethod rappresenta un metodo di un tipo.
type CLDKMethod struct {
	QualifiedName string            `json:"qualified_name"`
	Name          string            `json:"name"`
	Signature     string            `json:"signature"`
	ReceiverType  string            `json:"receiver_type"`
	ReceiverPtr   bool              `json:"receiver_ptr"`
	Parameters    []CLDKParameter   `json:"parameters"`
	Results       []CLDKParameter   `json:"results"`
	Position      *CLDKPosition     `json:"position"`
	EndPosition   *CLDKPosition     `json:"end_position,omitempty"`
	Documentation string            `json:"documentation,omitempty"`
	Body          *CLDKFunctionBody `json:"body,omitempty"`
}

// CLDKTypeParam rappresenta un parametro di tipo generico.
type CLDKTypeParam struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
}

// ============================================================================
// Callable Declarations
// ============================================================================

// CLDKCallable rappresenta una funzione o metodo.
type CLDKCallable struct {
	QualifiedName  string            `json:"qualified_name"`
	Name           string            `json:"name"`
	Signature      string            `json:"signature"`
	Kind           string            `json:"kind"` // function|method
	ReceiverType   string            `json:"receiver_type,omitempty"`
	ReceiverPtr    bool              `json:"receiver_ptr,omitempty"`
	Parameters     []CLDKParameter   `json:"parameters"`
	Results        []CLDKParameter   `json:"results"`
	Position       *CLDKPosition     `json:"position"`
	EndPosition    *CLDKPosition     `json:"end_position,omitempty"`
	Documentation  string            `json:"documentation,omitempty"`
	Exported       bool              `json:"exported"`
	TypeParameters []CLDKTypeParam   `json:"type_parameters,omitempty"`
	Body           *CLDKFunctionBody `json:"body,omitempty"`
	CallExamples   []string          `json:"call_examples,omitempty"`
}

// CLDKParameter rappresenta un parametro o valore di ritorno.
type CLDKParameter struct {
	Name     string `json:"name,omitempty"`
	Type     string `json:"type"`
	Variadic bool   `json:"variadic,omitempty"`
}

// CLDKFunctionBody contiene informazioni sul corpo della funzione.
type CLDKFunctionBody struct {
	StartLine   int            `json:"start_line"`
	EndLine     int            `json:"end_line"`
	LineCount   int            `json:"line_count"`
	Complexity  int            `json:"complexity,omitempty"`
	CallSites   []CLDKCallSite `json:"call_sites,omitempty"`
	LocalVars   []string       `json:"local_vars,omitempty"`
}

// CLDKCallSite rappresenta una chiamata a funzione nel corpo.
type CLDKCallSite struct {
	Target   string        `json:"target"`
	Position *CLDKPosition `json:"position"`
	Kind     string        `json:"kind"` // call|defer|go
}

// ============================================================================
// Variables and Constants
// ============================================================================

// CLDKVariable rappresenta una variabile package-level.
type CLDKVariable struct {
	QualifiedName string        `json:"qualified_name"`
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	Position      *CLDKPosition `json:"position"`
	Exported      bool          `json:"exported"`
	Documentation string        `json:"documentation,omitempty"`
}

// CLDKConstant rappresenta una costante package-level.
type CLDKConstant struct {
	QualifiedName string        `json:"qualified_name"`
	Name          string        `json:"name"`
	Type          string        `json:"type,omitempty"`
	Value         string        `json:"value,omitempty"`
	Position      *CLDKPosition `json:"position"`
	Exported      bool          `json:"exported"`
	Documentation string        `json:"documentation,omitempty"`
}

// ============================================================================
// Position
// ============================================================================

// CLDKPosition rappresenta una posizione nel codice sorgente.
type CLDKPosition struct {
	File        string `json:"file"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line,omitempty"`
	StartColumn int    `json:"start_column"`
	EndColumn   int    `json:"end_column,omitempty"`
}

// ============================================================================
// Call Graph
// ============================================================================

// CLDKCallGraph rappresenta il call graph.
type CLDKCallGraph struct {
	Algorithm string       `json:"algorithm"`
	Nodes     []CLDKCGNode `json:"nodes"`
	Edges     []CLDKCGEdge `json:"edges"`
}

// CLDKCGNode rappresenta un nodo del call graph.
type CLDKCGNode struct {
	ID            string        `json:"id"`
	QualifiedName string        `json:"qualified_name"`
	Package       string        `json:"package"`
	Name          string        `json:"name"`
	Kind          string        `json:"kind"` // function|method
	Position      *CLDKPosition `json:"position,omitempty"`
}

// CLDKCGEdge rappresenta un arco del call graph.
type CLDKCGEdge struct {
	Source   string        `json:"source"`
	Target   string        `json:"target"`
	CallSite *CLDKPosition `json:"call_site,omitempty"`
	Kind     string        `json:"kind,omitempty"` // call|defer|go
}

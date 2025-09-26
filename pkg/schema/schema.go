package schema

type Position struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type File struct {
	Path string `json:"path"`
}

type Import struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

type TypeDecl struct {
	Name string   `json:"name"`
	Kind string   `json:"kind,omitempty"`
	Pos  Position `json:"pos"`
}

type Function struct {
	Name      string   `json:"name"`
	Receiver  string   `json:"receiver,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Pos       Position `json:"pos"`
}

type Package struct {
	Path      string     `json:"path"`
	Files     []File     `json:"files"`
	Imports   []Import   `json:"imports"`
	Types     []TypeDecl `json:"types"`
	Functions []Function `json:"functions"`
}

type SymbolTable struct {
	Language string    `json:"language"`
	Packages []Package `json:"packages"`
}

// Call graph (placeholder for now)
type CGNode struct {
	ID  string   `json:"id"`
	Pos Position `json:"pos,omitempty"`
}

type CGEdge struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

type CallGraph struct {
	Language string   `json:"language"`
	Nodes    []CGNode `json:"nodes"`
	Edges    []CGEdge `json:"edges"`
}

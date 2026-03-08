// Package schema definisce i tipi CLDK per l'output dell'analyzer Go.
package schema

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ToCompact converte CLDKAnalysis in CompactAnalysis per output LLM.
func ToCompact(full *CLDKAnalysis) *CompactAnalysis {
	compact := &CompactAnalysis{
		Meta: &CompactMeta{
			Ver:  full.Metadata.Version,
			Lang: full.Metadata.Language,
			Lvl:  full.Metadata.AnalysisLevel,
			Dur:  full.Metadata.AnalysisDurationMs,
		},
		PDG: nil,
		SDG: nil,
		Iss: convertIssues(full.Issues),
	}

	// Converti symbol table
	if full.SymbolTable != nil && len(full.SymbolTable.Packages) > 0 {
		compact.Pkgs = make(map[string]*CompactPkg)
		for pkgPath, pkg := range full.SymbolTable.Packages {
			compact.Pkgs[pkgPath] = convertPackage(pkg)
		}
	}

	// Converti call graph
	if full.CallGraph != nil {
		compact.CG = convertCallGraph(full.CallGraph)
	}

	// Converti PDG
	if full.PDG != nil {
		compact.PDG = convertPDG(full.PDG)
	}

	// Converti SDG
	if full.SDG != nil {
		compact.SDG = convertSDG(full.SDG)
	}

	return compact
}

// convertIssues converte gli Issue in CompactIssue.
func convertIssues(issues []Issue) []CompactIssue {
	if len(issues) == 0 {
		return []CompactIssue{}
	}
	result := make([]CompactIssue, 0, len(issues))
	for _, iss := range issues {
		ci := CompactIssue{
			Sev: iss.Severity,
			Msg: iss.Message,
		}
		if iss.Position != nil {
			ci.Loc = iss.Position.File
		}
		result = append(result, ci)
	}
	return result
}

// convertPackage converte un CLDKPackage in CompactPkg.
func convertPackage(pkg *CLDKPackage) *CompactPkg {
	cp := &CompactPkg{
		Name: pkg.Name,
	}

	// Package documentation
	if pkg.Documentation != "" {
		cp.Doc = truncateDoc(pkg.Documentation)
	}

	// Files - estrai solo il basename
	if len(pkg.Files) > 0 {
		cp.Files = make([]string, len(pkg.Files))
		for i, f := range pkg.Files {
			cp.Files[i] = filepath.Base(f)
		}
	}

	// Imports - solo i path
	if len(pkg.Imports) > 0 {
		cp.Imps = make([]string, 0, len(pkg.Imports))
		for _, imp := range pkg.Imports {
			cp.Imps = append(cp.Imps, imp.Path)
		}
	}

	// Type declarations
	if len(pkg.TypeDeclarations) > 0 {
		cp.Types = make(map[string]*CompactType)
		for _, td := range pkg.TypeDeclarations {
			ct := &CompactType{
				Kind: td.Kind,
			}

			// Fields per struct
			if len(td.Fields) > 0 {
				ct.Fields = make(map[string]string)
				for _, f := range td.Fields {
					ct.Fields[f.Name] = f.Type
				}
			}

			// Methods - solo signature
			if len(td.Methods) > 0 {
				ct.Methods = make([]string, 0, len(td.Methods))
				for _, m := range td.Methods {
					ct.Methods = append(ct.Methods, m.Signature)
				}
			}

			// Embedded types
			if len(td.EmbeddedTypes) > 0 {
				ct.Embeds = td.EmbeddedTypes
			}

			// Documentation solo per tipi esportati
			if isExported(td.Name) && td.Documentation != "" {
				ct.Doc = truncateDoc(td.Documentation)
			}

			// Interface methods
			if len(td.InterfaceMethods) > 0 {
				ct.IM = make([]string, 0, len(td.InterfaceMethods))
				for _, im := range td.InterfaceMethods {
					ct.IM = append(ct.IM, im.Signature)
				}
			}

			cp.Types[td.Name] = ct
		}
	}

	// Callable declarations (functions/methods)
	if len(pkg.CallableDeclarations) > 0 {
		cp.Funcs = make(map[string]*CompactFunc)
		for _, cd := range pkg.CallableDeclarations {
			cf := &CompactFunc{
				Sig: cd.Signature,
			}

			// Kind solo per method
			if cd.Kind == "method" {
				cf.Kind = "m"
				cf.Recv = cd.ReceiverType
			}

			// Documentation solo per funzioni esportate
			if cd.Exported && cd.Documentation != "" {
				cf.Doc = truncateDoc(cd.Documentation)
			}

			// Call examples
			if len(cd.CallExamples) > 0 {
				cf.Ex = cd.CallExamples
			}

			cp.Funcs[cd.Name] = cf
		}
	}

	// Variables - name → type
	if len(pkg.Variables) > 0 {
		cp.Vars = make(map[string]string)
		for _, v := range pkg.Variables {
			if v.Exported { // solo variabili esportate
				cp.Vars[v.Name] = v.Type
			}
		}
		if len(cp.Vars) == 0 {
			cp.Vars = nil
		}
	}

	// Constants - name → value
	if len(pkg.Constants) > 0 {
		cp.Consts = make(map[string]string)
		for _, c := range pkg.Constants {
			if c.Exported { // solo costanti esportate
				if c.Value != "" {
					cp.Consts[c.Name] = c.Value
				} else {
					cp.Consts[c.Name] = c.Type
				}
			}
		}
		if len(cp.Consts) == 0 {
			cp.Consts = nil
		}
	}

	return cp
}

// convertCallGraph converte CLDKCallGraph in CompactCallGraph.
func convertCallGraph(cg *CLDKCallGraph) *CompactCallGraph {
	ccg := &CompactCallGraph{
		Algo:  cg.Algorithm,
		Edges: make([][2]string, 0, len(cg.Edges)),
	}

	for _, edge := range cg.Edges {
		ccg.Edges = append(ccg.Edges, [2]string{edge.Source, edge.Target})
	}

	return ccg
}

// convertPDG converte CLDKPDG in CompactPDG (per-package).
func convertPDG(p *CLDKPDG) *CompactPDG {
	if p == nil || len(p.Packages) == 0 {
		return nil
	}

	cp := &CompactPDG{
		Pkgs: make(map[string]*CompactPkgPDG, len(p.Packages)),
	}

	for pkgPath, pkgPDG := range p.Packages {
		cpkg := &CompactPkgPDG{
			Fns: make(map[string]*CompactFnPDG, len(pkgPDG.Functions)),
		}

		for qn, fn := range pkgPDG.Functions {
			cfn := &CompactFnPDG{}

			// Nodi: "id:kind:instr"
			cfn.Nodes = make([]string, len(fn.Nodes))
			for i, n := range fn.Nodes {
				instr := n.Instr
				if len(instr) > 80 {
					instr = instr[:77] + "..."
				}
				cfn.Nodes[i] = fmt.Sprintf("%d:%s:%s", n.ID, n.Kind, instr)
			}

			// Data edges: [from, to, var]
			if len(fn.DataEdges) > 0 {
				cfn.Data = make([][3]string, len(fn.DataEdges))
				for i, e := range fn.DataEdges {
					cfn.Data[i] = [3]string{
						fmt.Sprintf("%d", e.From),
						fmt.Sprintf("%d", e.To),
						e.VarName,
					}
				}
			}

			// Control edges: [from, to, condition]
			if len(fn.ControlEdges) > 0 {
				cfn.Ctrl = make([][3]string, len(fn.ControlEdges))
				for i, e := range fn.ControlEdges {
					cfn.Ctrl[i] = [3]string{
						fmt.Sprintf("%d", e.From),
						fmt.Sprintf("%d", e.To),
						e.Condition,
					}
				}
			}

			cpkg.Fns[qn] = cfn
		}

		cp.Pkgs[pkgPath] = cpkg
	}

	return cp
}

// convertSDG converte CLDKSDG in CompactSDG (per-caller-package).
func convertSDG(s *CLDKSDG) *CompactSDG {
	if s == nil || len(s.Packages) == 0 {
		return nil
	}

	cs := &CompactSDG{
		Pkgs: make(map[string]*CompactPkgSDG, len(s.Packages)),
	}

	for pkgPath, pkgSDG := range s.Packages {
		if len(pkgSDG.InterEdges) == 0 {
			continue
		}

		cpkg := &CompactPkgSDG{
			Edges: make([][7]string, len(pkgSDG.InterEdges)),
		}

		for i, e := range pkgSDG.InterEdges {
			cpkg.Edges[i] = [7]string{
				e.Kind,
				e.CallerFunc,
				e.CalleeFunc,
				fmt.Sprintf("%d", e.CallerNode),
				fmt.Sprintf("%d", e.CalleeNode),
				fmt.Sprintf("%d", e.ParamIndex),
				e.VarName,
			}
		}

		cs.Pkgs[pkgPath] = cpkg
	}

	return cs
}

// isExported verifica se un nome è esportato (inizia con maiuscola).
func isExported(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	return first >= 'A' && first <= 'Z'
}

// truncateDoc tronca la documentazione eccessivamente lunga.
func truncateDoc(doc string) string {
	// Rimuovi newline e spazi extra
	doc = strings.TrimSpace(doc)
	doc = strings.ReplaceAll(doc, "\n", " ")
	doc = strings.ReplaceAll(doc, "\r", "")

	// Limita a 200 caratteri
	if len(doc) > 200 {
		return doc[:197] + "..."
	}
	return doc
}

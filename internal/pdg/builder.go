// Package pdg fornisce la costruzione del Program Dependence Graph (PDG) CLDK-compatible.
// Il PDG cattura le dipendenze di dati (use-def) e di controllo (post-dominator)
// per ogni funzione, a partire dalla rappresentazione SSA.
package pdg

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"

	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Config configura la costruzione del PDG.
type Config struct {
	EmitPositions string   // detailed|minimal
	OnlyPkg       []string // filtra per sottostringa nel path
}

// Build costruisce il PDG per tutte le funzioni dei pacchetti caricati.
func Build(result *loader.LoadResult, cfg Config) (*schema.CLDKPDG, error) {
	if result.SSAProgram == nil {
		return nil, fmt.Errorf("SSAProgram is nil, call LoadWithSSA with NeedSSA=true")
	}

	pdg := &schema.CLDKPDG{
		Packages: make(map[string]*schema.CLDKPackagePDG),
	}

	for _, ssaPkg := range result.SSAPackages {
		if ssaPkg == nil || ssaPkg.Pkg == nil {
			continue
		}

		pkgPath := ssaPkg.Pkg.Path()

		// Filtra per onlyPkg
		if len(cfg.OnlyPkg) > 0 {
			keep := false
			for _, s := range cfg.OnlyPkg {
				if s != "" && strings.Contains(pkgPath, s) {
					keep = true
					break
				}
			}
			if !keep {
				continue
			}
		}

		// Processa ogni funzione del pacchetto
		for _, member := range ssaPkg.Members {
			fn, ok := member.(*ssa.Function)
			if !ok {
				continue
			}
			buildFunctionPDG(fn, pkgPath, result, cfg, pdg)

			// Processa anche i metodi dei tipi
			if tp, ok := member.(*ssa.Type); ok {
				mset := result.SSAProgram.MethodSets.MethodSet(tp.Type())
				for i := 0; i < mset.Len(); i++ {
					sel := mset.At(i)
					mfn := result.SSAProgram.MethodValue(sel)
					if mfn != nil {
						buildFunctionPDG(mfn, pkgPath, result, cfg, pdg)
					}
				}
			}
		}
	}

	return pdg, nil
}

// buildFunctionPDG costruisce il PDG per una singola funzione SSA.
func buildFunctionPDG(fn *ssa.Function, pkgPath string, result *loader.LoadResult, cfg Config, pdg *schema.CLDKPDG) {
	if fn == nil || len(fn.Blocks) == 0 {
		return // funzioni senza body (es. built-in, abstract)
	}

	fid := stableFuncID(fn)
	if fid == "" {
		return
	}

	// Assicurati che il package esista nella mappa
	pkgPDG, exists := pdg.Packages[pkgPath]
	if !exists {
		pkgPDG = &schema.CLDKPackagePDG{
			Functions: make(map[string]*schema.CLDKFunctionPDG),
		}
		pdg.Packages[pkgPath] = pkgPDG
	}

	// Evita duplicati
	if _, exists := pkgPDG.Functions[fid]; exists {
		return
	}

	fnPDG := &schema.CLDKFunctionPDG{
		QualifiedName: fid,
		Package:       pkgPath,
		Nodes:         []schema.PDGNode{},
		DataEdges:     []schema.PDGDataEdge{},
		ControlEdges:  []schema.PDGCtrlEdge{},
	}

	// ---- Fase 1: Enumera nodi da istruzioni SSA ----
	instrIndex := make(map[ssa.Instruction]int) // istruzione → node ID
	valueIndex := make(map[ssa.Value]int)       // valore SSA → node ID del suo definer
	nodeID := 0

	// Nodo entry speciale
	fnPDG.Nodes = append(fnPDG.Nodes, schema.PDGNode{
		ID:   nodeID,
		Kind: "entry",
		Instr: fmt.Sprintf("entry: %s", fn.Name()),
	})
	entryNodeID := nodeID
	nodeID++

	// Registra i parametri come valori definiti dall'entry node
	for _, p := range fn.Params {
		valueIndex[p] = entryNodeID
	}
	// I free variables (closures) sono anche definiti all'entry
	for _, fv := range fn.FreeVars {
		valueIndex[fv] = entryNodeID
	}

	// Itera su tutti i blocchi e istruzioni
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			node := schema.PDGNode{
				ID:    nodeID,
				Kind:  instrKind(instr),
				Instr: instrString(instr),
			}

			// Posizione
			if cfg.EmitPositions != "minimal" && result.Fset != nil {
				pos := result.Fset.Position(instr.Pos())
				if pos.IsValid() {
					file := pos.Filename
					if rel, err := filepath.Rel(result.Root, file); err == nil {
						file = filepath.ToSlash(rel)
					}
					node.Position = &schema.CLDKPosition{
						File:        file,
						StartLine:   pos.Line,
						StartColumn: pos.Column,
					}
				}
			}

			fnPDG.Nodes = append(fnPDG.Nodes, node)
			instrIndex[instr] = nodeID

			// Se l'istruzione definisce un valore, registralo
			if v, ok := instr.(ssa.Value); ok {
				valueIndex[v] = nodeID
			}

			nodeID++
		}
	}

	// ---- Fase 2: Data Dependency Edges (use-def chains) ----
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			toID, ok := instrIndex[instr]
			if !ok {
				continue
			}

			// Per ogni operando dell'istruzione, creo un data edge da dove è definito
			operands := instr.Operands(nil)
			for _, opPtr := range operands {
				if opPtr == nil || *opPtr == nil {
					continue
				}
				op := *opPtr

				// Cerca il nodo che definisce questo operando
				if fromID, defined := valueIndex[op]; defined {
					if fromID != toID { // evita self-loop
						varName := ""
						if named, ok := op.(interface{ Name() string }); ok {
							varName = named.Name()
						}
						fnPDG.DataEdges = append(fnPDG.DataEdges, schema.PDGDataEdge{
							From:    fromID,
							To:      toID,
							VarName: varName,
						})
					}
				}
			}
		}
	}

	// ---- Fase 3: Control Dependency Edges ----
	// Usando l'approccio basato sulla dominance frontier inversa:
	// Se un blocco B termina con un If, i blocchi nei due rami
	// che NON sono post-dominati da B sono control-dependent su B.
	//
	// Approccio semplificato per l'SSA: per ogni *ssa.If, i blocchi
	// successori diretti (Succs[0]=true, Succs[1]=false) e i loro
	// blocchi esclusivi sono control-dependent sulla condizione.
	postDom := computePostDominators(fn)

	for _, block := range fn.Blocks {
		// Cerca terminatori If
		if len(block.Instrs) == 0 {
			continue
		}
		lastInstr := block.Instrs[len(block.Instrs)-1]
		ifInstr, ok := lastInstr.(*ssa.If)
		if !ok {
			continue
		}

		branchNodeID, hasBranch := instrIndex[ifInstr]
		if !hasBranch {
			continue
		}

		// Per ogni successore, i blocchi che non sono post-dominati
		// dall'altro successore sono control-dependent sull'If.
		succs := block.Succs
		if len(succs) != 2 {
			continue
		}

		for si, succ := range succs {
			condition := "true"
			if si == 1 {
				condition = "false"
			}

			// Tutte le istruzioni nel successore diretto sono control-dependent
			addControlEdgesForBlock(succ, branchNodeID, condition, instrIndex, postDom, block, fnPDG)
		}
	}

	// Dedup data edges
	fnPDG.DataEdges = dedupDataEdges(fnPDG.DataEdges)

	// Ordina edges per stabilità
	sort.Slice(fnPDG.DataEdges, func(i, j int) bool {
		if fnPDG.DataEdges[i].From == fnPDG.DataEdges[j].From {
			return fnPDG.DataEdges[i].To < fnPDG.DataEdges[j].To
		}
		return fnPDG.DataEdges[i].From < fnPDG.DataEdges[j].From
	})
	sort.Slice(fnPDG.ControlEdges, func(i, j int) bool {
		if fnPDG.ControlEdges[i].From == fnPDG.ControlEdges[j].From {
			return fnPDG.ControlEdges[i].To < fnPDG.ControlEdges[j].To
		}
		return fnPDG.ControlEdges[i].From < fnPDG.ControlEdges[j].From
	})

	pdg.Packages[pkgPath].Functions[fid] = fnPDG
}

// addControlEdgesForBlock aggiunge control edges per le istruzioni di un blocco
// che è control-dependent su un branch.
func addControlEdgesForBlock(block *ssa.BasicBlock, branchNodeID int, condition string,
	instrIndex map[ssa.Instruction]int, postDom map[*ssa.BasicBlock]*ssa.BasicBlock,
	branchBlock *ssa.BasicBlock, fnPDG *schema.CLDKFunctionPDG) {

	// Un blocco è control-dependent sulla condizione se non è
	// post-dominato dal blocco del branch.
	if postDominates(postDom, block, branchBlock) {
		return // Il blocco viene eseguito comunque, non c'è control dependency
	}

	for _, instr := range block.Instrs {
		if toID, ok := instrIndex[instr]; ok {
			fnPDG.ControlEdges = append(fnPDG.ControlEdges, schema.PDGCtrlEdge{
				From:      branchNodeID,
				To:        toID,
				Condition: condition,
			})
		}
	}
}

// computePostDominators calcola un post-dominator immediato approssimato per ogni blocco.
// Usa un traversal inverso iterativo semplificato (sufficiente per PDG intra-procedurale).
func computePostDominators(fn *ssa.Function) map[*ssa.BasicBlock]*ssa.BasicBlock {
	blocks := fn.Blocks
	if len(blocks) == 0 {
		return nil
	}

	// Identifica i blocchi di uscita (senza successori o con Return/Panic)
	var exitBlocks []*ssa.BasicBlock
	for _, b := range blocks {
		if len(b.Succs) == 0 {
			exitBlocks = append(exitBlocks, b)
		}
	}
	if len(exitBlocks) == 0 {
		// Fallback: l'ultimo blocco è l'uscita
		exitBlocks = append(exitBlocks, blocks[len(blocks)-1])
	}

	// Post-dominator semplificato: per ogni blocco con un solo successore,
	// il post-dominator è quel successore. Per blocchi con due successori
	// (If), il post-dominator è il primo blocco comune raggiungibile da entrambi.
	idom := make(map[*ssa.BasicBlock]*ssa.BasicBlock)

	// Imposta i blocchi di uscita come post-dominati da sé stessi
	for _, eb := range exitBlocks {
		idom[eb] = eb
	}

	// Iterazione semplificata: dal fondo verso l'alto
	changed := true
	for iter := 0; changed && iter < len(blocks)*2; iter++ {
		changed = false
		// Itera in ordine inverso
		for i := len(blocks) - 1; i >= 0; i-- {
			b := blocks[i]
			if len(b.Succs) == 0 {
				continue // exit block, già impostato
			}

			var newIdom *ssa.BasicBlock
			if len(b.Succs) == 1 {
				newIdom = b.Succs[0]
			} else {
				// Per blocchi If: trova il Lowest Common Ancestor dei successori
				// nel post-dominator tree. Approssimazione: primo successore
				// comune raggiungibile.
				newIdom = findCommonPostDom(b.Succs, idom, exitBlocks)
			}

			if newIdom != nil && idom[b] != newIdom {
				idom[b] = newIdom
				changed = true
			}
		}
	}

	return idom
}

// findCommonPostDom trova il post-dominator comune tra i successori.
func findCommonPostDom(succs []*ssa.BasicBlock, idom map[*ssa.BasicBlock]*ssa.BasicBlock, exitBlocks []*ssa.BasicBlock) *ssa.BasicBlock {
	if len(succs) == 0 {
		return nil
	}
	if len(succs) == 1 {
		return succs[0]
	}

	// Raccogli tutti i blocchi raggiungibili dal primo successore
	reachable := make(map[*ssa.BasicBlock]bool)
	var walk func(b *ssa.BasicBlock, depth int)
	walk = func(b *ssa.BasicBlock, depth int) {
		if b == nil || reachable[b] || depth > 100 {
			return
		}
		reachable[b] = true
		for _, s := range b.Succs {
			walk(s, depth+1)
		}
	}
	walk(succs[0], 0)

	// Trova il primo blocco raggiungibile anche dal secondo successore
	// usando BFS
	visited := make(map[*ssa.BasicBlock]bool)
	queue := []*ssa.BasicBlock{succs[1]}
	visited[succs[1]] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if reachable[curr] && curr != succs[0] && curr != succs[1] {
			return curr
		}

		for _, s := range curr.Succs {
			if !visited[s] {
				visited[s] = true
				queue = append(queue, s)
			}
		}
	}

	// Fallback: se c'è un exit block raggiungibile, usalo
	if len(exitBlocks) > 0 {
		return exitBlocks[0]
	}

	return nil
}

// postDominates restituisce true se pdom post-domina target.
func postDominates(idom map[*ssa.BasicBlock]*ssa.BasicBlock, pdom, target *ssa.BasicBlock) bool {
	if pdom == target {
		return true
	}

	// Risali la catena dei post-dominatori di target
	visited := make(map[*ssa.BasicBlock]bool)
	curr := target
	for curr != nil && !visited[curr] {
		visited[curr] = true
		if curr == pdom {
			return true
		}
		next := idom[curr]
		if next == curr {
			break // raggiunto un nodo root
		}
		curr = next
	}

	return false
}

// dedupDataEdges rimuove data edges duplicati.
func dedupDataEdges(edges []schema.PDGDataEdge) []schema.PDGDataEdge {
	seen := make(map[string]bool)
	result := make([]schema.PDGDataEdge, 0, len(edges))
	for _, e := range edges {
		key := fmt.Sprintf("%d→%d:%s", e.From, e.To, e.VarName)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

// instrKind classifica un'istruzione SSA in una categoria PDG.
func instrKind(instr ssa.Instruction) string {
	switch instr.(type) {
	case *ssa.Phi:
		return "phi"
	case *ssa.Call:
		return "call"
	case *ssa.Store:
		return "store"
	case *ssa.If:
		return "branch"
	case *ssa.Return:
		return "return"
	case *ssa.FieldAddr, *ssa.IndexAddr, *ssa.Field, *ssa.Index:
		return "field"
	case *ssa.BinOp, *ssa.UnOp:
		return "assign"
	case *ssa.Alloc:
		return "assign"
	case *ssa.MakeInterface, *ssa.MakeSlice, *ssa.MakeMap, *ssa.MakeChan:
		return "assign"
	case *ssa.Convert, *ssa.ChangeType, *ssa.ChangeInterface:
		return "assign"
	case *ssa.Extract, *ssa.Slice, *ssa.Lookup:
		return "assign"
	case *ssa.Go:
		return "call"
	case *ssa.Defer:
		return "call"
	case *ssa.Send:
		return "call"
	case *ssa.Jump:
		return "branch"
	case *ssa.Panic:
		return "return"
	default:
		return "other"
	}
}

// instrString restituisce una rappresentazione leggibile dell'istruzione.
func instrString(instr ssa.Instruction) string {
	s := instr.String()
	// Tronca stringhe troppo lunghe
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
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
		t = normalizeReceiverType(t, pkg)
		return fmt.Sprintf("%s.%s.%s", pkg, t, name)
	}

	return fmt.Sprintf("%s.%s", pkg, name)
}

// normalizeReceiverType normalizza il tipo receiver per l'ID.
func normalizeReceiverType(t, pkg string) string {
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

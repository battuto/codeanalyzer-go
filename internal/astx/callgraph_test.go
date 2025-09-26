package astx

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCallGraph_CHA_Hello(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filepath.Dir(file)), "..", "testdata", "hello")
	root = filepath.Clean(root)

	cg, err := BuildCallGraph(CallGraphConfig{Root: root, Algo: "cha", IncludeTest: false, EmitPositions: "detailed"})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if len(cg.Nodes) == 0 || len(cg.Edges) == 0 {
		t.Fatalf("expected non-empty nodes and edges for CHA on hello; got nodes=%d edges=%d", len(cg.Nodes), len(cg.Edges))
	}
	// should have an edge to fmt.Println
	hasPrint := false
	for _, e := range cg.Edges {
		if strings.Contains(e.Dst, "fmt.Println") {
			hasPrint = true
			break
		}
	}
	if !hasPrint {
		t.Fatalf("expected an edge to fmt.Println in CHA graph")
	}
}

func TestCallGraph_CHA_RTA_Print(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filepath.Dir(file)), "..", "testdata", "print")
	root = filepath.Clean(root)

	chaCG, err := BuildCallGraph(CallGraphConfig{Root: root, Algo: "cha", IncludeTest: false, EmitPositions: "minimal"})
	if err != nil {
		t.Fatalf("CHA BuildCallGraph: %v", err)
	}
	rtaCG, err := BuildCallGraph(CallGraphConfig{Root: root, Algo: "rta", IncludeTest: false, EmitPositions: "minimal"})
	if err != nil {
		t.Fatalf("RTA BuildCallGraph: %v", err)
	}

	if len(chaCG.Edges) == 0 || len(chaCG.Nodes) == 0 {
		t.Fatalf("expected non-empty CHA graph on print sample")
	}
	if len(rtaCG.Edges) == 0 || len(rtaCG.Nodes) == 0 {
		t.Fatalf("expected non-empty RTA graph on print sample")
	}

	// RTA should be <= CHA in number of edges
	if len(rtaCG.Edges) > len(chaCG.Edges) {
		t.Fatalf("expected RTA edges <= CHA edges, got rta=%d cha=%d", len(rtaCG.Edges), len(chaCG.Edges))
	}

	// There should be an edge to fmt.Println and from main.main
	hasToPrint := false
	hasFromMain := false
	for _, e := range rtaCG.Edges {
		if strings.Contains(e.Dst, "fmt.Println") {
			hasToPrint = true
		}
		// src includes .main (function name)
		if strings.HasSuffix(e.Src, ".main") || strings.Contains(e.Src, ".(main)") || strings.Contains(e.Src, ".(main.main)") {
			hasFromMain = true
		}
	}
	if !hasToPrint {
		t.Fatalf("expected an edge to fmt.Println in RTA graph")
	}
	if !hasFromMain {
		t.Fatalf("expected an edge from main in RTA graph")
	}
}

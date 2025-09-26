package astx

import (
	"path/filepath"
	"testing"
)

func TestE2E_Symbols_Print(t *testing.T) {
	bin := resolveAnalyzerPath(t)
	root := repoRootTB(t)
	fixture := filepath.Join(root, "testdata", "print")

	args := []string{
		"--root", fixture,
		"--mode", "symbol-table",
		"--out", "-",
	}
	stdout, stderr, err := runCLI(t, bin, args...)
	if err != nil {
		t.Fatalf("cli error: %v\n%s", err, string(stderr))
	}
	got := normalizeOutputPaths(stdout, root)
	golden := filepath.Join(root, "testdata", "golden", "print_symbol_table.json")
	writeOrCompareGolden(t, got, golden)
}

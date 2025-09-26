package astx

import (
	"path/filepath"
	"testing"
)

func TestE2E_CallGraph(t *testing.T) {
	bin := resolveAnalyzerPath(t)
	root := repoRootTB(t)

	cases := []struct {
		name    string
		fixture string
		cg      string
		minimal bool
		golden  string
	}{
		{name: "print_cha", fixture: "print", cg: "cha", golden: "print_callgraph_cha.json"},
		{name: "print_rta", fixture: "print", cg: "rta", golden: "print_callgraph_rta.json"},
		{name: "print_cha_min", fixture: "print", cg: "cha", minimal: true, golden: "print_callgraph_cha_minimal.json"},
		{name: "iface_cha", fixture: "iface", cg: "cha", golden: "iface_callgraph_cha.json"},
		{name: "iface_rta", fixture: "iface", cg: "rta", golden: "iface_callgraph_rta.json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixtureRoot := filepath.Join(root, "testdata", tc.fixture)
			emit := "detailed"
			if tc.minimal {
				emit = "minimal"
			}
			// limit to only the fixture package to stabilize edges
			only := pkgPathForFixture(t, tc.fixture)

			args := []string{
				"--root", fixtureRoot,
				"--mode", "call-graph",
				"--cg", tc.cg,
				"--emit-positions", emit,
				"--only-pkg", only,
				"--out", "-",
			}
			stdout, stderr, err := runCLI(t, bin, args...)
			if err != nil {
				t.Fatalf("cli error: %v\n%s", err, string(stderr))
			}
			got := normalizeOutputPaths(stdout, root)
			golden := filepath.Join(root, "testdata", "golden", tc.golden)
			writeOrCompareGolden(t, got, golden)
		})
	}
}

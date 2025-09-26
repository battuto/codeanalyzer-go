package astx

import (
	"path/filepath"
	"testing"
)

func TestE2E_Full(t *testing.T) {
	bin := resolveAnalyzerPath(t)
	root := repoRootTB(t)

	cases := []struct {
		name    string
		fixture string
		cg      string
		golden  string
	}{
		{name: "print_full_cha", fixture: "print", cg: "cha", golden: "print_full_cha.json"},
		{name: "print_full_rta", fixture: "print", cg: "rta", golden: "print_full_rta.json"},
		{name: "iface_full_cha", fixture: "iface", cg: "cha", golden: "iface_full_cha.json"},
		{name: "iface_full_rta", fixture: "iface", cg: "rta", golden: "iface_full_rta.json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixtureRoot := filepath.Join(root, "testdata", tc.fixture)
			only := pkgPathForFixture(t, tc.fixture)
			args := []string{
				"--root", fixtureRoot,
				"--mode", "full",
				"--cg", tc.cg,
				"--emit-positions", "detailed",
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

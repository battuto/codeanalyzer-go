package astx

import (
	"bytes"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files in testdata/golden")

func repoRootTB(tb testing.TB) string {
	tb.Helper()
	_, file, _, _ := runtime.Caller(0)
	start := filepath.Dir(file)
	root, ok := findRepoRoot(start)
	if !ok {
		tb.Fatalf("could not locate repo root starting from %s", start)
	}
	return root
}

func findRepoRoot(start string) (string, bool) {
	cur := start
	for i := 0; i < 10; i++ { // cap to avoid infinite loop
		gomod := filepath.Join(cur, "go.mod")
		if st, err := os.Stat(gomod); err == nil && !st.IsDir() {
			return cur, true
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return "", false
}

func resolveAnalyzerPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("CLDK_GO_ANALYZER"); strings.TrimSpace(p) != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		t.Logf("CLDK_GO_ANALYZER set but not found: %s", p)
	}
	// build locally into temp dir
	tmp := t.TempDir()
	bin := "codeanalyzer-go"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	outPath := filepath.Join(tmp, bin)
	// build cmd/codeanalyzer-go with working directory at repo root
	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/codeanalyzer-go")
	cmd.Dir = repoRootTB(t)
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("skipping E2E: failed to build analyzer: %v\n%s", err, stderr.String())
	}
	return outPath
}

func runCLI(t *testing.T, bin string, args ...string) (stdout, stderr []byte, err error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRootTB(t)
	cmd.Env = os.Environ()
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func normalizeOutputPaths(b []byte, root string) []byte {
	s := string(b)
	// normalize path separators to '/'
	s = strings.ReplaceAll(s, "\\", "/")
	// replace absolute root with $ROOT
	root = strings.ReplaceAll(root, "\\", "/")
	s = strings.ReplaceAll(s, root, "$ROOT")
	return []byte(s)
}

func writeOrCompareGolden(t *testing.T, got []byte, goldenPath string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("golden missing: %s (run with -update to create)", goldenPath)
		}
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		// on mismatch, write a .got file near golden for inspection
		_ = os.WriteFile(goldenPath+".got", got, 0o644)
		// show a short diff-like context
		t.Fatalf("output does not match golden %s\n--- got (saved as .got)\n--- want\nfirst 200 bytes:\nGOT:  %q\nWANT: %q", goldenPath, preview(got), preview(want))
	}
}

func preview(b []byte) string {
	if len(b) > 200 {
		return string(b[:200])
	}
	return string(b)
}

func mustJoin(tb testing.TB, elems ...string) string {
	tb.Helper()
	p := filepath.Join(elems...)
	return p
}

func readAll(r io.Reader) []byte {
	var b bytes.Buffer
	_, _ = b.ReadFrom(r)
	return b.Bytes()
}

func modulePath(t *testing.T) string {
	t.Helper()
	root := repoRootTB(t)
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Skipf("skipping E2E: unable to read go.mod: %v", err)
	}
	lines := strings.Split(string(b), "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "module "))
		}
	}
	t.Skipf("skipping E2E: module path not found in go.mod")
	return ""
}

func pkgPathForFixture(t *testing.T, fixture string) string {
	mod := modulePath(t)
	// use URL-style join for module paths
	return joinSlash(mod, "testdata", fixture)
}

func joinSlash(elems ...string) string {
	s := strings.Join(elems, "/")
	s = strings.ReplaceAll(s, "//", "/")
	return s
}

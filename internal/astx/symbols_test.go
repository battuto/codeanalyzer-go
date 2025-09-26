package astx

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

func TestExtractSymbols_Hello(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filepath.Dir(file)), "..", "testdata", "hello")
	root = filepath.Clean(root)

	prog, err := loader.LoadWithOptions(root, loader.Options{IncludeTest: false})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	st := ExtractSymbols(prog)
	if st == nil || len(st.Packages) == 0 {
		t.Fatalf("expected symbol table with packages")
	}

	// Find hello package
	var pkg *schema.Package
	for i := range st.Packages {
		if st.Packages[i].Path == "hello" {
			pkg = &st.Packages[i]
			break
		}
	}
	if pkg == nil {
		t.Fatalf("package 'hello' not found")
	}

	// Imports should include fmt without alias
	foundFmt := false
	for _, im := range pkg.Imports {
		if im.Path == "fmt" {
			foundFmt = true
			if im.Alias != "" {
				t.Fatalf("expected no alias for fmt import, got %q", im.Alias)
			}
		}
	}
	if !foundFmt {
		t.Fatalf("fmt import not found")
	}

	// Types should include T struct
	hasT := false
	for _, td := range pkg.Types {
		if td.Name == "T" {
			hasT = true
			if td.Kind != "struct" {
				t.Fatalf("expected T kind struct, got %q", td.Kind)
			}
		}
	}
	if !hasT {
		t.Fatalf("type T not found")
	}

	// Functions signatures
	foundDo := false
	foundFree := false
	for _, fn := range pkg.Functions {
		if fn.Name == "Do" {
			foundDo = true
			if fn.Receiver == "" || !strings.Contains(fn.Signature, "func (t *T) Do()") {
				t.Fatalf("unexpected signature for Do: %q receiver=%q", fn.Signature, fn.Receiver)
			}
		}
		if fn.Name == "Free" {
			foundFree = true
			if fn.Signature != "func Free()" {
				t.Fatalf("unexpected signature for Free: %q", fn.Signature)
			}
		}
	}
	if !foundDo || !foundFree {
		t.Fatalf("expected functions Do and Free; got Do=%v Free=%v", foundDo, foundFree)
	}
}

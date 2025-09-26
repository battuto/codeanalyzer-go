# codeanalyzer-go (skeleton)

Minimal CLI analyzer for Go meant to integrate with the CodeLLM DevKit (CLDK) style.
This MVP scans a project root, parses `.go` files (excluding `vendor/`, `.git/`, `testdata/`),
and emits a **symbol table** and a **(placeholder) call graph** as JSON to STDOUT.

## Build
```bash
go build -o bin/codeanalyzer-go ./cmd/codeanalyzer-go
```

## Usage

```bash
codeanalyzer-go   --root /path/to/project   --mode symbol-table|call-graph|full   --cg cha|rta   --out -
```

* `--out -` writes JSON to STDOUT (default). If a file path is provided, JSON is written there.
* `--mode` selects what to emit; `full` emits both symbol table and call graph.

## Notes

* The call-graph is a placeholder for now. Wire up `golang.org/x/tools/go/ssa` and
  `golang.org/x/tools/go/callgraph` to make it precise.
* JSON schema is intentionally simple and CLDK-friendly.

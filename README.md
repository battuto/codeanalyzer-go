# codeanalyzer-go

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Static analyzer for Go projects**, compatible with [CodeLLM DevKit (CLDK)](https://github.com/codellm-devkit). Produces Symbol Table and Call Graph in stable JSON format.

## Features

- **Symbol Table Extraction**: packages, imports, types (struct/interface/alias), functions, methods, variables, constants
- **Interface Method Signatures**: extracts methods declared in interfaces with parameters, results and documentation
- **Package Documentation**: extracts package-level doc comments
- **Call Examples**: identifies callers of each function (requires `--include-body`)
- **Clean Documentation**: newlines removed from all docstrings for cleaner JSON output
- **Call Graph Construction**: using `golang.org/x/tools/go/ssa` with CHA or RTA algorithms
- **CLDK Compatible**: output follows CLDK schema conventions for seamless integration
- **Compact Output**: LLM-optimized format with ~70-85% size reduction
- **Flexible Filtering**: exclude directories, filter by package path, include/exclude tests
- **Position Tracking**: detailed or minimal source position information

## Installation

### Prerequisites

This analyzer is designed to work with [CodeLLM DevKit (CLDK)](https://github.com/codellm-devkit/cldk). If you plan to use it with CLDK:

1. First, fork/clone the CLDK repository:
   ```bash
   git clone https://github.com/codellm-devkit/cldk.git
   ```

2. Then, fork/clone this analyzer (separate repository):
   ```bash
   git clone https://github.com/battuto/codeanalyzer-go.git
   ```

### Build from Source

```bash
cd codeanalyzer-go
go build -o bin/codeanalyzer-go ./cmd/codeanalyzer-go
```

### Cross-Platform 64-bit Builds

Build for all platforms at once:

```bash
# Linux/macOS
make build-all

# Windows (PowerShell)
.\scripts\build.ps1
```

Generates binaries in `bin/`:
- `codeanalyzer-go-windows-amd64.exe`
- `codeanalyzer-go-linux-amd64`
- `codeanalyzer-go-darwin-amd64` (Intel Mac)
- `codeanalyzer-go-darwin-arm64` (Apple Silicon)

### Standalone Usage

You can also use this analyzer independently without CLDK:

```bash
go install github.com/battuto/codeanalyzer-go/cmd/codeanalyzer-go@latest
```

## Quick Start

```bash
# Analyze a project (full analysis to stdout)
codeanalyzer-go --input /path/to/project

# Symbol table only
codeanalyzer-go --input ./myproject --analysis-level symbol_table

# Call graph with RTA algorithm
codeanalyzer-go --input ./myproject --analysis-level call_graph --cg rta

# Save output to directory
codeanalyzer-go --input ./myproject --output ./output
```

## CLI Reference

```
codeanalyzer-go [flags]
```

### Main Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--input` | `-i` | Path to Go project root | `.` |
| `--output` | `-o` | Output directory (omit for stdout) | stdout |
| `--analysis-level` | `-a` | Analysis level: `symbol_table`, `call_graph`, `full` | `full` |
| `--cg` | | Call graph algorithm: `cha`, `rta` | `rta` |
| `--format` | `-f` | Output format: `json` | `json` |
| `--compact` | `-c` | **LLM-optimized output** (~70-85% smaller) | `false` |

### Filtering Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--include-tests` | Include `*_test.go` files | `--include-tests` |
| `--exclude-dirs` | Comma-separated directories to exclude | `--exclude-dirs vendor,testdata` |
| `--only-pkg` | Filter packages by path substring | `--only-pkg myapp/internal` |

### Output Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--emit-positions` | Position detail: `detailed`, `minimal` | `detailed` |
| `--include-body` | Include function body information | `false` |
| `--verbose`, `-v` | Enable verbose logging to stderr | `false` |
| `--quiet`, `-q` | Suppress non-error output | `false` |
| `--version` | Show version and exit | |

## Output Schema

The output follows CLDK conventions with this structure:

```json
{
  "metadata": {
    "analyzer": "codeanalyzer-go",
    "version": "2.0.0",
    "language": "go",
    "analysis_level": "full",
    "timestamp": "2026-02-01T12:00:00Z",
    "project_path": "/path/to/project",
    "go_version": "go1.21",
    "analysis_duration_ms": 1234
  },
  "symbol_table": {
    "packages": {
      "example.com/myapp": {
        "path": "example.com/myapp",
        "name": "myapp",
        "documentation": "Package myapp provides the main entry point.",
        "files": ["main.go", "util.go"],
        "imports": [{"path": "fmt"}],
        "type_declarations": {
          "example.com/myapp.Service": {
            "kind": "interface",
            "interface_methods": [
              {
                "name": "Start",
                "signature": "Start() error",
                "parameters": [],
                "results": [{"type": "error"}],
                "documentation": "Start initializes the service."
              }
            ]
          }
        },
        "callable_declarations": {
          "example.com/myapp.main": {
            "name": "main",
            "signature": "func main()",
            "kind": "function",
            "call_examples": ["called by init() [call]"]
          }
        },
        "variables": {},
        "constants": {}
      }
    }
  },
  "call_graph": {
    "algorithm": "rta",
    "nodes": [{"id": "example.com/myapp.main", "kind": "function"}],
    "edges": [{"source": "example.com/myapp.main", "target": "fmt.Println", "kind": "call"}]
  },
  "pdg": null,
  "sdg": null,
  "issues": []
}
```

### Key Schema Conventions

- **Maps, not arrays**: `packages`, `type_declarations`, `callable_declarations` are maps keyed by qualified name
- **Qualified names**: Format is `pkg.Func` or `pkg.(*Type).Method`
- **Positions**: Include `file`, `start_line`, `start_column`
- **Clean documentation**: all newlines removed from docstrings for cleaner output
- **Interface methods**: `interface_methods` array on interface types with name, signature, parameters, results, documentation
- **Call examples**: `call_examples` array on callables (requires `--include-body`)

## LLM Compact Output

Use `--compact` for LLM-optimized output with ~70-85% size reduction:

```bash
# Compact symbol table
codeanalyzer-go -i ./myproject -a symbol_table --compact > analysis.json

# Compact full analysis with body
codeanalyzer-go -i ./myproject -a full --compact --include-body -o ./output
```

**Compact schema features:**
- Abbreviated JSON keys (`metadata` -> `m`, `packages` -> `p`)
- Package documentation: `d` field on packages
- Interface methods: `im` field on types (signature strings)
- Call examples: `ex` field on functions
- Documentation only for exported functions (truncated to 200 chars)
- No position information
- Simplified call graph edges: `[[source, target], ...]`

## Call Graph Algorithms

| Algorithm | Description | Best For |
|-----------|-------------|----------|
| **CHA** | Class Hierarchy Analysis - conservative, includes all possible call targets | Complete analysis, interface-heavy code |
| **RTA** | Rapid Type Analysis - more precise, starts from `main()` | Focused analysis, smaller output |

```bash
# CHA (more conservative)
codeanalyzer-go --input ./myapp --analysis-level call_graph --cg cha

# RTA (more precise, requires main package)
codeanalyzer-go --input ./myapp --analysis-level call_graph --cg rta
```

## CLDK Python Integration

```python
from cldk.analysis import GoAnalyzer

analyzer = GoAnalyzer()
result = analyzer.analyze("/path/to/project", level="call_graph")

# Access symbol table
for pkg_path, pkg in result.symbol_table.packages.items():
    print(f"Package: {pkg.name}")
    for fn_name, fn in pkg.callable_declarations.items():
        print(f"  Function: {fn.name} - {fn.signature}")

# Access call graph
for edge in result.call_graph.edges:
    print(f"{edge.source} -> {edge.target}")
```

## Testing

```bash
# Build all platform binaries
.\scripts\build.ps1

# Run Go unit tests
go test ./...

# Run Python integration tests (35 tests)
python tests/cldk_integration_test.py
```

The integration tests cover: schema validation, new features (interface methods, package doc, call examples), compact format, real-world target (FRP project), legacy compatibility, positions, and error handling.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Analysis errors (partial results may be available) |
| `2` | Configuration or validation errors |

## Deprecated Flags (Legacy)

The following flags are deprecated but still supported for backward compatibility:

| Deprecated | Use Instead |
|------------|-------------|
| `--root` | `--input` |
| `--mode` | `--analysis-level` |
| `--out` | `--output` |
| `--include-test` | `--include-tests` |

## Troubleshooting

### Empty Call Graph with RTA

RTA requires a `main` package as entry point. If your project is a library, use CHA instead:

```bash
codeanalyzer-go --input ./mylib --analysis-level call_graph --cg cha
```

### Large Projects

For large codebases, use filters to reduce analysis scope:

```bash
codeanalyzer-go --input ./bigproject \
  --exclude-dirs vendor,testdata,examples \
  --only-pkg mycompany/bigproject/core
```

### Verbose Mode

Enable verbose mode to see analysis progress:

```bash
codeanalyzer-go --input ./myproject --verbose
```

## Project Structure

```
codeanalyzer-go/
├── cmd/codeanalyzer-go/    # CLI entry point
├── internal/
│   ├── loader/             # Package loading with SSA support
│   ├── symbols/            # Symbol table extraction
│   ├── callgraph/          # Call graph construction
│   └── output/             # JSON output writer
├── pkg/schema/             # CLDK schema definitions
├── tests/                  # Integration tests
└── sampleapp/              # Sample Go project for testing
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---
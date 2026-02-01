package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/codellm-devkit/codeanalyzer-go/internal/callgraph"
	"github.com/codellm-devkit/codeanalyzer-go/internal/loader"
	"github.com/codellm-devkit/codeanalyzer-go/internal/output"
	"github.com/codellm-devkit/codeanalyzer-go/internal/symbols"
	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

const (
	version = "2.0.0"

	// Analysis levels
	levelSymbolTable = "symbol_table"
	levelCallGraph   = "call_graph"
	levelPDG         = "pdg"
	levelSDG         = "sdg"
	levelFull        = "full"
)

type config struct {
	// Flag principali CLDK
	input         string
	outputDir     string
	format        string
	analysisLevel string

	// Flag avanzati
	cgAlgo        string
	includeTests  bool
	excludeDirs   string
	onlyPkg       string
	emitPositions string
	includeBody   bool
	verbose       bool
	quiet         bool
	showVersion   bool

	// Flag legacy (retrocompatibilità)
	root string
	mode string
	out  string
}

func main() {
	cfg := parseFlags()

	// Gestisci --version
	if cfg.showVersion {
		fmt.Printf("codeanalyzer-go %s\n", version)
		os.Exit(0)
	}

	// Retrocompatibilità: mappa flag legacy a nuovi flag
	cfg = handleLegacyFlags(cfg)

	// Valida configurazione
	if err := validateConfig(&cfg); err != nil {
		logError("configuration error: %v", err)
		os.Exit(2)
	}

	// Esegui analisi
	if err := runAnalysis(cfg); err != nil {
		logError("analysis error: %v", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config

	// Flag principali CLDK
	flag.StringVar(&cfg.input, "input", ".", "Path to the root of the Go project to analyze")
	flag.StringVar(&cfg.input, "i", ".", "Path to the root of the Go project to analyze (shorthand)")
	flag.StringVar(&cfg.outputDir, "output", "", "Output directory (omit for stdout)")
	flag.StringVar(&cfg.outputDir, "o", "", "Output directory (shorthand)")
	flag.StringVar(&cfg.format, "format", "json", "Output format: json|msgpack")
	flag.StringVar(&cfg.format, "f", "json", "Output format (shorthand)")
	flag.StringVar(&cfg.analysisLevel, "analysis-level", "full", "Analysis level: symbol_table|call_graph|pdg|sdg|full")
	flag.StringVar(&cfg.analysisLevel, "a", "full", "Analysis level (shorthand)")

	// Flag avanzati
	flag.StringVar(&cfg.cgAlgo, "cg", "rta", "Call graph algorithm: cha|rta")
	flag.BoolVar(&cfg.includeTests, "include-tests", false, "Include *_test.go files in analysis")
	flag.StringVar(&cfg.excludeDirs, "exclude-dirs", "", "Comma-separated directory basenames to exclude (e.g., vendor,.git)")
	flag.StringVar(&cfg.onlyPkg, "only-pkg", "", "Comma-separated package path filters (substring match)")
	flag.StringVar(&cfg.emitPositions, "emit-positions", "detailed", "Position verbosity: detailed|minimal")
	flag.BoolVar(&cfg.includeBody, "include-body", false, "Include function body information")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Enable verbose logging to stderr")
	flag.BoolVar(&cfg.verbose, "v", false, "Enable verbose logging (shorthand)")
	flag.BoolVar(&cfg.quiet, "quiet", false, "Suppress all non-error output")
	flag.BoolVar(&cfg.quiet, "q", false, "Suppress non-error output (shorthand)")
	flag.BoolVar(&cfg.showVersion, "version", false, "Show version and exit")

	// Flag legacy (retrocompatibilità deprecata)
	flag.StringVar(&cfg.root, "root", "", "[DEPRECATED] Use --input instead")
	flag.StringVar(&cfg.mode, "mode", "", "[DEPRECATED] Use --analysis-level instead")
	flag.StringVar(&cfg.out, "out", "", "[DEPRECATED] Use --output instead")
	// Alias per retrocompatibilità con vecchio flag
	flag.BoolVar(&cfg.includeTests, "include-test", false, "[DEPRECATED] Use --include-tests instead")

	flag.Parse()
	return cfg
}

func handleLegacyFlags(cfg config) config {
	// --root → --input
	if cfg.root != "" {
		logWarning("--root is deprecated, use --input instead")
		if cfg.input == "." {
			cfg.input = cfg.root
		}
	}

	// --mode → --analysis-level
	if cfg.mode != "" {
		logWarning("--mode is deprecated, use --analysis-level instead")
		if cfg.analysisLevel == "full" {
			// Mappa vecchi mode a nuovi
			switch cfg.mode {
			case "symbol-table":
				cfg.analysisLevel = levelSymbolTable
			case "call-graph":
				cfg.analysisLevel = levelCallGraph
			case "full":
				cfg.analysisLevel = levelFull
			default:
				cfg.analysisLevel = cfg.mode
			}
		}
	}

	// --out → --output
	if cfg.out != "" {
		logWarning("--out is deprecated, use --output instead")
		if cfg.outputDir == "" {
			// Il vecchio --out era un file path, non una directory
			if cfg.out != "-" {
				cfg.outputDir = filepath.Dir(cfg.out)
			}
		}
	}

	return cfg
}

func validateConfig(cfg *config) error {
	// Valida input path
	absInput, err := filepath.Abs(cfg.input)
	if err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	cfg.input = absInput

	// Verifica che input esista
	if _, err := os.Stat(cfg.input); os.IsNotExist(err) {
		return fmt.Errorf("input path does not exist: %s", cfg.input)
	}

	// Valida analysis level
	validLevels := map[string]bool{
		levelSymbolTable: true,
		levelCallGraph:   true,
		levelPDG:         true,
		levelSDG:         true,
		levelFull:        true,
	}
	if !validLevels[cfg.analysisLevel] {
		return fmt.Errorf("invalid analysis-level: %s (valid: symbol_table, call_graph, pdg, sdg, full)", cfg.analysisLevel)
	}

	// Valida format
	if cfg.format != "json" && cfg.format != "msgpack" {
		return fmt.Errorf("invalid format: %s (valid: json, msgpack)", cfg.format)
	}

	// Valida cg algorithm
	cgAlgo := strings.ToLower(cfg.cgAlgo)
	if cgAlgo != "cha" && cgAlgo != "rta" {
		return fmt.Errorf("invalid cg algorithm: %s (valid: cha, rta)", cfg.cgAlgo)
	}
	cfg.cgAlgo = cgAlgo

	// Valida emit-positions
	if cfg.emitPositions != "detailed" && cfg.emitPositions != "minimal" {
		return fmt.Errorf("invalid emit-positions: %s (valid: detailed, minimal)", cfg.emitPositions)
	}

	return nil
}

func runAnalysis(cfg config) error {
	startTime := time.Now()

	logVerbose(cfg, "Starting analysis...")
	logVerbose(cfg, "  Input: %s", cfg.input)
	logVerbose(cfg, "  Level: %s", cfg.analysisLevel)
	logVerbose(cfg, "  Algorithm: %s", cfg.cgAlgo)
	logVerbose(cfg, "  Go version: %s", runtime.Version())

	// Determina se serve SSA
	needSSA := cfg.analysisLevel == levelCallGraph || cfg.analysisLevel == levelFull

	// Carica pacchetti
	loaderOpts := loader.Options{
		IncludeTest: cfg.includeTests,
		ExcludeDirs: splitCSV(cfg.excludeDirs),
		OnlyPkg:     splitCSV(cfg.onlyPkg),
		NeedSSA:     needSSA,
	}

	logVerbose(cfg, "Loading packages...")
	result, err := loader.LoadWithSSA(cfg.input, loaderOpts)
	if err != nil {
		return fmt.Errorf("load packages: %w", err)
	}
	logVerbose(cfg, "Loaded %d packages", len(result.Packages))

	// Inizializza analisi CLDK
	analysis := &schema.CLDKAnalysis{
		Metadata: schema.Metadata{
			Analyzer:      "codeanalyzer-go",
			Version:       version,
			Language:      "go",
			AnalysisLevel: cfg.analysisLevel,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			ProjectPath:   cfg.input,
			GoVersion:     runtime.Version(),
		},
		PDG:    nil,
		SDG:    nil,
		Issues: []schema.Issue{},
	}

	// Estrai symbol table se richiesto
	if cfg.analysisLevel == levelSymbolTable || cfg.analysisLevel == levelFull {
		logVerbose(cfg, "Extracting symbols...")
		symbolCfg := symbols.ExtractConfig{
			IncludeBody:      cfg.includeBody,
			EmitPositions:    cfg.emitPositions,
			IncludeCallSites: cfg.includeBody,
		}
		analysis.SymbolTable = symbols.Extract(result, symbolCfg)
		logVerbose(cfg, "Extracted %d packages", len(analysis.SymbolTable.Packages))
	}

	// Costruisci call graph se richiesto
	if cfg.analysisLevel == levelCallGraph || cfg.analysisLevel == levelFull {
		logVerbose(cfg, "Building call graph with %s...", cfg.cgAlgo)
		cgCfg := callgraph.Config{
			Algorithm:     cfg.cgAlgo,
			EmitPositions: cfg.emitPositions,
			OnlyPkg:       splitCSV(cfg.onlyPkg),
		}
		cg, err := callgraph.Build(result, cgCfg)
		if err != nil {
			// Non bloccare, aggiungi issue
			analysis.Issues = append(analysis.Issues, schema.Issue{
				Severity: "warning",
				Code:     "CALLGRAPH_ERROR",
				Message:  fmt.Sprintf("Failed to build call graph: %v", err),
			})
			logWarning("call graph build failed: %v", err)
		} else {
			analysis.CallGraph = cg
			logVerbose(cfg, "Call graph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
		}
	}

	// PDG/SDG placeholder (non implementato)
	if cfg.analysisLevel == levelPDG || cfg.analysisLevel == levelSDG {
		analysis.Issues = append(analysis.Issues, schema.Issue{
			Severity: "info",
			Code:     "NOT_IMPLEMENTED",
			Message:  fmt.Sprintf("%s analysis is not yet implemented", strings.ToUpper(cfg.analysisLevel)),
		})
	}

	// Calcola durata
	analysis.Metadata.AnalysisDurationMs = time.Since(startTime).Milliseconds()

	// Scrivi output
	logVerbose(cfg, "Writing output...")
	outCfg := output.Config{
		OutputDir: cfg.outputDir,
		Format:    output.Format(cfg.format),
		Indent:    true,
	}
	if err := output.Write(analysis, outCfg); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	logVerbose(cfg, "Analysis completed in %dms", analysis.Metadata.AnalysisDurationMs)

	return nil
}

// ============================================================================
// Helper functions
// ============================================================================

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func logVerbose(cfg config, format string, args ...interface{}) {
	if cfg.verbose && !cfg.quiet {
		fmt.Fprintf(os.Stderr, "[info] "+format+"\n", args...)
	}
}

func logWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[warning] "+format+"\n", args...)
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[error] "+format+"\n", args...)
}

// Package output gestisce la scrittura dell'output CLDK.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/codellm-devkit/codeanalyzer-go/pkg/schema"
)

// Format rappresenta il formato di output supportato.
type Format string

const (
	FormatJSON    Format = "json"
	FormatMsgpack Format = "msgpack" // placeholder per futuro supporto
)

// Config configura l'output writer.
type Config struct {
	OutputDir string // directory output (vuoto = stdout)
	Format    Format // json|msgpack (default: json)
	Indent    bool   // indentazione JSON (default: true)
}

// Write scrive l'analisi CLDK nel formato specificato.
func Write(analysis *schema.CLDKAnalysis, cfg Config) error {
	if cfg.Format == "" {
		cfg.Format = FormatJSON
	}

	switch cfg.Format {
	case FormatJSON:
		return writeJSON(analysis, cfg)
	case FormatMsgpack:
		return fmt.Errorf("msgpack format not yet implemented")
	default:
		return fmt.Errorf("unsupported format: %s", cfg.Format)
	}
}

// writeJSON scrive l'output in formato JSON.
func writeJSON(analysis *schema.CLDKAnalysis, cfg Config) error {
	return writeJSONGeneric(analysis, cfg)
}

// WriteCompact scrive l'analisi in formato compatto per LLM.
// Usa indentazione per leggibilità.
func WriteCompact(analysis *schema.CompactAnalysis, cfg Config) error {
	cfg.Indent = true
	return writeJSONGeneric(analysis, cfg)
}

// writeJSONGeneric scrive qualsiasi struttura in formato JSON.
func writeJSONGeneric(data interface{}, cfg Config) error {
	var w io.Writer

	if cfg.OutputDir == "" {
		// Output su stdout
		w = os.Stdout
	} else {
		// Crea directory se non esiste
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}

		// Crea file analysis.json
		outPath := filepath.Join(cfg.OutputDir, "analysis.json")
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	enc := json.NewEncoder(w)
	if cfg.Indent {
		enc.SetIndent("", "  ")
	}
	// Assicura che i caratteri speciali non siano escaped
	enc.SetEscapeHTML(false)

	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	return nil
}

// WriteToFile scrive direttamente su un file specificato.
func WriteToFile(analysis *schema.CLDKAnalysis, filePath string, indent bool) error {
	// Crea directory se non esiste
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}
	}

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if indent {
		enc.SetIndent("", "  ")
	}
	enc.SetEscapeHTML(false)

	if err := enc.Encode(analysis); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	return nil
}

// WriteToStdout scrive su stdout con opzione di indentazione.
func WriteToStdout(analysis *schema.CLDKAnalysis, indent bool) error {
	enc := json.NewEncoder(os.Stdout)
	if indent {
		enc.SetIndent("", "  ")
	}
	enc.SetEscapeHTML(false)

	if err := enc.Encode(analysis); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	return nil
}

// ToJSON converte l'analisi in JSON string.
func ToJSON(analysis *schema.CLDKAnalysis, indent bool) (string, error) {
	var data []byte
	var err error

	if indent {
		data, err = json.MarshalIndent(analysis, "", "  ")
	} else {
		data, err = json.Marshal(analysis)
	}

	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	return string(data), nil
}

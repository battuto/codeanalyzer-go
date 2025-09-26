package loader

import (
	"os"
	"path/filepath"
	"strings"
)

// Program is a simple file listing rooted at Root.
type Program struct {
	Root  string
	Files []string // absolute paths to .go files
}

// Options controlla il comportamento del loader.
type Options struct {
	IncludeTest bool
	ExcludeDirs []string // basenames da escludere
	OnlyPkg     []string // filtra per sottostringa nel path relativo
}

// Load walks the root directory and collects .go files, excluding vendor/.git/testdata.
func Load(root string) (*Program, error) {
	return LoadWithOptions(root, Options{})
}

// LoadWithOptions cammina la directory root e raccoglie i file .go secondo le opzioni.
func LoadWithOptions(root string, opts Options) (*Program, error) {
	ex := map[string]struct{}{
		"vendor":   {},
		".git":     {},
		"testdata": {},
	}
	for _, d := range opts.ExcludeDirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		ex[d] = struct{}{}
	}

	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if _, skip := ex[base]; skip || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			if !opts.IncludeTest && strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// only-pkg filtro su path relativo
			if len(opts.OnlyPkg) > 0 {
				rel := path
				if rp, err := filepath.Rel(root, path); err == nil {
					rel = rp
				}
				keep := false
				rp := filepath.ToSlash(rel)
				for _, s := range opts.OnlyPkg {
					s = strings.TrimSpace(s)
					if s == "" {
						continue
					}
					if strings.Contains(rp, s) {
						keep = true
						break
					}
				}
				if !keep {
					return nil
				}
			}
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Program{Root: root, Files: files}, nil
}

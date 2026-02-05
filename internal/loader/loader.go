package loader

import (
	"fmt"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Program is a simple file listing rooted at Root (legacy).
type Program struct {
	Root  string
	Files []string // absolute paths to .go files
}

// LoadResult contiene il risultato del caricamento con supporto SSA opzionale.
type LoadResult struct {
	Packages    []*packages.Package
	SSAProgram  *ssa.Program   // nil se NeedSSA è false
	SSAPackages []*ssa.Package // nil se NeedSSA è false
	Fset        *token.FileSet
	Root        string
}

// Options controlla il comportamento del loader.
type Options struct {
	IncludeTest bool
	ExcludeDirs []string // basenames da escludere
	OnlyPkg     []string // filtra per sottostringa nel path relativo
	NeedSSA     bool     // se true, costruisce anche SSA
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

// LoadWithSSA carica i pacchetti Go usando go/packages e opzionalmente costruisce SSA.
func LoadWithSSA(rootPath string, opts Options) (*LoadResult, error) {
	verbose := false // Could be added to Options if needed

	// Convert to absolute path
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Use "./..." pattern to load all packages recursively
	pattern := "./..."

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo,
		Dir: absRoot,
		// Include test files if requested
		Tests: opts.IncludeTest,
	}

	// Load all packages matching the pattern
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", rootPath)
	}

	// Check for errors in loaded packages (log only, don't fail)
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				log.Printf("Package error in %s: %v", pkg.PkgPath, e)
			}
		}
	}

	// Filter out packages with errors and apply user filters
	validPkgs := filterLoadedPackages(pkgs, opts.ExcludeDirs, opts.OnlyPkg)

	if len(validPkgs) == 0 {
		return nil, fmt.Errorf("no valid packages found (all had errors or were filtered)")
	}

	if verbose {
		log.Printf("Loaded %d valid packages out of %d total", len(validPkgs), len(pkgs))
	}

	// Get FileSet from first package (all packages share the same FileSet)
	var fset *token.FileSet
	for _, pkg := range validPkgs {
		if pkg.Fset != nil {
			fset = pkg.Fset
			break
		}
	}
	if fset == nil {
		fset = token.NewFileSet()
	}

	result := &LoadResult{
		Packages: validPkgs,
		Root:     absRoot,
		Fset:     fset,
	}

	// Build SSA if requested
	if opts.NeedSSA {
		result.SSAProgram, result.SSAPackages = buildSSAProgram(validPkgs, verbose)
	}

	return result, nil
}

// buildSSAProgram costruisce il programma SSA dai pacchetti caricati.
func buildSSAProgram(pkgs []*packages.Package, verbose bool) (*ssa.Program, []*ssa.Package) {
	if len(pkgs) == 0 {
		return nil, nil
	}

	// Create SSA program from packages
	// InstantiateGenerics è RICHIESTO per RTA: senza questo flag, RTA
	// va in panic quando incontra tipi generici (TypeParam).
	// Vedi: https://github.com/golang/go/issues/60137
	mode := ssa.SanityCheckFunctions | ssa.InstantiateGenerics
	prog, ssaPkgs := ssautil.AllPackages(pkgs, mode)
	prog.Build()

	// Filter out nil packages
	validSSA := make([]*ssa.Package, 0, len(ssaPkgs))
	for _, p := range ssaPkgs {
		if p != nil {
			validSSA = append(validSSA, p)
		}
	}

	if verbose {
		log.Printf("Built SSA for %d packages", len(validSSA))
	}

	return prog, validSSA
}

// filterLoadedPackages applica i filtri di directory e package.
func filterLoadedPackages(pkgs []*packages.Package, excludeDirs, onlyPkg []string) []*packages.Package {
	if len(excludeDirs) == 0 && len(onlyPkg) == 0 {
		return pkgs
	}

	ex := make(map[string]struct{})
	for _, d := range excludeDirs {
		d = strings.TrimSpace(d)
		if d != "" {
			ex[d] = struct{}{}
		}
	}

	keep := func(p *packages.Package) bool {
		if len(onlyPkg) > 0 {
			ok := false
			for _, s := range onlyPkg {
				s = strings.TrimSpace(s)
				if s != "" && strings.Contains(p.PkgPath, s) {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
		if len(ex) == 0 {
			return true
		}
		// Se uno qualsiasi dei file del pacchetto sta in una dir esclusa, escludi il pkg.
		for _, f := range p.GoFiles {
			base := filepath.Base(filepath.Dir(f))
			if _, ok := ex[base]; ok {
				return false
			}
		}
		return true
	}

	out := make([]*packages.Package, 0, len(pkgs))
	for _, p := range pkgs {
		if p != nil && keep(p) {
			out = append(out, p)
		}
	}
	return out
}

// collectAllPackages visita ricorsivamente Imports per includere tutte le dipendenze.
func collectAllPackages(roots []*packages.Package) []*packages.Package {
	seen := make(map[*packages.Package]struct{})
	var out []*packages.Package

	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
		for _, ip := range p.Imports {
			visit(ip)
		}
	}

	for _, r := range roots {
		visit(r)
	}
	return out
}

// dedupPackages rimuove duplicati per PkgPath mantenendo l'ordine di prima occorrenza.
func dedupPackages(in []*packages.Package) []*packages.Package {
	seen := make(map[string]struct{}, len(in))
	out := make([]*packages.Package, 0, len(in))
	for _, p := range in {
		if p == nil {
			continue
		}
		key := p.PkgPath
		if key == "" {
			key = fmt.Sprintf("%p", p)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

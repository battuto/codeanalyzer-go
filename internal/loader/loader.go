package loader

import (
	"fmt"
	"go/token"
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
func LoadWithSSA(root string, opts Options) (*LoadResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}

	// Configura go/packages
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedTypesSizes |
			packages.NeedImports |
			packages.NeedModule |
			packages.NeedDeps,
		Dir:   root,
		Tests: opts.IncludeTest,
		Env:   os.Environ(),
	}

	// Carica tutti i pacchetti sotto root
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %w", err)
	}

	// Log errori di caricamento (non bloccanti)
	if n := packages.PrintErrors(pkgs); n > 0 {
		if os.Getenv("LOG_LEVEL") == "debug" {
			fmt.Fprintf(os.Stderr, "[debug] packages.Load reported %d issues\n", n)
		}
	}

	// Recupera fset
	var fset *token.FileSet
	if len(pkgs) > 0 && pkgs[0].Fset != nil {
		fset = pkgs[0].Fset
	} else {
		fset = token.NewFileSet()
	}

	// Applica filtri exclude-dirs e only-pkg
	pkgs = filterLoadedPackages(pkgs, opts.ExcludeDirs, opts.OnlyPkg)

	result := &LoadResult{
		Packages: pkgs,
		Fset:     fset,
		Root:     root,
	}

	// Costruisci SSA se richiesto
	if opts.NeedSSA {
		// Raccogli tutti i pacchetti inclusi gli import
		allPkgs := collectAllPackages(pkgs)
		allPkgs = dedupPackages(allPkgs)

		// Costruisci SSA
		prog, ssaPkgs := ssautil.AllPackages(allPkgs, ssa.InstantiateGenerics)
		prog.Build()

		result.SSAProgram = prog
		result.SSAPackages = ssaPkgs
	}

	return result, nil
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

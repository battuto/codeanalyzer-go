# codeanalyzer-go

Analyzer Go compatibile con CodeLLM DevKit (CLDK) che emette Symbol Table e Call Graph in formato JSON stabile.

## Panoramica

Questo tool analizza un progetto Go a partire da una root e produce:
- Symbol Table: file, import, tipi (struct/interface/alias), funzioni/metodi con firma e posizione;
- Call Graph: nodi (funzioni/metodi) ed archi (chiamate), costruito davvero con `golang.org/x/tools` (CHA o RTA).

L’output JSON rispetta rigidamente lo schema definito in `pkg/schema/schema.go` (nessuna modifica a campi/chiavi).

## Funzionalità

- Caricamento progetto con `go/packages` (rispetta mod/workspace, GOOS/GOARCH, build tags).
- SSA + Call Graph reali:
  - CHA: conservative class-hierarchy analysis;
  - RTA: rapid type analysis, più snello (raggiungibilità da `main`).
- Posizioni nei sorgenti opzionali: dettagliate o minimali.
- Filtri:
  - `--exclude-dirs` per saltare directory (es. `vendor,.git`);
  - `--only-pkg` per limitare a uno o più package path (substring match);
  - `--include-test` per includere `*_test.go` (di default esclusi).
- Robustezza/perf:
  - ignora file con parse error ma continua;
  - deduplica nodi/archi (map/set);
  - carica la stdlib solo quando serve (RTA) per evitare panics e ridurre tempi.

## Requisiti

- Go 1.21+ installato ed in `PATH`.

## Build

Windows (cmd.exe):
```bat
cd /d C:\Users\ka-tu\Desktop\Tesi\codeanalyzer-go
go build -o bin\codeanalyzer-go.exe .\cmd\codeanalyzer-go
```

Linux/macOS:
```bash
cd codeanalyzer-go
go build -o bin/codeanalyzer-go ./cmd/codeanalyzer-go
```

Nota: su Windows il binario generato ha estensione `.exe`.

## Utilizzo

Sintassi generale:
```text
codeanalyzer-go --root <path> --mode symbol-table|call-graph|full --cg cha|rta --out - [altri flag]
```

Esecuzione rapida senza build (Windows, cmd.exe):
```bat
go run .\cmd\codeanalyzer-go\ --root sampleapp --mode full --cg rta --emit-positions detailed --out -
```

Flag supportate (compatibili e retro-compatibili):
- `--root` (string): root del progetto da analizzare (default: `.`).
- `--mode` (string): `symbol-table` | `call-graph` | `full` (default: `full`).
- `--cg` (string): `cha` | `rta` (usato quando `mode` include il call-graph; default: `cha`).
- `--out` (string): `-` per STDOUT (default) oppure path file.
- `--include-test` (bool): include i file `*_test.go`.
- `--exclude-dirs` (csv): directory da escludere per basename, es. `vendor,.git`.
- `--only-pkg` (csv): include solo i package il cui path contiene uno di questi token.
- `--emit-positions` (string): `detailed|minimal` (default: `detailed`).

Variabili d’ambiente:
- `LOG_LEVEL=debug`: stampa su STDERR un breve riepilogo (root, mode, cg, counts file/pkgs, versione go, OS/ARCH) senza inquinare STDOUT.

## Esempi veloci

Windows (cmd.exe):
```bat
cd /d C:\Users\ka-tu\Desktop\Tesi\codeanalyzer-go

:: Esecuzione senza build (consigliato in sviluppo)
go run .\cmd\codeanalyzer-go\ --root sampleapp --mode full --cg rta --emit-positions detailed --out -

:: In alternativa, usando il binario buildato
bin\codeanalyzer-go.exe --root sampleapp --mode symbol-table --out -
bin\codeanalyzer-go.exe --root sampleapp --mode call-graph --cg cha --emit-positions detailed --out -
bin\codeanalyzer-go.exe --root sampleapp --mode call-graph --cg rta --emit-positions minimal --only-pkg example.com/sampleapp --out -
```

Linux/macOS:
```bash
# Esecuzione senza build
go run ./cmd/codeanalyzer-go --root sampleapp --mode full --cg rta --emit-positions detailed --out -

# In alternativa, usando il binario
bin/codeanalyzer-go --root sampleapp --mode symbol-table --out -
bin/codeanalyzer-go --root sampleapp --mode call-graph --cg cha --emit-positions detailed --out -
bin/codeanalyzer-go --root sampleapp --mode call-graph --cg rta --emit-positions minimal --only-pkg example.com/sampleapp --out -
```

Suggerimenti:
- per avere un call-graph più “locale”, usa `--only-pkg <path_del_pacchetto>`;
- se non ti servono le posizioni, `--emit-positions minimal` riduce l’output.

## Compatibilità schema JSON

Lo schema è definito in `pkg/schema/schema.go` e non viene modificato dal tool. Campi principali:
- `SymbolTable`: `packages[]` con `files[]`, `imports[]`, `types[]`, `functions[]` (ognuno con `pos`).
- `CallGraph`: `nodes[]` con `id` stabile (es. `pkg.Func`, `pkg.(T).Method`, `pkg.(*T).Method`) e `pos`, `edges[]` (`src`,`dst`).

## Debug & Troubleshooting

- Setta `LOG_LEVEL=debug` per vedere su STDERR riepiloghi utili (versione Go, OS/ARCH, conteggi, eventuali warning di `go/packages`).
- Se il call-graph RTA risultasse vuoto, verifica che sotto `--root` ci sia un `main` eseguibile; RTA parte dalle radici `main`.
- Su workspace grandi, usa `--exclude-dirs` (es. `vendor,.git`) e `--only-pkg` per ridurre la superficie.

## Licenza

Questo progetto è distribuito sotto licenza MIT. Vedi il file `LICENSE` per i dettagli.

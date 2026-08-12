# SemiBase

SemiBase is the deployable PostgreSQL service for the semiconductor-tools installation. It
provisions and configures the instance that hosts the Simple-Scada 2 archive: the SCADA writes
`trends`/`messages` into it, SemiPlot and future tools read from it. The deliverable is one
CLI binary, `semibase.exe`, plus the instance's architecture docs.
Platform: Windows. Language: Go (module `github.com/Semiteq/SemiBase`), driver `pgx/v5`,
`sql/semiplot_tags.sql` embedded via `go:embed`. Entry point: `cmd/semibase`.
All commands run from the repository root.

## Build

```powershell
go build -o semibase.exe ./cmd/semibase
```

## Test

```powershell
go test ./...
go vet ./...
```

Unit tests live beside the source (`internal/provision/*_test.go`), table-driven. There are no
database-touching tests; the `verify` command is the integration check, run against a live server.

## Format

```powershell
gofmt -w .    # run before presenting changes; gofmt is authoritative
```

## Run

```powershell
.\semibase.exe --help                     # commands: config | create | verify | all
.\semibase.exe create --port 15432 --database semiplot_dev --expected-major 14
.\semibase.exe verify --reader-password <pw>   # post-writer proof of the reader access chain
```

Passwords come from flags, env, or a `.env` file in the working directory (flag > env > `.env`;
template `.env.example`): `SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD`,
`SEMIBASE_READER_PASSWORD`, `SEMIBASE_ADMIN_PASSWORD`.
`verify` failing with "writer has not run" before the SCADA's first start is the expected
order, not a bug.

## Layout

```
cmd/semibase/        CLI entry point: command dispatch, flags, usage text
internal/provision/  the phases: config (ALTER SYSTEM), create (db/roles/grants), verify
sql/                 DDL for objects we own (semiplot_tags), embedded into the binary
docs/                human docs in Russian (enter at docs/readme.md)
docs/architecture/   agent-facing design docs in English
docs/plans/          dated implementation plans
```

## Docs

- `docs/` (outside `architecture/`) — human-readable docs, Russian. Entry point `docs/readme.md`.
  These must never mention or link `docs/architecture/`.
- `docs/architecture/` — agent-facing design (machine-readable), English. Start at `README.md`.
- `docs/plans/` — dated implementation plans (`YYYYMMDD-<name>.md`); completed ones
  in `docs/plans/completed/`.
- The archive schema itself belongs to the SCADA and is documented in the SemiPlot repository
  (`docs/architecture/scada-archive.md` there); this repository documents the instance around it.

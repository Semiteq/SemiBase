# SemiBase

SemiBase is the deployable PostgreSQL service for the semiconductor-tools installation. It
provisions and configures the instance that hosts the Simple-Scada 2 archive: the SCADA writes
`trends`/`messages` into it, SemiPlot and future tools read from it. The deliverable is one
CLI binary, `semibase.exe`, plus the instance's architecture docs.
Deployment target: Windows. Language: Go (module `github.com/Semiteq/SemiBase`), driver
`pgx/v5`, `sql/semiplot_tags.sql` embedded via `go:embed`. Entry point: `cmd/semibase`.
The module is pure `pgx` and compiles for any GOOS; Linux builds serve the containerised
test bench of consumers (SemiPlot runs `create` against an ephemeral `postgres:17` container).
All commands run from the repository root.

## Build

```powershell
go build -o semibase.exe ./cmd/semibase
go build -ldflags "-X main.revision=<rev>" -o semibase.exe ./cmd/semibase   # release: embed the revision
```

Without ldflags, `version` falls back to the VCS revision recorded by the Go toolchain.

## Release

`.github/workflows/release.yml` fires on a `v*` tag push. One `ubuntu-latest` job
cross-compiles `windows/amd64` and `linux/amd64` with `CGO_ENABLED=0` — the module is pure
`pgx`, so native runners are CI's job, not the release's — and attaches
`semibase_<version>_<goos>_amd64[.exe]` plus a sha256 `checksums.txt` to the GitHub release.
`-X main.revision=<tag>` is embedded, so `semibase version` prints the tag.

The annotated tag carries the release text: its subject line becomes the release name,
everything after the first blank line becomes the body, verbatim. Blank lines survive, so
write the body in sections.

```bash
git tag -a v0.2.0 -F - <<'EOF'
v0.2.0 — short title

**New Features**

* what the user can now do, lowercase [#12](https://github.com/Semiteq/SemiBase/pull/12)

**Fixes**

* what stopped going wrong [#13](https://github.com/Semiteq/SemiBase/pull/13)
EOF
git push origin v0.2.0
```

Bold section labels, not `###` headings: git's default tag cleanup deletes every line
starting with `#`, which would silently strip the structure out of the annotation.
A lightweight tag still releases — the name falls back to `SemiBase <tag>`, the body to
`Release <tag>`.

## Test

```powershell
go test ./...
go vet ./...
golangci-lint run
```

golangci-lint is pinned to 2.12.2 (`winget install GolangCI.golangci-lint --version 2.12.2`);
config in `.golangci.yml`. CI (`.github/workflows/ci.yml`) runs `go build`, `go test -race`,
and the same lint on `windows-latest` and `ubuntu-latest` for every push and pull request;
the Linux job also provisions a `postgres:17-alpine` service container by running `all`
twice — the bench path and the idempotency check in one step.

Unit tests live beside the source (`cmd/semibase/*_test.go`, `internal/provision/*_test.go`),
table-driven. There are no database-touching tests; the `verify` command is the integration
check, run against a live server.

## Format

```powershell
gofmt -w .    # run before presenting changes; gofmt is authoritative
```

## Run

```powershell
.\semibase.exe --help                     # commands: config | create | verify | all | version
.\semibase.exe create --port 15432 --database semiplot_dev --expected-major 17
.\semibase.exe verify --reader-password <pw>   # post-writer proof of the reader access chain
.\semibase.exe version                    # print the build revision
```

Passwords come from flags, env, or a `.env` file in the working directory (flag > env > `.env`;
template `.env.example`): `SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD`,
`SEMIBASE_READER_PASSWORD`.
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

# SemiBase

SemiBase is the deployable PostgreSQL service for the semiconductor-tools installation. It
provisions and configures the instance that hosts the Simple-Scada 2 archive: the SCADA writes
`trends`/`messages` into it, SemiPlot and future tools read from it. The deliverables are a
Windows setup, `SemiBase-Setup.exe` (Inno Setup, `Installer/`), which installs the vendored EDB
PostgreSQL 17 when no Windows service runs PostgreSQL and runs `semibase site`; the CLI binary
`semibase.exe` it carries; a container image carrying the binary's Linux build; and the
instance's architecture docs.
Deployment target: Windows. Language: Go (module `github.com/Semiteq/SemiBase`), driver
`pgx/v5`, the five `sql/semiplot_*.sql` files and `sql/trends.sql` embedded via `go:embed`.
Entry point: `cmd/semibase`. The module is pure Go (`pgx/v5`, plus `golang.org/x/sys` for the
Windows service list and restart) and compiles for any GOOS; Linux builds serve the
containerised test bench of consumers (SemiPlot runs `bench` against an ephemeral `postgres:17`
container). All commands run from the repository root.

## Build

```powershell
go build -o semibase.exe ./cmd/semibase
go build -ldflags "-X main.revision=<rev>" -o semibase.exe ./cmd/semibase   # release: embed the revision
```

Without ldflags, `version` falls back to the VCS revision recorded by the Go toolchain.

The setup needs Inno Setup 6.7.3 (`winget install --id JRSoftware.InnoSetup --exact --version
6.7.3`, the version its command-line and silent-mode behaviour was measured on) and the pinned EDB
installer, which the fetch script downloads and checks against its SHA256:

```powershell
& "$env:ProgramFiles\Git\bin\bash.exe" Installer/fetch-postgresql.sh   # a bare bash may be WSL's
iscc /DAppVersion=0.0.0-dev /DFileVersion=0.0.0 "/DSemibaseExe=$PWD\semibase.exe" Installer\SemiBase.iss
```

The output is `Installer/Output/SemiBase-Setup.exe`. Under Git Bash, prefix `iscc` with
`MSYS2_ARG_CONV_EXCL='*'`, or MSYS rewrites the `/D` arguments as paths. A pin bump edits the
fetch script's constants alone (`docs/architecture/installer.md#the-pinned-payload`).

The container image, built the way both workflows build it: `--platform linux/amd64`, and a
context of one file (`docs/architecture/provisioning.md#distribution`):

```powershell
$env:CGO_ENABLED = "0"; $env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w -X main.revision=v0.0.0-dev" -o image/semibase ./cmd/semibase
docker build --platform linux/amd64 -f Dockerfile -t semibase:dev image
docker run --rm semibase:dev version
```

A `v*` tag push runs `.github/workflows/release.yml`, which attaches the Linux binary and the
setup to a GitHub release and pushes `ghcr.io/semiteq/semibase:vX.Y.Z` and, for the newest
release tag, `:latest` (`docs/architecture/provisioning.md#distribution`). The release is named
after the tag, and the whole annotated tag message becomes the body, verbatim; blank lines
survive, so write it in sections and give it no title line.

```bash
git tag -a v0.2.0 -F - <<'EOF'
**New Features**

* what the user can now do, lowercase [#12](https://github.com/Semiteq/SemiBase/pull/12)

**Fixes**

* what stopped going wrong [#13](https://github.com/Semiteq/SemiBase/pull/13)
EOF
git push origin v0.2.0
```

Bold section labels, not `###` headings: git's default tag cleanup deletes every line
starting with `#`, which would silently strip the structure out of the annotation.
A lightweight tag still releases, and the body falls back to `Release <tag>`.

Run:

```powershell
.\semibase.exe --help                     # commands: site | bench | engine | version
.\semibase.exe site                       # an installation machine: tuning, then everything else
.\semibase.exe site --restart-service postgresql-x64-17   # also restart the service when settings wait
.\semibase.exe bench --port 15432 --database semiplot_dev --expected-major 17
.\semibase.exe engine --expected-major 17 # the service Setup would configure, as key=value lines; unelevated
.\semibase.exe version                    # print the build revision
# the setup, in an elevated PowerShell, on the conventional passwords unless SEMIBASE_SUPER_PASSWORD
# or SEMIBASE_WRITER_PASSWORD is set: exit 0 provisioned, 7 refused; add /LANG=english or
# /LANG=russian to fix the language; logs in C:\Program Files\SemiBase\Logs
(Start-Process .\Installer\Output\SemiBase-Setup.exe -Wait -PassThru `
  -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART').ExitCode
```

`site` and `bench` differ in one thing: `site` applies the `ALTER SYSTEM` server settings,
`bench` does not. Both create the database, the roles, the grants, the five `semiplot_*` tables,
`semiplot_register_new_pens()` and `public.trends`, and end with the `semiplot` and `scada_writer` access checks; a
failed check is a non-zero exit (`docs/architecture/provisioning.md`). `engine` provisions
nothing: it reads the Windows service list and the data folders and prints the state Setup acts
on (`docs/architecture/installer.md#semibase-engine`).

Rules the code depends on:

- Passwords come from a flag, then `SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD`,
  `SEMIBASE_PLOT_PASSWORD`, then the convention that a role's password is its name; never from a
  file (`docs/architecture/provisioning.md#passwords`).
- `--superuser` must name a real superuser: every superuser connection sets the five
  statement-logging parameters, which are superuser-only, to keep password statements out of the
  server log (`docs/architecture/provisioning.md#invariants-the-provision-package-owns`).
- The `semiplot` `UPDATE` grant on `semiplot_tags` is column-level (the nine settings columns,
  never `id`), so `has_table_privilege(..., 'UPDATE')` answers false for that table and no check
  may ask it
  (`docs/architecture/provisioning.md#the-roles`).

## Test

```powershell
go test ./...
go vet ./...
golangci-lint run
```

golangci-lint is pinned to 2.12.2 (`winget install GolangCI.golangci-lint --version 2.12.2`);
config in `.golangci.yml`. Unit tests live beside the source (`cmd/semibase/*_test.go`,
`internal/engine/*_test.go`, `internal/provision/*_test.go`), table-driven, and touch no database.

CI (`.github/workflows/ci.yml`) runs `go build` and `go test -race` on `windows-latest` and
`ubuntu-latest` for every push to `master` and every pull request, unless every changed file is
on its `paths-ignore` list, and the lint in the Windows `build` job only. The database checks run
there too: the service restart in `build`, `site` and `bench` against PostgreSQL 17 and 14
containers in `linux`, the setup end to end in `installer`. Each step and what it requires:
`docs/architecture/verification.md`.

## Format

```powershell
gofmt -w .    # run before presenting changes; gofmt is authoritative
```

## Layout

```
cmd/semibase/        CLI entry point: command dispatch, flags, usage text, the engine output
internal/engine/     the engine command's detection: engine.go (data-folder facts, conf port
                     resolution, Classify), services_windows.go (the query-only service list,
                     the pg_ctl command line, DataDirectoryFlag), services_other.go (the
                     non-Windows refusal)
internal/provision/  one file per phase behind the two commands: config.go (ALTER SYSTEM and
                     the optional service restart, whose service control lives in
                     service_windows.go and service_other.go), create.go
                     (db/roles/grants/trends), schema.go (the semiplot schema files, their
                     grants, the pen-registration function's grant and its owner's
                     registrarStatements), check.go (semiplot access). provision.go
                     holds Options, Site/Bench and the connection plumbing they share;
                     console.go the step/ok/warn/note output helpers
sql/                 embedded DDL: the five semiplot_* files (ours: the tables, the
                     pen-registration function, the time markers, the version stamp) and trends
                     (the vendor's shape)
Installer/           SemiBase.iss (the Inno Setup script: the engine run, the refusals, EDB
                     install, the site run, all in PrepareToInstall), fetch-postgresql.sh (the
                     pinned EDB payload into the git-ignored payload/; ISCC writes Output/),
                     make-icon.py and SemiBase.ico
.github/             workflows/ (ci, release), actions/build-setup (fetch, Inno Setup, iscc for
                     both), scripts/ (CI helpers; installer-case.ps1 for the setup's cases)
Dockerfile           the container image: FROM scratch, the Linux binary alone
docs/images/         the README's screenshots
docs/architecture/   agent-facing design docs in English
docs/plans/          dated implementation plans
```

## Docs

- `README.md`: the only human doc, Russian, for the commissioning engineer: what SemiBase is,
  requirements, the setup, the passwords, the recoveries the setup's screens cannot carry,
  removal. Its screenshots live in `docs/images/`. It must never mention or link
  `docs/architecture/`.
- `docs/architecture/`: agent-facing design (machine-readable), English. Start at `README.md`.
- `docs/plans/`: dated implementation plans (`YYYYMMDD-<name>.md`); completed ones
  in `docs/plans/completed/`.
- The archive schema itself belongs to the SCADA and is documented in the SemiPlot repository
  (`docs/architecture/scada-archive.md` there); this repository documents the instance around it.

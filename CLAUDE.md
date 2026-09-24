# SemiBase

SemiBase is the deployable PostgreSQL service for the semiconductor-tools installation. It
provisions and configures the instance that hosts the Simple-Scada 2 archive: the SCADA writes
`trends`/`messages` into it, SemiPlot and future tools read from it. The deliverable is one
CLI binary, `semibase.exe`, a container image carrying its Linux build, plus the instance's
architecture docs.
Deployment target: Windows. Language: Go (module `github.com/Semiteq/SemiBase`), driver
`pgx/v5`, the four `sql/semiplot_*.sql` files and `sql/trends.sql` embedded via `go:embed`. Entry point:
`cmd/semibase`. The module is pure `pgx` and compiles for any GOOS; Linux builds serve the
containerised test bench of consumers (SemiPlot runs `bench` against an ephemeral `postgres:17`
container). All commands run from the repository root.

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
`semibase_<version>_<goos>_amd64[.exe]` to the GitHub release — no extension on the Linux
artifact, an ELF is executable by its mode bit. `-X main.revision=<tag>` is embedded, so
`semibase version` prints the tag.

The same job then pushes `ghcr.io/semiteq/semibase:latest` and `:vX.Y.Z` — the root
`Dockerfile`, `FROM scratch`, a copy of the Linux artifact as the only file. The image is built
and smoke-run before the release is created, so a bad `Dockerfile` aborts with nothing
published, and pushed after it, since consumers track `:latest` and still download the release
assets. "Latest" is one decision made once in the `Read the tag` step — the tag has no `-`
suffix **and** it is the newest release tag in the repository (`git tag -l --sort=-v:refname`
over plain `vN.N.N` tags) — and both the release page's `prerelease`/`make_latest` and the
`:latest` push read it, so they cannot disagree. A prerelease publishes its version tag alone,
and re-running an old tag's workflow cannot walk either "latest" backwards. Build it by hand the
way both workflows do — `--platform linux/amd64`, since `FROM scratch` otherwise stamps the
builder's architecture into the manifest, and a context of one file, so the Dockerfile's
`COPY semibase` finds it and nothing else reaches the daemon:

```powershell
$env:CGO_ENABLED = "0"; $env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w -X main.revision=v0.0.0-dev" -o image/semibase ./cmd/semibase
docker build --platform linux/amd64 -f Dockerfile -t semibase:dev image
docker run --rm semibase:dev version
```

The release is named after the tag and nothing else. The whole annotated tag message becomes
the body, verbatim; blank lines survive, so write it in sections and give it no title line.

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
A lightweight tag still releases — the body falls back to `Release <tag>`.

## Test

```powershell
go test ./...
go vet ./...
golangci-lint run
```

golangci-lint is pinned to 2.12.2 (`winget install GolangCI.golangci-lint --version 2.12.2`);
config in `.golangci.yml`. CI (`.github/workflows/ci.yml`) runs `go build`, `go test -race`,
and the same lint on `windows-latest` and `ubuntu-latest` for every push and pull request;
the Linux job also provisions a `postgres:17-alpine` service container by running `site`
twice — the tuning path and the idempotency check in one step.
It then runs `bench` against a `postgres:14-alpine` container — the version floor, and the only
place `REVOKE CREATE ON SCHEMA public FROM PUBLIC` bites, because 15 and later withhold that grant
by engine default — provisions a third container with `bench` as a `postgres` image init script
over the unix socket, and builds the container image, so a broken `Dockerfile` fails on the pull
request rather than at tag time. CI pushes nothing. Between the 17 and the 14 steps, the
`Register pens from the keys SCADA wrote` step writes `trends` rows as `scada_writer`, calls
`semiplot_register_new_pens()` twice as `semiplot` and requires 3, then 0, and the three rows it
added: the only place the function body runs over a non-empty archive. It needs `psql` on the
runner; to run it locally, point `psql` at a container.

Unit tests live beside the source (`cmd/semibase/*_test.go`, `internal/provision/*_test.go`),
table-driven. There are no database-touching tests in the repository; the integration checks are
the `semiplot` access check both commands run at the tail and the four CI database steps above,
against a live server.

## Format

```powershell
gofmt -w .    # run before presenting changes; gofmt is authoritative
```

## Run

```powershell
.\semibase.exe --help                     # commands: site | bench | version
.\semibase.exe site                       # an installation machine: tuning, then everything else
.\semibase.exe bench --port 15432 --database semiplot_dev --expected-major 17
.\semibase.exe version                    # print the build revision
```

`site` and `bench` differ in one thing: `site` applies the `ALTER SYSTEM` memory constants,
`bench` does not. Both create the database, the roles, the grants, the four SemiPlot tables
(`semiplot_tags`, `semiplot_groups`, `semiplot_pen_groups`, `semiplot_meta`), the
`semiplot_register_new_pens()` function and `public.trends`, and both end by actually reading
`public.trends` and `semiplot_meta.schema_version` as `semiplot`, actually calling
`semiplot_register_new_pens()`, proving it ignores a temporary `trends` of its caller, and writing
`semiplot_groups`, `semiplot_pen_groups` and the settings columns of `semiplot_tags` as `semiplot`
in a rolled-back transaction, requiring 42501 for `INSERT`, `DELETE` and `UPDATE ... SET id` on
`semiplot_tags` in the same transaction, and checking that `PUBLIC` holds no `EXECUTE` on
`semiplot_register_new_pens()`, and that `semiplot` holds no `INSERT`, `UPDATE` or `DELETE`
on the archive or on `semiplot_meta` and no `CREATE` on schema `public` (revoked from `PUBLIC` by
`create`, since 14 still grants it). A failed check is a non-zero exit.

The `semiplot` grant on `semiplot_tags` is column-level: `SELECT` and `UPDATE` on the eight settings
columns, never `id`, so `has_table_privilege(..., 'UPDATE')` answers false for that table and no
check may ask it.

Four roles take part: the superuser the tool connects as (it owns the `semiplot_*` tables),
`scada_writer`, `semiplot`, and `semiplot_registrar`, a `NOLOGIN` role that owns
`semiplot_register_new_pens()` and holds only what its body needs. The function is `SECURITY
DEFINER` with `search_path = pg_catalog, pg_temp` and a schema-qualified body; the reasons are in
`docs/architecture/provisioning.md#registering-new-pens`. `scada_writer` is trusted: the grants bound
the operator acting through `semiplot`, not the SCADA (`docs/architecture/provisioning.md#trust`).
`semiplot_meta.schema_version` is `1` in
this release and is a floor: a viewer refuses a database below the version it needs and accepts one
above.

Passwords come from flags, env, or a `.env` file in the working directory (flag > env > `.env`;
template `.env.example`): `SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD`,
`SEMIBASE_PLOT_PASSWORD`. The writer and `semiplot` passwords are needed on a first run, which
creates those roles. Otherwise the superuser password alone carries a run, and the `semiplot` password only decides
whether the run also tests that role's TCP login. `public.trends` is created under
`SET ROLE scada_writer` on the superuser connection, so the table's owner is the role the SCADA
writes with and no `pg_hba.conf` line has to admit a `scada_writer` login.

## Layout

```
cmd/semibase/        CLI entry point: command dispatch, flags, usage text
internal/provision/  one file per phase behind the two commands: config.go (ALTER SYSTEM),
                     create.go (db/roles/grants/trends), schema.go (the semiplot schema files,
                     their grants, the pen-registration function's grant and its owner's
                     registrarStatements), check.go (semiplot access). provision.go
                     holds Options, Site/Bench and the connection plumbing they share
sql/                 embedded DDL: the four semiplot_* files (ours: the tables, the
                     pen-registration function, the version stamp) and trends (the vendor's shape)
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
  `sql/trends.sql` is a transcription of the vendor's shape, created once and never altered —
  whether the SCADA tolerates finding it already there is an unverified assumption, stated with
  its experiment in `docs/architecture/provisioning.md`.

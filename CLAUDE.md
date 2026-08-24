# SemiBase

SemiBase is the deployable PostgreSQL service for the semiconductor-tools installation. It
provisions and configures the instance that hosts the Simple-Scada 2 archive: the SCADA writes
`trends`/`messages` into it, SemiPlot and future tools read from it. The deliverable is one
CLI binary, `semibase.exe`, a container image carrying its Linux build, plus the instance's
architecture docs.
Deployment target: Windows. Language: Go (module `github.com/Semiteq/SemiBase`), driver
`pgx/v5`, `sql/semiplot_tags.sql` and `sql/trends.sql` embedded via `go:embed`. Entry point:
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
twice — the tuning path and the idempotency check in one step — provisions a second container
with `bench` as a `postgres` image init script over the unix socket, and builds the container
image, so a broken `Dockerfile` fails on the pull request rather than at tag time. CI pushes
nothing.

Unit tests live beside the source (`cmd/semibase/*_test.go`, `internal/provision/*_test.go`),
table-driven. There are no database-touching tests in the repository; the integration check is
the reader-access check both commands run at the tail, against a live server.

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
`bench` does not. Both create the database, the roles, the grants, `semiplot_tags` and
`public.trends`, and both end by actually reading `public.trends` as `semiplot_reader` and
checking that the same role holds no `INSERT`. A failed check is a non-zero exit.

Passwords come from flags, env, or a `.env` file in the working directory (flag > env > `.env`;
template `.env.example`): `SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD`,
`SEMIBASE_READER_PASSWORD`. The writer and reader passwords are needed only on a first run,
which creates those roles; `public.trends` is created under `SET ROLE scada_writer` on the
superuser connection, so the table's owner is the role the SCADA writes with and no
`pg_hba.conf` line has to admit a `scada_writer` login.

## Layout

```
cmd/semibase/        CLI entry point: command dispatch, flags, usage text
internal/provision/  the phases behind the two commands: config (ALTER SYSTEM), create
                     (db/roles/grants/semiplot_tags/trends), check (reader access)
sql/                 embedded DDL: semiplot_tags (ours) and trends (the vendor's shape)
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

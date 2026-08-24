# Provisioning

One tool, `semibase.exe` (Go, `cmd/semibase`), provisions development benches and the
production instance alike. That is the point: a tool that has created the development database
many times is proven; a tool written for commissioning and run once, on site, on the day it
matters, is a liability.

The binary is self-contained: `pgx/v5` speaks the wire protocol itself, over TCP or a unix
socket, so neither `psql.exe` nor any runtime needs to be present on the target machine.
`sql/semiplot_tags.sql` is embedded at build time. Installing the PostgreSQL engine itself is one documented `winget`
line, deliberately outside the tool — OS package management is not database provisioning.

## Distribution

A tagged release carries three artifacts, all built from one commit by
`.github/workflows/release.yml`:

| Artifact | Form | Consumer |
| --- | --- | --- |
| `semibase_<version>_windows_amd64.exe` | GitHub release asset | Commissioning on the installation machine |
| `semibase_<version>_linux_amd64` | GitHub release asset | Provisioning a bench from a Linux host or CI runner |
| `ghcr.io/semiteq/semibase:latest` and `:vX.Y.Z` | Container image | Bench images that layer the binary onto `postgres:17-alpine` |

The image is `FROM scratch` and holds one file, `/semibase` — the same bytes as the Linux
release asset, copied rather than built a second time. It carries no shell, no libc and no CA
certificates, because nothing runs it as a base: the consumer copies the binary out
(`COPY --from=ghcr.io/semiteq/semibase:latest /semibase /semibase`) onto its own postgres image
and runs it as an init script over the unix socket. The init script belongs to that consumer,
not to the image. The `ENTRYPOINT` is there so
`docker run ghcr.io/semiteq/semibase:latest version` answers.

`latest` is the tag consumers track, and that is deliberate. A delivered installation updates
neither the database service nor the viewer on its own, so the only pair ever newly deployed is
the newest SemiBase with the reader as it stands; a bench pinned to an old image would test a
pair nobody runs. The `vX.Y.Z` tag is immutable and exists to place blame and to reproduce a
failing pair. A prerelease tag (`v1.2.3-rc1`) publishes its version tag only and leaves `latest`
where it is.

The image is pushed after the GitHub release exists, never before: consumers still download the
release assets, so `latest` must never name a version whose assets are missing.

The GHCR package is public, because consumers pull it anonymously from their own CI. The first
push creates it private; that is switched once, in the package's settings, and the workflow has
no say in it.

## Commands

| Command | What it does | When |
| --- | --- | --- |
| `config` | Writes the `ALTER SYSTEM` deltas from `configuration.md` and applies them with `pg_reload_conf()`; prints the server's pending-restart list (`shared_buffers` is the one entry, active after the next service restart or reboot) | Production. Never against a shared dev server — it retunes the whole instance; a throwaway bench container needs no tuning |
| `create` | Creates the archive database, the roles, the grants, the default privileges, and `semiplot_tags` | Before the SCADA's first start |
| `verify` | Proves the reader access chain against the tables the writer created | After the SCADA's first start |
| `all` | `config` + `create` + `verify`; `verify` reports "writer has not run" as a distinct state, not a failure | Fresh instance |

`version` (or `--version`) prints the build revision: the ldflags value
(`-X main.revision=...`) when a release sets it, otherwise the VCS revision from
`debug.ReadBuildInfo` with a `-dirty` suffix for a modified tree.

Every step checks before it creates, so re-running any command is safe. Role passwords are set
only when the corresponding flag or environment variable is present (`SEMIBASE_SUPER_PASSWORD`,
`SEMIBASE_WRITER_PASSWORD`, `SEMIBASE_READER_PASSWORD`); a `.env`
file in the working directory is read as the lowest-precedence source (flag > environment >
`.env`; `.env.example` is the template, `.env` is gitignored). A run without passwords leaves
existing credentials untouched. Flag usage output shows only the environment variable names,
never their values.

## The connection target

`--host` takes a hostname or a unix socket directory, and the split is the driver's own:
`pgconn.isAbsolutePath` calls a value a socket directory when it starts with `/` or is a
drive-letter path such as `C:\pgsock`. A socket directory cannot ride in the URL authority —
its slashes would end the authority — so the connection string moves it, and the port with it,
into the query, percent-encoded:

```
postgres://postgres:***@/scada_archive?host=%2Fvar%2Frun%2Fpostgresql&port=5432
```

pgx then dials `<directory>/.s.PGSQL.<port>` on the `unix` network. Widening the test past pgx's
would be a trap, not a courtesy: libpq also accepts a leading `@` for Linux's abstract namespace,
but pgx resolves such a host as a TCP name, so `semibase` leaves it on the TCP path.

The socket form is what lets `create` run as an init script in the official `postgres` image,
whose entrypoint serves `/docker-entrypoint-initdb.d/` from a temporary server with
`listen_addresses` set to empty — reachable over the socket only. The Linux CI job runs exactly
that: it layers the freshly built binary onto `postgres:17-alpine` behind such an init script and
asserts the container reaches a state where the reader can query.

Messages name the socket file rather than a URL —
`/var/run/postgresql/.s.PGSQL.5432 (scada_archive)`. The database is in parentheses because a
slash would make the socket file read as a directory and send the operator to a path that can
never exist.

## Invariants the provision package owns

The package that runs the SQL owns its guards; the CLI is a thin dispatcher.

- **Database-name validation.** `Options.Validate()` checks `Database` against
  `^[a-z_][a-z0-9_]*$` and runs first in every phase (`Config`, `Create`, `verify`). The
  pattern lives only in `internal/provision`; the CLI calls the same method for the friendly
  early error.
- **Identifier quoting.** Every interpolation of the database name into DDL goes through
  `pgx.Identifier{...}.Sanitize()`. The connection URL is not sanitized — `url.URL` escapes
  the path itself, and quoted identifiers do not belong in it.
- **Literal escaping.** `connect` issues `SET standard_conforming_strings = on` for the
  session, which makes the quote-doubling `escapeLiteral` sufficient in every server mode,
  including backslash-containing passwords.
- **Cancellable execution.** Every phase honors its context: cancellation stops the
  in-flight query through pgx instead of killing the process mid-DDL. The context itself —
  `signal.NotifyContext(..., os.Interrupt)` plus the five-minute per-command timeout
  (`phaseTimeout`) — is owned by the CLI dispatcher in `cmd/semibase`.
- **Pending-restart reporting.** The tool never touches the service manager. `config` applies
  its deltas with `pg_reload_conf()` and reads `pg_settings.pending_restart` back from the
  server; `verify` warns while any setting still waits. The operator restarts the service or
  reboots the machine — the one manual step, and the tool says so instead of doing it.

## The reader-access chain

`semiplot_reader` needs `SELECT` on `trends` and `messages` — tables that do not exist until the
SCADA has run once. A plain `GRANT` cannot be issued on a table that is not there, so `create`
sets:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer IN SCHEMA public
    GRANT SELECT ON TABLES TO semiplot_reader;
```

This must be in place **before** the writer first starts; tables the writer creates afterwards are
readable automatically. If the writer ran first, the repair is a one-time
`GRANT SELECT ON ALL TABLES IN SCHEMA public` plus the default-privileges statement for the
partitions still to come — `verify` detects this state and says so.

On PostgreSQL 15 and later the `public` schema no longer grants `CREATE` to everyone, so
`create` grants it to `scada_writer` explicitly. On 14 the grant is redundant and harmless.

## What `verify` proves

1. `public.trends` and `public.messages` exist — the writer has run.
2. `semiplot_reader` holds `SELECT` and does not hold `INSERT` on `trends`.
3. When the reader password is supplied, an actual connection as `semiplot_reader` reads one row —
   the whole chain, not just the catalog view of it.
4. The default partition is empty. Rows in it mean a day partition was missing at write time,
   which is a writer-side fault worth surfacing during commissioning.
5. No setting waits for a service restart (`pg_settings.pending_restart`) — a warning, not a
   failure, so a pending `shared_buffers` never blocks the access-chain checks.

## Development benches

The bench is an ephemeral vanilla `postgres:17-alpine` container. The consumer's test fixture
starts it and provisions it by running `create --database semiplot_dev --expected-major 17` —
the same code path production takes, which is what keeps the fixture from drifting into a
hand-written copy of the grants. `config` is not part of the bench: a throwaway container has
nothing to tune. 14 remains the floor `create` accepts through `--expected-major`; the bench
runs the pinned major, 17.

Two rules make the bench exercise what production exercises:

- The bench seeder connects **as `scada_writer`** and creates the archive tables itself, the way
  the SCADA would. This is what makes the default-privileges chain a daily-tested path instead of
  a commissioning-day surprise.
- Bench integration tests read **as `semiplot_reader`**, never as a superuser, so a broken grant
  fails a test today.

The seeder and the tests live in the SemiPlot repository; this repository only promises them a
correctly provisioned database.

## Passwords

The tool takes passwords through flags or environment variables and never stores them. The reader
credential ends up in plaintext in client configuration files by design — it grants reading
process history and nothing more. The writer credential is recorded wherever the installation
keeps its commissioning records, outside this repository.

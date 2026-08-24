# Provisioning

One tool, `semibase.exe` (Go, `cmd/semibase`), provisions development benches and the
production instance alike. That is the point: a tool that has created the development database
many times is proven; a tool written for commissioning and run once, on site, on the day it
matters, is a liability.

The binary is self-contained: `pgx/v5` speaks the wire protocol itself, over TCP or a unix
socket, so neither `psql.exe` nor any runtime needs to be present on the target machine.
`sql/semiplot_tags.sql` and `sql/trends.sql` are embedded at build time. Installing the PostgreSQL
engine itself is one documented `winget` line, deliberately outside the tool — OS package management
is not database provisioning.

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
`docker run ghcr.io/semiteq/semibase:latest version` answers, and it is what the release job
runs against the built image before pushing it — the one execution of the shipped bytes.

The manifest says `linux/amd64` because every build passes `--platform linux/amd64`. `FROM
scratch` otherwise takes the platform of whatever machine built it, so the manifest would state
the runner rather than the payload. A `FROM --platform=` pin does not substitute for the flag —
it sets the base stage's platform, and scratch carries no config for the output to inherit — so
the release job reads the built image back and fails if the manifest says anything else.

`latest` is the tag consumers track, and that is deliberate. A delivered installation updates
neither the database service nor the viewer on its own, so the only pair ever newly deployed is
the newest SemiBase with the reader as it stands; a bench pinned to an old image would test a
pair nobody runs. The `vX.Y.Z` tag is immutable and exists to place blame and to reproduce a
failing pair.

"Latest" only ever moves forward, and that is enforced rather than assumed. One decision, made
once in the release job, drives all of it: the tag is a release tag (no `-` suffix) **and** it is
the newest release tag in the repository. That answer sets the GitHub release's `prerelease` and
`make_latest` fields and gates the `:latest` push alike, so `/releases/latest` and `:latest`
cannot name different versions. Without the newest test, re-running an old tag's workflow from
the Actions UI, or pushing two tags at once, walks "latest" backwards with no signal to anyone
tracking it. A skipped `:latest` push says so in the job log; it is never silent.

The newest tag is found with git's version ordering (`git tag -l --sort=-v:refname`) over plain
`vN.N[.N…]` tags. That ordering compares the numeric fields as numbers, which a string compare
does not — `v0.10.0` outranks `v0.9.0`. Narrowing the candidates to unsuffixed tags keeps
prereleases out of the comparison, and with them git's own placement of `v1.0.0-rc1` *after*
`v1.0.0` unless `versionsort.suffix` is configured. All tags have to be in the checkout for any
of it to mean anything; `fetch-depth: 0` is what puts them there, and the job fails loudly if
the tag being released is not among them.

The image is pushed after the GitHub release exists, never before: consumers still download the
release assets, so `latest` must never name a version whose assets are missing. The image is
*built* before it, because a bad copy or a bad `Dockerfile` has to abort while there is still
nothing published to withdraw.

The GHCR package is public, because consumers pull it anonymously from their own CI. The first
push creates it private; that is switched once, in the package's settings, and the workflow has
no say in it.

## Commands

The tool is a setup script: it brings an instance to a known state and exits. It is run once, at
commissioning, and nobody comes back to run a second command by hand. So the surface names the two
situations it is run in, not the phases it runs through.

| Command | What it does | When |
| --- | --- | --- |
| `site` | The memory tuning, then everything else | An installation machine |
| `bench` | Everything else, without the tuning | A throwaway container: a consumer's test bench or a development database |

The two differ in exactly one thing, the tuning. The rest is identical, which is what makes a bench
a test of what a site runs:

| Step | `site` | `bench` |
| --- | --- | --- |
| `ALTER SYSTEM` constants from `configuration.md`, applied with `pg_reload_conf()` | yes | no — the constants are sized for an installation machine and mean nothing to a container |
| Archive database, both roles, grants, default privileges, `semiplot_tags` | yes | yes |
| `public.trends` with the `tpdefault` partition, created as `scada_writer` | yes | yes |
| Reader-access checks at the tail | yes | yes |
| Pending-restart warning at the tail | yes | yes — `bench` tunes nothing, but it runs against existing servers too, and one a `site` run tuned can still be waiting for its restart |

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

The socket form is what lets `bench` run as an init script in the official `postgres` image,
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
  `^[a-z_][a-z0-9_]*$` and runs first in every phase (`config`, `create`, `check`). The
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
- **Pending-restart reporting.** The tool never touches the service manager. The tuning phase
  applies its deltas with `pg_reload_conf()` and reads `pg_settings.pending_restart` back from
  the server; both commands read it again as the **last** statement of the tail, after the
  access checks, so the last line either prints names a pending `shared_buffers`. The operator
  restarts the service or reboots the machine — the one manual step, and the tool says so
  instead of doing it.

## The reader-access chain

`semiplot_reader` needs `SELECT` on `trends` and on every day partition the SCADA creates later —
objects that do not exist when the grant has to be decided. A plain `GRANT` cannot be issued on a
table that is not there, so the create phase sets:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer IN SCHEMA public
    GRANT SELECT ON TABLES TO semiplot_reader;
```

This is in place **before** `public.trends` is created and before the writer first starts; tables
`scada_writer` creates afterwards are readable automatically. If a table was created before the
statement ran, the repair is a one-time `GRANT SELECT ON ALL TABLES IN SCHEMA public` for the
tables that exist plus the default-privileges statement for the partitions still to come, then
another run of the tool — the tail check detects that state and prints all three.

On PostgreSQL 15 and later the `public` schema no longer grants `CREATE` to everyone, so
`create` grants it to `scada_writer` explicitly. On 14 the grant is redundant and harmless.

## The archive table

`public.trends` is created by this tool, by both commands, from `sql/trends.sql` — the vendor's
shape, transcribed from a customer archive dump. Three properties of how it is created carry the
weight:

- **The role, more than the shape.** The table is created as `scada_writer`, because the reader's
  `SELECT` has to arrive through the default privileges set for that role a moment earlier. A
  superuser-owned table would give the reader access for a different reason than a site gets it,
  and a bench would then test a different thing from production. The role is assumed with
  `SET ROLE` on the superuser connection, not through a `scada_writer` login: measured on
  `postgres:17-alpine`, both routes leave the same `relowner` and the same `relacl`
  (`{scada_writer=arwdDxtm/scada_writer,semiplot_reader=r/scada_writer}`), while a login also has
  to be admitted by `pg_hba.conf` — and `local all all peer`, the default on Debian, Ubuntu and
  RHEL, refuses it on a unix socket. Creating the table therefore needs no writer password; the
  writer password is needed only on a first run, to create the role itself.
- **`messages` is not created.** Nothing we ship reads it, and every object we create is a surface
  that can drift from the vendor.
- **Day partitions are not created.** `tpYYYYmMMdDD` belongs to the SCADA on a site and to the
  consumer's seeder on a bench. Only the `tpdefault` catch-all is created, so a row written while
  no day partition exists lands somewhere instead of failing.

Existence is checked first (`to_regclass('public.trends')`), so a second run leaves the table
untouched — including a table the SCADA has since altered. Untouched is not unread: on that path
the run reads back what it promises. `public.tpdefault` must be there, and its absence is a
failure with the one `CREATE TABLE ... PARTITION OF ... DEFAULT` that repairs it, because a table
without it rejects any row no day partition covers. Its row count is then reported — always zero
on the create path, but on a table that may be months old a non-empty `tpdefault` is the only
signal that a day partition was missing at write time, so it is a warning rather than a failure.

## Assumption: the SCADA meeting an existing `trends`

**Unverified.** Simple-Scada 2's behaviour when its archive target already exists is undocumented
and unmeasured. The expectation is that it writes into the existing table the way it does after a
reconnect, because the shape it finds is the shape it makes.

The experiment that settles it: set `log_statement = 'all'`, start a Simple-Scada project once
against a database provisioned by `semibase site`, and read from the PostgreSQL log the DDL the
SCADA issues. If it creates the table unconditionally, or alters what it finds, this section is
replaced by what was observed and `sql/trends.sql` is reconsidered.

## What the tail checks prove

Both commands end with them, and a failure is a non-zero exit — the run did not reach the state it
promises. They are knowable at exit precisely because this tool creates `public.trends` itself:

1. **`semiplot_reader` reads `public.trends`.** The read itself, not a catalog bit:
   `SET ROLE semiplot_reader` on the superuser connection, then
   `SELECT count(*) FROM (SELECT 1 FROM public.trends LIMIT 1) probe`. `has_table_privilege`
   cannot carry this check — after `REVOKE USAGE ON SCHEMA public FROM public` it still answers
   `t` while every read the reader issues fails with `permission denied for schema public`
   (measured). The catalog bit is asked only when the read has already failed, to split a missing
   table grant from a schema the reader cannot enter, and each answer prints its own repair.
   `SET ROLE` rather than a login for the `pg_hba.conf` reason above.
2. **The `semiplot_reader` login reads it too**, over TCP, when the reader password is present in
   the run. This is the half `SET ROLE` cannot reach: `pg_hba.conf` admitting the role and the
   password a consumer will carry. It is skipped, with a printed note, when no reader password
   was given, and on a socket host — `peer` is the platform default there and no consumer reads
   over the socket.
3. `semiplot_reader` does not hold `INSERT` on `public.trends` — the reader is read-only. This one
   stays a catalog question: a failed `INSERT` proves nothing about the next one.
4. No setting waits for a service restart (`pg_settings.pending_restart`) — a warning, not a
   failure, so a pending `shared_buffers` never blocks the access checks. Both commands read it,
   last, after the access checks.

## Development benches

The bench is an ephemeral vanilla `postgres:17-alpine` container. The consumer's test fixture
starts it and provisions it by running `bench --database semiplot_dev --expected-major 17` — the
same code path a site takes minus the tuning, which is what keeps the fixture from drifting into a
hand-written copy of the grants. 14 remains the floor accepted through `--expected-major`; the
bench runs the pinned major, 17.

Two rules make the bench exercise what production exercises:

- `public.trends` is created **as `scada_writer`**, by both commands, so the default-privileges
  chain is a daily-tested path instead of a commissioning-day surprise.
- Bench integration tests read **as `semiplot_reader`**, never as a superuser, so a broken grant
  fails a test today.

The consumer's seeder fills the archive and creates the day partitions its data needs; it and the
tests live in the SemiPlot repository. This repository promises them a provisioned database with an
empty `public.trends` in it.

## Passwords

The tool takes passwords through flags or environment variables and never stores them. The reader
credential ends up in plaintext in client configuration files by design — it grants reading
process history and nothing more. The writer credential is recorded wherever the installation
keeps its commissioning records, outside this repository.

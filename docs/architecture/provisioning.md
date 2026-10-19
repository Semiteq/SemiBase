# Provisioning

One tool, `semibase.exe` (Go, `cmd/semibase`), provisions development benches and the
production instance alike. That is the point: a tool that has created the development database
many times is proven; a tool written for commissioning and run once, on site, on the day it
matters, is a liability.

The binary is self-contained: `pgx/v5` speaks the wire protocol itself, over TCP or a unix
socket, so neither `psql.exe` nor any runtime needs to be present on the target machine.
The SQL is embedded at build time: `sql/trends.sql` for the archive table, and
`sql/semiplot_tags.sql`, `sql/semiplot_register.sql`, `sql/semiplot_groups.sql`,
`sql/semiplot_markers.sql` and `sql/semiplot_meta.sql` for the SemiPlot configuration schema. Installing the PostgreSQL
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
the newest SemiBase with the viewer as it stands; a bench pinned to an old image would test a
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
| `site` | The server settings, then everything else | An installation machine |
| `bench` | Everything else, without the tuning | A throwaway container: a consumer's test bench or a development database |

The two differ in exactly one thing, the tuning. The rest is identical, which is what makes a bench
a test of what a site runs:

| Step | `site` | `bench` |
| --- | --- | --- |
| `ALTER SYSTEM` deltas from `configuration.md`, applied with `pg_reload_conf()` | yes | no: the memory constants are sized for an installation machine and mean nothing to a container, and a loopback-only `listen_addresses` would shut out the clients that reach a bench through a published port |
| Restarting the Windows service the deltas wait for ([The service restart](#the-service-restart)) | only with `--restart-service` | no: `bench` does not register the flag |
| Archive database, the three created roles, grants, default privileges | yes | yes |
| `public.trends` with the `tpdefault` partition, created as `scada_writer` | yes | yes |
| The five SemiPlot tables and `semiplot_register_new_pens()`, after `public.trends` because the function body names it | yes | yes |
| The `semiplot` access checks at the tail | yes | yes |
| Pending-restart warning at the tail | yes, and after a `--restart-service` restart the list is empty | yes — `bench` tunes nothing, but it runs against existing servers too, and one a `site` run tuned can still be waiting for its restart |

`version` (or `--version`) prints the build revision: the ldflags value
(`-X main.revision=...`) when a release sets it, otherwise the VCS revision from
`debug.ReadBuildInfo` with a `-dirty` suffix for a modified tree.

Every step is written to survive a re-run, most of them by checking before they create, so
re-running any command is safe.

Role passwords are set
only when the corresponding flag or environment variable is present (`SEMIBASE_SUPER_PASSWORD`,
`SEMIBASE_WRITER_PASSWORD`, `SEMIBASE_PLOT_PASSWORD`); a flag wins over its variable. No file is
read. A password file beside `semibase.exe` would keep the superuser password in plaintext on the
installation machine, and no caller needs one: CI sets the variables at job level, a consumer's
bench passes them with `docker -e`, and a hand run sets them in its shell. A run without
passwords leaves existing credentials untouched. Flag usage output shows only the environment
variable names, never their values. The statements that set a password stay out of the server
log ([Invariants the provision package owns](#invariants-the-provision-package-owns)).

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
asserts the container reaches a state where `semiplot` can query.

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
- **Object resolution.** `connect` also issues `SET search_path = public` for the session. The
  `semiplot_*` objects are named unqualified in the embedded DDL and in the tail check's
  `has_table_privilege` arguments, so without the setting a `semiplot_tags` in another schema on
  the superuser's `search_path` would take the grants and the probes.
- **Password statements stay out of the server log.** `connectSuperuser` adds
  `SET log_statement = 'none'`, `SET log_min_error_statement = 'panic'`,
  `SET log_min_duration_statement = -1`, `SET log_min_duration_sample = -1` and
  `SET log_transaction_sample_rate = 0` to the session settings. `CREATE ROLE ... PASSWORD` and
  `ALTER ROLE ... PASSWORD` carry the password as a literal, and each setting otherwise prints a
  statement's text in one case: every statement under `log_statement = 'all'`, which an operator
  sets for the SCADA's first start ([the SCADA assumption](#assumption-the-scada-meeting-an-existing-trends));
  a failing statement under the default `log_min_error_statement = error`; a statement slower than
  `log_min_duration_statement`, which `config` sets to 1000 ms; a sampled statement slower than
  `log_min_duration_sample`; every statement of a transaction `log_transaction_sample_rate`
  samples. The second has a cost: a failing superuser statement, a broken `sql/semiplot_*.sql`
  included, leaves its error in the server log without its text, and the run's own output names
  what failed. Extensions that keep statement text, such as `pg_stat_statements` with
  `track_utility`, are outside this guard; `semibase` installs none. The five are SUSET, and the
  project requires the role `semibase` connects as to be a superuser; when the server refuses one
  with 42501, the error names the role and the repair. The `semiplot` login of
  [check 8](#what-the-tail-checks-prove) sets none of them and issues no password statement.
  `TestSessionSettings` pins both lists. CI reads the server log through
  `.github/scripts/check-server-log.sh` in the `Keep the password statements out of the server log`
  step (the `ALTER ROLE` path) and the
  `Keep the CREATE ROLE password statements out of the PostgreSQL 14 log` step.
- **Cancellable execution.** Every phase honors its context: cancellation stops the
  in-flight query through pgx instead of killing the process mid-DDL. The context itself —
  `signal.NotifyContext(..., os.Interrupt)` plus the five-minute per-command timeout
  (`phaseTimeout`) — is owned by the CLI dispatcher in `cmd/semibase`. The one exception is a
  service restart: once the service accepts the Stop, the waits and the Start ignore the
  command's cancellation, so that an interrupt does not leave the service stopped
  ([The service restart](#the-service-restart)). A run that used up `phaseTimeout` says so in a
  second error line.
- **Pending-restart reporting.** The tuning phase applies its deltas with `pg_reload_conf()` and
  reads `pg_settings.pending_restart` back from the server; both commands read it again as the
  **last** statement of the tail, after the access checks, so a pending `shared_buffers` or
  `listen_addresses` is named in the run's last line. Without `--restart-service` the tool never
  touches the service manager: the operator restarts the service or reboots the machine, and the
  tool says so instead of doing it. With the flag, `site` restarts the named Windows service
  itself, and the tail reads an empty list.

## The service restart

`site --restart-service <name>` restarts that Windows service when, and only when, the tuning
phase leaves settings waiting for a restart, then waits for the server to accept connections and
continues. Without the flag the run reports the pending settings and changes nothing else.

Only `site` registers the flag: `bench` applies no settings, so nothing it does waits for a
restart, and `bench --restart-service` fails as an undefined flag. `Options.Validate` refuses a
`--host` other than `localhost`, `127.0.0.1` or `::1`, because after the restart the server listens
on the loopback interface only.

`config` opens the service before it connects, so a service it cannot restart fails the run with
nothing changed. `openService` (`service_windows.go`) refuses a service that does not exist, a
service manager or service the run may not control (the message says to run `semibase` as an
administrator), and a Disabled start type, which would leave the service stopped after the Stop.
A service in Start Pending gets up to 2 minutes to reach Running: an installer may run `site` right
after it started the service, and an EDB service (`pg_ctl runservice ... -w`, the `PathName` the
Windows CI step prints) reports Running only once the server accepts connections. That step starts
the service with `sc.exe start`, which returns while it is Start Pending, and runs `site` at once. Stopped, Stop Pending, Paused, or a start that ends in Stopped,
is refused. The tool does not check that the service is the server behind `--port`: a wrong name
restarts another running instance, and the run then fails with the settings still pending and a
hint to compare the service's `-D` with `SHOW data_directory`. Off Windows, `service_other.go`
refuses the flag the same way and names the platform, so the module stays one cross-platform
build.

The restart runs inside `config`, right after `pendingRestartSettings` returns a non-empty list, so
`create` and `check` run against the restarted server. `applySettings` closes the phase connection
before it returns; then:

1. Stop, then wait up to 3 minutes from the Stop for Stopped: a shutdown checkpoint with
   `max_wal_size = 8GB` can be long.
2. Start, then wait up to 2 minutes from the Start for Running. Stopped after the first query fails
   at once: the postmaster exited on the new settings.
3. Connect to `postgres` as the superuser every second, up to 60 s. A refused connection and 57P03
   are retried; any other server error, a wrong password for one, fails at once. A timeout
   reports the last connect error, never the context's.
4. Read `pending_restart` on the new connection. A non-empty list is an error naming the settings.

Once the service accepts the Stop, steps 1 and 2 run on `context.WithoutCancel`, each to its own
bound. An interrupt or the command timeout during them does not stop the sequence: the service is
started, and the run then ends with the cancellation. Each wait error names the service and the
last state it observed. Three cases still leave the service stopped:

| Case | What the run says | Recovery |
| --- | --- | --- |
| The stop takes longer than 3 minutes | the service is still stopping and will stay stopped | once `Get-Service <name>` shows Stopped, `Start-Service <name>` |
| The process ends without running its code: console window closed, logoff, shutdown, a killed process. `signal.NotifyContext` catches `os.Interrupt` only, and Windows ends a console process a few seconds after a close event, which no handler can extend | nothing | `Start-Service <name>` |
| The postmaster refuses the new settings, for example it cannot allocate `shared_buffers = 2GB`, and the service falls back to Stopped | the service stopped again after the Start; the settings stay in `postgresql.auto.conf` | read the server log in the data directory's `log` folder, remove the offending line from `postgresql.auto.conf`, then `Start-Service <name>` |

The command's five-minute `phaseTimeout` also covers the restart. A stop and a start that use
their whole bounds leave no time for the ready wait, `create` and `check`; the run then fails with
the service running, says it ran out of its limit, and a re-run finishes the work.

Service control goes through `golang.org/x/sys/windows/svc/mgr`, not `net stop` and `net start`.
Its errors do not depend on the Windows display language, it reads the start type, so a Disabled
service is refused before any change, and it reports the service state for the bounded waits.
`x/sys` is pure Go, so the release still cross-compiles with `CGO_ENABLED=0`.

A restart drops every open connection to the server, the SCADA's included.

CI's Linux job runs `site` without the flag: its server is a GitHub Actions service container, not
a Windows service, and nothing in the job restarts it. The `Require the listen_addresses delta`
step then reads `listen_addresses` still pending, which proves `site` wrote the delta. The Windows
job's `Restart the runner's PostgreSQL service through site` step is the one test of the restart
itself. The runner image ships PostgreSQL 17 as `postgresql-x64-17`, Stopped and Disabled, so the
step sets it to Manual and starts it. It then requires the first
`site --restart-service postgresql-x64-17` run to print `restarting service postgresql-x64-17`
and to leave `listen_addresses = localhost` with no setting pending, and a second run not to
restart the service.

## The roles

Four roles take part, and the tool creates three of them.

| Role | Created by | What it holds |
| --- | --- | --- |
| The superuser (`--superuser`, default `postgres`) | the engine install | Every connection this tool makes. It owns the five `semiplot_*` tables, because it is the connection that applies their DDL |
| `semiplot_registrar` | `semibase` | `NOLOGIN`. Owns `semiplot_register_new_pens()` and holds what its body needs: `SELECT` on `public.trends`, `SELECT (id)` and `INSERT (id, name, color, enabled_on_start)` on `semiplot_tags` ([Registering new pens](#registering-new-pens)) |
| `scada_writer` | `semibase` | Owns `public.trends`, its day partitions and `messages`, and writes them. `CREATE` on schema `public`. Nothing on any `semiplot_*` table |
| `semiplot` | `semibase` | `SELECT` on `trends` and `messages`; `SELECT` on `semiplot_tags` and `UPDATE` on its nine settings columns, never on `id`; `SELECT, INSERT, UPDATE, DELETE` on `semiplot_groups`, `semiplot_pen_groups` and `semiplot_markers`; `SELECT` on `semiplot_meta`; `EXECUTE` on `semiplot_register_new_pens()`. No `CREATE` on schema `public` |

`semiplot` edits its own pen catalogue and time markers and cannot touch the archive. Both facts are grants on
individual tables, so the second does not weaken when the first is given: the viewer's editor
saves a pen through the same connection it reads history on, and a bug in it cannot corrupt
`trends`.

A `semiplot_tags` row is keyed by `id`, the SCADA variable number. SemiPlot owns the settings in the
row and none of the keys: its editor changes a pen and never adds one, deletes one or moves one onto
another variable. The grant on that table is therefore column-level, `GRANT SELECT, UPDATE (name,
unit, format, color, line_style, enabled_on_start, scale_min_on_start, scale_max_on_start,
log_scale_on_start)`, and a table-level `UPDATE` would not do: it would let `UPDATE semiplot_tags
SET id = ...` re-key a pen. A column grant is not a table grant, so `has_table_privilege('semiplot',
'semiplot_tags', 'UPDATE')` answers false, and no check asks it. A pen row is added only by
`semiplot_register_new_pens()` ([Registering new pens](#registering-new-pens)), and only the
superuser deletes a pen.

The missing `CREATE` is issued rather than inherited. PostgreSQL 15 dropped the default that
grants `CREATE` on schema `public` to `PUBLIC`, but 14 is this tool's floor and still carries it,
so `create` issues `REVOKE CREATE ON SCHEMA public FROM PUBLIC` and grants it back to
`scada_writer` alone, and the tail check reads `has_schema_privilege` back. Without the `REVOKE`
the property held on 15 and later by engine default and not at all on 14.

The `Provision a PostgreSQL 14 container, where the REVOKE bites` CI step is what gates the
statement. On `postgres:17-alpine` the tail check reads `has_schema_privilege` false with or
without it, so every other step in the file stays green when the `REVOKE` is deleted; on
`postgres:14-alpine` `semiplot` holds `CREATE` through `PUBLIC` and the run exits 1. Measured
both ways.

A second, write-only role would keep the long-lived read connection free of any write privilege.
It was rejected: both passwords would sit in the same plaintext `connection.yaml` on the same
machine, so the separation separates nothing that an attacker who reads one file does not already
have.

### Trust

`scada_writer` is the SCADA itself, on the same machine, and it is trusted. It owns `public.trends`
and can already rewrite or drop every sample in it. A role that reads a table runs code its owner
attaches to it: a row-level-security policy on `trends`, or a view `scada_writer` puts in its
place, calls its functions as the reading role. So `scada_writer` acts as `semiplot` whenever
`semiplot` reads `trends`, and as `semiplot_registrar` whenever `semiplot_register_new_pens()` runs.
Measured on 14 and 17: through a policy, one `SELECT semiplot_register_new_pens()` inserted a pen
and re-granted the function to `PUBLIC`, and one plain read by `semiplot` renamed every pen and
deleted every group. No per-function setting closes this: `SET row_security = off` is answered by
replacing the table with a view.

The grants enforce one boundary: the SemiPlot operator, acting through `semiplot`, updates pen
settings, groups and time markers and never inserts, deletes or re-keys a `semiplot_tags` row. `semiplot_registrar`
still earns its place against `scada_writer`: under a superuser owner the same view code ran as the
superuser and reached the whole cluster; under `semiplot_registrar` it reaches `semiplot_tags`
(`SELECT (id)` and `INSERT` on four columns) and the function the role owns.

## The SemiPlot configuration schema

Five tables and one function, applied on every run after `public.trends` exists and in the order
their references need. `semiplot_register.sql` follows `semiplot_tags.sql`, because PostgreSQL
checks a `LANGUAGE sql` body against the relations it names when the function is created: a missing
`trends` or `semiplot_tags` fails the statement. A membership row names a pen, so
`semiplot_tags.sql` also runs before `semiplot_groups.sql`. `semiplot_markers.sql` names no other
relation and runs after them, and `semiplot_meta.sql` runs last.

| Table | Holds |
| --- | --- |
| `semiplot_tags` | One pen: `id` matching `trends.id`, `name`, `unit`, `format` as a .NET numeric format string the viewer applies and the server does not validate, `color` as `#RRGGBB`, `line_style` (0 interpolated, 1 stepped), `enabled_on_start`, the `scale_min_on_start`/`scale_max_on_start` pair that bounds the pen's own Y axis (both `NULL` means autoscale), and `log_scale_on_start`, the type of that axis: `false` is linear, `true` is log10. A column with the `_on_start` suffix holds a start value: what the pen opens with in a new window; for the scale pair and the log flag, also what the viewer's "Restore initial scale" returns to. Editing the pen on the chart changes the current view and never a start value, and a changed start value never changes a chart already open. Every other settings column is a live setting, which a running chart applies at its next catalogue read |
| `semiplot_groups` | A group name. `GENERATED ALWAYS AS IDENTITY` rather than `serial`, so `semiplot` needs no `USAGE` on a sequence to insert one |
| `semiplot_pen_groups` | Membership, `(pen_id, group_id)`. A pen may sit in several groups and in none: a pen with no row here is legal and the viewer shows it ungrouped |
| `semiplot_markers` | One time marker the operator placed on the chart: `id` as `bigint GENERATED ALWAYS AS IDENTITY`, the moment `t`, and an optional `label` of 1 to 80 characters. An unlabelled marker stores `NULL`, and the `CHECK` refuses `''`. `t` is `timestamptz`, not the naive local wall-clock time of `trends.t`: the viewer places an exact UTC moment, and a naive local value is ambiguous in the autumn repeated hour. No index, because the viewer reads the whole table. Not partitioned and outside any retention, so a marker outlives the day partition it points into |
| `semiplot_meta` | One row: `schema_version`. A `singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton)` admits no second row, and the value is written by one `INSERT ... ON CONFLICT (singleton) DO UPDATE`, so a failure cannot leave the table empty and a re-run cannot leave two rows |

These five are granted table by table, not through default privileges: the superuser connection
creates them, so no runtime owner exists whose future tables a default-privileges rule could cover,
and one explicit `GRANT` per table says what the role holds. `semiplotTables` in
`internal/provision` is the one list carrying both the table names and the grant each gets, from
three named privilege sets: read-only, full DML, and the `semiplot_tags` column grant built from
`plotEditableTagColumns`. `TestTagsUpdateGrantCoversEverySettingsColumn` derives that column list
from `sql/semiplot_tags.sql` minus `id`, so a column added to the table fails the suite instead of
arriving uneditable.

### Registering new pens

The archive carries no variable catalogue of its own, so `trends.id` is the only reliable source of
pen keys. `semiplot_register_new_pens()` (`sql/semiplot_register.sql`) inserts one `semiplot_tags`
row for every key in `trends` that has none and returns how many it added. The viewer calls it only
from its pen editor's "Refresh pen list" button, never at start. A new row is named by its number,
takes a colour from a fixed twelve-colour palette by `id % 12`, which assumes SCADA variable numbers
are non-negative (a negative `id` gets a `NULL` colour, and the viewer picks one), starts with
`enabled_on_start = false`, so a SCADA with 500 variables does not draw 500 lines at the next start,
autoscales, and opens on a linear axis (`log_scale_on_start = false`). A variable SCADA stops
writing keeps its row, because its history is still in `trends`. A trigger on `trends` was
rejected: it would run on every sample SCADA writes, on the one table SemiPlot does not own.

The function is `SECURITY DEFINER`, so `semiplot` adds a pen through it and holds no `INSERT` on
`semiplot_tags`: the role can add a pen only for a variable SCADA has written, never for an id it
picks. Two properties keep that true.

- **The owner is `semiplot_registrar`, not the superuser.** The body runs with its owner's
  privileges while it reads `public.trends`, a table `scada_writer` owns and can rename, replace
  with a view, or retype. Under a superuser owner, a view `scada_writer` puts in its place runs
  `scada_writer`'s code as the superuser (measured: a probe function in such a view reported
  `running as postgres (super=t)`). The superuser creates the function, because only it can, and
  `create` then issues `ALTER FUNCTION semiplot_register_new_pens() OWNER TO semiplot_registrar`.
  That role logs in nowhere and holds `SELECT` on `public.trends`, `INSERT` on the four columns the
  body writes, and `SELECT (id)`, which `ON CONFLICT (id)` needs to infer the primary key (measured:
  without it the insert is refused with 42501). `scada_writer` still runs its code as this role
  ([Trust](#trust)).
- **The body reads no schema its caller controls.** `pg_temp` is searched first for relations
  unless a `search_path` names it, and every role may create a temporary table. Under
  `SET search_path = public`, `semiplot` created a temporary `trends` holding ids 7 and 424242,
  and the function registered both (measured). So the function sets
  `search_path = pg_catalog, pg_temp`, leaving out `public`, where `scada_writer` holds `CREATE`, and
  its body names `public.trends` and `public.semiplot_tags` by schema.

The tail check replays the temporary-table attack by execution
([What the tail checks prove](#what-the-tail-checks-prove), check 3). The owner is the
`ALTER FUNCTION` that `create` issues in the same run, which either succeeds or aborts it.

PostgreSQL grants `EXECUTE` on a new function to `PUBLIC`, so the grant statement is `REVOKE ALL
ON FUNCTION semiplot_register_new_pens() FROM PUBLIC; GRANT EXECUTE ON FUNCTION
semiplot_register_new_pens() TO semiplot`.

The key scan is a loose index scan: a recursive CTE that asks each step for the smallest `id`
above the last one, `ORDER BY id LIMIT 1`. `trends` carries the primary key `(id, l, t)` on every
partition, so each step is a `Merge Append` of one index-only probe per partition, and the cost
grows with keys times partitions, not with rows. Measured as `semiplot` with psql `\timing`,
50 keys at `l = 0` every second and `l = 1` every minute:

| Server | Archive | First call (adds 50) | Second call (adds 0) |
| --- | --- | --- | --- |
| `postgres:17.11-alpine` | 1 day partition plus `tpdefault`, 4,392,000 rows | 13.1 ms | 1.9 ms |
| `postgres:17.11-alpine` | the same plus 89 day partitions at one row a minute, 91 partitions, 10,800,000 rows | 119.0 ms | 64.8 ms |
| `postgres:14.24-alpine` | 1 day partition plus `tpdefault`, 4,320,000 rows | 8.7 ms | 2.4 ms |

`EXPLAIN ANALYZE` of the key scan alone shows the same plan on both versions: one index-only scan
per partition per step, 0.8 ms on 17 and 1.4 ms on 14 over the one-day archive.

The measured archives are small, and the cost is linear in keys times partitions. A linear
extrapolation from the 91-partition row, not a measurement: 500 keys over 365 day partitions is 40
times the probes, about 2.6 s for a call that adds nothing and about 4.8 s for one that adds every
key, which is past one second at every press of the refresh button. Whether a site reaches that
depends on the retention depth, which is `UNDECIDED` ([README.md](./README.md)); the call stays
inside the `semiplot` role's 30 s `statement_timeout` either way.

### The schema version is a floor

This release writes `1`. A viewer states the minimum version it needs and refuses a database
storing a **lower** one; a **higher** one is accepted, because the versions this repository issues
grow by addition and a database carrying more than a viewer needs still carries what it needs.
The rule starts with the first installation. Until a site runs a provisioned database, a schema
change, a column rename included, edits the `CREATE TABLE` alone, issues no `ALTER`, and leaves the
number at `1`. A database provisioned before such a change is recreated, not provisioned again:
`CREATE TABLE IF NOT EXISTS` keeps its old table, and the column grant then fails with 42703 on the
column that table lacks. `semiplot_markers` is such a change: it leaves the number at `1`, and a
database provisioned without the table is recreated. After the first installation an added table
raises the number, and a viewer that reads only the tables it already knew has to keep working
against it, which an equality check would break.

A removal is not signalled by this number. An older viewer meeting a dropped column gets 42703,
which SemiPlot maps to an unexpected-shape failure naming the column.

`provision` is the only writer of `semiplot_meta`: a viewer that could raise the number would claim
a shape the database does not carry. The stamp is an upsert carrying
`WHERE semiplot_meta.schema_version < EXCLUDED.schema_version`, so an older binary re-run over a
newer database cannot write the number down.

## The SemiPlot read chain

`semiplot` needs `SELECT` on `trends` and on every day partition the SCADA creates later — objects
that do not exist when the grant has to be decided. A plain `GRANT` cannot be issued on a table that
is not there, so the create phase sets:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer IN SCHEMA public
    GRANT SELECT ON TABLES TO semiplot;
```

This is in place **before** `public.trends` is created and before the writer first starts; tables
`scada_writer` creates afterwards are readable automatically. If a table was created before the
statement ran, the repair is a one-time `GRANT SELECT ON ALL TABLES IN SCHEMA public` for the
tables that exist plus the default-privileges statement for the partitions still to come, then
another run of the tool — the tail check detects that state and prints all three.

`create` issues `REVOKE CREATE ON SCHEMA public FROM PUBLIC` and then grants `CREATE` to
`scada_writer` explicitly, so both statements say the same thing on every accepted major. On
PostgreSQL 15 and later the `public` schema no longer grants `CREATE` to everyone and the `REVOKE`
is redundant; on 14 it is the statement that makes "`semiplot` holds no `CREATE` on schema
`public`" true rather than aspirational.

## The archive table

`public.trends` is created by this tool, by both commands, from `sql/trends.sql` — the vendor's
shape, transcribed from a customer archive dump. Three properties of how it is created carry the
weight:

- **The role, more than the shape.** The table is created as `scada_writer`, because the `semiplot`
  `SELECT` has to arrive through the default privileges set for that role a moment earlier. A
  superuser-owned table would give `semiplot` access for a different reason than a site gets it,
  and a bench would then test a different thing from production. The role is assumed with
  `SET ROLE` on the superuser connection, not through a `scada_writer` login: measured on
  `postgres:17-alpine`, both routes leave the same `relowner` and the same `relacl`
  (`{scada_writer=arwdDxtm/scada_writer,semiplot=r/scada_writer}`), while a login also has
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

1. **`semiplot` reads `public.trends`.** The read itself, not a catalog bit: `SET ROLE semiplot` on
   the superuser connection, then
   `SELECT count(*) FROM (SELECT 1 FROM public.trends LIMIT 1) probe`. `has_table_privilege`
   cannot carry this check — after `REVOKE USAGE ON SCHEMA public FROM public` it still answers
   `t` while every read the role issues fails with `permission denied for schema public`
   (measured). The catalog bit is asked only when the read has already failed, to split a missing
   table grant from a schema the role cannot enter, and each answer prints its own repair.
   `SET ROLE` rather than a login for the `pg_hba.conf` reason above.
2. **`semiplot` reads `semiplot_meta.schema_version`**, as the role, and the run prints the value.
   This is the first question a viewer asks a database, and the `GRANT SELECT ON semiplot_meta`
   that answers it is the only grant no other check exercises: each writable table's `UPDATE` probe
   carries a `WHERE`, so its read privilege is proved as a side effect, while `semiplot_meta` is
   granted `SELECT` alone. Removing that grant used to leave every check passing.
3. **`semiplot` registers new pens, writes what it owns, and cannot add, delete or re-key a pen
   by hand.** The transaction opens with `SELECT semiplot_register_new_pens()` as the role, and the
   run prints how many keys in `trends` have no pen yet; a failed call is reported as it is, since a
   42501 may come from inside the body rather than from `EXECUTE`. Then the role plays the temporary-table
   attack: it creates a temporary `trends` holding id `-1`, grants the function's owner `SELECT` on
   it, calls the function again, and the run fails if `public.semiplot_tags` now holds pen `-1`.
   With `SET search_path = public` and the body's `public.` qualifiers removed in
   `sql/semiplot_register.sql`, the run exits 1 here; with only the `SET` changed, the qualified
   body still reads `public.trends` and the check passes (both measured on 17). The qualifiers are
   what keeps the shadow out.
   Then `INSERT`, `UPDATE` and `DELETE` on `semiplot_groups`, `semiplot_pen_groups` and
   `semiplot_markers`, and one `UPDATE` setting all nine settings columns of `semiplot_tags` to
   themselves (`WHERE id = -1`, which matches no row), issued as the role, in the
   order the foreign keys need. The membership `INSERT` pairs the probe group with whatever pen
   exists (`INSERT ... SELECT tag.id, grp.id FROM semiplot_tags tag, semiplot_groups grp ... LIMIT
   1`): the role cannot create a pen for it, and on an empty catalogue the statement inserts no row
   and still needs the privilege. The marker `INSERT` writes `t = now()` with the probe name as its
   label, and the marker `UPDATE` and `DELETE` find that row by the label. Then, in the same transaction, `INSERT INTO semiplot_tags`,
   `DELETE FROM semiplot_tags` and `UPDATE semiplot_tags SET id = id WHERE false`, each inside its
   own savepoint because a refused statement aborts the transaction around it; each must fail with
   42501. The whole transaction is rolled back: these are the tables an operator fills, and a probe
   row must not survive the check that wrote it. A refused positive write names the table and the
   operation and prints the `GRANT` that table gets, the column list for `semiplot_tags`. A
   negative write that succeeds, or fails with a class 23 integrity error and so got past the
   privilege check, names the grant as too wide and prints the `REVOKE` for the role, `PUBLIC` or a
   membership. Any other error, a timeout or a lost connection, is reported as it is, because it
   says nothing about the grant (`refusalVerdict`).
   Both halves are statements rather than catalog bits for the reason check 1 states, and because
   `has_table_privilege` cannot see a column grant.
4. **`PUBLIC` holds no `EXECUTE` on `semiplot_register_new_pens()`**, read through
   `has_function_privilege('public', 'semiplot_register_new_pens()', 'EXECUTE')`. The role's own call
   in check 3 succeeds whether or not `PUBLIC` holds it, so this one is a catalog question. Deleting
   the `REVOKE` from the grant statement makes a fresh database exit 1 here (measured).
5. **`semiplot` holds no `INSERT`, `UPDATE` or `DELETE` on `public.trends`**, nor on
   `public.messages` where the SCADA has created it. This is the invariant the writing role does not
   weaken, and it stays a catalog question: a refused statement and a statement that fails on its
   own terms look alike, and an `UPDATE` cannot even be spelled against a table whose columns this
   tool does not define. A failure names the privilege found and the `REVOKE` that removes it.
6. **`semiplot` holds none of the three on `semiplot_meta`** — the schema version is written by
   `provision` and by nothing else. A catalog question, for the same reason.
7. **`semiplot` holds no `CREATE` on schema `public`**, read through `has_schema_privilege`, so the
   role adds nothing beside the tables granted to it by name. On 14 this is a property `create`
   establishes with a `REVOKE`; on 15 and later the engine default already carries it and the check
   reads the same answer either way.
8. **The `semiplot` login reads `trends` too**, over TCP, when the `semiplot` password is present in
   the run. This is the half `SET ROLE` cannot reach: `pg_hba.conf` admitting the role and the
   password a consumer will carry. It is skipped, with a printed note, when no password
   was given, and on a socket host — `peer` is the platform default there and no consumer reads
   over the socket.
9. No setting waits for a service restart (`pg_settings.pending_restart`) — a warning, not a
   failure, so a pending `shared_buffers` or `listen_addresses` never blocks the access checks.
   Both commands read it, last, after the access checks. After a `--restart-service` restart it
   reads an empty list.

## Development benches

The bench is an ephemeral vanilla `postgres:17-alpine` container. The consumer's test fixture
starts it and provisions it by running `bench --database semiplot_dev --expected-major 17` — the
same code path a site takes minus the tuning, which is what keeps the fixture from drifting into a
hand-written copy of the grants. 14 remains the floor accepted through `--expected-major`; the
bench runs the pinned major, 17.

Two rules make the bench exercise what production exercises:

- `public.trends` is created **as `scada_writer`**, by both commands, so the default-privileges
  chain is a daily-tested path instead of a commissioning-day surprise.
- Bench integration tests read **as `semiplot`**, never as a superuser, so a broken grant
  fails a test today.

The consumer's seeder fills the archive and creates the day partitions its data needs; it and the
tests live in the SemiPlot repository. This repository promises them a provisioned database with an
empty `public.trends` in it.

## Passwords

The tool takes passwords through flags or environment variables and never stores them. The
`semiplot` credential ends up in plaintext in client configuration files by design — it grants
reading process history and editing the pen catalogue and the time markers, and nothing more. The writer credential is
recorded wherever the installation keeps its commissioning records, outside this repository.

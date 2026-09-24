# One SemiPlot role that may write its own tables, and the pen schema its editor needs

## Overview

SemiPlot is about to let an operator edit a pen: its display name, colour, units, line style, scale
bounds, visibility and group membership. Two things stop it today, and both live here.

**The SemiPlot role may not write anything.** `provision` creates `scada_writer` and `semiplot_reader`
(the role constants in `internal/provision/provision.go`), and the second held `SELECT` and nothing
else. The viewer needs to write its own configuration tables while remaining unable to touch `trends`
or `messages`.

**The pen table cannot hold a pen.** `semiplot_tags` carries `id`, `name`, `group_name`, `unit`,
`color` and `line_style` — the before-state, kept as
`internal/provision/testdata/semiplot_tags_v0.3.0.sql`. `group_name` is one string per pen, so a pen
cannot sit in two groups; there is no place for the pen's own scale bounds and no stored visibility.

This change gives SemiPlot one role that may write its own tables and no others, brings `semiplot_tags`
to the shape one pen row needs, moves groups into their own tables, and records a schema version so a
viewer built against the other shape says so instead of failing on a missing column.

**One role, not two.** A second write-only role would let the viewer's long-lived read connection stay
free of any write privilege, and against a threat model where the two passwords live apart that is
worth having. They do not live apart: both would sit in the same plaintext `connection.yaml` on the
same machine, so whoever reads one reads the other, and the separation separates nothing. What a second
role would actually buy is that a bug in the viewer cannot corrupt its own pen catalogue through the
connection it reads history on — a recoverable bug, not a breach. The invariant that matters is
untouched by this decision, because PostgreSQL grants per table: SemiPlot cannot write `trends` or
`messages`, and the tail check goes on proving it.

**The role is renamed.** `semiplot_reader` describes a role that now writes, so it becomes `semiplot`,
and `--reader-password` / `SEMIBASE_READER_PASSWORD` become `--plot-password` /
`SEMIBASE_PLOT_PASSWORD`. Leaving either name in place would recreate the naming defect this change
exists to remove. Renaming costs almost nothing today — there is no installer and no deployed
installation — and it gets more expensive every month, because `ALTER ROLE ... RENAME` leaves a dead
user name in every `connection.yaml` that already exists.

Both halves ship in one branch and one release. The grants are on the tables this change adds, so a
release carrying only the role change would grant on objects that do not exist; and every consumer
tracks `ghcr.io/semiteq/semibase:latest`, so the image should move under them once rather than twice.

## Context (from discovery)

Files and components involved:

- `internal/provision/` — the role constants and the `Options` struct carrying one password per role
  (`provision.go`); `create` (`create.go`) builds the role list, sets the SemiPlot role's timeouts and
  applies the files `semiplotSchemaFiles` names; `migrateSemiplotSchema` (`schema.go`) runs the
  migration, the version stamp and `semiplotGrantStatements` in one transaction. `check` (`check.go`)
  runs the access chain. `runAs` (`provision.go`) is the `SET ROLE` helper the assertions use, and
  `ensureRole` (`create.go`) creates a role or re-sets its password, refusing to create one it has no
  password for. Inside this repository the plan cites symbols and section anchors rather than line
  numbers: four review rounds found the numbers stale, and a symbol survives the next commit. The
  `SemiPlot/` citations keep their lines, because no commit here can move them. The names here are the
  pre-change ones where task 1 renamed them.
- `sql/semiplot_tags.sql` — the pen catalogue, `CREATE TABLE IF NOT EXISTS` with eight columns and
  two named `CHECK` constraints. It arrived with six columns and no constraint.
- `embed.go` — five `go:embed` directives: `SemiplotTagsSQL`, `SemiplotGroupsSQL`, `SemiplotMetaSQL`,
  `SemiplotMigrateSQL` and `TrendsSQL`. It arrived with the first and the last.
- `cmd/semibase/main.go` — `newFlagSet` and `resolvePasswordsFromEnvironment`, one entry per password;
  `.env.example` lists the three variables.
- `.github/workflows/ci.yml`, the `Provision the service container` step — the Linux job runs `site`
  twice in a row against a live `postgres:17-alpine` service. **This is the idempotency gate and it
  already exists**: any statement that cannot run twice fails the pull request. Its limit, found in
  review and closed by the `Provision over the shape the previous release left` step added afterwards:
  the container is **fresh**, so the migration's `DO` block enters no branch on either run — no
  `group_name` column has ever existed there and no `semiplot_reader` role does either. The migration's
  top-level statements do run there; what the double run proves nothing about is that block's body.
- `docs/architecture/provisioning.md` — the agent-facing design, with `## Invariants the provision
  package owns`, `## The SemiPlot read chain`, `## What the tail checks prove` and `## Passwords`.
- `docs/deployment.md` — the human-facing Russian document, with `## Пароли` and `## Роли`.

Related patterns found:

- Every statement `create` issues is written to be safe on a re-run: `CREATE TABLE IF NOT EXISTS`,
  `ensureRole` checking before creating, `GRANT` being idempotent by nature.
- The tail check proves a privilege by exercising it, not by reading a catalogue bit. The comment above
  `assertPlotReads` states why: `has_table_privilege` answers from one bit and says yes for a role that
  `REVOKE USAGE ON SCHEMA public` has locked out.
- `SET ROLE` rather than a login, because `pg_hba.conf` need not admit these roles; the login is checked
  separately where it can be (`checkPlotLogin`).

Dependencies identified:

- None inside this repository. The consumers are `Semiteq/SemiPlot#65` and `#67`, which move after the
  release this change produces.

## Development Approach

- **testing approach**: Regular (code first, then tests), matching the repository.
- complete each task fully before moving to the next
- `go build ./...`, `go test ./...`, `go vet ./...` and `golangci-lint run` all clean before the next task
- `gofmt -w .` before presenting any change
- **update this plan when scope changes during implementation**

## Testing Strategy

- **unit tests** beside the source, table-driven (`internal/provision/provision_test.go`,
  `cmd/semibase/*_test.go`). These cover statement construction and the option surface: the SQL text a
  helper builds, the rename decision, the flag and environment resolution for the renamed password.
- **there are no database-touching tests in this repository**, and this change does not add the first
  one. The integration check is the tail check both commands run against a live server, and this change
  extends it: the role must prove it can write `semiplot_tags` and prove it cannot write `trends`.
- **the idempotency gate is CI's double `site` run**, the `Provision the service container` step,
  against a live `postgres:17-alpine`. Every statement this change adds must survive it, which is why
  none of them may be a bare `ALTER TABLE ... ADD CONSTRAINT` and why the rename is guarded.
- **the upgrade gate is a second CI step**, added after review: the double run above is against a
  fresh database, where the migration's `DO` block enters no branch, so it cannot fail on a wrong
  column or table name inside one — PL/pgSQL parses a branch when that branch first runs. The
  `Provision over the shape the previous release left` step stamps
  `internal/provision/testdata/semiplot_tags_v0.3.0.sql` and rows that have to move, renames the role
  back with an MD5-hashed password, requires the no-password run to refuse, runs `site` twice with the
  password and reads the result back, deletes a pen and a group inside a rolled-back transaction —
  the only thing that executes `ON DELETE CASCADE` — and ends with the schema-version refusal of item
  2c, stamping `schema_version = 2` and granting `CREATE` back to `PUBLIC`, requiring a non-zero exit
  and `PUBLIC` still holding `CREATE`, then restoring both. Four unit tests carry what SQL text alone
  can carry: the dropped and the added constraint name in one statement must match
  (`TestSemiplotMigrateReplacesEachConstraintByItsOwnName`); the create file and the migration must
  state the same **normalised `CHECK` body**, not merely the same constraint name, because the bodies
  are written twice and a rule changed in one file keeps its name
  (`TestEveryCreatedConstraintIsMigrated`); every `CHECK` in the create file must carry a name, or the
  body-parity test cannot see it (`TestEveryCheckInSemiplotTagsIsNamed`); and the same parity must hold
  for columns (`TestEveryCreatedColumnReachesAnUpgradedDatabase`). The four are listed with their
  reasoning at `docs/architecture/provisioning.md#what-proves-the-migration`.
- **the version floor is a third CI step**, `Provision a PostgreSQL 14 container, where the REVOKE
  bites`. `REVOKE CREATE ON SCHEMA public FROM PUBLIC` does nothing on 15 and later, so on the
  `postgres:17-alpine` service container above the statement can be deleted with every step still
  green. On `postgres:14-alpine` its absence makes the tail check exit 1.

## Acceptance Evidence

Each item is a command with the result it must produce, against the measured before-state.

1. **A fresh database gets the whole shape, twice.**

   ```bash
   docker run -d --name sb -e POSTGRES_PASSWORD=pw -p 15432:5432 postgres:17-alpine
   go build -o semibase ./cmd/semibase
   SEMIBASE_SUPER_PASSWORD=pw SEMIBASE_WRITER_PASSWORD=w SEMIBASE_PLOT_PASSWORD=p \
     ./semibase site --port 15432 --database semiplot_dev --expected-major 17
   # and again, unchanged
   ```

   Both runs exit 0. This is the fresh path: the migration's top-level statements run on it too, and
   only its `DO` block finds no `group_name` and does nothing, so neither run proves anything about
   that block's body.
   This is what CI does in the `Provision the service container` step; running it by hand is the
   local form of the same gate.
   Today (2026-09-16): the second run exits 0 with the current statements, and that is the property this
   change must not lose.

2. **An existing v0.3.0 database is renamed once and stays renamed.**

   Against a database provisioned by v0.3.0, one `site` run **carrying `SEMIBASE_PLOT_PASSWORD`** leaves
   `semiplot` present, `semiplot_reader` absent, and the role's password equal to what the run supplied —
   proved by the tail check's own login step (`checkPlotLogin`), which connects over TCP with that
   password. A second run finds `semiplot` already there and renames
   nothing. Without that password the run is the refusal of item 2a instead, and changes nothing.

2a. **A rename that would wipe an MD5-hashed password is refused.**

   On a cluster storing `semiplot_reader`'s password as MD5 — which a cluster upgraded from PostgreSQL 13
   or earlier still does by default — a `site` run carrying no `semiplot` password exits 1 naming
   `--plot-password` and `SEMIBASE_PLOT_PASSWORD`, and leaves `semiplot_reader` un-renamed. This is the
   branch's most operator-visible new failure mode. The CI upgrade step gates it on both halves: the
   non-zero exit, and the captured stderr naming `SEMIBASE_PLOT_PASSWORD`, because the exit code alone
   does not distinguish this refusal from a regression in the tuning phase.

2b. **`semiplot` holds no `CREATE` on schema `public`, on the version where that is not free.**

   PostgreSQL 15 dropped the default that grants `CREATE` on schema `public` to `PUBLIC`; 14, this
   repository's floor, still carries it. So the property is gated on `postgres:14-alpine`, in the
   `Provision a PostgreSQL 14 container, where the REVOKE bites` CI step: `bench` against a fresh 14
   container exits 0 with the `REVOKE`, and exits 1 at `assertPlotCannotCreateInPublic` without it.
   Measured both ways on `postgres:14.24-alpine`. On `postgres:17-alpine` the same binary exits 0 either
   way, which is why the 17 service container cannot carry this item.

2c. **A database stamped past this build's version is refused, before the run touches it.**

   `provision`'s migration states each constraint rather than testing for it, so an older binary over a
   newer database would replace that release's `CHECK` with its own while `semiplot_meta` went on
   reporting the higher number. `assertSchemaVersionIsNotNewer` is therefore the first statement
   `create` issues on the archive connection, ahead of `REVOKE CREATE ON SCHEMA public FROM PUBLIC`.
   Gated in the CI upgrade step on both halves: with `schema_version` stamped at `2` and `CREATE`
   granted back to `PUBLIC`, the run exits non-zero with a stderr naming both numbers, and
   `has_schema_privilege('public', 'public', 'CREATE')` still reads `t` afterwards. Measured on
   `postgres:17.11-alpine`: `t` before the refused run, `t` after it; before the guard moved, the same
   run left it `f`.

2d. **The whole upgrade path converges in one run, on the version floor. Hand-verified, not gated.**

   Items 2, 2a, 2b and 2c each gate one half on the server that can carry it: the rename, the MD5
   refusal and the migration on `postgres:17-alpine`, the `REVOKE` on `postgres:14-alpine` over a
   fresh database. Nothing runs all four together. Measured by hand on `postgres:14.24-alpine`,
   2026-09-16, against a database provisioned by the `v0.3.0` tag build: the refusal without
   `SEMIBASE_PLOT_PASSWORD`, then one `site` carrying it exits 0, `semiplot_groups` holds `Etch` and
   `Vacuum`, `semiplot_pen_groups` three rows, `semiplot_meta` one row at `1`,
   `has_schema_privilege('public', 'public', 'CREATE')` moves `t` to `f`, and a second run exits 0
   changing nothing.

   No CI step covers the combination, and this plan does not add one. A fourth container step would
   duplicate the `Provision over the shape the previous release left` block against a second server to
   gate an interaction the code does not have: the `REVOKE` is one unconditional statement issued
   before the migration transaction opens, and no branch of the rename, the refusal or the migration
   reads the server version. The two halves are gated where each one bites; the cost of gating their
   product is a container and forty duplicated lines for no new failure mode.

3. **An existing v0.3.0 database migrates, and migrates only once.**

   ```sql
   -- against a database provisioned by semibase v0.3.0, before the run
   INSERT INTO semiplot_tags (id, name, group_name, unit, color, line_style)
   VALUES (1, 'Chamber pressure', 'Vacuum', 'Pa', '#3C7DD9', 0),
          (2, 'RF forward',       'Vacuum', 'W',  '#D97B3C', 0),
          (3, 'Ungrouped signal', NULL,     NULL, NULL,      1);
   ```

   After one `site`: `semiplot_groups` holds exactly one row, `Vacuum`; `semiplot_pen_groups` holds two,
   for pens 1 and 2; pen 3 has no group row; `group_name` is gone from `semiplot_tags`. After a second
   `site`: the same, and exit 0. The second run is the one that fails on a migration written without a
   guard, because `SELECT DISTINCT group_name` then reads a column that no longer exists.

4. **A colour the format does not accept is reported and nulled, not fatal.**

   With `INSERT INTO semiplot_tags (id, name, color) VALUES (4, 'Legacy', 'red')` present before the run,
   `site` exits 0, prints how many colours it cleared and their ids, and leaves `semiplot_tags.color` at
   `NULL` for pen 4. A `CHECK` added without this step aborts the whole migration with a bare SQL error
   and no remedy, on a database an operator filled by hand during commissioning.

5. **The role may write its own tables.**

   The tail check exercises it: as `semiplot`, `INSERT` then `DELETE` a row in `semiplot_tags` inside a
   transaction that rolls back, and the same for `semiplot_groups` and `semiplot_pen_groups`. A failure
   is a non-zero exit naming the table and the operation. The check exercises the privilege rather than
   reading `has_table_privilege`, for the reason the comment above `assertPlotReads` states.

6. **The role may not touch the archive.**

   The tail check keeps asserting `semiplot` holds no `INSERT` on `public.trends`, and extends it to
   `UPDATE` and `DELETE`, and to `messages` when that relation exists. A non-zero exit names the
   privilege found. This is the invariant the one-role decision does not weaken, and it is the one that
   matters.

7. **The role may not write the schema version.**

   `INSERT INTO semiplot_meta` as `semiplot` is refused with `42501`. `semiplot_meta` is written by
   `provision` and by nothing else.

8. **The schema version is readable and correct.**

   `SELECT schema_version FROM semiplot_meta` returns exactly one row holding `1`, readable as
   `semiplot`. A second `site` run leaves one row, not two, which the `singleton` primary key and the
   single upsert statement together guarantee.

9. **The option surface carries the renamed password, and the retired one does not fall through.**

   `go test ./cmd/semibase/...` covers flag, environment and `.env` resolution for `--plot-password`
   with the precedence the others follow, and `./semibase site --help` lists the flag naming its
   environment variable and never its value, which is the rule `newFlagSet` states in its own comment.
   A run supplying only the retired
   `SEMIBASE_READER_PASSWORD` fails with the message `ensureRole` already produces, naming the role it
   cannot create, rather than silently proceeding with an empty password.

10. **Nothing else moved.**

    `go build ./...`, `go test ./...`, `go vet ./...` and `golangci-lint run` are clean;
    `gofmt -l .` prints nothing.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with a plus prefix
- document blockers with a warning prefix
- update this plan if the implementation deviates from the scope above

## Solution Overview

**The role is renamed before it is ensured, and the rename is guarded.** `ALTER ROLE semiplot_reader
RENAME TO semiplot` runs only when `semiplot_reader` exists and `semiplot` does not. When neither
exists, `ensureRole` creates `semiplot` as it creates any role. When both exist, the run stops and says
so: two roles claiming one purpose is a state this tool cannot resolve for the operator. The rename runs
ahead of `ensureRole`, so the password the run supplies is applied to the renamed role in the same pass.

That ordering is not enough for the one PostgreSQL wrinkle here: `ALTER ROLE ... RENAME TO` clears an
**MD5-hashed** password, because an MD5 verifier is salted with the role name, and a cluster upgraded
from PostgreSQL 13 or earlier still defaults to `password_encryption = md5`. The version floor does not
close it — the floor is the *server* version, not the setting. So when `pg_authid.rolpassword` for
`semiplot_reader` begins `md5` and the run carries no `semiplot` password to put back, the rename is
**refused** rather than performed, naming `--plot-password` and `SEMIBASE_PLOT_PASSWORD`. The
alternative is a run that wipes the password, reports the credentials untouched, exits 0 and leaves a
role no consumer can log in as.

**Two SQL files describe the shape, one describes the move.** `semiplot_tags.sql` states the tables as
they should be on a fresh database, all columns present. `semiplot_migrate.sql` brings an existing
database to that shape and is guarded at every statement. Splitting them keeps each file answering one
question, and both run on every `site`, as the embedded SQL already does.

**Every added statement survives a second run.** `ADD COLUMN IF NOT EXISTS` and `CREATE TABLE IF NOT
EXISTS` carry their own guard. `ADD CONSTRAINT` does not, so each constraint is named and dropped by
that name in the same `ALTER TABLE` that adds it — PostgreSQL applies the pair as one action, and the
file then states the constraint rather than testing for it. The group move and the `DROP COLUMN` are
wrapped in a single `DO` block testing `information_schema.columns` for `group_name`, so the second
run finds the column gone and does nothing. CI runs `site` twice in one job, so a statement that
cannot repeat fails the pull request rather than an installation — but that run is against a fresh
database, so it enters no branch of the `DO` block and proves nothing about the block's body. The
upgrade step described in Testing Strategy is what executes it.

**An unparseable colour is cleared, not tolerated and not fatal.** The `color` column is free text today
and an operator may have typed `red` during commissioning. Adding `CHECK (color ~ '^#[0-9A-Fa-f]{6}$')`
validates existing rows, so that database stops provisioning with a bare SQL error naming no remedy. The
migration therefore sets any non-matching colour to `NULL` before adding the constraint, and reports how
many rows and which ids. `NULL` is a state the column, the constraint and the viewer already handle — it
means the viewer picks the colour — while `red` is a state nothing handles. The alternative, adding the
constraint `NOT VALID`, leaves the unparseable value in a column every consumer believes is validated,
which is the worse of the two.

**The schema version is 1, and it is a floor.** `semiplot_meta` holds one row with one `schema_version`
integer, and this release writes `1`. A viewer states the minimum it needs and refuses a database whose
stored version is **lower**; a higher one is accepted, because the versions this repository issues grow
by addition and a database carrying more than a viewer needs still carries what it needs.
`Semiteq/SemiBase#10` adds `semiplot_markers` and bumps the version to 2; a viewer that needs only the
pen tables must keep working against it, which an equality check would break. A removal is a different
matter and is not signalled by this number: an older viewer meeting a dropped column gets 42703, which
SemiPlot maps to an unexpected-shape failure naming the column
(`SemiPlot.DataSource.Postgres/ArchiveExceptionMapper.cs:61`), and that is what the `group_name` drop in
this change produces. The row is written with one `INSERT ... ON CONFLICT (singleton) DO UPDATE`, so a
failure cannot leave the table empty and a second run cannot leave it with two rows.

**The tail check gains a write chain beside the read one.** The existing pair proves the role reads
`trends` and holds no `INSERT` on it. The new pair proves the same role writes every `semiplot_*`
configuration table and holds no write on `trends` or `messages`. The write assertion exercises the
privilege inside a transaction that rolls back, rather than reading `has_table_privilege`, because a
catalogue bit says yes for a role locked out of the schema — the reason already written above
`assertPlotReads`. The two cannot-write assertions stay catalogue questions: a refused statement
and a statement that fails on its own terms look alike, and an `UPDATE` cannot be spelled against
`messages`, whose columns this tool does not define.

## Technical Details

**The tables, as a fresh database gets them:**

```sql
CREATE TABLE IF NOT EXISTS semiplot_tags (
	id         integer PRIMARY KEY,   -- matches trends.id
	name       text    NOT NULL,
	unit       text,
	color      text,
	line_style smallint NOT NULL DEFAULT 0,
	enabled_on_start boolean NOT NULL DEFAULT true,
	scale_min  double precision,
	scale_max  double precision,
	CONSTRAINT semiplot_tags_scale_paired CHECK (
		(scale_min IS NULL) = (scale_max IS NULL)
		AND (scale_min IS NULL OR scale_min < scale_max)),
	CONSTRAINT semiplot_tags_color_hex CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$')
);

CREATE TABLE IF NOT EXISTS semiplot_groups (
	id   integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	name text NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS semiplot_pen_groups (
	pen_id   integer NOT NULL REFERENCES semiplot_tags (id) ON DELETE CASCADE,
	group_id integer NOT NULL REFERENCES semiplot_groups (id) ON DELETE CASCADE,
	PRIMARY KEY (pen_id, group_id)
);

CREATE TABLE IF NOT EXISTS semiplot_meta (
	singleton      boolean PRIMARY KEY DEFAULT true CHECK (singleton),
	schema_version integer NOT NULL
);
```

`GENERATED ALWAYS AS IDENTITY` rather than `serial`, so the role needs no `USAGE` on a sequence to
insert a group.

Every constraint carries its own name, because the migration drops and re-adds each one by that name and
an inline two-column `CHECK` would auto-name to `semiplot_tags_check`, which the migration could not
name.

The scale rule pairs the two bounds rather than only ordering them. `scale_min < scale_max` alone admits
one bound set and the other `NULL`, because a `CHECK` passes on `NULL`, and the consumer has no
representation for that row: `SemiPlot.Core/Trends/PenScaleSettings.cs:3-10` is a `ScaleMode` plus two
non-nullable doubles. A half-set pair would make each viewer build invent its own rule.

**Column meanings.** `unit` is free text (`Pa`, `C`, `mbar`, `sccm`, `W`, `%`). `color` is `#RRGGBB`.
`line_style` is 0 interpolated, 1 stepped. `enabled_on_start` is whether the viewer draws the pen when
it starts, and nothing more: which pens a running window shows is the state of that window, not of the
archive, so two stations may show different sets and neither writes the other's.
`scale_min` and `scale_max` bound the pen's own Y axis, and both `NULL` means autoscale. A pen with no
row in `semiplot_pen_groups` is legal and the viewer shows it ungrouped.

**Grants:**

| Table | `semiplot` | `scada_writer` |
| --- | --- | --- |
| `semiplot_tags` | `SELECT, INSERT, UPDATE, DELETE` | none |
| `semiplot_groups` | `SELECT, INSERT, UPDATE, DELETE` | none |
| `semiplot_pen_groups` | `SELECT, INSERT, UPDATE, DELETE` | none |
| `semiplot_meta` | `SELECT` | none |
| `trends`, `messages` | `SELECT` | owner |

`semiplot` gets no `CREATE` on schema `public`. Grants are explicit per table, because these tables are
created by `provision` rather than at runtime by their owner, so default privileges buy nothing and say
less.

**Order inside `create`:** the rename runs before the roles are ensured; the roles exist before the
database; the tables exist before their grants; the migration runs after the tables are created, so a
fresh database runs it and finds nothing to do. The rest of the ordering — where the schema-version
refusal sits, what runs inside the migration transaction, and why each is there — is
`docs/architecture/provisioning.md`, which this plan points at rather than copies.

## What Goes Where

- **Implementation Steps** (`[ ]`): the rename, the DDL, the migration, the grants, the tail check, the
  tests and the documentation — all inside this repository.
- **Post-Completion** (no checkboxes): the release, and the consumer work that waits on it.

## Implementation Steps

### Task 1: Rename the role and its password across the option surface

**Files:**
- Modify: `internal/provision/provision.go`
- Modify: `cmd/semibase/main.go`
- Modify: `.env.example`
- Modify: `cmd/semibase/main_test.go`
- Modify: `internal/provision/provision_test.go`
- Modify: `cmd/semibase/env_test.go`
- Modify: `.github/workflows/ci.yml` (added during implementation: the live-postgres steps pass the
  renamed variable and connect as the renamed role)

- [x] rename the `ReaderRole` constant and its value to `semiplot` (`provision.go`), and every
      identifier carrying the old word: the option field, `readerProbe`, `assertReaderReads`,
      `assertReaderCannotWrite`, `checkReaderLogin`, `diagnoseFailedRead`
- [x] rename the flag and the environment variable to `--plot-password` and `SEMIBASE_PLOT_PASSWORD`
      (`newFlagSet` and `resolvePasswordsFromEnvironment`, `.env.example`), keeping the rule that usage
      shows the variable and never a value
- [x] add the guarded `ALTER ROLE ... RENAME TO` ahead of the role loop: rename only when the old name
      exists and the new one does not, and stop with a named error when both exist
- [x] write tests for the rename decision: old only, new only, neither, both
- [x] write tests for flag, environment and `.env` precedence for the renamed password
- [x] run `go test ./...` - must pass before task 2

### Task 2: State the full table shape and the new tables

**Files:**
- Modify: `sql/semiplot_tags.sql`
- Create: `sql/semiplot_groups.sql`
- Create: `sql/semiplot_meta.sql`
- Modify: `embed.go`
- Modify: `internal/provision/provision.go`
- Modify: `internal/provision/provision_test.go` (added during implementation: the embedded-SQL tests
  live beside the existing `TestEmbeddedTrendsSQL`)

- [x] give `semiplot_tags` its three new columns and its two named constraints, and drop `group_name`
      from it, so a fresh database never has the column
- [x] add `semiplot_groups` and `semiplot_pen_groups` in one file, named for the pair
- [x] add `semiplot_meta` with its `singleton` primary key and its `schema_version` column, holding `1`
- [x] embed both new files beside the existing two
- [x] apply them in `create` next to the existing `semiplot_tags.sql` application
      + only `semiplot_groups.sql` ended up there. `semiplot_meta.sql` was moved into the migration
        transaction in review, and `TestSemiplotSchemaFileOrder` now fails if it returns to
        `semiplotSchemaFiles`. The reasoning is
        `docs/architecture/provisioning.md#migrating-a-database-an-earlier-release-provisioned`.
- [x] write tests asserting every embedded file is non-empty and names the objects it should
      + the goldens that satisfied this were replaced in review by assertions over the objects
        rather than over the file text: `TestSemiplotTablesMatchTheEmbeddedFiles` and
        `TestSemiplotGroupsUseAnIdentityColumn`.
- [x] run `go test ./...` - must pass before task 3

### Task 3: Migrate an existing database, guarded at every statement

**Files:**
- Create: `sql/semiplot_migrate.sql`
- Modify: `embed.go`
- Modify: `internal/provision/provision.go`
- Modify: `internal/provision/provision_test.go`

Proved against a live `postgres:17-alpine`: a database provisioned by v0.3.0 and filled with four legacy
pens took one `site` run, which printed `semiplot_tags color cleared to NULL where it was not #RRGGBB:
ids 4 (1 total)`, moved `Vacuum` and `RF` into `semiplot_groups`, wrote three `semiplot_pen_groups` rows,
left pen 3 ungrouped and dropped `group_name`. A second run changed nothing and exited 0.

- [x] `ADD COLUMN IF NOT EXISTS` for `enabled_on_start`, `scale_min` and `scale_max`
- [x] make each constraint survive a second run, because `ADD CONSTRAINT` has no `IF NOT EXISTS` and
      returns 42710 on a second run
      + the shipped form is not the `DO` block testing `pg_constraint` this step first described: a
        name guard finds the name present and skips, so a *changed* `CHECK` would never reach a
        database carrying the old one. `DROP CONSTRAINT IF EXISTS x, ADD CONSTRAINT x` in one
        `ALTER TABLE`, which PostgreSQL applies as one action, repeats and replaces alike
- [x] clear every `color` the pattern rejects to `NULL` before the constraint is added, and report the
      count and the ids through the existing `ok` output
      + the clearing is `provision`'s own statement rather than a line of the migration: the ids reach
        `ok` only through Go. One pattern serves all three places, and a test pins that.
- [x] wrap the group move and the `DROP COLUMN group_name` in one `DO` block testing
      `information_schema.columns`, so the second run finds nothing to do
- [x] write the schema version with one `INSERT ... ON CONFLICT (singleton) DO UPDATE`, so no failure
      leaves the table empty and no re-run leaves it with two rows
      + the upsert sits in `sql/semiplot_meta.sql` and is the only writer of `semiplot_meta` across
        every applied file, which `TestSchemaVersionIsWrittenByOneRaisingStatement` pins. Where that
        file is applied from, and why it is not applied beside the other create files, is
        `docs/architecture/provisioning.md#migrating-a-database-an-earlier-release-provisioned` —
        this plan does not restate it, because three review rounds corrected the restatement.
- [x] write tests for the statement text and for the version this release defines
- [x] run `go test ./...` - must pass before task 4

### Task 4: Grant the new tables to the role

**Files:**
- Modify: `internal/provision/provision.go`
- Modify: `internal/provision/provision_test.go`

Proved against a live `postgres:17-alpine`: as `semiplot`, `INSERT`/`UPDATE`/`DELETE` succeeded on
`semiplot_tags`, `semiplot_groups` and `semiplot_pen_groups`; `SELECT semiplot_meta` returned
`schema_version` 1 while `INSERT` and `UPDATE` on it were denied; `SELECT public.trends` worked and
`INSERT`, `UPDATE`, `DELETE` on it were denied; `CREATE TABLE public....` was denied for schema
`public`; `scada_writer` was denied both `SELECT` and `INSERT` on `semiplot_tags`. The catalogue
agrees: `semiplot=arwd` on the three tables, `semiplot=r` on `semiplot_meta` and on `trends`, and
`nspacl` on `public` carries `scada_writer=C` and no `semiplot`.

- [x] grant `SELECT, INSERT, UPDATE, DELETE` on the three configuration tables to `semiplot`
- [x] grant `SELECT` only on `semiplot_meta`
- [x] grant nothing new on `trends` or `messages`, and no `CREATE` on schema `public`
- [x] write tests for the grant statements, one case per table
      + the privileges moved into `semiplotTables`, which now carries one grant string per table:
        one list states which tables exist and what the role holds on each, so the report and the
        grants cannot drift apart.
- [x] run `go test ./...` - must pass before task 5

### Task 5: Extend the tail check with the write chain

**Files:**
- Modify: `internal/provision/provision.go`
- Modify: `internal/provision/provision_test.go`

Proved against a live `postgres:17-alpine`: a clean `site` run printed `semiplot inserts, updates and
deletes its own tables: semiplot_tags, semiplot_groups, semiplot_pen_groups`, `semiplot holds no INSERT,
UPDATE or DELETE on public.trends, public.messages` and the same line for `semiplot_meta`, and the three
probe tables were empty afterwards. Six mutations were then made one at a time and every one failed with
a non-zero exit naming the table and the operation: `REVOKE INSERT ON semiplot_tags`, `REVOKE UPDATE ON
semiplot_groups` and `REVOKE DELETE ON semiplot_pen_groups` against the check path, and `GRANT INSERT ON
public.trends`, `GRANT UPDATE ON public.messages` and `GRANT DELETE ON semiplot_meta` against a whole
`site` run.

- [x] assert the role writes each configuration table, as `SET ROLE` inside a transaction that rolls
      back, through the existing `runAs` helper
- [x] extend the archive assertion from `INSERT` to `INSERT`, `UPDATE` and `DELETE`, and to `messages`
      when that relation exists
- [x] assert the role cannot write `semiplot_meta`
      + the two negative halves stay catalogue questions, which is the reasoning already written above
        `assertPlotReads`: a refused statement and a statement that fails on its own terms look alike,
        and an `UPDATE` cannot be spelled against `messages`, whose columns this tool does not define.
        A `42501` on `INSERT INTO semiplot_meta` was confirmed by hand instead (acceptance item 7).
- [x] name the table and the operation in every failure, so a missing or excess grant is readable
      + the two messages are built by `writeProbeFailure` and `excessPrivilegeFailure`, so the text is
        pinned by a test rather than by a live run
- [x] write tests for the failure messages
- [x] run `go test ./...` - must pass before task 6

### Task 6: Verify acceptance criteria

Run against `postgres:17.11-alpine` in three throwaway containers, all removed afterwards. Port 15533
and 15633 rather than the 15432 the acceptance items name: a native PostgreSQL already listens on
15432 on this machine and Windows lets a second socket bind the same port, so the first run reached
the wrong server.

- [x] run acceptance items 1 to 3 against a live `postgres:17-alpine`, including the second `site` run
      and a database first provisioned by v0.3.0
      + item 1: a fresh `semiplot_dev` provisioned twice, both runs exit 0, the second printing
        `role semiplot exists, password updated` and `public.trends exists, left untouched`
      + items 2 and 3: v0.3.0 (tag build, from a worktree) provisioned `scada_archive`, the three pens
        of the acceptance INSERT were added by hand, then one HEAD `site` printed `role semiplot_reader
        renamed to semiplot` and `semiplot logs in and reads public.trends` with the supplied password.
        After it: `semiplot_groups` one row `Vacuum`, `semiplot_pen_groups` two rows for pens 1 and 2,
        pen 3 none, no `group_name` column, `semiplot_reader` absent. A second run renamed nothing and
        left all three counts unchanged, exit 0
      + the both-roles-exist stop was seen too: a cluster carrying `semiplot` and `semiplot_reader`
        fails with `both semiplot_reader and semiplot exist: ... Drop the one no connection.yaml names`
- [x] run acceptance item 4 with a hand-filled `red` present before the migration
      + `INSERT INTO semiplot_tags (id, name, color) VALUES (4, 'Legacy', 'red')` before the run; the
        run exited 0 printing `semiplot_tags color cleared to NULL where it was not #RRGGBB: ids 4
        (1 total). The viewer picks a color for those pens until one is set.` and pen 4's color is NULL
- [x] run acceptance items 5 to 8 and record what the tail check printed
      + `semiplot reads public.trends`; `semiplot inserts, updates and deletes its own tables:
        semiplot_tags, semiplot_groups, semiplot_pen_groups`; `semiplot holds no INSERT, UPDATE or
        DELETE on public.trends, public.messages` (both named once a `messages` owned by `scada_writer`
        exists, `public.trends` alone before that); `semiplot holds no INSERT, UPDATE or DELETE on
        semiplot_meta`; `semiplot logs in and reads public.trends`
      + item 6 bites: with `GRANT INSERT ON public.trends TO semiplot` the run exits 1 with
        `semiplot holds INSERT on public.trends and must not write it. Repair with REVOKE INSERT ON
        public.trends FROM semiplot, then re-run this command`
      + item 7: `INSERT INTO semiplot_meta` as `semiplot` answers `ERROR: 42501: permission denied for
        table semiplot_meta`
      + item 8: `SELECT schema_version FROM semiplot_meta` as `semiplot` returns one row holding `1`,
        and still one row after the second `site`
      + item 9: `site --help` lists `-plot-password string / semiplot password (env
        SEMIBASE_PLOT_PASSWORD)` and no value; `TestPlotPasswordPrecedenceAcrossDotEnv` and the two
        `TestParseOptions` plot cases pass; a run carrying only `SEMIBASE_READER_PASSWORD` stops with
        `role semiplot does not exist and no password was given to create it`
- [x] `go build ./...`, `go test ./...`, `go vet ./...`, `golangci-lint run`, `gofmt -l .`
      + all clean, `golangci-lint` printing `0 issues.` and `gofmt -l .` nothing
- [x] prove two guards by mutation: remove the `information_schema.columns` test from the group move and
      confirm the second `site` run fails; remove the rename guard and confirm the second run fails.
      Restore both.
      + group move without its guard: first run 0, second `error: applying semiplot_migrate.sql: ERROR:
        column "group_name" does not exist (SQLSTATE 42703)`, exit 1
      + rename without its guard: first run 0, second `error: ALTER ROLE semiplot_reader RENAME TO
        semiplot: ERROR: role "semiplot_reader" does not exist (SQLSTATE 42704)`, exit 1
      + both restored by `git checkout --`; the restored tree is clean and green
- [x] correct two claims this pass found untrue of the branch: the Solution Overview said both new
      assertions exercise the privilege in a rolled-back transaction, and acceptance item 2 named
      `checkReaderLogin` after task 1 renamed it

### Task 7: Update documentation

**Files:**
- Modify: `docs/architecture/provisioning.md`
- Modify: `docs/architecture/overview.md` (added during implementation: its consumer table named the
  retired role and its component table named one SQL file)
- Modify: `docs/architecture/README.md` (added during implementation: the Roles and Objects-we-add
  locked decisions named the retired role and one table)
- Modify: `docs/deployment.md`
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [x] `docs/architecture/provisioning.md`: three roles, what each may touch, the rename and its guard,
      the new tables, the migration and its guards, and the write half of the tail check
      + three new sections: `## The roles` with its rename table, `## The SemiPlot configuration
        schema` with the four tables, the version floor and the migration guards, and the tail-check
        list grown from four items to six
- [x] `docs/deployment.md` (Russian): the password table under `## Пароли` and the role table under
      `## Роли`
      + both rewritten, plus two new sections an operator needs: `## Таблицы SemiPlot` and
        `## Обновление ранее развёрнутой базы`, which states what one `site` run does to a v0.3.0
        database and that the connection file has to name `semiplot` afterwards
- [x] `README.md` and `CLAUDE.md`: both commands now create four tables, and the SemiPlot role is
      `semiplot`
      + `README.md` stays Russian: this repository's `CLAUDE.md` calls only `docs/architecture/`
        English, and the file has been Russian since it was written
- [x] archiving deferred to delivery: the plan stays in `docs/plans/` until the operator has tested
      the branch, so nothing was moved

### Task 8: Narrow the `semiplot` grant on `semiplot_tags` to its settings columns (➕)

A `semiplot_tags` row is keyed by `id`, the SCADA variable number. SemiPlot owns the settings in the row
and none of the keys: its pen editor changes a pen's settings, and never adds a pen, deletes one, or moves
one onto another SCADA variable. Today `semiplotTables` (`internal/provision/schema.go:20-28`) marks the
table `writable`, so `semiplotGrantStatements()` issues `SELECT, INSERT, UPDATE, DELETE`. A table-level
`UPDATE` would still let `UPDATE semiplot_tags SET id = 99` re-key an ungrouped pen, so the grant is
column-level: `GRANT SELECT, UPDATE (name, unit, format, color, line_style, enabled_on_start, scale_min,
scale_max) ON semiplot_tags TO semiplot`. `semiplot_groups` and `semiplot_pen_groups` keep full DML:
groups are SemiPlot's own data. Rows in `semiplot_tags` are created by Task 9's function, never by the
role directly.

Two consequences to respect. A column grant is not a table grant, so `has_table_privilege(semiplot,
'semiplot_tags', 'UPDATE')` answers false and no check may ask it for this table; privileges stay proved by
execution under `SET ROLE` in a rolled-back transaction. And the column list is a second copy of the list
`TestSemiplotTagsColumns` pins, so a test derives one from the other.

**Files:**
- Modify: `internal/provision/schema.go`
- Modify: `internal/provision/check.go`
- Modify: `internal/provision/schema_test.go`
- Modify: `internal/provision/check_test.go`
- Modify: `docs/architecture/provisioning.md`, `docs/architecture/overview.md`, `docs/deployment.md`,
  `README.md`, `CLAUDE.md`
  + `README.md` was left unchanged by this task: its one line about provisioning lists the roles and
    the tables `site` creates, and a column-level grant changes neither

- [x] replace `semiplotTables`' `writable bool` with the grant each table gets, and emit the column-level
      grant for `semiplot_tags` from a named list of its settings columns
- [x] write `TestTagsUpdateGrantCoversEverySettingsColumn`: the grant's column list equals the columns
      `columnNamesOf` parses out of `sql/semiplot_tags.sql`, minus `id` — a column added to the table
      later fails this test instead of arriving silently uneditable
      + a `REVOKE ALL ON <table> FROM semiplot` ahead of each grant was added here and cut in
        review: on a fresh server the role holds nothing to revoke

- [x] reshape `plotWriteProbes()` (`check.go:24-46`): drop the `semiplot_tags` INSERT and DELETE probes,
      keep the `UPDATE ... SET name = ... WHERE id = -1` probe, and rewrite the `semiplot_pen_groups`
      INSERT probe so it no longer inserts pen `-1`, which no longer exists in the transaction:
      `INSERT INTO semiplot_pen_groups (pen_id, group_id) SELECT tag.id, grp.id FROM semiplot_tags tag,
      semiplot_groups grp WHERE grp.name = '<probe>' LIMIT 1`
- [x] add the negative half, executed under `SET ROLE` in the same rolled-back transaction: `INSERT INTO
      semiplot_tags`, `DELETE FROM semiplot_tags` and `UPDATE semiplot_tags SET id = id WHERE false` each
      fail with 42501, and a success is a check failure naming the grant that is too wide
      + each refused statement runs in its own savepoint, because a 42501 aborts the transaction
        around it; the failure prescribes `REVOKE ... FROM semiplot` first, then the same revoke on
        `PUBLIC` or a membership (corrected in review: the `REVOKE ALL` wrapper that would have
        cleared the role's own grant was cut, so the grant by name is the first suspect)
- [x] correct the `ok` line (`check.go:198`), `writeProbeFailure`'s repair text (it must prescribe the
      column list, not the wide grant back), and rewrite `TestSemiplotGrantStatements`,
      `TestPlotWriteProbesCoverTheWritableTables` and `TestPlotWriteProbeOrderSatisfiesTheForeignKeys`
- [x] correct the role row at `provisioning.md:174` and the write-probe item at `:316-323`, the role row at
      `overview.md:19`, the role row at `deployment.md:65`, and `CLAUDE.md`
- [x] rewrite the commissioning step at `overview.md:61-62` and `deployment.md:137-138`: pens appear in
      `semiplot_tags` on their own when the viewer starts (Task 9); a person never inserts them, and only
      the superuser can delete one
- [x] run `gofmt -l .`, `go vet ./...`, `go test ./...`, then on a bench container, as `semiplot`: the
      eight-column UPDATE succeeds, and `SET id`, INSERT and DELETE on `semiplot_tags` each fail 42501
      + `postgres:17-alpine` on port 15833, two `bench` runs exit 0; as `semiplot`: `UPDATE 1`, then
        42501 three times. A table-level `GRANT INSERT, DELETE, UPDATE ... TO semiplot` left by hand
        was gone after the next run; `GRANT INSERT` and `GRANT UPDATE (id)` to `PUBLIC` each made the
        run exit 1 naming the grant as too wide. Container removed

### Task 9: Register new pens from the keys SCADA writes (➕)

Nothing creates a `semiplot_tags` row today but a person. The archive carries no variable catalogue of its
own, so `trends.id` is the only reliable source of keys. SemiBase provides one function that mirrors the
keys SCADA already writes into the pen catalogue with default settings. The viewer calls it at start and
from its pen editor's refresh button. The role gets `EXECUTE` on the function and no `INSERT` on the
table, so SemiPlot can add a pen only for a variable SCADA has actually written, never for an id somebody
typed. A trigger on `trends` is rejected: it would fire on every sample SCADA writes and hang our code on
the one table SemiPlot does not own.

Superseded: the block below is the function as first shipped. The review fix at the end of this task
replaced it; `sql/semiplot_register.sql` holds the current body.

```sql
CREATE OR REPLACE FUNCTION semiplot_register_new_pens() RETURNS integer
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
    WITH RECURSIVE keys AS (
        (SELECT id FROM trends ORDER BY id LIMIT 1)
        UNION ALL
        SELECT (SELECT t.id FROM trends t WHERE t.id > keys.id ORDER BY t.id LIMIT 1)
        FROM keys WHERE keys.id IS NOT NULL
    ), added AS (
        INSERT INTO semiplot_tags (id, name, color, enabled_on_start)
        SELECT id, id::text,
               (ARRAY['#4E79A7','#F28E2B','#E15759','#76B7B2','#59A14F','#EDC948',
                      '#B07AA1','#FF9DA7','#9C755F','#17BECF','#D62728','#9467BD'])[id % 12 + 1],
               false
        FROM keys WHERE id IS NOT NULL
        ON CONFLICT (id) DO NOTHING
        RETURNING 1
    )
    SELECT count(*)::integer FROM added;
$$;
REVOKE ALL ON FUNCTION semiplot_register_new_pens() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION semiplot_register_new_pens() TO semiplot;
```

The recursive CTE is a loose index scan over `trends`' primary key `(id, l, t)` (`sql/trends.sql:29`): one
index probe per distinct key per partition, not a scan of the rows. A new pen is named by its number,
drawn from a fixed palette by `id`, starts hidden (`enabled_on_start = false`, so a SCADA with 500
variables does not draw 500 lines at the next start) and autoscales. A variable SCADA stops writing keeps
its row: its history is still in `trends`. PostgreSQL grants `EXECUTE` on a new function to `PUBLIC`, so the
`REVOKE` is load-bearing.

**Files:**
- Create: `sql/semiplot_register.sql`
- Modify: `embed.go`
- Modify: `internal/provision/schema.go`
- Modify: `internal/provision/check.go`
- Modify: `internal/provision/schema_test.go`, `internal/provision/check_test.go`
- Modify: `docs/architecture/provisioning.md`, `docs/deployment.md`
- Modify: `internal/provision/create.go` (added during implementation: `public.trends` is now created
  ahead of the SemiPlot schema files)
- Modify: `docs/architecture/overview.md`, `CLAUDE.md` (added during implementation: the overview said
  the tool adds no functions, and both listed the SQL files)

- [x] add `sql/semiplot_register.sql` with the function above, embed it, and apply it after
      `semiplot_tags.sql` in `semiplotSchemaFiles`; a second `site` run replaces the function in place
      + PostgreSQL checks a `LANGUAGE sql` body against the relations it names at `CREATE FUNCTION`
        (measured: `relation "trends" does not exist`), and `create` used to create `public.trends`
        last. `ensureArchiveTable` now runs right after the default privileges and ahead of the
        schema files, so the default privileges still precede the table
- [x] issue the `REVOKE ... FROM PUBLIC` and `GRANT EXECUTE ... TO semiplot` with the other grants
      + one simple-protocol string, the fifth entry of `semiplotGrantStatements()`, run on every run:
        a new function is executable by `PUBLIC`, and `CREATE OR REPLACE` keeps whatever grants the
        function already carries, so a hand `GRANT ... TO PUBLIC` lasts until the next run's `REVOKE`
        (corrected in review)
- [x] check phase, under `SET ROLE semiplot` in the rolled-back transaction: `SELECT
      semiplot_register_new_pens()` succeeds; and `has_function_privilege('public',
      'semiplot_register_new_pens()', 'EXECUTE')` is false
      + the call opens the write-probe transaction and the run prints how many keys have no pen yet;
        the `PUBLIC` question is its own step, `assertPublicCannotRegisterPens`. Mutation: with the
        `REVOKE` deleted from the grant statement, `bench` on a fresh database exits 1 naming
        `PUBLIC holds EXECUTE on semiplot_register_new_pens()`; a hand `GRANT EXECUTE ... TO PUBLIC`
        is gone after the next run
- [x] measure the loose scan on the bench archive (`converge` fills one day of 50 pens) and record the
      time in `provisioning.md`; a scan past one second is a finding to report, not to optimise here
      + filled with `generate_series` as `scada_writer` rather than `converge`, which lives in
        SemiPlot: one day partition, 50 ids, `l = 0` every second and `l = 1` every minute, 4,392,000
        rows. As `semiplot`: 13.1 ms first call (50 added), 1.9 ms second (0) on `postgres:17.11`;
        8.7 ms and 2.4 ms on `postgres:14.24`. With 91 partitions and 10,800,000 rows on 17: 119.0 ms
        and 64.8 ms. Both versions plan one index-only probe per partition per step. No finding on
        the measured archives; the linear extrapolation to 500 keys over 365 partitions, about 2.6 s
        and 4.8 s, is past one second and recorded in `provisioning.md` (corrected in review)
- [x] tests: the schema file order, the grant statements, and the check phase's new probes
      + as of the last review round: `TestSemiplotSchemaFileOrder`, `TestSemiplotGrantStatements`,
        `TestRegisterFunctionIgnoresTheCallersSchemas`, `TestRegistrarStatements`,
        `TestRegistrarInsertGrantMatchesTheFunctionBody`, `TestRefusalVerdict`,
        `TestPlotTagsUpdateProbeWritesEverySettingsColumn`, `TestPublicRegistersPensFailure`. The
        first-shipped `TestRegisterFunctionRunsAsItsOwnerOverPublicOnly`,
        `TestRegisterNewPensProbeCallsTheFunction` and `TestRegisterNewPensFailure` were renamed or
        deleted in review
- [x] document the function in `provisioning.md` beside the configuration schema, and the
      pens-appear-on-their-own behaviour in `deployment.md`
      + `### Registering new pens` under the configuration schema, tail check 4 added and the rest
        renumbered; `overview.md` no longer says the tool adds no functions
- [x] run `gofmt -l .`, `go vet ./...`, `go test ./...`, then rebuild `semibase:local` as SemiPlot's
      `docs/plans/20260917-pen-catalogue-and-groups.md` describes
      + all clean, `golangci-lint run` `0 issues.`; `docker run --rm semibase:local version` prints
        `local`

- [x] + review fix: the function as first shipped ran as the superuser with `SET search_path =
      public`. Two attacks worked on a fresh `postgres:17-alpine`: `semiplot` shadowed `trends` with
      a temporary table and registered ids 7 and 424242, and `scada_writer` replaced `trends` with a
      view whose function reported `running as postgres (super=t)`. The function now belongs to the
      `NOLOGIN` role `semiplot_registrar` (`SELECT` on `public.trends`, `SELECT (id)` and `INSERT` on
      the four columns it writes), sets `search_path = pg_catalog, pg_temp` and names its relations
      by schema; the tail check replays the temporary-table attack. The shadow table now registers
      nothing. The view attack still runs `scada_writer`'s code, now as `semiplot_registrar
      (super=f)`: its reach falls from the whole cluster to `semiplot_tags` (`SELECT (id)` and
      `INSERT` on four columns) and the function, not to nothing. `scada_writer` is trusted and can act as `semiplot` and as `semiplot_registrar`
      through a policy or a view on the `trends` it owns; the grants bound the operator acting
      through `semiplot`. `provisioning.md#registering-new-pens` and `provisioning.md#trust` hold the
      reasoning
- [x] + review fix: the `rolsuper` tail check is removed. It ran right after the `ALTER FUNCTION ...
      OWNER TO` in the same invocation, which succeeds or aborts the run, so it guarded an
      unreachable state; the checks after it are renumbered. A failed pen-registration call no
      longer prescribes `GRANT EXECUTE`: a 42501 raised inside the body is not a missing `EXECUTE`
- [x] + review fix: the CI step `Register pens from the keys SCADA wrote` runs the function body over
      three keys written as `scada_writer` and requires 3, then 0, and each row's name, colour and
      `enabled_on_start = false`
- [x] + review fix: the temporary-trends replay stays although `create` writes the function from
      embedded bytes that `TestRegisterFunctionIgnoresTheCallersSchemas` pins. It is the only
      execution proof that `semiplot` cannot make the function add a key absent from `public.trends`,
      and it tests the real server's name resolution, which a byte pin cannot; the `rolsuper` check
      only re-read one statement's result from the same run. Measured on 17: the qualifiers alone
      keep the shadow out, so a mutant changing only the `SET` passes and one that also drops the
      `public.` qualifiers exits 1

SemiPlot's integration suite proves the same behaviour against the bench seeder's archive; see
SemiPlot's `docs/plans/20260924-pen-and-group-editor.md`.

## Post-Completion

*No checkboxes: these need action outside this repository.*

**Prerequisites in the consumer, before this releases**

`SemiPlot/bench/Dockerfile:3` and `SemiPlot/SemiPlot.Tests.Integration/DockerCli.cs:8` both name
`ghcr.io/semiteq/semibase:latest`. The moment this change is tagged, SemiPlot's `master` pulls it with no
commit of its own. Three things then break, in this order:

1. **The role name.** The bench passes `SEMIBASE_READER_PASSWORD`, which no longer exists, so
   `ensureRole` refuses to create `semiplot` with no password, the init script exits, `initdb` aborts
   and the published port never opens. Every container test times out before reaching anything else.
   The variable and the `user:` value are named in
   `SemiPlot.Tests.Integration/PostgresContainerFixture.cs:93-94`, `SemiPlot.AppHost/AppHost.cs:32-33`,
   `SemiPlot.Tools.ArchiveSeeder/BenchRoles.cs` and `ConfigFiles/connection/connection.yaml:4`.
2. **The dropped column.** `SemiPlot.DataSource.Postgres/ArchiveStatements.cs:22` selects `group_name`,
   `SemiPlot.Tools.ArchiveSeeder/TagCatalogWriter.cs:12` inserts it, and
   `SemiPlot.Tests.Integration/PostgresCatalogReadTests.cs:33` inserts it in its own fixture rows.
3. **Nothing reads the new shape.** That is `Semiteq/SemiPlot#65`.

Pinning both references to `v0.3.0` is a one-commit pull request in SemiPlot and it has to land first.
The pin moves to the new version inside `#65`, together with the role rename and the new catalogue read.

**The release**

A tag and its annotation, per the release convention in `CLAUDE.md`. The tag push builds and pushes
`:latest` and `:vX.Y.Z`.

**What waits on it**

- `Semiteq/SemiPlot#65` — read units, per-pen scale, visibility and multiple groups from the new shape,
  move the pin and the role name.
- `Semiteq/SemiPlot#67` — the pen editor, writing over the same role's connection.
- `Semiteq/SemiBase#10` — `semiplot_markers`, which needs the same grants and bumps the schema version
  to 2. It is not part of this change.

**Known consequences to watch**

- Last write wins, and nothing else is possible with this schema. There is no `updated_at` and no row
  version, so two operator stations editing one pen resolve by whoever saved last.
  `Semiteq/SemiPlot#67` leaves the question open; optimistic concurrency would be another migration here.
- A database provisioned by this release and then read by a viewer built against `v0.3.0` answers 42703
  on `group_name`. SemiPlot maps that to an unexpected-shape failure naming the column, which is
  readable. The reverse — a new viewer against an old database — is the case `semiplot_meta` exists for.
- An operator who filled `color` by hand with a name rather than a hex triplet loses that value. The run
  says so, with the ids, and the viewer picks a colour for those pens until someone sets one.

### Cut after the fact, 2026-09-17

The migration apparatus described above was deleted. No database provisioned by an earlier
release exists: `docs/architecture/provisioning.md` on master already states that the only pair
ever newly deployed is the newest SemiBase with the viewer as it stands, the SCADA-meets-existing-
`trends` case is still marked Unverified, the release downloads add up to one author testing, and
every bench is a throwaway container. The rename (`semiplot_reader` to `semiplot`),
`sql/semiplot_migrate.sql`, the colour clearing, the newer-database refusal, the migration parity
tests, the `v0.3.0` fixture and the CI upgrade step all defended a database that was never
provisioned. `semiplot_meta` stays as a plain stamp, moved into `semiplotSchemaFiles` as the third
file, because the viewer is the half of a delivered pair that does get updated. The pre-cut state
is commit `c788577`: everything named above is one `git show` away.

## Verify it yourself

**Executed by exec:**

- branch: semiplot-role-and-pen-schema
- commits: 14, 27 files changed, +3112 / -568

Measured on 2026-09-16 from a clean tree, each command run from the repository root:

```bash
go build ./...
go test ./... -count=1     # cmd/semibase ok, internal/provision ok
go vet ./...
gofmt -l .                 # prints nothing
golangci-lint run          # 0 issues
```

The acceptance items are in `## Acceptance Evidence` above and every one of them was run
against a live server during the run. Three checks are worth repeating by hand, because they
are the ones a green suite cannot show you:

1. **The upgrade path is gated, not hand-verified.** `.github/workflows/ci.yml` stamps the
   shape the previous release left onto the database the fresh step provisioned, then runs
   `site` over it. Prove the gate bites: delete the `INSERT INTO semiplot_pen_groups` from the
   `DO` body of `sql/semiplot_migrate.sql`, rebuild, and run that step's `run:` block. It exits
   3 with `memberships: got '0', want '3'` and `ungrouped pens: got '1,2,3,4,5', want '3,5'`.
2. **`REVOKE CREATE ON SCHEMA public FROM PUBLIC` is gated only on the version floor.** Delete
   the `revokePublicCreate` call from `create` and run `bench` twice: against
   `postgres:17-alpine` it exits 0, against `postgres:14-alpine` it exits 1 at
   `assertPlotCannotCreateInPublic`. PostgreSQL 15 withholds that grant by engine default, so
   the 14 step is the only place the statement can be proved to matter.
3. **The refusals refuse before they change anything.** Stamp a database at
   `schema_version = 2`, grant `CREATE ON SCHEMA public TO PUBLIC`, and run the binary. It
   exits 1 naming both versions, and `has_schema_privilege('public','public','CREATE')` reads
   `t` both before and after. An earlier revision of this branch read `t` then `f`: it revoked
   the privilege and then refused to do the work the revoke was for.

What no command here proves is what the consumer does with the shape this produces. That is
`Semiteq/SemiPlot#65`, and the prerequisites it must carry are in `## Post-Completion`.

### Tasks 8 and 9: the narrowed grant and pen registration

Executed on the same branch, `semiplot-role-and-pen-schema`, commits `222c014` to `3be448d`. Run it on a
throwaway container (never port 15432, where a native PostgreSQL 14 listens):

```bash
cd /c/Users/admin/projects/SemiBase
go test ./... -count=1
docker run -d --name sb17 -e POSTGRES_PASSWORD=super -p 15901:5432 postgres:17-alpine
go build -o semibase ./cmd/semibase
SEMIBASE_SUPER_PASSWORD=super SEMIBASE_WRITER_PASSWORD=writer SEMIBASE_PLOT_PASSWORD=plot \
  ./semibase bench --host localhost --port 15901 --database semiplot_dev
```

Every line prints `[ OK ]`, including tail check 3's four steps: registration, the temporary-trends replay,
the allowed writes, and the refused `semiplot_tags` writes.

1. **SemiPlot cannot manage SCADA's keys.** As `semiplot`: `UPDATE semiplot_tags SET name = name` succeeds;
   `INSERT INTO semiplot_tags (id, name) VALUES (1, 'x')`, `DELETE FROM semiplot_tags` and
   `UPDATE semiplot_tags SET id = id` each fail with `permission denied` (42501). On `v0.3.0` the INSERT and
   DELETE succeeded.
2. **A key SCADA writes becomes a hidden pen.** As `scada_writer`, create a day partition and write rows for
   ids 0, 13, 40 (the CI step "Register pens from the keys SCADA wrote" in `.github/workflows/ci.yml` has the
   exact statements). As `semiplot`: `SELECT semiplot_register_new_pens()` returns 3, a second call 0, and
   `SELECT id, name, color, enabled_on_start FROM semiplot_tags ORDER BY id` shows
   `0 0 #4E79A7 f`, `13 13 #F28E2B f`, `40 40 #59A14F f`.
3. **`semiplot` cannot register a key it invents.** As `semiplot`:
   `CREATE TEMP TABLE trends (id integer, l smallint, t timestamp); INSERT INTO trends VALUES (424242, 0, now());
   SELECT semiplot_register_new_pens();` returns 0 and no pen `424242` appears. Before `bf031e8` the same
   statements registered it.
4. **The function does not run as a superuser.** `SELECT proowner::regrole, prosecdef, proconfig FROM pg_proc
   WHERE proname = 'semiplot_register_new_pens'` shows `semiplot_registrar`, `t`,
   `{"search_path=pg_catalog, pg_temp"}`; `SELECT rolsuper, rolcanlogin FROM pg_roles WHERE rolname =
   'semiplot_registrar'` shows `f f`.

What no test here can show: the trust model. `scada_writer` owns `public.trends` and can act as `semiplot` or
`semiplot_registrar` through a policy or view on it; that is documented in
`docs/architecture/provisioning.md#trust`, not prevented. Remove the container: `docker rm -f sb17`.

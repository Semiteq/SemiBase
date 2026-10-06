# On-start pen columns: the scale pair renamed, the log-scale flag added

## Overview

A `semiplot_tags` column holds either a live pen setting or a start value. A live setting (name,
unit, format, colour, line style) changes a running SemiPlot chart at the next catalogue read. A
start value is what a pen opens with in a new window and what "Restore initial scale" goes back to;
editing the pen on the chart changes the current view and never the start value, and a changed start
value never changes a chart already open.

Start values carry the `_on_start` suffix. Today only `enabled_on_start` does, while the start scale
pair is `scale_min`/`scale_max`. SemiPlot#72 adds a third start value, the log10 axis type. This
plan makes the naming uniform and adds the flag, in two pull requests and one release:

1. Rename `scale_min`, `scale_max` to `scale_min_on_start`, `scale_max_on_start`.
2. Add `log_scale_on_start boolean NOT NULL DEFAULT false`, which `semiplot` may `UPDATE`.

No installation runs a database provisioned by an earlier release, so both changes go into the
`CREATE TABLE` alone, with no `ALTER` for an existing table, and `semiplot_meta.schema_version`
stays `1`. The docs that state the version rule ("the versions this repository issues grow by
addition") say that the rule starts with the first installation, so a rename before it carries no
bump. The SemiPlot half is `SemiPlot/docs/plans/20261006-log10-y-axis.md`; it runs its container
tests only after `v0.5.0` is tagged.

## Context (from discovery)

- `sql/semiplot_tags.sql:3-17`: nine columns ending in `scale_min`, `scale_max` (`:11-12`), then
  `semiplot_tags_scale_paired` (`:13-15`), whose body names both, and `semiplot_tags_color_hex`.
- `internal/provision/schema.go:22-31`: `plotEditableTagColumns`, which builds the column grant at
  `:38`.
- `internal/provision/check.go:29-30`: the tail check's write probe assigns every
  `plotEditableTagColumns` entry; `check.go` needs no change for a renamed or new entry.
- `sql/semiplot_register.sql:11-15`: the registrar inserts `(id, name, color, enabled_on_start)`;
  neither change touches it or its grant.
- Tests that spell the column set out:
  - `internal/provision/schema_test.go:60-86` (`TestSemiplotTagsColumns`, the list at `:64-74`);
  - `:147-153` (`TestSemiplotGrantStatements`, the text at `:152-153`);
  - `internal/provision/check_test.go:37-45` (`TestPlotTagsUpdateProbeWritesEverySettingsColumn`,
    `:38-40`) and `:85-100` (`TestWriteProbeFailure`, `wantTags` at `:95-96`).
  - `:217-229` (`TestTagsUpdateGrantCoversEverySettingsColumn`) derives its list from the SQL and
    needs no edit.
- `.github/workflows/ci.yml:65-96`: the service container is provisioned with `site` twice, then
  three pens (ids 0, 13, 40) are registered and `concat_ws(' ', id, name, color, enabled_on_start)`
  of each is compared with a fixed string.
- Docs naming the columns or their count, cited by text because task 1 shifts the line numbers
  task 2 reads:
  - the grant text "`GRANT SELECT, UPDATE (name, unit, ... scale_min, scale_max)`" and the
    `semiplot_tags` table row, `docs/architecture/provisioning.md`;
  - "eight settings columns", twice in `docs/architecture/provisioning.md` (the roles table and tail
    check step 3) and once in `CLAUDE.md`;
  - the defaults a registered pen starts with ("starts with `enabled_on_start = false` ... and
    autoscales"), `docs/architecture/provisioning.md#registering-new-pens`;
  - "границы шкалы" in the `semiplot_tags` row of `docs/deployment.md` (Russian).
- The version rule, "grow by addition": `docs/architecture/README.md:28`,
  `docs/architecture/provisioning.md:316-320`.
- Latest release tag: `v0.4.0`.

## Development Approach

- **testing approach**: Regular (code first, then tests in the same task)
- each of tasks 1 and 2 is its own branch and pull request, off `origin/master`, in that order
- every task ends with `go test ./...`, `go vet ./...` and `golangci-lint run` green; the next task
  starts only after that

## Testing Strategy

- unit tests: `internal/provision/*_test.go`, table-driven, beside the source
- database behaviour: the repository has no database-touching Go tests (`CLAUDE.md`, "There are
  no database-touching tests"); the CI `site`, register and `bench` steps run every statement
  against PostgreSQL 14 and 17

## Acceptance Evidence

Reproduce today, against a database provisioned by `v0.4.0`:

```powershell
psql -U semiplot -d semiplot_dev -c "SELECT scale_min_on_start, log_scale_on_start FROM semiplot_tags LIMIT 1"
# ERROR:  column "scale_min_on_start" does not exist
```

After both pull requests:

1. `go test ./...` passes: `TestSemiplotTagsColumns` lists
   `..., enabled_on_start, scale_min_on_start, scale_max_on_start, log_scale_on_start`;
   `TestSemiplotGrantStatements` and `TestWriteProbeFailure` name the same three in the `UPDATE`
   list; `TestPlotTagsUpdateProbeWritesEverySettingsColumn` assigns them.
2. The CI register step passes with `want='0 0 #4E79A7 f f,13 13 #F28E2B f f,40 40 #59A14F f f'`.
3. The CI `site` twice step and the `bench` steps on 14 and 17 pass: the tail check's write probe
   has updated all three columns as `semiplot` on both server versions, and the scale-pair
   constraint was created over the renamed columns.
4. `git grep -nw "scale_min\|scale_max" -- ':!docs/plans'` prints nothing.

Checks 1 and 4 passed on the unmerged `on-start-columns` branch. Checks 2 and 3 are pending the
pull-request CI run: they need the `linux` job's live PostgreSQL 14 and 17.

## Solution Overview

- The suffix marks a start value; a live setting has none. The constraint keeps its name,
  `semiplot_tags_scale_paired`, which names the rule rather than the columns.
- `log_scale_on_start` is `NOT NULL DEFAULT false`: a pen opens on a linear axis until the operator
  sets the flag, and the registrar needs no change.
- No `CHECK` ties the flag to `scale_min_on_start > 0`. SemiPlot's editor refuses that combination;
  a constraint would make the order of two single-column writes matter to the operator.

## Technical Details

```sql
CREATE TABLE IF NOT EXISTS semiplot_tags (
	id                 integer PRIMARY KEY,
	...
	enabled_on_start   boolean  NOT NULL DEFAULT true,
	scale_min_on_start double precision,
	scale_max_on_start double precision,
	log_scale_on_start boolean  NOT NULL DEFAULT false,
	CONSTRAINT semiplot_tags_scale_paired CHECK (
		(scale_min_on_start IS NULL) = (scale_max_on_start IS NULL)
		AND (scale_min_on_start IS NULL OR scale_min_on_start < scale_max_on_start)),
	CONSTRAINT semiplot_tags_color_hex CHECK (...)
);
```

The grant becomes `GRANT SELECT, UPDATE (name, unit, format, color, line_style, enabled_on_start,
scale_min_on_start, scale_max_on_start, log_scale_on_start) ON semiplot_tags TO semiplot`.

## What Goes Where

- Implementation Steps: SQL, the column list, tests, CI and docs in this repository
- Post-Completion: the release tag, the order against SemiPlot, the benches

## Implementation Steps

### Task 1: Rename the start scale pair (pull request 1)

**Files:**
- Modify: `sql/semiplot_tags.sql`
- Modify: `internal/provision/schema.go`
- Modify: `internal/provision/schema_test.go`, `internal/provision/check_test.go`
- Modify: `docs/architecture/provisioning.md`, `docs/architecture/README.md`, `docs/deployment.md`

- [x] rename both columns in the `CREATE` and in the `semiplot_tags_scale_paired` body; re-align the
      column block to the longer names
- [x] rename both entries of `plotEditableTagColumns`
- [x] update `TestSemiplotTagsColumns`, `TestSemiplotGrantStatements`,
      `TestPlotTagsUpdateProbeWritesEverySettingsColumn` and `TestWriteProbeFailure`
- [x] docs: the grant text and the `semiplot_tags` row in `provisioning.md` use the new names and
      say what a start value is; `docs/deployment.md` reads "границы шкалы при старте";
      `README.md:28` and `provisioning.md:316-320` start the version rule at the first installation
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run` (the `linux` job is open under
      Post-Completion)

### Task 2: Add the log-scale flag (pull request 2)

**Files:**
- Modify: `sql/semiplot_tags.sql`
- Modify: `internal/provision/schema.go`
- Modify: `internal/provision/schema_test.go`, `internal/provision/check_test.go`
- Modify: `.github/workflows/ci.yml`
- Modify: `docs/architecture/provisioning.md`, `CLAUDE.md`, `docs/deployment.md`

- [x] declare `log_scale_on_start boolean NOT NULL DEFAULT false` after `scale_max_on_start`
- [x] append `"log_scale_on_start"` to `plotEditableTagColumns`
- [x] extend the four tests of task 1 with the column
- [x] register step: `concat_ws(' ', id, name, color, enabled_on_start, log_scale_on_start)` and
      `want='0 0 #4E79A7 f f,13 13 #F28E2B f f,40 40 #59A14F f f'`
- [x] docs: the grant text and the `semiplot_tags` row in `provisioning.md` name the column (the
      axis type a pen opens with, `false` is linear); the three "eight settings columns" say "nine";
      `#registering-new-pens` adds that a registered pen opens on a linear axis;
      `docs/deployment.md` appends "логарифмическая шкала при старте"
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run` (the `linux` job is open under
      Post-Completion)

### Task 3: Verify acceptance criteria

- [x] run acceptance checks 1 and 4 on the clean `on-start-columns` branch (checks 2 and 3, and the
      run from `master` after both merges, are open under Post-Completion)

## Post-Completion

**Delivery, open**: these were not done by the execution run.

- [ ] split the branch into the two pull requests the plan names: task 1's commit, then task 2's
      commit with the review fixes; the plan-only commit goes with the second, and no review-fix
      header (`fix(docs): address review findings` among them) ships
- [ ] push each pull request and read its `linux` job; task 1's must pass before task 2's merges
- [ ] acceptance checks 2 and 3 pass in that CI run; then all four from a clean `master` after both
      merges
- [ ] move this plan to `docs/plans/completed/`

**Release**: tag `v0.5.0` after both merges, with a `**New Features**` entry for the log-scale flag
and a `**Breaking Changes**` entry for the renamed pair. The workflow pushes
`ghcr.io/semiteq/semibase:latest`.

**SemiPlot**: its container tests pull `:latest` before every build
(`SemiPlot/SemiPlot.Tests.Integration/DockerCli.cs:8-14`), so its pull requests run them after the
tag. Nothing merges into SemiPlot between the tag and its rename pull request.

**Benches**: rebuild every long-lived bench container after the tag. `CREATE TABLE IF NOT EXISTS`
leaves a table provisioned by `v0.4.0` as it is, the new grant then fails on the missing column, and
`converge` clones the stale `semiplot_provisioned` rather than provisioning it again.

**Executed by exec:**

- branch: on-start-columns

## Verify it yourself

1. Unit tests and the rename, on the branch:

   ```powershell
   cd C:\Users\admin\projects\SemiBase
   go test -count=1 ./...
   git grep -nw "scale_min\|scale_max" -- ':!docs/plans'   # prints nothing
   ```

   On `origin/master` the grep prints the old names in `sql/semiplot_tags.sql`,
   `internal/provision/schema.go` and the docs; on `a7ed621` and later it prints nothing.

2. The schema on a real server (what CI's `linux` job proves after the push):

   ```powershell
   go build -o semibase.exe ./cmd/semibase
   docker run -d --name sb17 -e POSTGRES_PASSWORD=super -p 15435:5432 postgres:17-alpine
   $env:SEMIBASE_SUPER_PASSWORD="super"; $env:SEMIBASE_WRITER_PASSWORD="w"; $env:SEMIBASE_PLOT_PASSWORD="p"
   .\semibase.exe bench --host 127.0.0.1 --port 15435 --database semiplot_dev --expected-major 17
   psql "host=127.0.0.1 port=15435 dbname=semiplot_dev user=semiplot password=p" -c `
     "SELECT scale_min_on_start, scale_max_on_start, log_scale_on_start FROM semiplot_tags LIMIT 1"
   docker rm -f sb17
   ```

   Before (a `v0.4.0` binary): `column "scale_min_on_start" does not exist`. After: the query
   runs, and `bench` ends on a passing access check whose write probe sets all nine settings
   columns as `semiplot`.

3. The constraint probe: the CI step `Register pens and probe the semiplot_tags constraints` runs
   on the pull request. It fails if `(5, NULL)` or `(5, 1)` is accepted as a scale pair, or a
   `NULL` `log_scale_on_start` is accepted; four mutated schemas were run against it locally and
   each failed the step.

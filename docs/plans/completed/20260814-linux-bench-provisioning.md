# Provision a containerised bench from Linux

## Overview

`semibase create` cannot run on a Linux machine while the package that owns it imports
`golang.org/x/sys/windows`: the whole module fails to compile for any other target.

The consumer that needs this is SemiPlot. Its integration tests start a PostgreSQL container and
must provision it exactly the way production is provisioned — the roles, the grants, the
default-privileges chain — because its own bench rules require the seeder to write as
`scada_writer` and the tests to read as `semiplot_reader`. If SemiPlot replicates that setup in its
own fixture, the copy becomes the thing exercised daily and `semibase.exe` goes back to being a
tool run once, on site, on the day it matters. This repository's stated reason to exist is the
opposite of that.

## Solution Overview

**Remove the OS-bound code instead of constraining it.** Both Win32 call sites exist for reasons
the installation profile does not support:

- `GlobalMemoryStatusEx` feeds `computeSettings` with the machine's RAM. Installation machines
  guarantee an 8 GB floor, so the memory settings are fixed constants sized to that floor
  (`shared_buffers = 2GB`, `effective_cache_size = 4GB`) — identical configuration on every
  machine, no host inspection.
- The service-manager restart (`restartService`/`stopService`/`startService`, flag `--service`)
  exists to make `ALTER SYSTEM` take effect. Of the eleven deltas, only `shared_buffers` is
  restart-context; the other ten apply through `SELECT pg_reload_conf()` — plain SQL. `config`
  reloads, then reads `pg_settings.pending_restart` back from the server and names what still
  waits; `verify` warns while the list is non-empty. The operator restarts the service or reboots
  the machine — the one manual step, reported instead of automated.
- Console colours (`EnableColors`, `windows.SetConsoleMode`) are dropped entirely; the helpers
  print plain `[ OK ]`/`[WARN]`/`[NOTE]` text, which is also the right output for a CI log.

With those gone the module is pure `pgx` and compiles for any GOOS. No build constraints, no
platform stubs, no `x/sys` dependency. Every command — including `config` and `all` — runs
anywhere, so the CI bench check is one `all` invocation against a service container.

**The bench** is an ephemeral vanilla `postgres:17-alpine` container the consumer's fixture starts
and provisions with `create --database semiplot_dev --expected-major 17`. Production installs
vanilla PostgreSQL 17 via `winget` (`PostgreSQL.PostgreSQL.17`), so the bench runs the same engine.
14 remains the floor `create` accepts (`date_bin`; the Simple-Scada docs also require 14+).

## Acceptance Evidence

1. [x] `go build ./...`, `go test -race ./...`, `go vet ./...`, `golangci-lint run` pass on
       Windows.
2. [x] `GOOS=linux GOARCH=amd64 go build ./...` and `GOOS=darwin GOARCH=arm64 go build ./...`
       both exit 0; `GOOS=linux go vet ./...` exits 0.
3. [x] `go mod tidy` drops `golang.org/x/sys` — nothing else in the module uses it.
4. [x] The Ubuntu CI job is green: build, unit tests, and the provisioning step — `all` run twice
       against the `postgres:17-alpine` service container, proving the bench path and idempotency
       in one step (`verify` inside `all` reports "writer has not run" as a state, not a failure).
       Lint runs once, on the Windows job — the same config over the same untagged code.
       Confirmed 2026-08-14: master push run `31793374588` succeeded.
5. [x] The Windows CI job stays green. Same run.

## Implementation Steps

### Task 1: Fixed settings, reload instead of restart — done

- [x] `settings` is a fixed list in `internal/provision/config.go`; `computeSettings`,
      `memoryStatusEx`, `globalMemoryStatusEx`, `totalPhysicalMemoryMB` removed
- [x] `Config` applies the deltas, runs `pg_reload_conf()`, waits `pg_sleep(0.5)` for the
      asynchronous reload, then prints `pendingRestartSettings` (from
      `pg_settings.pending_restart`)
- [x] `restartService`, `stopService`, `startService`, `servicePollInterval`, the
      `Options.ServiceName` field and the `--service` flag removed
- [x] `verify` warns while `pending_restart` is non-empty, before the writer checks, so the
      warning appears even when the writer has not run
- [x] the fixed list is specified by `configuration.md` and exercised by the CI provisioning
      step; no unit test duplicates it verbatim

### Task 2: Drop colours — done

- [x] `console.go` keeps `SetConsoleOutput` and the plain output helpers; colour constants,
      `colorsEnabled`, `EnableColors` and the `x/sys/windows` import removed
- [x] `main.go` no longer calls `EnableColors`; usage text names the reload behaviour
- [x] `console_test.go` removed with the colours — plain `Fprintf` wrappers carry no test

### Task 3: CI on Linux — code done, green run pending

- [x] `ubuntu-latest` job in `.github/workflows/ci.yml`: build, `go test -race`, and a
      `postgres:17-alpine` service container provisioned by running `all` twice
- [x] the stale "does not build on a Linux runner" comment removed from `ci.yml` and `CLAUDE.md`
- [x] first push shows both jobs green (acceptance items 4 and 5)

### Task 4: Documentation — done

- [x] `configuration.md`: the hardware floor, the fixed values, the reload/pending-restart
      application path
- [x] `provisioning.md`: `config` command row, pending-restart invariant, verify item 5, the
      containerised bench section
- [x] `overview.md`, `architecture/README.md`, `deployment.md`, `README.md`, `CLAUDE.md` aligned;
      the engine install line is `winget install --id PostgreSQL.PostgreSQL.17 --exact`

### Task 5: Close the plan

- [x] CI green on master confirms acceptance items 4 and 5
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

*Manual or external — no checkboxes.*

**Cut the first tag.** SemiPlot pins a SemiBase version so its fixture provisions with a known
build; delivery is headless (`go run github.com/Semiteq/SemiBase/cmd/semibase@<tag>` or a release
binary). A moving `@latest` would let a change here fail a consumer's suite without a version bump
to blame.

**Not in this change.** Retention and capacity planning currently lives in the consumer's
`postgres-instance.md` and belongs here. Two figures move with it, both still open and both needing
a measured write rate from a running installation: the retention depth in days and the resulting
disk size. The backup method and schedule is a third open question in the same document, and is an
operations decision rather than a capacity one. This is a separate documentation change with no
code in it.

# Site: loopback-only listening and an optional service restart

## Overview
- `semibase site` sets `listen_addresses = 'localhost'`: every client of the instance runs on the
  same Windows machine, and nothing on the operator network may reach the server.
- `semibase site --restart-service <name>` restarts that Windows service when, and only when, the
  tuning phase leaves settings waiting for a restart, then waits for the server to accept
  connections and continues. Without the flag the run behaves as today and warns.
- The Windows installer (`20261019-windows-installer.md`) is the caller of the flag. CI's Linux job
  runs `site` against a service container the tool cannot restart, so the restart stays opt-in.

## Context (from discovery)
- `internal/provision/config.go:16-28` — the `settings` list applied with `ALTER SYSTEM`;
  `config.go:30-71` applies it, reloads, and reports `pending_restart` (`config.go:60-70`);
  `config.go:36-40` opens the one connection the phase uses.
- `internal/provision/provision.go:39-44` — `Options.Validate`; `provision.go:48-56` — `Site` runs
  `config`, `create`, `check` in that order; `provision.go:26-35` — `Options`.
- `internal/provision/check.go:422` — `reportPendingRestart` at the tail of every run.
- `cmd/semibase/main.go:158-175` — one flag set shared by `site` and `bench`; the comment at
  `main.go:154-155` states every flag is used by either command; `main.go:184` calls `Validate`.
- `cmd/semibase/main.go:60,112` — one five-minute timeout covers the whole command.
- `go.mod:5` — the only direct dependency is `pgx/v5`.
- `.github/workflows/ci.yml:11-29` — the Windows job builds, tests and lints only;
  `ci.yml:31-47` — the Linux job's service container and its password `ci-super`;
  `ci.yml:65-71` — the Linux job runs `site` twice against `postgres:17-alpine`.
- `docs/architecture/provisioning.md:160-165` — "The tool never touches the service manager."
- `docs/architecture/configuration.md:57-58` — "listens on the loopback interface plus the operator
  network only"; this no longer holds.
- Read 2026-10-19 from `actions/runner-images` `images/windows/Windows2025-Readme.md`
  (`windows-latest`): PostgreSQL 17.11, service `postgresql-x64-17`, status Stopped, start type
  Disabled, user `postgres`, password `root`, `PGBIN=C:\Program Files\PostgreSQL\17\bin`,
  `PGDATA=C:\PostgreSQL\17\data`.

## Development Approach
- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- every task includes new or updated tests for the code it changes
- all tests pass before the next task starts
- update this plan when scope changes

## Testing Strategy
- unit tests beside the source, table-driven (`cmd/semibase/main_test.go`,
  `internal/provision/*_test.go`); platform-specific halves live in build-tagged test files
- integration: CI steps against live servers; the repository has no database-touching unit tests
  (CLAUDE.md, Test section)

## Acceptance Evidence
- Linux CI, after the two `site` runs (`ci.yml:70-71`):
  `PGHOST=localhost PGUSER=postgres PGPASSWORD=ci-super psql -d postgres -tAc "SELECT pending_restart FROM pg_settings WHERE name = 'listen_addresses'"`
  prints `t`. The container is never restarted, so the value stays pending; this proves the delta
  is written.
- Windows CI, new step against the runner's PostgreSQL 17, with `$psql = "$env:PGBIN\psql.exe"`:
  1. `semibase site --port 5432 --expected-major 17 --restart-service postgresql-x64-17` exits 0,
     and its output contains `restarting service postgresql-x64-17`.
  2. `& $psql -h 127.0.0.1 -U postgres -d postgres -tAc "SHOW listen_addresses"` prints
     `localhost`, and `SELECT count(*) FROM pg_settings WHERE pending_restart` prints `0`.
  3. A second identical run exits 0, and its output does not contain `restarting service`.
- Unit tests: `bench --restart-service x` is a usage error; `--restart-service` with a
  non-loopback `--host` is a usage error; `site --restart-service x` with a loopback host parses.

## Solution Overview
- `listen_addresses` joins the `settings` list. It is postmaster-context, so it lands in
  `pending_restart` exactly like `shared_buffers`, and the existing reporting covers it.
- The restart happens inside the tuning phase, right after `pendingRestartSettings` returns a
  non-empty list, so `create` and `check` run against the restarted server and the tail's
  `reportPendingRestart` reads an empty list.
- Service control uses `golang.org/x/sys/windows/svc/mgr` behind a `windows` build tag, not
  `net stop`/`net start`: its errors do not depend on the Windows display language, it reads the
  service's start type, so a Disabled service is refused before any change, and it exposes the
  service state for bounded waits. `x/sys` is pure Go, so cross-compilation without cgo stays.
- The flag parses on every platform. The service is opened and checked at the start of
  `config`, before it connects: a wrong name, a missing right, a Disabled start type, or a
  non-Windows build (`service_other.go` returns "service restart is not supported on <GOOS>")
  fails with nothing changed. The module stays one cross-platform build: the Linux binary is what
  the docker benches run.
- Once Stop has been sent, Start is always sent, on a context detached from the command's
  cancellation and timeout (`context.WithoutCancel`) with its own bound. An interrupt or a timeout
  during the restart never leaves the service stopped.
- After the service reports Running, the tool reconnects in a bounded loop. Whether an EDB
  service reports Running before the postmaster accepts connections is unverified; the loop costs
  one successful connect when it does not.

## Technical Details
- `Options.RestartService string`; empty means no restart.
- `--restart-service <name>`, accepted by `site` only. `Validate` refuses it when the command
  is `bench` (no tuning, so nothing it does creates a pending restart) and when `--host` is not
  `localhost`, `127.0.0.1` or `::1`: after the restart the server listens on loopback only.
- Decision function `restartNeeded(pending []string, service string) bool`: true only when both
  are non-empty.
- Restart sequence in `config`:
  1. Close the phase connection.
  2. `Stop`, then wait for `Stopped`, bounded by 3 minutes: a shutdown checkpoint with
     `max_wal_size = 8GB` can be long.
  3. `Start` on `context.WithoutCancel(ctx)`, then wait for `Running`, bounded by 2 minutes. This
     step runs whether or not the stop wait succeeded.
  4. The ready wait connects to `postgres` as the superuser every second, bounded by 60 s. It
     returns the new connection, and the last connect error goes into the timeout message.
  5. `pendingRestartSettings` runs on the new connection. A non-empty list is an error naming the
     settings.
- Errors name the service and its last observed state.
- Output: `step("restarting service %s: %s waiting for a restart", name, list)`, then `ok` lines
  for stopped, running, accepting connections.

## What Goes Where
- Implementation Steps: code, tests, CI steps and docs in this repository.
- Post-Completion: checks on an installation machine.

## Implementation Steps

### Task 1: Listen on loopback only

**Files:**
- Modify: `internal/provision/config.go`
- Modify: `.github/workflows/ci.yml`

- [x] add `{"listen_addresses", "localhost"}` to `settings` (`config.go:16-28`)
- [x] add the Linux CI check from Acceptance Evidence after `ci.yml:71`; it is this task's test,
      since a unit test of a constant list proves nothing
- [x] run `go test ./...` and `go vet ./...` - must pass before task 2

### Task 2: Parse and validate `--restart-service`

**Files:**
- Modify: `internal/provision/provision.go`
- Modify: `cmd/semibase/main.go`
- Modify: `cmd/semibase/main_test.go`

- [ ] add `RestartService` and the command name `Validate` needs to `Options`
      (`provision.go:26-35`)
- [ ] extend `Validate` (`provision.go:39-44`) with the two refusals from Technical Details
- [ ] register `--restart-service` in `newFlagSet` (`main.go:158`); rewrite the comment at
      `main.go:154-155`, which stops being true; add the flag to `usage` (`main.go:20-55`)
- [ ] write table tests: `bench` rejects the flag, a non-loopback host rejects it, `site` with a
      loopback host accepts it, no flag parses as today
- [ ] run `go test ./...` - must pass before task 3

### Task 3: Restart the service and wait for the server

**Files:**
- Create: `internal/provision/service_windows.go`, `internal/provision/service_other.go`
- Modify: `internal/provision/config.go`
- Create: `internal/provision/config_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] add `golang.org/x/sys`; in `service_windows.go` implement opening and checking the service
      (exists, access, not Disabled) and the stop-start sequence with the bounds above
- [ ] `service_other.go` returns an error naming the platform from the same check function
- [ ] in `config`: check the service before connecting; after
      `pendingRestartSettings` (`config.go:60`) call `restartNeeded`, run the sequence, re-read
      on the new connection
- [ ] extract the ready wait over an injected `func(ctx) (*pgx.Conn, error)` dialer
- [ ] write tests: `restartNeeded` table; ready wait succeeds at once, succeeds after N failures,
      times out with the last error, stops on context cancellation
- [ ] run `go test ./...`, `go vet ./...`, `golangci-lint run`; `GOOS=linux go build ./...` -
      must pass before task 4

### Task 4: Exercise the restart on the Windows runner

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] add a step to the Windows job: `Set-Service postgresql-x64-17 -StartupType Manual`,
      `Start-Service postgresql-x64-17`
- [ ] run `semibase site` with `SEMIBASE_SUPER_PASSWORD=root`, writer and plot passwords set
      (the first run creates those roles), and the flags from Acceptance Evidence
- [ ] run the three Windows checks from Acceptance Evidence; this step is the only test of the
      service sequence itself
- [ ] push the branch and read the CI run - both jobs green before task 5

### Task 5: Verify acceptance criteria
- [ ] every item in Acceptance Evidence holds on the CI run
- [ ] `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `gofmt -l .` prints nothing

### Task 6: [Final] Update documentation
- [ ] `docs/architecture/provisioning.md:160-165`: the opt-in restart, where it runs, the
      Start guarantee, why CI's Linux job does not use it; `provisioning.md:94` follows
- [ ] `docs/architecture/configuration.md`: `listen_addresses` in the deltas table;
      `configuration.md:15-16` and `:29` stop calling `shared_buffers` the only restart setting;
      `configuration.md:57-58` becomes loopback only, single machine, and notes that
      `postgresql.auto.conf` overrides `postgresql.conf`
- [ ] `docs/architecture/README.md:22` and `docs/architecture/overview.md:61` (provisioning
      step 3) name the flag
- [ ] `docs/deployment.md`, in Russian: the flags table gets `--restart-service`; step 4 says the
      flag does the restart; one sentence that a restart drops the SCADA's connections, so the
      SCADA is stopped before a re-run on a working site
- [ ] "memory constants" and "memory tuning" become "server settings" in `CLAUDE.md`,
      `main.go:26`; "pure `pgx`" becomes "pure Go" in `CLAUDE.md`, `release.yml:124` and
      `Dockerfile:3-4`; the Run section of `CLAUDE.md` mentions the flag
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
- On an installation machine, confirm the Simple-Scada project connects to `localhost` or
  `127.0.0.1`. A project that names the machine's own IP or host name stops writing after the
  restart.

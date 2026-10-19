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
  prints `t`, and `SELECT setting FROM pg_file_settings WHERE name = 'listen_addresses' AND
  sourcefile LIKE '%postgresql.auto.conf'` prints `localhost`. The container is never restarted,
  so the value stays pending; this proves the delta is written.
- Windows CI, new step against the runner's PostgreSQL 17, with `$psql = "$env:PGBIN\psql.exe"`:
  1. `semibase site --port 5432 --expected-major 17 --restart-service postgresql-x64-17` exits 0,
     and its output contains `restarting service postgresql-x64-17`.
  2. `& $psql -h 127.0.0.1 -U postgres -d postgres -tAc "SHOW listen_addresses"` prints
     `localhost`, and `SELECT count(*) FROM pg_settings WHERE pending_restart` prints `0`.
  3. A second identical run exits 0, and its output does not contain `restarting service`.
  4. `site --restart-service` against the shipped Disabled service, and against
     `no-such-service`, exits 1 with no `ALTER SYSTEM SET` line in its output. The restart run
     follows `sc.exe start` with no other readiness wait, so it begins while the service may
     still be Start Pending.
- Unit tests: `bench --restart-service x` is a usage error; `--restart-service` with a
  non-loopback `--host` is a usage error; `site --restart-service x` with a loopback host parses;
  the restart sends Start after the Stop when the command context is already cancelled or
  expired.

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
- The flag parses on every platform, for `site` only. The service is opened and checked at the
  start of `config`, before it connects: a wrong name, a missing right, a Disabled start type, a
  state other than Running once a Start Pending has finished, or a non-Windows build
  (`service_other.go` returns "service restart is not supported on <GOOS>") fails with nothing
  changed. The module stays one cross-platform build: the Linux binary is what the docker benches
  run.
- The service must be the run's server. Once it is Running, `openService` connects, reads
  `SHOW data_directory` and the connection's remote IP:port, and compares the directory with the
  `-D` in the service's `PathName` (`windows.DecomposeCommandLine`, `filepath.Clean` on both
  sides, case ignored). A mismatch or a `PathName` with no `-D` fails with nothing changed: a Stop
  sent to another service would leave it stopped while the run's server keeps answering.
- Once the service accepts the Stop, the exit wait, the Start and the Running wait run on a
  context detached from the command's cancellation and timeout (`context.WithoutCancel`), each
  wait with its own bound. An interrupt or the command timeout during them does not stop the
  sequence.
- SCM Stopped does not prove the postmaster is gone: `pg_ctl runservice` reports Stopped after at
  most 12 x 5 s of a stop and leaves a slower postmaster running (`pg_ctl.c`, REL_17_STABLE). So
  the Start waits, within the stop bound, for a TCP connect to be refused. The probe dials the
  literal IP:port the identity check's connection reached: Go dials `localhost` to `::1` and
  `127.0.0.1` in parallel and reports the first family's error, so a non-refusal on one family
  would hide the refusal on the other. Each dial has a 15 s timeout, since Windows resends a SYN
  after an RST.
- `pg_ctl -w` without `-t` gives up on a startup after 60 s and reports Stopped while the
  postmaster keeps starting. Stopped after the Start therefore probes the port once: an answer
  names that timeout, a refusal names a postmaster that exited during its startup.
- Four cases still leave the service stopped, each with its recovery in
  `docs/architecture/provisioning.md#the-service-restart`: an old postmaster still running at the
  stop bound, a process ended by the console close, logoff, shutdown or a kill, a postmaster that
  exits during its startup, and a startup longer than `pg_ctl`'s 60 s wait.
- After the service reports Running, the tool reconnects in a bounded loop. An EDB service runs
  `pg_ctl runservice ... -w` and reports Running only once the postmaster accepts connections
  (read with CIM on the development machine, and printed by the Windows CI step for
  `postgresql-x64-17`: `pg_ctl.exe runservice -N "postgresql-x64-17" -D "C:\PostgreSQL\17\data" -w`),
  so the loop normally costs one connect.

## Technical Details
- `Options.RestartService string`; empty means no restart.
- `--restart-service <name>`, registered by `site` only: `bench` has no tuning, so nothing it
  does creates a pending restart, and the flag is undefined for it. `Validate` refuses it when
  `--host` is not `localhost`, `127.0.0.1` or `::1`: after the restart the server listens on
  loopback only.
- The restart runs when the pending list is non-empty and a service was opened; the condition
  sits inline in `config`'s switch.
- `openService` waits up to 2 minutes for a Start Pending service to reach Running, then refuses
  any state other than Running. It then calls `describeServer` (`config.go`), which connects as
  the superuser and returns `SHOW data_directory` and `RemoteAddr()`, and refuses a service whose
  `-D` differs or is absent. The address becomes the probe's target.
- Restart sequence in `config`:
  1. Close the phase connection.
  2. `Stop` on the command context. From here on, steps 3 and 4 run on
     `context.WithoutCancel(ctx)`.
  3. Wait for `Stopped`, then for a refused TCP connect to the identity check's IP:port
     (`WSAECONNREFUSED`; any other connect error is retried; each dial times out after 15 s),
     probing once a second, both bounded by 3 minutes from
     the Stop: a shutdown checkpoint with `max_wal_size = 8GB` can be long. If the wait gives up,
     Start is not sent and the error says the service will stay stopped and how to start it.
  4. `Start`, then wait for `Running`, bounded by 2 minutes from the Start. Stopped from the
     second query on fails at once; one more probe decides the message: an answer means
     `pg_ctl`'s 60 s startup wait ran out and the server runs outside the service, a refusal
     means the server exited during its startup and points to the server log, to removing the
     line it names from `postgresql.auto.conf` before `Start-Service`, and warns that a re-run
     writes the line again.
  5. The ready wait connects to `postgres` as the superuser every second, bounded by 60 s. It
     retries network errors and 57P03 only, returns the new connection, and the last connect
     error that is not a context error goes into the timeout message.
  6. `pendingRestartSettings` runs on the new connection. A non-empty list is an error naming the
     settings.
- Service control goes through a four-method seam over `*mgr.Service` (`Query`, `Control`,
  `Start`, `Config`), the port probe is a function field on `managedService`, and the server read
  is a function `openService` receives, so the check and the sequence are unit-tested with
  scripted states, configurations, servers and probe results.
- A wait that runs out its bound names the service and its last observed state.
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

- [x] add `RestartService` and the command name `Validate` needs to `Options`
      (`provision.go:26-35`)
- [x] extend `Validate` (`provision.go:39-44`) with the two refusals from Technical Details
- [x] register `--restart-service` in `newFlagSet` (`main.go:158`); rewrite the comment at
      `main.go:154-155`, which stops being true; add the flag to `usage` (`main.go:20-55`)
- [x] write table tests: `bench` rejects the flag, a non-loopback host rejects it, `site` with a
      loopback host accepts it, no flag parses as today
- [x] run `go test ./...` - must pass before task 3

### Task 3: Restart the service and wait for the server

**Files:**
- Create: `internal/provision/service_windows.go`, `internal/provision/service_other.go`
- Modify: `internal/provision/config.go`
- Create: `internal/provision/config_test.go`
- Modify: `go.mod`, `go.sum`

- [x] add `golang.org/x/sys`; in `service_windows.go` implement opening and checking the service
      (exists, access, not Disabled) and the stop-start sequence with the bounds above
- [x] `service_other.go` returns an error naming the platform from the same check function
- [x] in `config`: check the service before connecting; after
      `pendingRestartSettings` (`config.go:60`) call `restartNeeded`, run the sequence, re-read
      on the new connection
- [x] extract the ready wait over an injected `func(ctx) (*pgx.Conn, error)` dialer
- [x] write tests: `restartNeeded` table; ready wait succeeds at once, succeeds after N failures,
      times out with the last error, stops on context cancellation
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run`; `GOOS=linux go build ./...` -
      must pass before task 4

### Task 4: Exercise the restart on the Windows runner

**Files:**
- Modify: `.github/workflows/ci.yml`

- [x] add a step to the Windows job: `Set-Service postgresql-x64-17 -StartupType Manual`,
      `Start-Service postgresql-x64-17`
- [x] run `semibase site` with `SEMIBASE_SUPER_PASSWORD=root`, writer and plot passwords set
      (the first run creates those roles), and the flags from Acceptance Evidence
- [x] run the three Windows checks from Acceptance Evidence; this step is the only test of the
      service sequence itself
- [x] push the branch and read the CI run - both jobs green before task 5

### Task 5: Verify acceptance criteria
- [x] every item in Acceptance Evidence holds on the CI run (run 39249102852 at `9577716`, both
      jobs green: Linux `Require the listen_addresses delta` passed; Windows first `site` printed
      `restarting service postgresql-x64-17: listen_addresses, shared_buffers waiting for a
      restart`, the `SHOW`/`count(*)` check passed, the second run printed no restart line)
- [x] `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `gofmt -l .` prints nothing
      (locally `-race` needs cgo and this host has no C compiler, so `go test ./...` ran without
      it; both CI jobs run `go test -race ./...` green; `GOOS=linux go vet ./...` clean)

### Task 6: [Final] Update documentation
- [x] `docs/architecture/provisioning.md:160-165`: the opt-in restart, where it runs, the
      Start guarantee, why CI's Linux job does not use it; `provisioning.md:94` follows
      (the bullet now at `:183`, plus a new "The service restart" section and a table row)
- [x] `docs/architecture/configuration.md`: `listen_addresses` in the deltas table;
      `configuration.md:15-16` and `:29` stop calling `shared_buffers` the only restart setting;
      `configuration.md:57-58` becomes loopback only, single machine, and notes that
      `postgresql.auto.conf` overrides `postgresql.conf`
- [x] `docs/architecture/README.md:22` and `docs/architecture/overview.md:61` (provisioning
      step 3) name the flag
- [x] `docs/deployment.md`, in Russian: the flags table gets `--restart-service`; step 4 says the
      flag does the restart; one sentence that a restart drops the SCADA's connections, so the
      SCADA is stopped before a re-run on a working site
- [x] "memory constants" and "memory tuning" become "server settings" in `CLAUDE.md`,
      `main.go:26`; "pure `pgx`" becomes "pure Go" in `CLAUDE.md`, `release.yml:124` and
      `Dockerfile:3-4`; the Run section of `CLAUDE.md` mentions the flag (also the same phrases
      in `provision.go:65,77`, `README.md:25` and `provisioning.md:81`; the CLAUDE.md Test
      section gains both new CI steps and counts eight database steps)
- [ ] move this plan to `docs/plans/completed/` (left for the delivery step)

## Post-Completion
- On an installation machine, confirm the Simple-Scada project connects to `localhost` or
  `127.0.0.1`. A project that names the machine's own IP or host name stops writing after the
  restart.

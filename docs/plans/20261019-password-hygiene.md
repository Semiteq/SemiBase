# Passwords: flags and environment only, none in the server log

## Overview
- `semibase` takes passwords from flags and `SEMIBASE_*` environment variables only. The `.env`
  file reader goes: no caller remains once the installer passes passwords through its process
  environment, and the file would leave the superuser password in plaintext beside
  `{app}\semibase.exe`.
- Every connection `semibase` opens keeps its statements out of the server log, so
  `CREATE ROLE ... PASSWORD` and `ALTER ROLE ... PASSWORD` never reach `data\log`.
- A prerequisite of `20261019-windows-installer.md`, task 7.

## Context (from discovery)
- `cmd/semibase/main.go:75` calls `loadDotEnv`; `main.go:203-253` holds `dotEnvName`,
  `loadDotEnv`, `applyEnv`, `trimMatchingQuotes`. `main.go:237` applies a file value only when
  the variable is absent, so an inherited variable silently wins; `trimMatchingQuotes` and
  `TrimSpace` alter a password with matching edge quotes or edge whitespace.
- `cmd/semibase/env_test.go` (82 lines) tests only that reader. `.env.example` is its template.
- `cmd/semibase/main.go:42-43` (usage) and `main.go:191-201` (`resolvePasswordsFromEnvironment`,
  which stays).
- Callers after the installer plan, read 2026-10-19: the installer sets the variables in its
  process; CI sets them at job level (`.github/workflows/ci.yml:33-35,67`); SemiPlot's bench
  passes them with `docker -e` (`SemiPlot/SemiPlot.AppHost/AppHost.cs:31-32`,
  `SemiPlot/bench/provision.sh:18`). None reads a `.env`.
- `internal/provision/provision.go:110-124` — `connect` already sets two session settings in a
  loop (`provision.go:117`).
- PostgreSQL defaults `log_min_error_statement` to `error`, so the text of a failing statement is
  logged; `docs/deployment.md:174` recommends `log_statement = 'all'` for the SCADA's first
  start, which logs every statement.

## Development Approach
- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- all tests pass before the next task starts

## Testing Strategy
- unit tests beside the source; the server-log behaviour is checked by a CI step against the
  Linux service container

## Acceptance Evidence
- `grep -rn "dotEnv\|\.env" cmd/ internal/` prints nothing; `.env.example` does not exist.
- Linux CI, new step after the `site` runs: `ALTER SYSTEM SET log_statement = 'all'`, reload,
  run `semibase site` with `SEMIBASE_PLOT_PASSWORD` set to a random token, then
  `docker exec` into the service container and grep its log directory and `docker logs` for the
  token: no match.

## Solution Overview
- Delete the reader outright; no flag or fallback replaces it. A hand run sets the variables in
  its shell, as `docs/deployment.md`'s recovery section shows.
- `connect` appends `SET log_statement = 'none'` and `SET log_min_error_statement = 'panic'` to
  its session settings. Both are superuser-settable; every `semibase` connection that issues a
  password statement is a superuser connection. The `semiplot` login check also goes through
  `connect` (`check.go:408`), as `semiplot`, which may not set either parameter and issues no
  password statement: the two settings apply to superuser connections only.

## Implementation Steps

### Task 1: Remove the `.env` reader

**Files:**
- Modify: `cmd/semibase/main.go`
- Delete: `cmd/semibase/env_test.go`, `.env.example`
- Modify: `.gitignore`

- [x] delete the `loadDotEnv` call (`main.go:75`) and `main.go:203-253`
- [x] rewrite the usage text at `main.go:42-43`: flag wins over environment
- [x] delete `env_test.go`, `.env.example`, the `.env` line of `.gitignore`
- [x] the existing tests in `main_test.go`, `TestFlagUsageHidesPasswordValues` among them, still
      pass
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run` - must pass before task 2

### Task 2: Keep password statements out of the server log

**Files:**
- Modify: `internal/provision/provision.go`
- Modify: `internal/provision/provision_test.go`
- Modify: `.github/workflows/ci.yml`

- [x] in `connect` (`provision.go:110-124`), add the two `SET` statements for superuser
      connections (plus `SET log_min_duration_statement = -1`: `config.go` sets it to 1000)
- [x] write a test over the session-settings list that `connect` builds for the superuser and
      for `semiplot`
- [x] add the Linux CI step from Acceptance Evidence
- [x] run `go test ./...` and read the CI run - green before task 3

### Task 3: [Final] Update documentation
- [x] `CLAUDE.md` (Run section) and `docs/deployment.md:47-49,152`: passwords come from flags or
      environment; the `.env` sentences go
- [x] `CLAUDE.md` (Test section): the CI step list gains the server-log step
- [x] `docs/architecture/provisioning.md:103-108`: the same, plus the two session settings and
      why
- [x] move this plan to `docs/plans/completed/` (left for the delivery step)

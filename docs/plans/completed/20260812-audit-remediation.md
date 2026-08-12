# Audit remediation — umputun-style findings

## Overview

A multi-agent audit compared SemiBase against conventions extracted from three umputun Go
projects (reproxy, sys-agent, weblist) and confirmed 14 distinct findings. One is a security
defect: `semibase create -h` prints every configured password in cleartext. The rest are
correctness, robustness, and tooling gaps: dead error branches in flag parsing, an injection
gate living two packages away from the SQL it guards, hand-rolled literal escaping that is
only correct under `standard_conforming_strings=on`, no context cancellation, ANSI garbage in
redirected output, mojibake in service-restart errors, no version embedding, no linter, no CI.

This plan fixes all of them. The binary's behavior changes only where the current behavior is
wrong; every command keeps its name and flags. `--help` keeps working with exit 0.

## Context (from discovery)

All claims verified against the working tree at commit `fc5f8de` (verified 2026-08-12).

**Findings by file:**

- `cmd/semibase/main.go:83-89` — all four password flags bind `os.Getenv(...)` as the flag
  default; `flag.PrintDefaults` echoes non-empty defaults, so `-h` and any mistyped flag print
  the passwords. `.env` is loaded before parsing, so its secrets leak the same way. **high**
- `cmd/semibase/main.go:75` — `flag.ExitOnError`: `Parse` exits the process itself, the error
  branch at `:95-97` is dead for flag errors, and malformed-flag paths are untestable. **medium**
- `cmd/semibase/main.go:41,98` vs `internal/provision/provision.go:114,218` — the
  database-name regex gate lives only in the CLI package while `provision` interpolates
  `o.Database` into superuser DDL (`GRANT CONNECT ON DATABASE %s`, `CREATE DATABASE ` +
  concatenation). **medium**
- `internal/provision/provision.go:77-79,192,198` — `escapeLiteral` doubles single quotes only;
  wrong under `standard_conforming_strings=off` (backslash passwords stored wrong or injectable),
  and `provision_test.go` covers only the quote path. **medium**
- `cmd/semibase/main.go:138` — bare `context.Background()`, no `signal.NotifyContext`, no
  timeout anywhere; `provision.go:303` runs an unbounded `SELECT count(*) FROM public.tpdefault`
  as superuser (the 30s `statement_timeout` at `:121` is set on the reader role only). **medium**
- `internal/provision/console.go:30-42` — color constants are emitted unconditionally;
  `EnableColors` (`:20-27`) detects a redirected console but its result is discarded, so
  `semibase all > log.txt` gets raw `\x1b[` bytes. The doc comment at `:19` promises the
  opposite. **medium**
- `internal/provision/config.go:98,101` — `restartService` embeds raw multi-line `net.exe`
  output into the error string; on a Russian-locale Windows that output is CP866 and renders as
  mojibake inside a UTF-8 error. **low**
- `cmd/semibase/main.go` — no revision variable, no `--version`; a binary on a plant machine
  cannot report its build. **medium**
- repository root — no `.golangci.yml`; gosec G201/G202 would flag the identifier
  concatenation above for review. **medium**
- repository root — no CI workflow; tests run only from `.zed/tasks.json`. **medium**
- `cmd/semibase/main.go:52,139` — command allow-list duplicated across two switches; `run()`'s
  `default` arm silently maps an unmapped command to `All`, the most invasive path. **low**
- `cmd/semibase/main.go:47-48` vs `:57` — bare invocation prints usage to stdout with exit 2;
  the unknown-command path prints to stderr. Inconsistent error-path streams. **low**
- `cmd/semibase/main.go:107-113` — `loadDotEnv` swallows every `os.Open` error (not just
  not-exist) and never checks `scanner.Err()`; an unreadable `.env` silently degrades to a
  misleading "no password was given" failure. **low**
- `cmd/semibase/main.go:74,98` — `parseOptions` and the regex gate have no test, despite the
  regex being the injection gate in front of superuser DDL. **low**

**Patterns to follow (from the reference repos):** options resolved so that help shows env var
names, never values; `signal.NotifyContext` root context (weblist `main.go:97`); bounded
external calls; `var revision` + `--version` with `debug.ReadBuildInfo` fallback;
`.golangci.yml` with an explicit enable list; CI running build, race tests, and lint.

**Dependencies:** no new modules. `golang.org/x/sys` is already direct (`go.mod:7`) and its
`windows/svc/mgr` package replaces the `net.exe` shell-out. `pgx.Identifier` ships with pgx v5.
golangci-lint is not installed on this machine (`golangci-lint --version` → not found); Task 8
installs and pins it.

## Development Approach

- **testing approach**: Regular — implement, then add or update tests in the same task.
- Complete each task fully before moving to the next.
- Every task that changes code carries its own tests, listed as separate checklist items; a
  task whose deliverable has no unit-test path states the reason and the covering check.
- All tests pass (`go test ./...`) and `go vet ./...` is clean before the next task starts.
- `gofmt -w .` before presenting changes.
- Update this plan when scope changes during implementation.

## Testing Strategy

Unit tests beside the source, table-driven with `t.Run`, as in the existing
`provision_test.go` and `env_test.go`. The flag-parsing rework makes the CLI package testable
(`ContinueOnError` plus a `newFlagSet` seam), so it gains its first real test surface:
flag/env precedence, help behavior, command dispatch, usage output. The database-name gate is
tested once, in `provision`, where the pattern lives after this plan. Live-server behavior
(the `SET standard_conforming_strings` session setting, the service restart) is covered by the
dev-bench run in Post-Completion. `console_test.go` mutates package-level state, so it takes no
`t.Parallel()`; the colors-enabled case sets the package flag directly because `EnableColors`
cannot succeed under `go test`.

## Acceptance Evidence

Reproduced at commit `fc5f8de` (verified 2026-08-12) and measured after the fix:

1. **Password leak.** Before: `SEMIBASE_SUPER_PASSWORD=hunter2 go run ./cmd/semibase create -h 2>&1 | grep hunter2`
   prints the password inside `(default "hunter2")`. After: the same command produces no match —
   asserted by a unit test that builds the FlagSet via the `newFlagSet` seam with a poisoned
   environment, renders `PrintDefaults` into a buffer, and asserts the value is absent; the
   command above is the manual double-check.
2. **Color bleed.** Before: `go run ./cmd/semibase create > out.txt 2>&1` (no server needed —
   `step()` prints before `connect` fails) leaves `\x1b[` bytes in `out.txt`:
   `grep -c $'\x1b' out.txt` is non-zero. After: the same command yields zero; asserted by a
   unit test rendering the helpers with colors disabled.
3. **Help regression guard.** `go run ./cmd/semibase create --help` prints the flag list and
   exits 0 — asserted by a unit test on the `flag.ErrHelp` path.
4. **Silent `all` dispatch.** A unit test asserts an unmapped command returns an error instead
   of running `All`.
5. **Lint.** `golangci-lint run` exits 0 on the repository (tool installed and pinned in Task 8).
6. **Suite.** `go test ./...` and `go vet ./...` exit 0.
7. **Live proof (manual, needs the dev-bench password):**
   `semibase create --port 15432 --database semiplot_dev --expected-major 14` still provisions,
   and `semibase version` prints a revision.

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- Add newly discovered tasks with ➕.
- Document blockers with ⚠️.
- Keep this file in sync with the work actually done.

## Solution Overview

**The package that runs the SQL owns its invariants.** `provision.Options` gains `Validate()`
— the database-name identifier pattern moves here from the CLI and exists nowhere else —
called at the top of `Config`, `Create`, and `verify`. Identifier interpolation in DDL goes
through `pgx.Identifier{...}.Sanitize()`. `connect` issues
`SET standard_conforming_strings = on` after connecting, which makes the existing pure
`escapeLiteral` provably correct in every server mode — no per-role round-trips, the function
and its tests stay, and the session setting also covers the `ALTER SYSTEM` literals in
`config.go`. Role names are package constants and stay literal.

**The CLI surface is decided once.** Pre-dispatch words (`help`, `-h`, `--help`, `version`,
`--version`) are handled before the command map; the map
`map[string]func(context.Context, provision.Options) error` is the single source for both
validation and execution, and a miss is an error to stderr with exit 2 — never a silent `All`.
`ContinueOnError` makes `Parse` return its error; `flag.ErrHelp` maps to exit 0, everything
else to exit 2, and `main` alone exits. Password flags default to empty and resolve from the
environment after `Parse`, so `PrintDefaults` can only ever show env var names. A `newFlagSet`
function is the seam that lets tests render usage into a buffer.

**The process is cancellable and bounded.** `signal.NotifyContext(ctx, os.Interrupt)` roots
the context in `main`; the dispatch site wraps each command in `context.WithTimeout` with a
package constant (`phaseTimeout = 5 * time.Minute` — provisioning is DDL on empty objects,
generous is fine; a flag would add surface for a knob nobody asked for). Ctrl+C cancels the
in-flight query through pgx instead of hard-killing mid-DDL.

Console output gates every color constant on the `EnableColors` result and honors `NO_COLOR`.
`restartService` drops the `net.exe` shell-out for `x/sys/windows/svc/mgr` — stop, wait for
`Stopped`, start — which returns typed errors, so there is no console-encoding problem to
decode and no output parsing. `version`/`--version` print a `revision` variable with
`debug.ReadBuildInfo` fallback. `.golangci.yml` and a windows-runner CI workflow enforce all of
it from now on.

## Technical Details

- **Password resolution order** (unchanged semantics, new mechanism): flag if set, else process
  env, else `.env` (which `loadDotEnv` already merges into the process env at lowest
  precedence). Implementation: flag default `""`; after `Parse`,
  `if options.X == "" { options.X = os.Getenv("SEMIBASE_X") }`.
- **FlagSet seam:** `func newFlagSet(command string, options *provision.Options) *flag.FlagSet`
  with `ContinueOnError`; `parseOptions` wraps it; tests call `fs.SetOutput(&buf)` and
  `fs.PrintDefaults()` directly.
- **Identifier quoting:** `pgx.Identifier{o.Database}.Sanitize()` at the two DDL sites
  (`provision.go:114`, `:218`). Never on the connection URL — `url.URL.String()` already
  escapes the path, and `Sanitize()` would embed literal double quotes into the database name.
- **Conforming strings:** `connect` runs `SET standard_conforming_strings = on` (a USERSET GUC,
  no privilege needed) immediately after connecting; `escapeLiteral` keeps quote-doubling and
  gains backslash test cases documenting why the setting makes it sufficient.
- **Timeout:** package constant in `main`, applied at the dispatch site. No new flag.
- **Color gating:** `EnableColors` sets an unexported package var `colorsEnabled`; the four
  print helpers select empty strings when it is false. `NO_COLOR` set (any value) forces off.
  Helpers write through a package-level `io.Writer` defaulting to `os.Stdout` so tests capture
  output.
- **Service restart:** `mgr.Connect()` → `OpenService(name)` → `Control(svc.Stop)` → poll
  `Query()` until `svc.Stopped` (bounded by the phase context) → `Start()`. Typed errors,
  single-line messages, no `os/exec`, no `golang.org/x/text`.
- **Version:** `var revision = "unknown"` in `main`, overridable with
  `-ldflags "-X main.revision=..."`, falling back to `debug.ReadBuildInfo` (`vcs.revision` +
  dirty suffix) when unset. Printed by `version`/`--version` only — no startup banner.
- **`.golangci.yml`:** v2 config, explicit enable list: `govet, staticcheck, revive, gosec,
  errorlint, gocritic, testifylint, modernize`; `_test.go` relaxations for gosec.
  ASSUMPTION: the exact linter set compiles against the pinned golangci-lint version — adjust
  the list at execution time rather than fighting it. gosec G201 will still fire on
  `fmt.Sprintf`-built DDL even after sanitization (`config.go:78`, `provision.go:114,218`);
  those sites get `//nolint:gosec // <reason>` (values are package constants or
  driver-sanitized) rather than a blanket exclude.
- **CI:** `.github/workflows/ci.yml`, `windows-latest` (the `x/sys/windows` imports do not
  build on a Linux runner), steps: checkout, setup-go from `go.mod`, `go build ./...`,
  `go test -race ./...`, golangci-lint action pinned to the same version as the local install.

## What Goes Where

- **Implementation Steps** — code, tests, lint config, CI file, docs.
- **Post-Completion** — the live dev-bench run (needs the postgres password), pushing to
  GitHub so CI actually executes, release ldflags wiring when a release process appears.

## Implementation Steps

### Task 1: Provision package owns validation and safe quoting

**Files:**
- Modify: `internal/provision/provision.go`
- Modify: `internal/provision/config.go`
- Modify: `internal/provision/provision_test.go`

- [x] add `Options.Validate()` checking `Database` against the identifier pattern
      (`^[a-z_][a-z0-9_]*$`, moved from `cmd/semibase/main.go:41`); call it first in `Config`
      (`config.go:64`), `Create`, and `verify`
- [x] quote the two DDL interpolations of `o.Database` with `pgx.Identifier{...}.Sanitize()`
      (`provision.go:114`, `:218`); leave the connection URL untouched
- [x] `connect` executes `SET standard_conforming_strings = on` after connecting
      (`provision.go:63-75`), making `escapeLiteral`'s quote-doubling sufficient in every
      server mode; add a comment stating that invariant on `escapeLiteral`
- [x] write table tests for `Validate()`: accepts valid names; rejects uppercase, hyphen,
      leading digit, quote, semicolon, empty
- [x] extend `TestEscapeLiteral` with backslash cases (`\`, `\'`) documenting the
      conforming-strings dependency
- [x] write a test asserting the sanitized identifier form for a hostile database name
- [x] run tests — must pass before Task 2

### Task 2: Rework the CLI surface once — no secrets in defaults, testable errors, single dispatch

**Files:**
- Modify: `cmd/semibase/main.go`
- Create: `cmd/semibase/main_test.go`

- [x] extract `newFlagSet(command string, options *provision.Options) *flag.FlagSet` using
      `flag.ContinueOnError` with output to `os.Stderr`; `parseOptions` wraps it
      (`main.go:74-101`)
- [x] `errors.Is(err, flag.ErrHelp)` → exit 0 with no extra output (usage already printed);
      any other parse error → exit 2; `main` alone calls `os.Exit`
- [x] password flags default to `""`; resolve each empty field from its `SEMIBASE_*` env var
      after `Parse`; flag descriptions keep naming the env var (`main.go:83-89`)
- [x] replace the two switches (`main.go:52,139`) with pre-dispatch words
      (`help`, `-h`, `--help` → usage, exit 0; `version`, `--version` → stub printing
      `revision`, wired fully in Task 7) plus one
      `map[string]func(context.Context, provision.Options) error` for the four commands;
      unknown command → usage to stderr, exit 2 — never `All`
- [x] bare invocation prints usage to stderr with exit 2, consistent with the unknown-command
      path (`main.go:47-48` vs `:57`)
- [x] delete `databaseNamePattern` from the CLI; `parseOptions` calls `options.Validate()`
      after `Parse` for the friendly early message (pattern now lives only in `provision`)
- [x] write a test that sets a poisoned password env var, renders `newFlagSet` usage via
      `PrintDefaults` into a buffer, and asserts the password value does not appear
- [x] write table tests for `parseOptions`: flag beats env, env fills empty flag, invalid
      database name rejected via `Validate`, unknown flag returns an error, `--help` returns
      `flag.ErrHelp`
- [x] write a test asserting an unmapped command name yields the error path, not `All`
- [x] run tests — must pass before Task 3

### Task 3: Report real .env errors

**Files:**
- Modify: `cmd/semibase/main.go`
- Modify: `cmd/semibase/env_test.go`

- [x] `loadDotEnv` distinguishes `fs.ErrNotExist` (silent, normal) from any other open error,
      and checks `scanner.Err()`, reporting both to stderr with the file name
      (`main.go:107-113`)
- [x] give the loader a path-or-chdir seam: tests use `t.Chdir(t.TempDir())`
- [x] write a test: a directory named `.env` surfaces the `scanner.Err()` warning path on
      Windows (`os.Open` on a directory succeeds; the read fails); an absent file stays
      silent. The non-`ErrNotExist` open branch has no cheap Windows test — covered by
      inspection
- [x] run tests — must pass before Task 4

### Task 4: Cancellable, bounded context

**Files:**
- Modify: `cmd/semibase/main.go`
- Modify: `cmd/semibase/main_test.go`

- [x] root context: `signal.NotifyContext(context.Background(), os.Interrupt)` in `main`
      (`main.go:138`)
- [x] `phaseTimeout = 5 * time.Minute` package constant; the dispatch site wraps each command
      context in `context.WithTimeout`, covering the unbounded superuser scan at
      `provision.go:303`
- [x] write a dispatch-level test with a stub command func asserting the context it receives
      carries a deadline
- [x] write a cancelled-context test: valid `Database` (so `Validate` passes), pre-cancelled
      context, assert `errors.Is(err, context.Canceled)` and prompt return
- [x] run tests — must pass before Task 5

### Task 5: Gate colors on console capability

**Files:**
- Modify: `internal/provision/console.go`
- Create: `internal/provision/console_test.go`

- [x] `EnableColors` records success in a package-level `colorsEnabled`; the `step/ok/warn/note`
      helpers emit empty strings for every color when it is false (`console.go:30-42`)
- [x] honor `NO_COLOR`: when set (any value), colors stay off even if the console probe
      succeeds
- [x] helpers write through a package-level `io.Writer` defaulting to `os.Stdout`
- [x] write tests (no `t.Parallel()` — package state): colors disabled → captured output has
      no `\x1b`; colors force-enabled by setting the package flag directly → it does;
      `NO_COLOR` wins
- [x] run tests — must pass before Task 6

### Task 6: Service restart through the service manager

**Files:**
- Modify: `internal/provision/config.go`

- [x] replace the `net stop`/`net start` shell-out (`config.go:96-104`) with
      `x/sys/windows/svc/mgr`: connect, open the service, `Control(svc.Stop)`, poll `Query()`
      until `svc.Stopped` bounded by the phase context, `Start()`
- [x] errors are single-line and wrapped (`stopping service %s: %w`); no process output is
      embedded — the CP866 problem is removed, not decoded
- [x] no unit-test path: the service manager requires a real elevated session; covered by the
      live restart in Post-Completion and by inspection. `go vet` and lint still gate the code
- [x] ➕ review fix: `Start()` now polls until `Running` (StartService returns at
      START_PENDING) and fails when the service falls back to `Stopped`; an already-stopped
      service skips the stop, keeping the restart idempotent
- [x] run tests — must pass before Task 7

### Task 7: Version embedding

**Files:**
- Modify: `cmd/semibase/main.go`
- Modify: `cmd/semibase/main_test.go`

- [x] `var revision = "unknown"` in `main`; resolve through `debug.ReadBuildInfo`
      (`vcs.revision` + dirty suffix) when the ldflags value is unset
- [x] wire the Task 2 `version`/`--version` stub to print it; no startup banner on other
      commands
- [x] write tests: the resolver returns the ldflags value verbatim when set, and a non-empty
      string from the fallback path when not
- [x] run tests — must pass before Task 8

### Task 8: Linter installed, pinned, configured

**Files:**
- Create: `.golangci.yml`

- [x] install golangci-lint (winget or `go install`, pick what pins cleanly) and record the
      chosen version — it is not present on this machine. Installed via
      `winget install GolangCI.golangci-lint --version 2.12.2` → golangci-lint 2.12.2
- [x] write `.golangci.yml` with the explicit enable list from Technical Details and `_test.go`
      gosec relaxation; adjust linter names against the installed version if any fail to load
      (all eight names load in 2.12.2 unchanged)
- [x] run `golangci-lint run`; fix every finding or add `//nolint:gosec // <reason>` at the
      three known Sprintf-DDL sites (`config.go:78`, `provision.go:114,218`). gosec G201 did
      not fire on the Sprintf-DDL sites; the real findings were G104 (unhandled `os.Setenv`
      in `applyEnv` — now handled; ignored `conn.Close` on the connect error path — explicit
      `_ =`), G103/G115 in `totalPhysicalMemoryMB` (nolint with reason: Win32 calling
      convention, MB fits int), and revive `redefines-builtin-id` in `console_test.go`
      (parameter renamed `print` → `emit`)
- [x] no unit tests in this task: the deliverable is a clean lint run, asserted again in
      Task 10
- [x] run `go test ./...` — must still pass before Task 9

### Task 9: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [x] workflow on push and pull_request: `windows-latest` (the `x/sys/windows` imports do not
      build on Linux), setup-go from `go.mod`, `go build ./...`, `go test -race ./...`,
      golangci-lint action pinned to Task 8's version (`v2.12.2`)
- [x] confirm every referenced action version exists; parse the YAML locally (PowerShell
      `ConvertFrom-Yaml` or equivalent) before committing — verified via `gh api`
      (`actions/checkout@v7`, `actions/setup-go@v7`, `golangci/golangci-lint-action@v9`,
      golangci-lint release `v2.12.2`); `ConvertFrom-Yaml` is absent on this machine, parsed
      with a scratch Go program on `gopkg.in/yaml.v3` instead
- [x] no unit tests in this task: the workflow proves itself on first push (Post-Completion)

### Task 10: Verify acceptance criteria

- [x] every check in Acceptance Evidence runs and produces the stated result, including the
      poisoned-env grep, the `create`-redirect grep, and the `--help` exit-0 check
      (poisoned-env grep: no match; redirect grep: 0 escape bytes; `create --help` exits 0;
      unmapped-command error path asserted by the Task 2 unit test)
- [x] `go test ./...`, `go vet ./...`, `golangci-lint run` — all exit 0
- [x] `gofmt -l .` reports nothing
- [x] build `semibase.exe`; `--help` prints no password; `version` prints a revision
      (prints the vcs revision `876654d2…` via the `debug.ReadBuildInfo` fallback)

### Task 11: Update documentation

- [x] `CLAUDE.md`: `version` command in Run, lint install + `golangci-lint run` in Format/Test,
      release build line
      `go build -ldflags "-X main.revision=<rev>" -o semibase.exe ./cmd/semibase`, CI note
- [x] `docs/deployment.md` (stays Russian, no architecture links): add a `version` row to the
      «Команды» table (lines 25-33); mention `semibase version` in support context
- [x] `docs/architecture/provisioning.md`: validation lives in the provision package; quoting
      via `pgx.Identifier` + `standard_conforming_strings`; context timeout; service-manager
      restart
- [ ] move this plan to `docs/plans/completed/` (deferred to delivery)

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification**

- Live dev-bench run (needs the postgres password):
  `semibase create --port 15432 --database semiplot_dev --expected-major 14` — proves the
  conforming-strings session setting and identifier sanitization against a real server,
  including a password containing `'` and `\`.
- Live service restart on a dedicated instance (elevated session):
  `semibase config --service <name>` stops and starts the service through the service manager.
- `semibase all > log.txt` on a real console — plain text in the file, colors on the terminal.

**External systems**

- First push to `github.com/Semiteq/SemiBase` executes the CI workflow; a green run is the
  workflow's acceptance. Nothing is pushed as part of this plan.
- Release ldflags wiring belongs to whatever release process the repository adopts later; until
  then `debug.ReadBuildInfo` covers locally built binaries.

**Executed by exec:**

- branch: audit-remediation

# Setup without the EDB registration

## Overview

- Setup finds the PostgreSQL it works with in the Windows service list, not in EDB's
  `HKLM\SOFTWARE\PostgreSQL` keys. A PostgreSQL service is one whose command line runs
  `pg_ctl.exe runservice ... -D <data>`; everything else Setup needs follows from that line and
  from the files in the data folder.
- The registry-only states go away: the incomplete registration, the orphaned registration
  (no Service ID or no Port) and the registered port nothing answers on. A PostgreSQL installed
  without EDB (a zip, another vendor's bundle) is found the same way as EDB's.
- The superuser is `postgres` on every machine. A server whose superuser has another name is
  outside what Setup configures; the README requirements say so.
- The detection lives in the `semibase` binary as a fourth command, `semibase engine`, which
  Setup already carries and extracts. Setup keeps the texts and the decisions about what to
  show; the binary reports the facts and the state.

## Context (from discovery)

- `Installer/SemiBase.iss` reads the registration in `RegisteredEngineRefusal`
  (`Installer/SemiBase.iss:415-464`: Version, Super User, Data Directory, Service ID,
  Base Directory, then the Port under `SOFTWARE\PostgreSQL\Services`), lists other majors in
  `RegisteredInstallations` and `OtherMajorRefusal` (`:467-493`), and decides fresh versus
  registered in `EngineIsFresh` (`:527-530`), `RegistryRefusal` (`:534-540`) and
  `EngineRefusal` (`:543-553`). The key names are constants at `:195-198`; `TEngine` at
  `:201-207` carries ServiceName, Port, SuperUser, BaseDirectory, DataDirectory.
- Those functions run many times per wizard: `InitializeWizard` (`:715`, `:732`),
  `ShouldSkipPage` (`:752`), `NextButtonClick` (`:765`), `UpdateReadyMemo` (`:791`, `:812`,
  which Inno runs on every visit to the Ready page, `:781-784`) and `PrepareToInstall`
  (`:844`, `:852`, and the re-read of the fresh install's own registration at `:858`).
- Registry wording reaches the setup log through detail keys: `IncompleteVersion` to
  `IncompleteInstallationDetail` (`:84-88`), `OrphanedRegistrationDetail` (`:93`),
  `OtherMajorDetail` (`:96`), `NoRegistrationDetail` (`:105`, joined with `PortOwnerDetail` at
  `:511`), `SitePortWrongDetail` (`:130`). `UninstallerFolder` (`:294-301`) falls back to EDB's
  prefix when Base Directory is missing.
- The superuser comes from the registration's Super User (`:423`, passed to site at `:655`).
  `site` defaults `--superuser` to `postgres` (`cmd/semibase/main.go:169`). A missing role
  answers a password login with SQLSTATE 28P01, which `AppendSiteLine` (`:630`) maps to
  `SitePasswordWrong`.
- The registered-port refusal (`SitePortWrong`, `Installer/SemiBase.iss:130-132`, detected in
  `AppendSiteLine` at `:627-636` and raised at `:662-665`) exists because the registry's Port
  could disagree with the server. The CI case at `.github/workflows/ci.yml:510-520` is the only
  one that reaches a site failure after the connection.
- `semibase.exe` is the first `[Files]` entry (`Installer/SemiBase.iss:186`), so under solid
  compression `ExtractTemporaryFile` reaches it without decompressing the EDB payload;
  `KeepToolRefusal` already extracts it (`:307`). The Inno Setup help topic
  "ExtractTemporaryFile" shows the call in `InitializeSetup`.
- The Go side already parses a service command line: `dataDirectoryFlag` in
  `internal/provision/service_windows.go:149-164`, through `windows.DecomposeCommandLine`
  (`golang.org/x/sys@v0.47.0/windows/exec_windows.go:147`, Windows-only); that file and its test
  carry `//go:build windows` (`service_windows.go:1`, `service_windows_test.go:1`), and the
  Linux CI job runs `go test -race ./...` (`ci.yml:136`). `golang.org/x/sys` is pinned at
  v0.47.0 (`go.mod:7`).
- `mgr.Connect` opens the Service Control Manager with `SC_MANAGER_ALL_ACCESS` and
  `Mgr.OpenService` asks for `SERVICE_ALL_ACCESS` (`x/sys@v0.47.0/windows/svc/mgr/mgr.go:43`,
  `:195`); `Mgr.Handle` is exported (`mgr.go:24`), so a handle opened with narrower rights
  through `windows.OpenSCManager` and `windows.OpenService` can back a `mgr.Mgr`.
- Commands are dispatched in `cmd/semibase/main.go:72-75` (`site`, `bench`) and `:90-97`
  (`help`, `version`); the usage text is `:18-58`.
- CI fakes the registry in the step at `.github/workflows/ci.yml:395-431` (another major, an
  incomplete 17 key, a 17 key without a service; with a token superuser password checked absent
  from the logs at `:400-409`), the details at `:459` and `:466`, and the registered-port case
  at `:510-520`, which restores the Port in `finally` (`:516`). The diagnostics step dumps the
  registry at `:607`. `installer-case.ps1` reads Base Directory from the registry in
  `Start-PostgreSQLRemoval` (`.github/scripts/installer-case.ps1:97-104`) and requires the
  empty keys in `Assert-NothingRegistered` (`:74-79`, called at `:124` after each EDB removal
  and at `ci.yml:398`).
- Registry wording in the docs: `docs/architecture/installer.md:9`, `:29`, `:35`, `:63`, `:99`,
  the engine table `:134-141`, `:149-152`, `:194`, `:208`, `:221-223`, the registry list
  `:361-369`, `:442-445`, `:468`, `:521`; `docs/architecture/verification.md:101`, `:105`,
  `:109`, `:115-116`, `:127-129`, `:147`; the README section «PostgreSQL 17 без имени службы
  или порта», which reads `Super User` from the registry.
- Measured on the development machine, 2026-10-27, read-only:
  - two pg_ctl services: `postgresql-14` (`-D "C:\Program Files\PostgreSQL\14\data"`) and
    `postgresql-x64-17` (`"C:\Program Files\PostgreSQL\17\bin\pg_ctl.exe" runservice -N
    "postgresql-x64-17" -D "C:\Program Files\PostgreSQL\17\data" -w`), both Auto and Running;
  - `PG_VERSION` 14 and 17;
  - `postgresql.conf:64` holds `port = 15432` for 14 and `port = 5432` for 17; neither
    `postgresql.auto.conf` sets a port;
  - `postmaster.pid` line 4 holds the listening port (15432, 5432), line 2 the data folder;
  - `uninstall-postgresql.exe` exists in the 17 folder and not in the 14 folder;
  - the shell is not elevated (`net session` fails).

## Development Approach

- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run `go test ./...`, `go vet ./...` and `golangci-lint run` after each Go change
- no task runs `SemiBase-Setup.exe` on the development machine, a scratch copy included; the
  setup's cases run in the CI `installer` job only
- a commit and a push happen only after the operator approves them

## Testing Strategy

- **unit tests**: table-driven, beside the source. Untagged files hold conf and pid parsing and
  `Classify`, so the state table is tested on Linux and Windows; the command-line recognition
  and its tests are `//go:build windows`.
- **integration**: the Windows `build` job runs `semibase engine` against the runner's own
  service; the `installer` job's cases exercise every state through Setup with fake services
  that `New-FakeService` (`.github/scripts/installer-case.ps1`) creates through `New-Service`,
  which are never started.
- No UI e2e framework exists; the installer cases are the e2e layer.

## Acceptance Evidence

Today the defect reproduces in CI: the step `Refuse another major, an incomplete 17 key and a
17 key without a service` (`ci.yml:395-431`) passes only because Setup reads the hand-made
`HKLM\SOFTWARE\PostgreSQL` keys, and a PostgreSQL service without those keys is invisible to it.

After the change:

1. `grep -cF 'SOFTWARE\PostgreSQL' Installer/SemiBase.iss` prints `0` (a fixed-string match:
   the escaped regex form prints `0` under Git Bash on master too).

   Verified 2026-10-27: prints `0`; the same command on master `089022e` prints `3`, and
   `grep -niE 'HKLM|RegQuery|RegGetSubkey|RegKeyExists'` finds nothing in the file.
2. `go test ./internal/engine/... ./internal/provision/... ./cmd/semibase/...` passes on Windows
   and in the Linux CI job, with the classification table covering every row of the state table.

   Verified 2026-10-27: on Windows, `go test -count=1 ./...` prints `ok` for the three
   packages. `TestClassify` has subtests for all seven states: `ambiguous` (two 17 services; an
   unknown major beside a good 17; an unqueried `postgresql*` service beside a good 17),
   `incomplete` (a problem; a problem beside an unreadable setting), `unreadable` (a 17 whose
   `postgresql.conf` does not parse; an unqueried `postgresql*` service alone), `port-mismatch`
   (a conf port, a command-line port, the default), `found` (running, stopped, 14 beside 17, a
   broken other major beside 17), `other-major`, `fresh`. CI run 40533842262 on `76111e7`, job
   `linux`, step `Test` (`go test -race ./...`): `ok github.com/Semiteq/SemiBase/cmd/semibase
   1.025s`, `ok github.com/Semiteq/SemiBase/internal/engine 1.032s`, `ok
   github.com/Semiteq/SemiBase/internal/provision (cached)`.
3. Read-only and unelevated, on the development machine: `go build -o semibase.exe
   ./cmd/semibase` then `.\semibase.exe engine --expected-major 17` prints `state=found`,
   `service=postgresql-x64-17`, `port=5432`, `data=C:\Program Files\PostgreSQL\17\data`, with
   the `postgresql-14` service beside it and no other service named in `detail=`.

   Verified 2026-10-27: `net session` exits 2 (not elevated); `Get-Service postgresql*` shows
   `postgresql-14` and `postgresql-x64-17`, both Running. The command exits 0 and prints the
   ten lines recorded under Task 3, byte for byte; `detail=service postgresql-x64-17 runs
   PostgreSQL 17 from C:\Program Files\PostgreSQL\17\data on port 5432` names no other service.
4. The CI `build` job requires `semibase engine --expected-major 17` on the runner to print
   `state=found` and `service=postgresql-x64-17`.

   Verified 2026-10-27: run 40533842262 on `76111e7`, job `build`, step `Find the runner's
   PostgreSQL service through engine` passes and prints `state=found`,
   `service=postgresql-x64-17`, `base=C:\Program Files\PostgreSQL\17`,
   `data=C:\PostgreSQL\17\data`, `port=5432`,
   `port-file=C:\PostgreSQL\17\data\postgresql.conf`, `listening=5432`, `uninstaller=yes`,
   `unreadable=`. The same job's `Test` step runs `TestDetectReadsTheServiceList`, which asserts
   `found` for `postgresql-x64-17` only when `GITHUB_ACTIONS` is set.
5. The CI `installer` job passes with these cases, each asserted by `Assert-Refused` on the
   English text and detail, or by exit 0:
   - a fake `postgresql-ci-14` service with `PG_VERSION` 14 and no 17 service: exit 7,
     `OtherMajor`; the token superuser password is absent from every log;
   - a fake `postgresql-ci-17` service whose data folder has no `PG_VERSION`: exit 7,
     `IncompleteInstallation`, detail naming the service and `no file <data>\PG_VERSION`;
   - a fake `postgresql-ci-17` service whose command line has no `-D`: exit 7,
     `IncompleteInstallationNoUninstaller`, detail `service postgresql-ci-17 runs pg_ctl.exe
     with no -D`;
   - a fake `postgresql-ci-17` service whose `postgresql.conf` holds `port = '5432`: exit 7,
     `UnreadableService`, naming the file, line 1 and the unterminated quoted value;
   - a fake service whose data folder path has Cyrillic letters: its `data=` reaches the setup
     log unchanged;
   - the installed 17 with a fake `postgresql-ci-14` beside it: exit 0;
   - the installed 17 with a second fake 17 service that has `PG_VERSION`: exit 7,
     `AmbiguousEngine` naming both services;
   - the installed 17 with `port = 15433` appended to `postgresql.conf` and no restart: exit 7,
     `PortMismatch` with `5432` and `15433`; the file is restored in `finally` before the
     assertion;
   - the installed 17 with `-o "-p 15433"` appended to its service command line and no restart:
     exit 7, `PortMismatchNoFile`; the command line is restored in `finally` before the
     assertion;
   - the installed 17 with `pg_hba.conf` rejecting `postgres` on `127.0.0.1/32` and `::1/128`:
     exit 7, `SiteFailed`, the exit code in the detail; the file is restored in `finally`, and
     each new backend reads it with no reload or restart;
   - `SEMIBASE_SUPER_PASSWORD=wrong`: exit 7, `SitePasswordWrong`, the site log path in the
     detail;
   - the busy-port and non-empty-folder cases keep their texts, their detail starting with
     `no service runs pg_ctl.exe runservice`;
   - after each case no `postgresql-ci-*` service remains, and before the clean install
     `Assert-NoPostgresService` still holds.

   The refusal and detail each case asserts, as `Installer/SemiBase.iss` and
   `internal/engine/engine.go` print them on 2026-10-27 (`<fake>` is
   `D:\a\_temp\fake-services`):
   - other major: `PostgreSQL of another version is installed, and Setup works with PostgreSQL
     17 only. ...` / `no service runs PostgreSQL 17; postgresql-ci-14 runs PostgreSQL 14`;
   - no `PG_VERSION`: `The PostgreSQL service postgresql-ci-17 is installed incompletely. Remove
     PostgreSQL with <fake>\17\uninstall-postgresql.exe, ...` / `service postgresql-ci-17: no
     file <fake>\17\data\PG_VERSION`;
   - no `-D`: `The PostgreSQL service postgresql-ci-17 is installed incompletely. Open Command
     Prompt with Run as administrator, run sc.exe stop postgresql-ci-17 and sc.exe delete
     postgresql-ci-17, ...` / `service postgresql-ci-17 runs pg_ctl.exe with no -D`;
   - unreadable conf: `Setup could not read the settings of the PostgreSQL service
     postgresql-ci-17: <fake>\conf\data\postgresql.conf line 1: syntax error: an unterminated
     quoted value. ...` / `service postgresql-ci-17: <fake>\conf\data\postgresql.conf line 1:
     syntax error: an unterminated quoted value`;
   - Cyrillic: `service postgresql-ci-17: no file <fake>\данныеё\PG_VERSION`;
   - 14 beside 17: exit 0, site log holding `Done: site completed against
     localhost:5432/scada_archive.`;
   - second 17: `Several services that run PostgreSQL 17 or PostgreSQL of an unknown version are
     found: postgresql-x64-17, postgresql-ci-17. ...` / `services postgresql-x64-17,
     postgresql-ci-17 run PostgreSQL 17 or a server of unknown major`;
   - conf port mismatch: `PostgreSQL 17 runs on port 5432, and the file C:\Program
     Files\PostgreSQL\17\data\postgresql.conf sets port 15433. ...` / `service
     postgresql-x64-17 listens on port 5432, and C:\Program
     Files\PostgreSQL\17\data\postgresql.conf sets port 15433`;
   - command-line port mismatch: `PostgreSQL 17 runs on port 5432, and the service
     postgresql-x64-17 starts it on port 15433. ...` / `service postgresql-x64-17 listens on port
     5432, and the service command line sets port 15433`;
   - rejected administrator: `The semibase.exe tool failed. Fix the error named in the last lines
     of the log C:\Program Files\SemiBase\Logs\site-<stamp>.log, ...` / `semibase.exe exit
     code 1.`, and the site log names SQLSTATE 28000; `finally` restores `pg_hba.conf`, and
     `postgres` logs in afterwards with no restart (run 40539642752 on 823e5a8);
   - wrong password: `Wrong password of the PostgreSQL administrator (database user postgres).
     ...` / `semibase.exe site log: C:\Program Files\SemiBase\Logs\site-<stamp>.log.`;
   - busy port: `Port 5432 is busy, and no installed PostgreSQL is found. ...` / `no service runs
     pg_ctl.exe runservice; port 5432 held by pwsh (pid <the step's $PID>)`; non-empty folder:
     `The folder C:\Program Files\PostgreSQL\17\data is not empty, ...` / `no service runs
     pg_ctl.exe runservice`;
   - `Assert-NoFakeService` passes after the 14-beside-17 and second-17 cases, and
     `Assert-NoPostgresService` passes after the fake-service step and before the clean install.

   Verified 2026-10-27: run 40533842262 on `76111e7`, job `installer`, passes and prints each
   refusal above word for word, with `pid 7148` and the stamps `20261027-154726` and
   `20261027-154730`. Its other-major, busy-port and non-empty-folder details are the same
   clauses in the form of `76111e7`, capitalized and closed with a period: `No service runs
   PostgreSQL 17; postgresql-ci-14 runs PostgreSQL 14.`, `No service runs pg_ctl.exe runservice.
   Port 5432 held by pwsh (pid 7148).` and `No service runs pg_ctl.exe runservice.`.

   Only `InitializeSetup` and `PrepareToInstall` spawn `semibase.exe`
   (`Installer/SemiBase.iss`). `DetectEngine`, the one `ExecAndCaptureOutput` of the binary, is
   called once in `InitializeSetup` and twice in `PrepareToInstall`, before `EngineRefusal` and
   after `InstallEngineRefusal`. The only other spawn is `site` in `SiteRefusal`, called from
   `PrepareToInstall`. `InitializeWizard`, `ShouldSkipPage`, `NextButtonClick` and
   `UpdateReadyMemo` read the `Detected` global. The `KeepToolRefusal` call in `UpdateReadyMemo`
   extracts and copies the binary and does not run it.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

- `semibase engine --expected-major <n>` opens the Service Control Manager with
  `SC_MANAGER_CONNECT | SC_MANAGER_ENUMERATE_SERVICE` and each service with
  `SERVICE_QUERY_CONFIG | SERVICE_QUERY_STATUS`, so it runs unelevated. It keeps the services
  whose command line runs `pg_ctl.exe` with `runservice`, describes each one, classifies the set
  and prints the result as `key=value` lines. It opens no connection and controls no service.
- The detection is Go, not Pascal Script, because the command-line parser already exists in Go
  (`dataDirectoryFlag`) and one parser lives in one place, and because the classification gets
  table-driven tests that Pascal Script cannot have.
- Setup runs the command once in `InitializeSetup` into a `[Code]` global, and twice in
  `PrepareToInstall`: before changing the machine, and after the EDB install to find the service
  EDB created. Every wizard function reads the global; none spawns.
- The major comes from `PG_VERSION` alone. A pg_ctl service without a readable `PG_VERSION`
  has no known major, and Setup treats it as a broken install of the expected major: installing
  a second server beside a PostgreSQL service Setup cannot place is the wrong action.
- The port Setup connects to is the server's configured port. When the service runs on another
  port, the configured one takes effect only after a restart, which site would perform: Setup
  refuses and names both ports rather than connecting to the running one.
- `SitePortWrong` and its `connectex` detection go: the registry port that could be wrong is
  gone, and a site that still cannot connect falls to `SiteFailed` with its log.

## Technical Details

### Per-service facts

For each service whose command line, split by `windows.DecomposeCommandLine`, has an executable
named `pg_ctl.exe` (case-insensitive) and the argument `runservice`:

| field | source |
|-------|--------|
| `service` | the service name |
| `base` | parent of the `bin` folder that holds `pg_ctl.exe` |
| `data` | the `-D` argument (`DataDirectoryFlag`, moved to `internal/engine`) |
| `major` | the first line of `<data>\PG_VERSION`; unknown when the file is missing or unreadable, and the read error goes to `detail=` |
| `running` | the service state is Running |
| `port` | the last `-p N`, `-pN`, `--port=N` or `-c port=N` inside the `-o`/`--options` values of the command line; else the last active `port` setting of `postgresql.conf` with its `include`, `include_if_exists` and `include_dir` files, then of `postgresql.auto.conf`, in the order PostgreSQL reads them; else 5432 |
| `port-file` | the configuration file that set `port`; empty when the command line or the default did |
| `listening` | line 4 of `<data>\postmaster.pid`, read only when `running` |
| `uninstaller` | `<base>\uninstall-postgresql.exe` exists |
| `unreadable` | what the engine could not read: a configuration error with its file and line, the `-o` port, the `postmaster.pid` error or the query error |

The conf parser accepts what PostgreSQL accepts for one setting: a case-insensitive name, an
optional `=`, a quoted or bare value and a trailing `#` comment. A relative include resolves
against the folder of the file that names it; an `include_dir` contributes its `*.conf` files
in name order. The configuration files are read only when the command line sets no port; then a
missing `postgresql.conf`, `include` file or `include_dir` folder and a syntax error are a read
error in `unreadable=`, and the state is `unreadable`.

### States, first match wins

"Candidate" means a pg_ctl service whose major is the expected one or unknown.

| state | condition | Setup |
|-------|-----------|-------|
| `ambiguous` | two or more candidates | refuses, names them |
| `incomplete` | the one candidate has no `-D` or no readable `PG_VERSION` | refuses |
| `unreadable` | the one candidate has a major, and its conf or `-o` port does not parse, or it runs with no readable port on line 4 of `postmaster.pid`; or it is a `postgresql*` service the engine cannot query | refuses, names what it could not read |
| `port-mismatch` | the one candidate runs, and `listening` differs from `port` | refuses, names both |
| `found` | one candidate | configures it |
| `other-major` | no candidate, at least one pg_ctl service of another major | refuses |
| `fresh` | no pg_ctl service | port and data-folder probes, then the EDB install |

A service in a pending state (StartPending, StopPending) counts as not running; Setup starts it
as today (`StartServiceRefusal`).

### Output

```
state=found
service=postgresql-x64-17
base=C:\Program Files\PostgreSQL\17
data=C:\Program Files\PostgreSQL\17\data
port=5432
port-file=C:\Program Files\PostgreSQL\17\data\postgresql.conf
listening=5432
uninstaller=yes
unreadable=
detail=<one English line for the setup log>
```

`ambiguous` adds `services=<name>, <name>`. The service keys print in the `incomplete`,
`unreadable`, `port-mismatch` and `found` states only; `port=` and `listening=` print empty when
unknown. `port-file=` is the file that set the port, empty when the command line or the default
did; it fills `PortMismatch`'s %2, and an empty one selects `PortMismatchNoFile`. `detail=` is a
lowercase clause with no closing period in every state. Exit 0 for every state; exit 1 when the
service list cannot be read, with the error on stderr. A service whose configuration cannot be
queried is skipped unless its name starts with `postgresql` in any letter case; such a service is
an `unreadable` candidate. Non-Windows builds print `engine detection needs the Windows service
list` and exit 1.
ASSUMPTION: `ExecAndCaptureOutput` decodes the UTF-8 that Go writes; the Cyrillic-path CI case
measures it, and Task 4 switches the command to an `--output <file>` that Setup reads with
`LoadStringsFromFile` if it does not.

### Texts

New and changed `[CustomMessages]`, final wording checked against
`docs/architecture/installer-text.md` in the task that adds them:

- `AmbiguousEngine`, RU: «Найдено несколько служб PostgreSQL 17 или PostgreSQL неизвестной
  версии: %1. Откройте программу «Командная строка» командой «Запуск от имени администратора»,
  выполните sc.exe stop <имя службы> и sc.exe delete <имя службы> для каждой службы, которую не
  использует SCADA, и запустите установщик снова.»
- `PortMismatch`, RU: «PostgreSQL 17 работает на порту %1, а в файле %2 указан порт %3.
  Перезапустите службу %4, чтобы PostgreSQL перешёл на порт %3, или укажите в файле порт %1,
  затем запустите установщик снова.»; `PortMismatchNoFile` covers a port no file sets.
- `UnreadableService` names the service and `unreadable=` and tells the reader to fix the error.
- `IncompleteInstallation` names `<base>\uninstall-postgresql.exe` and is chosen by
  `uninstaller=yes`; `IncompleteInstallationNoUninstaller` gives the `sc.exe stop` and `sc.exe
  delete` steps for that service.
- After the EDB install, any state other than `found` refuses with `EdbFailed` and the command's
  `detail=` as the detail.
- Deleted: `IncompleteVersion`, `IncompleteSuperUser`, `IncompleteDataDirectory`,
  `IncompletePgVersion`, `IncompleteInstallationDetail`, `OrphanedRegistration*`,
  `OtherMajorDetail`, `NoRegistrationDetail`, `SitePortWrong*`. Every detail line is the
  `detail=` the command prints; the busy-port detail is `detail=`, `; ` and `PortOwnerDetail`.

## What Goes Where

- **Implementation Steps**: the Go command, the Setup rewrite with its CI cases, the docs.
- **Post-Completion**: measurements on machines CI does not have, and the operator's GUI pass.

## Implementation Steps

### Task 1: Parsing and classification in `internal/engine`

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`

- [x] add the untagged facts type and the pure functions: `PG_VERSION` reading that tells a
  missing file from a read error, conf `port` resolution with includes, `postmaster.pid` line 4,
  and `Classify(services, expectedMajor)` over already-decomposed facts, implementing the state
  table
- [x] write table-driven tests for every state row, the 14-beside-17 machine, a candidate with
  unknown major beside a good 17, a running service without a readable pid file
- [x] write table-driven tests for the conf syntax (no `=`, upper-case name, quoted value,
  trailing comment, commented-out line), `postgresql.auto.conf` over `postgresql.conf`, a
  relative `include`, `include_dir` order, a missing `include` file
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run` - must pass before task 2

### Task 2: Windows enumeration in `internal/engine`

**Files:**
- Create: `internal/engine/services_windows.go`
- Create: `internal/engine/services_windows_test.go`
- Create: `internal/engine/services_other.go`
- Modify: `internal/provision/service_windows.go`
- Modify: `internal/provision/service_windows_test.go`

- [x] move `dataDirectoryFlag` and its tests from `internal/provision/service_windows.go:149-164`
  into the windows-tagged files as `DataDirectoryFlag`; `confirmServes` calls it from there
- [x] add the windows-tagged command-line recognition: `pg_ctl.exe` executable, `runservice`,
  `-D`, and the port inside `-o`/`--options` decomposed by the same parser
- [x] add the enumeration with query-only rights (`windows.OpenSCManager` with
  `SC_MANAGER_CONNECT|SC_MANAGER_ENUMERATE_SERVICE`, `windows.OpenService` with
  `SERVICE_QUERY_CONFIG|SERVICE_QUERY_STATUS`, wrapped in `mgr.Mgr`/`mgr.Service`), feeding
  `Classify`
- [x] add the non-Windows stub returning the error from Technical Details
- [x] write windows-tagged table tests for recognition: quoted and unquoted paths, `-D` joined
  and separate, `-o "-p 5433"`, `-o "--port=5433"`, `-o "-c port=5433"`, a non-pg_ctl service,
  `register` instead of `runservice`
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run` - must pass before task 3

### Task 3: `semibase engine` command

**Files:**
- Modify: `cmd/semibase/main.go`
- Modify: `cmd/semibase/main_test.go`
- Modify: `.github/workflows/ci.yml`

- [x] dispatch `engine` beside `version` (`cmd/semibase/main.go:90-97`), with its own
  `--expected-major` flag and `--help`; it takes no connection or password flags
- [x] print the `key=value` lines of Technical Details; exit 0 per state, 1 on a read failure
- [x] add the command to the usage text (`cmd/semibase/main.go:18-58`)
- [x] write tests for the output of each state and for the flag errors, with the enumeration
  stubbed as `commands` is stubbed today (`cmd/semibase/main.go:71-75`)
- [x] add to the `build` job, after the restart step (`ci.yml:34`), the requirement that
  `semibase engine --expected-major 17` prints `state=found` and `service=postgresql-x64-17`
- [x] run Acceptance Evidence item 3 on the development machine, unelevated
- [x] run `go test ./...` - must pass before task 4

Evidence for Acceptance Evidence item 3, 2026-10-27, unelevated (`net session` exits 2),
`postgresql-14` running beside it: `.\semibase.exe engine --expected-major 17` exits 0 and prints

```
state=found
service=postgresql-x64-17
base=C:\Program Files\PostgreSQL\17
data=C:\Program Files\PostgreSQL\17\data
port=5432
port-file=C:\Program Files\PostgreSQL\17\data\postgresql.conf
listening=5432
uninstaller=yes
unreadable=
detail=service postgresql-x64-17 runs PostgreSQL 17 from C:\Program Files\PostgreSQL\17\data on port 5432
```

### Task 4: Setup reads `semibase engine`, with its CI cases

**Files:**
- Modify: `Installer/SemiBase.iss`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/scripts/installer-case.ps1`

- [x] add `DetectEngine`: extract `semibase.exe`, run `engine --expected-major {#PgMajor}`
  through `ExecAndCaptureOutput`, parse the lines into a `TEngine` with a `State` field; a
  failed run is a refusal with the stderr line as detail
- [x] call it from `InitializeSetup` into a global; `InitializeWizard`, `ShouldSkipPage`,
  `NextButtonClick` and `UpdateReadyMemo` read the global; in `PrepareToInstall` call it before
  `EngineRefusal`, and after `InstallEngineRefusal` in place of the re-read at
  `Installer/SemiBase.iss:858`, where a state other than `found` refuses with `EdbFailed`
- [x] delete the registry constants, `RegisteredEngineRefusal`, `RegisteredInstallations`,
  `UninstallerFolder`, `EngineIsFresh`, `RegistryRefusal`, the `SitePortWrong` branch,
  `SitePortRefused` and `TEngine.SuperUser`; drop `--superuser` from the site command line
  (`site`'s default is `postgres`)
- [x] add `AmbiguousEngine` and `PortMismatch`, choose the uninstaller forms by
  `uninstaller=yes`, delete the keys listed in Technical Details, and check every new or changed
  text against `docs/architecture/installer-text.md`
- [x] compile with `iscc` as AGENTS.md's Build section shows; it compiles with no warning
- [x] in `installer-case.ps1` add `New-FakeService` and `Remove-FakeService` (`New-Service
  -Name postgresql-ci-<n> -BinaryPathName '"<dir>\bin\pg_ctl.exe" runservice -N ... -D ... -w'
  -StartupType Manual`, `sc.exe delete`); `CreateService` does not check that the binary
  exists, so no `pg_ctl.exe` is created; take the base folder in `Start-PostgreSQLRemoval` from
  the `base=` of the workspace's `semibase.exe engine --expected-major 17`; keep
  `Assert-NothingRegistered` as the precondition after each EDB removal and delete only its call
  at `ci.yml:398`
- [x] rewrite the step at `ci.yml:395-431` as another major, an incomplete 17 without
  `PG_VERSION`, a 17 without `-D` and the Cyrillic data path, keeping the token superuser
  password check of `:400-409`; every fake is deleted in `finally`
- [x] after `Run the same setup again` (`ci.yml:497`) add the 14-beside-17 case (exit 0) and the
  ambiguous case; replace `ci.yml:510-520` with the port-mismatch case and add the `SiteFailed`
  case, both restoring their file in `finally`
- [x] update the details at `ci.yml:459` and `:466`; replace the registry dump at `ci.yml:607`
  with the pg_ctl services' names, states and command lines
- [x] after the operator approves the commit and the push, require the `build` and `installer`
  jobs green (deferred to the orchestrator: push and CI)

Measured for the ASSUMPTION in Output, 2026-10-27, read-only: a throwaway Inno Setup 6.7.3 probe
(not SemiBase-Setup) ran a Go binary printing `data=C:\Папка ё\данные` with LF line ends
through `ExecAndCaptureOutput` under ANSI code page 1251 and saved `Output.StdOut` as UTF-8: the
bytes came back unchanged, one array item per line, so the command keeps stdout and has no
`--output`. The Cyrillic CI case still measures it on the runner.

### Task 5: Verify acceptance criteria

- [x] run every item of Acceptance Evidence and record the outputs in this plan
- [x] confirm that no wizard function spawns `semibase.exe` besides `InitializeSetup` and
  `PrepareToInstall`
- [x] run `go test ./...`, `go vet ./...`, `golangci-lint run`, `gofmt -l .` (empty)

Verified 2026-10-27: `go test ./...` ok for all three packages, `go vet ./...` clean,
`golangci-lint run` prints `0 issues.`, `gofmt -l .` prints nothing.

### Task 6: [Final] Update documentation

- [x] `docs/architecture/installer.md`: replace the registration with the service list at every
  line listed in Context, the engine table `:134-141` included, and add `semibase engine` to
  `## Command line` (`:317`)
- [x] `docs/architecture/verification.md`: rewrite the case list at `:101-129` and the
  diagnostics line `:147` for the fake services and the new cases
- [x] `docs/architecture/provisioning.md`: the `engine` command beside `site` and `bench`
- [x] README: delete the section «PostgreSQL 17 без имени службы или порта»; the requirements
  name a PostgreSQL 17 service whose administrator is the database user postgres
- [x] AGENTS.md: the `engine` command in Build and Layout (`internal/engine/`)
- [x] move this plan to `docs/plans/completed/`, in the delivery commit

## Post-Completion

**Manual verification**
- the operator's GUI pass, if the operator decides to run Setup on the development machine:
  with `postgresql-14` beside `postgresql-x64-17` the Ready page shows `postgresql-x64-17` and
  port 5432
- the offline clean-machine run in Windows Sandbox, which needs a reboot to enable
- EDB unattended on a machine with a leftover `Installations\postgresql-x64-17` key and no
  service: Setup now classifies it as `fresh`, and EDB's handling of the leftover key is not
  measured

**External system updates**
- none: SemiPlot consumes the database, not Setup

**Executed by exec:**
- branch: setup-without-registry

## Verify it yourself

1. Setup reads no EDB registration: `grep -cF 'SOFTWARE\PostgreSQL' Installer/SemiBase.iss`
   prints `0`; on master (`git show 089022e:Installer/SemiBase.iss | grep -cF
   'SOFTWARE\PostgreSQL'`) it prints `3`.
2. Detection on this machine, read-only and unelevated: `go build -o semibase.exe
   ./cmd/semibase`, then `.\semibase.exe engine --expected-major 17`. It exits 0 and prints
   `state=found`, `service=postgresql-x64-17`, `port=5432`, `listening=5432`, `uninstaller=yes`;
   `postgresql-14` on 15432 does not change the result. Master has no `engine` command.
3. The state table: `go test -count=1 ./internal/engine/...` runs `TestClassify` over the seven
   states, a 14 or a 9.6 beside a good 17 (`found`), a 9.6 alone (`other-major`), an unqueried
   `postgresql*` service (`unreadable`, never `fresh`).
4. Every refusal end to end: CI run 40541188107 on `631b411`, job `installer`. Each case step
   prints `refusal:` and `detail:` lines through `Assert-Refused`; the texts are the ones listed
   under Acceptance Evidence item 5. `gh run view 40541188107 --log` shows them.
5. The GUI wording has no automated check: no CI case renders a wizard page. Reading the Russian
   texts of `AmbiguousEngine`, `IncompleteInstallation*`, `UnreadableService`, `PortMismatch*`
   and `EngineFailed` in `Installer/SemiBase.iss` `[CustomMessages]` is the check. The Windows UI
   names in them («Командная строка», «Запуск от имени администратора») are not verified on a
   target Windows.

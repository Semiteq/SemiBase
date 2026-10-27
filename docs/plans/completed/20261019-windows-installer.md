# Windows installer for SemiBase

## Overview
- `SemiBase-Setup.exe` (Inno Setup) brings a Windows machine to the commissioned state in one
  run: PostgreSQL 17 present, `semibase site` passed, the service restarted when tuning needs it.
- The headless form is the main channel: `SemiBase-Setup.exe /VERYSILENT /SUPPRESSMSGBOXES
  /NORESTART`, with no password: every role gets its conventional password, `postgres`,
  `scada_writer`, `semiplot` (Tasks 13 and 14). `SEMIBASE_SUPER_PASSWORD` and
  `SEMIBASE_WRITER_PASSWORD` in Setup's environment override the first two; exit code 0 means
  provisioned, anything else means not. Setup takes no password parameter, so no password is on
  its command line. A future suite installer chains it the same way it chains SemiPlot's.
- The `semiplot` password is the convention `semiplot`, and Setup takes no parameter for it.
- Every own text is in Russian and English, picked in Setup's first window or by `/LANG=`; one
  folder, `C:\Program Files\SemiBase`, holds `semibase.exe` and `Logs\`.
- The setup carries the EDB PostgreSQL 17 installer and downloads nothing.
- The setup replaces every other commissioning path: the release no longer attaches a bare
  Windows binary, and `docs/deployment.md` describes the setup plus one recovery section for the
  states the setup refuses.
- Depends on `20261019-site-loopback-and-restart.md` (`--restart-service`) and
  `20261019-password-hygiene.md` (passwords from flags and environment only, none in the server
  log). Both are satisfied: this branch is stacked on `site-loopback-and-restart` (PR #19), which
  is stacked on `password-hygiene` (PR #17).

## Context (from discovery)
- SemiPlot's installer is the convention source: `SemiPlot/Installer/SemiPlot.iss` (admin
  privileges, ru and en languages, `C:\DISTR\...` site folders with `uninsneveruninstall`,
  `/VERYSILENT` contract; `SemiPlot.iss:30` sets `VersionInfoVersion`, which accepts digits and
  dots only) and its Windows release job `SemiPlot/.github/workflows/release.yml` (Inno Setup via
  `winget`, `iscc.exe /DAppVersion=...`).
- `cmd/semibase/main.go:42-46,191-201` — passwords come from flags, then the `SEMIBASE_*`
  environment variables, read with no parsing; `main.go:161-173` — the connection flags,
  `--superuser` and `--expected-major` among them.
- Verified 2026-10-19 with a scratch setup built by ISCC 6.7.3: a variable set in `[Code]` through
  `SetEnvironmentVariableW` (kernel32) reaches a child started by `Exec`, directly and through
  `cmd.exe /c "... > log 2>&1"`, byte for byte, including Cyrillic and `"q" %x% ^&|`;
  `PrepareToInstall` returning a string under `/VERYSILENT` exits 7; the `/LOG` file's fourth line
  is `Setup command line: ...` with every parameter, and `[Code]` `Exec` command lines are not
  logged.
- EDB 14.24 `installer\server\initcluster.ps1` on the development machine: writes the superuser
  password with `Set-Content` and no `-Encoding` (`:244`), so in the ANSI code page, for
  `initdb --pwfile`, and passes `--encoding=UTF8` (`:260`). The 17.11-5 file is the same apart
  from its signature block (task 1, `docs/architecture/installer.md`).
- `internal/provision/create.go:216-236` — an existing role keeps its password unless one is
  given; a missing role without a password is an error (`create.go:222-223`).
- `internal/provision/create.go:18-20,257-272` — `CREATE DATABASE` with no options, so a new
  database inherits `template1`'s encoding and locale; an existing database is used as is.
- `internal/provision/create.go:138-147,166-189` — an existing `public.trends` is never altered,
  only inspected.
- `internal/provision/create.go:191-205` — `--expected-major` mismatch is an error.
- `.github/workflows/release.yml:17-238` — one `ubuntu-latest` job: test, read the tag
  (`release.yml:48-122`, outputs `tag`, `version`, `image`, `prerelease`, `newest`,
  `make_latest`, `body`), cross-compile both targets (`release.yml:126-153`), build and smoke the
  image, create the release with `dist/*`, push the image; the concurrency group is the workflow
  (`release.yml:13-15`). `release.yml:198-200` justifies the release-before-push order with
  consumers downloading release assets.
- Nothing outside this repository downloads a release asset: the only matches for
  `releases/download`, `gh release download` or `semibase_<v>_` in SemiPlot and SemiStep are in
  `SemiPlot/docs/plans/completed/`; SemiPlot's bench pulls `ghcr.io/semiteq/semibase:latest`.
- `.github/workflows/ci.yml:11-29` — the Windows CI job.
- Verified 2026-10-19 on the development machine: `HKLM\SOFTWARE\PostgreSQL\Installations\
  postgresql-x64-14` holds `Version`, `Data Directory`, `Service ID`, `Super User`;
  `HKLM\SOFTWARE\PostgreSQL\Services\postgresql-x64-14` holds `Port` (`0x3c48` = 15432) and
  `Locale` (`Russian_Russia.1251`). A second service, `postgresql-14`, runs with no EDB registry
  entry. Inno Setup 6 is installed at `C:\Program Files (x86)\Inno Setup 6\ISCC.exe`.
- Read 2026-10-19 from `actions/runner-images` `images/windows/Windows2025-Readme.md`
  (`windows-latest`): PostgreSQL 17.11, service `postgresql-x64-17`, Stopped, start type
  Disabled, user `postgres`, password `root`, `PGDATA=C:\PostgreSQL\17\data`,
  `PGBIN=C:\Program Files\PostgreSQL\17\bin`.

## Development Approach
- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- the installer's tests are end-to-end runs in a dedicated Windows CI job, created in task 3 and
  extended by one case per task
- update this plan when scope changes

## Testing Strategy
- end-to-end: the `installer` CI job builds the setup and runs it silently on the runner, one
  case per reachable row of the state tables, thirteen case steps in all, each run with
  `/LANG=english`, on the conventions unless the case sets a `SEMIBASE_*` variable
- shell: `shellcheck` on the fetch script, in the Linux CI job

## Acceptance Evidence
All cases run in the `installer` CI job on `windows-latest`, in this order, one step each, each
asserting Setup's exit code as `Process.ExitCode` reports it (Setup is a GUI exe, so
`$LASTEXITCODE` is never set). A case that passes a password sets its `SEMIBASE_*` variable
inside its step; the others run on the conventions. Setup gets no argument but
`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LANG=english`, so the refusal texts are the English
`[CustomMessages]`. A refusal is exit 7 with its reason as the last line of the newest
`C:\Program Files\SemiBase\Logs\setup-*.log`. The case passwords are two random 24-character
tokens generated per run.
1. The runner's existing 17 (`SEMIBASE_SUPER_PASSWORD=root`, no writer password; nothing has
   created `scada_writer` on that cluster yet): exit 0; the site log holds `role scada_writer
   created with its conventional password`; `C:\Program Files\SemiBase\semibase.exe` exists;
   the service start type is Automatic; `semiplot`/`semiplot` and `scada_writer`/`scada_writer`
   log in.
2. The runner's PostgreSQL is removed; `Get-Service postgresql*` is empty, and
   `HKLM\SOFTWARE\PostgreSQL` holds no subkey under `Installations` or `Services`, the state a
   real EDB uninstall leaves.
3. Registry refusals, each fake key deleted afterwards: an `Installations\postgresql-x64-14`
   key, refused with a message naming the key and not the password; an incomplete 17 key
   without `Super User`, refused naming `uninstall-postgresql.exe`; a complete 17 key with an
   empty `Service ID`, refused naming the key.
4. Password rules on the clean machine, with no `postgresql*` service afterwards: a superuser
   password that is non-ASCII, contains `"`, ends in `\`, starts with `-`, or starts with a
   space; a non-ASCII writer password.
5. Fresh-row states, with no `postgresql*` service afterwards: a listener on `127.0.0.1:5432`,
   a file in `C:\Program Files\PostgreSQL\17\data`.
6. Clean machine, no variables: exit 0; the site log contains `restarting service
   postgresql-x64-17`; `SELECT count(*) FROM pg_settings WHERE pending_restart` prints `0`;
   `SHOW listen_addresses` prints `localhost`; `template1` and `scada_archive` are `C`, `C`,
   `UTF8`; `postgres`/`postgres`, `scada_writer`/`scada_writer` and `semiplot`/`semiplot` log in.
7. The same run again, no variables: exit 0; the new site log does not contain `restarting
   service`; `scada_writer` still logs in with `scada_writer`.
8. The registered `Port` set to 15433: a refusal with `semibase site exited with code`; the site
   log names the refused connection to `localhost:15433`; the port is restored.
9. `SEMIBASE_SUPER_PASSWORD=wrong`: a refusal; the site log names `password authentication
   failed`.
10. The clean install's PostgreSQL is removed as in case 2.
11. Clean machine, the superuser and writer tokens set: exit 0; `postgres` and `scada_writer`
    log in with the tokens, `semiplot` with `semiplot`.
12. `Select-String` for the two tokens over the EDB install logs and
    `C:\Program Files\SemiBase\Logs\*.log` finds nothing. The server log is the Linux job's:
    its two `check-server-log.sh` steps own the rule that no password statement reaches it.
13. `unins000.exe /VERYSILENT`: exit 0; `C:\Program Files\SemiBase\semibase.exe` is gone;
    `postgresql-x64-17` is still present, `semiplot` still logs in, and
    `C:\Program Files\SemiBase\Logs\*.log` remain.

The release workflow's smoke step runs the shipped setup with `SEMIBASE_SUPER_PASSWORD=wrong`
against the release runner's own 17: exit 7, a refusal naming
`C:\Program Files\SemiBase\Logs\site-*.log`, and `C:\Program Files\SemiBase\semibase.exe
version` prints the tag.

## Solution Overview
- All provisioning runs in `PrepareToInstall`, before Inno copies a file. A non-empty return
  string from `PrepareToInstall` ends Setup with exit code 7 under `/VERYSILENT`. Raising in
  `CurStepChanged` happens after files and the uninstaller are registered and may still exit 0.
- Under `/VERYSILENT` the returned refusal text reaches no one, so `PrepareToInstall` also appends every refusal, with no password in it, to
  `C:\Program Files\SemiBase\Logs\setup-<yyyymmdd-hhnnss>.log`, creating the directory
  itself because `[Dirs]` has not run yet. Setup's logs and the recovery copy live in
  `{app}`, `{autopf}\SemiBase` with `DisableDirPage=yes`, not under `C:\DISTR`: any user can
  create, fill or replace a `C:\DISTR` folder, and elevated Setup writes there
  (`installer.md#the-page-the-folder-and-the-texts`).
- `semibase.exe` has one `[Files]` entry into `{app}`; `ExtractTemporaryFile` also takes it to
  `{tmp}` for the run, and every run first copies it to `C:\Program Files\SemiBase\semibase.exe`,
  before any check. That copy is the recovery tool when Setup refuses a state, and the release
  attaches no Windows binary, which an offline site could not download anyway; the `[Files]`
  entry replaces it on a successful run and registers it for the uninstaller. It answers
  `semibase version` for support. The EDB installer is a
  `dontcopy` entry with `nocompression`, so extracting it does not decompress 413 MB of solid
  archive first. The uninstaller removes only `semibase.exe`.
- The installer decides the engine state from the EDB registry (`HKLM64`), never from the service
  list alone. `semibase` owns every database-level decision; the installer duplicates none.
- Passwords stay off every child command line and off the disk where the installer controls it:
  `[Code]` sets `SEMIBASE_SUPER_PASSWORD` and `SEMIBASE_WRITER_PASSWORD`, empty included, in
  Setup's own environment with `SetEnvironmentVariableW`, and `semibase` inherits them and
  applies its conventions to an empty one. EDB reads its password from an option file under
  `{tmp}`, deleted as soon as the EDB process returns.
- What remains exposed: the environment block of the elevated Setup and its children, readable by another administrator during the run; EDB's own
  `powershell.exe -File initcluster.ps1` command line, which carries the superuser password as a
  positional argument, and its temporary pwfile during `initdb`. EDB's log masks the argument as
  `****` (`docs/architecture/installer.md#where-the-superuser-password-goes`).
- The `scada_writer` password must be printable ASCII on every run. The superuser password rules
  hold on the fresh-install row only, where EDB runs: printable ASCII, because EDB writes it in
  the ANSI code page while `semibase` sends UTF-8; no `"` and no trailing `\`, either of which
  breaks EDB's `initcluster.ps1` command line and leaves the install half-done; no leading `-`
  and no leading or trailing space, both unmeasured (`installer.md#password-characters`). On a
  registered 17 the password reaches `semibase` byte for byte. Setup takes no password
  parameter: its parser drops every `"`, and an unquoted space splits a value into a second
  argument it skips, so a parameter would cut a password silently (`installer.md#inno-setup`).
- Setup runs with no password. The superuser password is `SEMIBASE_SUPER_PASSWORD` or the page
  value, else `postgres`, which also fills the EDB option file of a fresh install
  (`#define ConventionalSuperPassword`); it is stored nowhere. A missing `scada_writer` is
  created with `scada_writer`, an existing one keeps its password unless one is given.
- Every run sets the `semiplot` role to `semiplot`. SemiPlot's connection file relies on the
  convention, so the role has one writer and no site-specific value to lose on an upgrade.

## Technical Details

### Engine states (decided in `PrepareToInstall`, first matching row wins)

| State | Action |
| --- | --- |
| `Installations\postgresql-x64-17`, `Version` starts with `17.`, `Super User` is set, `<Data Directory>\PG_VERSION` exists, its `Service ID` has a `Services\<Service ID>\Port` | Use it, even when other majors are also registered: port from `Services\<Service ID>\Port`, superuser from `Super User`; the service is set to Automatic, a Manual one included, and started |
| `Installations\postgresql-x64-17` exists with a `Version` not starting with `17.`, without `Super User` or without `<Data Directory>\PG_VERSION` | Refuse: name the key and say that the recovery is `<Base Directory>\uninstall-postgresql.exe`; a failed EDB install leaves exactly this (`installer.md#a-failed-install`) |
| `Installations\postgresql-x64-17` exists with an empty `Service ID` or no `Services\<Service ID>\Port` | Refuse: name the key; an orphaned registration is not ours to repair. A `Service ID` naming no service is refused by `sc.exe config`; a wrong `Port` is refused by `site` |
| Another major is registered, 17 is not | Refuse: name the registered keys |
| Nothing registered, `127.0.0.1:5432` accepts TCP | Refuse: name the port; a server the registry does not know is not ours to provision |
| Nothing registered, `{#PgDataDir}` exists and is not empty | Refuse: name the directory and say that removing it is the recovery, for example after an interrupted install; existing data is never overwritten |
| Nothing registered, port free, no data directory | Install EDB 17 unattended with the given superuser password or `postgres`, then continue as in the first row (Task 14 removed the writer-password refusal) |

- Every refusal but the password ones, which name their own fix, ends with
  `See the recovery section of the SemiBase deployment guide, docs/deployment.md on GitHub.`:
  an installation machine has no repository checkout, so the hint names the guide, not a path.
- EDB exit codes (task 1, `installer.md#exit-codes`): 0 continues; any other code refuses,
  naming `%TEMP%\install-postgresql.log`. Every measured failure exits 1, and no reboot-required
  code was reproduced, so Setup has no reboot branch. A failed install leaves a partial
  registration, which the next run refuses through the second row.
- EDB exits 0 without a `superpassword` and leaves a cluster with an unknown password, so Setup
  never starts EDB without one.
- After starting a service or installing EDB, the installer does not wait for the port:
  `site --restart-service` waits for the service's Running state itself. The port-busy row probes
  `localhost:5432` with a PowerShell `System.Net.Sockets.TcpClient` call run through `Exec`,
  because Pascal Script has no sockets; it dials `localhost` as `semibase` does, and a probe that
  fails rather than finding the port closed refuses.

### Database states (decided by `semibase site`, unchanged)

| State | Behaviour |
| --- | --- |
| `scada_archive` exists | Used as is (`create.go:263-266`) |
| A role exists, no password given | Password kept (`create.go:216-236`) |
| A role is missing, no password given | Created with its conventional password (Task 13: `roleStatement` in `create.go`) |
| `public.trends` exists | Never altered; `tpdefault` is checked (`create.go:166-189`); a missing grant fails the tail check, and the message names the fix |
| Existing 17 cluster with a locale other than `C` | Used as is: `scada_archive` inherits that cluster's `template1` (`create.go:18-20`). Locale `C` with UTF8 holds for clusters this setup installs |
| Server major is not 17 | Error from `--expected-major 17` (`create.go:197-199`) |

### Run states

| State | Behaviour |
| --- | --- |
| Setup re-run | First engine row; `site` is idempotent; the restart runs only for pending settings |
| EDB fails | Refusal naming the EDB log; the partial registration it leaves takes the second engine row on a re-run |
| `site` fails after EDB installed | PostgreSQL stays; a re-run with the same superuser password takes the first engine row |
| Uninstall | Removes `semibase.exe` only; the engine, its data and the roles stay |

### EDB invocation
- Verified in task 1 (`installer.md#command-line`): `--mode unattended --unattendedmodeui none
  --optionfile <file> --disable-components pgAdmin,stackbuilder`, the file holding
  `superpassword`, `serverport=5432`, `servicename=postgresql-x64-17`, `locale=C` and
  `datadir={#PgDataDir}`. EDB leaves the option file in place.
- `initcluster.ps1` passes `--encoding=UTF8` and `--locale="C"` to `initdb` in 17.11-5 as well;
  the result is `UTF8` with `datcollate` and `datctype` `C`. Case 7 asserts it.
- `{#PgDataDir}` is one `#define`, `C:\Program Files\PostgreSQL\17\data` (EDB's default), used
  by the option file and by the data-directory row.

### `semibase` invocation
- `semibase.exe site --port <port> --superuser <Super User> --expected-major 17
  --restart-service <Service ID>`, run through `ExecAndLogOutput`, whose callback appends each
  output line as it arrives to `C:\Program Files\SemiBase\Logs\site-<yyyymmdd-hhnnss>.log`.
  No shell sits between, and no password is on this line.
- Before the `Exec`, `[Code]` sets the two variables through
  `function SetEnvironmentVariable(lpName, lpValue: string): BOOL; external 'SetEnvironmentVariableW@kernel32.dll stdcall';`.

### Versions
- `iscc /DAppVersion=<tag without v> /DFileVersion=<numeric part> /DSemibaseExe=<path>`.
  `AppVersion` keeps a prerelease suffix; `VersionInfoVersion` takes `FileVersion`, digits and
  dots only. `SemibaseExe` names the binary: CI's plain build or the `build` job's
  `semibase_<version>_windows_amd64.exe`.

### Payload pinning
- `Installer/fetch-postgresql.sh` holds the EDB version, URL and SHA256 as its only constants,
  downloads into `Installer/payload/postgresql-17-windows-x64.exe` (git-ignored, a fixed name the
  `.iss` uses), and fails on a hash mismatch. CI and release both call it, but both restore the
  payload from a cache keyed on the script's hash, so a retired URL surfaces on the first run
  after a cache miss, which can be a tag; the payload is pinned by hash either way.

### Release workflow shape
- `build` (ubuntu): checkout with `fetch-depth: 0`, test, read the tag, cross-compile both
  targets, upload them as an artifact. Its job outputs are every `Read the tag` output. The
  Windows binary has one consumer, the `installer` job.
- `installer` (windows, needs `build`): download the artifact, fetch the payload, install Inno
  Setup, run `iscc`, upload `SemiBase-Setup.exe`.
- `release` (ubuntu, needs both): download both artifacts, `chmod +x` the Linux binary (artifact
  transfer drops file modes, and the `Dockerfile` copies without `--chmod`), build and smoke the
  image, create the GitHub release with the Linux binary and the setup in one `action-gh-release`
  call, then push the image. The workflow-level concurrency group covers all three jobs.
- The Linux binary stays on the release: it is the published reference that the image carries
  the same bytes (`release.yml:146-152`).
- The release still comes before the image push, for a different reason than
  `release.yml:198-200` states: `:latest` is the publish consumers act on, and the release page
  is its record. A push that lands while the release step fails leaves `:latest` on a version
  with no release and no notes, and nobody re-runs a push that worked.

## What Goes Where
- Implementation Steps: the script, the `.iss`, workflows and docs in this repository.
- Post-Completion: SemiPlot-side changes and checks on an installation machine.

## Implementation Steps

### Task 1: Verify the EDB facts the design rests on

**Files:**
- Create: `docs/architecture/installer.md`

- [x] run the EDB 17 installer with `--help`; record the flags the design names
- [x] run it unattended on a scratch runner or VM; record its exit codes (success, wrong option,
      reboot required), the registry keys it writes, `server_encoding` and `datcollate` of the
      result, and whether `install-postgresql.log` contains the password (reboot required was
      not reproduced: the runner's Visual C++ runtime is newer than EDB's, so EDB runs no
      `vcredist`; recorded as such)
- [x] read the 17 `initcluster.ps1`: confirm the ANSI `Set-Content` of the password and
      `--encoding=UTF8`
- [x] record these results and the Inno facts from Context as dated measurements in
      `docs/architecture/installer.md`; revise this plan where a result contradicts it

### Task 2: Pin and fetch the EDB payload

**Files:**
- Create: `Installer/fetch-postgresql.sh`
- Modify: `.gitignore`, `.github/workflows/ci.yml`

- [x] write the script: `set -euo pipefail`, version, URL and SHA256 constants (17.11-5, values in
      `installer.md#the-pinned-payload`), download to
      `Installer/payload/`, `sha256sum -c`, skip the download when the file already matches; a
      present file that does not match fails the run and is left for the operator to delete,
      because a download lands under its final name only after it matched
- [x] add `Installer/payload/` and `Installer/Output/` to `.gitignore`
- [x] add `shellcheck Installer/fetch-postgresql.sh` to the Linux CI job
- [x] run it twice locally: the second run downloads nothing; a corrupted file fails the run

### Task 3: Installer skeleton and its CI job

**Files:**
- Create: `Installer/SemiBase.iss`
- Modify: `.github/workflows/ci.yml`

- [x] `[Setup]` after `SemiPlot.iss`: own `AppId`, `PrivilegesRequired=admin`, x64 only,
      `{autopf}\SemiBase`, `OutputBaseFilename=SemiBase-Setup`, the three `/D` defines required
- [x] `[Languages]` ru and en; `[Dirs]` `C:\DISTR\Logs\SemiBase` with `uninsneveruninstall`
      (removed in review: `ForceDirectories` creates the folder before `[Dirs]` runs, and the
      uninstaller never removes a folder it did not create; a later review moved the logs to
      `{commonpf}\SemiBase Setup\Logs`, and Task 14 to `{app}\Logs`); `[Files]` as in Solution
      Overview
- [x] resolve the superuser and writer passwords from the parameter, else the `SEMIBASE_*`
      variable; refuse an empty superuser password in silent mode, any password that is not
      printable ASCII, and a superuser password that contains `"` or ends in `\`, from
      `PrepareToInstall` (Task 11 removed the parameters: the variables are the only source;
      Task 14 removed the empty-password refusal: the superuser falls back to `postgres`)
- [x] new CI job `installer` on `windows-latest`: `go build`, cache `Installer/payload/` keyed
      on the fetch script's hash, fetch, install Inno Setup via `winget`, `iscc`, Acceptance case 1
- [x] push and read the CI run - green before task 4 (run 39278350797)

### Task 4: Engine state detection

**Files:**
- Modify: `Installer/SemiBase.iss`, `.github/workflows/ci.yml`

- [x] `[Code]`: read the `HKLM64` EDB keys, `Super User` and `<Data Directory>\PG_VERSION`
      among them, check the service named by `Service ID`, probe the port, check `{#PgDataDir}`
      (Task 11 removed the service check: an empty `Service ID` refuses, and `sc.exe config`
      refuses a missing service)
- [x] map them to the engine-state table; each refusal returns its message from
      `PrepareToInstall`, including the silent writer-password guard (Task 14 removed the guard:
      a missing `scada_writer` is created with `scada_writer`)
- [x] run once on the development machine (14 registered): Setup refuses and names
      `postgresql-x64-14` and port 15432
- [x] extend the CI job with Acceptance case 4; until task 7 removes the runner's PostgreSQL,
      the case renames the runner's 17 key aside and restores it afterwards, and task 7 moves the
      case after the removal
- [x] push and read the CI run - green before task 5 (run 39284793107)

### Task 5: Provisioning run

**Files:**
- Modify: `Installer/SemiBase.iss`, `.github/workflows/ci.yml`

- [x] install EDB through the option file when the state requires it; delete the option file as
      soon as the EDB `Exec` returns; handle its exit codes as in Technical Details (the EDB path
      first runs in CI with case 6, task 7)
- [x] set a Disabled service to Automatic, start a stopped one, wait for the port (Task 11
      removed the port wait: `site --restart-service` waits for Running)
- [x] set the three password variables, run `semibase site` as in Technical Details, keep the
      log; a non-zero exit code is a refusal naming the log (Task 11 replaced the `cmd.exe`
      redirect with `ExecAndLogOutput` and the variable read-back with plain sets; Task 14 stopped
      setting `SEMIBASE_PLOT_PASSWORD`, so two variables remain)
- [x] extend the CI job with Acceptance case 2
- [x] push and read the CI run - green before task 6 (run 39288473310)

### Task 6: Password page for an interactive install

**Files:**
- Modify: `Installer/SemiBase.iss`

- [x] a `CreateInputQueryPage` with the superuser and writer password fields, prefilled from the
      parameters or variables; not shown under `/SILENT` or `/VERYSILENT` (Task 11: prefilled
      from the variables alone)
- [x] the superuser field is required; the writer field is required when the engine state is the
      fresh-install row (checked on Next from the registry alone: no 17 key and no other major;
      `PrepareToInstall` stays the enforcement) (Task 14 made both optional: the superuser field
      is prefilled with `postgres`, an empty writer field means `scada_writer` on a new role)
- [x] skipped, not done: build and click through once in a VM (no VM in this run; the manual
      check is listed in Post-Completion)

### Task 7: Full CI case list

**Files:**
- Modify: `.github/workflows/ci.yml`

- [x] remove the runner's PostgreSQL after case 2: `uninstall-postgresql.exe --mode unattended
      --unattendedmodeui none`, then wait for the `%TEMP%\_uninstall*\_uninstall*.000` copy it
      starts to exit (the uninstaller itself exits before doing anything,
      `installer.md#the-uninstaller`), delete `C:\PostgreSQL\17\data`,
      `C:\Program Files\PostgreSQL` and `HKLM\SOFTWARE\PostgreSQL`; assert Acceptance case 3
- [x] add Acceptance cases 4 to 10 in order (Task 11 rewrote the list to the twelve cases in
      Acceptance Evidence and deleted the parameter cases)
- [x] push and read the CI run - green before task 8 (run 39299830501)

### Task 8: Release the setup

**Files:**
- Modify: `.github/workflows/release.yml`

- [x] split the job into `build`, `installer`, `release` as in Technical Details; keep every
      comment whose reasoning still holds, and move each one to the step it explains
- [x] the `release` job restores the exec bit, then attaches the Linux binary and
      `SemiBase-Setup.exe` in one call; the Windows binary is not attached
- [x] rewrite the ordering rationale at `release.yml:198-200` to the reason in Technical Details
- [x] skipped, not done: push a prerelease tag on a fork of the repository: the release page has
      two assets, `:latest` did not move, and nothing is published to `ghcr.io/semiteq` (tags,
      forks and releases are outside this run's git permission; moved to Post-Completion;
      actionlint 1.7.12 is clean on `release.yml`)

### Task 9: Verify acceptance criteria
- [x] every case in Acceptance Evidence passes on the CI run (run 39302087408 on 2d30572: build,
      linux and installer green, cases 1 to 10 each asserted by its own installer step; Task 11
      replaced the cases, and run 40325855098 on 4679cf4 passed the twelve)
- [x] `go test ./...`, `go vet ./...`, `golangci-lint run` are clean; `shellcheck` passes in the
      Linux job (run 39302087408; locally golangci-lint 2.14.0, CI runs the pinned 2.12.2)

### Task 10: [Final] Update documentation
- [x] `docs/deployment.md`, in Russian: the setup command line, its parameters, environment
      fallbacks and exit codes, the three state tables in operator terms, the commissioning order
      with the setup as step 1; `winget` and the manual restart go
- [x] `docs/deployment.md`: one recovery section for the states the setup refuses, running
      `{app}\semibase.exe site --port <p> --superuser <u> --expected-major 17 --restart-service
      <service>` with the password variables set in the shell; `deployment.md:22` stops pointing
      at a bare Windows binary
- [x] `README.md:28-36` becomes the setup command
- [x] `docs/architecture/README.md:19`: the locked decision becomes "EDB PostgreSQL 17 vendored
      in the setup, pinned by SHA256"; `README.md:30` (Distribution) and the document index
      gain the setup and `installer.md`
- [x] `docs/architecture/provisioning.md:12-14` (the `winget` line outside the tool) and the
      artifact table at `provisioning.md:18-25`: the Windows row becomes the setup, the Linux
      row's consumer becomes the image build and its provenance
- [x] `docs/architecture/overview.md`: Components gets `Installer/`; Provisioning order step 1
      (`overview.md:56`) becomes the setup
- [x] `CLAUDE.md`: the deliverable list, the Layout, and the Release section, including the
      ordering rationale and "consumers ... still download the release assets"
- [x] grep both repositories for `winget` next to `PostgreSQL`; every hit in this repository is
      updated, and SemiPlot's go to Post-Completion
- [x] skipped, not done: move this plan to `docs/plans/completed/` (left for the delivery step)

### ➕ Task 11: Cut the installer to the task

The threat model is a single machine with PostgreSQL on localhost: the passwords must exist and
stay off logs and command lines, nobody attacks them. Code that guards anything else goes.

**Files:**
- Modify: `Installer/SemiBase.iss`, `.github/workflows/ci.yml`, `.github/scripts/installer-case.ps1`,
  this plan (Overview and Acceptance Evidence)

- [x] passwords come from `SEMIBASE_SUPER_PASSWORD` and `SEMIBASE_WRITER_PASSWORD` only: delete
      `/SUPERPASSWORD=`, `/WRITERPASSWORD=` and their parsing (`CommandLineParameter`,
      `ParameterReachesSetup`, `ParameterRefusal`, `IsNameInList`, `IsKnownArgument`,
      `StrayArgumentRefusal`, `ResolvePassword`) and every refusal text that names them; the
      password page stays, prefilled from the variables, with its Next check on the shared rule
      function
- [x] measure `ExecAndLogOutput` with an `OnLog` callback on a scratch setup: an argument holding
      `%`, a non-ASCII UTF-8 output line, the exit code, the order of stdout and stderr lines;
      then run `semibase.exe` through it instead of `cmd.exe /s /c`, the callback appending each
      line to the site log as it arrives, and delete `IsCmdSafe` and its refusal.
      `ExecAndLogOutput` is chosen over `ExecAndCaptureOutput` because it keeps the site log
      progressive and in order, so a Setup killed during a long stop still leaves a log
- [x] delete the 60 s port wait (`PortWaitRefusal`, `GetTickCount`, `PortWaitMilliseconds`):
      `site --restart-service` waits for Running itself
- [x] engine rows: keep the incomplete-17 row; replace the orphaned-registration checks
      (`ServiceExists` and its branches) with one refusal for an empty `Service ID` and a guard
      that `Port` was read, so a broken key can never reach `site` with `--restart-service ""`;
      a `Service ID` naming no service is refused by `sc.exe config`, a wrong `Port` by `site`;
      `OtherMajorRefusal` names the registered keys only
- [x] keep the fresh-row password rules (non-ASCII, `"`, trailing `\`, leading `-`, leading or
      trailing space) and the writer non-ASCII rule on every run; replace the
      `SetPasswordVariable` read-back with plain `SetEnvironmentVariable` calls, the writer one
      always set, also when empty
- [x] CI `installer` job, every case through the variables, twelve cases plus the diagnostics
      step:
      1. no superuser password: exit 7, no `{app}\semibase.exe`, the recovery copy prints its
         version, the runner's service still Disabled;
      2. the runner's existing 17 provisions: exit 0, Automatic, `semiplot` and `scada_writer` log
         in;
      3. the runner's PostgreSQL is removed;
      4. registry refusals, fake keys deleted afterwards: another major (the refusal names the
         key), an incomplete 17 key without `Super User` (the refusal names
         `uninstall-postgresql.exe`), a complete 17 key with an empty `Service ID` (the new
         refusal);
      5. password rules: superuser non-ASCII, `"`, trailing `\`, leading `-`, leading space on
         the fresh row, writer non-ASCII;
      6. fresh-row states: busy 5432, non-empty data directory, missing writer password;
      7. a clean machine installs 17: C/UTF8, `localhost`, nothing pending, restart logged,
         `semiplot` logs in;
      8. a superuser-only re-run restarts nothing and `scada_writer` still logs in;
      9. a wrong registered `Port` (15433): `semibase site exited with code`, the site log names
         the refused connect;
      10. a wrong superuser password: the site log names `password authentication failed`;
      11. no generated password in the EDB, setup and site logs; the server-log check and the
          `log_statement` re-run go, the Linux job's two `check-server-log.sh` steps own that
          invariant;
      12. uninstall keeps PostgreSQL, the logs and the recovery copy.
      (Task 14 removed case 1 and case 6's missing-writer refusal, since a run without a
      password provisions, moved the recovery copy into `{app}`, which the uninstaller removes,
      and added the conventions clean install and a second clean install with tokens; the list
      is in Acceptance Evidence)
      Delete the parameter refusals, the option-file watch, the writer-change step, the fourth
      token, `Invoke-Setup`'s argument parameter, and the comments that cite deleted steps
- [x] rewrite this plan's Overview headless line and Acceptance Evidence to the cases above
- [x] push and read the CI run: green on every job, each Acceptance case asserted by its own step
      (run 40325855098 on 4679cf4: build, linux, installer green, cases 1 to 12 each its
      own step)

### ➕ Task 12: Docs after the cut

**Files:**
- Modify: `docs/architecture/installer.md`, `docs/deployment.md`, `CLAUDE.md`, `README.md`,
  this plan

- [x] Solution Overview, Technical Details (engine-state table, `semibase` invocation) and Testing
      Strategy of this plan describe the cut setup; annotate the checkboxes Task 11 reverses in
      Tasks 3, 5, 6, 7 and 9 the way Task 3's `[Dirs]` line is annotated
- [x] `docs/architecture/installer.md`: replace the parameter measurements with one dated
      paragraph on why the setup takes no password parameters (Inno drops every `"` from
      `ParamStr`; an unquoted space splits a value and Inno ignores the unknown switch, so a
      password is cut silently); drop every paragraph on deleted machinery; add the dated
      `ExecAndLogOutput` measurement; engine-table row 3 becomes "empty `Service ID` or missing
      `Port`: refused; a `Service ID` naming no service: refused by `sc.exe config`; a wrong
      `Port`: refused by `site`"
- [x] `docs/deployment.md` (Russian): the setup is started with the variables set; the parameter
      section and the rows of deleted checks go; one row for `sc.exe ... exited with code` (the
      key's service does not exist: `uninstall-postgresql.exe`, then Setup again) and one for
      `semibase site exited with code` with a refused connect (the registered `Port` is wrong);
      the password page stays described (Task 15 rewrote the section for the conventions and
      dropped the two no-password rows)
- [x] `CLAUDE.md` Test and Run sections and `README.md` match the cases and the variables,
      including the sentence on which logs the installer job searches
- [x] grep outside `docs/plans/` for `SUPERPASSWORD`, `WRITERPASSWORD`, `IsCmdSafe`,
      `PortWaitMilliseconds`, `accepted no TCP connection within`, `ParamStr`, `GetCmdTail`,
      `cmd.exe` and `/LOG`: no hit describes current behaviour

### ➕ Task 13: Conventional passwords in the CLI

Every role has a conventional password equal to its name: `postgres`, `scada_writer`, `semiplot`.
The passwords are a formality on a localhost-only machine; a real need changes them explicitly.
`semibase` owns the conventions, so Setup, a manual recovery run and the suite behave alike.

**Files:**
- Modify: `cmd/semibase/main.go`, `cmd/semibase/main_test.go`, `internal/provision/create.go`,
  `internal/provision/provision_test.go` (or the test file beside `ensureRole`)

- [x] superuser: flag, then `SEMIBASE_SUPER_PASSWORD`, then `postgres`; `semibase` never changes
      the superuser's password, so on a server whose `postgres` password differs the operator
      passes the real one and a wrong one fails the run as now
- [x] `semiplot`: flag, then `SEMIBASE_PLOT_PASSWORD`, then `semiplot`; every run sets it, so the
      role is created with or reset to the resolved value, as Setup does today
- [x] `scada_writer`: flag, then `SEMIBASE_WRITER_PASSWORD`; when neither is given, a missing role
      is created with `scada_writer` and an existing role keeps its password (the SCADA holds it,
      a reset would cut the archive off); `ensureRole` takes the creation default and the
      "no password was given to create it" error goes
- [x] the usage text lists the three conventions and the rule that only `semiplot` is reset
- [x] table-driven tests: resolution order for each role (flag, variable, convention), and
      `ensureRole`'s create / keep / update decision with and without an explicit password
- [x] `go test ./...`, `go vet ./...`, `golangci-lint run`, `gofmt -l .` clean; push and read CI:
      every job green before task 14 (run 40336580699 on 99b2635: build, linux, installer green)

### ➕ Task 14: Setup on the conventions, in the chosen language, in one folder

**Files:**
- Create: `Installer/SemiBase.ico`
- Modify: `Installer/SemiBase.iss`, `.github/workflows/ci.yml`, `.github/scripts/installer-case.ps1`,
  `.github/workflows/release.yml` (smoke step), this plan's Acceptance Evidence

- [x] passwords: Setup no longer requires one. The EDB option file of a fresh install gets
      `SEMIBASE_SUPER_PASSWORD` or `postgres` (a `#define` naming the CLI convention; the two
      values are kept equal by the CI case below); Setup sets `SEMIBASE_SUPER_PASSWORD` and
      `SEMIBASE_WRITER_PASSWORD` for `semibase` only when the caller or the page gave a value and
      stops setting `SEMIBASE_PLOT_PASSWORD`, so the CLI applies its own conventions. Delete the
      "no superuser password" and "no scada_writer password" refusals; the fresh-row superuser
      rules and the writer ASCII rule stay for explicit values
- [x] password page: the superuser field is prefilled from the variable, else `postgres`; the
      writer field from the variable, else empty, with the text that empty means `scada_writer`
      on a new role and no change on an existing one
- [x] language: every text of our own (page captions, refusal messages, the ready-page line)
      moves to `[CustomMessages]` with `russian.` and `english.` entries; the page follows the
      language chosen in the first window. CI passes `/LANG=english` so its assertions are
      deterministic
- [x] one folder: `DisableDirPage=yes`; `{app}` is `{autopf}\SemiBase` and holds `semibase.exe`
      and `Logs\`. `PrepareToInstall` copies `semibase.exe` there before any check, so a refused
      machine has the recovery tool; the `[Files]` entry registers it for the uninstaller, which
      removes it and leaves `Logs\`; delete `{#ToolRoot}` and `{commonpf}\SemiBase Setup`
- [x] ready page: `UpdateReadyMemo` shows one line from the registry reads alone: PostgreSQL 17
      will be installed to `C:\Program Files\PostgreSQL\17`, or the registered 17 with its service
      and port is used, or the other registered version Setup will refuse
- [x] icon: `Installer/SemiBase.ico` (16 to 256 px) generated once from the repository's
      `logo.png` with Pillow, the command recorded in `docs/architecture/installer.md`;
      `SetupIconFile` and `UninstallDisplayIcon` point at it (the icon also ships to `{app}`)
- [x] CI: the runner's 17 case passes its real password (`root`) and no writer password, and
      requires `scada_writer` created with `scada_writer`. After the removal: the password-rule
      and fresh-row refusal steps stay; a clean install with no variables requires
      `postgres`/`postgres`, `scada_writer`/`scada_writer` and `semiplot`/`semiplot` to log in,
      `C`/`UTF8`, `localhost`, nothing pending; a re-run with no variables restarts nothing; the
      wrong-port and wrong-password (`SEMIBASE_SUPER_PASSWORD=wrong`) cases stay; then the EDB
      install is removed and a second clean install with a generated superuser and writer token
      runs, and the log search looks for those tokens; uninstall keeps PostgreSQL and `Logs\`
      and removes `semibase.exe`. A run with no password no longer refuses, so the release smoke
      step runs Setup with `SEMIBASE_SUPER_PASSWORD=wrong` against the release runner's own 17
      (its password is `root`) and requires exit 7, a refusal naming the site log, and
      `{app}\semibase.exe version` printing the tag
- [x] rewrite Acceptance Evidence to the cases above; push and read the CI run: every job green,
      each case its own step (run 40339756779 on 8a7b70a: build, linux, installer green, 14 case steps)

### ➕ Task 15: Docs for the conventions

**Files:**
- Modify: `docs/deployment.md`, `docs/architecture/installer.md`,
  `docs/architecture/provisioning.md`, `CLAUDE.md`, `README.md`, this plan

- [x] `docs/deployment.md` (Russian): the three conventions, that `postgres`/`postgres` is known to
      anyone who reads this guide and the server admits localhost only, how to change each
      password (superuser by hand, then passed on every run; writer passed explicitly, then the
      SCADA config; `semiplot` never), the one folder, the ready-page line, the language
      choice; the recovery section names `C:\Program Files\SemiBase\semibase.exe`
- [x] `docs/architecture/installer.md` and `provisioning.md`: the conventions and where they live
      (the CLI; the `.iss` `#define` for the EDB option file), the folder, the icon command, the
      localized texts
- [x] `CLAUDE.md` passwords paragraph, Test and Run sections, and `README.md`: the conventions, the
      cases, `/LANG=english` in CI
- [x] this plan's Overview and Solution Overview: Setup runs with no password; annotate the
      checkboxes tasks 13 and 14 reverse
- [x] grep outside `docs/plans/` for `SemiBase Setup`, `No superuser password`,
      `no password was given` and `SEMIBASE_PLOT_PASSWORD=semiplot`: no hit describes current
      behaviour

## Post-Completion
- SemiPlot `ConfigFiles/connection/connection.yaml:3` names database `semiplot`, while
  `semibase` defaults to `scada_archive` (`cmd/semibase/main.go:164`). One of them changes before
  the suite installer chains both.
- SemiPlot `ConfigFiles/connection/connection.yaml:2` fixes port 5432, while an existing 17 may
  sit on another registered port; `connection.yaml:5` ships an empty password, which becomes the
  `semiplot` convention.
- SemiPlot tells the operator to "run `semibase site`" in `docs/architecture/data-integration.md`
  (`:1017-1021`, `:1092`, `:1342`) and in `SemiPlot.UI/Localization/Resources.resx` and
  `Resources.ru.resx` (`:316`, `:322`, `:340`); each becomes "run the SemiBase setup again".
- SemiPlot `docs/architecture/testing-strategy.md:418` names the `winget` install. Its other hit,
  `docs/plans/completed/20260810-archive-populator.md:1270`, is a completed plan and stays.
- Install once on a clean, offline Windows VM without the Visual C++ runtime: EDB then runs its
  bundled `vcredist`, which the runners never did, and may ask for a reboot, a code Setup has no
  branch for (`installer.md#exit-codes`); the runners also always had internet.
- Click through `SemiBase-Setup.exe` once, interactively, in a clean Windows VM, once in each
  language: the first window picks the language, and every page, the password page captions and
  the Ready line follow it; no folder page appears; the password page shows the superuser field
  prefilled with `postgres` and an empty writer field; Next with a superuser password containing
  `"` shows an error and stays; the Ready page says PostgreSQL 17 will be installed to
  `C:\Program Files\PostgreSQL\17`; Setup installs and provisions it, and `postgres`/`postgres`,
  `scada_writer`/`scada_writer` and `semiplot`/`semiplot` log in.
- Push a prerelease tag such as `v0.0.0-rc.1` on a fork of the repository and read the run:
  `build`, `installer` and `release` pass, the release page is marked prerelease and has two
  assets, `semibase_<v>_linux_amd64` and `SemiBase-Setup.exe`, the fork's
  `ghcr.io/<fork owner>/semibase` has the version tag only, its `:latest` did not move, and
  nothing is published to `ghcr.io/semiteq`.
- The suite installer passes a superuser password other than `postgres` to every SemiBase run,
  through `SEMIBASE_SUPER_PASSWORD` rather than the command line, and stops the SCADA before an
  upgrade that may restart the service.

**Executed by exec:**
- branch: windows-installer

## Verify it yourself
All checks run on a machine or VM you can wipe, never on one whose PostgreSQL holds data you
need. The CI evidence is the `installer` job of run 40339756779 on `8a7b70a`; each Acceptance
case is one step there.

1. Build the setup on the development machine, without running it (this machine has a
   registered PostgreSQL 17, and Setup would provision it):
   `& "$env:ProgramFiles\Git\bin\bash.exe" Installer/fetch-postgresql.sh`, then
   `go build -o semibase.exe ./cmd/semibase` and
   `iscc /DAppVersion=0.0.0-dev /DFileVersion=0.0.0 "/DSemibaseExe=$PWD\semibase.exe" Installer\SemiBase.iss`.
   Expected: `Installer\Output\SemiBase-Setup.exe`, about 421 MB.
2. On a clean Windows VM, offline, in an elevated PowerShell, run it with no password:
   `(Start-Process .\SemiBase-Setup.exe -Wait -PassThru -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/LANG=russian').ExitCode`.
   Expected: `0`; `Get-Service postgresql-x64-17` is Running and Automatic; the newest
   `C:\Program Files\SemiBase\Logs\site-*.log` contains `restarting service postgresql-x64-17`;
   `psql -h localhost -U semiplot -d scada_archive -c "SHOW listen_addresses"` with password
   `semiplot` prints `localhost`, and `postgres`/`postgres` and `scada_writer`/`scada_writer` log
   in. This is also the open vcredist and offline check from Post-Completion: a reboot request
   from EDB shows as a non-zero exit naming `%TEMP%\install-postgresql.log`.
3. Run it again with `$env:SEMIBASE_SUPER_PASSWORD = 'wrong'`. Expected: `7`; the last line of
   the newest `setup-*.log` is in Russian and names the site log, which holds
   `password authentication failed`; `C:\Program Files\SemiBase\semibase.exe version` prints the
   revision.
4. Remove the variable and run it again. Expected: `0`, and the new `site-*.log` has no
   `restarting service`.
5. Double-click the setup on the same VM: the click-through in Post-Completion.
6. Uninstall from Programs and Features. Expected: `C:\Program Files\SemiBase\semibase.exe` is
   gone; the service, the data and `C:\Program Files\SemiBase\Logs` stay.

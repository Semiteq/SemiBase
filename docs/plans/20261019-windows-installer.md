# Windows installer for SemiBase

## Overview
- `SemiBase-Setup.exe` (Inno Setup) brings a Windows machine to the commissioned state in one
  run: PostgreSQL 17 present, `semibase site` passed, the service restarted when tuning needs it.
- The headless form is the main channel: `SemiBase-Setup.exe /VERYSILENT /SUPPRESSMSGBOXES
  /NORESTART /SUPERPASSWORD=... [/WRITERPASSWORD=...]`, exit code 0 means provisioned, anything
  else means not. An absent parameter falls back to `SEMIBASE_SUPER_PASSWORD` or
  `SEMIBASE_WRITER_PASSWORD` in Setup's environment, so a caller can keep passwords off Setup's
  command line. A future suite installer chains it the same way it chains SemiPlot's.
- The `semiplot` password is the convention `semiplot`, and Setup takes no parameter for it.
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
  case per reachable row of the state tables
- shell: `shellcheck` on the fetch script, in the Linux CI job

## Acceptance Evidence
All cases run in the `installer` CI job on `windows-latest`, in this order, each asserting
`$LASTEXITCODE`. The superuser and writer passwords are random 24-character tokens generated per
run; `$psql = "$env:PGBIN\psql.exe"`.
1. No `/SUPERPASSWORD` and no `SEMIBASE_SUPER_PASSWORD`: non-zero exit; `{app}\semibase.exe`
   does not exist; the runner's `postgresql-x64-17` is still Disabled.
2. Existing 17 (the runner's, `SEMIBASE_SUPER_PASSWORD=root` in the environment, no parameter):
   exit 0; the service start type is Automatic; `semiplot` with password `semiplot` reads
   `semiplot_tags`.
3. The runner's PostgreSQL is removed (task 7); `Get-Service postgresql*` is empty and
   `HKLM\SOFTWARE\PostgreSQL` does not exist.
4. A fake `Installations\postgresql-x64-14` key: non-zero exit; the message names version 14;
   the key is deleted afterwards.
5. A Cyrillic `/SUPERPASSWORD` on the clean machine, then one containing `"`: non-zero exit
   each time; no `postgresql*` service exists afterwards.
6. Clean machine, `/SUPERPASSWORD` and `/WRITERPASSWORD` tokens: exit 0; the site log contains
   `restarting service postgresql-x64-17`; `SELECT count(*) FROM pg_settings WHERE
   pending_restart` prints `0`; `SHOW listen_addresses` prints `localhost`; `SELECT datname,
   datcollate, datctype, pg_encoding_to_char(encoding) FROM pg_database WHERE datname IN
   ('template1', 'scada_archive')` prints `C`, `C`, `UTF8` on both rows; `semiplot` logs in
   with password `semiplot`.
7. The same command again: exit 0; the new site log does not contain `restarting service`.
8. `/SUPERPASSWORD=wrong`: non-zero exit; the site log names the failed connection.
9. `Select-String` for both tokens over the EDB install logs, `C:\DISTR\Logs\SemiBase\*.log`,
   `C:\Program Files\PostgreSQL\17\data\log\*` and `%TEMP%\is-*.tmp` finds nothing. No case
   passes `/LOG`: the Inno log carries Setup's command line by design.
10. `unins000.exe /VERYSILENT`: exit 0; `{app}\semibase.exe` is gone; `postgresql-x64-17` is
    still present and `semiplot` still logs in.

## Solution Overview
- All provisioning runs in `PrepareToInstall`, before Inno copies a file. A non-empty return
  string from `PrepareToInstall` ends Setup with exit code 7 under `/VERYSILENT`. Raising in
  `CurStepChanged` happens after files and the uninstaller are registered and may still exit 0.
- `semibase.exe` has one `[Files]` entry into `{app}`; `ExtractTemporaryFile` also takes it to
  `{tmp}` for the run. The copy in `{app}` is the recovery tool when Setup refuses a state, and
  the source of `semibase version` for support (`docs/deployment.md:42`). The EDB installer is a
  `dontcopy` entry with `nocompression`, so extracting it does not decompress 413 MB of solid
  archive first. The uninstaller removes only `semibase.exe`.
- The installer decides the engine state from the EDB registry (`HKLM64`), never from the service
  list alone. `semibase` owns every database-level decision; the installer duplicates none.
- Passwords stay off every child command line and off the disk where the installer controls it:
  `[Code]` sets `SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD` and
  `SEMIBASE_PLOT_PASSWORD=semiplot` in Setup's own environment with `SetEnvironmentVariableW`, and
  `semibase` inherits them. EDB reads its password from an option file under `{tmp}`, deleted as
  soon as the EDB process returns.
- What remains exposed: Setup's own command line when the caller passes parameters, which Inno
  writes into its log under `/LOG` (`SetupLogging` stays off); the environment block of the
  elevated Setup and its children, readable by another administrator during the run; EDB's own
  `powershell.exe -File initcluster.ps1` command line, which carries the superuser password as a
  positional argument, and its temporary pwfile during `initdb`. EDB's log masks the argument as
  `****` (`docs/architecture/installer.md#where-the-superuser-password-goes`).
- Both passwords must be printable ASCII; `PrepareToInstall` refuses anything else. EDB writes
  the superuser password in the ANSI code page while `semibase` sends UTF-8, so a non-ASCII
  superuser password produces a cluster nobody can log into. The superuser password also must
  not contain `"` or end in `\`: either one breaks EDB's `initcluster.ps1` command line, and the
  install fails half-done (`installer.md#password-characters`). Setup's own command-line parser
  drops every `"`, so a password parameter whose value does not appear verbatim in
  `GetCmdTail` is refused too; a password with `"` reaches Setup only through its variable
  (`installer.md#inno-setup`).
- The superuser password is required on every run and stored nowhere.
- Every run sets the `semiplot` role to `semiplot`. SemiPlot's connection file relies on the
  convention, so the role has one writer and no site-specific value to lose on an upgrade.

## Technical Details

### Engine states (decided in `PrepareToInstall`, first matching row wins)

| State | Action |
| --- | --- |
| `Installations\postgresql-x64-17`, `Version` starts with `17.`, `Super User` is set, `<Data Directory>\PG_VERSION` exists, its `Service ID` names an existing service | Use it, even when other majors are also registered: port from `Services\<Service ID>\Port`, superuser from `Super User`; a Disabled service is set to Automatic, a stopped one is started |
| `Installations\postgresql-x64-17` exists without `Super User` or without `<Data Directory>\PG_VERSION` | Refuse: name the key and say that the recovery is `<Base Directory>\uninstall-postgresql.exe`; a failed EDB install leaves exactly this (`installer.md#a-failed-install`) |
| `Installations\postgresql-x64-17` exists, its service does not | Refuse: name the key; an orphaned registration is not ours to repair |
| Another major is registered, 17 is not | Refuse: name the version, port and service |
| Nothing registered, `127.0.0.1:5432` accepts TCP | Refuse: name the port; a server the registry does not know is not ours to provision |
| Nothing registered, `{#PgDataDir}` exists and is not empty | Refuse: name the directory and say that removing it is the recovery, for example after an interrupted install; existing data is never overwritten |
| Nothing registered, port free, no data directory | Without a writer password in silent mode: refuse before installing anything. Otherwise install EDB 17 unattended, then continue as in the first row |

- Every refusal message points to the recovery section of `docs/deployment.md`.
- EDB exit codes (task 1, `installer.md#exit-codes`): 0 continues; any other code refuses,
  naming `%TEMP%\install-postgresql.log`. Every measured failure exits 1, and no reboot-required
  code was reproduced, so Setup has no reboot branch. A failed install leaves a partial
  registration, which the next run refuses through the second row.
- EDB exits 0 without a `superpassword` and leaves a cluster with an unknown password, so Setup
  never starts EDB without one.
- After starting a service or installing EDB, the installer waits for `127.0.0.1:<port>` to
  accept TCP, bounded by 60 s. The probe is a PowerShell `System.Net.Sockets.TcpClient` call run
  through `Exec`, because Pascal Script has no sockets; the port-busy row uses the same probe.

### Database states (decided by `semibase site`, unchanged)

| State | Behaviour |
| --- | --- |
| `scada_archive` exists | Used as is (`create.go:263-266`) |
| A role exists, no password given | Password kept (`create.go:216-236`) |
| A role is missing, no password given | Error (`create.go:222-223`); the writer password is needed on a first install only |
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
  the result is `UTF8` with `datcollate` and `datctype` `C`. Case 6 asserts it.
- `{#PgDataDir}` is one `#define`, `C:\Program Files\PostgreSQL\17\data` (EDB's default), used
  by the option file and by the data-directory row.

### `semibase` invocation
- `semibase.exe site --port <port> --superuser <Super User> --expected-major 17
  --restart-service <Service ID>`, output redirected through `cmd.exe /c` to
  `C:\DISTR\Logs\SemiBase\site-<yyyymmdd-hhnnss>.log`. No password is on this line.
- Before the `Exec`, `[Code]` sets the three variables through
  `function SetEnvironmentVariable(lpName, lpValue: string): BOOL; external 'SetEnvironmentVariableW@kernel32.dll stdcall';`.

### Versions
- `iscc /DAppVersion=<tag without v> /DFileVersion=<numeric part> /DSemibaseExe=<path>`.
  `AppVersion` keeps a prerelease suffix; `VersionInfoVersion` takes `FileVersion`, digits and
  dots only. `SemibaseExe` names the binary: CI's plain build or the `build` job's
  `semibase_<version>_windows_amd64.exe`.

### Payload pinning
- `Installer/fetch-postgresql.sh` holds the EDB version, URL and SHA256 as its only constants,
  downloads into `Installer/payload/` (git-ignored), and fails on a hash mismatch. CI and release
  both call it, so a retired URL fails on a pull request rather than at tag time.

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

- [ ] `[Setup]` after `SemiPlot.iss`: own `AppId`, `PrivilegesRequired=admin`, x64 only,
      `{autopf}\SemiBase`, `OutputBaseFilename=SemiBase-Setup`, the three `/D` defines required
- [ ] `[Languages]` ru and en; `[Dirs]` `C:\DISTR\Logs\SemiBase` with `uninsneveruninstall`;
      `[Files]` as in Solution Overview
- [ ] resolve the superuser and writer passwords from the parameter, else the `SEMIBASE_*`
      variable; refuse an empty superuser password in silent mode, any password that is not
      printable ASCII, and a superuser password that contains `"` or ends in `\`, from
      `PrepareToInstall`
- [ ] new CI job `installer` on `windows-latest`: `go build`, cache `Installer/payload/` keyed
      on the fetch script's hash, fetch, install Inno Setup via `winget`, `iscc`, Acceptance case 1
- [ ] push and read the CI run - green before task 4

### Task 4: Engine state detection

**Files:**
- Modify: `Installer/SemiBase.iss`, `.github/workflows/ci.yml`

- [ ] `[Code]`: read the `HKLM64` EDB keys, `Super User` and `<Data Directory>\PG_VERSION`
      among them, check the service named by `Service ID`, probe the port, check `{#PgDataDir}`
- [ ] map them to the engine-state table; each refusal returns its message from
      `PrepareToInstall`, including the silent writer-password guard
- [ ] run once on the development machine (14 registered): Setup refuses and names
      `postgresql-x64-14` and port 15432
- [ ] extend the CI job with Acceptance case 4; until task 7 removes the runner's PostgreSQL,
      the case renames the runner's 17 key aside and restores it afterwards, and task 7 moves the
      case after the removal
- [ ] push and read the CI run - green before task 5

### Task 5: Provisioning run

**Files:**
- Modify: `Installer/SemiBase.iss`, `.github/workflows/ci.yml`

- [ ] install EDB through the option file when the state requires it; delete the option file as
      soon as the EDB `Exec` returns; handle its exit codes as in Technical Details
- [ ] set a Disabled service to Automatic, start a stopped one, wait for the port
- [ ] set the three password variables, run `semibase site` as in Technical Details, keep the
      log; a non-zero exit code is a refusal naming the log
- [ ] extend the CI job with Acceptance case 2
- [ ] push and read the CI run - green before task 6

### Task 6: Password page for an interactive install

**Files:**
- Modify: `Installer/SemiBase.iss`

- [ ] a `CreateInputQueryPage` with the superuser and writer password fields, prefilled from the
      parameters or variables; not shown under `/SILENT` or `/VERYSILENT`
- [ ] the superuser field is required; the writer field is required when the engine state is the
      fresh-install row
- [ ] build and click through once in a VM

### Task 7: Full CI case list

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] remove the runner's PostgreSQL after case 2: `uninstall-postgresql.exe --mode unattended
      --unattendedmodeui none`, then wait for the `%TEMP%\_uninstall*\_uninstall*.000` copy it
      starts to exit (the uninstaller itself exits before doing anything,
      `installer.md#the-uninstaller`), delete `C:\PostgreSQL\17\data`,
      `C:\Program Files\PostgreSQL` and `HKLM\SOFTWARE\PostgreSQL`; assert Acceptance case 3
- [ ] add Acceptance cases 4 to 10 in order
- [ ] push and read the CI run - green before task 8

### Task 8: Release the setup

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] split the job into `build`, `installer`, `release` as in Technical Details; keep every
      comment whose reasoning still holds, and move each one to the step it explains
- [ ] the `release` job restores the exec bit, then attaches the Linux binary and
      `SemiBase-Setup.exe` in one call; the Windows binary is not attached
- [ ] rewrite the ordering rationale at `release.yml:198-200` to the reason in Technical Details
- [ ] push a prerelease tag on a fork of the repository: the release page has two assets,
      `:latest` did not move, and nothing is published to `ghcr.io/semiteq`

### Task 9: Verify acceptance criteria
- [ ] every case in Acceptance Evidence passes on the CI run
- [ ] `go test ./...`, `go vet ./...`, `golangci-lint run` are clean; `shellcheck` passes in the
      Linux job

### Task 10: [Final] Update documentation
- [ ] `docs/deployment.md`, in Russian: the setup command line, its parameters, environment
      fallbacks and exit codes, the three state tables in operator terms, the commissioning order
      with the setup as step 1; `winget` and the manual restart go
- [ ] `docs/deployment.md`: one recovery section for the states the setup refuses, running
      `{app}\semibase.exe site --port <p> --superuser <u> --expected-major 17 --restart-service
      <service>` with the password variables set in the shell; `deployment.md:22` stops pointing
      at a bare Windows binary
- [ ] `README.md:28-36` becomes the setup command
- [ ] `docs/architecture/README.md:19`: the locked decision becomes "EDB PostgreSQL 17 vendored
      in the setup, pinned by SHA256"; `README.md:30` (Distribution) and the document index
      gain the setup and `installer.md`
- [ ] `docs/architecture/provisioning.md:12-14` (the `winget` line outside the tool) and the
      artifact table at `provisioning.md:18-25`: the Windows row becomes the setup, the Linux
      row's consumer becomes the image build and its provenance
- [ ] `docs/architecture/overview.md`: Components gets `Installer/`; Provisioning order step 1
      (`overview.md:56`) becomes the setup
- [ ] `CLAUDE.md`: the deliverable list, the Layout, and the Release section, including the
      ordering rationale and "consumers ... still download the release assets"
- [ ] grep both repositories for `winget` next to `PostgreSQL`; every hit in this repository is
      updated, and SemiPlot's go to Post-Completion
- [ ] move this plan to `docs/plans/completed/`

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
- SemiPlot `docs/architecture/testing-strategy.md:417` names the `winget` install.
- The suite installer passes the superuser password to every SemiBase run, through
  `SEMIBASE_SUPER_PASSWORD` rather than the command line, and stops the SCADA before an upgrade
  that may restart the service.

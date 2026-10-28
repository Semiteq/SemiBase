# Verification

What runs against a live server, where, and what each step requires. The repository has no
database-touching unit tests: unit tests sit beside the source (`cmd/semibase/*_test.go`,
`internal/engine/*_test.go`, `internal/provision/*_test.go`, table-driven; the `//go:build
windows` ones run in the `build` job only), and the integration checks are the `semiplot`
access checks every run ends with ([provisioning.md](./provisioning.md#what-the-tail-checks-prove)),
the CI steps below and the release smoke runs.

`.github/workflows/ci.yml` runs on every pull request and on a push to `master`, unless every
changed file is on its `paths-ignore` list, which names the paths no job reads: Markdown,
`docs/**`, `LICENSE`, `logo.png`, `.zed/**`, `Installer/make-icon.py` and `release.yml`. Three
jobs:

| Job | Runner | Covers |
| --- | --- | --- |
| `build` | `windows-latest` | `go build`, `go test -race`, golangci-lint 2.12.2, the service restart, `semibase engine` against the runner's service |
| `linux` | `ubuntu-latest` | `go build`, `go test -race`, shellcheck, `site` and `bench` against containers, the image build |
| `installer` | `windows-latest` | `SemiBase-Setup.exe` end to end, with a case for every row of the [engine table](./installer.md#the-engine-state); a failed engine run, a missing or empty `state=` line and an engine run after EDB that does not report `found` have none |

CI pushes nothing. The Windows steps call the runner's `$env:PGBIN\psql.exe`. In the linux job,
`Require the listen_addresses delta`, the pens step and the 17 log step call the Ubuntu runner's
`psql`; to run them locally, point `psql` at a container. The socket step and
`check-server-log.sh` call the container's own `psql` through `docker exec`.

## The build job

`Restart the runner's PostgreSQL service through site` is the only direct test of the service
restart ([provisioning.md](./provisioning.md#the-service-restart)); the `installer` job runs the
restart through the setup, which passes `--restart-service` on every run. The runner image ships
PostgreSQL 17.11-5 ([installer.md](./installer.md#what-a-successful-install-leaves)) as `postgresql-x64-17`, Stopped and Disabled, superuser password `root`, data
directory `C:\PostgreSQL\17\data`. The step requires, in order:

1. `site --restart-service postgresql-x64-17` refuses the Disabled service with exit 1,
   `service postgresql-x64-17 is Disabled` and no `ALTER SYSTEM SET` line.
2. After `Set-Service -StartupType Manual` and `sc.exe start`, which returns while the service is
   Start Pending, the next run, started at once, exits 0 and prints
   `restarting service postgresql-x64-17`,
   `service postgresql-x64-17 runs data directory C:\PostgreSQL\17\data, as the server on <IP>:5432 does`
   and `service postgresql-x64-17 stopped, nothing answers on <IP>:5432`. So `openService`'s own
   Start Pending wait is what runs.
3. `listen_addresses` reads `localhost` and no setting is pending.
4. A second run exits 0 without `restarting service`.
5. `--restart-service no-such-service` is refused with `service no-such-service does not exist`
   and no `ALTER SYSTEM SET` line.

`Find the runner's PostgreSQL service through engine` follows it with the same `semibase.exe`:
`engine --expected-major 17` ([installer.md](./installer.md#semibase-engine)) exits 0 and prints
the lines `state=found` and `service=postgresql-x64-17`.

## The linux job

Job-level `SEMIBASE_SUPER_PASSWORD=ci-super` and `SEMIBASE_WRITER_PASSWORD=ci-writer`; the service
container is `postgres:17-alpine` on port 5432. `Shellcheck the shell scripts` covers
`Installer/*.sh` and `.github/scripts/*.sh`.

| Step | Requires |
| --- | --- |
| `Provision the service container` | `site --database semiplot_dev --expected-major 17` exits 0 twice: the tuning path and the idempotency check |
| `Require the listen_addresses delta` | `listen_addresses` still pending and `localhost` from `postgresql.auto.conf` in `pg_file_settings`. The container is never restarted, so these two prove `site` wrote the delta |
| `Register pens and probe the semiplot_tags constraints` | As `scada_writer`: a day partition and four `trends` rows over keys 0, 13 and 40. As `semiplot`: `semiplot_register_new_pens()` returns 3, then 0, and `semiplot_tags` reads `0 0 #4E79A7 f f,13 13 #F28E2B f f,40 40 #59A14F f f`. Then pen 0 takes the bounds (1, 10), the scale-pair `CHECK` refuses (5, `NULL`) and (5, 1) with `check_violation` (23514), and `log_scale_on_start = NULL` fails with `not_null_violation` (23502). The only place the function body runs over a non-empty archive |
| `Keep the password statements out of the server log` | Sets `log_statement = 'all'`, runs `site` again with a random `semiplot` password (the `ALTER ROLE ... PASSWORD` path), then `check-server-log.sh` with that password and `ci-writer` |
| `Provision a PostgreSQL 14 container, where the REVOKE bites` | `bench --host 127.0.0.1 --port 15434 --database semiplot_dev --expected-major 14` against `postgres:14-alpine`, started with `log_statement=all`, `log_min_error_statement=error`, `log_min_duration_statement=0`, `log_min_duration_sample=0` and `log_transaction_sample_rate=1` |
| `Keep the CREATE ROLE password statements out of the PostgreSQL 14 log` | `check-server-log.sh` over that container with `ci-writer` and `ci-plot` (the `CREATE ROLE ... PASSWORD` path) |
| `Provision over a unix socket, as a postgres image init script` | A `CGO_ENABLED=0` build that `file` reports `statically linked`, layered onto `postgres:17-alpine` as `/docker-entrypoint-initdb.d/10-semibase.sh` running `bench --host /var/run/postgresql`. The entrypoint's temporary server listens on the socket only, so `semiplot` selecting `semiplot_tags` over TCP afterwards proves `bench` completed over the socket |
| `Build the container image` | The root `Dockerfile` builds with `--platform linux/amd64` around that binary, and `docker run --rm semibase:ci version` answers. A broken `Dockerfile` fails the pull request |

The 14 step gates `REVOKE CREATE ON SCHEMA public FROM PUBLIC`
([provisioning.md](./provisioning.md#the-roles)). On `postgres:17-alpine` check 7 reads
`has_schema_privilege` false with or without the statement; on `postgres:14-alpine` `semiplot`
holds `CREATE` through `PUBLIC` without it and the run exits 1. Measured both ways.

`.github/scripts/check-server-log.sh <container> <password>...` requires `logging_collector = off`,
so `docker logs` is the whole server log; requires the `semiplot` login's probe text
(`FROM public.trends LIMIT 1) probe`, `plotProbe` in `internal/provision/check.go`) in it, so the
server logged statements and check 8 ran; and requires every given password absent.
`.github/scripts/wait-for-postgres.sh <container>` waits up to 120 s for `pg_isready` and fails
with the container log when the container stops or the wait runs out.

## The installer job

`Build the setup` runs `.github/actions/build-setup`, the composite action `release.yml` uses too:
the EDB payload cached on the hash of `Installer/fetch-postgresql.sh`, the fetch through Git Bash,
Inno Setup 6.7.3 through `winget`, then ISCC with `/DAppVersion`, `/DFileVersion` (the part before
the first `-`) and `/DSemibaseExe`.

Each case step dot-sources `.github/scripts/installer-case.ps1`: the paths (`C:\Program
Files\SemiBase`, its `Logs`, `semibase.exe`), the `postgresql-x64-17` service name and the data
folder `C:\Program Files\PostgreSQL\17\data` of the install the setup makes, and the helpers.
`Invoke-Setup` runs Setup through `ProcessStartInfo`, because the call operator does not wait for
a GUI program, with `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LANG=english` and nothing else, so
the asserted texts are the English `[CustomMessages]`. No case passes `/LOG`, which records
Setup's command line. `Get-Refusal` reads the last two lines of the newest setup log, and
`Assert-Refused` requires exit 7, the refusal in the last line and, where given, its detail in
the line before. `Assert-Login` logs in with `psql` to `scada_archive`. No `SEMIBASE_*` variable is
set at job level: a case that passes a password sets it inside its own step, the others run on
the conventions. `Generate the case passwords` writes two masked tokens to
`$env:RUNNER_TEMP\case-passwords.txt`, outside every directory the log search reads.

A fake service is a pg_ctl service that no case starts. `New-FakeService <name> <base> [<data>]`
creates it with `New-Service`, start type Manual, command line `"<base>\bin\pg_ctl.exe" runservice
-N "<name>" -D "<data>" -w`, without `-D` when `<data>` is empty; no `pg_ctl.exe` exists there,
since `CreateService` does not check the binary. `Remove-FakeService` runs `sc.exe delete` after
`sc.exe query` finds the service, and treats 1060 (no such service) as done; `Get-Service` is not
used, since a handle it keeps open would leave the service marked for deletion.
`Assert-NoFakeService` requires no `postgresql-ci-*` service, `Assert-NoPostgresService` no
`postgresql*` service. Each fake lives under `$env:RUNNER_TEMP\fake-services`, and each step
deletes its fakes and that folder in `finally`.

The cases, in order:

1. `Provision the runner's existing PostgreSQL 17`, with `SEMIBASE_SUPER_PASSWORD=root` and no
   writer password: exit 0, `role scada_writer created with its conventional password` in the
   site log, `C:\Program Files\SemiBase\semibase.exe` present, start type Automatic,
   `semiplot`/`semiplot` and `scada_writer`/`scada_writer` log in.
2. `Remove the runner's PostgreSQL, refusing while its uninstaller runs`: `Start-PostgreSQLRemoval`
   runs the workspace's `semibase.exe engine --expected-major 17`, requires exit 0, `state=found`
   and `service=postgresql-x64-17`, takes the base folder from `base=` and starts its
   `uninstall-postgresql.exe`; the step waits up to 60 s for a `_uninstall*.000` process, runs
   Setup once and requires `The removal of PostgreSQL has not ended yet. Wait until the PostgreSQL uninstaller closes, then run Setup again.` with
   the detail `PostgreSQL uninstaller process running: `, then waits for the copy to end.
   Afterwards the uninstall log ends with `Exiting with code 0`, the
   `HKLM\SOFTWARE\PostgreSQL\Installations` and `Services` keys are empty
   (`Assert-NothingRegistered`, the precondition that EDB left nothing of the runner's install),
   no `postgresql*` service is left and `127.0.0.1:5432` refuses
   ([the uninstaller](./installer.md#the-uninstaller)).
3. `Refuse another major and the services Setup cannot use`, with a token as the superuser
   password and no `postgresql*` service before and after. Each fake is deleted right after its
   run:
   - `postgresql-ci-14`, `PG_VERSION` 14: `PostgreSQL of another version is installed, and Setup
     works with PostgreSQL 17 only.`, detail `no service runs PostgreSQL 17; postgresql-ci-14
     runs PostgreSQL 14`; the token is absent from every log the run wrote;
   - `postgresql-ci-17`, a data folder without `PG_VERSION` and an empty
     `uninstall-postgresql.exe` in its base: `The PostgreSQL service postgresql-ci-17 is installed
     incompletely. Remove PostgreSQL with <base>\uninstall-postgresql.exe, wait until the removal
     ends, then run Setup again.`, detail `service postgresql-ci-17: no file <data>\PG_VERSION`;
   - `postgresql-ci-17` without `-D` and without an uninstaller: `The PostgreSQL service
     postgresql-ci-17 is installed incompletely. Open Command Prompt with Run as administrator,
     run sc.exe stop postgresql-ci-17 and sc.exe delete postgresql-ci-17, then run Setup again.`,
     detail `service postgresql-ci-17 runs pg_ctl.exe with no -D`;
   - `postgresql-ci-17` with `PG_VERSION` 17 and a `postgresql.conf` holding `port = '5432`:
     `Setup could not read the settings of the PostgreSQL service postgresql-ci-17:
     <data>\postgresql.conf line 1: syntax error: an unterminated quoted value. Fix the error,
     then run Setup again.`, detail `service postgresql-ci-17: <data>\postgresql.conf line 1:
     syntax error: an unterminated quoted value`;
   - `postgresql-ci-17` whose data folder is `данныеё`, its name built from code points so the
     step's encoding cannot change it: `The PostgreSQL service postgresql-ci-17 is installed
     incompletely.`, detail
     `service postgresql-ci-17: no file <root>\данныеё\PG_VERSION`. So the Cyrillic path crosses
     `semibase engine`'s stdout and `ExecAndCaptureOutput` into the setup log unchanged.
4. `Refuse the password rules on a clean machine`: `The password of the PostgreSQL administrator
   (database user postgres) is not allowed for a new PostgreSQL install.` for a non-ASCII
   character, `"`, a trailing `\`, a leading `-` and a leading space; `The password of the
   database user scada_writer has characters that are not allowed.` for a non-ASCII writer
   password; no `postgresql*` service is left.
5. `Refuse a busy port and a non-empty data directory`: a `TcpListener` on `127.0.0.1:5432`
   refuses with `Port 5432 is busy, and no installed PostgreSQL is found.`, detail `no service
   runs pg_ctl.exe runservice; port 5432 held by pwsh (pid <the step's $PID>)`; a file in
   `C:\Program Files\PostgreSQL\17\data` refuses with `The folder C:\Program
   Files\PostgreSQL\17\data is not empty, and no installed PostgreSQL is found.`, detail `no
   service runs pg_ctl.exe runservice`; no `postgresql*` service is left.
6. `Install and provision PostgreSQL 17 on a clean machine`, no variables: exit 0,
   `restarting service postgresql-x64-17` in the site log, nothing pending, `listen_addresses`
   `localhost`, `scada_archive` and `template1` `C`/`C`/`UTF8`, and `postgres`/`postgres`,
   `scada_writer`/`scada_writer` and `semiplot`/`semiplot` log in.
7. `Run the same setup again`, with `SEMIBASE_PLOT_PASSWORD=ci-other-plot` in the step: exit 0, no
   `restarting service`, `scada_writer`/`scada_writer` and `semiplot`/`semiplot` log in, because
   Setup empties the caller's `SEMIBASE_PLOT_PASSWORD`.
8. `Configure the installed 17 beside a fake PostgreSQL 14 service`: `postgresql-ci-14` with
   `PG_VERSION` 14 is no candidate, so the run exits 0, the site log holds `Done: site completed
   against localhost:5432/scada_archive.`, and no `postgresql-ci-*` service is left.
9. `Refuse a second PostgreSQL 17 service`: `postgresql-ci-17` with `PG_VERSION` 17 beside
   `postgresql-x64-17` refuses with a refusal that contains `Several services that run
   PostgreSQL 17 or PostgreSQL of an unknown version are found: ` and a detail that contains
   ` run PostgreSQL 17 or a server of unknown major`. The order of the names follows the service
   list, so the step requires each of `postgresql-ci-17` and `postgresql-x64-17` in both lines
   separately; no `postgresql-ci-*` service is left.
10. `Refuse a configured port the running server does not listen on`: `port = 15433` appended to
    `C:\Program Files\PostgreSQL\17\data\postgresql.conf`, with no restart, so the server keeps
    5432; the original bytes go back in `finally`, before any assertion. The refusal is
    `PostgreSQL 17 runs on port 5432, and the file C:\Program Files\PostgreSQL\17\data\postgresql.conf
    sets port 15433. Restart the service postgresql-x64-17 so that PostgreSQL moves to port 15433,
    or set port 5432 in the file, then run Setup again.`, detail `service postgresql-x64-17
    listens on port 5432, and C:\Program Files\PostgreSQL\17\data\postgresql.conf sets port
    15433`.
11. `Refuse a command-line port the running server does not listen on`: `Win32_Service.Change`
    appends `-o "-p 15433"` to the command line of `postgresql-x64-17`, with no restart, so the
    server keeps 5432; `finally` sets the original command line back, and the step requires it
    back before any assertion. The refusal is `PostgreSQL 17 runs on port 5432, and the service
    postgresql-x64-17 starts it on port 15433. Restart the service postgresql-x64-17, then run
    Setup again.`, detail `service postgresql-x64-17 listens on port 5432, and the service command
    line sets port 15433`.
12. `Refuse a server that rejects the PostgreSQL administrator`: two lines, `host all postgres
    127.0.0.1/32 reject` and `host all postgres ::1/128 reject`, go before EDB's own in
    `pg_hba.conf`, and a `psql` login as `postgres` over `127.0.0.1` must fail before Setup runs.
    No reload is needed: Windows builds are `EXEC_BACKEND`, and each new backend reads
    `pg_hba.conf`. `finally` writes the original bytes back, after which `postgres`/`postgres`
    logs in to `postgres` with no reload either. The refusal
    contains `The semibase.exe tool failed. Fix the error named in the last lines of the log
    C:\Program Files\SemiBase\Logs\site-`, the detail `semibase.exe exit code 1.`, and the site
    log `SQLSTATE 28000`.
13. `Refuse a wrong superuser password`, `SEMIBASE_SUPER_PASSWORD=wrong`: `Wrong password of the
    PostgreSQL administrator (database user postgres). Enter the current password, then run Setup
    again.`, detail `semibase.exe site log: C:\Program Files\SemiBase\Logs\site-...`, and
    `password authentication failed for postgres (SQLSTATE 28P01)` in the site log.
14. `Remove the clean install's PostgreSQL`, with the waits and the checks of case 2.
15. `Install PostgreSQL 17 with a generated superuser and writer password`: the two tokens through
    `SEMIBASE_SUPER_PASSWORD` and `SEMIBASE_WRITER_PASSWORD`, which EDB and `initcluster.ps1`
    inherit, so the log search has a real target; `postgres` and `scada_writer` log in with them,
    `semiplot` with `semiplot`.
16. `Find no password in the logs`: neither token in `%TEMP%\install-postgresql.log`, the
    `*postgres*`, `installbuilder*` and `bitrock*` files and folders of `%TEMP%`, or the setup and
    site logs in `C:\Program Files\SemiBase\Logs`. The search prints file names and hit counts,
    never a matched line. The server log belongs to the linux job's `check-server-log.sh` steps.
17. `Uninstall SemiBase and keep PostgreSQL`: `unins000.exe /VERYSILENT` exits 0, `semibase.exe`
    is gone within 60 s, since Inno's uninstaller re-runs itself from `%TEMP%`, the
    `postgresql-x64-17` service and the logs stay, and `semiplot` still logs in.

`Print the installer diagnostics` runs on failure: the `postgresql*` services with their status
and start type, every service whose command line contains `pg_ctl` with its name, state, start
mode and command line, then, through `Write-Redacted` with the tokens, the last 200 lines of the
EDB logs and the SemiBase logs whole.

## The release smoke runs

`.github/workflows/release.yml` runs each shipped artifact once before the release exists
([provisioning.md](./provisioning.md#distribution)):

- `installer` job, `Smoke-run the setup`: Setup against the runner's own 17 with
  `SEMIBASE_SUPER_PASSWORD=wrong` must refuse with `SitePasswordWrong` and the site log path in
  its detail, and the `semibase.exe` it copied to `C:\Program Files\SemiBase` must print the tag.
- `release` job, `Build the container image`: `docker run --rm <image>:<tag> version`, the only run
  of the shipped Linux bytes, then the manifest must read `linux/amd64`.

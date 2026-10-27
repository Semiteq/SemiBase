# Installer

What `SemiBase-Setup.exe` does, then the facts about the EDB PostgreSQL 17 installer and Inno
Setup that it rests on. Each fact is a dated measurement. The EDB runs happened on GitHub's
`windows-latest` runner (Windows Server 2025, build 26100, ANSI code page 1252) on 2026-10-19, in
a throwaway CI job: the probe script is in commits `6f6413e`, `0b7eebd`, `e837805` and `4649439`,
its logs in CI runs 39268177188, 39269744425, 39270947396 and 39273096839. Every install ran on a
machine whose earlier PostgreSQL had been uninstalled, its uninstaller process waited for, and
its service, `C:\PostgreSQL`, `C:\Program Files\PostgreSQL` and `HKLM\SOFTWARE\PostgreSQL`
deleted.

## The setup

`Installer/SemiBase.iss` compiles to `SemiBase-Setup.exe` with `iscc /DAppVersion=<version>
/DFileVersion=<digits and dots> /DSemibaseExe=<path>`, around the EDB installer that
`Installer/fetch-postgresql.sh` puts into `Installer/payload/`. The main channel is headless:
`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`, optionally `/LANG=russian` or `/LANG=english`. With no
password given, every role gets its conventional password ([passwords](#passwords)); a caller
overrides the superuser and writer ones through `SEMIBASE_SUPER_PASSWORD` and
`SEMIBASE_WRITER_PASSWORD` in Setup's environment; the writer one applies only when `semibase`
creates `scada_writer`. Setup takes no password parameter: Inno's
parser would cut one silently ([Inno Setup](#inno-setup)). Exit code 0 means provisioned.

All of the work runs in `PrepareToInstall`, before Inno copies a file. A string it returns ends
Setup with exit code 7 under `/VERYSILENT`; a failure raised later comes after the uninstaller is
registered and may still exit 0. An exception inside `PrepareToInstall` becomes a refusal too.
Under `/VERYSILENT` the returned text reaches no one, so every refusal is also appended,
password-free, to `C:\Program Files\SemiBase\Logs\setup-<yyyymmdd-hhnnss>.log` (`{app}\Logs`).
Each refusal is one line that names the problem in the reader's terms and the next step, and
points at nothing the machine does not hold ([installer-text.md](./installer-text.md), rules 9,
14 and 17). Its technical detail, the registry key, the exit code or the `site` log path, is not
on screen: the refusal function returns `Refuse(Text, Detail)`, which keeps the detail in the
`RefusalDetail` global, and `LogRefusal` writes it on the line before the refusal, both into one
file whose path it computes once (each `LogPath` call stamps the current second). A detail is an
English `[CustomMessages]` entry without a language prefix, `<Message>Detail`, or
`NoRegistrationDetail`, which `PortBusy` and `DataDirectoryNotEmpty` share, so it reads the
same under either language; a refusal with nothing technical to add writes its line alone.
`PrepareToInstall` clears `RefusalDetail` before its chain and in its `except` branch, so a
detail left by the wizard pages or by an earlier step never precedes `SetupError`.

Inno shows a `PrepareToInstall` refusal on the Preparing page in `PreparingLabel`, static text
that cannot be selected. `CurPageChanged(wpPreparing)` moves it into `PreparingMemo`, the page's
read-only memo that Inno otherwise fills only with the applications that hold files in use, so a
path in the refusal can be copied (rule 26). Inno enters the page twice: once before
`PrepareToInstall` with the label hidden, and once after a refusal with the label holding the
refusal, two blank lines and the stock `CannotContinue` text ("Setup cannot continue. Please
click Cancel to exit."), which the memo copies with it (`TWizardForm.PrepareToInstall`
and `NextButtonClick` in `Projects/Src/Setup.WizardForm.pas` at tag `is-6_7_3`). The move runs
only when the `Refused` global, which `PrepareToInstall` sets, is true and the label is visible;
the applications-in-use path shows the label with `Refused` false. The script never sets
`NeedsRestart`, so the restart radio buttons under the label never show. Under `/SILENT` Inno
shows the refusal in a message box instead, and under `/VERYSILENT /SUPPRESSMSGBOXES` the log is
the only record.

In order, `PrepareToInstall`:

1. copies the carried `semibase.exe` to `C:\Program Files\SemiBase\semibase.exe` (`{app}`), the
   recovery tool for the manual run, so a machine where Setup never succeeded has one, offline
   included;
2. requires a printable ASCII `scada_writer` password, an empty one included: the page field on a
   fresh install, else the caller's `SEMIBASE_WRITER_PASSWORD` (`WriterPasswordValue`);
3. decides the engine state, applying the [superuser password rules](#password-characters) on the
   fresh row only to the given password or the convention, and installs EDB there;
4. runs `sc.exe config <service> start= auto` and `sc.exe start`; a non-zero exit of either
   refuses (`ServiceConfigFailed`, `ServiceStartFailed`) with its `SysErrorMessage` text on
   screen and the code in the detail line, since `sc.exe` exits with a Win32 error code, except
   1056 (already running) from `sc.exe start`. `ServiceConfigFailed` names
   `<Base Directory>\uninstall-postgresql.exe` and `ServiceStartFailed` the folder
   `<Data Directory>\log`, both from the `TEngine` that `RegisteredEngineRefusal` filled;
5. runs `semibase.exe site --port <Port> --superuser <Super User> --expected-major 17
   --restart-service <Service ID>` through `ExecAndLogOutput`, whose callback appends each output
   line, stdout and stderr in write order, to `site-<yyyymmdd-hhnnss>.log` beside the setup log
   as it arrives. The file appears with the first line, so a Setup killed during a long service
   stop still leaves a log. A non-zero exit refuses: `SitePasswordWrong` when a line of the
   output carried `SQLSTATE 28P01`, with the log path in its detail line, else `SiteFailed`,
   which names the log on screen, since the reader acts on its last lines, and puts the exit
   code in the detail line. `semibase` puts `password
   authentication failed for <role> (SQLSTATE 28P01)` in English before the server's own text
   (`connectFailure` in `internal/provision/provision.go`), so the code is there whatever the
   server's `lc_messages`. A `semibase.exe` that does not start refuses with the system error
   text.

The port probe of the fresh row is PowerShell through `Exec`, since Pascal Script has no sockets:
`[Net.Sockets.TcpClient]::new('localhost', <port>)`, the name `semibase` dials too, so a server
on `::1` or on `127.0.0.1` counts. Exit 0 means accepted and 2 refused, the constructor's
`SocketException`; any other code, a constructor Constrained Language Mode forbids among them
(exit 3, measured), refuses. Setup does not wait for the port after `sc.exe start`: with
`--restart-service`, `site` waits for the service's Running state before its first connection,
and `pg_ctl runservice -w` reports Running only once the postmaster accepts sessions, crash
recovery included (`pgwin32_ServiceMain` in `src/bin/pg_ctl/pg_ctl.c`, REL_17_STABLE). A wrong
registered `Port` is found by `site`, which cannot connect, and a wrong superuser password on a
registered 17 is found by `site` too, after the service was set to Automatic and started: only a
running server can check it.

`ExecAndLogOutput` starts `semibase.exe` with no shell between, so a `%` in the registered `Super
User` or `Service ID` reaches it verbatim ([measured](#inno-setup)). Both values go between `"`;
a `"` inside one is not checked for.

The engine state comes from the `HKLM64` EDB keys, never from the service list alone; the first
matching row wins:

| State | Action |
| --- | --- |
| `Installations\postgresql-x64-17` with a `Version` starting `17.`, a `Super User`, `<Data Directory>\PG_VERSION`, and a `Service ID` with a `Services\<Service ID>\Port` | Use it, even when other majors are registered too |
| `Installations\postgresql-x64-17` with another `Version`, no `Super User` or no `PG_VERSION` | Refuse, naming `<Base Directory>\uninstall-postgresql.exe` (`IncompleteInstallation`), or Programs and Features when the key has no `Base Directory` (`IncompleteInstallationNoFolder`); the detail line names the key and what it lacks: a [failed install](#a-failed-install) leaves exactly this |
| `Installations\postgresql-x64-17` with an empty `Service ID` or no `Services\<Service ID>\Port` | Refuse (`OrphanedRegistration`), naming `<Base Directory>\uninstall-postgresql.exe`, with both keys in the detail line, so `site` never gets `--restart-service ""`. A `Service ID` naming no service passes the table and is refused by `sc.exe config`; a wrong `Port` is refused by `site` |
| Another major registered, 17 not | Refuse; the detail line names each key under `Installations` by its bare name, `postgresql-x64-14` |
| Nothing registered, a superuser password that breaks the [password rules](#password-characters) | Refuse |
| Nothing registered, `localhost:5432` accepts TCP | Refuse: a server the registry does not know is not the setup's to provision |
| Nothing registered, `C:\Program Files\PostgreSQL\17\data` not empty | Refuse: existing data is never overwritten |
| Nothing registered, port free, no data directory | Install EDB with the option file under [command line](#command-line), its `superpassword` the given superuser password or `postgres`, then re-read the new registration as in the first row |

Any non-zero EDB exit refuses (`EdbFailed`), naming
`C:\Program Files\PostgreSQL\17\uninstall-postgresql.exe` on screen and the exit code and
`%TEMP%\install-postgresql.log` in the detail line; there is no reboot branch, since no reboot
code was reproduced ([exit codes](#exit-codes)). Every database-level decision belongs to
`semibase site`, and the setup duplicates none of them.

`OrphanedRegistration` and `ServiceConfigFailed` read the uninstaller's folder through
`UninstallerFolder`, which falls back to EDB's default prefix `C:\Program Files\PostgreSQL\17`
when `Base Directory` is empty, so a text never names a bare `\uninstall-postgresql.exe`. EDB
writes `Base Directory` in every registration measured, both [failed installs](#a-failed-install)
included, so only a hand-made key reaches the fallback, and no CI case does.
`IncompleteInstallation` keeps its own branch for that case, `IncompleteInstallationNoFolder`,
which sends the reader to Programs and Features instead.

### Passwords

Every role's conventional password is its name: `postgres`, `scada_writer`, `semiplot`. The
server listens on `localhost` only and every client runs on the installation machine, so the
conventions are a formality, and a site that needs another password changes it explicitly.
`semibase` owns the conventions (`resolvePasswords` in `cmd/semibase/main.go`, the creation
password `ensureRole` takes in `internal/provision/create.go`,
[provisioning.md](./provisioning.md#passwords)), so Setup, a manual recovery run and the suite
installer behave alike. Setup repeats one of them: `#define ConventionalSuperPassword "postgres"`
fills the EDB option file of a fresh install when no superuser password is given, because EDB
runs before `semibase`. The clean-install CI case, which logs in as `postgres`/`postgres`, keeps
the two values equal.

Passwords stay off every child command line the setup controls. `[Code]` sets
`SEMIBASE_SUPER_PASSWORD` and `SEMIBASE_WRITER_PASSWORD` to the page values in Setup's own
environment through `SetEnvironmentVariableW`, after EDB has run, and `semibase` inherits them. On a
registered 17 the page has no writer field, and `WriterPasswordValue` returns the caller's
`SEMIBASE_WRITER_PASSWORD`, so that variable still reaches `semibase`, which applies it only to a
`scada_writer` it creates. Both are set even when empty, so a value the caller's environment holds
does not reach `semibase` after the user cleared a field; `semibase` reads an empty value as unset
and applies its convention. Setup sets no `SEMIBASE_PLOT_PASSWORD`, so `semibase` resets `semiplot`
to `semiplot` on every run, the value SemiPlot's connection file relies on, unless the caller's
environment holds `SEMIBASE_PLOT_PASSWORD`, which nothing supported sets. The caller's variables are
in Setup's environment from the start, and EDB, `initcluster.ps1` and `initdb` inherit them; the CI
fresh install runs that channel, and its log search covers the EDB logs. EDB reads its password from
`{tmp}\postgresql-options.txt`, deleted as soon as the EDB process returns. What stays exposed:
Setup's environment block, readable by another administrator during the run, and EDB's
`initcluster.ps1` command line and pwfile ([where the superuser password
goes](#where-the-superuser-password-goes)).

The `scada_writer` password must be printable ASCII on every run: the SCADA logs in with it from
its own project settings, whose encoding of anything else nobody has measured. The superuser
rules exist for EDB alone and hold on the fresh row only ([password
characters](#password-characters)); on a registered 17 the password reaches `semibase` byte for
byte through the variable. The conventional `postgres` passes those rules.

### The page, the folder and the texts

Without `/SILENT` or `/VERYSILENT`, a password page comes before the Ready page, a custom page
(`CreateCustomPage`) where each field has a bold prompt, its own wrapped explanation and a plain
edit: the passwords are conventions, so the page shows them unmasked. `InitializeWizard` builds
it from `EngineIsFresh`, true when no `Installations\postgresql-x64-17` key exists:

| Engine | Page description | Superuser field explanation | Writer field |
| --- | --- | --- | --- |
| Fresh, no 17 key | `PasswordPageDescriptionNew`: Setup uses the passwords in the fields, an empty field takes the database user's name, and the server accepts local connections only | `SuperPasswordHintNew`: PostgreSQL 17 is installed with this password, and a repeat run does not change it | Shown, prefilled from `SEMIBASE_WRITER_PASSWORD`, else `scada_writer`; `WriterPasswordHint` says the SCADA writes the archive with this password, the same password goes into the SCADA project (Project → Settings → Database), and a repeat run does not change it |
| Registered 17 | `PasswordPageDescriptionExisting`: Setup connects to PostgreSQL 17 with the password below | `SuperPasswordHintExisting`: replace the value if the current password is different | Absent: `semibase` never changes an existing writer's password, and only a fresh install is sure to create the role |

The superuser field is prefilled from `SEMIBASE_SUPER_PASSWORD`, else `postgres`. An emptied field
means the convention. The texts say "database user" for every role, the Simple-Scada manual's term
(`postgresql.html`, `db-access-rights.html`), and "superuser" appears in no text. Neither field is
required. The Next check only lets the user correct a field, and `PrepareToInstall` stays the
enforcement. The three superuser rules share one message, `SuperPasswordRules`, so a password
that breaks two of them bounces once.

`RegistryRefusal` is the part of the engine table the registry alone decides, with no probe and no
connection: `RegisteredEngineRefusal` on a registered 17, `OtherMajorRefusal` on a fresh one.
`ShouldSkipPage` skips the password page when it returns a refusal, since no password changes
the outcome, and `UpdateReadyMemo` then shows only `ReadyRefusal`: "Setup cannot continue.", the
same text `PrepareToInstall` will log, and "Click Cancel.". The page writes that refusal and its
detail to the setup log once per interactive run (the `ReadyRefusalLogged` global), because Inno
runs `UpdateReadyMemo` on every visit to the page and a user who clicks Cancel as told never
reaches `PrepareToInstall`; a user who clicks Install instead logs it a second time. Under
`/SILENT` and `/VERYSILENT` Inno runs `UpdateReadyMemo` too, and the page logs nothing there,
since `PrepareToInstall` always follows. `ReadyMemo` is read-only, so a path in the refusal can be
copied. Otherwise the Ready page shows what the run will do, from the registry reads alone: for a
fresh install the folder, data folder, service and port of the PostgreSQL 17 it installs, the
port line `ReadyPortNew` adding that 5432 is the PostgreSQL default; for a registered 17 its
`Base Directory`, `Service ID` and `Port` (`ReadyPort`), the Automatic startup type Setup sets,
and `ReadyRestart` as a paragraph of its own, which names `shared_buffers` and
`listen_addresses` as the parameters whose difference from Setup's values restarts the service,
says they differ on the first configuration, and tells the reader to stop the SCADA. In both it
names the logs folder and the database and users `semibase site` creates, with
`WriterPasswordValue` or its convention as the `scada_writer` password (`ReadyCreateNew`,
`ReadyCreateExisting`, the latter adding that an existing user keeps its password and that the
`semiplot` password is reset). A refusal only `PrepareToInstall` finds (busy port, non-empty data
directory, wrong password) does not show here.

After a provisioned run, `CurPageChanged(wpFinished)` replaces the stock `FinishedLabel` with
what the SCADA project and SemiPlot connect with, one value per line (`localhost`, the `Port` of
the `TEngine` that `PrepareToInstall` kept as `Provisioned`, `scada_archive`, the two database
users, the `semiplot` password) and the logs folder. After a fresh install it shows
`FinishedTextNew`, whose `%4` is the `scada_writer` password the run set
(`WriterPasswordOrConvention(WriterPasswordValue)`); after a registered 17 it shows
`FinishedText`, which says the password is the one set when the user was created, `scada_writer`
by default. `EngineIsFresh` is false by then, since the install registered 17, so
`PrepareToInstall` keeps its own answer in the `ProvisionedFresh` global. Under `/VERYSILENT` no
page shows. The `[Messages]` section overrides `ConfirmUninstall` and `UninstalledAll`, whose
stock texts promise that every component goes: the uninstaller asks "Remove SemiBase? PostgreSQL
17 with the archive and the logs stay." and ends with the logs path, which is literal there,
since a `[Messages]` entry expands no constant and `{app}` is fixed.

Measured by the operator on 2026-10-27 on a machine with a registered 17 whose `scada_writer` had
the password `writer-new`: a Setup re-run logged `role scada_writer exists, credentials
untouched` in the site log, and `scada_writer` still logged in with `writer-new`.

There is one folder, `{app}`, which is `{autopf}\SemiBase`, `C:\Program Files\SemiBase`;
`DisableDirPage=yes`, so nobody picks another. It holds `semibase.exe`, `SemiBase.ico`,
`unins000.exe` and `Logs\`. `semibase.exe` has one `[Files]` entry into `{app}`, and
`ExtractTemporaryFile` takes it to `{tmp}` for the run and for the copy `PrepareToInstall` makes
into `{app}` before any check; the `[Files]` entry later replaces that copy and registers it for
the uninstaller. The uninstaller removes `semibase.exe` and the icon; the engine, its data, the
roles and `Logs\` stay, since no `[Dirs]` or `[Files]` entry names the logs. The `installer` job
in `.github/workflows/ci.yml` runs the setup end to end on `windows-latest`, one case per
reachable row of the engine table.

`Installer/SemiBase.ico` (16, 24, 32, 48, 64, 128 and 256 px) is generated once from the
repository's `logo.png` with Pillow 12.2.0 by `python Installer/make-icon.py`: the pictogram
without its caption, the white background made transparent, the same shape as SemiPlot's icon.
`SetupIconFile` and `UninstallDisplayIcon` point at it, and it ships to `{app}`.

Every text of the setup's own, the refusals, the page texts, the Ready lines and the Finished
text, is a `[CustomMessages]` entry with a `russian.` and an `english.` form, filled through
`FmtMessage(CustomMessage('<name>'), [...])` at each call site ([measured](#inno-setup)); the
two uninstaller texts are `[Messages]` entries in the same two forms. The texts follow the rules
in [installer-text.md](./installer-text.md). The refusal details and the four `Incomplete*`
fragments that fill `IncompleteInstallationDetail` reach the log only, so each is one entry
without a language prefix, which Inno applies to every language (Inno Setup 6.7.3 help,
"[CustomMessages] section"). The
language is the one picked in Setup's first window, or `/LANG=` under `/VERYSILENT`. Without
`/LANG=`, Inno's documented default (`LanguageDetectionMethod=uilanguage`, not measured here)
takes the Windows UI language and falls back to the first `[Languages]` entry, `russian`. The
setup log carries a refusal in that language and its detail line in English; `semibase`'s own
output in the site log is English always. The CI cases pass `/LANG=english`, so their assertions read the English entries.

The logs and the recovery copy sit in Program Files, not in the site's
`C:\DISTR\<kind>\<product>` folders that SemiPlot uses, because elevated Setup writes there on
every run and the manual recovery runs that binary elevated with the passwords in its
environment. `C:\` grants Authenticated Users
Modify to every folder created under it and lets any user create one, so a `C:\DISTR` path is a
folder a user can fill, replace or turn into a junction. Program Files grants Users read and
execute only, and that inherited DACL is the whole protection: Setup sets no ACL and checks no
reparse point. Measured 2026-10-20 on the development machine, non-elevated, in a scratch folder:
a protected leaf DACL (Administrators and SYSTEM full, Users read) does not stop a user who can
modify the parent from renaming the parent and putting a junction at its old path, which the leaf
path then resolves through. So a `C:\DISTR\Logs\SemiBase` DACL holds only with `C:\DISTR` and
`C:\DISTR\Logs` locked too, folders the setup shares with other products.

## The pinned payload

| Field | Value |
| --- | --- |
| Version | 17.11-5 (PostgreSQL 17.11, EDB build 5) |
| URL | `https://get.enterprisedb.com/postgresql/postgresql-17.11-5-windows-x64.exe` |
| SHA256 | `612c7f5400a003aaffa0372cb273eed52278f93c680ab911eb9cb2229e329072` |
| Size | 413001200 bytes |
| Signature | Authenticode, Valid, `CN=EnterpriseDB Corporation, O=EnterpriseDB Corporation, L=Wilmington, S=Delaware, C=US` |

`Installer/fetch-postgresql.sh` holds the Version, URL and SHA256 the build uses and saves the
file as `Installer/payload/postgresql-17-windows-x64.exe`, the name `#define PgInstaller` in the
`.iss` uses, so a pin bump leaves the `.iss` unchanged. It edits the script's constants and
re-measures this table. A file already there that does not match the pin fails the script, so a
stale payload never reaches `iscc` through a run that fetched first. CI and release cache `Installer/payload/` on the script's hash: an edit to the script,
a comment included, costs one download of 413 MB (about 25 s), and a cached run never asks the
URL, so a retired URL surfaces on the first uncached run, not on the pull request that follows it.

On 2026-10-19 the EDB downloads page linked this build for Windows x86-64 (`fileid=1260614`).
`get.enterprisedb.com` also serves builds 17.11-1 to 17.11-4. The installer is built with
InstallBuilder: it writes `installbuilder_installer_<pid>.log` next to `install-postgresql.log`
in `%TEMP%`.

## Command line

`--help` prints the options to stdout and exits 0, after 62 s. `--version` prints nothing to
stdout and exits 0, after 61 s. The options the setup uses, with their printed defaults:

| Option | Default | Note |
| --- | --- | --- |
| `--mode` | `qt` | `qt`, `win32`, `unattended` |
| `--unattendedmodeui` | `minimal` | `none`, `minimal`, `minimalWithDialogs` |
| `--optionfile` | empty | installation option file |
| `--disable-components` | empty | from `server`, `pgAdmin`, `stackbuilder`, `commandlinetools` |
| `--superpassword` | empty | |
| `--serverport` | `5432` | |
| `--servicename` | empty | |
| `--locale` | `DEFAULT` | a long name or a BCP 47 code; `DEFAULT` passes no `--locale` to `initdb` |
| `--datadir` | `C:\Program Files\PostgreSQL\17\data` | |
| `--prefix` | `C:\Program Files\PostgreSQL\17` | |
| `--superaccount` | `postgres` | |
| `--install_runtimes` | `1` | installs the bundled Visual C++ runtimes first |

The option file holds `key=value` lines, keys are option names without `--`. A file with
`superpassword`, `serverport=5432`, `servicename=postgresql-x64-17`, `locale=C` and
`datadir=C:\Program Files\PostgreSQL\17\data`, passed as `--mode unattended --unattendedmodeui
none --optionfile <file> --disable-components pgAdmin,stackbuilder`, produced exactly that port,
service, locale and data directory. The installer leaves the option file in place.

## Exit codes

| Run | Exit | Duration | What it leaves |
| --- | --- | --- | --- |
| Success | 0 | 38 to 51 s | the install below |
| Unknown option (`--no-such-option 1`) | 1 | 11 to 12 s | nothing |
| `--optionfile` naming a missing file | 1 | 12 to 16 s | nothing |
| No `superpassword` at all | 0 | 49 s | a running cluster whose superuser password the caller does not know |
| `initcluster.ps1` fails (see [password characters](#password-characters)) | 1 | 28 to 61 s | a partial install, see [a failed install](#a-failed-install) |
| Reboot required | not reproduced | | |

The reboot case did not occur. The runner already has Visual C++ runtime 14.51.36247, newer than
the bundled 14.44.35211; the log records both versions and no `vcredist` run. One
`PendingFileRenameOperations` entry existed after every install; it was not counted before, so
nothing attributes it to EDB.

## What a successful install leaves

- `HKLM\SOFTWARE\PostgreSQL\Installations\postgresql-x64-17`: `Base Directory`
  (`C:\Program Files\PostgreSQL\17`), `Version` (`17.11-5`), `Shortcuts`, `Data Directory`,
  `Service ID` (`postgresql-x64-17`), `Service Account` (`NT AUTHORITY\NetworkService`),
  `Super User` (`postgres`), `Branding`, `CLT_Version`. All `REG_SZ`. With pgAdmin and Stack
  Builder installed it also has `pgAdmin_Version` and `SB_Version`.
- `HKLM\SOFTWARE\PostgreSQL\Services\postgresql-x64-17`: `Data Directory`, `Port` (`REG_DWORD`
  5432), `Locale` (`C`, the value passed), `Database Superuser`, `Service Account`,
  `Product Code`, `Display Name`.
- `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PostgreSQL 17`. Nothing under
  `WOW6432Node`.
- Service `postgresql-x64-17`: Running, start type Automatic, account
  `NT AUTHORITY\NetworkService`, `PathName` `"C:\Program Files\PostgreSQL\17\bin\pg_ctl.exe"
  runservice -N "postgresql-x64-17" -D "C:\Program Files\PostgreSQL\17\data" -w`. That `-D` is
  the argument `site --restart-service` compares with `SHOW data_directory`
  ([provisioning.md](./provisioning.md#the-service-restart)).
- The cluster: `PostgreSQL 17.11 on x86_64-windows, compiled by msvc-19.44.35229, 64-bit`;
  `server_encoding` `UTF8`; `postgres`, `template0` and `template1` with `datcollate` `C`,
  `datctype` `C`, `datlocprovider` `c`, encoding `UTF8`; one role, `postgres`, superuser;
  `listen_addresses` `*`, which `initcluster.ps1:294` writes into `postgresql.conf`.
- `pg_hba.conf`: `scram-sha-256` for `local`, `127.0.0.1/32` and `::1/128`, for `all` and for
  `replication`. `initdb` ran with `--auth=scram-sha-256`.
- `--disable-components pgAdmin,stackbuilder` leaves no `pgAdmin 4` directory; the command-line
  tools, `psql` among them, are installed.

The runner image's own install (17.11-5 as well) differs: `Data Directory`
`C:\PostgreSQL\17\data`, `Locale` `English_United` in the `Services` key, pgAdmin and Stack
Builder present, and machine-level `PGUSER=postgres` and `PGPASSWORD` set.

## Where the superuser password goes

- `initcluster.ps1` receives it as its fourth positional argument:
  `powershell.exe -ExecutionPolicy Bypass -File "<prefix>\installer\server\initcluster.ps1"
  "NT AUTHORITY\NetworkService" "postgres" "<installing user's SID>" "<password>" "<temp dir>"
  "<prefix>" "<datadir>" 5432 "C" 0`. That is Windows PowerShell 5.1, and the command line is
  readable in the process list while the script runs (seen by polling `Win32_Process` every
  200 ms).
- `install-postgresql.log` writes that argument as `****`. A search for the password over
  `%TEMP%`, `C:\Windows\Temp`, `<prefix>\installer`, `<datadir>\log`, `postgresql.conf`,
  `postgresql.auto.conf` and `pg_hba.conf` found it only in the caller's option file. The same
  search found `initcluster` in 5 to 14 of those files, so it read the logs.
- `initcluster.ps1:241-244` writes the password into
  `%TEMP%\postgresql_installer_<10 hex>\<8 hex>.tmp` with `Set-Content` and no `-Encoding`,
  which in Windows PowerShell 5.1 is the ANSI code page. `initdb` reads it through `--pwfile`,
  and `:282-285` deletes it. A non-ASCII password therefore reaches the cluster in the ANSI code
  page.
- The `initdb` call (`initcluster.ps1:257-266`): `--pgdata="<datadir>" --username="postgres"
  --encoding=UTF8 --pwfile="<file>" --auth=scram-sha-256 --locale="C"`.
- 17.11-5's `initcluster.ps1` has the same code, line for line, as the EDB 14.24 file on the
  development machine; only the signature block differs. The 17 copy was compared as printed in
  the CI log, so whitespace and line endings were not compared.

## Password characters

One install per password; "passes" means exit 0 and a `psql` login with that password.

| Password | Result |
| --- | --- |
| `` Aa1 '`$!;^&\|<>()*?z `` | passes |
| `Aa1%PATH%z` | passes |
| `Aa1"z` | passes |
| `Aa1-_.,:@#+=~/[]{}\z` | passes |
| `` Aa1 !#$%&'()*+,-./:;<=>?@[\]^_`{\|}~z ``, every printable ASCII punctuation character but `"` | passes |
| `Aa1z\` | exit 1: `initcluster.ps1` prints its usage line |
| `Aa1"z&y` | exit 1: `initcluster.ps1` prints its usage line |
| `` Q"w e%PATH%^&\|<>'`$x!;z `` | exit 1: `initcluster.ps1` exits 255 |

A trailing `\` escapes the closing quote of the password argument, so the arguments after it
shift. The `"` failures look the same, a quote alone passing and a quote with `&` failing, which
points at a `cmd.exe` layer between the installer and PowerShell; that mechanism is inferred, not
read in EDB's source. A printable ASCII password without `"` that does not end in `\` passed in
every combination tried.

Every password measured starts with `Aa1` and has spaces only inside it. A password starting with
`-` could bind as a parameter name of `initcluster.ps1`, and InstallBuilder could trim spaces at
either end of an option-file value; neither was measured, so the setup refuses both on the fresh
row along with non-ASCII, `"` and a trailing `\`. These rules guard EDB's command line and option
file alone, so a registered 17 never meets them.

## A failed install

A failed `initcluster.ps1` leaves the installed files, the uninstaller and the `Uninstall` entry,
and no data directory. `Installations\postgresql-x64-17` exists in every case, and `Super User`
is missing in every case:

| Failure | `Installations\postgresql-x64-17` | Service |
| --- | --- | --- |
| exit 255 | `Base Directory`, `Version`, `Shortcuts`, `Data Directory`, `Service ID`, `Branding`, `CLT_Version` | `postgresql-x64-17` registered, Stopped, Automatic |
| usage line, then a retry without `--disable-components` | `Base Directory`, `Version`, `Shortcuts`, `Branding`, `CLT_Version`, `SB_Version`, `pgAdmin_Version` | none |

So `Version` and `Service ID` with an existing service do not prove a usable install; `Super
User` and `<Data Directory>\PG_VERSION` do. The uninstaller removes a partial install like a
complete one.

## The uninstaller

`<prefix>\uninstall-postgresql.exe --mode unattended --unattendedmodeui none` exits 0 after 6 to
9 s, before it has done anything. It starts a copy of itself,
`%TEMP%\_uninstall<8 hex>\_uninstall<pid>.000`, which runs another 14 to 70 s and ends with
`Exiting with code 0` in `%TEMP%\uninstall-postgresql.log`. A caller waits for that process.
Afterwards:

- the service is gone, a running one included;
- `<prefix>` holds nothing but the data directory, when the data directory is inside it;
- the `postgresql-x64-17` subkeys and the `Uninstall\PostgreSQL 17` entry are gone, while the
  empty `HKLM\SOFTWARE\PostgreSQL\Installations` and `Services` keys stay;
- the data directory stays; the log says `The data directory (<datadir>) has not been removed.`

## Inno Setup

Measured 2026-10-19 on the development machine (Windows 11, build 26300) with a scratch setup
built by ISCC 6.7.3:

- A variable set in `[Code]` through `SetEnvironmentVariableW` (kernel32) reaches a child started
  by `Exec` byte for byte, Cyrillic and
  `"q" %x% ^&|` included.
- `PrepareToInstall` returning a non-empty string under `/VERYSILENT` makes Setup exit with 7.
- The fourth line of a `/LOG` file is `Setup command line: ...` with every parameter. `Exec`
  command lines from `[Code]` are not logged.

The setup takes no password parameter because Inno's parser cuts a password silently. Measured
2026-10-19 and 2026-10-20 the same way, ISCC 6.7.3, with copies of `Installer/SemiBase.iss` that
had `PrivilegesRequired=lowest`, no payload and a refusal in place of success: `ParamStr` and
`{param:...}` drop every `"` from a value, a trailing one included, while keeping `\`, so
`/SUPERPASSWORD=Aa1"z` and `/SUPERPASSWORD=Aa1z"` both arrive as `Aa1z`; an unquoted
`/SUPERPASSWORD=a b` arrives as two arguments, `/SUPERPASSWORD=a` and `b`; and an unknown switch
does not stop Setup, since the argument loop of `InitializeSetup` in
`Projects/Src/Setup.MainFunc.pas` at tag `is-6_7_3` skips every name it does not know, so
`/SUPERPASSWORD=Aa1z /Aa2z` reached `PrepareToInstall` as `Aa1z` and Setup ran on. `GetCmdTail`
keeps the line as typed, but a check against it still let a password such as `x /NOCANCEL`
arrive as `x`. A `SEMIBASE_*` variable reaches `[Code]` byte for byte, a `"` or a space
included.

Measured 2026-10-19 the same way, with a scratch setup that extracts two text files and refuses:

- `ExtractTemporaryFile` matches an entry by its `DestName`. For `Source: "source_v1_name.txt";
  DestName: "dest.txt"`, `ExtractTemporaryFile('dest.txt')` puts `{tmp}\dest.txt` in place and
  `ExtractTemporaryFile('source_v1_name.txt')` raises `The file "source_v1_name.txt" was not
  found`. An entry without `DestName` matches by its source file name.
- It extracts an ordinary `DestDir: "{app}"` entry as well as a `dontcopy` one. So the setup
  extracts `semibase.exe` by that name, whatever file `/DSemibaseExe` names.

Measured 2026-10-19 the same way, with a copy of the setup that has the password page and a
refusal right after the password checks:

- Under `/SILENT` and `/VERYSILENT` Setup calls `NextButtonClick` for the welcome page, the
  custom password page and the Ready page, although it shows none of them. So the page's check
  returns `True` under `WizardSilent`; a `False` would end Setup without exit code 7 and without
  a setup log.
- A value written to `TInputQueryWizardPage.Values` and read back is unchanged, a tab, CR LF and
  Cyrillic included. So `PrepareToInstall` reads the passwords from the page in every mode, and
  under `/VERYSILENT` it sees exactly the `SEMIBASE_*` value.

Measured 2026-10-19 the same way, ISCC 6.7.3, with a copy of the setup whose state checks run
against a registry key that does not exist and that refuses before the EDB install:

- `SetEnvironmentVariableW` with an empty value returns TRUE whether the variable existed or not,
  and `GetEnv` then reads `''`.
- An `ExtractTemporaryFile` of a missing entry raises; the `except` in `PrepareToInstall` turns
  `GetExceptionMessage` into a refusal, exit 7, logged.
- `[Net.Sockets.TcpClient]::new('localhost', <port>)` reached a listener on `127.0.0.1` and one
  on `::1`, and a closed port exited 2 after about 7 s, Windows resending the SYN after the RST
  on both addresses. Under Constrained Language Mode the constructor is refused and the probe
  exits 3.
- `CopyFile` replaces `FileCopy`, which ISCC 6.7.3 reports as renamed.

Measured 2026-10-26 on the development machine, ISCC 6.7.3, with a scratch setup
(`PrivilegesRequired=lowest`, `InitializeSetup` returning `False`) that runs a Go executable
through `ExecAndLogOutput` with an `OnLog` callback appending each line through
`SaveStringsToUTF8FileWithoutBOM`:

- The argument `"a%PATH%b" 50%` reaches the child verbatim: no expansion, no `cmd.exe` between.
- UTF-8 output decodes intact: the stdout line `out2 кириллица ü €` and the stderr line
  `err2 ошибка` arrive with 18 and 11 characters.
- Exit code 3 comes back in `ResultCode`, and the function returns `True`.
- stdout and stderr lines arrive in write order (`out1`, `err1`, `out2`, `err2`): the two
  streams share one pipe. A last line without a newline is delivered.
- Delivery is progressive: 1.5 s into a run whose child sleeps 3 s after `err1`, the file held
  `out1` and `err1`.
- A missing executable returns `False` with `ResultCode` 2 (file not found), and raises nothing.

Measured 2026-10-26 on the development machine, ISCC 6.7.3, with scratch setups
(`PrivilegesRequired=lowest`) that print their messages and refuse:

- A UTF-8 `.iss` without a BOM compiles Cyrillic `[CustomMessages]` correctly: the strings match
  those of the same file with a BOM.
- `/LANG=russian` and `/LANG=english` pick the matching entries. `FmtMessage` fills `%1` to `%4`
  and turns `%%` into `%`.
- A `[Code]` helper that forwards `const Args: array of String` to `FmtMessage` compiles but
  raises `Runtime error: Type Mismatch` at run time, so every call site writes
  `FmtMessage(CustomMessage('<name>'), [...])` itself.
- A `[Code]` line that starts with `[` is read as a section tag (`Invalid section tag`), so a
  wrapped array literal breaks the line after its `[`.

Measured 2026-10-27 the same way, with a copy whose `InitializeSetup` writes each message through
`FmtMessage` to a file and returns `False`, and whose `PrepareToInstall` is renamed out of reach:
under `/LANG=russian` and `/LANG=english` every refusal, the page, Ready and Finished texts and
`SetupMessage(msgConfirmUninstall)` and `msgUninstalledAll` come out in their language, with the
`{#PgMajor}` that ISPP expands inside `[Messages]` and `[CustomMessages]` as `17`. The wizard pages
themselves, the Finished label's height included, were not looked at.

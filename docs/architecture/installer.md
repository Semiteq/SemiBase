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
`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`, the passwords from `/SUPERPASSWORD` and
`/WRITERPASSWORD`, else from `SEMIBASE_SUPER_PASSWORD` and `SEMIBASE_WRITER_PASSWORD`. Exit code 0
means provisioned.

All of the work runs in `PrepareToInstall`, before Inno copies a file. A string it returns ends
Setup with exit code 7 under `/VERYSILENT`; a failure raised later comes after the uninstaller is
registered and may still exit 0. An exception inside `PrepareToInstall` becomes a refusal too.
Under `/VERYSILENT` the returned text reaches no one, and `/LOG` would record the command line,
so every refusal is also appended, password-free, to
`C:\Program Files\SemiBase Setup\Logs\setup-<yyyymmdd-hhnnss>.log` (`{commonpf}`, whatever
folder the install takes). Every refusal but the password and parameter ones, which name their
own fix, ends with `See the recovery section of the SemiBase deployment guide, docs/deployment.md
on GitHub.`

In order, `PrepareToInstall`:

1. copies the carried `semibase.exe` to `C:\Program Files\SemiBase Setup\semibase.exe`, the
   recovery tool for the manual run, so a machine where Setup never succeeded has one, offline
   included;
2. checks that each password parameter arrived as typed (`GetCmdTail`: the value stands verbatim
   in the tail and ends at the end, at a space, or at a `"` closing a quote opened right before
   `/NAME=`) and that every argument starts with `/`, since an unquoted space splits a password
   parameter into two arguments;
3. requires a superuser password and a printable ASCII `scada_writer` password;
4. decides the engine state, applying the [superuser password rules](#password-characters) on the
   fresh row only, and installs EDB there;
5. runs `sc.exe config <service> start= auto` and `sc.exe start`, then waits up to 60 s for
   `localhost:<port>` to accept TCP;
6. runs `semibase.exe site --port <Port> --superuser <Super User> --expected-major 17
   --restart-service <Service ID>` through `cmd.exe /c` into `site-<yyyymmdd-hhnnss>.log` beside
   the setup log. A non-zero exit refuses, naming that log.

The port probe is PowerShell through `Exec`, since Pascal Script has no sockets:
`[Net.Sockets.TcpClient]::new('localhost', <port>)`, the name `semibase` dials too, so a server
on `::1` or on `127.0.0.1` counts. Exit 0 means accepted and 2 refused, the constructor's
`SocketException`; any other code, a constructor Constrained Language Mode forbids among them
(exit 3, measured), refuses. A TCP accept is enough to start `site`: with `--restart-service` it
waits for the service's Running state before its first connection, and `pg_ctl runservice -w`
reports Running only once the postmaster accepts sessions, crash recovery included
(`pgwin32_ServiceMain` in `src/bin/pg_ctl/pg_ctl.c`, REL_17_STABLE). A wrong superuser password
on a registered 17 is found by `site`, after the service was set to Automatic and started: only
a running server can check it.

The registered `Super User` and `Service ID` go into the `cmd.exe /s /c` line between `"`, where
`cmd.exe` still expands `%...%` and a `"` ends the quoting, so a value with either is refused as
an orphaned registration is.

The engine state comes from the `HKLM64` EDB keys, never from the service list alone; the first
matching row wins:

| State | Action |
| --- | --- |
| `Installations\postgresql-x64-17` with a `Version` starting `17.`, a `Super User`, `<Data Directory>\PG_VERSION`, and a `Service ID` naming an existing service with a `Services\<Service ID>\Port` | Use it, even when other majors are registered too |
| `Installations\postgresql-x64-17` with another `Version`, no `Super User` or no `PG_VERSION` | Refuse, naming `<Base Directory>\uninstall-postgresql.exe`: a [failed install](#a-failed-install) leaves exactly this |
| `Installations\postgresql-x64-17` whose service or `Port` does not exist, or whose `Super User` or `Service ID` holds `"` or `%` | Refuse: an orphaned registration is not the setup's to repair |
| Another major registered, 17 not | Refuse, naming each key's version, service and port |
| Nothing registered, a superuser password that breaks the [password rules](#password-characters) | Refuse |
| Nothing registered, `localhost:5432` accepts TCP | Refuse: a server the registry does not know is not the setup's to provision |
| Nothing registered, `C:\Program Files\PostgreSQL\17\data` not empty | Refuse: existing data is never overwritten |
| Nothing registered, port free, no data directory | Refuse without a writer password; otherwise install EDB with the option file under [command line](#command-line), then re-read the new registration as in the first row |

Any non-zero EDB exit refuses, naming `%TEMP%\install-postgresql.log`; there is no reboot branch,
since no reboot code was reproduced ([exit codes](#exit-codes)). Every database-level decision
belongs to `semibase site`, and the setup duplicates none of them.

Passwords stay off every child command line the setup controls. `[Code]` sets
`SEMIBASE_SUPER_PASSWORD`, `SEMIBASE_WRITER_PASSWORD` and `SEMIBASE_PLOT_PASSWORD=semiplot` in
Setup's own environment through `SetEnvironmentVariableW`, after EDB has run, and `semibase`
inherits them. The writer variable is set even when empty, so a value the caller's environment
holds does not reach `semibase` after the user cleared the field; each variable is read back with
`GetEnv`, and `semibase` reads an empty value as unset. Setting them after EDB keeps them from EDB
for a caller who passes parameters only. A caller who uses the variables hands them to Setup's
environment, which EDB, `initcluster.ps1` and `initdb` inherit; the CI fresh install runs that
channel, and its log search covers the EDB logs. EDB reads its password from
`{tmp}\postgresql-options.txt`, deleted as soon as the EDB process returns. What stays exposed:
Setup's own command line when the caller passes the parameters, which Inno writes into its log
under `/LOG` (`SetupLogging` stays off); Setup's environment block, readable by another
administrator during the run; and EDB's `initcluster.ps1` command line and pwfile ([where the
superuser password goes](#where-the-superuser-password-goes)).

The `scada_writer` password must be printable ASCII on every run: the SCADA logs in with it from
its own project settings, whose encoding of anything else nobody has measured. The superuser
rules exist for EDB alone and hold on the fresh row only ([password
characters](#password-characters)); on a registered 17 the password reaches `semibase` byte for
byte through the variable. A password with `"` reaches Setup only through its variable ([Inno
Setup](#inno-setup)). Every run sets the `semiplot` password to `semiplot`, the convention
SemiPlot's connection file relies on, so the role has one writer.

Without `/SILENT` or `/VERYSILENT`, a password page after the folder page holds both fields,
prefilled the same way; its Next check only lets the user correct a field, and `PrepareToInstall`
stays the enforcement. `semibase.exe` has one `[Files]` entry into `{app}`, and
`ExtractTemporaryFile` takes it to `{tmp}` for the run and for the copy in
`{commonpf}\SemiBase Setup`. The `{app}` copy exists only after a successful run; the
`SemiBase Setup` copy after any run, so the recovery section of `docs/deployment.md` points
there. The uninstaller removes the `{app}` copy alone; the engine, its data, the roles and
`SemiBase Setup` with its `Logs` stay, since no `[Dirs]` or `[Files]` entry names them. The
`installer` job in `.github/workflows/ci.yml` runs the setup end to end on `windows-latest`, one
case per reachable row of the engine table.

`SemiBase Setup` sits in Program Files, not in the site's `C:\DISTR\<kind>\<product>` folders
that SemiPlot uses, because elevated Setup writes there on every run and the manual recovery runs
that binary elevated with the passwords in its environment. `C:\` grants Authenticated Users
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

`Installer/fetch-postgresql.sh` holds these constants alone and saves the file as
`Installer/payload/postgresql-17-windows-x64.exe`, the name `#define PgInstaller` in the `.iss`
uses, so a pin bump edits the script and nothing else. A file already there that does not match
the pin fails the script, so a stale payload never reaches `iscc` through a run that fetched
first. CI and release cache `Installer/payload/` on the script's hash: an edit to the script,
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
  by `Exec`, directly and through `cmd.exe /c "... > log 2>&1"`, byte for byte, Cyrillic and
  `"q" %x% ^&|` included.
- `PrepareToInstall` returning a non-empty string under `/VERYSILENT` makes Setup exit with 7.
- The fourth line of a `/LOG` file is `Setup command line: ...` with every parameter. `Exec`
  command lines from `[Code]` are not logged.

Measured 2026-10-19 the same way, with a copy of `Installer/SemiBase.iss` that has
`PrivilegesRequired=lowest`, no payload and a refusal in place of success:

- `ParamStr` and `{param:...}` return the same value, and both drop every `"` while keeping `\`:
  `/SUPERPASSWORD=Aa1"z` and `"/SUPERPASSWORD=Aa1""z"` arrive as `Aa1z`,
  `"/SUPERPASSWORD=Aa1\"z"` as `Aa1\z`, `"/SUPERPASSWORD=a b"` as `a b`. `{x}|y%PATH%` arrives
  unchanged. A parameter name matches in any case.
- `GetCmdTail` returns the command line as typed, quotes included, after a leading
  `/SL5="..."` that the bootstrapper adds. The setup compares the two and refuses a password
  parameter whose value does not appear in it verbatim.
- Measured 2026-10-20 the same way, with a probe that records `GetCmdTail` and `ParamStr`: a
  trailing `"` is dropped too, so `/SUPERPASSWORD=Aa1z"` arrives as `Aa1z`, and so does
  `"/SUPERPASSWORD=Aa1z""`; the tail keeps both quotes. A prefix match would accept them, so the
  value must also end at the end of the tail, at a space, or at the `"` that closes a quote
  opened right before `/SUPERPASSWORD=`. `"/SUPERPASSWORD=two words"`, written by
  `ProcessStartInfo` and by `Start-Process` (whose tail ends in a space), arrives as `two words`
  and passes; `/SUPERPASSWORD="Aa1z"` and `/SUPERPASSWORD=Aa1z /WRITERPASSWORD=Aa1w"` are refused.
- An unquoted `/SUPERPASSWORD=a b` arrives as two arguments, `/SUPERPASSWORD=a` and `b`, and the
  first matches the tail verbatim. So the setup also refuses any argument that does not start
  with `/`; the bootstrapper's `/SL5=...` is argument 1 and does.

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
  under `/VERYSILENT` it sees exactly the parameter or `SEMIBASE_*` value.

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

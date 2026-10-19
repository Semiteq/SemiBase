# Installer

The facts about the EDB PostgreSQL 17 installer and Inno Setup that the Windows setup rests on.
Each one is a dated measurement. The EDB runs happened on GitHub's `windows-latest` runner
(Windows Server 2025, build 26100, ANSI code page 1252) on 2026-10-19, in a throwaway CI job: the
probe script is in commits `6f6413e`, `0b7eebd`, `e837805` and `4649439`, its logs in CI runs
39268177188, 39269744425, 39270947396 and 39273096839. Every install ran on a machine whose
earlier PostgreSQL had been uninstalled, its uninstaller process waited for, and its service,
`C:\PostgreSQL`, `C:\Program Files\PostgreSQL` and `HKLM\SOFTWARE\PostgreSQL` deleted.

## The pinned payload

| Field | Value |
| --- | --- |
| Version | 17.11-5 (PostgreSQL 17.11, EDB build 5) |
| URL | `https://get.enterprisedb.com/postgresql/postgresql-17.11-5-windows-x64.exe` |
| SHA256 | `612c7f5400a003aaffa0372cb273eed52278f93c680ab911eb9cb2229e329072` |
| Size | 413001200 bytes |
| Signature | Authenticode, Valid, `CN=EnterpriseDB Corporation, O=EnterpriseDB Corporation, L=Wilmington, S=Delaware, C=US` |

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
- 17.11-5's `initcluster.ps1` equals the EDB 14.24 file on the development machine except for the
  Authenticode signature block.

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

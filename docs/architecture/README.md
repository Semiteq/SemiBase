# SemiBase Architecture

Declarative architecture docs (English, present tense). These describe the system
**as it is**, not how it got here. Build, test and run commands live in the root `AGENTS.md`.

## Documents

- [overview.md](./overview.md) — purpose, consumers, components, provisioning order.
- [configuration.md](./configuration.md) — every setting changed from the PostgreSQL default, with why.
- [provisioning.md](./provisioning.md) — the tool's two provisioning commands, idempotency rules, the four
  roles, the SemiPlot configuration schema, the access
  chain, and the stated assumption about the SCADA meeting an existing `trends`.
- [installer.md](./installer.md) — what `SemiBase-Setup.exe` does, and the dated EDB and Inno
  Setup measurements it rests on.
- [installer-text.md](./installer-text.md) — the rules every setup text follows, each with the
  operator objection it comes from.
- [verification.md](./verification.md) — what CI and the release smoke runs execute against a
  live server, step by step, and what each step requires.

## Locked decisions

| Area | Decision |
| --- | --- |
| Installation | EDB PostgreSQL 17 vendored in the setup, pinned by SHA256 (`Installer/fetch-postgresql.sh`); the setup installs the engine, `semibase.exe` never does |
| Major version | 17 for new installs, pinned latest; floor 14 (SemiPlot needs `date_bin`; Simple-Scada 2 docs require 14+) |
| Provisioning tool | Go single binary `semibase.exe` (`pgx/v5`, `golang.org/x/sys` for the Windows service, embedded SQL); no runtime or `psql` on the target machine |
| Instance configuration | Fixed `ALTER SYSTEM` deltas applied by `semibase site` via `pg_reload_conf()`, never by editing `postgresql.conf`; two wait for a restart, which `site --restart-service <name>` performs, opt-in ([configuration.md](./configuration.md)) |
| Network | Loopback only (`listen_addresses = 'localhost'`): every client runs on the installation machine |
| Hardware floor | Installation machines guarantee 8 GB RAM and 50 GB database disk; memory settings are fixed constants sized to that floor, identical on every machine |
| Roles | Four: the superuser (owns the `semiplot_*` tables), `scada_writer` (SCADA), `semiplot` (viewers, one role, reads the archive and edits only pen settings, groups and markers), `semiplot_registrar` (`NOLOGIN`, owns `semiplot_register_new_pens()`) ([provisioning.md](./provisioning.md#the-roles)) |
| Command surface | Two provisioning commands, `site` and `bench`, named for the situation rather than for the tool's internal steps; they differ in one thing, the server settings, which only `site` applies. `engine` reads the service list for Setup and provisions nothing |
| Archive read access | `ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer` set **before** `public.trends` is created, and before the writer first runs. The `semiplot_*` tables are granted table by table instead, since the superuser creates them |
| Objects we add | `semiplot_tags`, `semiplot_groups`, `semiplot_pen_groups`, `semiplot_markers`, `semiplot_meta`, the `semiplot_register_new_pens()` function the viewer calls, and the vendor-shaped `public.trends` — no triggers, scheduled jobs, extensions, or other functions; `messages` is not created, the SCADA makes it |
| SemiPlot schema version | `semiplot_meta.schema_version`, `1` in this release, written by `provision` alone; a floor, not an equality ([provisioning.md](./provisioning.md#the-schema-version-is-a-floor)) |
| Archive schema | Owned by Simple-Scada 2 and documented in the SemiPlot repository. This tool creates `public.trends` once, with the vendor's shape, as `scada_writer` ([provisioning.md](./provisioning.md#the-archive-table)), and never alters it afterwards; day partitions and every later change to the schema remain the vendor's |
| Distribution | Two GitHub release assets: `SemiBase-Setup.exe`, which carries the Windows binary, and the Linux binary; the Linux binary also as `ghcr.io/semiteq/semibase`, `FROM scratch`, one file, pushed after the release exists; benches track `:latest` and a prerelease never moves it |
| Backup method | `UNDECIDED` — commissioning-time, needs the customer's archive size |
| Retention depth / disk sizing | `UNDECIDED` — needs a measured write rate from a working installation |

## Decision records

Frozen snapshots of a real evaluation behind a locked decision — the options
considered, the rejected alternatives, and the constraints at the time. Written
once, never edited; supersede by adding a new file whose first line is
`Supersedes <file>`. Add one **only** when a genuine comparison happened that git
history cannot reconstruct.

- none yet — the summary-tables / TimescaleDB / in-database-scheduler evaluation lives in the
  SemiPlot repository (`docs/architecture/history-read-path-evaluation.md` there)

# SemiBase Architecture

Declarative architecture docs (English, present tense). These describe the system
**as it is**, not how it got here. Provisioning commands live in the root
`CLAUDE.md`; dated plans live in `docs/plans/`.

## Documents

- [overview.md](./overview.md) — purpose, consumers, components, provisioning order.
- [configuration.md](./configuration.md) — every setting changed from the PostgreSQL default, with why.
- [provisioning.md](./provisioning.md) — the tool's two commands, idempotency rules, the four
  roles, the SemiPlot configuration schema, the access
  chain, and the stated assumption about the SCADA meeting an existing `trends`.

## Locked decisions

| Area | Decision |
| --- | --- |
| Installation | PostgreSQL via `winget`, major version pinned, automatic upgrade disabled; engine install stays outside the tool |
| Major version | 17 for new installs, pinned latest; floor 14 (SemiPlot needs `date_bin`; Simple-Scada 2 docs require 14+) |
| Provisioning tool | Go single binary `semibase.exe` (`pgx/v5`, embedded SQL); no runtime or `psql` on the target machine |
| Instance configuration | Fixed `ALTER SYSTEM` deltas applied by `semibase site` via `pg_reload_conf()`, never by editing `postgresql.conf`; `shared_buffers` takes effect at the next service restart, reported through `pg_settings.pending_restart` |
| Hardware floor | Installation machines guarantee 8 GB RAM and 50 GB database disk; memory settings are fixed constants sized to that floor, identical on every machine |
| Roles | Four: the superuser the tool connects as, `scada_writer` (SCADA), `semiplot` (viewers), and `semiplot_registrar`, a `NOLOGIN` role that owns `semiplot_register_new_pens()` so its `SECURITY DEFINER` body never runs as the superuser. `semiplot` holds `SELECT` on the archive; `SELECT` on `semiplot_tags` and `UPDATE` on its settings columns, never on `id`; full row access to `semiplot_groups` and `semiplot_pen_groups`; `SELECT` on `semiplot_meta`; `EXECUTE` on the function; and no `CREATE` on schema `public` — revoked from `PUBLIC` by `create`, since PostgreSQL 14 still grants it by default. The superuser owns the tables, because it is the connection that creates them. One viewer role rather than a second write-only one: both passwords would share one plaintext file, so the split would separate nothing |
| Command surface | Two commands, `site` and `bench`, named for the situation rather than for the tool's internal steps; they differ in one thing, the memory tuning, which only `site` applies |
| Archive read access | `ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer` set **before** `public.trends` is created, and before the writer first runs. The `semiplot_*` tables are granted table by table instead, since the superuser creates them |
| Objects we add | `semiplot_tags`, `semiplot_groups`, `semiplot_pen_groups`, `semiplot_meta`, the `semiplot_register_new_pens()` function the viewer calls, and the vendor-shaped `public.trends` — no triggers, scheduled jobs, extensions, or other functions; `messages` is not created, the SCADA makes it |
| SemiPlot schema version | `semiplot_meta.schema_version`, `1` in this release, written by `provision` alone. A floor, not an equality: a viewer refuses a database below the version it needs and accepts one above, because the versions this repository issues grow by addition. |
| Archive schema | Owned by Simple-Scada 2 and documented in the SemiPlot repository. This tool creates `public.trends` once, with the vendor's shape, connected as `scada_writer`, and never alters it afterwards; day partitions and every later change to the schema remain the vendor's |
| Distribution | Binaries as GitHub release assets; the Linux binary also as `ghcr.io/semiteq/semibase`, `FROM scratch`, one file, pushed after the release exists; benches track `:latest` and a prerelease never moves it |
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

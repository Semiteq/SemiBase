# SemiBase Architecture

Declarative architecture docs (English, present tense). These describe the system
**as it is**, not how it got here. Provisioning commands live in the root
`CLAUDE.md`; dated plans live in `docs/plans/`.

## Documents

- [overview.md](./overview.md) — purpose, consumers, components, provisioning order.
- [configuration.md](./configuration.md) — every setting changed from the PostgreSQL default, with why.
- [provisioning.md](./provisioning.md) — the tool's commands, idempotency rules, and the
  reader-access chain.

## Locked decisions

| Area | Decision |
| --- | --- |
| Installation | PostgreSQL via `winget`, major version pinned, automatic upgrade disabled; engine install stays outside the tool |
| Major version | 17 for new installs; floor 14 (SemiPlot needs `date_bin`); vendor floor is 12 |
| Provisioning tool | Go single binary `semibase.exe` (`pgx/v5`, embedded SQL); no runtime or `psql` on the target machine |
| Instance configuration | Applied as `ALTER SYSTEM` deltas by `semibase config`, never by editing `postgresql.conf` |
| Roles | `scada_writer` (SCADA), `semiplot_reader` (viewers, `SELECT` only), `semiplot_admin` (commissioning, owns `semiplot_*`) |
| Reader access | `ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer` set **before** the writer first runs |
| Objects we add | `semiplot_tags` only — no triggers, functions, scheduled jobs, or extensions |
| Archive schema | Owned by Simple-Scada 2; documented in the SemiPlot repository, never created or altered here |
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

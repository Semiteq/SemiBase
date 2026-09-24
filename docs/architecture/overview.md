# SemiBase Overview

## Purpose

SemiBase provisions the one PostgreSQL instance shared by every application on a semiconductor
process-tool installation. The instance is infrastructure with several consumers and its own
deployment lifecycle, which is why it is a repository of its own rather than a folder inside any
consumer.

We supply and administer the database server. The SCADA is a client of it: it creates and writes
its own tables inside a database we provisioned, and deletes its own old partitions according to
its own retention setting.

## Consumers

| Consumer | Role | Access |
| --- | --- | --- |
| Simple-Scada 2 (archives into PostgreSQL) | `scada_writer` | Owns and writes `trends`, creates its day partitions, creates and writes `messages`; executes retention |
| SemiPlot (trend viewer) | `semiplot` | `SELECT` on `trends` and `messages`; read and edit a pen's settings in `semiplot_tags`, never its `id`, and never add or delete a pen by hand; `EXECUTE` on `semiplot_register_new_pens()`, which adds a pen for every key SCADA has written; read and write `semiplot_groups` and `semiplot_pen_groups`; `SELECT` on `semiplot_meta` — nothing else |

Two more roles take part. The superuser the tool connects as (`--superuser`, default `postgres`)
owns the four `semiplot_*` tables, because it is the connection that applies their DDL.
`semiplot_registrar`, which logs in nowhere, owns `semiplot_register_new_pens()`, so the function
runs with that role's narrow grants rather than as the superuser
(`provisioning.md#registering-new-pens`). `semiplot` edits
pen settings and groups and can create nothing beside the four tables, because `create` revokes `CREATE` on schema `public`
from `PUBLIC` and the tail check reads it back.

The `semiplot` credential grants process-history reads and pen-catalogue edits, and nothing more,
which is what makes a plaintext password in a client configuration file an acceptable risk. A bug
in the viewer can corrupt the pen catalogue, which is recoverable; it cannot touch `trends`.

## Components

| Component | Responsibility |
| --- | --- |
| `cmd/semibase`, `internal/provision` | `semibase.exe` — idempotent provisioning in two commands, `site` and `bench`: instance tuning (`site` only), database, roles, grants, the four SemiPlot tables, `semiplot_register_new_pens()`, `public.trends`, and the `semiplot` access checks that end every run |
| `sql/semiplot_tags.sql`, `sql/semiplot_register.sql`, `sql/semiplot_groups.sql`, `sql/semiplot_meta.sql` | DDL for the objects we invented — the pen catalogue, the function that registers new pens, the groups and their membership, and the schema version — embedded into the binary |
| `sql/trends.sql` | The vendor's archive-table shape, transcribed and embedded; applied under `SET ROLE scada_writer`, so the table's owner is the writer |
| `Dockerfile` | `ghcr.io/semiteq/semibase` — the Linux binary alone on `scratch`, for consumers that layer it into a bench image |
| `docs/architecture/` | The instance as it is: configuration deltas, provisioning order, ownership |

Nothing else is added to the database. Specifically, and deliberately: no summary tables, no
triggers, no scheduled jobs, no extensions, and no function but `semiplot_register_new_pens()`,
which runs only when the viewer calls it (`provisioning.md#registering-new-pens`). The SCADA's own archive layers already
provide the coarse resolutions; a second copy would duplicate them and add a process that can
silently stop.

## Provisioning order

The order matters, because the default privileges have to be in place before any table
`scada_writer` owns is created.

1. Install the engine: `winget install --id PostgreSQL.PostgreSQL.17 --exact`.
2. `semibase site` — tuning, then the archive database, the three created roles, default privileges,
   `public.trends`, the four SemiPlot tables and `semiplot_register_new_pens()`; it ends by proving `semiplot` reads `trends`, writes
   what it owns, cannot add, delete or re-key a pen and cannot write the archive, and warns while `shared_buffers` waits for a service
   restart.
3. Restart the PostgreSQL service or reboot the machine, so `shared_buffers` takes effect.
4. Point the Simple-Scada project at the database and start it once. It is expected to find
   `trends` in place, write into it, and create the day partitions and `messages` itself — an
   **unverified** assumption, with the experiment that settles it, in
   `provisioning.md` ("Assumption: the SCADA meeting an existing `trends`"). Set
   `log_statement = 'all'` for that first start and read back the DDL the SCADA issued.
5. Start the viewer. Pens appear in `semiplot_tags` on their own, one per variable the SCADA has
   written to `trends`, with default settings the viewer's editor then changes. A person never
   inserts a pen, and only the superuser can delete one.
6. Write the SemiPlot connection file, including the source time zone.

A bench replaces step 2 with `semibase bench` and has no step 3 or 4: the consumer's seeder plays
the writer's part.

Every consumer must survive every intermediate state of this sequence — no database, empty
`trends`, `trends` without `semiplot_tags`, `semiplot_tags` without rows. Each is a normal
condition, not a crash.

## Upgrades

- PostgreSQL minor versions apply in place with a service restart and are safe.
- PostgreSQL major versions require `pg_upgrade` and a planned outage. Never automatic.
- Simple-Scada upgrades can change the archive schema without notice — it is a vendor internal,
  not a published interface. Readers probe the table shape at startup and report incompatibility
  instead of producing wrong charts; that probe belongs to the readers, not to this repository.

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
| SemiPlot (trend viewer) | `semiplot_reader` | `SELECT` on `trends` and `semiplot_tags` — nothing else |

The `semiplot_*` objects are owned by `postgres`; commissioning fills `semiplot_tags` as the
superuser. A dedicated owner role appears when a tag-editing mechanism exists to hold it.

The reader credential grants process-history reads and nothing more, which is what makes a
plaintext password in a client configuration file an acceptable risk.

## Components

| Component | Responsibility |
| --- | --- |
| `cmd/semibase`, `internal/provision` | `semibase.exe` — idempotent provisioning in two commands, `site` and `bench`: instance tuning (`site` only), database, roles, grants, `semiplot_tags`, `public.trends`, and the reader-access checks that end every run |
| `sql/semiplot_tags.sql` | DDL for the one object we invented, embedded into the binary |
| `sql/trends.sql` | The vendor's archive-table shape, transcribed and embedded; applied under `SET ROLE scada_writer`, so the table's owner is the writer |
| `Dockerfile` | `ghcr.io/semiteq/semibase` — the Linux binary alone on `scratch`, for consumers that layer it into a bench image |
| `docs/architecture/` | The instance as it is: configuration deltas, provisioning order, ownership |

Nothing else is added to the database. Specifically, and deliberately: no summary tables, no
triggers, no functions, no scheduled jobs, no extensions. The SCADA's own archive layers already
provide the coarse resolutions; a second copy would duplicate them and add a process that can
silently stop.

## Provisioning order

The order matters, because the default privileges have to be in place before any table
`scada_writer` owns is created.

1. Install the engine: `winget install --id PostgreSQL.PostgreSQL.17 --exact`.
2. `semibase site` — tuning, then the archive database, both roles, default privileges,
   `semiplot_tags` and `public.trends`; it ends by proving the reader reads `trends` and cannot
   write it, and warns while `shared_buffers` waits for a service restart.
3. Restart the PostgreSQL service or reboot the machine, so `shared_buffers` takes effect.
4. Point the Simple-Scada project at the database and start it once. It is expected to find
   `trends` in place, write into it, and create the day partitions and `messages` itself — an
   **unverified** assumption, with the experiment that settles it, in
   `provisioning.md` ("Assumption: the SCADA meeting an existing `trends`"). Set
   `log_statement = 'all'` for that first start and read back the DDL the SCADA issued.
5. Fill `semiplot_tags` with the variables to be trended.
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

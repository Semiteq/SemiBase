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
| Simple-Scada 2 (archives into PostgreSQL) | `scada_writer` | Creates and writes `trends`/`messages` and their partitions; executes retention |
| SemiPlot (trend viewer) | `semiplot_reader` | `SELECT` on `trends`, `messages`, `semiplot_tags` — nothing else |

The `semiplot_*` objects are owned by `postgres`; commissioning fills `semiplot_tags` as the
superuser. A dedicated owner role appears when a tag-editing mechanism exists to hold it.

The reader credential grants process-history reads and nothing more, which is what makes a
plaintext password in a client configuration file an acceptable risk.

## Components

| Component | Responsibility |
| --- | --- |
| `cmd/semibase`, `internal/provision` | `semibase.exe` — idempotent provisioning: instance configuration, database, roles, grants, `semiplot_tags`, post-writer verification |
| `sql/semiplot_tags.sql` | DDL for the one object we add to the archive database, embedded into the binary |
| `docs/architecture/` | The instance as it is: configuration deltas, provisioning order, ownership |

Nothing else is added to the database. Specifically, and deliberately: no summary tables, no
triggers, no functions, no scheduled jobs, no extensions. The SCADA's own archive layers already
provide the coarse resolutions; a second copy would duplicate them and add a process that can
silently stop.

## Provisioning order

The order matters, because the archive tables do not exist until the SCADA has run once.

1. Install the engine: `winget install --id PostgreSQL.PostgreSQL.17 --exact`.
2. `semibase config` — apply the configuration deltas; `shared_buffers` takes effect at the
   next service restart or reboot, and `verify` warns while it waits.
3. `semibase create` — archive database, both roles, default privileges, `semiplot_tags`.
4. Point the Simple-Scada project at the database and start it once. It creates `trends`,
   `messages` and the first daily partitions.
5. `semibase verify` — prove the reader access chain against the tables the writer created.
6. Fill `semiplot_tags` with the variables to be trended.
7. Write the SemiPlot connection file, including the source time zone.

Every consumer must survive every intermediate state of this sequence — no database, database
without `trends`, `trends` without `semiplot_tags`, `semiplot_tags` without rows. Each is a normal
condition, not a crash.

## Upgrades

- PostgreSQL minor versions apply in place with a service restart and are safe.
- PostgreSQL major versions require `pg_upgrade` and a planned outage. Never automatic.
- Simple-Scada upgrades can change the archive schema without notice — it is a vendor internal,
  not a published interface. Readers probe the table shape at startup and report incompatibility
  instead of producing wrong charts; that probe belongs to the readers, not to this repository.

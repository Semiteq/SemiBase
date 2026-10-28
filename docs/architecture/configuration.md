# Instance configuration

Every setting the `site` command writes is listed. Most differ from the PostgreSQL default.
Two (`effective_cache_size`, `checkpoint_completion_target`) equal it and are pinned
deliberately, so the values in force are visible here rather than implied. `listen_addresses`
equals the compiled default too, and its pin is what changes the server: it overrides the value
the installed `postgresql.conf` sets ([Network](#network)). The workload is a
sustained append-only insert stream from one writer plus a small number of read-heavy
analytical queries.

Every installation machine guarantees at least 8 GB of RAM and 50 GB of disk for the database,
so the memory figures are fixed constants sized to that floor: every machine runs the identical
configuration, and no setting depends on inspecting the host.

`semibase site` writes these through `ALTER SYSTEM`, so they live in `postgresql.auto.conf` and
survive a reinstall of `postgresql.conf`, and applies them with `pg_reload_conf()`. Every setting
below except `shared_buffers` and `listen_addresses` is reload-context or weaker and takes effect
immediately; those two take effect at the next service restart or reboot. The tuning phase prints
the server's own pending-restart list (`pg_settings.pending_restart`), and the run reads it again
last ([provisioning.md](./provisioning.md#what-the-tail-checks-prove), check 10).
`site --restart-service <name>` restarts that Windows service inside the tuning phase when the
list is not empty ([provisioning.md](./provisioning.md#the-service-restart)).

`semibase bench` writes none of them ([provisioning.md](./provisioning.md#commands) says why).

## Server settings

| Setting | Default | Ours | Why |
| --- | --- | --- | --- |
| `shared_buffers` | 128 MB | 2 GB | The archive working set is far larger than the default cache. A quarter of the 8 GB hardware floor. Takes effect at a service restart. |
| `effective_cache_size` | 4 GB | 4 GB | Planner hint only; set explicitly at half the hardware floor so it never falls below the value the readers' index scans depend on. |
| `work_mem` | 4 MB | 64 MB | The readers' pixel-bucket query groups and sorts; at the default it spills to disk on wide windows. |
| `maintenance_work_mem` | 64 MB | 512 MB | Index builds and vacuum on daily partitions. |
| `max_wal_size` | 1 GB | 8 GB | Under a sustained insert stream the default forces frequent checkpoints, each a write burst that stalls queries. |
| `checkpoint_timeout` | 5 min | 30 min | Same reason: fewer, larger, smoother checkpoints. |
| `checkpoint_completion_target` | 0.9 | 0.9 | Already the default in 14+; set explicitly because it matters and must not be lowered. |
| `wal_compression` | off | on | Cuts write-ahead log volume on a partitioned insert workload for a small CPU cost. |
| `random_page_cost` | 4.0 | 1.1 | The default assumes a spinning disk and discourages index scans. The machine has a solid-state drive. |
| `log_min_duration_statement` | off | 1000 ms | A slow query must leave a trace; the only diagnostic that survives an operator restarting the client. The exception is `semibase`'s own superuser sessions, which set it to -1 to keep password statements out of the log ([provisioning.md](./provisioning.md#invariants-the-provision-package-owns)). |
| `track_io_timing` | off | on | Makes `EXPLAIN (ANALYZE, BUFFERS)` usable when a query is slow in the field. |
| `listen_addresses` | `localhost` | `localhost` | Every client of the instance runs on the installation machine ([Network](#network)). Pinned because the installed `postgresql.conf` sets another value: on the Windows runner's PostgreSQL 17 and in the `postgres:17-alpine` container the setting is pending after `site` (measured in CI). Takes effect at a service restart. |

Left at defaults deliberately: `max_connections` (a handful of clients), autovacuum
(PostgreSQL 13+ already triggers vacuum on insert-only tables, which is what sets the visibility
map on closed partitions), and `fillfactor` (the archive is append-only, never updated).

## Role-level settings

Set on the reading role rather than globally, so a badly framed chart query can never stall the
SCADA's writes:

| Setting | Value | Why |
| --- | --- | --- |
| `statement_timeout` | 30 s | A read that exceeds this is a bug in the reader's layer selection, not a slow disk. Fail it and report. |
| `idle_in_transaction_session_timeout` | 60 s | A stuck client must not hold back vacuum on the partitions. |

## Network

The server listens on the loopback interface only. The service the setup installs runs as
`NT AUTHORITY\NetworkService` ([installer.md](./installer.md#what-a-successful-install-leaves)).
Every client of the instance runs on the same Windows machine: the SCADA, the viewers and
`semibase` itself. Nothing on the operator network may reach the server.

`site` sets `listen_addresses = 'localhost'` through `ALTER SYSTEM`. The value lands in
`postgresql.auto.conf`, which the server reads after `postgresql.conf`, so it overrides whatever
the installer wrote there. Once the service restarts, a client that names the machine's own IP
address or host name instead of `localhost` or `127.0.0.1` is refused.

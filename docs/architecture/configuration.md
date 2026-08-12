# Instance configuration

Only settings changed from the PostgreSQL default are listed. The workload is a sustained
append-only insert stream from one writer plus a small number of read-heavy analytical queries.
The `config` command of `semibase.exe` applies these through `ALTER SYSTEM`, so they live in
`postgresql.auto.conf` and survive a reinstall of `postgresql.conf`.

## Server settings

| Setting | Default | Ours | Why |
| --- | --- | --- | --- |
| `shared_buffers` | 128 MB | 25% of RAM | The archive working set is far larger than the default cache; the standard starting point for a dedicated server. |
| `effective_cache_size` | 4 GB | 50% of RAM | Planner hint only. Too low a value pushes the planner away from the index scans the readers depend on. |
| `work_mem` | 4 MB | 64 MB | The readers' pixel-bucket query groups and sorts; at the default it spills to disk on wide windows. |
| `maintenance_work_mem` | 64 MB | 512 MB | Index builds and vacuum on daily partitions. |
| `max_wal_size` | 1 GB | 8 GB | Under a sustained insert stream the default forces frequent checkpoints, each a write burst that stalls queries. |
| `checkpoint_timeout` | 5 min | 30 min | Same reason: fewer, larger, smoother checkpoints. |
| `checkpoint_completion_target` | 0.9 | 0.9 | Already the default in 14+; set explicitly because it matters and must not be lowered. |
| `wal_compression` | off | on | Cuts write-ahead log volume on a partitioned insert workload for a small CPU cost. |
| `random_page_cost` | 4.0 | 1.1 | The default assumes a spinning disk and discourages index scans. The machine has a solid-state drive. |
| `log_min_duration_statement` | off | 1000 ms | A slow query must leave a trace; the only diagnostic that survives an operator restarting the client. |
| `track_io_timing` | off | on | Makes `EXPLAIN (ANALYZE, BUFFERS)` usable when a query is slow in the field. |

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

The service runs under a dedicated Windows account and listens on the loopback interface plus the
operator network only.

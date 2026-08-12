# Provisioning

One tool, `semibase.exe` (Go, `cmd/semibase`), provisions development benches and the
production instance alike. That is the point: a tool that has created the development database
many times is proven; a tool written for commissioning and run once, on site, on the day it
matters, is a liability.

The binary is self-contained: `pgx/v5` talks to the server directly over TCP, so neither
`psql.exe` nor any runtime needs to be present on the target machine. `sql/semiplot_tags.sql`
is embedded at build time. Installing the PostgreSQL engine itself is one documented `winget`
line, deliberately outside the tool — OS package management is not database provisioning.

## Commands

| Command | What it does | When |
| --- | --- | --- |
| `config` | Applies the `ALTER SYSTEM` deltas from `configuration.md`; restarts the Windows service when `--service` names one | Production and dedicated benches. Never against a shared dev server — it retunes the whole instance |
| `create` | Creates the archive database, the three roles, the grants, the default privileges, and `semiplot_tags` | Before the SCADA's first start |
| `verify` | Proves the reader access chain against the tables the writer created | After the SCADA's first start |
| `all` | `config` + `create` + `verify`; `verify` reports "writer has not run" as a distinct state, not a failure | Fresh instance |

Every step checks before it creates, so re-running any command is safe. Role passwords are set
only when the corresponding flag or environment variable is present (`SEMIBASE_SUPER_PASSWORD`,
`SEMIBASE_WRITER_PASSWORD`, `SEMIBASE_READER_PASSWORD`, `SEMIBASE_ADMIN_PASSWORD`); a `.env`
file in the working directory is read as the lowest-precedence source (flag > environment >
`.env`; `.env.example` is the template, `.env` is gitignored). A run without passwords leaves
existing credentials untouched.

## The reader-access chain

`semiplot_reader` needs `SELECT` on `trends` and `messages` — tables that do not exist until the
SCADA has run once. A plain `GRANT` cannot be issued on a table that is not there, so `create`
sets:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE scada_writer IN SCHEMA public
    GRANT SELECT ON TABLES TO semiplot_reader;
```

This must be in place **before** the writer first starts; tables the writer creates afterwards are
readable automatically. If the writer ran first, the repair is a one-time
`GRANT SELECT ON ALL TABLES IN SCHEMA public` plus the default-privileges statement for the
partitions still to come — `verify` detects this state and says so.

On PostgreSQL 15 and later the `public` schema no longer grants `CREATE` to everyone, so
`create` grants it to `scada_writer` explicitly. On 14 the grant is redundant and harmless.

## What `verify` proves

1. `public.trends` and `public.messages` exist — the writer has run.
2. `semiplot_reader` holds `SELECT` and does not hold `INSERT` on `trends`.
3. When the reader password is supplied, an actual connection as `semiplot_reader` reads one row —
   the whole chain, not just the catalog view of it.
4. The default partition is empty. Rows in it mean a day partition was missing at write time,
   which is a writer-side fault worth surfacing during commissioning.

## Development benches

The same `create` command provisions a bench database on a developer machine
(`--port 15432 --database semiplot_dev --expected-major 14` against the local PostgreSQL 14).
Two rules make the bench exercise what production exercises:

- The bench seeder connects **as `scada_writer`** and creates the archive tables itself, the way
  the SCADA would. This is what makes the default-privileges chain a daily-tested path instead of
  a commissioning-day surprise.
- Bench integration tests read **as `semiplot_reader`**, never as a superuser, so a broken grant
  fails a test today.

The seeder and the tests live in the SemiPlot repository; this repository only promises them a
correctly provisioned database.

## Passwords

The tool takes passwords through flags or environment variables and never stores them. The reader
credential ends up in plaintext in client configuration files by design — it grants reading
process history and nothing more. The writer and admin credentials are recorded wherever the
installation keeps its commissioning records, outside this repository.

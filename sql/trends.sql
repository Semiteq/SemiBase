-- The Simple-Scada 2 archive table, transcribed from the vendor's own definition (a
-- customer archive dump read with `pg_restore --schema-only`). The shape is the vendor's,
-- not ours: this tool creates the table so the reader's access chain is complete before the
-- SCADA's first start, and never alters it afterwards.
--
-- Applied under `SET ROLE scada_writer`, so the table's owner is the role the SCADA writes
-- with and the reader's SELECT arrives through the default privileges set for that role. A
-- superuser-owned table would give the reader access for a different reason than a site gets
-- it. SET ROLE rather than a `scada_writer` login: the owner and the ACL come out identical,
-- and no pg_hba.conf line has to admit that role.
--
-- `messages` is not created: nothing we ship reads it, and every object we create is a
-- surface that can drift from the vendor.
--
-- Day partitions `tpYYYYmMMdDD` are not created here — the SCADA creates them on a site,
-- the consumer's bench seeder creates them on a bench. Only the `tpdefault` catch-all is
-- created; rows landing in it signal a missing day partition.

CREATE TABLE public.trends (
	id integer DEFAULT 0 NOT NULL,
	l smallint DEFAULT 0 NOT NULL,
	t timestamp(3) without time zone NOT NULL,
	v double precision,
	q integer NOT NULL
)
PARTITION BY RANGE (t);

ALTER TABLE ONLY public.trends
	ADD CONSTRAINT tpk PRIMARY KEY (id, l, t);

CREATE TABLE public.tpdefault PARTITION OF public.trends DEFAULT;

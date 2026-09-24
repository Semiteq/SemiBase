-- docs/architecture/provisioning.md#the-semiplot-configuration-schema

CREATE TABLE IF NOT EXISTS semiplot_groups (
	id   integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	name text NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS semiplot_pen_groups (
	pen_id   integer NOT NULL REFERENCES semiplot_tags (id) ON DELETE CASCADE,
	group_id integer NOT NULL REFERENCES semiplot_groups (id) ON DELETE CASCADE,
	PRIMARY KEY (pen_id, group_id)
);

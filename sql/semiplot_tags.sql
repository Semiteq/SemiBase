-- docs/architecture/provisioning.md#the-semiplot-configuration-schema

CREATE TABLE IF NOT EXISTS semiplot_tags (
	id               integer PRIMARY KEY,
	name             text    NOT NULL,
	unit             text,
	format           text,
	color            text,
	line_style       smallint NOT NULL DEFAULT 0,
	enabled_on_start boolean  NOT NULL DEFAULT true,
	scale_min        double precision,
	scale_max        double precision,
	CONSTRAINT semiplot_tags_scale_paired CHECK (
		(scale_min IS NULL) = (scale_max IS NULL)
		AND (scale_min IS NULL OR scale_min < scale_max)),
	CONSTRAINT semiplot_tags_color_hex CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$')
);

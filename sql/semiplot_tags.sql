-- docs/architecture/provisioning.md#the-semiplot-configuration-schema

CREATE TABLE IF NOT EXISTS semiplot_tags (
	id                 integer PRIMARY KEY,
	name               text    NOT NULL,
	unit               text,
	format             text,
	color              text,
	line_style         smallint NOT NULL DEFAULT 0,
	enabled_on_start   boolean  NOT NULL DEFAULT true,
	scale_min_on_start double precision,
	scale_max_on_start double precision,
	log_scale_on_start boolean  NOT NULL DEFAULT false,
	CONSTRAINT semiplot_tags_scale_paired CHECK (
		(scale_min_on_start IS NULL) = (scale_max_on_start IS NULL)
		AND (scale_min_on_start IS NULL OR scale_min_on_start < scale_max_on_start)),
	CONSTRAINT semiplot_tags_color_hex CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$')
);

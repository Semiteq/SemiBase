-- The one object SemiBase adds to the archive database: the mapping from the
-- archive's variable number to a human-readable pen definition. The archive
-- itself has no such mapping. Filled during commissioning; optionally seeded
-- from the vendor's variables_data table when the operator has created it.

CREATE TABLE IF NOT EXISTS semiplot_tags (
	id         integer PRIMARY KEY,   -- matches trends.id
	name       text    NOT NULL,
	group_name text,
	unit       text,
	color      text,
	line_style smallint NOT NULL DEFAULT 0
);

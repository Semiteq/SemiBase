-- docs/architecture/provisioning.md#the-semiplot-configuration-schema

CREATE TABLE IF NOT EXISTS semiplot_markers (
	id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	t      timestamptz NOT NULL,
	label  text CHECK (char_length(label) BETWEEN 1 AND 80),
	kind   smallint NOT NULL DEFAULT 0 CHECK (kind BETWEEN 0 AND 3),
	locked boolean  NOT NULL DEFAULT false
);

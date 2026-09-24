-- docs/architecture/provisioning.md#the-schema-version-is-a-floor

CREATE TABLE IF NOT EXISTS semiplot_meta (
	singleton      boolean PRIMARY KEY DEFAULT true CHECK (singleton),
	schema_version integer NOT NULL
);

INSERT INTO semiplot_meta (singleton, schema_version)
VALUES (true, 1)
ON CONFLICT (singleton) DO UPDATE SET schema_version = EXCLUDED.schema_version
WHERE semiplot_meta.schema_version < EXCLUDED.schema_version;

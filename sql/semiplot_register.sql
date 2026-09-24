-- docs/architecture/provisioning.md#registering-new-pens

CREATE OR REPLACE FUNCTION semiplot_register_new_pens() RETURNS integer
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
	WITH RECURSIVE keys AS (
		(SELECT id FROM public.trends ORDER BY id LIMIT 1)
		UNION ALL
		SELECT (SELECT t.id FROM public.trends t WHERE t.id > keys.id ORDER BY t.id LIMIT 1)
		FROM keys WHERE keys.id IS NOT NULL
	), added AS (
		INSERT INTO public.semiplot_tags (id, name, color, enabled_on_start)
		SELECT id, id::text,
			(ARRAY['#4E79A7','#F28E2B','#E15759','#76B7B2','#59A14F','#EDC948',
				'#B07AA1','#FF9DA7','#9C755F','#17BECF','#D62728','#9467BD'])[id % 12 + 1],
			false
		FROM keys WHERE id IS NOT NULL
		ON CONFLICT (id) DO NOTHING
		RETURNING 1
	)
	SELECT count(*)::integer FROM added;
$$;

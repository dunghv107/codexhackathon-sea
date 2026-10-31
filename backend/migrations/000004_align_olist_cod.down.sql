DO $$ BEGIN RAISE EXCEPTION 'Olist alignment is forward-only: restore a backup to revert without losing provenance or auth links'; END $$;

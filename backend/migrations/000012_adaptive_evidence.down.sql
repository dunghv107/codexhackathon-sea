-- Evidence and assessment history must not be silently discarded by a rollback.
DO $$ BEGIN
    RAISE EXCEPTION 'Migration 12 is forward-only: roll back application code while retaining risk evidence';
END $$;

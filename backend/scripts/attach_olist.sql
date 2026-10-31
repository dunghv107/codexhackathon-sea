-- Run only against a database with the canonical sfd schema already loaded.
\set ON_ERROR_STOP on
SET search_path TO public;
SELECT to_regclass('public.users') IS NULL AS needs_users \gset
\if :needs_users
\ir ../migrations/000001_create_users.up.sql
\endif
\ir ../migrations/000004_align_olist_cod.up.sql
-- Adopt the imported schema only if no migration ledger exists. Never override a dirty ledger.
CREATE TABLE IF NOT EXISTS public.schema_migrations(version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL);
INSERT INTO public.schema_migrations SELECT 4,false WHERE NOT EXISTS (SELECT 1 FROM public.schema_migrations);

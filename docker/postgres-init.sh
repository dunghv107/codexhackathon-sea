#!/bin/sh
set -eu

# Runs only on an empty volume. Quote psql variables as identifiers/literals.
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 \
    --set=app_user="$APP_DB_USER" --set=app_password="$APP_DB_PASSWORD" \
    --set=db_name="$POSTGRES_DB" --set=admin_user="$POSTGRES_USER" <<'SQL'
CREATE ROLE :"app_user" LOGIN PASSWORD :'app_password'
    NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
GRANT CONNECT ON DATABASE :"db_name" TO :"app_user";
GRANT USAGE ON SCHEMA public TO :"app_user";
ALTER DEFAULT PRIVILEGES FOR ROLE :"admin_user"
    GRANT USAGE ON SCHEMAS TO :"app_user";
ALTER DEFAULT PRIVILEGES FOR ROLE :"admin_user"
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"app_user";
ALTER DEFAULT PRIVILEGES FOR ROLE :"admin_user"
    GRANT USAGE, SELECT ON SEQUENCES TO :"app_user";
SQL

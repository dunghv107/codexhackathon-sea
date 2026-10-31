#!/usr/bin/env bash
# Requires PostgreSQL client tools and an EMPTY, disposable test database.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${BOOTSTRAP_TEST_DATABASE_URL:?Set to an empty disposable PostgreSQL database}"
query() { psql "$BOOTSTRAP_TEST_DATABASE_URL" -X -qAt -v ON_ERROR_STOP=1 "$@"; }
[[ $(query -c "SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')") == 0 ]] || {
  echo 'Refusing nonempty test database' >&2; exit 1;
}
for migration in \
  backend/migrations/000001_create_users.up.sql \
  backend/migrations/000002_create_sfd_schema.up.sql \
  backend/migrations/000004_align_olist_cod.up.sql \
  backend/migrations/000005_demo_scoring_batch.up.sql; do
  query -f "$migration" >/dev/null
done

# Exercise the actual bootstrap stdin/credential handling with local PostgreSQL in place of Docker.
qa_dir=$(mktemp -d)
trap 'rm -rf "$qa_dir"' EXIT
cat > "$qa_dir/docker" <<'SH'
#!/usr/bin/env bash
exec psql "$BOOTSTRAP_TEST_DATABASE_URL" -X -q -v ON_ERROR_STOP=1
SH
chmod +x "$qa_dir/docker"
export PATH="$qa_dir:$PATH"
export BOOTSTRAP_BUYER_PASSWORD="qa-buyer-'quote" BOOTSTRAP_SELLER_PASSWORD=qa-seller-password

if BOOTSTRAP_BUYER_PASSWORD=short bash scripts/bootstrap-demo.sh >"$qa_dir/invalid.log" 2>&1; then
  echo 'Accepted short password' >&2; exit 1
fi
[[ $(query -c 'SELECT count(*) FROM public.users') == 0 ]]
query -c "INSERT INTO public.users(username,email,password) VALUES ('buyer_demo','existing@example.test','untouched')"
if bash scripts/bootstrap-demo.sh >"$qa_dir/conflict.log" 2>&1; then
  echo 'Took over an existing username' >&2; exit 1
fi
[[ $(query -c "SELECT count(*) FROM public.users WHERE username='seller_demo'") == 0 ]]
[[ $(query -c "SELECT password FROM public.users WHERE username='buyer_demo'") == untouched ]]
query -c "DELETE FROM public.users WHERE username='buyer_demo'"

bash scripts/bootstrap-demo.sh
[[ $(query -c 'SELECT count(*) FROM public.users') == 2 ]]
[[ $(query -c "SELECT count(*) FROM sfd.listings WHERE currency='VND' AND batch_id='olist_vn_sim'") == 3 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.saved_addresses WHERE is_default') == 1 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 0 ]]
[[ $(query -c "SELECT count(*) FROM public.users u JOIN sfd.buyers b ON b.auth_subject='local:' || u.id WHERE u.username='buyer_demo'") == 1 ]]
[[ $(query -c "SELECT count(*) FROM public.users u JOIN sfd.sellers s ON s.auth_subject='local:' || u.id JOIN sfd.shops sh ON sh.owner_seller_id=s.seller_id WHERE u.username='seller_demo'") == 1 ]]
[[ $(query -c "SELECT password=public.crypt(\$qa\$qa-buyer-'quote\$qa\$,password) FROM public.users WHERE username='buyer_demo'") == t ]]
original_hashes=$(query -c 'SELECT password FROM public.users ORDER BY username')
query -c "UPDATE sfd.listings SET current_price=99000,stock_quantity=7 WHERE listing_id='live:bootstrap-listing-1'"
BOOTSTRAP_BUYER_PASSWORD=different-password BOOTSTRAP_SELLER_PASSWORD=different-password bash scripts/bootstrap-demo.sh
[[ $(query -c 'SELECT count(*) FROM public.users') == 2 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.listings') == 3 ]]
[[ $(query -c 'SELECT password FROM public.users ORDER BY username') == "$original_hashes" ]]
[[ $(query -c "SELECT current_price || ':' || stock_quantity FROM sfd.listings WHERE listing_id='live:bootstrap-listing-1'") == '99000.00:7' ]]
echo 'PASS: validation, ownership conflict rollback, quoted password hash, linked profiles, VND catalog and non-destructive rerun'

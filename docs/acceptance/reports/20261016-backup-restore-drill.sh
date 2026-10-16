#!/usr/bin/env bash
set -euo pipefail
NS=hnb-system
POD=postgresql-0

cat > /tmp/pgdrill.sh <<'INNER'
set -e
export PGPASSWORD="$(cat /var/run/hnb-pg-admin/password)"
PSQL="psql -h 127.0.0.1 -U hnb_admin -v ON_ERROR_STOP=1 -tA"
echo "== databases =="
$PSQL -d postgres -c "select datname from pg_database order by 1"
echo "== source table count (hnb_platform) =="
$PSQL -d hnb_platform -c "select count(*) from information_schema.tables where table_schema='public'"
echo "== dump =="
pg_dump -h 127.0.0.1 -U hnb_admin -d hnb_platform -Fc -f /tmp/platform.dump
ls -l /tmp/platform.dump
echo "== restore into scratch db =="
$PSQL -d postgres -c "drop database if exists hnb_platform_restore"
$PSQL -d postgres -c "create database hnb_platform_restore"
pg_restore -h 127.0.0.1 -U hnb_admin -d hnb_platform_restore --no-owner --no-privileges /tmp/platform.dump
echo "== restored table count =="
$PSQL -d hnb_platform_restore -c "select count(*) from information_schema.tables where table_schema='public'"
echo "== cleanup scratch db =="
$PSQL -d postgres -c "drop database hnb_platform_restore"
echo PG_DRILL_OK
INNER

echo "=== upload + run PG drill ==="
if sudo k3s kubectl cp /tmp/pgdrill.sh "$NS/$POD:/tmp/pgdrill.sh" 2>/dev/null; then
  sudo k3s kubectl exec -n "$NS" "$POD" -- sh /tmp/pgdrill.sh
else
  echo "kubectl cp unavailable; streaming via stdin"
  sudo k3s kubectl exec -i -n "$NS" "$POD" -- sh -c 'cat > /tmp/pgdrill.sh' < /tmp/pgdrill.sh
  sudo k3s kubectl exec -n "$NS" "$POD" -- sh /tmp/pgdrill.sh
fi

echo "=== identity material backup/restore integrity ==="
sudo tar czf /tmp/identity-backup.tgz -C /var/lib/hnb-minimal-eval identity
echo "archive contents:"; sudo tar tzf /tmp/identity-backup.tgz
rm -rf /tmp/idrestore && mkdir -p /tmp/idrestore
sudo tar xzf /tmp/identity-backup.tgz -C /tmp/idrestore
ok=0; bad=0
for f in master.key admin-bootstrap.password private.pem public.pem manifest.json identity-state.json; do
  a=$(sudo sha256sum "/var/lib/hnb-minimal-eval/identity/$f" | cut -d' ' -f1)
  b=$(sudo sha256sum "/tmp/idrestore/identity/$f" | cut -d' ' -f1)
  if [ "$a" = "$b" ]; then echo "OK $f $a"; ok=$((ok+1)); else echo "MISMATCH $f"; bad=$((bad+1)); fi
done
rm -rf /tmp/idrestore
echo "identity files ok=$ok bad=$bad"
echo DRILL_DONE

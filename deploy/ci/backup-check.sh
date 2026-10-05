#!/bin/sh
# Tests of the backup sidecar script (docs §12.5).
# Usage: deploy/ci/backup-check.sh <backup-image>
#   1. an unreachable database fails the run: non-zero exit, nothing
#      published, nothing pruned, no .part left behind;
#   2. an invalid age recipient fails before anything is written;
#   3. with a recipient every artifact is encrypted and decryptable, and
#      no plaintext file is ever published.
set -eu
IMAGE=${1:?backup image}
WORK=$(mktemp -d)
NET=lpbackup$$
PG=lpbackup-db-$$
cleanup() {
  docker rm -f "$PG" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker run --rm -v "$WORK:/w" --entrypoint rm "$IMAGE" -rf /w/backups >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() { echo "backup-check: $*" >&2; exit 1; }
mkdir -p "$WORK/backups" "$WORK/src/config" "$WORK/src/secrets" "$WORK/data/uploads"
echo "pw" > "$WORK/pg_password"
echo "base_url: http://x" > "$WORK/src/config/config.yaml"
echo "hash" > "$WORK/src/secrets/admin_owner"
echo "token" > "$WORK/src/secrets/tunnel_token"
echo "k" > "$WORK/data/.secret_key"
echo "img" > "$WORK/data/uploads/a.png"
touch -t 202001010000 "$WORK/backups/db-old.dump"

run() { # run [docker args...] IMAGE [script args]; hardened like compose.yaml
  docker run --rm --read-only --tmpfs /tmp --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE \
    --security-opt no-new-privileges:true -v "$WORK/backups:/backups" -v "$WORK/pg_password:/run/secrets/pg_password:ro" \
    -v "$WORK/src:/src:ro" -v "$WORK/data:/data:ro" -e BACKUP_KEEP_DAYS=1 "$@"
}

# 1. Dead database.
if run -e PGHOST=127.0.0.1 -e PGPORT=1 "$IMAGE" once; then fail "dead database must fail"; fi
[ -f "$WORK/backups/db-old.dump" ] || fail "old backups must not be pruned after a failure"
[ "$(find "$WORK/backups" -type f | wc -l)" -eq 1 ] || fail "failed run left files: $(ls -A "$WORK/backups")"

# 2. Invalid recipient.
if run -e PGHOST=127.0.0.1 -e BACKUP_AGE_RECIPIENT=age1invalid "$IMAGE" once; then fail "invalid recipient must fail"; fi
[ "$(find "$WORK/backups" -type f | wc -l)" -eq 1 ] || fail "invalid recipient wrote files"

# 3. Encrypted backup against a real database.
docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_USER=linkspage -e POSTGRES_DB=linkspage \
  -e POSTGRES_PASSWORD=pw postgres:18-alpine >/dev/null
i=0
until docker exec "$PG" pg_isready -h 127.0.0.1 -U linkspage -d linkspage >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -lt 60 ] || fail "postgres did not start"; sleep 1
done
key=$(docker run --rm --entrypoint age-keygen "$IMAGE" 2>/dev/null)
recipient=$(printf '%s\n' "$key" | sed -n 's/^# public key: //p')
[ -n "$recipient" ] || fail "could not generate an age key"
run --network "$NET" -e PGHOST="$PG" -e BACKUP_AGE_RECIPIENT="$recipient" "$IMAGE" once
[ ! -f "$WORK/backups/db-old.dump" ] || fail "successful run must prune old backups"
for prefix in db uploads secret-key config; do
  ls "$WORK/backups/$prefix"-*.age >/dev/null 2>&1 || fail "missing encrypted $prefix backup"
done
plain=$(find "$WORK/backups" -type f ! -name '*.age')
[ -z "$plain" ] || fail "plaintext files published: $plain"
printf '%s\n' "$key" > "$WORK/key.txt"
listing=$(docker run --rm -v "$WORK:/w:ro" --entrypoint sh "$IMAGE" -c \
  'age -d -i /w/key.txt /w/backups/config-*.age | tar -tf -')
printf '%s\n' "$listing" | grep -qx 'secrets/admin_owner' || fail "config archive lacks admin hashes"
if printf '%s\n' "$listing" | grep -q 'tunnel_token'; then fail "config archive must not contain tunnel_token"; fi
docker run --rm -v "$WORK:/w:ro" --entrypoint sh "$IMAGE" -c \
  'age -d -i /w/key.txt /w/backups/db-*.dump.age | pg_restore -l >/dev/null' || fail "database dump is not restorable"
echo "backup check OK"

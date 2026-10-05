#!/bin/sh
# End-to-end smoke test of a built image with the release compose files.
# Usage: deploy/ci/smoke.sh <image> <tag>
set -eu
IMAGE=${1:?image}
TAG=${2:?tag}
PORT=${SMOKE_PORT:-18080}
WORK=$(mktemp -d)
PROJECT=lpsmoke$$
export COMPOSE_PROJECT_NAME="$PROJECT" LINKSPAGE_IMAGE="$IMAGE" LINKSPAGE_TAG="$TAG" LINKSPAGE_PORT="$PORT"

cp compose.yaml .env.example deploy/compose.direct.yaml deploy/init.sh "$WORK/"
cp -R config "$WORK/config"
# A static-only seed (no outbound provider requests) for the M1 routes.
cat > "$WORK/config/smoke-seed.yaml" <<'YAML'
version: 1
communities:
  - slug: qq
    platform: qq-group
    name: { zh-CN: QQ 群, en: QQ group }
    qq_group: "123456789"
    invite: https://qm.qq.com/q/SmokeTest
links:
  - slug: blog
    label: { en: Blog }
    url: https://blog.example.com
YAML
chmod 644 "$WORK/config/smoke-seed.yaml"
cd "$WORK"
dc() { docker compose -f compose.yaml -f compose.direct.yaml "$@"; }
cleanup() {
  dc --profile tunnel logs --no-color app cloudflared > "${SMOKE_LOG:-/tmp/linkspage-smoke.log}" 2>&1 || true
  dc --profile tunnel down -v >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

./init.sh -y --base-url "http://127.0.0.1:$PORT" --profiles tunnel
echo "ci-dummy-token" > secrets/tunnel_token
printf 'seed_file: /etc/linkspage/smoke-seed.yaml\n' >> config/config.yaml
dc run --rm --no-deps app config check
dc --profile tunnel up -d --wait --wait-timeout 180 app db

B="http://127.0.0.1:$PORT"
WECHAT_UA='Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 MicroMessenger/8.0.50'
expect() { # expect PATH STATUS [USER_AGENT]
  got=$(curl -s -o /dev/null -w '%{http_code}' -A "${3:-curl}" "$B$1")
  [ "$got" = "$2" ] || { echo "GET $1: got $got, want $2" >&2; exit 1; }
}
expect /healthz 200
expect /readyz 200
expect / 200
expect /privacy 200
expect /c/qq 200
expect /c/test 404
expect /admin/ 200
expect /api/v1/public/bootstrap 200
expect /api/does-not-exist 404
expect /does-not-exist 404
expect /assets/missing.js 404
expect /api/v1/public/live 200
expect /go/blog 302
expect /go/qq 302
expect /go/qq 200 "$WECHAT_UA"
expect /go/nope 404
expect /media/u/0123456789abcdef0123456789abcdef.webp 404
expect /media/p/AAAAAAAAAAAAAAAAAAAAAA.png 404
expect /media/q/not-a-uuid 404
expect /robots.txt 200
expect /site.webmanifest 200
expect /favicon.ico 200
curl -s -D - -o /dev/null "$B/api/does-not-exist" | grep -qi '^content-type: application/problem+json'
header() { # header NAME < response headers
  awk -F': ' -v n="$1" 'tolower($1)==n{print $2}' | tr -d '\r'
}
h200=$(curl -s -D - -o /dev/null -H 'Accept-Encoding: gzip' "$B/")
etag=$(printf '%s' "$h200" | header etag)
h304=$(curl -s -D - -o /dev/null -H 'Accept-Encoding: gzip' -H "If-None-Match: $etag" "$B/")
printf '%s' "$h304" | head -1 | grep -q ' 304' || { echo "conditional GET is not 304" >&2; exit 1; }
csp=$(printf '%s' "$h200" | header content-security-policy)
csp304=$(printf '%s' "$h304" | header content-security-policy)
if [ -z "$csp" ] || [ "$csp" != "$csp304" ]; then
  echo "304 must carry the same CSP as the 200" >&2
  exit 1
fi
printf '%s' "$h304" | header vary | grep -qi 'accept-encoding' || { echo "304 lacks Vary" >&2; exit 1; }

loc=$(curl -s -D - -o /dev/null "$B/go/blog" | header location)
[ "$loc" = "https://blog.example.com" ] || { echo "/go/blog location: $loc" >&2; exit 1; }
ltag=$(curl -s -D - -o /dev/null "$B/api/v1/public/live" | header etag)
curl -s -D - -o /dev/null -H "If-None-Match: $ltag" "$B/api/v1/public/live" | head -1 | grep -q ' 304' ||
  { echo "conditional live GET is not 304" >&2; exit 1; }

# cloudflared must get the reserved address (the dummy token makes it exit).
dc --profile tunnel up -d --no-deps cloudflared
sleep 3
ip=$(docker inspect "$PROJECT-cloudflared-1" --format '{{json .NetworkSettings.Networks}}' | grep -o '"IPv4Address":"[^"]*"')
[ "$ip" = '"IPv4Address":"172.31.255.2"' ] || { echo "cloudflared address: $ip" >&2; exit 1; }
if docker logs "$PROJECT-cloudflared-1" 2>&1 | grep -q 'Address already in use'; then
  echo "cloudflared: address already in use" >&2
  exit 1
fi
echo "smoke OK"

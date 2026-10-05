#!/bin/sh
# Static checks of the rendered compose configuration (docs §13.2).
# Usage: deploy/ci/compose-check.sh [compose-binary...]
#   e.g. deploy/ci/compose-check.sh docker compose
#        deploy/ci/compose-check.sh ./docker-compose-v2.20.1
set -eu
[ $# -gt 0 ] || set -- docker compose
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

cleanup=""
if [ ! -f .env ]; then
  cp .env.example .env
  cleanup="$cleanup .env"
fi
mkdir -p secrets
for f in pg_password tunnel_token; do
  if [ ! -f "secrets/$f" ]; then
    echo "ci-placeholder" > "secrets/$f"
    cleanup="$cleanup secrets/$f"
  fi
done
trap 'rm -f $cleanup' EXIT

json=$("$@" --profile tunnel --profile backup -f compose.yaml -f deploy/compose.direct.yaml config --format json)

# No interpolated value may start with '#' (inline comment pasted into .env).
bad=$(printf '%s' "$json" | jq -r '[.. | strings | select(startswith("#"))] | .[]')
if [ -n "$bad" ]; then
  echo "values starting with '#':" >&2
  printf '%s\n' "$bad" >&2
  exit 1
fi

# The app environment must not carry secrets; they are passed as *_FILE.
leaks=$(printf '%s' "$json" | jq -r '.services.app.environment | to_entries[]
  | select((.key | test("PASSWORD|SECRET|TOKEN|PRIVATE"; "i")) and (.key | endswith("_FILE") | not)) | .key')
if [ -n "$leaks" ]; then
  echo "secret-looking variables in app environment: $leaks" >&2
  exit 1
fi

# Every service must run with no-new-privileges and without default capabilities.
unhardened=$(printf '%s' "$json" | jq -r '.services | to_entries[]
  | select(((.value.security_opt // []) | index("no-new-privileges:true") | not)
      or ((.value.cap_drop // []) | index("ALL") | not)) | .key')
if [ -n "$unhardened" ]; then
  echo "services without no-new-privileges / cap_drop ALL: $unhardened" >&2
  exit 1
fi

# Profiles and the fixed cloudflared address must survive rendering.
printf '%s' "$json" | jq -e '.services.cloudflared.networks.edge.ipv4_address == "172.31.255.2"' >/dev/null
printf '%s' "$json" | jq -e '.services.backup and .services.app.read_only == true' >/dev/null
echo "compose config OK ($*)"

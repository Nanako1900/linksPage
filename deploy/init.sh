#!/bin/sh
# LinksPage first-time setup: creates .env, secrets/ and config/config.yaml.
# Safe to re-run: existing files are never overwritten.
#
# Usage:
#   ./init.sh                                  interactive
#   ./init.sh -y --base-url URL [--profiles tunnel,backup]
#
# Environment overrides for non-interactive use: LP_BASE_URL, LINKSPAGE_PROFILES.
set -eu
umask 077

APP_GID=65532
NON_INTERACTIVE=0
BASE_URL="${LP_BASE_URL:-}"
PROFILES="${LINKSPAGE_PROFILES:-}"
PROFILES_SET=0
[ -n "${LINKSPAGE_PROFILES+x}" ] && PROFILES_SET=1

die() { printf 'init.sh: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }

usage() {
  sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes|--non-interactive) NON_INTERACTIVE=1 ;;
    --base-url) [ $# -ge 2 ] || die "--base-url needs a value"; BASE_URL="$2"; shift ;;
    --base-url=*) BASE_URL="${1#*=}" ;;
    --profiles) [ $# -ge 2 ] || die "--profiles needs a value"; PROFILES="$2"; PROFILES_SET=1; shift ;;
    --profiles=*) PROFILES="${1#*=}"; PROFILES_SET=1 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
  shift
done

# Run from the directory that contains compose.yaml (release bundle root or
# repository root when invoked as deploy/init.sh).
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
if [ -f "$SCRIPT_DIR/compose.yaml" ]; then
  ROOT="$SCRIPT_DIR"
elif [ -f "$SCRIPT_DIR/../compose.yaml" ]; then
  ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)
else
  die "compose.yaml not found next to init.sh or in its parent directory"
fi
cd "$ROOT"

command -v openssl >/dev/null 2>&1 || die "openssl is required"

ask() { # ask "question" default -> answer on stdout
  if [ "$NON_INTERACTIVE" -eq 1 ] || [ ! -t 0 ]; then
    printf '%s' "$2"
    return
  fi
  printf '%s [%s]: ' "$1" "$2" >&2
  read -r reply || reply=""
  printf '%s' "${reply:-$2}"
}

yes_no() { # yes_no "question" default(y|n) -> exit status
  answer=$(ask "$1 (y/n)" "$2")
  case "$answer" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}

valid_base_url() {
  # Whitelist URL characters: no whitespace, quotes, backslashes or '#',
  # so the value can never break out of its .env line.
  case "$1" in *[!]A-Za-z0-9.:/_~%[-]*) return 1 ;; esac
  case "$1" in
    http://?*|https://?*) ;;
    *) return 1 ;;
  esac
  case "$1" in */) return 1 ;; esac
  return 0
}

# Replace or append KEY=VALUE in .env (values never contain newlines).
# Values are passed through the environment, not `awk -v`, because -v
# interprets backslash escapes (e.g. "\n" would inject a new line).
set_env() {
  tmp=$(mktemp .env.XXXXXX)
  ENV_KEY="$1" ENV_VALUE="$2" awk '
    BEGIN { k = ENVIRON["ENV_KEY"]; v = ENVIRON["ENV_VALUE"]; done = 0 }
    index($0, k "=") == 1 { print k "=" v; done = 1; next }
    { print }
    END { if (!done) print k "=" v }
  ' .env > "$tmp"
  chmod 600 "$tmp"
  mv "$tmp" .env
}

# --- .env ---
if [ -f .env ]; then
  info ".env exists, keeping it"
else
  [ -f .env.example ] || die ".env.example is missing"
  cp .env.example .env
  chmod 600 .env
  info "created .env"
  if [ -z "$BASE_URL" ]; then
    BASE_URL=$(ask "Public URL (no trailing slash)" "https://links.example.com")
  fi
  valid_base_url "$BASE_URL" || die "base URL must be an absolute http(s) URL without a trailing slash: $BASE_URL"
  set_env LP_BASE_URL "$BASE_URL"

  if [ "$PROFILES_SET" -eq 0 ]; then
    PROFILES=""
    if yes_no "Enable the Cloudflare Tunnel (cloudflared)?" y; then PROFILES="tunnel"; fi
    if yes_no "Enable scheduled backups?" n; then PROFILES="${PROFILES:+$PROFILES,}backup"; fi
  fi
  case ",$PROFILES," in
    *[!a-z,]*) die "--profiles accepts a comma-separated list of: tunnel, backup" ;;
  esac
  set_env COMPOSE_PROFILES "$PROFILES"
fi

# --- secrets/ ---
mkdir -p secrets backups
chmod 700 secrets
if [ -f secrets/pg_password ]; then
  info "secrets/pg_password exists, keeping it"
else
  openssl rand -hex 24 > secrets/pg_password
  info "created secrets/pg_password"
fi
# Compose bind-mounts file secrets with their host mode; the app (UID 65532)
# and postgres (UID 70) must read it. secrets/ itself stays 0700.
chmod 644 secrets/pg_password
case ",$(sed -n 's/^COMPOSE_PROFILES=//p' .env)," in
  *,tunnel,*)
    if [ ! -s secrets/tunnel_token ]; then
      : > secrets/tunnel_token
      chmod 644 secrets/tunnel_token
      warn "paste your Cloudflare Tunnel token into secrets/tunnel_token before 'docker compose up -d'"
    fi
    ;;
esac

# --- config/ ---
mkdir -p config
if [ -f config/config.yaml ]; then
  info "config/config.yaml exists, keeping it"
else
  [ -f config/config.example.yaml ] || die "config/config.example.yaml is missing"
  cp config/config.example.yaml config/config.yaml
  info "created config/config.yaml"
fi

# The app runs as UID/GID 65532: give it group read access only.
restrict_config() {
  chgrp "$APP_GID" config config/config.yaml &&
    chmod 750 config && chmod 640 config/config.yaml &&
    for f in secrets/admin_*; do
      [ -e "$f" ] || continue
      chgrp "$APP_GID" "$f" && chmod 640 "$f" || return 1
    done
}
if (restrict_config) 2>/dev/null; then
  info "config/ is readable by group $APP_GID only"
elif [ "$NON_INTERACTIVE" -eq 0 ] && [ -t 0 ] && command -v sudo >/dev/null 2>&1; then
  info "setting group $APP_GID on config/ (needs sudo)"
  sudo sh -c "$(command -v chgrp) $APP_GID config config/config.yaml && chmod 750 config && chmod 640 config/config.yaml" ||
    warn "could not restrict config/; run: sudo chgrp $APP_GID config config/config.yaml && sudo chmod 750 config && sudo chmod 640 config/config.yaml"
else
  chmod 755 config
  chmod 644 config/config.yaml
  warn "could not chgrp config/ to $APP_GID; it stays world-readable. Before adding password hashes run:"
  warn "  sudo chgrp $APP_GID config config/config.yaml secrets/admin_* && sudo chmod 750 config && sudo chmod 640 config/config.yaml secrets/admin_*"
fi

cat <<MSG

Next steps:
  1. Review .env (LP_BASE_URL, COMPOSE_PROFILES).
  2. Tunnel: put the token into secrets/tunnel_token.
  3. Add administrators to config/config.yaml, then:
       docker compose run --rm --no-deps app config check
  4. docker compose up -d
MSG

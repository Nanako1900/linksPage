# LinksPage

English | [简体中文](README.zh-CN.md)

A self-hosted share page that gathers your Discord, KOOK, QQ and WeChat communities behind **one link**, showing live names, online and member counts, channels and online members, with a join button for each.

> Status: **M1 public page MVP**. The public page works end to end with live Discord and KOOK data; its content comes from a `seed.yaml` file because the admin console (M2a) does not exist yet. See [docs/m1/notes.md](docs/m1/notes.md) (Chinese) for what M1 delivers and its known limitations, and [docs/tech-selection.md](docs/tech-selection.md) (Chinese) for the design.

## Features

| Feature | Status |
|---|---|
| Discord cards: widget.json + permanent invite (name, icon, online/member counts, channels, online members), click-to-load official iframe | ✅ M1 |
| KOOK cards without a bot token (badge parsing: name, online/total) | ✅ M1 |
| QQ group (copy number, QR) and WeChat group (inline QR, fallback contact) cards; static cards for 12 platform presets | ✅ M1 |
| `/go/{slug}` join links; "open in browser" guidance inside WeChat/QQ; "open on desktop" on phones | ✅ M1 |
| Server-rendered fallback markup, hash-based CSP, OG image and favicon generation, `/c/{slug}` share pages, zh-CN and en | ✅ M1 |
| Optional Cloudflare Worker front end (topology C, [deploy/cloudflare-worker](deploy/cloudflare-worker/README.zh-CN.md)) | ✅ M1 |
| Content from `seed.yaml` ([example](config/seed.example.yaml)) | ✅ M1 (until the admin console) |
| Admin console; administrators defined in the config file (local accounts, then OAuth2/OIDC allow-list) | M2a / M2b |
| Theme editor, more locales | M2b |
| Cookie-less analytics that never store raw IPs (`/go` clicks are not counted yet) | M3 |
| KOOK bot token tier, presence charts | M4 |

Stack: Vite + React + Tailwind CSS, Go, PostgreSQL; Docker Compose behind Cloudflare.

## Quick start (Docker Compose)

Requires Docker Engine 24+, Compose 2.20.1+ and a 64-bit host.

> Until v0.1.0 (M2a) is released there is no image on GHCR yet. Build it locally first with `make docker TAG=dev`, then set `LINKSPAGE_IMAGE=ghcr.io/nanako1900/linkspage` and `LINKSPAGE_TAG=dev` in `.env`.

From the repository root (in a release deploy bundle the script sits at the top level, so use `./init.sh` and `compose.direct.yaml` there):

```sh
sh deploy/init.sh         # creates .env, secrets/pg_password, config/config.yaml
# set LP_BASE_URL in .env; for the tunnel put the token into secrets/tunnel_token
# optional: cp config/seed.example.yaml config/seed.yaml, edit it and set
#   seed_file: /etc/linkspage/seed.yaml in config/config.yaml (imported once, on first start)
docker compose run --rm --no-deps app config check
docker compose up -d
```

Without the tunnel, behind your own reverse proxy, add `deploy/compose.direct.yaml` (binds `127.0.0.1:8080` by default):

```sh
docker compose -f compose.yaml -f deploy/compose.direct.yaml up -d
```

## Development

```sh
make web-install          # frontend dependencies (pnpm via corepack)
make dev                  # dev database, Go server and Vite on http://localhost:5173
make lint test            # Go and web lint and tests
make gen                  # regenerate sqlc, OpenAPI and the orval client
make docker               # build the image
```

End-to-end tests (Playwright + axe) run against an offline Docker Compose stack with a stub Discord/KOOK upstream:

```sh
docker build -t linkspage:e2e . && e2e/run.sh
```

## Disclaimer

LinksPage is not affiliated with Discord, KOOK or Tencent. Brand icons belong to their respective owners.

## License

[MIT](LICENSE)

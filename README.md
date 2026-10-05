# LinksPage

English | [简体中文](README.zh-CN.md)

A self-hosted share page that gathers your Discord, KOOK, QQ and WeChat communities behind **one link**, showing live names, online and member counts, channels and online members, with a join button for each.

> Status: **M0 scaffold**. The server starts, migrates the database and renders the public page skeleton; community cards, the admin console and analytics are not implemented yet. See [docs/tech-selection.md](docs/tech-selection.md) (Chinese) for the design.

## Planned features

- Visitors never log in; administrators are defined in the config file (OAuth2/OIDC allow-list or local accounts)
- Discord (widget.json, invites, official iframe), KOOK (token-free badge parsing), QQ group and WeChat group QR cards
- "Open in browser" guidance inside WeChat/QQ in-app browsers
- Cookie-less analytics that never store raw IPs
- Everything visitor-facing is customizable from the admin console
- Stack: Vite + React + Tailwind CSS, Go, PostgreSQL; Docker Compose behind Cloudflare

## Quick start (Docker Compose)

Requires Docker Engine 24+, Compose 2.20.1+ and a 64-bit host.

> Until v0.1.0 (M2a) is released there is no image on GHCR yet. Build it locally first with `make docker TAG=dev`, then set `LINKSPAGE_IMAGE=ghcr.io/nanako1900/linkspage` and `LINKSPAGE_TAG=dev` in `.env`.

From the repository root (in a release deploy bundle the script sits at the top level, so use `./init.sh` and `compose.direct.yaml` there):

```sh
sh deploy/init.sh         # creates .env, secrets/pg_password, config/config.yaml
# set LP_BASE_URL in .env; for the tunnel put the token into secrets/tunnel_token
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

## Disclaimer

LinksPage is not affiliated with Discord, KOOK or Tencent. Brand icons belong to their respective owners.

## License

[MIT](LICENSE)

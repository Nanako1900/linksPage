# Contributing

Thanks for helping! Docs are bilingual (English and 简体中文); either language is fine for issues and PRs.

## Workflow

1. Open an issue first for features or provider requests.
2. Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `perf:`, `ci:`).
3. Keep PRs focused and include a test plan (and screenshots for UI changes).

## Local setup

- Go 1.26+, Node 22.18+ (24 recommended) with corepack, Docker.
- `make web-install && make dev`
- Before pushing: `make lint test` and, when the API or queries changed, `make gen`.

## Rules of thumb

- Coverage target is 80% for `internal/**` and for `web/src/{shared,public}`.
- No `style=` attributes in Go templates; no `dangerouslySetInnerHTML`.
- Public pages must not import admin-only libraries (Biome enforces this).
- Never commit secrets; configuration secrets go through `*_FILE` variables.

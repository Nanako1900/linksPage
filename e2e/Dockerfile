# Playwright runner for the E2E stack (doc 13.2: Playwright image plus
# fonts-noto-cjk, so Chinese text is laid out with real CJK metrics).
# The image tag must match @playwright/test in package.json.
FROM mcr.microsoft.com/playwright:v1.63.0-noble@sha256:eff16c30e6f3f4af0a03fa4b706120d5e9b0891c344a27d64559aff5900a4a27

RUN apt-get update \
 && apt-get install -y --no-install-recommends fonts-noto-cjk \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /e2e
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY playwright.config.ts tsconfig.json ./
COPY tests ./tests

CMD ["pnpm", "test"]

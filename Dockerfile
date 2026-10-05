# syntax=docker/dockerfile:1

# --- Frontend: Vite build (web/dist with .vite/manifest.json) ---
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/ ./
# Tailwind also scans the Go templates (@source in src/styles/public.css).
COPY internal/webui/templates /src/internal/webui/templates
RUN pnpm build

# --- Backend: static Go binary with the frontend embedded ---
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/webui/dist
ARG TARGETOS TARGETARCH VERSION=dev COMMIT=none
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags nodynamic \
    -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT" -o /out/linkspage ./cmd/linkspage \
 && mkdir -p /out/data/uploads

# --- Runtime: distroless, UID 65532 ---
FROM gcr.io/distroless/static-debian13:nonroot
LABEL org.opencontainers.image.source="https://github.com/Nanako1900/linksPage" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="LinksPage"
COPY --from=api /out/linkspage /linkspage
COPY --from=api --chown=65532:65532 /out/data /data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/linkspage", "healthcheck"]
ENTRYPOINT ["/linkspage"]
CMD ["serve"]

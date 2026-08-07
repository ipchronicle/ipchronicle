# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:24.19.0-bookworm-slim AS web-build
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN --mount=type=cache,target=/root/.npm npm --prefix web ci
COPY openapi ./openapi
COPY web ./web
RUN npm --prefix web run generate:api && npm --prefix web run build

FROM --platform=$TARGETPLATFORM golang:1.26.5-bookworm AS go-build
ARG VERSION=dev
ARG REVISION=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web-build /src/web/dist ./internal/webui/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath \
      -ldflags "-s -w -X github.com/ipchronicle/ipchronicle/internal/version.Value=${VERSION}" \
      -o /out/ipchronicle-center ./cmd/ipchronicle-center

FROM debian:bookworm-slim
ARG VERSION=dev
ARG REVISION=unknown
ARG SOURCE_URL=https://github.com/ipchronicle/ipchronicle
LABEL org.opencontainers.image.title="IPChronicle Center" \
      org.opencontainers.image.description="Self-hosted IPChronicle center" \
      org.opencontainers.image.source=$SOURCE_URL \
      org.opencontainers.image.url=$SOURCE_URL \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$REVISION \
      org.opencontainers.image.licenses="AGPL-3.0-only"

RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 ipchronicle \
    && useradd --system --uid 10001 --gid ipchronicle --home-dir /var/lib/ipchronicle ipchronicle \
    && install -d -o ipchronicle -g ipchronicle /var/lib/ipchronicle /licenses

COPY --from=go-build /out/ipchronicle-center /usr/local/bin/ipchronicle-center
COPY LICENSE THIRD_PARTY_NOTICES.md /licenses/

USER ipchronicle
WORKDIR /var/lib/ipchronicle
VOLUME ["/var/lib/ipchronicle"]
EXPOSE 8080
ENV IPCHRONICLE_LISTEN_ADDRESS=:8080
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=6 \
  CMD ["/usr/local/bin/ipchronicle-center", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/ipchronicle-center"]

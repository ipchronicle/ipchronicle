# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

ARG SOURCE_DATE_EPOCH=0

FROM --platform=$BUILDPLATFORM node:24.19.0-bookworm-slim@sha256:3638d9a6fe4030bd716be989438248074489337ba3275657f93595428be4fc03 AS web-build
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN --mount=type=cache,target=/root/.npm npm --prefix web ci
COPY openapi ./openapi
COPY web ./web
RUN npm --prefix web run generate:api && npm --prefix web run build

FROM --platform=$TARGETPLATFORM golang:1.26.5-bookworm@sha256:6c5605ab3a9a9fb3c4eafe5b3d63cdbf3881caf113262b67862547b54a9db599 AS go-build
ARG VERSION=dev
ARG REVISION=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web-build /src/web/dist ./internal/webui/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid= -extldflags=-Wl,--build-id=none -X github.com/ipchronicle/ipchronicle/internal/version.Value=${VERSION} -X github.com/ipchronicle/ipchronicle/internal/version.Revision=${REVISION}" \
      -o /out/ipchronicle-center ./cmd/ipchronicle-center

FROM debian:bookworm-slim@sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241
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

COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go-build /usr/share/zoneinfo /usr/share/zoneinfo

RUN groupadd --system --gid 10001 ipchronicle \
    && useradd --system --uid 10001 --gid ipchronicle --home-dir /var/lib/ipchronicle ipchronicle \
    && install -d -o ipchronicle -g ipchronicle \
      /var/lib/ipchronicle /var/lib/ipchronicle/config \
      /var/lib/ipchronicle/history /licenses

COPY --from=go-build /out/ipchronicle-center /usr/local/bin/ipchronicle-center
COPY LICENSE THIRD_PARTY_NOTICES.md /licenses/

USER ipchronicle
WORKDIR /var/lib/ipchronicle
VOLUME ["/var/lib/ipchronicle/config", "/var/lib/ipchronicle/history"]
EXPOSE 8080
ENV IPCHRONICLE_LISTEN_ADDRESS=:8080
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=6 \
  CMD ["/usr/local/bin/ipchronicle-center", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/ipchronicle-center"]

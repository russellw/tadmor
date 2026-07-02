# Container image for the tadmor server, suitable for Cloud Run (or any
# container host). The server embeds the built SPA and serves the API, so this
# produces a single self-contained image.
#
# The Go build stays hermetic (vendored deps, pinned toolchain, no network):
# GOPROXY=off / -mod=vendor / GOTOOLCHAIN=local mirror the Makefile. The pnpm
# stage installs from the frozen lockfile (the go.sum analog).

# ---- Stage 1: build the SPA (pnpm, pinned via corepack) ----
FROM node:22-bookworm-slim AS web
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@10.18.0 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---- Stage 2: hermetic, vendored Go build (embeds web/dist) ----
FROM golang:1.25.11-bookworm AS build
WORKDIR /src
ENV GOFLAGS=-mod=vendor GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY cmd/ cmd/
COPY internal/ internal/
COPY db/ db/
COPY web/ web/
COPY --from=web /src/web/dist ./web/dist
RUN go build -o /out/server ./cmd/server

# ---- Stage 3: minimal runtime ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /src/db/migrations /app/db/migrations
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/server"]

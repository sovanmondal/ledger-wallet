# syntax=docker/dockerfile:1

# ---- build stage: compiles the static Go binary (host needs no Go toolchain) ----
FROM golang:1.22-bookworm AS build
WORKDIR /src

# Resilient module resolution for the non-vendored path. Pipe (|) separators fall through to the
# next source on ANY error (comma only falls through on 404/410). goproxy.cn is reliable and serves
# all modules incl. golang.org/x; then goproxy.io; then direct (git). proxy.golang.org is omitted
# because it is commonly blocked and would stall the chain on timeouts.
ENV GOPROXY="https://goproxy.cn|https://goproxy.io|direct" \
    GOSUMDB=off

COPY . .

# Preferred path: if deps are vendored (committed vendor/), build is FULLY OFFLINE — no proxy,
# no downloads, hermetic and reproducible anywhere (Render, corporate networks, air-gapped CI).
# Fallback path: build using the committed go.mod + go.sum (needs a reachable GOPROXY).
RUN if [ -d vendor ]; then \
      echo ">> building from vendor/ (offline, no module downloads)"; \
      CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/ledger-wallet ./cmd/server; \
    else \
      echo ">> resolving modules via GOPROXY (go.sum pinned)"; \
      CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ledger-wallet ./cmd/server; \
    fi

# ---- runtime stage: distroless static, non-root, with a self healthcheck ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/ledger-wallet /ledger-wallet

# Runs as the built-in "nonroot" user (uid 65532), never root.
USER nonroot:nonroot
EXPOSE 8080

# distroless has no shell; use the binary's own healthcheck subcommand.
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/ledger-wallet", "healthcheck"]

ENTRYPOINT ["/ledger-wallet"]

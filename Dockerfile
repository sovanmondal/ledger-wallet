# syntax=docker/dockerfile:1

# ---- build stage: compiles the static Go binary (host needs no Go toolchain) ----
FROM golang:1.22-bookworm AS build
WORKDIR /src

# Resilient module resolution: mirrors first (a mirror serves ALL modules incl. golang.org/x),
# then the default proxy, then direct (git). GOSUMDB=off avoids sum.golang.org (also often blocked).
ENV GOPROXY=https://goproxy.io,https://goproxy.cn,https://proxy.golang.org,direct \
    GOSUMDB=off \
    GOFLAGS=-mod=mod

COPY . .

# If dependencies are vendored (run `make vendor` once on an unblocked network and commit vendor/),
# the build is FULLY OFFLINE and needs no proxy at all. Otherwise resolve via GOPROXY/direct.
RUN if [ -d vendor ]; then \
      echo ">> building from vendor/ (offline, no module downloads)"; \
      CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/ledger-wallet ./cmd/server; \
    else \
      echo ">> resolving modules via GOPROXY (needs internet)"; \
      go mod tidy && \
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

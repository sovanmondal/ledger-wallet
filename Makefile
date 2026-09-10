.PHONY: up down build logs burst tidy psql clean

# Bring up app + Postgres with one command (builds the Go binary in-container).
up:
	docker compose up --build

# Same but detached.
up-d:
	docker compose up --build -d

down:
	docker compose down -v

build:
	docker compose build

logs:
	docker compose logs -f app

# Run the correctness burst against a running instance (default: localhost:8080).
burst:
	BASE_URL?=http://localhost:8080 ./scripts/burst.sh

# Regenerate go.sum / tidy deps inside a Go container (host has no Go toolchain).
tidy:
	docker run --rm -v "$(PWD)":/src -w /src \
	  -e GOSUMDB=off \
	  -e GOPROXY=https://goproxy.io,https://goproxy.cn,https://proxy.golang.org,direct \
	  golang:1.22-bookworm go mod tidy

# ONE-TIME on any open network (e.g. phone hotspot): download deps into vendor/ so that
# every later `docker compose up --build` compiles FULLY OFFLINE (bypasses blocked proxy.golang.org).
vendor:
	docker run --rm -v "$(PWD)":/src -w /src \
	  -e GOFLAGS=-mod=mod -e GOSUMDB=off \
	  -e GOPROXY=https://proxy.golang.org,https://goproxy.io,https://goproxy.cn,direct \
	  golang:1.22-bookworm sh -c 'go mod tidy && go mod vendor && echo "vendored OK — commit the vendor/ dir"'

# Diagnose which Go module source is reachable from inside a build container on your network.
netcheck:
	docker run --rm golang:1.22-bookworm sh -c '\
	  for u in proxy.golang.org goproxy.io goproxy.cn github.com go.googlesource.com; do \
	    printf "%-22s " "$$u:"; \
	    (timeout 8 wget -q -O /dev/null https://$$u/ && echo REACHABLE) || echo BLOCKED; \
	  done'

# Open a psql shell into the compose Postgres.
psql:
	docker compose exec db psql -U app -d wallet

clean:
	docker compose down -v --remove-orphans

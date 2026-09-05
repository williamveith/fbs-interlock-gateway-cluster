APP := fbs-interlock-gateway-cluster
CMD := ./cmd/$(APP)

BUILD_DIR := build
BINARY := $(BUILD_DIR)/$(APP)

CONFIGS := config.yaml

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

.PHONY: \
	run \
	fmt \
	fmt-check \
	tidy-check \
	vet \
	staticcheck \
	test \
	test-race \
	scripts-check \
	shellcheck \
	build \
	build-check \
	verify \
	init-config \
	shelly-auth \
	ca \
	gateway-cert \
	shelly-cert \
	clean

# =========================
# DEVELOPMENT
# =========================

run:
	go run $(CMD) -config $(CONFIGS)

fmt:
	go fmt ./...

test:
	go test -count=1 ./...

# =========================
# CONFIGURATION
# =========================

init-config:
	@if [ -f "$(CONFIGS)" ]; then \
		echo "$(CONFIGS) already exists; not overwriting."; \
	else \
		echo "Creating $(CONFIGS)"; \
		printf '%s\n' \
			'bind: 0.0.0.0' \
			'' \
			'defaults:' \
			'  timeout_ms: 5000' \
			'  safe_state_on_error: "off"' \
			'  shelly_tls:' \
			'    server_ca_file: "./tls/server-ca.crt"' \
			'    client_cert_file: "./tls/gateway-client.crt"' \
			'    client_key_file: "./tls/gateway-client.key"' \
			'' \
			'tools:' \
			'  - interlock_name:' \
			'    ip:' \
			'    port:' \
			'    switch_id:' \
			'    username:' \
			'    password:' \
			'    enabled:' \
			> "$(CONFIGS)"; \
	fi

# =========================
# BUILD
# =========================

build:
	mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BINARY)" \
		$(CMD)

build-check:
	mkdir -p "$(BUILD_DIR)/ci"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)" \
		$(CMD)

# =========================
# VALIDATION
# =========================

fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "The following Go files are not formatted:"; \
		echo "$$files"; \
		exit 1; \
	fi

tidy-check:
	@set -eu; \
	tmp_dir="$$(mktemp -d)"; \
	cp go.mod go.sum "$$tmp_dir/"; \
	trap 'cp "$$tmp_dir/go.mod" go.mod; cp "$$tmp_dir/go.sum" go.sum; rm -rf "$$tmp_dir"' EXIT; \
	go mod tidy; \
	diff -u "$$tmp_dir/go.mod" go.mod; \
	diff -u "$$tmp_dir/go.sum" go.sum

vet:
	go vet ./...

staticcheck:
	go tool staticcheck ./...

test-race:
	go test -race -count=1 ./...

scripts-check:
	@find scripts \
		-type f \( -name '*.sh' -o -name '*.sh.in' \) \
		-exec bash -n {} +

shellcheck:
	find scripts \
		-type f \( -name '*.sh' -o -name '*.sh.in' \) \
		-exec shellcheck {} +

verify: \
	fmt-check \
	tidy-check \
	vet \
	staticcheck \
	test-race \
	scripts-check \
	shellcheck \
	build-check

# =========================
# UTILITIES
# =========================

shelly-auth:
	@chmod +x scripts/set-shelly-auth.sh
	@./scripts/set-shelly-auth.sh

# =========================
# TLS UTILITIES
# =========================

ca:
	@chmod +x scripts/tls/create-ca.sh
	@./scripts/tls/create-ca.sh

gateway-cert:
	@chmod +x scripts/tls/create-gateway-client.sh
	@./scripts/tls/create-gateway-client.sh

shelly-cert:
	@chmod +x scripts/tls/create-shelly-cert.sh
	@./scripts/tls/create-shelly-cert.sh

# =========================
# CLEANUP
# =========================

clean:
	rm -rf "$(BUILD_DIR)"
	go clean
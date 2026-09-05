SHELL := /bin/bash

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

# =========================
# CONTAINER
# =========================

CONTAINER_IMAGE ?= $(APP)
CONTAINER_TAG ?= dev
CONTAINER_PLATFORM ?= linux/amd64

# =========================
# SWARM
# =========================

ENV_FILE ?= .env

SWARM_STACK ?= fbs
SWARM_SERVICE ?= $(SWARM_STACK)_gateway
SWARM_STACK_FILE ?= cluster/swarm/stack.yml

SWARM_VOLUME ?= fbs-gateway-swarm-data

SWARM_CONTAINER_TAG ?= swarm-test

# Build for the architecture of the Docker Engine actually running the Swarm.
# Docker Desktop on Apple Silicon will resolve to linux/arm64.
SWARM_PLATFORM ?= $(shell \
	docker version \
		--format '{{.Server.Os}}/{{.Server.Arch}}' \
		2>/dev/null || echo linux/amd64)

SWARM_DATA_UID ?= 65532
SWARM_DATA_GID ?= 65532

R2_ACCESS_SECRET := fbs_r2_access_key_id
R2_SECRET_SECRET := fbs_r2_secret_access_key

SERVER_CA_SECRET := server-ca
GATEWAY_CLIENT_CERT_SECRET := gateway-client-cert
GATEWAY_CLIENT_KEY_SECRET := gateway-client-key

TLS_SERVER_CA_SOURCE := pki/ca/server-ca.crt
TLS_CLIENT_CERT_SOURCE := pki/gateway/gateway-client.crt
TLS_CLIENT_KEY_SOURCE := pki/gateway/gateway-client.key

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
	container-build \
	container-run \
	swarm-preflight \
	swarm-image \
	swarm-init \
	swarm-volume \
	swarm-volume-reset \
	swarm-r2-secrets \
	swarm-cert-secrets \
	swarm-secrets \
	swarm-secrets-remove \
	swarm-secrets-recreate \
	swarm-stop \
	swarm-deploy \
	swarm-local-deploy \
	swarm-reset-deploy \
	swarm-status \
	swarm-logs \
	swarm-local-purge \
	swarm-full-test \
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
# CONTAINER
# =========================

container-build:
	docker build \
		--platform "$(CONTAINER_PLATFORM)" \
		-f Containerfile \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg DATE="$(DATE)" \
		-t "$(CONTAINER_IMAGE):$(CONTAINER_TAG)" \
		.

container-run:
	docker run --rm \
		--platform "$(CONTAINER_PLATFORM)" \
		--network host \
		-e R2_ACCOUNT_ID \
		-e R2_ACCESS_KEY_ID \
		-e R2_SECRET_ACCESS_KEY \
		-v fbs-gateway-data:/data \
		"$(CONTAINER_IMAGE):$(CONTAINER_TAG)"

# =========================
# SWARM PREFLIGHT
# =========================

swarm-preflight:
	@command -v docker >/dev/null 2>&1 || { \
		echo "ERROR: docker is not installed."; \
		exit 1; \
	}
	@docker info >/dev/null 2>&1 || { \
		echo "ERROR: Docker Engine is not running."; \
		exit 1; \
	}
	@test -f "$(ENV_FILE)" || { \
		echo "ERROR: Missing $(ENV_FILE)."; \
		exit 1; \
	}
	@set -a; \
	source "$(ENV_FILE)"; \
	set +a; \
	: "$${R2_ACCOUNT_ID:?R2_ACCOUNT_ID is missing from $(ENV_FILE)}"; \
	: "$${R2_ACCESS_KEY_ID:?R2_ACCESS_KEY_ID is missing from $(ENV_FILE)}"; \
	: "$${R2_SECRET_ACCESS_KEY:?R2_SECRET_ACCESS_KEY is missing from $(ENV_FILE)}"
	@test -f "$(SWARM_STACK_FILE)" || { \
		echo "ERROR: Missing $(SWARM_STACK_FILE)."; \
		exit 1; \
	}
	@test -f "$(TLS_SERVER_CA_SOURCE)" || { \
		echo "ERROR: Missing $(TLS_SERVER_CA_SOURCE)."; \
		exit 1; \
	}
	@test -f "$(TLS_CLIENT_CERT_SOURCE)" || { \
		echo "ERROR: Missing $(TLS_CLIENT_CERT_SOURCE)."; \
		exit 1; \
	}
	@test -f "$(TLS_CLIENT_KEY_SOURCE)" || { \
		echo "ERROR: Missing $(TLS_CLIENT_KEY_SOURCE)."; \
		exit 1; \
	}

# =========================
# SWARM IMAGE
# =========================

swarm-image:
	@echo "Building Swarm image for $(SWARM_PLATFORM)"
	@$(MAKE) container-build \
		CONTAINER_PLATFORM="$(SWARM_PLATFORM)" \
		CONTAINER_TAG="$(SWARM_CONTAINER_TAG)"

# =========================
# SWARM INITIALIZATION
# =========================

swarm-init:
	@state="$$(docker info --format '{{.Swarm.LocalNodeState}}')"; \
	case "$$state" in \
		active) \
			echo "Docker Swarm is already active."; \
			;; \
		inactive) \
			echo "Initializing Docker Swarm..."; \
			docker swarm init; \
			;; \
		*) \
			echo "ERROR: Unexpected Swarm state: $$state"; \
			exit 1; \
			;; \
	esac

# =========================
# SWARM DATA VOLUME
# =========================

swarm-volume:
	@if docker volume inspect "$(SWARM_VOLUME)" >/dev/null 2>&1; then \
		echo "Swarm data volume already exists: $(SWARM_VOLUME)"; \
	else \
		echo "Creating Swarm data volume: $(SWARM_VOLUME)"; \
		docker volume create "$(SWARM_VOLUME)" >/dev/null; \
		docker run --rm \
			--platform "$(SWARM_PLATFORM)" \
			-v "$(SWARM_VOLUME):/data" \
			alpine:3.22 \
			sh -c 'chown $(SWARM_DATA_UID):$(SWARM_DATA_GID) /data && chmod 0750 /data'; \
	fi

swarm-volume-reset:
	@if docker service inspect "$(SWARM_SERVICE)" >/dev/null 2>&1; then \
		echo "ERROR: $(SWARM_SERVICE) is still running."; \
		echo "Run 'make swarm-stop' before resetting the volume."; \
		exit 1; \
	fi
	@echo "Removing Swarm data volume if present..."
	@docker volume rm "$(SWARM_VOLUME)" >/dev/null 2>&1 || true
	@echo "Creating empty Swarm data volume..."
	@docker volume create "$(SWARM_VOLUME)" >/dev/null
	@docker run --rm \
		--platform "$(SWARM_PLATFORM)" \
		-v "$(SWARM_VOLUME):/data" \
		alpine:3.22 \
		sh -c 'chown $(SWARM_DATA_UID):$(SWARM_DATA_GID) /data && chmod 0750 /data'
	@echo "Created empty volume: $(SWARM_VOLUME)"

# =========================
# SWARM R2 SECRETS
# =========================

swarm-r2-secrets:
	@set -a; \
	source "$(ENV_FILE)"; \
	set +a; \
	: "$${R2_ACCESS_KEY_ID:?R2_ACCESS_KEY_ID is missing}"; \
	: "$${R2_SECRET_ACCESS_KEY:?R2_SECRET_ACCESS_KEY is missing}"; \
	if docker secret inspect "$(R2_ACCESS_SECRET)" >/dev/null 2>&1; then \
		echo "Swarm secret already exists: $(R2_ACCESS_SECRET)"; \
	else \
		printf '%s' "$$R2_ACCESS_KEY_ID" | \
			docker secret create "$(R2_ACCESS_SECRET)" - >/dev/null; \
		echo "Created Swarm secret: $(R2_ACCESS_SECRET)"; \
	fi; \
	if docker secret inspect "$(R2_SECRET_SECRET)" >/dev/null 2>&1; then \
		echo "Swarm secret already exists: $(R2_SECRET_SECRET)"; \
	else \
		printf '%s' "$$R2_SECRET_ACCESS_KEY" | \
			docker secret create "$(R2_SECRET_SECRET)" - >/dev/null; \
		echo "Created Swarm secret: $(R2_SECRET_SECRET)"; \
	fi

# =========================
# SWARM TLS SECRETS
# =========================

swarm-cert-secrets:
	@if docker secret inspect "$(SERVER_CA_SECRET)" >/dev/null 2>&1; then \
		echo "Swarm secret already exists: $(SERVER_CA_SECRET)"; \
	else \
		docker secret create \
			"$(SERVER_CA_SECRET)" \
			"$(TLS_SERVER_CA_SOURCE)" >/dev/null; \
		echo "Created Swarm secret: $(SERVER_CA_SECRET)"; \
	fi
	@if docker secret inspect "$(GATEWAY_CLIENT_CERT_SECRET)" >/dev/null 2>&1; then \
		echo "Swarm secret already exists: $(GATEWAY_CLIENT_CERT_SECRET)"; \
	else \
		docker secret create \
			"$(GATEWAY_CLIENT_CERT_SECRET)" \
			"$(TLS_CLIENT_CERT_SOURCE)" >/dev/null; \
		echo "Created Swarm secret: $(GATEWAY_CLIENT_CERT_SECRET)"; \
	fi
	@if docker secret inspect "$(GATEWAY_CLIENT_KEY_SECRET)" >/dev/null 2>&1; then \
		echo "Swarm secret already exists: $(GATEWAY_CLIENT_KEY_SECRET)"; \
	else \
		docker secret create \
			"$(GATEWAY_CLIENT_KEY_SECRET)" \
			"$(TLS_CLIENT_KEY_SOURCE)" >/dev/null; \
		echo "Created Swarm secret: $(GATEWAY_CLIENT_KEY_SECRET)"; \
	fi

swarm-secrets:
	@$(MAKE) swarm-r2-secrets
	@$(MAKE) swarm-cert-secrets

swarm-secrets-remove:
	@if docker service inspect "$(SWARM_SERVICE)" >/dev/null 2>&1; then \
		echo "ERROR: Cannot remove secrets while $(SWARM_SERVICE) is using them."; \
		echo "Run 'make swarm-stop' first."; \
		exit 1; \
	fi
	@for secret in \
		"$(R2_ACCESS_SECRET)" \
		"$(R2_SECRET_SECRET)" \
		"$(SERVER_CA_SECRET)" \
		"$(GATEWAY_CLIENT_CERT_SECRET)" \
		"$(GATEWAY_CLIENT_KEY_SECRET)"; do \
		if docker secret inspect "$$secret" >/dev/null 2>&1; then \
			docker secret rm "$$secret" >/dev/null; \
			echo "Removed Swarm secret: $$secret"; \
		fi; \
	done

swarm-secrets-recreate:
	@$(MAKE) swarm-secrets-remove
	@$(MAKE) swarm-secrets

# =========================
# SWARM STACK
# =========================

swarm-stop:
	@if docker stack ls --format '{{.Name}}' | grep -qx "$(SWARM_STACK)"; then \
		echo "Removing Swarm stack: $(SWARM_STACK)"; \
		docker stack rm "$(SWARM_STACK)"; \
		echo "Waiting for $(SWARM_SERVICE) to stop..."; \
		while docker service inspect "$(SWARM_SERVICE)" >/dev/null 2>&1; do \
			sleep 1; \
		done; \
	else \
		echo "Swarm stack is not deployed: $(SWARM_STACK)"; \
	fi

swarm-deploy:
	@$(MAKE) swarm-preflight
	@$(MAKE) swarm-init
	@$(MAKE) swarm-volume
	@$(MAKE) swarm-secrets
	@echo "Deploying Swarm stack: $(SWARM_STACK)"
	@set -a; \
	source "$(ENV_FILE)"; \
	set +a; \
	export R2_ACCOUNT_ID; \
	docker stack deploy \
		--resolve-image never \
		-c "$(SWARM_STACK_FILE)" \
		"$(SWARM_STACK)"
	@echo
	@docker stack services "$(SWARM_STACK)"

# Normal local deployment.
# Existing database volume and existing Swarm secrets are preserved.
swarm-local-deploy:
	@$(MAKE) swarm-preflight
	@$(MAKE) swarm-image
	@$(MAKE) swarm-init
	@$(MAKE) swarm-volume
	@$(MAKE) swarm-secrets
	@$(MAKE) swarm-deploy

# Full clean/reprovisioning test.
# Removes the stack, destroys the local SQLite volume,
# recreates all Swarm secrets, and redeploys.
#
# Litestream must restore gateway.sqlite3 from R2.
swarm-reset-deploy:
	@$(MAKE) swarm-preflight
	@$(MAKE) swarm-image
	@$(MAKE) swarm-init
	@$(MAKE) swarm-stop
	@$(MAKE) swarm-volume-reset
	@$(MAKE) swarm-secrets-recreate
	@$(MAKE) swarm-deploy

swarm-status:
	@echo "=== Nodes ==="
	@docker node ls
	@echo
	@echo "=== Services ==="
	@docker stack services "$(SWARM_STACK)"
	@echo
	@echo "=== Gateway Tasks ==="
	@docker service ps "$(SWARM_SERVICE)"

swarm-logs:
	docker service logs -f "$(SWARM_SERVICE)"

# =========================
# LOCAL SWARM PURGE / TEST
# =========================

swarm-local-purge:
	@command -v docker >/dev/null 2>&1 || { \
		echo "ERROR: docker is not installed."; \
		exit 1; \
	}
	@docker info >/dev/null 2>&1 || { \
		echo "ERROR: Docker Engine is not running."; \
		exit 1; \
	}
	@echo "Purging local FBS Swarm deployment..."

	@state="$$(docker info --format '{{.Swarm.LocalNodeState}}')"; \
	if [ "$$state" = "active" ]; then \
		if docker stack ls --format '{{.Name}}' | grep -qx "$(SWARM_STACK)"; then \
			echo "Removing stack: $(SWARM_STACK)"; \
			docker stack rm "$(SWARM_STACK)"; \
		fi; \
		echo "Waiting for Swarm service removal..."; \
		while docker service inspect "$(SWARM_SERVICE)" >/dev/null 2>&1; do \
			sleep 1; \
		done; \
	fi

	@containers="$$(docker ps -aq --filter volume="$(SWARM_VOLUME)")"; \
	if [ -n "$$containers" ]; then \
		echo "Removing containers still referencing $(SWARM_VOLUME)..."; \
		docker rm -f $$containers >/dev/null; \
	fi

	@for secret in \
		"$(R2_ACCESS_SECRET)" \
		"$(R2_SECRET_SECRET)" \
		"$(SERVER_CA_SECRET)" \
		"$(GATEWAY_CLIENT_CERT_SECRET)" \
		"$(GATEWAY_CLIENT_KEY_SECRET)"; do \
		if docker secret inspect "$$secret" >/dev/null 2>&1; then \
			echo "Removing secret: $$secret"; \
			docker secret rm "$$secret" >/dev/null; \
		fi; \
	done

	@if docker volume inspect "$(SWARM_VOLUME)" >/dev/null 2>&1; then \
		echo "Removing Swarm data volume: $(SWARM_VOLUME)"; \
		docker volume rm "$(SWARM_VOLUME)"; \
	else \
		echo "Swarm data volume already absent: $(SWARM_VOLUME)"; \
	fi

	@if docker image inspect \
		"$(CONTAINER_IMAGE):$(SWARM_CONTAINER_TAG)" >/dev/null 2>&1; then \
		echo "Removing local Swarm image..."; \
		docker image rm \
			"$(CONTAINER_IMAGE):$(SWARM_CONTAINER_TAG)" >/dev/null; \
	fi

	@state="$$(docker info --format '{{.Swarm.LocalNodeState}}')"; \
	if [ "$$state" = "active" ]; then \
		echo "Leaving local Docker Swarm..."; \
		docker swarm leave --force >/dev/null; \
	fi

	@echo
	@echo "Local FBS Swarm environment purged."

swarm-full-test:
	@$(MAKE) swarm-local-purge
	@$(MAKE) swarm-local-deploy
	@echo
	@echo "Full Swarm rebuild completed."
	@$(MAKE) swarm-status

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
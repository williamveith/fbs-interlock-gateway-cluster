SHELL := /bin/bash

APP := fbs-interlock-gateway-cluster
ENTRYPOINT := swarm-entrypoint
ENTRYPOINT_CMD := ./cmd/swarm-entrypoint

BUILD_DIR := build
ENTRYPOINT_AMD64 := $(BUILD_DIR)/$(ENTRYPOINT)-linux-amd64
ENTRYPOINT_ARM64 := $(BUILD_DIR)/$(ENTRYPOINT)-linux-arm64

GATEWAY_VERSION ?= v4.0.1

CLUSTER_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# =========================
# CONTAINER
# =========================

DOCKER_PLATFORM ?= $(shell \
	docker version \
		--format '{{.Server.Os}}/{{.Server.Arch}}' \
		2>/dev/null || echo linux/amd64)

CONTAINER_IMAGE ?= $(APP)
CONTAINER_TAG ?= dev
CONTAINER_PLATFORM ?= $(DOCKER_PLATFORM)

DOCKERHUB_NAMESPACE ?= williamveith
DOCKERHUB_IMAGE ?= docker.io/$(DOCKERHUB_NAMESPACE)/$(APP)
DOCKERHUB_TAG ?= 1.0.0
DOCKERHUB_PLATFORMS ?= linux/amd64,linux/arm64
BUILDX_BUILDER ?= fbs-multiplatform

# =========================
# SWARM
# =========================

ENV_FILE ?= .env

SWARM_STACK ?= fbs
SWARM_SERVICE ?= $(SWARM_STACK)_gateway
SWARM_STACK_FILE ?= cluster/swarm/stack.yml

SWARM_VOLUME ?= fbs-gateway-swarm-data

SWARM_IMAGE ?= $(DOCKERHUB_IMAGE):$(DOCKERHUB_TAG)
SWARM_PLATFORM ?= $(DOCKER_PLATFORM)

SWARM_DATA_UID ?= 65532
SWARM_DATA_GID ?= 65532

R2_ACCESS_SECRET := fbs_r2_access_key_id
R2_SECRET_SECRET := fbs_r2_secret_access_key

SERVER_CA_SECRET := server-ca
GATEWAY_CLIENT_CERT_SECRET := gateway-client-cert
GATEWAY_CLIENT_KEY_SECRET := gateway-client-key

TLS_SERVER_CA_SOURCE ?= pki/ca/server-ca.crt
TLS_CLIENT_CERT_SOURCE ?= pki/gateway/gateway-client.crt
TLS_CLIENT_KEY_SOURCE ?= pki/gateway/gateway-client.key

.PHONY: \
	fmt \
	fmt-check \
	tidy \
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
	container-build \
	container-run \
	container-smoke \
	container-builder \
	container-login \
	container-publish \
	container-inspect \
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
	swarm-wait \
	swarm-local-deploy \
	swarm-reset-deploy \
	swarm-status \
	swarm-logs \
	swarm-local-purge \
	swarm-full-test \
	clean

# =========================
# DEVELOPMENT
# =========================

fmt:
	go fmt ./...

tidy:
	go mod tidy

test:
	go test -count=1 ./...

# =========================
# BUILD
# =========================

build:
	mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="-s -w" \
		-o "$(ENTRYPOINT_AMD64)" \
		$(ENTRYPOINT_CMD)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="-s -w" \
		-o "$(ENTRYPOINT_ARM64)" \
		$(ENTRYPOINT_CMD)

build-check: build
	@file "$(ENTRYPOINT_AMD64)"
	@file "$(ENTRYPOINT_ARM64)"

# =========================
# CONTAINER
# =========================

container-build:
	docker build \
		--platform "$(CONTAINER_PLATFORM)" \
		-f Dockerfile \
		--build-arg GATEWAY_VERSION="$(GATEWAY_VERSION)" \
		--build-arg VERSION="$(CLUSTER_VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg DATE="$(DATE)" \
		-t "$(CONTAINER_IMAGE):$(CONTAINER_TAG)" \
		.

container-smoke: container-build
	@echo "Testing upstream gateway..."
	@output="$$(docker run --rm \
		--platform "$(CONTAINER_PLATFORM)" \
		--entrypoint /fbs-interlock-gateway \
		"$(CONTAINER_IMAGE):$(CONTAINER_TAG)" \
		-version)"; \
	echo "$$output"; \
	echo "$$output" | grep -F "version=$(GATEWAY_VERSION)" >/dev/null || { \
		echo "ERROR: image does not contain gateway $(GATEWAY_VERSION)."; \
		exit 1; \
	}; \
	echo "Verified gateway $(GATEWAY_VERSION)."

	@echo "Testing Swarm entrypoint..."
	@output="$$(docker run --rm \
		--platform "$(CONTAINER_PLATFORM)" \
		-e R2_ACCESS_KEY_ID=test \
		-e R2_SECRET_ACCESS_KEY=test \
		"$(CONTAINER_IMAGE):$(CONTAINER_TAG)" \
		version)"; \
	echo "$$output"; \
	echo "$$output" | grep -F "v0.5.17" >/dev/null || { \
		echo "ERROR: swarm-entrypoint failed to launch Litestream."; \
		exit 1; \
	}; \
	echo "Verified swarm-entrypoint and Litestream."

	@echo "Container smoke tests passed."

container-run:
	docker run --rm \
		--platform "$(CONTAINER_PLATFORM)" \
		--network host \
		-e R2_ACCOUNT_ID \
		-e R2_ACCESS_KEY_ID \
		-e R2_SECRET_ACCESS_KEY \
		-v fbs-gateway-data:/data \
		"$(CONTAINER_IMAGE):$(CONTAINER_TAG)"

container-builder:
	@command -v docker >/dev/null 2>&1 || { \
		echo "ERROR: docker is not installed."; \
		exit 1; \
	}
	@docker buildx version >/dev/null 2>&1 || { \
		echo "ERROR: docker buildx is not available."; \
		exit 1; \
	}
	@if docker buildx inspect "$(BUILDX_BUILDER)" >/dev/null 2>&1; then \
		docker buildx use "$(BUILDX_BUILDER)"; \
	else \
		docker buildx create \
			--name "$(BUILDX_BUILDER)" \
			--driver docker-container \
			--use >/dev/null; \
	fi
	@docker buildx inspect --bootstrap >/dev/null

container-login:
	docker login --username "$(DOCKERHUB_NAMESPACE)"

container-publish: container-builder
	docker buildx build \
		--builder "$(BUILDX_BUILDER)" \
		--platform "$(DOCKERHUB_PLATFORMS)" \
		-f Dockerfile \
		--build-arg GATEWAY_VERSION="$(GATEWAY_VERSION)" \
		--build-arg VERSION="$(DOCKERHUB_TAG)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg DATE="$(DATE)" \
		-t "$(DOCKERHUB_IMAGE):$(DOCKERHUB_TAG)" \
		--provenance=mode=max \
		--sbom=true \
		--push \
		.
	@$(MAKE) container-inspect

container-inspect:
	docker buildx imagetools inspect \
		"$(DOCKERHUB_IMAGE):$(DOCKERHUB_TAG)"

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
	@echo "Pulling Docker Hub Swarm image for $(SWARM_PLATFORM): $(SWARM_IMAGE)"
	docker pull \
		--platform "$(SWARM_PLATFORM)" \
		"$(SWARM_IMAGE)"

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
	@echo "Deploying Swarm stack: $(SWARM_STACK)"
	@echo "Swarm image: $(SWARM_IMAGE)"
	@set -a; \
	source "$(ENV_FILE)"; \
	set +a; \
	SWARM_IMAGE="$(SWARM_IMAGE)" \
	docker stack deploy \
		--with-registry-auth \
		--resolve-image always \
		-c "$(SWARM_STACK_FILE)" \
		"$(SWARM_STACK)"

swarm-wait:
	@echo "Waiting for $(SWARM_SERVICE) to reach 1/1..."
	@for attempt in $$(seq 1 60); do \
		replicas="$$(docker service ls \
			--filter name="$(SWARM_SERVICE)" \
			--format '{{.Name}} {{.Replicas}}' 2>/dev/null | \
			awk '$$1 == "$(SWARM_SERVICE)" { print $$2 }')"; \
		if [ "$$replicas" = "1/1" ]; then \
			echo "$(SWARM_SERVICE) is running: 1/1"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "ERROR: $(SWARM_SERVICE) did not reach 1/1."; \
	docker service ps "$(SWARM_SERVICE)"; \
	exit 1

swarm-local-deploy:
	@$(MAKE) swarm-preflight
	@$(MAKE) swarm-image
	@$(MAKE) swarm-init
	@$(MAKE) swarm-volume
	@$(MAKE) swarm-secrets
	@$(MAKE) swarm-deploy
	@$(MAKE) swarm-wait

swarm-reset-deploy:
	@$(MAKE) swarm-preflight
	@$(MAKE) swarm-image
	@$(MAKE) swarm-init
	@$(MAKE) swarm-stop
	@$(MAKE) swarm-volume-reset
	@$(MAKE) swarm-secrets-recreate
	@$(MAKE) swarm-deploy
	@$(MAKE) swarm-wait

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
	@docker service logs -f "$(SWARM_SERVICE)" || { \
		status=$$?; \
		[ "$$status" -eq 130 ] || exit "$$status"; \
	}

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

	@if docker image inspect "$(SWARM_IMAGE)" >/dev/null 2>&1; then \
		echo "Removing cached Docker Hub Swarm image: $(SWARM_IMAGE)"; \
		docker image rm "$(SWARM_IMAGE)" >/dev/null 2>&1 || true; \
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
	@echo "Full Swarm rebuild completed successfully."
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
	cp go.mod "$$tmp_dir/go.mod"; \
	if [ -f go.sum ]; then cp go.sum "$$tmp_dir/go.sum"; else : > "$$tmp_dir/go.sum"; fi; \
	trap 'cp "$$tmp_dir/go.mod" go.mod; if [ -s "$$tmp_dir/go.sum" ]; then cp "$$tmp_dir/go.sum" go.sum; else rm -f go.sum; fi; rm -rf "$$tmp_dir"' EXIT; \
	go mod tidy; \
	diff -u "$$tmp_dir/go.mod" go.mod; \
	if [ -f go.sum ]; then \
		diff -u "$$tmp_dir/go.sum" go.sum; \
	else \
		test ! -s "$$tmp_dir/go.sum"; \
	fi

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
# CLEANUP
# =========================

clean:
	rm -rf "$(BUILD_DIR)"
	go clean

include cluster/node/Makefile.mk

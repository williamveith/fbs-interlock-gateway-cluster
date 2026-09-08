# Host-level networking and Swarm bootstrap for physical Debian cluster nodes.
# This file is included by the repository root Makefile.

PRIVATE_CONNECTION ?=
NODE_ADDRESS ?=
NODE_IP ?= $(word 1,$(subst /, ,$(NODE_ADDRESS)))
MANAGER_IP ?=
MANAGER_TOKEN ?=
MANAGER_TOKEN_FILE ?=

UPLINK_CONNECTION ?= utexas-iot
SHARED_MAC ?=
PEER_MANAGER_IPS ?=
UPLINK_CHECK_INTERVAL ?= 1
UPLINK_SERVICE_NAME ?= $(SWARM_SERVICE)

.PHONY: \
	node-private-network \
	swarm-init-private \
	swarm-join-manager \
	swarm-manager-token \
	swarm-uplink-configure \
	swarm-uplink-install \
	swarm-uplink-status \
	swarm-uplink-uninstall

node-private-network:
	@test -n "$(PRIVATE_CONNECTION)" || { \
		echo 'ERROR: PRIVATE_CONNECTION is required.'; \
		exit 1; \
	}
	@test -n "$(NODE_ADDRESS)" || { \
		echo 'ERROR: NODE_ADDRESS is required, for example 10.50.0.11/24.'; \
		exit 1; \
	}
	sudo ./scripts/cluster/configure-private-network.sh \
		"$(PRIVATE_CONNECTION)" \
		"$(NODE_ADDRESS)"

swarm-init-private:
	@test -n "$(NODE_IP)" || { \
		echo 'ERROR: NODE_IP is required.'; \
		exit 1; \
	}
	@state="$$(docker info --format '{{.Swarm.LocalNodeState}}')"; \
	if [ "$$state" != "inactive" ]; then \
		echo "ERROR: Docker Swarm state is $$state; expected inactive."; \
		exit 1; \
	fi
	@echo "Initializing Swarm on private address $(NODE_IP)..."
	docker swarm init \
		--listen-addr "$(NODE_IP):2377" \
		--advertise-addr "$(NODE_IP)" \
		--data-path-addr "$(NODE_IP)"

swarm-manager-token:
	@docker swarm join-token -q manager

swarm-join-manager:
	@test -n "$(NODE_IP)" || { \
		echo 'ERROR: NODE_IP is required.'; \
		exit 1; \
	}
	@test -n "$(MANAGER_IP)" || { \
		echo 'ERROR: MANAGER_IP is required.'; \
		exit 1; \
	}
	@token='$(MANAGER_TOKEN)'; \
	if [ -n "$(MANAGER_TOKEN_FILE)" ]; then \
		token="$$(cat "$(MANAGER_TOKEN_FILE)")"; \
	fi; \
	if [ -z "$$token" ]; then \
		echo 'ERROR: Set MANAGER_TOKEN or MANAGER_TOKEN_FILE.'; \
		exit 1; \
	fi; \
	state="$$(docker info --format '{{.Swarm.LocalNodeState}}')"; \
	if [ "$$state" != "inactive" ]; then \
		echo "ERROR: Docker Swarm state is $$state; expected inactive."; \
		exit 1; \
	fi; \
	docker swarm join \
		--token "$$token" \
		--listen-addr "$(NODE_IP):2377" \
		--advertise-addr "$(NODE_IP)" \
		--data-path-addr "$(NODE_IP)" \
		"$(MANAGER_IP):2377"

swarm-uplink-configure:
	@test -n "$(UPLINK_CONNECTION)" || { \
		echo 'ERROR: UPLINK_CONNECTION is required.'; \
		exit 1; \
	}
	@test -n "$(SHARED_MAC)" || { \
		echo 'ERROR: SHARED_MAC is required.'; \
		exit 1; \
	}
	sudo ./scripts/cluster/configure-uplink.sh \
		"$(UPLINK_CONNECTION)" \
		"$(SHARED_MAC)"

swarm-uplink-install: swarm-uplink-configure
	@test -n "$(PEER_MANAGER_IPS)" || { \
		echo 'ERROR: PEER_MANAGER_IPS is required.'; \
		exit 1; \
	}
	sudo ./scripts/cluster/install-uplink-controller.sh \
		"$(UPLINK_CONNECTION)" \
		"$(UPLINK_SERVICE_NAME)" \
		"$(PEER_MANAGER_IPS)" \
		"$(UPLINK_CHECK_INTERVAL)"

swarm-uplink-status:
	@sudo systemctl status fbs-swarm-uplink.service --no-pager
	@echo
	@echo 'Active uplink connection:'
	@nmcli -t -f NAME connection show --active | \
		grep -Fx "$(UPLINK_CONNECTION)" || true

swarm-uplink-uninstall:
	@sudo systemctl disable --now fbs-swarm-uplink.service 2>/dev/null || true
	@sudo rm -f \
		/etc/systemd/system/fbs-swarm-uplink.service \
		/etc/default/fbs-swarm-uplink \
		/usr/local/sbin/fbs-swarm-uplink
	@sudo systemctl daemon-reload
	@echo 'Removed fbs-swarm-uplink service.'

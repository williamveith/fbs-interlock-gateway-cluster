APP := fbs-interlock-gateway-cluster
CMD := ./cmd/$(APP)

SERVICE_DIR_WINDOWS := services/windows
SERVICE_DIR_LINUX := services/linux
SERVICE_DIR_MACOS := services/macos

BUILD_DIR := build
MAC_DIR := $(BUILD_DIR)/darwin
MAC_ARM64_DIR := $(MAC_DIR)/arm64
MAC_AMD64_DIR := $(MAC_DIR)/amd64
MAC_ARM64_TLS_DIR := $(MAC_ARM64_DIR)/tls
MAC_AMD64_TLS_DIR := $(MAC_AMD64_DIR)/tls
LINUX_DIR := $(BUILD_DIR)/linux
LINUX_TLS_DIR := $(LINUX_DIR)/tls
WINDOWS_DIR := $(BUILD_DIR)/windows
WINDOWS_TLS_BUILD_DIR := $(WINDOWS_DIR)/tls

CONFIGS := config.yaml
CONFIG_DIR ?= /etc/$(APP)
CONFIG_PATH ?= $(CONFIG_DIR)/$(CONFIGS)

MACOS_CONFIG_DIR := /Library/Application Support/fbs-interlock-gateway-cluster
MACOS_DB_PATH := $(MACOS_CONFIG_DIR)/gateway.sqlite3

WINDOWS_INSTALL_DIR ?= C:/FBS/$(APP)
WINDOWS_CONFIG_DIR ?= $(WINDOWS_INSTALL_DIR)
WINDOWS_CONFIG_PATH ?= $(WINDOWS_CONFIG_DIR)/config.yaml
WINDOWS_DB_PATH ?= $(WINDOWS_CONFIG_DIR)/gateway.sqlite3
WINDOWS_TLS_DIR ?= $(WINDOWS_CONFIG_DIR)/tls
WINDOWS_LOG_DIR ?= $(WINDOWS_CONFIG_DIR)/logs

PKI_DIR := pki
PKI_CA_DIR := $(PKI_DIR)/ca
PKI_GATEWAY_DIR := $(PKI_DIR)/gateway
TLS_DIR ?= $(CONFIG_DIR)/tls

TLS_SERVER_CA_SOURCE := $(PKI_CA_DIR)/server-ca.crt
TLS_CLIENT_CERT_SOURCE := $(PKI_GATEWAY_DIR)/gateway-client.crt
TLS_CLIENT_KEY_SOURCE := $(PKI_GATEWAY_DIR)/gateway-client.key

TLS_SOURCE_FILES := \
	$(TLS_SERVER_CA_SOURCE) \
	$(TLS_CLIENT_CERT_SOURCE) \
	$(TLS_CLIENT_KEY_SOURCE)

INSTALL_DIR ?= /opt/$(APP)
SERVICE_USER ?= fbs-gateway
SERVICE_GROUP ?= $(SERVICE_USER)

FBS_SOURCE_IP=146.6.76.61
FBS_PORT_RANGE=8081:8981

DEPLOYMENT_GUIDES_DIR := docs/deployment guides

# Every Markdown guide matching the platform pattern is rendered to PDF and
# placed in that platform's build directory. Override a pattern at invocation
# time when needed, for example:
#   make build-linux-amd64 LINUX_DEPLOYMENT_GUIDE_PATTERN='*.md'
LINUX_DEPLOYMENT_GUIDE_PATTERN ?= Linux*.md
WINDOWS_DEPLOYMENT_GUIDE_PATTERN ?= Windows*.md
MACOS_DEPLOYMENT_GUIDE_PATTERN ?= macOS*.md

PANDOC ?= pandoc
PDF_ENGINE ?= xelatex
PDF_MARGIN ?= 0.3in
PDF_FONT_SIZE ?= 12pt
PDF_MAIN_FONT ?= IBMPlexMono-Regular
PDF_MONO_FONT ?= IBMPlexMono-Regular
PANDOC_INLINE_CODE_FILTER ?= scripts/pandoc/wrap-inline-code.lua

# =========================
# LINUX SERVICE CONFIGS
# =========================
SERVICE_TEMPLATE := $(SERVICE_DIR_LINUX)/app.service.in
SERVICE_OUT := $(LINUX_DIR)/$(APP).service

INSTALL_TEMPLATE := $(SERVICE_DIR_LINUX)/install-linux.sh.in
INSTALL_OUT := $(LINUX_DIR)/install.sh

INSTALL_DEV_TEMPLATE := $(SERVICE_DIR_LINUX)/install-linux-dev.sh.in
INSTALL_DEV_OUT := $(LINUX_DIR)/install-dev.sh
UNINSTALL_TEMPLATE := $(SERVICE_DIR_LINUX)/uninstall-linux.sh.in
UNINSTALL_OUT := $(LINUX_DIR)/uninstall.sh

UPDATE_TEMPLATE := $(SERVICE_DIR_LINUX)/update-linux.sh.in
UPDATE_OUT := $(LINUX_DIR)/update.sh

UPDATE_SERVICE_TEMPLATE := $(SERVICE_DIR_LINUX)/update.service.in
UPDATE_TIMER_TEMPLATE := $(SERVICE_DIR_LINUX)/update.timer.in

UPDATE_SERVICE_OUT := $(LINUX_DIR)/$(APP)-update.service
UPDATE_TIMER_OUT := $(LINUX_DIR)/$(APP)-update.timer

# =========================
# WINDOWS SERVICE CONFIGS
# =========================
WINDOWS_INSTALL_DIR ?= C:/FBS/$(APP)
WINDOWS_CONFIG_DIR ?= $(WINDOWS_INSTALL_DIR)
WINDOWS_CONFIG_PATH ?= $(WINDOWS_CONFIG_DIR)/config.yaml
WINDOWS_TLS_DIR ?= $(WINDOWS_CONFIG_DIR)/tls
WINDOWS_LOG_DIR ?= $(WINDOWS_CONFIG_DIR)/logs

WINDOWS_START_TEMPLATE := $(SERVICE_DIR_WINDOWS)/start.bat.in
WINDOWS_INSTALL_BAT_TEMPLATE := $(SERVICE_DIR_WINDOWS)/install.bat.in
WINDOWS_INSTALL_DEV_BAT_TEMPLATE := $(SERVICE_DIR_WINDOWS)/install-dev.bat.in
WINDOWS_INSTALL_PS1_TEMPLATE := $(SERVICE_DIR_WINDOWS)/install.ps1.in
WINDOWS_UPDATE_BAT_TEMPLATE := $(SERVICE_DIR_WINDOWS)/update.bat.in
WINDOWS_UPDATE_PS1_TEMPLATE := $(SERVICE_DIR_WINDOWS)/update.ps1.in
WINDOWS_UNINSTALL_BAT_TEMPLATE := $(SERVICE_DIR_WINDOWS)/uninstall.bat.in
WINDOWS_UNINSTALL_PS1_TEMPLATE := $(SERVICE_DIR_WINDOWS)/uninstall.ps1.in

WINDOWS_START_OUT := $(WINDOWS_DIR)/start.bat
WINDOWS_INSTALL_BAT_OUT := $(WINDOWS_DIR)/install.bat
WINDOWS_INSTALL_DEV_BAT_OUT := $(WINDOWS_DIR)/install-dev.bat
WINDOWS_INSTALL_PS1_OUT := $(WINDOWS_DIR)/install.ps1
WINDOWS_UPDATE_BAT_OUT := $(WINDOWS_DIR)/update.bat
WINDOWS_UPDATE_PS1_OUT := $(WINDOWS_DIR)/update.ps1
WINDOWS_UNINSTALL_BAT_OUT := $(WINDOWS_DIR)/uninstall.bat
WINDOWS_UNINSTALL_PS1_OUT := $(WINDOWS_DIR)/uninstall.ps1

WINDOWS_DEPLOYMENT_FILES := \
	$(WINDOWS_START_OUT) \
	$(WINDOWS_INSTALL_BAT_OUT) \
	$(WINDOWS_INSTALL_DEV_BAT_OUT) \
	$(WINDOWS_INSTALL_PS1_OUT) \
	$(WINDOWS_UPDATE_BAT_OUT) \
	$(WINDOWS_UPDATE_PS1_OUT) \
	$(WINDOWS_UNINSTALL_BAT_OUT) \
	$(WINDOWS_UNINSTALL_PS1_OUT)

# =========================
# MACOS SERVICE CONFIGS
# =========================
MACOS_INSTALL_DIR ?= /usr/local/libexec/$(APP)
MACOS_CONFIG_DIR ?= /Library/Application Support/$(APP)
MACOS_CONFIG_PATH ?= $(MACOS_CONFIG_DIR)/config.yaml
MACOS_TLS_DIR ?= $(MACOS_CONFIG_DIR)/tls
MACOS_LOG_DIR ?= /Library/Logs/$(APP)
MACOS_SERVICE_USER ?= _fbs-gateway
MACOS_SERVICE_GROUP ?= $(MACOS_SERVICE_USER)
MACOS_LAUNCHD_LABEL ?= com.williamveith.$(APP)
MACOS_UPDATE_LAUNCHD_LABEL ?= $(MACOS_LAUNCHD_LABEL)-update
MACOS_PF_ANCHOR_NAME ?= $(MACOS_LAUNCHD_LABEL)
MACOS_PF_ANCHOR_PATH ?= /etc/pf.anchors/$(MACOS_PF_ANCHOR_NAME)

MACOS_INSTALL_TEMPLATE := $(SERVICE_DIR_MACOS)/install-macos.sh.in
MACOS_INSTALL_DEV_TEMPLATE := $(SERVICE_DIR_MACOS)/install-macos-dev.sh.in
MACOS_START_TEMPLATE := $(SERVICE_DIR_MACOS)/start.sh.in
MACOS_UNINSTALL_TEMPLATE := $(SERVICE_DIR_MACOS)/uninstall-macos.sh.in
MACOS_PLIST_TEMPLATE := $(SERVICE_DIR_MACOS)/com.williamveith.fbs-interlock-gateway-cluster.plist.in
MACOS_UPDATE_TEMPLATE := $(SERVICE_DIR_MACOS)/update-macos.sh.in
MACOS_UPDATE_PLIST_TEMPLATE := $(SERVICE_DIR_MACOS)/com.williamveith.fbs-interlock-gateway-cluster-update.plist.in
MACOS_PF_ANCHOR_TEMPLATE := $(SERVICE_DIR_MACOS)/fbs-interlock-gateway-cluster.pf.in

MACOS_ARM64_INSTALL_OUT := $(MAC_ARM64_DIR)/install.sh
MACOS_ARM64_INSTALL_DEV_OUT := $(MAC_ARM64_DIR)/install-dev.sh
MACOS_ARM64_START_OUT := $(MAC_ARM64_DIR)/start.sh
MACOS_ARM64_UNINSTALL_OUT := $(MAC_ARM64_DIR)/uninstall.sh
MACOS_ARM64_PLIST_OUT := $(MAC_ARM64_DIR)/$(MACOS_LAUNCHD_LABEL).plist
MACOS_ARM64_UPDATE_OUT := $(MAC_ARM64_DIR)/update.sh
MACOS_ARM64_UPDATE_PLIST_OUT := $(MAC_ARM64_DIR)/$(MACOS_UPDATE_LAUNCHD_LABEL).plist
MACOS_ARM64_PF_ANCHOR_OUT := $(MAC_ARM64_DIR)/$(MACOS_PF_ANCHOR_NAME).pf

MACOS_AMD64_INSTALL_OUT := $(MAC_AMD64_DIR)/install.sh
MACOS_AMD64_INSTALL_DEV_OUT := $(MAC_AMD64_DIR)/install-dev.sh
MACOS_AMD64_START_OUT := $(MAC_AMD64_DIR)/start.sh
MACOS_AMD64_UNINSTALL_OUT := $(MAC_AMD64_DIR)/uninstall.sh
MACOS_AMD64_PLIST_OUT := $(MAC_AMD64_DIR)/$(MACOS_LAUNCHD_LABEL).plist
MACOS_AMD64_UPDATE_OUT := $(MAC_AMD64_DIR)/update.sh
MACOS_AMD64_UPDATE_PLIST_OUT := $(MAC_AMD64_DIR)/$(MACOS_UPDATE_LAUNCHD_LABEL).plist
MACOS_AMD64_PF_ANCHOR_OUT := $(MAC_AMD64_DIR)/$(MACOS_PF_ANCHOR_NAME).pf

MACOS_ARM64_DEPLOYMENT_FILES := \
	$(MACOS_ARM64_INSTALL_OUT) \
	$(MACOS_ARM64_INSTALL_DEV_OUT) \
	$(MACOS_ARM64_START_OUT) \
	$(MACOS_ARM64_UNINSTALL_OUT) \
	$(MACOS_ARM64_PLIST_OUT) \
	$(MACOS_ARM64_UPDATE_OUT) \
	$(MACOS_ARM64_UPDATE_PLIST_OUT) \
	$(MACOS_ARM64_PF_ANCHOR_OUT)

MACOS_AMD64_DEPLOYMENT_FILES := \
	$(MACOS_AMD64_INSTALL_OUT) \
	$(MACOS_AMD64_INSTALL_DEV_OUT) \
	$(MACOS_AMD64_START_OUT) \
	$(MACOS_AMD64_UNINSTALL_OUT) \
	$(MACOS_AMD64_PLIST_OUT) \
	$(MACOS_AMD64_UPDATE_OUT) \
	$(MACOS_AMD64_UPDATE_PLIST_OUT) \
	$(MACOS_AMD64_PF_ANCHOR_OUT)

RELEASE_DIR := $(BUILD_DIR)/release
LINUX_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-linux-amd64
LINUX_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-linux-arm64
WINDOWS_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-windows-amd64.exe
DARWIN_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-darwin-arm64
DARWIN_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-darwin-amd64

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
SHA256SUM := shasum -a 256
else
SHA256SUM := sha256sum
endif

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
	build-check \
	verify \
	init-config \
	build \
	build-mac \
	build-darwin-arm64 \
	build-darwin-amd64 \
	build-linux-arm64 \
	build-linux-amd64 \
	build-windows-amd64 \
	check-runtime-tls \
	linux-deployment-guides \
	windows-deployment-guides \
	macos-arm64-deployment-guides \
	macos-amd64-deployment-guides \
	windows-deployment-files \
	macos-arm64-deployment-files \
	macos-amd64-deployment-files \
	macos-deployment-files \
	release-linux-amd64 \
	release-linux-arm64 \
	release-windows-amd64 \
	release-darwin-arm64 \
	release-darwin-amd64 \
	release \
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

build: build-darwin-arm64

build-mac: build-darwin-arm64

# =========================
# GENERATED DEPLOYMENT FILES
# =========================

$(SERVICE_OUT): $(SERVICE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@CONFIG_DIR@|$(CONFIG_DIR)|g' \
		-e 's|@CONFIG_PATH@|$(CONFIG_PATH)|g' \
		-e 's|@TLS_DIR@|$(TLS_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(SERVICE_TEMPLATE)" > "$@"

$(INSTALL_OUT): $(INSTALL_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@CONFIG_DIR@|$(CONFIG_DIR)|g' \
		-e 's|@CONFIG_PATH@|$(CONFIG_PATH)|g' \
		-e 's|@TLS_DIR@|$(TLS_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
		"$(INSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

$(INSTALL_DEV_OUT): $(INSTALL_DEV_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@CONFIG_DIR@|$(CONFIG_DIR)|g' \
		-e 's|@CONFIG_PATH@|$(CONFIG_PATH)|g' \
		-e 's|@TLS_DIR@|$(TLS_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
		"$(INSTALL_DEV_TEMPLATE)" > "$@"
	chmod +x "$@"

$(UNINSTALL_OUT): $(UNINSTALL_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@CONFIG_DIR@|$(CONFIG_DIR)|g' \
		-e 's|@CONFIG_PATH@|$(CONFIG_PATH)|g' \
		-e 's|@TLS_DIR@|$(TLS_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
		"$(UNINSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

$(UPDATE_OUT): $(UPDATE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(UPDATE_TEMPLATE)" > "$@"
	chmod +x "$@"

$(UPDATE_SERVICE_OUT): $(UPDATE_SERVICE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(UPDATE_SERVICE_TEMPLATE)" > "$@"

$(UPDATE_TIMER_OUT): $(UPDATE_TIMER_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(UPDATE_TIMER_TEMPLATE)" > "$@"

$(WINDOWS_START_OUT): $(WINDOWS_START_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		-e 's|@WINDOWS_CONFIG_DIR@|$(WINDOWS_CONFIG_DIR)|g' \
		-e 's|@WINDOWS_CONFIG_PATH@|$(WINDOWS_CONFIG_PATH)|g' \
		-e 's|@WINDOWS_DB_PATH@|$(WINDOWS_DB_PATH)|g' \
		-e 's|@WINDOWS_LOG_DIR@|$(WINDOWS_LOG_DIR)|g' \
		"$(WINDOWS_START_TEMPLATE)" > "$@"

$(WINDOWS_INSTALL_BAT_OUT): $(WINDOWS_INSTALL_BAT_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	cp "$(WINDOWS_INSTALL_BAT_TEMPLATE)" "$@"

$(WINDOWS_INSTALL_DEV_BAT_OUT): $(WINDOWS_INSTALL_DEV_BAT_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	cp "$(WINDOWS_INSTALL_DEV_BAT_TEMPLATE)" "$@"

$(WINDOWS_INSTALL_PS1_OUT): $(WINDOWS_INSTALL_PS1_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		-e 's|@WINDOWS_CONFIG_DIR@|$(WINDOWS_CONFIG_DIR)|g' \
		-e 's|@WINDOWS_CONFIG_PATH@|$(WINDOWS_CONFIG_PATH)|g' \
		-e 's|@WINDOWS_TLS_DIR@|$(WINDOWS_TLS_DIR)|g' \
		-e 's|@WINDOWS_DB_PATH@|$(WINDOWS_DB_PATH)|g' \
		-e 's|@WINDOWS_LOG_DIR@|$(WINDOWS_LOG_DIR)|g' \
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
		"$(WINDOWS_INSTALL_PS1_TEMPLATE)" > "$@"

$(WINDOWS_UPDATE_BAT_OUT): $(WINDOWS_UPDATE_BAT_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		-e 's|@WINDOWS_LOG_DIR@|$(WINDOWS_LOG_DIR)|g' \
		"$(WINDOWS_UPDATE_BAT_TEMPLATE)" > "$@"

$(WINDOWS_UPDATE_PS1_OUT): $(WINDOWS_UPDATE_PS1_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		-e 's|@WINDOWS_DB_PATH@|$(WINDOWS_DB_PATH)|g' \
		-e 's|@WINDOWS_LOG_DIR@|$(WINDOWS_LOG_DIR)|g' \
		"$(WINDOWS_UPDATE_PS1_TEMPLATE)" > "$@"

$(WINDOWS_UNINSTALL_BAT_OUT): $(WINDOWS_UNINSTALL_BAT_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	cp "$(WINDOWS_UNINSTALL_BAT_TEMPLATE)" "$@"

$(WINDOWS_UNINSTALL_PS1_OUT): $(WINDOWS_UNINSTALL_PS1_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		-e 's|@WINDOWS_CONFIG_DIR@|$(WINDOWS_CONFIG_DIR)|g' \
		-e 's|@WINDOWS_CONFIG_PATH@|$(WINDOWS_CONFIG_PATH)|g' \
		-e 's|@WINDOWS_TLS_DIR@|$(WINDOWS_TLS_DIR)|g' \
		-e 's|@WINDOWS_DB_PATH@|$(WINDOWS_DB_PATH)|g' \
		-e 's|@WINDOWS_LOG_DIR@|$(WINDOWS_LOG_DIR)|g' \
		"$(WINDOWS_UNINSTALL_PS1_TEMPLATE)" > "$@"

windows-deployment-files: $(WINDOWS_DEPLOYMENT_FILES)


$(MACOS_ARM64_INSTALL_OUT) $(MACOS_AMD64_INSTALL_OUT): $(MACOS_INSTALL_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_DIR@|$(MACOS_CONFIG_DIR)|g' \
		-e 's|@MACOS_CONFIG_PATH@|$(MACOS_CONFIG_PATH)|g' \
		-e 's|@MACOS_TLS_DIR@|$(MACOS_TLS_DIR)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_SERVICE_USER@|$(MACOS_SERVICE_USER)|g' \
		-e 's|@MACOS_SERVICE_GROUP@|$(MACOS_SERVICE_GROUP)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		-e 's|@MACOS_UPDATE_LAUNCHD_LABEL@|$(MACOS_UPDATE_LAUNCHD_LABEL)|g' \
		-e 's|@MACOS_PF_ANCHOR_NAME@|$(MACOS_PF_ANCHOR_NAME)|g' \
		-e 's|@MACOS_PF_ANCHOR_PATH@|$(MACOS_PF_ANCHOR_PATH)|g' \
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
		"$(MACOS_INSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_INSTALL_DEV_OUT) $(MACOS_AMD64_INSTALL_DEV_OUT): $(MACOS_INSTALL_DEV_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	cp "$(MACOS_INSTALL_DEV_TEMPLATE)" "$@"
	chmod +x "$@"

$(MACOS_ARM64_START_OUT) $(MACOS_AMD64_START_OUT): $(MACOS_START_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_DIR@|$(MACOS_CONFIG_DIR)|g' \
		-e 's|@MACOS_CONFIG_PATH@|$(MACOS_CONFIG_PATH)|g' \
		-e 's|@MACOS_DB_PATH@|$(MACOS_DB_PATH)|g' \
		"$(MACOS_START_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_UNINSTALL_OUT) $(MACOS_AMD64_UNINSTALL_OUT): $(MACOS_UNINSTALL_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_DIR@|$(MACOS_CONFIG_DIR)|g' \
		-e 's|@MACOS_CONFIG_PATH@|$(MACOS_CONFIG_PATH)|g' \
		-e 's|@MACOS_TLS_DIR@|$(MACOS_TLS_DIR)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		-e 's|@MACOS_UPDATE_LAUNCHD_LABEL@|$(MACOS_UPDATE_LAUNCHD_LABEL)|g' \
		-e 's|@MACOS_PF_ANCHOR_NAME@|$(MACOS_PF_ANCHOR_NAME)|g' \
		-e 's|@MACOS_PF_ANCHOR_PATH@|$(MACOS_PF_ANCHOR_PATH)|g' \
		"$(MACOS_UNINSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_PLIST_OUT) $(MACOS_AMD64_PLIST_OUT): $(MACOS_PLIST_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_DIR@|$(MACOS_CONFIG_DIR)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_SERVICE_USER@|$(MACOS_SERVICE_USER)|g' \
		-e 's|@MACOS_SERVICE_GROUP@|$(MACOS_SERVICE_GROUP)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		"$(MACOS_PLIST_TEMPLATE)" > "$@"

$(MACOS_ARM64_UPDATE_OUT) $(MACOS_AMD64_UPDATE_OUT): $(MACOS_UPDATE_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_SERVICE_USER@|$(MACOS_SERVICE_USER)|g' \
		-e 's|@MACOS_SERVICE_GROUP@|$(MACOS_SERVICE_GROUP)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		"$(MACOS_UPDATE_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_UPDATE_PLIST_OUT) $(MACOS_AMD64_UPDATE_PLIST_OUT): $(MACOS_UPDATE_PLIST_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_UPDATE_LAUNCHD_LABEL@|$(MACOS_UPDATE_LAUNCHD_LABEL)|g' \
		"$(MACOS_UPDATE_PLIST_TEMPLATE)" > "$@"

$(MACOS_ARM64_PF_ANCHOR_OUT) $(MACOS_AMD64_PF_ANCHOR_OUT): $(MACOS_PF_ANCHOR_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
		"$(MACOS_PF_ANCHOR_TEMPLATE)" > "$@"

macos-arm64-deployment-files: $(MACOS_ARM64_DEPLOYMENT_FILES)

macos-amd64-deployment-files: $(MACOS_AMD64_DEPLOYMENT_FILES)

macos-deployment-files: \
	macos-arm64-deployment-files \
	macos-amd64-deployment-files

# =========================
# DEPLOYMENT GUIDE PDFS
# =========================

define build_deployment_guides
set -eu; \
command -v "$(PANDOC)" >/dev/null 2>&1 || { \
    echo "ERROR: $(PANDOC) is required to build deployment guide PDFs."; \
    exit 1; \
}; \
command -v "$(PDF_ENGINE)" >/dev/null 2>&1 || { \
    echo "ERROR: $(PDF_ENGINE) is required to build deployment guide PDFs."; \
    exit 1; \
}; \
[ -f "$(PANDOC_INLINE_CODE_FILTER)" ] || { \
    echo "ERROR: Missing Pandoc inline-code filter: $(PANDOC_INLINE_CODE_FILTER)"; \
    exit 1; \
}; \
mkdir -p "$(1)"; \
if ! find "$(DEPLOYMENT_GUIDES_DIR)" -maxdepth 1 -type f -name '$(2)' \
    -print -quit | grep -q .; then \
    echo "ERROR: No deployment guides matched $(DEPLOYMENT_GUIDES_DIR)/$(2)"; \
    exit 1; \
fi; \
find "$(DEPLOYMENT_GUIDES_DIR)" -maxdepth 1 -type f -name '$(2)' \
    -exec sh -c '\
        out_dir=$$1; \
        shift; \
        for guide in "$$@"; do \
            pdf_name=$$(basename "$$guide" .md).pdf; \
            echo "Rendering $$guide -> $$out_dir/$$pdf_name"; \
            "$(PANDOC)" "$$guide" \
				-o "$$out_dir/$$pdf_name" \
				--pdf-engine="$(PDF_ENGINE)" \
				--lua-filter="$(PANDOC_INLINE_CODE_FILTER)" \
				-V geometry:margin="$(PDF_MARGIN)" \
				-V fontsize="$(PDF_FONT_SIZE)" \
				-V mainfont="$(PDF_MAIN_FONT)" \
				-V monofont="$(PDF_MONO_FONT)" \
				-V colorlinks=true \
				-V linkcolor=blue \
				-V urlcolor=blue \
				-V "header-includes=\usepackage{fvextra}" \
				-V "header-includes=\fvset{breaklines=true,breakanywhere=true}" \
				-V "header-includes=\setlength{\parindent}{0pt}" \
				-V "header-includes=\setlength{\parskip}{0.6em}" \
				-V "header-includes=\setlength{\emergencystretch}{3em}" \
				-V "header-includes=\raggedbottom"; \
        done \
    ' sh "$(1)" {} +
endef

linux-deployment-guides:
	@$(call build_deployment_guides,$(LINUX_DIR),$(LINUX_DEPLOYMENT_GUIDE_PATTERN))

windows-deployment-guides:
	@$(call build_deployment_guides,$(WINDOWS_DIR),$(WINDOWS_DEPLOYMENT_GUIDE_PATTERN))

macos-arm64-deployment-guides:
	@$(call build_deployment_guides,$(MAC_ARM64_DIR),$(MACOS_DEPLOYMENT_GUIDE_PATTERN))

macos-amd64-deployment-guides:
	@$(call build_deployment_guides,$(MAC_AMD64_DIR),$(MACOS_DEPLOYMENT_GUIDE_PATTERN))

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
# DEPLOYMENT BUILDS
# =========================


check-runtime-tls:
	@for file in $(TLS_SOURCE_FILES); do \
		if [ ! -f "$$file" ]; then \
			echo "ERROR: Missing gateway runtime TLS file: $$file"; \
			echo "Run scripts/tls/create-ca.sh and"; \
			echo "scripts/tls/create-gateway-client.sh first."; \
			exit 1; \
		fi; \
	done

define copy_linux_tls
	rm -rf "$(LINUX_TLS_DIR)"
	mkdir -p "$(LINUX_TLS_DIR)"
	install -m 0644 \
		"$(TLS_SERVER_CA_SOURCE)" \
		"$(LINUX_TLS_DIR)/server-ca.crt"
	install -m 0644 \
		"$(TLS_CLIENT_CERT_SOURCE)" \
		"$(LINUX_TLS_DIR)/gateway-client.crt"
	install -m 0600 \
		"$(TLS_CLIENT_KEY_SOURCE)" \
		"$(LINUX_TLS_DIR)/gateway-client.key"
endef


define copy_windows_tls
	rm -rf "$(WINDOWS_TLS_BUILD_DIR)"
	mkdir -p "$(WINDOWS_TLS_BUILD_DIR)"
	install -m 0644 \
		"$(TLS_SERVER_CA_SOURCE)" \
		"$(WINDOWS_TLS_BUILD_DIR)/server-ca.crt"
	install -m 0644 \
		"$(TLS_CLIENT_CERT_SOURCE)" \
		"$(WINDOWS_TLS_BUILD_DIR)/gateway-client.crt"
	install -m 0600 \
		"$(TLS_CLIENT_KEY_SOURCE)" \
		"$(WINDOWS_TLS_BUILD_DIR)/gateway-client.key"
endef


define copy_macos_tls
	rm -rf "$(1)"
	mkdir -p "$(1)"
	install -m 0644 \
		"$(TLS_SERVER_CA_SOURCE)" \
		"$(1)/server-ca.crt"
	install -m 0644 \
		"$(TLS_CLIENT_CERT_SOURCE)" \
		"$(1)/gateway-client.crt"
	install -m 0600 \
		"$(TLS_CLIENT_KEY_SOURCE)" \
		"$(1)/gateway-client.key"
endef


build-darwin-arm64: fmt check-runtime-tls macos-arm64-deployment-guides $(MACOS_ARM64_DEPLOYMENT_FILES)
	mkdir -p "$(MAC_ARM64_DIR)"
	cp "$(CONFIGS)" "$(MAC_ARM64_DIR)/"
	$(call copy_macos_tls,$(MAC_ARM64_TLS_DIR))
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_ARM64_DIR)/$(APP)" \
		$(CMD)

build-darwin-amd64: fmt check-runtime-tls macos-amd64-deployment-guides $(MACOS_AMD64_DEPLOYMENT_FILES)
	mkdir -p "$(MAC_AMD64_DIR)"
	cp "$(CONFIGS)" "$(MAC_AMD64_DIR)/"
	$(call copy_macos_tls,$(MAC_AMD64_TLS_DIR))
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_AMD64_DIR)/$(APP)" \
		$(CMD)

build-linux-arm64: fmt check-runtime-tls linux-deployment-guides $(SERVICE_OUT) $(INSTALL_OUT) $(INSTALL_DEV_OUT) $(UNINSTALL_OUT) $(UPDATE_OUT) $(UPDATE_SERVICE_OUT) $(UPDATE_TIMER_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	$(copy_linux_tls)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" \
		$(CMD)

build-linux-amd64: fmt check-runtime-tls linux-deployment-guides $(SERVICE_OUT) $(INSTALL_OUT) $(INSTALL_DEV_OUT) $(UNINSTALL_OUT) $(UPDATE_OUT) $(UPDATE_SERVICE_OUT) $(UPDATE_TIMER_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	$(copy_linux_tls)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" \
		$(CMD)

build-windows-amd64: fmt check-runtime-tls windows-deployment-guides $(WINDOWS_DEPLOYMENT_FILES)
	mkdir -p "$(WINDOWS_DIR)"
	cp "$(CONFIGS)" "$(WINDOWS_DIR)/"
	$(copy_windows_tls)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(WINDOWS_DIR)/$(APP).exe" \
		$(CMD)

# =========================
# RELEASE BUILDS
# =========================

release-linux-amd64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-linux-amd64" > "$(APP)-linux-amd64.sha256"

release-linux-arm64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_ARM64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-linux-arm64" > "$(APP)-linux-arm64.sha256"

release-windows-amd64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(WINDOWS_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-windows-amd64.exe" > "$(APP)-windows-amd64.exe.sha256"

release-darwin-arm64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(DARWIN_ARM64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-darwin-arm64" > \
		"$(APP)-darwin-arm64.sha256"

release-darwin-amd64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(DARWIN_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-darwin-amd64" > \
		"$(APP)-darwin-amd64.sha256"

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
	bash -n scripts/*.sh
	bash -n scripts/tls/*.sh
	bash -n \
		"$(INSTALL_TEMPLATE)" \
		"$(INSTALL_DEV_TEMPLATE)" \
		"$(UNINSTALL_TEMPLATE)" \
		"$(UPDATE_TEMPLATE)" \
		"$(MACOS_INSTALL_TEMPLATE)" \
		"$(MACOS_INSTALL_DEV_TEMPLATE)" \
		"$(MACOS_START_TEMPLATE)" \
		"$(MACOS_UNINSTALL_TEMPLATE)" \
		"$(MACOS_UPDATE_TEMPLATE)"
	@if command -v pwsh >/dev/null 2>&1; then \
		pwsh -NoLogo -NoProfile -Command '$$failed = $$false; foreach ($$path in @("$(WINDOWS_INSTALL_PS1_TEMPLATE)", "$(WINDOWS_UPDATE_PS1_TEMPLATE)", "$(WINDOWS_UNINSTALL_PS1_TEMPLATE)")) { $$tokens = $$null; $$errors = $$null; [void][System.Management.Automation.Language.Parser]::ParseFile($$path, [ref]$$tokens, [ref]$$errors); foreach ($$parseError in $$errors) { Write-Error ($$path + ": " + $$parseError.Message); $$failed = $$true } }; if ($$failed) { exit 1 }'; \
	else \
		echo "Skipping PowerShell syntax validation: pwsh unavailable."; \
	fi
	python3 -c 'import pathlib, re, sys; paths = [pathlib.Path(p) for p in ("services/windows/install.ps1.in", "services/windows/update.ps1.in", "services/windows/uninstall.ps1.in")]; pattern = re.compile(r"\$$[A-Za-z_][A-Za-z0-9_]*:(?![A-Za-z_][A-Za-z0-9_]*)"); failures = [(str(path), index, line.rstrip()) for path in paths for index, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1) if pattern.search(line)]; [print(f"{path}:{index}: ambiguous PowerShell variable interpolation before colon: {line}", file=sys.stderr) for path, index, line in failures]; sys.exit(1 if failures else 0)'
	@if command -v plutil >/dev/null 2>&1; then \
		plutil -lint "$(MACOS_PLIST_TEMPLATE)"; \
		plutil -lint "$(MACOS_UPDATE_PLIST_TEMPLATE)"; \
	elif command -v python3 >/dev/null 2>&1; then \
		python3 -c 'import plistlib; plistlib.load(open("$(MACOS_PLIST_TEMPLATE)", "rb")); plistlib.load(open("$(MACOS_UPDATE_PLIST_TEMPLATE)", "rb"))'; \
	else \
		echo "Skipping plist validation: plutil and python3 unavailable."; \
	fi

shellcheck:
	find scripts services \
		-type f \( -name '*.sh' -o -name '*.sh.in' \) \
		-exec shellcheck {} +

build-check:
	mkdir -p "$(BUILD_DIR)/ci"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-linux-amd64" \
		$(CMD)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-linux-arm64" \
		$(CMD)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-windows-amd64.exe" \
		$(CMD)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-darwin-arm64" \
		$(CMD)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-darwin-amd64" \
		$(CMD)

verify: fmt-check tidy-check vet staticcheck test-race scripts-check shellcheck build-check

release: \
	verify \
	release-linux-amd64 \
	release-linux-arm64 \
	release-windows-amd64 \
	release-darwin-arm64 \
	release-darwin-amd64

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

clean:
	rm -rf "$(BUILD_DIR)"
	go clean
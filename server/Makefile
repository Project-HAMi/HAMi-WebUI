.DEFAULT_GOAL := help

DOCKER ?= docker
DOCKER_IMAGE ?= hami-webui-backend
CMD_PATH ?= ./cmd/
BUILD_PATH ?= ./build/
VERSION ?= dev
PLATFORM ?=
DOCKER_PLATFORM_FLAG := $(if $(strip $(PLATFORM)),--platform $(PLATFORM),)
DIRS ?= $(notdir $(patsubst %/,%,$(sort $(dir $(wildcard $(CMD_PATH)*/main.go)))))
WIRE_DIRS = $(patsubst $(CMD_PATH)%/wire.go,%,$(wildcard $(CMD_PATH)*/wire.go))
TARGET_ARCH ?= amd64
WIRE_VERSION ?= v0.7.0
PROTOC_GEN_VALIDATE_VERSION ?= v1.0.4
KRATOS_VERSION ?= v2.0.0-20240322155018-41971ffa647a
PROTOC_GEN_GO_VERSION ?= v1.34.2
PROTOC_GEN_GO_GRPC_VERSION ?= v1.4.0
PROTOC_GEN_GO_HTTP_VERSION ?= v2.0.0-20240322155018-41971ffa647a
PROTOC_GEN_GO_ERRORS_VERSION ?= v2.0.0-20240322155018-41971ffa647a
PROTOC_GEN_OPENAPI_VERSION ?= v0.6.9
PROTO_TOOLS = \
	protoc \
	wire \
	kratos \
	protoc-gen-validate \
	protoc-gen-go \
	protoc-gen-go-grpc \
	protoc-gen-go-http \
	protoc-gen-go-errors \
	protoc-gen-openapi

.PHONY: help
help:
	@printf '%s\n' \
		'Local HAMi-WebUI backend development:' \
		'  make bootstrap        Install pinned Go code generators' \
		'  make run              Generate code and start the backend' \
		'  make test             Generate code and run all Go tests' \
		'  make vet              Generate code and run go vet' \
		'  make verify           Generate, build, vet, and test all Go packages' \
		'  make build            Generate and build Linux commands for TARGET_ARCH' \
		'  make build-local      Generate and build commands for the host platform' \
		'  make build-web-entry  Build only the production Web entry' \
		'  make build-image      Build a local backend image' \
		'' \
		'Install protoc separately before bootstrap; use PLATFORM for Docker builds.'

.PHONY: bootstrap
bootstrap:
	$(MAKE) install-deps
	$(MAKE) check-proto-tools

# protoc is an external prerequisite; install the pinned Go generators.
.PHONY: install-deps
install-deps:
	go install github.com/google/wire/cmd/wire@$(WIRE_VERSION)
	go install github.com/envoyproxy/protoc-gen-validate@$(PROTOC_GEN_VALIDATE_VERSION)
	go install github.com/go-kratos/kratos/cmd/kratos/v2@$(KRATOS_VERSION)
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@$(PROTOC_GEN_GO_HTTP_VERSION)
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-errors/v2@$(PROTOC_GEN_GO_ERRORS_VERSION)
	go install github.com/google/gnostic/cmd/protoc-gen-openapi@$(PROTOC_GEN_OPENAPI_VERSION)

.PHONY: generate
generate: generate-proto generate-wire

# Kratos otherwise runs `kratos upgrade`, resolving every generator from @latest.
.PHONY: check-proto-tools
check-proto-tools:
	@set -eu; missing=""; \
	for tool in $(PROTO_TOOLS); do \
		if ! command -v "$$tool" >/dev/null 2>&1; then missing="$$missing $$tool"; fi; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "missing required protobuf tools:$$missing; install protoc, run 'make install-deps', and ensure the Go bin directory is on PATH" >&2; \
		exit 1; \
	fi

.PHONY: generate-proto
generate-proto: check-proto-tools
	kratos proto client .

.PHONY: generate-wire
generate-wire:
	@set -eu; for dir in $(WIRE_DIRS); \
	do \
		(cd "$(CMD_PATH)$$dir" && wire .); \
	done

# Verify module metadata without changing it. Release builds must not repair
# dependency files as a side effect of code generation.
.PHONY: verify-mod
verify-mod:
	go mod tidy -diff

.PHONY: tidy-mod
tidy-mod:
	go mod tidy

.PHONY: build-linux
build-linux:
	@mkdir -p "$(BUILD_PATH)"
	@echo "Building for GOOS=linux GOARCH=$(TARGET_ARCH)"
	@set -e; for dir in $(DIRS); \
	do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGET_ARCH) go build -mod=readonly -trimpath -o "$(BUILD_PATH)$$dir" "$(CMD_PATH)$$dir"; \
		echo "Built $$dir for Linux/$(TARGET_ARCH)"; \
	done

.PHONY: build-local
build-local: generate
	$(MAKE) verify-mod
	@mkdir -p "$(BUILD_PATH)"
	@set -e; for dir in $(DIRS); \
	do \
		go build -mod=readonly -trimpath -o "$(BUILD_PATH)$$dir" "$(CMD_PATH)$$dir"; \
		echo "Built $$dir"; \
	done

.PHONY: build-web-entry
build-web-entry:
	@mkdir -p "$(BUILD_PATH)"
	go build -mod=readonly -trimpath -o "$(BUILD_PATH)web-entry" ./cmd/web-entry

# This target creates one local development image. Stable multi-platform images
# are published only by ../.github/workflows/release.yaml.
.PHONY: build-image
build-image:
	$(DOCKER) build $(DOCKER_PLATFORM_FLAG) -t $(DOCKER_IMAGE):$(VERSION) .

.PHONY: save-image
save-image:
	@mkdir -p "$(BUILD_PATH)"
	$(DOCKER) save -o "$(BUILD_PATH)hami_webui_release_$(VERSION).tar" $(DOCKER_IMAGE):$(VERSION)
	gzip -f "$(BUILD_PATH)hami_webui_release_$(VERSION).tar"

.PHONY: run
run: generate
	$(MAKE) verify-mod
	kratos run

.PHONY: test
test: generate
	$(MAKE) verify-mod
	go test -mod=readonly ./...

.PHONY: vet
vet: generate
	$(MAKE) verify-mod
	go vet -mod=readonly ./...

.PHONY: verify
verify: generate
	$(MAKE) verify-mod
	go build -mod=readonly ./...
	go vet -mod=readonly ./...
	go test -mod=readonly ./...

.PHONY: clean
clean:
	rm -rf $(BUILD_PATH)

.PHONY: build
build: generate
	$(MAKE) verify-mod
	$(MAKE) build-linux

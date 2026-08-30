DOCKER_IMAGE ?= projecthami/hami-webui-be
CMD_PATH ?= ./cmd/
BUILD_PATH ?= ./build/
VERSION ?= $(shell git describe --tags --always)
DIRS = $(shell ls $(CMD_PATH))
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
	kratos \
	protoc-gen-go \
	protoc-gen-go-grpc \
	protoc-gen-go-http \
	protoc-gen-go-errors \
	protoc-gen-openapi

# 1. Install Go dependencies
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

# 2. Generate proto and wire files
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
		echo "missing required protobuf tools:$$missing; install them before generating code (Go tools: 'make install-deps')" >&2; \
		exit 1; \
	fi

.PHONY: generate-proto
generate-proto: check-proto-tools
	kratos proto client .

.PHONY: generate-wire
generate-wire:
	@ for dir in $(WIRE_DIRS); \
	do \
		cd ${CMD_PATH}$$dir/ && wire . && cd ../../; \
	done

# Verify module metadata without changing it. Release builds must not repair
# dependency files as a side effect of code generation.
.PHONY: verify-mod
verify-mod:
	go mod tidy -diff

# 3. Clean Go modules
.PHONY: tidy-mod
tidy-mod:
	go mod tidy

# 4. Build-related commands
.PHONY: build-linux
build-linux:
	@echo "Building for GOOS=linux GOARCH=$(TARGET_ARCH)" # TARGET_ARCH 会从 make 命令传入
	@set -e; for dir in $(DIRS); \
	do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGET_ARCH) go build -o $(BUILD_PATH)$$dir ${CMD_PATH}$$dir; \
		echo "Built $$dir for Linux/$(TARGET_ARCH)"; \
	done

.PHONY: build-local
build-local:
	@ for dir in $(DIRS); \
	do \
		go build -o $(BUILD_PATH)$$dir ${CMD_PATH}$$dir; \
		echo "Built $$dir"; \
	done

# 5. Docker image commands
.PHONY: build-image
build-image:
	docker build --platform linux/amd64 -t ${DOCKER_IMAGE}:${VERSION} .

.PHONY: push-image
push-image:
	docker push ${DOCKER_IMAGE}:${VERSION}

.PHONY: save-image
save-image:
	docker save -o $(BUILD_PATH)hami_webui_release_${VERSION}.tar ${DOCKER_IMAGE}:${VERSION}
	gzip -f $(BUILD_PATH)hami_webui_release_${VERSION}.tar

# 6. Debugging and Testing
.PHONY: run
run: install-deps generate run-debug

.PHONY: run-debug
run-debug:
	kratos run

.PHONY: run-tests
run-tests:
	go test .

.PHONY: run-vet
run-vet:
	go vet ./...

# 7. Cleaning
.PHONY: clean-build
clean-build:
	rm -rf $(BUILD_PATH)

# 8. Complete build workflow
.PHONY: build
build: install-deps generate verify-mod build-linux

# 9. Release workflow
.PHONY: release
release: build-image push-image

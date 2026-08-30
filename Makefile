.DEFAULT_GOAL := help

PNPM ?= pnpm
DOCKER ?= docker
DOCKER_IMAGE ?= hami-webui
VERSION ?= dev
PLATFORM ?=
DOCKER_PLATFORM_FLAG := $(if $(strip $(PLATFORM)),--platform $(PLATFORM),)

.PHONY: help
help:
	@printf '%s\n' \
		'Local HAMi-WebUI development:' \
		'  make bootstrap              Install locked Node.js dependencies' \
		'  make dev                    Start the Vite development server' \
		'  make lint                   Lint Web-entry tooling and Vue sources' \
		'  make test                   Run Vue unit and contract tests' \
		'  make build                  Build and pre-compress browser assets' \
		'  make verify-vite-env        Check the Vite environment boundary' \
		'  make build-web-entry        Build the standalone Web-entry contract binary' \
		'  make verify                 Run all frontend and Web-entry checks' \
		'  make build-image            Build the application image' \
		'' \
		'Use PLATFORM=linux/amd64 (or linux/arm64) for an explicit image platform.'

.PHONY: bootstrap
bootstrap:
	$(PNPM) install --frozen-lockfile --config.package-lock=true

.PHONY: dev
dev:
	$(PNPM) --filter hami-webui-web run dev

.PHONY: lint
lint:
	$(PNPM) run lint
	$(PNPM) --filter hami-webui-web run lint

.PHONY: test
test:
	$(PNPM) --filter hami-webui-web run test

.PHONY: build
build:
	$(PNPM) --filter hami-webui-web run build
	$(PNPM) run precompress:web-assets

.PHONY: verify-vite-env
verify-vite-env:
	$(PNPM) run verify:vite-env

.PHONY: build-web-entry
build-web-entry:
	$(MAKE) -C server build-web-entry

.PHONY: test-web-entry-contract
test-web-entry-contract:
	$(PNPM) run test:web-entry-contract

.PHONY: test-web-entry-browser
test-web-entry-browser:
	$(PNPM) run test:web-entry-browser

.PHONY: verify
verify: lint test verify-vite-env build build-web-entry
	$(MAKE) test-web-entry-contract
	$(MAKE) test-web-entry-browser

# This target creates one local development image. Stable multi-platform images
# are published only by .github/workflows/release.yaml.
.PHONY: build-image
build-image:
	$(DOCKER) build $(DOCKER_PLATFORM_FLAG) -t $(DOCKER_IMAGE):$(VERSION) .

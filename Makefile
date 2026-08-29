VERSION?=latest
DOCKER_IMAGE=projecthami/hami-webui-fe
PROJECT_NAME?=test-project

# 按项目最小化构建
ROUTE_FILE=packages/web/src/router/index.js
PROJECT_PATH=packages/web/projects/
DISABLED_PROJECTS?=""

.PHONY: install-modules
install-modules:
	pnpm install

.PHONY: build-all
build-all: install-modules build-web

.PHONY: build-web
build-web:
	pnpm --filter hami-webui-web run build

.PHONY: start-dev
start-dev: install-modules start-web


.PHONY: start-web
start-web:
	pnpm --filter hami-webui-web run start:dev

.PHONY: build-image
build-image:
	docker build --platform linux/amd64 -t ${DOCKER_IMAGE}:${VERSION} .

.PHONY: push-image
push-image:
	docker push ${DOCKER_IMAGE}:${VERSION}

.PHONY: release
release: build-image push-image

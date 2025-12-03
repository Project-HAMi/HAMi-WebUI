VERSION?=latest
DOCKER_IMAGE=projecthami/hami-webui-fe
OUT=./dist
PROJECT_NAME?=test-project

# 按项目最小化构建
ROUTE_FILE=packages/web/src/router/index.js
PROJECT_PATH=packages/web/projects/
DISABLED_PROJECTS?=""

.PHONY: install-modules
install-modules:
	yarn install

.PHONY: build-all
build-all: install-modules build-bff build-web

.PHONY: build-bff
build-bff:
	yarn run build

.PHONY: build-web
build-web:
	yarn workspace hami-webui-web run build

.PHONY: start-dev
start-dev: install-modules start-bff start-web


.PHONY: start-bff
start-bff:
	yarn run start:dev &

.PHONY: start-web
start-web:
	yarn workspace hami-webui-web run start:dev

.PHONY: start-prod
start-prod:
	yarn run start:prod

.PHONY: build-image
build-image:
	docker build --platform linux/amd64 -t ${DOCKER_IMAGE}:${VERSION} .

.PHONY: push-image
push-image:
	docker push ${DOCKER_IMAGE}:${VERSION}

.PHONY: release
release: build-image push-image
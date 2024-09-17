# Developer guide

This guide helps you get started developing HAMi-WebUI.

## Dependencies

Make sure you have the following dependencies installed before setting up your developer environment:

- [Git](https://git-scm.com/)
- [Go](https://golang.org/dl/) (see [go.mod](../../server/go.mod) for minimum required version)
- [Node.js](https://nodejs.org/), [Vue.js](https://vuejs.org/), [Element-UI](https://element.eleme.cn/)


### macOS

We recommend using [Homebrew](https://brew.sh/) for installing any missing dependencies:

```
brew install git
brew install go
brew install node@20
```

## Download HAMi-WebUI

We recommend using the Git command-line interface to download the source code for the HAMi-WebUI project:

1. Open a terminal and run `git clone https://github.com/Project-HAMi/HAMi-WebUI.git`. This command downloads HAMi-WebUI to a new `hami-webui` directory in your current directory.
2. Open the `HAMi-WebUI` directory in your favorite code editor.

For alternative ways of cloning the HAMi-WebUI repository, refer to [GitHub's documentation](https://docs.github.com/en/github/creating-cloning-and-archiving-repositories/cloning-a-repository).

## Build HAMi-WebUI

When building HAMi-WebUI, be aware that it consists of two components:

- The _backend_.
- The _frontend_.

### Backend

Build and run the backend by running `make run` in the `server` directory of the repository. This command compiles the Go source code and starts a backend server.

By default, you can access the web-ui-server-swagger at `http://localhost:8000/q/swagger-ui`.

### Frontend

Build and run the frontend by running `make start-dev` in the `root` directory of the repository. This command installs the related dependencies and starts a bff server and a frontend server.

By default, you can access the web-ui at `http://localhost:3000/`.

## Build a Docker image

To build a HAMi-WebUI Frontend Docker image, run:

```
make build-image DOCKER_IMAGE=projecthami/hami-webui-fe VERSION=dev
```

The resulting image will be tagged as `projecthami/hami-webui-fe:dev`.


To build a HAMi-WebUI Backend Docker image, run:

```
make build-image DOCKER_IMAGE=projecthami/hami-webui-be VERSION=dev
```

The resulting image will be tagged as `projecthami/hami-webui-be:dev`.

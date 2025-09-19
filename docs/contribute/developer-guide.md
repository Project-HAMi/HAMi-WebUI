# Developer guide

This guide helps you get started developing HAMi-WebUI.

## Dependencies

Make sure you have the following dependencies installed before setting up your developer environment:

- [Git](https://git-scm.com/)
- [Go](https://golang.org/dl/) (see [server/go.mod](../../server/go.mod) for minimum required version)
- [Node.js](https://nodejs.org/) (v20 recommended)
- [pnpm](https://pnpm.io/) (v10.x recommended)
- [Vue.js](https://vuejs.org/) 3.x
- [Element Plus](https://element-plus.org/)

### macOS

We recommend using [Homebrew](https://brew.sh/) for installing any missing dependencies:

```
brew install git
brew install go
brew install protobuf  # generate-proto need
brew install node@20
npm install -g pnpm
```

## Download HAMi-WebUI

We recommend using the Git command-line interface to download the source code for the HAMi-WebUI project:

1. Open a terminal and run `git clone https://github.com/Project-HAMi/HAMi-WebUI.git`. This command downloads HAMi-WebUI to a new `hami-webui` directory in your current directory.
2. Open the `HAMi-WebUI` directory in your favorite code editor.

For alternative ways of cloning the HAMi-WebUI repository, refer to [GitHub's documentation](https://docs.github.com/en/github/creating-cloning-and-archiving-repositories/cloning-a-repository).

## Project Structure

HAMi-WebUI consists of three main components:

- The _backend_ (Go): Located in the `server` directory
- The _BFF layer_ (NestJS): Located in the `src` directory
- The _frontend_ (Vue 3): Located in the `packages/web` directory

## Build HAMi-WebUI

### Backend

Build and run the backend by running `make run` in the `server` directory of the repository. This command compiles the Go source code and starts a backend server.

By default, you can access the web-ui-server-swagger at `http://localhost:8000/q/swagger-ui`.

### Frontend and BFF Layer

Build and run the frontend and BFF layer by running `make start-dev` in the `root` directory of the repository. This command installs the related dependencies and starts a BFF server and a frontend server.

By default, you can access the web-ui at `http://localhost:3000/`.

## Development Workflow

1. Create a feature branch:

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes and test them locally.

3. Commit your changes following the conventional commit format:

   ```bash
   git commit -m "feat: add new feature"
   # or
   git commit -m "fix: resolve issue with..."
   ```

4. Push your branch to GitHub and create a Pull Request.

## Build Docker Images

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

## Debugging

### Frontend Debugging

- Use Vue DevTools for Vue component debugging
- Use browser developer tools for network and performance analysis
- Check the console for errors and warnings

### Backend Debugging

- Check logs in the terminal where the backend is running
- Use Go debugging tools if necessary
- Verify API responses using Swagger UI

## Testing

Run tests for the frontend:

```bash
pnpm run test
```

Run tests for the backend:

```bash
cd server/
make run-tests
```

## Additional Resources

- [Contributing Guide](../../CONTRIBUTING.md)
- [Installation Guide](../installation/helm/index.md)
- [HAMi Project](https://github.com/Project-HAMi/HAMi)

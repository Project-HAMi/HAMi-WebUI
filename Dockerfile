FROM --platform=$BUILDPLATFORM node:21.6.2 AS builder

WORKDIR /src

# Cache dependencies
COPY package.json yarn.lock ./
COPY packages/web/package.json packages/web/
RUN yarn install --frozen-lockfile

COPY . .

# Build
RUN make build-bff build-web

FROM node:21.6.2-slim

COPY --from=builder /src/dist/ /apps/dist/
COPY --from=builder /src/node_modules/ /apps/node_modules/
COPY --from=builder /src/public/ /apps/public/

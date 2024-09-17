FROM node:21.6.2 AS builder

WORKDIR /src

RUN npm install -g pnpm

COPY . .

RUN make build-all

FROM node:21.6.2-slim

COPY --from=builder /src/dist/ /apps/dist/
COPY --from=builder /src/node_modules/ /apps/node_modules/
COPY --from=builder /src/public/ /apps/public/

FROM --platform=$BUILDPLATFORM node:24.20.0-bookworm@sha256:be23f54a88d34e8824c741b19b91064094f92c1c97b194144bfc8b50d67258e2 AS builder

WORKDIR /src

# Enable corepack to use pnpm version from package.json packageManager field
RUN corepack enable

# Copy dependency files for better layer caching
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/web/package.json packages/web/

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy source code
COPY . .

# Build application
RUN make build-bff build-web

# Remove devDependencies to reduce image size (only keep production dependencies)
RUN CI=true pnpm prune --prod

# Clean pnpm store cache to reduce image size
RUN pnpm store prune

FROM node:24.20.0-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e

# Set production environment
ENV NODE_ENV=production

# Create app directory and non-root user for security
WORKDIR /apps
RUN groupadd -r appuser && useradd -r -g appuser appuser

# Copy built artifacts from builder stage
COPY --from=builder --chown=appuser:appuser /src/dist/ ./dist/
COPY --from=builder --chown=appuser:appuser /src/node_modules/ ./node_modules/
COPY --from=builder --chown=appuser:appuser /src/public/ ./public/

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 3000

# Health check using the application's health_check endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/health_check', (r) => {process.exit(r.statusCode === 200 ? 0 : 1)})" || exit 1

# Start application
CMD ["node", "dist/main"]

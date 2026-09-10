FROM golang:1.27.0-bookworm AS go
FROM node:26.7.0-bookworm-slim AS node
FROM mcr.microsoft.com/playwright:v1.63.0-noble
COPY --from=go /usr/local/go /usr/local/go
COPY --from=node /usr/local/bin/node /usr/local/bin/node
ENV PATH="/usr/local/go/bin:${PATH}"
RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global pnpm@11.24.0
WORKDIR /work/web
ENV CI=true
CMD ["sh", "-c", "pnpm install --frozen-lockfile && pnpm typecheck:e2e && pnpm test:e2e"]

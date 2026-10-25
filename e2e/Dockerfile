ARG PLAYWRIGHT_VERSION
FROM golang:1.27.3-bookworm AS go
FROM node:26.12.0-bookworm-slim AS node
FROM mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-noble
COPY --from=go /usr/local/go /usr/local/go
COPY --from=node /usr/local/bin/node /usr/local/bin/node
ENV PATH="/usr/local/go/bin:${PATH}"
RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global pnpm@11.24.0
WORKDIR /work/e2e
ENV CI=true
CMD ["sh", "-c", "pnpm install --frozen-lockfile && pnpm check && pnpm test"]

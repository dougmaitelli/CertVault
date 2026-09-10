#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
docker build -f web/e2e/Dockerfile -t certvault-e2e web/e2e
docker run --rm --ipc=host \
  -v "$PWD:/work" \
  -v /work/web/node_modules \
  -v /work/web/dist \
  -v /work/.cache \
  certvault-e2e sh -c 'pnpm install --frozen-lockfile && pnpm typecheck:e2e && pnpm test:e2e "$@"' sh "$@"

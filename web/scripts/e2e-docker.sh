#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
PLAYWRIGHT_VERSION=$(node --input-type=commonjs -e '
  const { devDependencies } = require("./web/package.json");
  const version = devDependencies["@playwright/test"];
  if (!/^\d+\.\d+\.\d+$/.test(version) || version !== devDependencies.playwright) {
    throw new Error("Pin playwright and @playwright/test to the same exact version");
  }
  process.stdout.write(version);
')
docker build --build-arg "PLAYWRIGHT_VERSION=$PLAYWRIGHT_VERSION" -f web/e2e/Dockerfile -t certvault-e2e web/e2e
docker run --rm --ipc=host \
  -v "$PWD:/work" \
  -v /work/web/node_modules \
  -v /work/web/dist \
  -v /work/.cache \
  certvault-e2e sh -c 'pnpm install --frozen-lockfile && pnpm typecheck:e2e && pnpm test:e2e "$@"' sh "$@"

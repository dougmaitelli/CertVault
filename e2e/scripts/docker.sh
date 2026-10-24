#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
PLAYWRIGHT_VERSION=$(node --input-type=commonjs -e '
  const { devDependencies } = require("./e2e/package.json");
  const version = devDependencies["@playwright/test"];
  if (!/^\d+\.\d+\.\d+$/.test(version) || version !== devDependencies.playwright) {
    throw new Error("Pin playwright and @playwright/test to the same exact version");
  }
  process.stdout.write(version);
')
docker build --network "${CERTVAULT_E2E_NETWORK:-default}" --build-arg "PLAYWRIGHT_VERSION=$PLAYWRIGHT_VERSION" -f e2e/Dockerfile -t certvault-e2e e2e
docker run --rm --ipc=host --network "${CERTVAULT_E2E_NETWORK:-bridge}" \
  -v "$PWD:/work" \
  -v /work/web/node_modules \
  -v /work/web/dist \
  -v /work/e2e/node_modules \
  -v /work/.cache \
  certvault-e2e sh -c 'pnpm --dir ../web install --frozen-lockfile && pnpm install --frozen-lockfile && pnpm check && pnpm test "$@"' sh "$@"

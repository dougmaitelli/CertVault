#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
playwright_version=$(node -e "process.stdout.write(require('$root/web/node_modules/playwright/package.json').version)")
image="mcr.microsoft.com/playwright:v${playwright_version}-noble"

echo "Using image: $image"

exec docker run --rm \
  --ipc=host \
  -v "$root:/work" \
  -w /work/web \
  -e PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
  "$image" \
  node scripts/take-screenshots.mjs

GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := $(CURDIR)/.bin/golangci-lint
GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
export GOLANGCI_LINT_CACHE

SHFMT_VERSION := v3.14.1
SHFMT := $(CURDIR)/.bin/shfmt
SHFMT_FLAGS := -ln posix -i 2 -ci
SHELL_SOURCES := $(wildcard web/client/*.sh web/scripts/*.sh)

.PHONY: dependencies dependencies-backend dependencies-frontend screenshots screenshots-docker \
	build build-backend build-frontend test test-backend test-frontend test-e2e test-e2e-docker \
	format format-backend format-frontend format-shell format-check \
	format-check-backend format-check-frontend format-check-shell lint lint-backend lint-frontend \
	vet check check-app tools clean docker config-check dev \
	docs-install docs-dev docs-check docs-build docs-preview format-shared format-check-shared format-check-docs

DEV_DIR := $(CURDIR)/.cache/dev
DEV_CONFIG := $(CURDIR)/config/config.dev.yaml
DEV_MASTER_KEY := $(DEV_DIR)/master-key
DEV_ADMIN_TOKEN := $(DEV_DIR)/admin-token

build: build-backend build-frontend

dependencies: dependencies-backend dependencies-frontend
	pnpm --dir e2e install --frozen-lockfile

dependencies-backend:
	cd backend && go mod download

dependencies-frontend:
	cd web && pnpm install --frozen-lockfile

build-backend:
	cd backend && go build -buildvcs=false -o ../certvault .

build-frontend:
	cd web && pnpm run build

test: test-backend test-frontend

test-backend:
	cd backend && go test ./...

test-frontend:
	cd web && pnpm run test

format: format-backend format-frontend format-shared format-shell

format-shared:
	cd web && pnpm exec prettier --write '../*.{md,yaml,yml}' '../.github/**/*.{yml,yaml}' '../config/*.{yml,yaml}' '../{docs,e2e}/**/*.{md,mdx,mjs,ts,css,json,yaml,yml}' --ignore-path ../.prettierignore

format-backend:
	gofmt -w backend

format-frontend:
	cd web && pnpm run format

format-shell: $(SHFMT)
	$(SHFMT) $(SHFMT_FLAGS) -w $(SHELL_SOURCES)

format-check: format-check-backend format-check-frontend format-check-shared format-check-docs format-check-shell

format-check-shared:
	cd web && pnpm exec prettier --check '../*.{yaml,yml}' '../.github/**/*.{yml,yaml}' '../config/*.{yml,yaml}' '../e2e/**/*.{md,mdx,mjs,ts,css,json,yaml,yml}' --ignore-path ../.prettierignore

format-check-docs:
	cd web && pnpm exec prettier --check '../*.md' '../docs/**/*.{md,mdx,mjs,ts,css,json,yaml,yml}' --ignore-path ../.prettierignore

format-check-backend:
	test -z "$$(gofmt -l backend)"

format-check-frontend:
	cd web && pnpm run format:check

format-check-shell: $(SHFMT)
	$(SHFMT) $(SHFMT_FLAGS) -d $(SHELL_SOURCES)

lint: lint-backend lint-frontend

lint-backend: $(GOLANGCI_LINT)
	cd backend && $(GOLANGCI_LINT) run ./...

lint-frontend:
	cd web && pnpm run lint

vet:
	cd backend && go vet ./...

check: check-app format-check-docs

check-app: format-check-backend format-check-frontend format-check-shared format-check-shell vet lint test build

tools: $(GOLANGCI_LINT) $(SHFMT)

$(GOLANGCI_LINT):
	mkdir -p $(dir $(GOLANGCI_LINT))
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(dir $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION)

$(SHFMT):
	mkdir -p $(dir $(SHFMT))
	GOBIN=$(dir $(SHFMT)) go install mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION)

clean:
	cd backend && go clean
	rm -f certvault
	rm -rf web/dist
	rm -rf .cache

screenshots:
	pnpm --dir e2e screenshots

screenshots-docker:
	pnpm --dir e2e screenshots

docker:
	docker compose build

docs-install:
	pnpm --dir docs install --frozen-lockfile

docs-dev:
	@echo "CertVault docs: http://localhost:4321/CertVault/"
	pnpm --dir docs dev

docs-check:
	pnpm --dir docs check

docs-build:
	pnpm --dir docs build

docs-preview:
	pnpm --dir docs preview

config-check:
	cd backend && go run . check-config --config ../config/config.yaml

dev: dependencies
	mkdir -p $(DEV_DIR)/data
	test -f $(DEV_MASTER_KEY) || openssl rand -base64 32 > $(DEV_MASTER_KEY)
	test -f $(DEV_ADMIN_TOKEN) || printf '%s\n' 'certvault-dev-admin' > $(DEV_ADMIN_TOKEN)
	@echo "CertVault UI: http://localhost:8081"
	@echo "Bootstrap token: certvault-dev-admin"
	@set -eu; \
		cd backend; \
		CERTVAULT_MASTER_KEY_FILE=$(DEV_MASTER_KEY) \
		CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE=$(DEV_ADMIN_TOKEN) \
		go run . -config $(DEV_CONFIG) & \
		backend_pid=$$!; \
		trap 'kill $$backend_pid 2>/dev/null || true' EXIT INT TERM; \
		cd ../web; \
		pnpm run dev

test-e2e:
	pnpm --dir e2e check
	pnpm --dir e2e test

test-e2e-docker:
	sh e2e/scripts/docker.sh

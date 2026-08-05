GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := $(CURDIR)/.bin/golangci-lint
GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
export GOLANGCI_LINT_CACHE

.PHONY: dependencies dependencies-backend dependencies-frontend screenshots screenshots-docker \
	build build-backend build-frontend test test-backend test-frontend \
	format format-backend format-frontend format-check \
	format-check-backend format-check-frontend lint lint-backend lint-frontend \
	vet check tools clean docker config-check dev docs-dev

DEV_DIR := $(CURDIR)/.cache/dev
DEV_CONFIG := $(CURDIR)/config/config.dev.yaml
DEV_MASTER_KEY := $(DEV_DIR)/master-key
DEV_ADMIN_TOKEN := $(DEV_DIR)/admin-token

build: build-backend build-frontend

dependencies: dependencies-backend dependencies-frontend

dependencies-backend:
	cd backend && go mod download

dependencies-frontend:
	cd web && npm ci

build-backend:
	cd backend && go build -buildvcs=false -o ../certvault .

build-frontend:
	cd web && npm run build

test: test-backend test-frontend

test-backend:
	cd backend && go test ./...

test-frontend:
	cd web && npm run test

format: format-backend format-frontend

format-backend:
	gofmt -w backend

format-frontend:
	cd web && npm run format

format-check: format-check-backend format-check-frontend

format-check-backend:
	test -z "$$(gofmt -l backend)"

format-check-frontend:
	cd web && npm run format:check

lint: lint-backend lint-frontend

lint-backend: $(GOLANGCI_LINT)
	cd backend && $(GOLANGCI_LINT) run ./...

lint-frontend:
	cd web && npm run lint

vet:
	cd backend && go vet ./...

check: format-check vet lint test build

tools: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	mkdir -p $(dir $(GOLANGCI_LINT))
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(dir $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION)

clean:
	cd backend && go clean
	rm -f certvault
	rm -rf web/dist
	rm -rf .cache

screenshots:
	cd web && npm run screenshots

screenshots-docker:
	cd web && npm run screenshots:docker

docker:
	docker compose build

docs-dev:
	@echo "CertVault docs: http://localhost:8000"
	docker run --rm -it -p 8000:8000 -v "$(CURDIR):/docs" squidfunk/mkdocs-material

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
		npm run dev

GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := $(CURDIR)/.bin/golangci-lint
GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
export GOLANGCI_LINT_CACHE

.PHONY: dependencies dependencies-backend dependencies-frontend \
	build build-backend build-frontend test test-backend test-frontend \
	format format-backend format-frontend format-check \
	format-check-backend format-check-frontend lint lint-backend lint-frontend \
	vet check tools clean docker config-check

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
	gofmt -w backend/main.go backend/api backend/config backend/service backend/store backend/vault

format-frontend:
	cd web && npm run format

format-check: format-check-backend format-check-frontend

format-check-backend:
	test -z "$$(gofmt -l backend/main.go backend/api backend/config backend/service backend/store backend/vault)"

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

docker:
	docker compose build

config-check:
	cd backend && go run . -config ../config/config.yaml -check-config

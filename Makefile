.PHONY: build test lint generate vet goafl ci

GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

build:
	go build -o bin/ ./...

goafl:
	go build -o bin/goafl ./cmd/goafl

test:
	go test ./... -race -count=1

lint:
	$(GOLANGCI_LINT) run ./...

vet:
	go vet ./...

generate:
	go generate ./modelsdev/...

coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out

ci: vet lint test build
	@echo "CI checks passed"

.PHONY: build test lint generate vet goafl ci

build:
	go build -o bin/ ./...

goafl:
	go build -o bin/goafl ./cmd/goafl

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

vet:
	go vet ./...

generate:
	go generate ./modelsdev/...

coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out

ci: vet lint test build
	@echo "CI checks passed"

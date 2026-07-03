#!/bin/bash
set -euo pipefail

echo "=== Building ==="
go build ./...

echo "=== Running go vet ==="
go vet ./...

echo "=== Running tests ==="
go test ./... -race -count=1

echo "=== All checks passed ==="

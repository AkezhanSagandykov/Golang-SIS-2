#!/usr/bin/env bash
# Quality gate for TaskFlow: the checks AGENTS.md section 7 requires, in one command.
# Exits with a non-zero code on the first failure. CI runs exactly this script.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}" # never download a toolchain silently

echo "== $(go version)  commit $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

echo "== gofmt -l ."
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "FAIL: files are not gofmt-formatted:"
  echo "$unformatted"
  exit 1
fi

echo "== go vet ./..."
go vet ./...

echo "== go build ./..."
go build ./...

echo "== go test -count=1 -race ./..."
go test -count=1 -race ./...

echo "== go test -count=1 -cover ./..."
go test -count=1 -cover ./...

echo "== OK: all checks passed"

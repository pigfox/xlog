#!/usr/bin/env bash
set -euo pipefail

# Run unit tests with race detector
go test ./... -race

# Run coverage across all packages (including cmd/*)
go test ./... -coverpkg=./... -coverprofile=cover.out

# Print total coverage
total=$(go tool cover -func=cover.out | tail -n 1 | awk '{print $3}' | tr -d '%')
echo "TOTAL COVERAGE: ${total}%"

# Enforce threshold
min=80.0
awk -v t="$total" -v min="$min" 'BEGIN { if (t+0 < min+0) exit 1 }' || {
  echo "Coverage ${total}% is below ${min}%"
  exit 1
}

echo "OK"

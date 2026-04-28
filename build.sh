#!/usr/bin/env bash
# Build script per Linux/macOS. Produce ./planck nella radice.
set -e
go build -o planck -trimpath -ldflags="-s -w" ./cmd/planck
echo "Built ./planck"

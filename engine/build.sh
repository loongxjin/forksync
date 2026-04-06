#!/bin/bash
set -e

echo "Building ForkSync engine..."

cd "$(dirname "$0")"

# Build for current platform
echo "Building for $(go env GOOS)/$(go env GOARCH)..."
go build -o bins/forksync .

echo "Build complete: bins/forksync"

#!/bin/bash
set -e

cd "$(dirname "$0")"

VERSION=$(grep '"version"' ../app/package.json | head -1 | sed 's/.*: "//;s/".*//')
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS="-X github.com/loongxjin/forksync/engine/pkg/version.Version=${VERSION} \
         -X github.com/loongxjin/forksync/engine/pkg/version.Commit=${COMMIT} \
         -X github.com/loongxjin/forksync/engine/pkg/version.BuildDate=${BUILD_DATE}"

echo "Building ForkSync engine v${VERSION}..."
echo "Building for $(go env GOOS)/$(go env GOARCH)..."
go build -ldflags "${LDFLAGS}" -o bins/forksync .
echo "Build complete: bins/forksync"

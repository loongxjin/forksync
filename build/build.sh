#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Read version from app/package.json
VERSION=$(grep '"version"' "$ROOT_DIR/app/package.json" | head -1 | sed 's/.*: "//;s/".*//')
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS="-X github.com/loongxjin/forksync/engine/pkg/version.Version=${VERSION} \
         -X github.com/loongxjin/forksync/engine/pkg/version.Commit=${COMMIT} \
         -X github.com/loongxjin/forksync/engine/pkg/version.BuildDate=${BUILD_DATE}"

echo "=== ForkSync Build v${VERSION} ==="
echo ""

# Step 1: Build Go engine
echo "Building Go engine..."
cd "$ROOT_DIR/engine"
CGO_ENABLED=0 go build -ldflags "${LDFLAGS}" -o "$ROOT_DIR/build/forksync" .
echo "Go engine built: build/forksync"

# Step 2: Install dependencies
echo ""
echo "Installing Electron dependencies..."
cd "$ROOT_DIR/app"
npm ci

# Step 3: Build Electron app
echo ""
echo "Building Electron app..."
npm run build

# Step 4: Package
echo ""
echo "Packaging..."
npx electron-builder

echo ""
echo "Build complete v${VERSION}! Output in app/dist/"

#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "=== ForkSync Build ==="
echo ""

# Step 1: Build Go engine
echo "📦 Building Go engine..."
cd "$ROOT_DIR/engine"
CGO_ENABLED=0 go build -o "$ROOT_DIR/build/forksync" .
echo "✅ Go engine built: build/forksync"

# Step 2: Install dependencies
echo ""
echo "📦 Installing Electron dependencies..."
cd "$ROOT_DIR/app"
npm ci

# Step 3: Build Electron app
echo ""
echo "📦 Building Electron app..."
npm run build

# Step 4: Package
echo ""
echo "📦 Packaging..."
npx electron-builder

echo ""
echo "✅ Build complete! Output in app/dist/"

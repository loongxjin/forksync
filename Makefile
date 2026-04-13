VERSION := $(shell cat app/package.json | grep '"version"' | head -1 | sed 's/.*: "//;s/".*//')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X github.com/loongxjin/forksync/engine/pkg/version.Version=$(VERSION) \
           -X github.com/loongxjin/forksync/engine/pkg/version.Commit=$(COMMIT) \
           -X github.com/loongxjin/forksync/engine/pkg/version.BuildDate=$(BUILD_DATE)

.PHONY: help build engine app release clean tag

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build Go engine + Electron app (full desktop build)
	@echo "=== Building ForkSync v$(VERSION) ==="
	@$(MAKE) engine
	@$(MAKE) app

engine: ## Build Go engine only
	@echo "Building Go engine v$(VERSION)..."
	cd engine && CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../build/forksync .
	@echo "Built: build/forksync"

app: ## Build Electron app only (requires engine already built)
	@echo "Building Electron app..."
	cd app && npm ci && npm run build && npx electron-builder
	@echo "Built: app/dist/"

clean: ## Clean build artifacts
	rm -rf build/forksync engine/bins/ app/dist/ app/out/

# Usage: make release-tag VERSION=0.2.0
# 1. Syncs app/package.json version from VERSION
# 2. Creates git tag vVERSION
# 3. Pushes the commit and tag (triggers GitHub Actions)
release-tag: ## Tag and push a release. Usage: make release-tag VERSION=0.2.0
	@test "$(VERSION)" || (echo "Usage: make release-tag VERSION=0.2.0" && exit 1)
	@echo "Syncing version to $(VERSION)..."
	cd app && npm version $(VERSION) --no-git-tag-version
	git add app/package.json app/package-lock.json
	git commit -m "chore: bump version to v$(VERSION)" --allow-empty
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	@echo ""
	@echo "Tag v$(VERSION) created. Run to push:"
	@echo "  git push origin main --tags"

release: ## Run GoReleaser locally (requires goreleaser installed)
	@echo "Releasing v$(VERSION)..."
	cd engine && goreleaser release --clean

release-snapshot: ## Run GoReleaser in snapshot mode (for testing)
	cd engine && goreleaser release --snapshot --clean

version: ## Print current version
	@echo "Version:  $(VERSION)"
	@echo "Commit:   $(COMMIT)"
	@echo "Date:     $(BUILD_DATE)"

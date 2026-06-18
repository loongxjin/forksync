VERSION := $(shell cat frontend/package.json | grep '"version"' | head -1 | sed 's/.*: "//;s/".*//')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X github.com/loongxjin/forksync/engine/pkg/version.Version=$(VERSION) \
           -X github.com/loongxjin/forksync/engine/pkg/version.Commit=$(COMMIT) \
           -X github.com/loongxjin/forksync/engine/pkg/version.BuildDate=$(BUILD_DATE)

.PHONY: help build wails wails-dev wails-dmg wails-pkg clean tag

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

wails: ## Build Wails app (single binary, ~18MB)
	@echo "=== Building ForkSync v$(VERSION) (Wails) ==="
	wails build

wails-dev: ## Run Wails dev server (hot reload)
	wails dev

wails-dmg: wails ## Build .app then package as .dmg installer with drag-to-Applications (macOS)
	@echo "=== Packaging ForkSync v$(VERSION) as DMG ==="
	@bash build/dmg.sh $(VERSION)

wails-pkg: wails ## Build .app then package as .pkg installer (macOS)
	@echo "=== Packaging ForkSync v$(VERSION) as PKG ==="
	pkgbuild --root build/bin \
		--identifier com.forksync.app \
		--version $(VERSION) \
		--install-location /Applications \
		build/bin/ForkSync-$(VERSION).pkg
	@echo "Built: build/bin/ForkSync-$(VERSION).pkg"

wails-nsis: wails ## Build .exe then package as NSIS installer (Windows)
	@echo "=== Packaging ForkSync v$(VERSION) as NSIS ==="
	@echo "Requires: makensis in PATH (install via 'choco install nsis')"
	makensis -DVERSION=$(VERSION) build/windows/installer.nsi
	@echo "Built: build/bin/ForkSync-Setup-$(VERSION).exe"

clean: ## Clean build artifacts
	rm -rf build/bin/ build/forksync engine/bins/

# Usage: make release-tag VERSION=0.6.0
release-tag: ## Tag and push a release. Usage: make release-tag VERSION=0.6.0
	@test "$(VERSION)" || (echo "Usage: make release-tag VERSION=0.6.0" && exit 1)
	@echo "Syncing version to $(VERSION)..."
	cd frontend && npm version $(VERSION) --no-git-tag-version
	git add frontend/package.json frontend/package-lock.json
	git commit -m "chore: bump version to v$(VERSION)"
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	@echo ""
	@echo "Tag v$(VERSION) created. Run to push:"
	@echo "  git push origin feature/wails --tags"

version: ## Print current version
	@echo "Version:  $(VERSION)"
	@echo "Commit:   $(COMMIT)"
	@echo "Date:     $(BUILD_DATE)"

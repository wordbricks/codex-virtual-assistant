.PHONY: help dev dev-yolo release-version release-verify release-assets release-tag release-gh release-npm release-manual release

SHELL := /bin/zsh

VERSION ?=
DIST_DIR := dist
NPM_DIR := npm
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

help:
	@echo "Development targets:"
	@echo "  make dev                             # run local server via go run"
	@echo "  make dev-yolo                        # run local server with --yolo"
	@echo ""
	@echo "Release targets:"
	@echo "  make release-version VERSION=0.1.0   # sync npm/package.json version"
	@echo "  make release-verify                  # go/node/npm dry-run checks"
	@echo "  make release-assets VERSION=0.1.0    # build release binaries into dist/"
	@echo "  make release-tag VERSION=0.1.0       # create and push git tag vVERSION"
	@echo "  make release-gh VERSION=0.1.0        # create GitHub release with dist assets"
	@echo "  make release-npm                     # publish npm package from npm/"
	@echo "  make release-manual VERSION=0.1.0    # run version sync, verify, and asset build"
	@echo "  make release VERSION=0.1.0           # full manual release flow after npm login"

dev:
	go run ./cmd/cva start

dev-yolo:
	go run ./cmd/cva start --yolo

release-version:
	@if [[ -z "$(VERSION)" ]]; then echo "VERSION is required, for example: make $@ VERSION=0.1.0"; exit 1; fi
	cd $(NPM_DIR) && npm version $(VERSION) --no-git-tag-version

release-verify:
	go test ./...
	node --check $(NPM_DIR)/bin/cva.js
	node --check $(NPM_DIR)/lib/install.js
	node --check $(NPM_DIR)/lib/platform.js
	node --test $(NPM_DIR)/lib/install.test.js
	cd $(NPM_DIR) && npm pack --dry-run

release-assets:
	@if [[ -z "$(VERSION)" ]]; then echo "VERSION is required, for example: make $@ VERSION=0.1.0"; exit 1; fi
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o $(DIST_DIR)/cva-linux-x64 ./cmd/cva
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o $(DIST_DIR)/cva-linux-arm64 ./cmd/cva
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o $(DIST_DIR)/cva-darwin-x64 ./cmd/cva
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o $(DIST_DIR)/cva-darwin-arm64 ./cmd/cva
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o $(DIST_DIR)/cva-win32-x64.exe ./cmd/cva
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o $(DIST_DIR)/cva-win32-arm64.exe ./cmd/cva

release-tag:
	@if [[ -z "$(VERSION)" ]]; then echo "VERSION is required, for example: make $@ VERSION=0.1.0"; exit 1; fi
	git tag v$(VERSION)
	git push origin v$(VERSION)

release-gh:
	@if [[ -z "$(VERSION)" ]]; then echo "VERSION is required, for example: make $@ VERSION=0.1.0"; exit 1; fi
	gh release create v$(VERSION) \
		$(DIST_DIR)/cva-linux-x64 \
		$(DIST_DIR)/cva-linux-arm64 \
		$(DIST_DIR)/cva-darwin-x64 \
		$(DIST_DIR)/cva-darwin-arm64 \
		$(DIST_DIR)/cva-win32-x64.exe \
		$(DIST_DIR)/cva-win32-arm64.exe \
		--title "v$(VERSION)" \
		--notes "Release v$(VERSION)"

release-npm:
	cd $(NPM_DIR) && npm publish --access public

release-manual: release-version release-verify release-assets
	@echo ""
	@echo "Next steps:"
	@echo "  make release-tag VERSION=$(VERSION)"
	@echo "  make release-gh VERSION=$(VERSION)"
	@echo "  make release-npm"

release:
	@if [[ -z "$(VERSION)" ]]; then echo "VERSION is required, for example: make $@ VERSION=0.1.0"; exit 1; fi
	$(MAKE) release-version VERSION=$(VERSION)
	$(MAKE) release-verify
	$(MAKE) release-assets VERSION=$(VERSION)
	$(MAKE) release-tag VERSION=$(VERSION)
	$(MAKE) release-gh VERSION=$(VERSION)
	$(MAKE) release-npm

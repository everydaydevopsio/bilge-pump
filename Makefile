# Root Makefile - builds all applications
.PHONY: all dev-cache git-cleaner bilge-macos bilge bilge-linux bilge-windows clean test fmt lint vet build release-dry-run release-snapshot goreleaser-check check-tools help

HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)

ifeq ($(HOST_GOOS),darwin)
HOST_PLATFORM_CLEANER := bilge-macos
HOST_BIN_SUFFIX :=
else ifeq ($(HOST_GOOS),linux)
HOST_PLATFORM_CLEANER := bilge-linux
HOST_BIN_SUFFIX :=
else ifeq ($(HOST_GOOS),windows)
HOST_PLATFORM_CLEANER := bilge-windows
HOST_BIN_SUFFIX := .exe
else
$(error Unsupported host OS: $(HOST_GOOS))
endif

# Default target - build all applications
all: dev-cache git-cleaner bilge-macos bilge bilge-linux bilge-windows

# Help target
help:
	@echo "Available targets:"
	@echo ""
	@echo "Setup:"
	@echo "  check-tools         - Verify all required development tools are installed"
	@echo ""
	@echo "Build targets:"
	@echo "  all                 - Build all applications (default)"
	@echo "  dev-cache           - Build dev-cache application"
	@echo "  git-cleaner         - Build git-cleaner application"
	@echo "  bilge-macos   - Build bilge-macos application"
	@echo "  bilge       - Build unified bilge command"
	@echo "  bilge-linux - Build Linux cleanup tool"
	@echo "  bilge-windows - Build Windows cleanup tool"
	@echo "  build               - Build all applications"
	@echo ""
	@echo "Quality targets:"
	@echo "  test                - Run tests in all applications"
	@echo "  fmt                 - Format code in all applications"
	@echo "  lint                - Lint all applications"
	@echo "  vet                 - Run go vet on all applications"
	@echo ""
	@echo "Release targets:"
	@echo "  goreleaser-check    - Validate GoReleaser configuration"
	@echo "  release-dry-run     - Test GoReleaser without building/publishing"
	@echo "  release-snapshot    - Build snapshot release locally (no git tag required)"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean               - Clean all applications and build artifacts"

# Build each application
dev-cache:
	@echo "Building dev-cache..."
	@$(MAKE) -C dev-cache all

git-cleaner:
	@echo "Building git-cleaner..."
	@$(MAKE) -C git-cleaner all

bilge-macos:
	@echo "Building bilge-macos..."
	@$(MAKE) -C bilge-macos all

bilge:
	@mkdir -p build
	@cd dev-cache && go build -o ../build/dev-cache$(HOST_BIN_SUFFIX) .
	@cd git-cleaner && go build -o ../build/git-cleaner$(HOST_BIN_SUFFIX) .
ifeq ($(HOST_GOOS),darwin)
	@cd bilge-macos && go build -o ../build/bilge-macos .
else
	@GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) go build -o build/$(HOST_PLATFORM_CLEANER)$(HOST_BIN_SUFFIX) ./cmd/platform-bilge
endif
	@go build -o build/bilge$(HOST_BIN_SUFFIX) ./cmd/bilge

bilge-linux:
	@mkdir -p build/cross/linux/amd64
	@GOOS=linux GOARCH=amd64 go build -o build/cross/linux/amd64/bilge-linux ./cmd/platform-bilge

bilge-windows:
	@mkdir -p build/cross/windows/amd64
	@GOOS=windows GOARCH=amd64 go build -o build/cross/windows/amd64/bilge-windows.exe ./cmd/platform-bilge

# Convenience targets that run in all directories
test:
	@echo "Running tests in all applications..."
	@$(MAKE) -C dev-cache test
	@$(MAKE) -C git-cleaner test
	@$(MAKE) -C bilge-macos test
	@go test ./cmd/...
	@sh scripts/test-install.sh

fmt:
	@echo "Formatting all applications..."
	@$(MAKE) -C dev-cache fmt
	@$(MAKE) -C git-cleaner fmt
	@$(MAKE) -C bilge-macos fmt

lint:
	@echo "Linting all applications..."
	@$(MAKE) -C dev-cache lint
	@$(MAKE) -C git-cleaner lint
	@$(MAKE) -C bilge-macos lint

vet:
	@echo "Running go vet on all applications..."
	@$(MAKE) -C dev-cache vet
	@$(MAKE) -C git-cleaner vet
	@$(MAKE) -C bilge-macos vet

build:
	@echo "Building all applications..."
	@$(MAKE) -C dev-cache build
	@$(MAKE) -C git-cleaner build
	@$(MAKE) -C bilge-macos build
	@$(MAKE) bilge bilge-linux bilge-windows

clean:
	@echo "Cleaning all applications..."
	@$(MAKE) -C dev-cache clean
	@$(MAKE) -C git-cleaner clean
	@$(MAKE) -C bilge-macos clean
	@rm -rf build

# Check required tools are installed
check-tools:
	@echo "Checking required development tools..."
	@echo ""
	@echo "Checking Go..."
	@if ! command -v go >/dev/null 2>&1; then \
		echo "❌ Go is not installed"; \
		echo "   Install from: https://go.dev/dl/"; \
		exit 1; \
	fi
	@go_version=$$(go version | awk '{print $$3}' | sed 's/go//'); \
	required_version="1.21"; \
	if ! printf '%s\n' "$$required_version" "$$go_version" | sort -V -C 2>/dev/null; then \
		echo "❌ Go version $$go_version is installed, but version $$required_version or higher is required"; \
		echo "   Install from: https://go.dev/dl/"; \
		exit 1; \
	fi
	@echo "✅ Go $$(go version | awk '{print $$3}') is installed"
	@echo ""
	@echo "Checking Make..."
	@if ! command -v make >/dev/null 2>&1; then \
		echo "❌ Make is not installed"; \
		echo "   On macOS: xcode-select --install"; \
		echo "   On Linux: apt-get install build-essential (Ubuntu/Debian) or yum groupinstall 'Development Tools' (RHEL/CentOS)"; \
		exit 1; \
	fi
	@echo "✅ Make $$(make --version | head -n1) is installed"
	@echo ""
	@echo "Checking optional tools..."
	@if command -v goreleaser >/dev/null 2>&1; then \
		echo "✅ GoReleaser $$(goreleaser --version | head -n1 | awk '{print $$3}') is installed"; \
	else \
		echo "⚠️  GoReleaser is not installed (optional, needed for releases)"; \
		echo "   Install with: brew install goreleaser"; \
	fi
	@if command -v pre-commit >/dev/null 2>&1; then \
		echo "✅ pre-commit $$(pre-commit --version | awk '{print $$2}') is installed"; \
	else \
		echo "⚠️  pre-commit is not installed (optional, needed for Git hooks)"; \
		echo "   Install with: pip install pre-commit"; \
	fi
	@echo ""
	@echo "All required tools are installed! 🎉"

# GoReleaser targets
goreleaser-check:
	@echo "Checking GoReleaser configuration..."
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "Error: goreleaser is not installed"; \
		echo "Install it with: brew install goreleaser"; \
		exit 1; \
	fi
	@echo "Note: Some deprecation warnings are expected and will be addressed in future versions."
	@goreleaser check

release-dry-run: goreleaser-check
	@echo "Running GoReleaser in dry-run mode (no release, no build)..."
	@goreleaser release --skip=publish --skip=validate --clean

release-snapshot: goreleaser-check
	@echo "Building snapshot release (local testing, no tags required)..."
	@goreleaser release --snapshot --clean --skip=publish
	@echo ""
	@echo "Snapshot built successfully!"
	@echo "Binaries are in: ./dist/"
	@ls -lh ./dist/ | grep -E "dev-cache|git-cleaner|bilge-macos"

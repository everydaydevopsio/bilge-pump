# Root Makefile - builds all applications
.PHONY: all dev-cache git-cleaner mac-cache-cleaner cache-cleaner linux-cache-cleaner windows-cache-cleaner clean test fmt lint vet build release-dry-run release-snapshot goreleaser-check check-tools help

# Default target - build all applications
all: dev-cache git-cleaner mac-cache-cleaner cache-cleaner linux-cache-cleaner windows-cache-cleaner

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
	@echo "  mac-cache-cleaner   - Build mac-cache-cleaner application"
	@echo "  cache-cleaner       - Build unified cache-cleaner command"
	@echo "  linux-cache-cleaner - Build Linux cleanup tool"
	@echo "  windows-cache-cleaner - Build Windows cleanup tool"
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

mac-cache-cleaner:
	@echo "Building mac-cache-cleaner..."
	@$(MAKE) -C mac-cache-cleaner all

cache-cleaner:
	@mkdir -p build
	@go build -o build/cache-cleaner ./cmd/cache-cleaner

linux-cache-cleaner:
	@mkdir -p build
	@GOOS=linux GOARCH=amd64 go build -o build/linux-cache-cleaner ./cmd/platform-cache-cleaner

windows-cache-cleaner:
	@mkdir -p build
	@GOOS=windows GOARCH=amd64 go build -o build/windows-cache-cleaner.exe ./cmd/platform-cache-cleaner

# Convenience targets that run in all directories
test:
	@echo "Running tests in all applications..."
	@$(MAKE) -C dev-cache test
	@$(MAKE) -C git-cleaner test
	@$(MAKE) -C mac-cache-cleaner test

fmt:
	@echo "Formatting all applications..."
	@$(MAKE) -C dev-cache fmt
	@$(MAKE) -C git-cleaner fmt
	@$(MAKE) -C mac-cache-cleaner fmt

lint:
	@echo "Linting all applications..."
	@$(MAKE) -C dev-cache lint
	@$(MAKE) -C git-cleaner lint
	@$(MAKE) -C mac-cache-cleaner lint

vet:
	@echo "Running go vet on all applications..."
	@$(MAKE) -C dev-cache vet
	@$(MAKE) -C git-cleaner vet
	@$(MAKE) -C mac-cache-cleaner vet

build:
	@echo "Building all applications..."
	@$(MAKE) -C dev-cache build
	@$(MAKE) -C git-cleaner build
	@$(MAKE) -C mac-cache-cleaner build
	@$(MAKE) cache-cleaner linux-cache-cleaner windows-cache-cleaner

clean:
	@echo "Cleaning all applications..."
	@$(MAKE) -C dev-cache clean
	@$(MAKE) -C git-cleaner clean
	@$(MAKE) -C mac-cache-cleaner clean
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
	@ls -lh ./dist/ | grep -E "dev-cache|git-cleaner|mac-cache-cleaner"

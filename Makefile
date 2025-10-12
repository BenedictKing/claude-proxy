# Claude Proxy - Unified Build System
# ==============================================================================

.DEFAULT_GOAL := help

# ==============================================================================
# Variables
# ==============================================================================
BACKEND_DIR := backend-go
FRONTEND_DIR := frontend
DIST_DIR := $(BACKEND_DIR)/dist
BUILD_SCRIPT := $(BACKEND_DIR)/build.sh

# Version information - read from VERSION file
VERSION := $(shell cat VERSION 2>/dev/null || echo "v0.0.0-dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S_UTC')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)

# Binary names
BINARY_PREFIX := claude-proxy
BINARY_LINUX_AMD64 := $(BINARY_PREFIX)-linux-amd64
BINARY_LINUX_ARM64 := $(BINARY_PREFIX)-linux-arm64
BINARY_DARWIN_AMD64 := $(BINARY_PREFIX)-darwin-amd64
BINARY_DARWIN_ARM64 := $(BINARY_PREFIX)-darwin-arm64
BINARY_WINDOWS_AMD64 := $(BINARY_PREFIX)-windows-amd64.exe

# Colors for output
COLOR_RESET := \033[0m
COLOR_CYAN := \033[36m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m

# 前端构建标记文件（用于缓存检测）
FRONTEND_BUILD_MARKER := $(BACKEND_DIR)/frontend/dist/.build-marker

# ==============================================================================
# Run & Development
# ==============================================================================
.PHONY: run
run: ensure-frontend-built ## Build frontend (if needed) and run Go server
	@echo "$(COLOR_GREEN)--- Starting Go server... ---$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go run -ldflags "$(LDFLAGS)" .

# 智能前端构建（仅在源文件变更时重新编译）
.PHONY: ensure-frontend-built
ensure-frontend-built:
	@if [ ! -f "$(FRONTEND_BUILD_MARKER)" ]; then \
		echo "$(COLOR_CYAN)📦 首次构建前端...$(COLOR_RESET)"; \
		$(MAKE) build-frontend-internal; \
	else \
		LATEST_SRC=$$(find $(FRONTEND_DIR)/src -type f -newer $(FRONTEND_BUILD_MARKER) 2>/dev/null | head -1); \
		if [ -n "$$LATEST_SRC" ]; then \
			echo "$(COLOR_CYAN)📦 检测到前端源文件变更，重新构建...$(COLOR_RESET)"; \
			$(MAKE) build-frontend-internal; \
		else \
			echo "$(COLOR_GREEN)✓ 前端已是最新，跳过构建$(COLOR_RESET)"; \
		fi; \
	fi

# 内部前端构建目标
.PHONY: build-frontend-internal
build-frontend-internal:
	@cd $(FRONTEND_DIR) && npm install
	@cd $(FRONTEND_DIR) && npm run build
	@mkdir -p $(BACKEND_DIR)/frontend/dist
	@cp -r $(FRONTEND_DIR)/dist/* $(BACKEND_DIR)/frontend/dist/
	@touch $(FRONTEND_BUILD_MARKER)
	@echo "$(COLOR_GREEN)✅ 前端构建完成$(COLOR_RESET)"

.PHONY: dev
dev: ensure-frontend-built ## Run in development mode (with air hot reload)
	@echo "$(COLOR_YELLOW)🔧 Starting development mode with air...$(COLOR_RESET)"
	@if ! command -v air &> /dev/null; then \
		echo "$(COLOR_YELLOW)⚠️  Air not installed, installing...$(COLOR_RESET)"; \
		go install github.com/air-verse/air@latest; \
		echo "$(COLOR_GREEN)✅ Air installed!$(COLOR_RESET)"; \
		echo "$(COLOR_YELLOW)💡 Adding ~/go/bin to PATH...$(COLOR_RESET)"; \
		export PATH="$$PATH:$$HOME/go/bin"; \
	fi
	@echo "$(COLOR_YELLOW)🔄 Hot reload enabled - changes will auto-restart server$(COLOR_RESET)"
	export PATH="$$PATH:$$HOME/go/bin" && cd $(BACKEND_DIR) && air

.PHONY: dev-backend
dev-backend: ## Run backend only with air (skips frontend build check)
	@echo "$(COLOR_YELLOW)🔧 Starting backend in dev mode with air...$(COLOR_RESET)"
	@if [ ! -d "$(BACKEND_DIR)/frontend/dist" ]; then \
		echo "$(COLOR_YELLOW)⚠️  前端未构建，请先运行: make build-frontend$(COLOR_RESET)"; \
		exit 1; \
	fi
	@if ! command -v air &> /dev/null; then \
		echo "$(COLOR_YELLOW)⚠️  Air not installed, installing...$(COLOR_RESET)"; \
		go install github.com/air-verse/air@latest; \
		echo "$(COLOR_GREEN)✅ Air installed!$(COLOR_RESET)"; \
	fi
	export PATH="$$PATH:$$HOME/go/bin" && cd $(BACKEND_DIR) && air

.PHONY: dev-frontend
dev-frontend: ## Run frontend development server
	@echo "$(COLOR_YELLOW)🔧 Starting frontend dev server...$(COLOR_RESET)"
	cd $(FRONTEND_DIR) && npm run dev

# ==============================================================================
# Build
# ==============================================================================
.PHONY: build
build: clean build-frontend build-backend ## Full build (frontend + backend for all platforms)
	@echo "$(COLOR_GREEN)✅ Build complete! Binaries are in $(DIST_DIR)/$(COLOR_RESET)"
	@ls -lh $(DIST_DIR)/

.PHONY: build-frontend
build-frontend: ## Build frontend only (force rebuild)
	@echo "$(COLOR_CYAN)📦 强制构建前端...$(COLOR_RESET)"
	$(MAKE) build-frontend-internal

.PHONY: build-backend
build-backend: build-frontend ## Build backend for all platforms (includes frontend)
	@echo "$(COLOR_CYAN)🔨 Building Go backend for all platforms...$(COLOR_RESET)"
	@mkdir -p $(BACKEND_DIR)/frontend/dist
	@cp -r $(FRONTEND_DIR)/dist/* $(BACKEND_DIR)/frontend/dist/
	@chmod +x $(BUILD_SCRIPT)
	@cd $(BACKEND_DIR) && ./build.sh

.PHONY: build-linux
build-linux: build-frontend ## Build for Linux (amd64 + arm64)
	@echo "$(COLOR_CYAN)🐧 Building for Linux...$(COLOR_RESET)"
	@mkdir -p $(BACKEND_DIR)/frontend/dist $(DIST_DIR)
	@cp -r $(FRONTEND_DIR)/dist/* $(BACKEND_DIR)/frontend/dist/
	cd $(BACKEND_DIR) && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_LINUX_AMD64) .
	cd $(BACKEND_DIR) && GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_LINUX_ARM64) .
	@echo "$(COLOR_GREEN)✅ Linux builds complete$(COLOR_RESET)"

.PHONY: build-darwin
build-darwin: build-frontend ## Build for macOS (amd64 + arm64)
	@echo "$(COLOR_CYAN)🍎 Building for macOS...$(COLOR_RESET)"
	@mkdir -p $(BACKEND_DIR)/frontend/dist $(DIST_DIR)
	@cp -r $(FRONTEND_DIR)/dist/* $(BACKEND_DIR)/frontend/dist/
	cd $(BACKEND_DIR) && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_DARWIN_AMD64) .
	cd $(BACKEND_DIR) && GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_DARWIN_ARM64) .
	@echo "$(COLOR_GREEN)✅ macOS builds complete$(COLOR_RESET)"

.PHONY: build-windows
build-windows: build-frontend ## Build for Windows (amd64)
	@echo "$(COLOR_CYAN)🪟 Building for Windows...$(COLOR_RESET)"
	@mkdir -p $(BACKEND_DIR)/frontend/dist $(DIST_DIR)
	@cp -r $(FRONTEND_DIR)/dist/* $(BACKEND_DIR)/frontend/dist/
	cd $(BACKEND_DIR) && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_WINDOWS_AMD64) .
	@echo "$(COLOR_GREEN)✅ Windows build complete$(COLOR_RESET)"

.PHONY: build-current
build-current: build-frontend ## Build for current platform only
	@echo "$(COLOR_CYAN)🔨 Building for current platform...$(COLOR_RESET)"
	@mkdir -p $(BACKEND_DIR)/frontend/dist $(DIST_DIR)
	@cp -r $(FRONTEND_DIR)/dist/* $(BACKEND_DIR)/frontend/dist/
	cd $(BACKEND_DIR) && go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_PREFIX) .
	@echo "$(COLOR_GREEN)✅ Build complete: $(DIST_DIR)/$(BINARY_PREFIX)$(COLOR_RESET)"

# ==============================================================================
# Clean
# ==============================================================================
.PHONY: clean
clean: ## Clean all build artifacts
	@echo "$(COLOR_YELLOW)🧹 Cleaning build artifacts...$(COLOR_RESET)"
	@rm -rf $(DIST_DIR)
	@rm -rf $(BACKEND_DIR)/frontend/dist
	@rm -rf $(FRONTEND_DIR)/dist
	@rm -rf $(FRONTEND_DIR)/node_modules/.vite
	@rm -f $(FRONTEND_BUILD_MARKER)
	@echo "$(COLOR_GREEN)✅ Clean complete$(COLOR_RESET)"

.PHONY: clean-all
clean-all: clean ## Deep clean (including node_modules)
	@echo "$(COLOR_YELLOW)🧹 Deep cleaning...$(COLOR_RESET)"
	@rm -rf $(FRONTEND_DIR)/node_modules
	@rm -rf $(BACKEND_DIR)/vendor
	@echo "$(COLOR_GREEN)✅ Deep clean complete$(COLOR_RESET)"

# ==============================================================================
# Dependencies
# ==============================================================================
.PHONY: deps
deps: deps-frontend deps-backend ## Install all dependencies

.PHONY: deps-frontend
deps-frontend: ## Install frontend dependencies
	@echo "$(COLOR_CYAN)📥 Installing frontend dependencies...$(COLOR_RESET)"
	cd $(FRONTEND_DIR) && npm install
	@echo "$(COLOR_GREEN)✅ Frontend dependencies installed$(COLOR_RESET)"

.PHONY: deps-backend
deps-backend: ## Install backend dependencies
	@echo "$(COLOR_CYAN)📥 Installing backend dependencies...$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go mod download && go mod tidy
	@echo "$(COLOR_GREEN)✅ Backend dependencies installed$(COLOR_RESET)"

# ==============================================================================
# Testing & Quality
# ==============================================================================
.PHONY: test
test: ## Run all tests
	@echo "$(COLOR_CYAN)🧪 Running tests...$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go test -v ./...

.PHONY: test-race
test-race: ## Run tests with race detector
	@echo "$(COLOR_CYAN)🧪 Running tests with race detector...$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go test -race -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "$(COLOR_CYAN)🧪 Running tests with coverage...$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go test -coverprofile=coverage.out ./...
	cd $(BACKEND_DIR) && go tool cover -html=coverage.out -o coverage.html
	@echo "$(COLOR_GREEN)✅ Coverage report: $(BACKEND_DIR)/coverage.html$(COLOR_RESET)"

.PHONY: lint
lint: ## Run linters
	@echo "$(COLOR_CYAN)🔍 Running linters...$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go fmt ./...
	cd $(BACKEND_DIR) && go vet ./...
	@echo "$(COLOR_GREEN)✅ Linting complete$(COLOR_RESET)"

.PHONY: fmt
fmt: ## Format code
	@echo "$(COLOR_CYAN)✨ Formatting code...$(COLOR_RESET)"
	cd $(BACKEND_DIR) && go fmt ./...
	cd $(FRONTEND_DIR) && npm run format || true
	@echo "$(COLOR_GREEN)✅ Formatting complete$(COLOR_RESET)"

# ==============================================================================
# Docker
# ==============================================================================
.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "$(COLOR_CYAN)🐳 Building Docker image...$(COLOR_RESET)"
	docker build -t claude-proxy:latest .
	@echo "$(COLOR_GREEN)✅ Docker image built$(COLOR_RESET)"

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "$(COLOR_CYAN)🐳 Running Docker container...$(COLOR_RESET)"
	docker run -p 3000:3000 --env-file .env claude-proxy:latest

# ==============================================================================
# Configuration
# ==============================================================================
.PHONY: init-config
init-config: ## Initialize configuration files
	@echo "$(COLOR_CYAN)⚙️  Initializing configuration...$(COLOR_RESET)"
	@if [ ! -f $(BACKEND_DIR)/.env ]; then \
		cp $(BACKEND_DIR)/.env.example $(BACKEND_DIR)/.env; \
		echo "$(COLOR_GREEN)✅ Created $(BACKEND_DIR)/.env$(COLOR_RESET)"; \
	else \
		echo "$(COLOR_YELLOW)⚠️  $(BACKEND_DIR)/.env already exists$(COLOR_RESET)"; \
	fi
	@mkdir -p $(BACKEND_DIR)/.config
	@echo "$(COLOR_GREEN)✅ Configuration initialized$(COLOR_RESET)"

# ==============================================================================
# Release
# ==============================================================================
.PHONY: release
release: clean build ## Create release builds for all platforms
	@echo "$(COLOR_GREEN)🎉 Release build complete!$(COLOR_RESET)"
	@echo "$(COLOR_CYAN)📦 Release artifacts:$(COLOR_RESET)"
	@ls -lh $(DIST_DIR)/

.PHONY: package
package: release ## Package release builds
	@echo "$(COLOR_CYAN)📦 Creating release packages...$(COLOR_RESET)"
	@mkdir -p $(DIST_DIR)/packages
	@cd $(DIST_DIR) && tar -czf packages/$(BINARY_PREFIX)-linux-amd64.tar.gz $(BINARY_LINUX_AMD64)
	@cd $(DIST_DIR) && tar -czf packages/$(BINARY_PREFIX)-linux-arm64.tar.gz $(BINARY_LINUX_ARM64)
	@cd $(DIST_DIR) && tar -czf packages/$(BINARY_PREFIX)-darwin-amd64.tar.gz $(BINARY_DARWIN_AMD64)
	@cd $(DIST_DIR) && tar -czf packages/$(BINARY_PREFIX)-darwin-arm64.tar.gz $(BINARY_DARWIN_ARM64)
	@cd $(DIST_DIR) && zip packages/$(BINARY_PREFIX)-windows-amd64.zip $(BINARY_WINDOWS_AMD64)
	@echo "$(COLOR_GREEN)✅ Release packages created in $(DIST_DIR)/packages/$(COLOR_RESET)"

# ==============================================================================
# Info & Help
# ==============================================================================
.PHONY: info
info: ## Show project information
	@echo "$(COLOR_CYAN)📊 Claude Proxy - Project Information$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_GREEN)Version Information:$(COLOR_RESET)"
	@echo "  Version:    $(VERSION)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo ""
	@echo "$(COLOR_GREEN)Frontend:$(COLOR_RESET)"
	@cd $(FRONTEND_DIR) && npm --version 2>/dev/null && node --version 2>/dev/null || echo "  Node.js not installed"
	@echo ""
	@echo "$(COLOR_GREEN)Backend:$(COLOR_RESET)"
	@cd $(BACKEND_DIR) && go version 2>/dev/null || echo "  Go not installed"
	@echo ""
	@echo "$(COLOR_GREEN)Project Structure:$(COLOR_RESET)"
	@echo "  Frontend: $(FRONTEND_DIR)/"
	@echo "  Backend:  $(BACKEND_DIR)/"
	@echo "  Dist:     $(DIST_DIR)/"
	@echo ""

.PHONY: help
help: ## Display this help message
	@echo "$(COLOR_CYAN)Claude Proxy - Makefile Commands$(COLOR_RESET)"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "; printf "Usage:\n  make $(COLOR_GREEN)<target>$(COLOR_RESET)\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*?## / { printf "  $(COLOR_GREEN)%-20s$(COLOR_RESET) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(COLOR_YELLOW)Examples:$(COLOR_RESET)"
	@echo "  make run              # Build frontend and run server"
	@echo "  make build            # Build for all platforms"
	@echo "  make build-linux      # Build for Linux only"
	@echo "  make dev              # Run in development mode"
	@echo "  make clean            # Clean build artifacts"
	@echo ""

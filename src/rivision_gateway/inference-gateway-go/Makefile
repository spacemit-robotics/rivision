# RiVision Gateway Build System

BINARY_NAME := rivision_gateway
VERSION ?= 1.0.0
BUILD_DIR := build
DIST_DIR ?= dist/rivision_gateway
PLATFORM ?= riscv64

LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

.PHONY: build build-riscv64 build-x86 gateway-x86 gateway-riscv64 clean help

# 按 PLATFORM 参数构建
build:
	@echo "Building $(BINARY_NAME) for $(PLATFORM)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(shell \
		if [ "$(PLATFORM)" = "riscv64" ]; then echo riscv64; \
		elif [ "$(PLATFORM)" = "x86_64" ] || [ "$(PLATFORM)" = "amd64" ]; then echo amd64; \
		else echo $(PLATFORM); fi) \
		go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/main.go
	@echo "Done: $(BUILD_DIR)/$(BINARY_NAME)"

build-riscv64:
	@$(MAKE) build PLATFORM=riscv64 --no-print-directory

build-x86:
	@$(MAKE) build PLATFORM=x86_64 --no-print-directory

# === 统一格式输出目标 ===

gateway-x86:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_gateway                                 ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  x86"
	@echo "  Platform: amd64 (GOARCH=amd64)"
	@echo "  Output:   $(BUILD_DIR)"
	@echo ""
	@echo "[1/2] Building Go binary..."
	@$(MAKE) build-x86 --no-print-directory
	@echo "[2/2] Verifying binary..."
	@file $(BUILD_DIR)/$(BINARY_NAME) | grep -q "x86-64" && echo "  ✓ x86_64 binary verified" || echo "  ✗ Architecture mismatch"
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Build Complete                                           ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "  Size:   $$(du -sh $(BUILD_DIR)/$(BINARY_NAME) | cut -f1)"
	@echo ""

gateway-riscv64:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_gateway                                 ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  riscv64"
	@echo "  Platform: riscv64 (GOARCH=riscv64)"
	@echo "  Output:   $(BUILD_DIR)"
	@echo ""
	@echo "[1/2] Building Go binary..."
	@$(MAKE) build-riscv64 --no-print-directory
	@echo "[2/2] Verifying binary..."
	@file $(BUILD_DIR)/$(BINARY_NAME) | grep -q "RISC-V" && echo "  ✓ RISC-V binary verified" || echo "  ✗ Architecture mismatch"
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Build Complete                                           ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "  Size:   $$(du -sh $(BUILD_DIR)/$(BINARY_NAME) | cut -f1)"
	@echo ""

all-platforms: build-riscv64
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-riscv64
	@$(MAKE) build PLATFORM=x86_64
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-x86_64
	@echo "All platforms built."

clean:
	@rm -rf $(BUILD_DIR)

help:
	@echo "RiVision Gateway Build System"
	@echo ""
	@echo "Usage: make <target> [PLATFORM=riscv64|x86_64]"
	@echo ""
	@echo "Targets:"
	@echo "  build            - Build for PLATFORM (default: riscv64)"
	@echo "  build-riscv64    - Build for RISC-V K3"
	@echo "  build-x86        - Build for x86_64"
	@echo "  all-platforms    - Build all platforms"
	@echo "  clean            - Remove build artifacts"

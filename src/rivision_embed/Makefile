# RiVision Embed Server Makefile

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

BUILD_DIR := build
# DIST_DIR 可从外部传入，默认 dist/rivision-embed
DIST_DIR ?= dist/rivision-embed
MODEL_SRC := models/embed
ORT_X86_LIB := third_party/onnxruntime_x86
ORT_RISCV_LIB := third_party/spacemit-ort-current/lib

# K3 Spacemit 专用工具链（支持 A100 NPU）
# 工具链位于项目 tools/ 目录，可通过环境变量 SPACEMIT_TOOLCHAIN 覆盖
SPACEMIT_TOOLCHAIN ?= $(CURDIR)/../../tools/spacemit-toolchain-linux-glibc-x86_64-v1.2.2
K3_CC := $(SPACEMIT_TOOLCHAIN)/bin/riscv64-unknown-linux-gnu-gcc
K3_CFLAGS := -march=rv64gcv_zba_zbb_zbc_zbs -mabi=lp64d

.PHONY: build build-x86 build-riscv64 clean test deps
.PHONY: embed-x86 embed-riscv64 embed-package embed-package-riscv64 test-server

# === 统一格式输出目标 ===

embed-x86:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_embed                                   ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  x86"
	@echo "  Platform: amd64 (GOARCH=amd64)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/3] Building Go binary..."
	@$(MAKE) build-x86 --no-print-directory
	@echo "[2/3] Copying models and libraries..."
	@$(MAKE) embed-package --no-print-directory 2>&1 | grep -E "^\[package\]"
	@echo "[3/3] Verifying package..."
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""
	@echo "  Contents:"
	@echo "    bin/    : rivision-embed-server"
	@echo "    models/ : $$(ls $(DIST_DIR)/models/*.onnx 2>/dev/null | wc -l) ONNX models"
	@echo "    lib/    : onnxruntime libraries"
	@echo ""

embed-riscv64:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_embed                                   ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  riscv64"
	@echo "  Platform: riscv64 (GOARCH=riscv64, K3 NPU)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/3] Building Go binary (K3 Spacemit toolchain)..."
	@$(MAKE) build-riscv64 --no-print-directory
	@echo "[2/3] Copying models and libraries..."
	@$(MAKE) embed-package-riscv64 --no-print-directory 2>&1 | grep -E "^\[package\]"
	@echo "[3/3] Verifying package..."
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""
	@echo "  Contents:"
	@echo "    bin/    : rivision-embed-server"
	@echo "    models/ : $$(ls $(DIST_DIR)/models/*.onnx 2>/dev/null | wc -l) ONNX models"
	@echo "    lib/    : onnxruntime + spacemit_ep libraries"
	@echo ""

embed-package:
	@echo "[package] 创建 x86_64 分发目录: $(DIST_DIR)"
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/models $(DIST_DIR)/config $(DIST_DIR)/scripts $(DIST_DIR)/lib
	@cp $(BUILD_DIR)/rivision-embed-server-linux-amd64 $(DIST_DIR)/bin/rivision-embed-server
	@chmod +x $(DIST_DIR)/bin/rivision-embed-server
	@cp $(ORT_X86_LIB)/libonnxruntime.so* $(DIST_DIR)/lib/
	@cp $(MODEL_SRC)/*.onnx $(DIST_DIR)/models/
	@echo "EMBED_HOST=127.0.0.1" > $(DIST_DIR)/config/embed.env
	@echo "EMBED_PORT=18081" >> $(DIST_DIR)/config/embed.env
	@echo "EMBED_THREADS=4" >> $(DIST_DIR)/config/embed.env
	@echo '#!/bin/bash' > $(DIST_DIR)/scripts/start-embed.sh
	@echo 'cd "$$(dirname "$$0")/.."' >> $(DIST_DIR)/scripts/start-embed.sh
	@echo 'export LD_LIBRARY_PATH=./lib:$$LD_LIBRARY_PATH' >> $(DIST_DIR)/scripts/start-embed.sh
	@echo './bin/rivision-embed-server --host $${EMBED_HOST:-127.0.0.1} --port $${EMBED_PORT:-18081} --text-model ./models/cn_clip_text.onnx --vision-model ./models/cn_clip_vision.onnx --threads $${EMBED_THREADS:-4}' >> $(DIST_DIR)/scripts/start-embed.sh
	@chmod +x $(DIST_DIR)/scripts/start-embed.sh
	@echo '#!/bin/bash' > $(DIST_DIR)/scripts/test-embed.sh
	@echo 'curl -s http://127.0.0.1:18081/health && echo ""' >> $(DIST_DIR)/scripts/test-embed.sh
	@chmod +x $(DIST_DIR)/scripts/test-embed.sh
	@echo "[package] 完成"

embed-package-riscv64:
	@echo "[package] 创建 RISC-V 分发目录: $(DIST_DIR)"
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/models $(DIST_DIR)/config $(DIST_DIR)/scripts $(DIST_DIR)/lib
	@cp $(BUILD_DIR)/rivision-embed-server-linux-riscv64 $(DIST_DIR)/bin/rivision-embed-server
	@chmod +x $(DIST_DIR)/bin/rivision-embed-server
	@cp $(ORT_RISCV_LIB)/libonnxruntime.so* $(DIST_DIR)/lib/
	@cp $(ORT_RISCV_LIB)/libspacemit_ep.so* $(DIST_DIR)/lib/ 2>/dev/null || true
	@cp $(ORT_RISCV_LIB)/libonnxruntime_providers_shared.so $(DIST_DIR)/lib/ 2>/dev/null || true
	@cp $(MODEL_SRC)/*.onnx $(DIST_DIR)/models/
	@echo "EMBED_HOST=127.0.0.1" > $(DIST_DIR)/config/embed.env
	@echo "EMBED_PORT=18081" >> $(DIST_DIR)/config/embed.env
	@echo "EMBED_THREADS=2" >> $(DIST_DIR)/config/embed.env
	@echo '#!/bin/bash' > $(DIST_DIR)/scripts/start-embed.sh
	@echo 'cd "$$(dirname "$$0")/.."' >> $(DIST_DIR)/scripts/start-embed.sh
	@echo 'export LD_LIBRARY_PATH=./lib:$$LD_LIBRARY_PATH' >> $(DIST_DIR)/scripts/start-embed.sh
	@echo './bin/rivision-embed-server --host $${EMBED_HOST:-127.0.0.1} --port $${EMBED_PORT:-18081} --text-model ./models/cn_clip_text.onnx --vision-model ./models/cn_clip_vision.onnx --threads $${EMBED_THREADS:-2}' >> $(DIST_DIR)/scripts/start-embed.sh
	@chmod +x $(DIST_DIR)/scripts/start-embed.sh
	@echo "[package] 完成"

build:
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/rivision-embed-server ./cmd/server

build-x86:
	@echo "[build] x86_64 编译..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/rivision-embed-server-linux-amd64 ./cmd/server
	@echo "[build] 完成"

K3_CXX := $(SPACEMIT_TOOLCHAIN)/bin/riscv64-unknown-linux-gnu-g++
ORT_RISCV_INCLUDE := $(CURDIR)/third_party/spacemit-ort-current/include

build-riscv64:
	@echo "[build] RISC-V (K3 Spacemit A100 NPU) 编译..."
	@if [ ! -x "$(K3_CC)" ]; then \
		echo "⚠️  K3 工具链未找到: $(K3_CC)"; \
		echo "   工具链应位于: ../../tools/spacemit-toolchain-linux-glibc-x86_64-v1.2.2/"; \
		echo "   或设置 SPACEMIT_TOOLCHAIN 环境变量"; \
		exit 1; \
	fi
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=1 GOOS=linux GOARCH=riscv64 \
		CC="$(K3_CC)" \
		CXX="$(K3_CXX)" \
		CGO_CFLAGS="$(K3_CFLAGS) -I$(ORT_RISCV_INCLUDE)" \
		CGO_CXXFLAGS="-std=c++17 $(K3_CFLAGS) -I$(ORT_RISCV_INCLUDE)" \
		CGO_LDFLAGS="-L$(ORT_RISCV_LIB) -lonnxruntime -lspacemit_ep -lstdc++" \
		go build $(LDFLAGS) -o $(BUILD_DIR)/rivision-embed-server-linux-riscv64 ./cmd/server
	@echo "[build] 完成 (使用 K3 Spacemit 工具链, 支持 A100 NPU)"

test-server:
	@cd $(DIST_DIR) && ./scripts/start-embed.sh

deps:
	go mod tidy
	go mod download

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR) dist/

help:
	@echo "embed-x86 / embed-riscv64 / test-server / clean"

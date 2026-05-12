# RiVision Worker Makefile
# C++ Worker with SpacemiT K3 NPU acceleration (riscv64 native)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

BUILD_DIR := build
DIST_DIR ?= dist/rivision-worker

# go2rtc (RTSP → WebRTC/fMP4 转换)
GO2RTC_VERSION := v1.9.4
GO2RTC_SRC := $(CURDIR)/third_party/go2rtc
GO2RTC_BUILD := $(GO2RTC_SRC)/build

.PHONY: all clean build test install package help
.PHONY: go2rtc go2rtc-clone go2rtc-clean

# ============================================================================
# 主目标
# ============================================================================

all: build

build:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_worker (C++, K3 native)                 ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: riscv64 (SpacemiT K3 NPU)"
	@echo "  Output:   $(BUILD_DIR)/"
	@echo ""
	@mkdir -p $(BUILD_DIR)
	@cd $(BUILD_DIR) && cmake .. \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=$(CURDIR)/$(BUILD_DIR)/bin
	@cmake --build $(BUILD_DIR) -j$$(nproc)
	@echo ""
	@echo "  ✅ Build complete -> $(BUILD_DIR)/"
	@echo ""

# ============================================================================
# 打包
# ============================================================================

package: build go2rtc
	@echo "[package] 创建分发目录: $(DIST_DIR)"
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config $(DIST_DIR)/scripts $(DIST_DIR)/models
	@cp $(BUILD_DIR)/bin/rivision_worker $(DIST_DIR)/bin/ 2>/dev/null || true
	@cp $(BUILD_DIR)/bin/rivision_vlm    $(DIST_DIR)/bin/ 2>/dev/null || true
	@cp -r config/*.yaml                 $(DIST_DIR)/config/   2>/dev/null || true
	@cp scripts/*.sh                     $(DIST_DIR)/scripts/  2>/dev/null || true
	@cp scripts/*.service                $(DIST_DIR)/scripts/  2>/dev/null || true
	@cp $(GO2RTC_BUILD)/go2rtc_linux_riscv64 $(DIST_DIR)/bin/go2rtc 2>/dev/null || true
	@tar -czf rivision_worker-$(VERSION)-riscv64.tar.gz -C dist rivision-worker
	@echo "[package] 完成 -> rivision_worker-$(VERSION)-riscv64.tar.gz"

# ============================================================================
# go2rtc 构建 (RTSP → WebRTC/fMP4 转换, K3 native)
# ============================================================================

go2rtc-clone:
	@if [ ! -d "$(GO2RTC_SRC)/.git" ]; then \
		echo "[go2rtc] Cloning $(GO2RTC_VERSION)..."; \
		git clone --depth 1 --branch $(GO2RTC_VERSION) https://github.com/AlexxIT/go2rtc.git $(GO2RTC_SRC); \
	else \
		echo "[go2rtc] Already cloned: $(GO2RTC_SRC)"; \
	fi

go2rtc: go2rtc-clone
	@if [ ! -f "$(GO2RTC_BUILD)/go2rtc_linux_riscv64" ] || [ "$(GO2RTC_SRC)/main.go" -nt "$(GO2RTC_BUILD)/go2rtc_linux_riscv64" ]; then \
		echo "[go2rtc] Building..."; \
		mkdir -p $(GO2RTC_BUILD); \
		cd $(GO2RTC_SRC) && go build \
			-ldflags "-s -w -X main.version=$(GO2RTC_VERSION)" \
			-o $(GO2RTC_BUILD)/go2rtc_linux_riscv64 .; \
		ls -lh $(GO2RTC_BUILD)/go2rtc_linux_riscv64; \
	else \
		echo "[go2rtc] 已是最新"; \
	fi

go2rtc-clean:
	@rm -rf $(GO2RTC_BUILD)
	@echo "[go2rtc] 清理完成"

# ============================================================================
# 安装
# ============================================================================

install: build
	@cmake --install $(BUILD_DIR) --prefix /opt/rivision/rivision_worker

# ============================================================================
# 清理和工具
# ============================================================================

clean:
	@rm -rf $(BUILD_DIR)
	@rm -rf dist *.tar.gz
	@echo "[clean] 完成"

format:
	@find src include -name "*.cpp" -o -name "*.h" | xargs clang-format -i 2>/dev/null || true
	@echo "[format] 完成"

# ============================================================================
# 帮助
# ============================================================================

help:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  RiVision Worker (C++) Build System — K3 RISC-V native       ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "  构建目标:"
	@echo "    make / make build    编译 C++ binary"
	@echo "    make package         编译 + 打包 (含 go2rtc)"
	@echo "    make install         安装到 /opt/rivision/rivision_worker"
	@echo ""
	@echo "  go2rtc (视频流转换):"
	@echo "    make go2rtc          编译 go2rtc $(GO2RTC_VERSION)"
	@echo "    make go2rtc-clone    仅克隆源码"
	@echo "    make go2rtc-clean    清理构建产物"
	@echo ""
	@echo "  其他:"
	@echo "    make clean           清理构建产物"
	@echo "    make format          clang-format 代码"
	@echo ""
	@echo "  运行时依赖 (apt 安装到系统):"
	@echo "    spacemit-onnxruntime           SpacemiT ONNX Runtime + EP (NPU)"
	@echo "    llama.cpp-tools-spacemit       llama.cpp + SpacemiT 后端"
	@echo "    libopencv-dev                  OpenCV (riscv64)"
	@echo ""

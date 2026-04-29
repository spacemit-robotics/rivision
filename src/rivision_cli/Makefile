# RiVision CLI - Unified Build System

BINARY_NAME := rivision-cli
VERSION ?= 1.0.0
BUILD_DIR := build
DIST_DIR := dist/rivision-cli
WEB_DIR := go2rtc-vue
WEB_DEST := internal/webserver/frontend/dist
EMBED_BIN_DIR := internal/embed/binaries

LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

# Makefile 所在目录（绝对路径，用于修复相对路径问题）
MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# 构建工具脚本路径
BUILD_UTILS := $(MAKEFILE_DIR)../../build/scripts/build_utils.sh

.PHONY: all clean deps web web-build help
.PHONY: cli-package cli-x86 cli-riscv64 cli-x86-ffmpeg44
.PHONY: build

# === 主要构建目标 ===

# RISC-V K3 (ffmpeg 6.2)
build-riscv64: web
	@./prepare_build.sh riscv64
	@echo "Building $(BINARY_NAME) for riscv64..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 \
		go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 .
	@echo "Done: $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64"

# x86_64 (ffmpeg 6.1, default)
build-x86: web
	@$(MAKE) _build-x86-do --no-print-directory

# 内部目标：不依赖 web
_build-x86-do:
	@./prepare_build.sh x86_64
	@echo "Building $(BINARY_NAME) for x86_64..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
		go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@echo "Done: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

# x86_64 (ffmpeg 4.4)
build-x86-ffmpeg44: web
	@$(MAKE) _build-x86-ffmpeg44-do --no-print-directory

# 内部目标：不依赖 web，用于 release 流程避免重复消息
_build-x86-ffmpeg44-do:
	@./prepare_build.sh x86_64 ffmpeg44
	@echo "Building $(BINARY_NAME) for x86_64 (ffmpeg 4.4)..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
		go build $(LDFLAGS) -tags ffmpeg44 -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-ffmpeg44 .
	@echo "Done: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-ffmpeg44"

# === 辅助目标 ===

# deps: 下载 Go 依赖（网络可用时执行，跳过 go mod tidy 避免阻塞构建）
deps:
	@echo "[deps] 尝试下载 Go 依赖..."
	@go mod download 2>/dev/null && echo "[deps] 依赖下载完成" || echo "[deps] 网络不可用，跳过（已有缓存则无影响）"

web:
	@# 确保嵌入目录存在（go:embed 要求目录非空且存在）
	@mkdir -p $(WEB_DEST)
	@# Vite 已配置 outDir 直接输出到 WEB_DEST，无需拷贝
	@if [ -d "$(WEB_DIR)/src" ]; then \
		NEED_BUILD=false; \
		if [ ! -f "$(WEB_DEST)/index.html" ]; then \
			echo "[web] dist/ 不存在或为空，需要编译"; \
			NEED_BUILD=true; \
		else \
			NEWEST_SRC=$$(find $(WEB_DIR)/src $(WEB_DIR)/public $(WEB_DIR)/index.html \
				$(WEB_DIR)/vite.config.ts $(WEB_DIR)/package.json \
				-newer $(WEB_DEST)/index.html 2>/dev/null | head -1); \
			if [ -n "$$NEWEST_SRC" ]; then \
				echo "[web] 源码有更新 ($$NEWEST_SRC)，需要重新编译"; \
				NEED_BUILD=true; \
			fi; \
		fi; \
		if [ "$$NEED_BUILD" = "true" ]; then \
			echo "[web] 编译 go2rtc-vue..."; \
			(cd $(WEB_DIR) && npm install --silent && npm run build); \
			echo "[web] 编译完成"; \
		else \
			echo "[web] 源码无变化，跳过编译"; \
		fi; \
	else \
		echo "[web] go2rtc-vue 源码不存在"; \
		if [ ! -f "$(WEB_DEST)/index.html" ]; then \
			echo '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><title>RiVision</title></head><body><p>请运行 make web 构建 Web 前端</p></body></html>' > $(WEB_DEST)/index.html; \
		fi; \
	fi

# 强制重新编译 Web 前端 (跳过增量检测)
web-build:
	@mkdir -p $(WEB_DEST)
	@if [ -d "$(WEB_DIR)/src" ]; then \
		echo "[web] 强制编译 go2rtc-vue..."; \
		(cd $(WEB_DIR) && npm install --silent && npm run build); \
		echo "[web] 强制编译完成"; \
	else \
		echo "[web] ERROR: $(WEB_DIR)/src 不存在"; \
		exit 1; \
	fi

all-platforms: build-riscv64 build-x86 build-x86-ffmpeg44
	@echo "All platforms built."
	@ls -la $(BUILD_DIR)/

# === 打包（rivision-owl 风格：build + package → dist/rivision-cli/） ===
#
# dist/rivision-cli/ 目录结构:
#   rivision-cli              可执行文件
#   config/rivision.yaml.example
#   config/go2rtc.yaml.example
#   models/                   YOLO 模型（*.onnx + class_names/）
#   mp4path/                  MP4 视频文件目录（由运行时创建）
#   data/                     运行时创建
#   logs/                     运行时创建
#   install.sh                安装脚本
#
# 用法（独立构建）:
#   make cli-x86              # x86_64 完整构建（build + package）
#   make cli-riscv64           # RISC-V K3 完整构建
#   make cli-x86-ffmpeg44      # x86_64 (ffmpeg 4.4)
#
# 用法（从 build/Makefile 调用，支持自定义 DIST_DIR）:
#   cd src/rivision_cli && make cli-x86 DIST_DIR=$(CURDIR)/dist/rivision-cli

cli-x86:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_cli                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  x86"
	@echo "  Platform: amd64 (GOARCH=amd64)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/4] Building Web UI..."
	@$(MAKE) web --no-print-directory
	@echo "[2/4] Building Go binary..."
	@$(MAKE) build-x86 --no-print-directory 2>&1 | grep -v "^make"
	@echo "[3/4] Packaging binaries and configs..."
	@$(MAKE) cli-package --no-print-directory 2>&1 | grep -E "^  |^Package"
	@echo "[4/4] Finalizing..."
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""

cli-riscv64:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_cli                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  riscv64"
	@echo "  Platform: riscv64 (GOARCH=riscv64)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/4] Building Web UI..."
	@$(MAKE) web --no-print-directory
	@echo "[2/4] Building Go binary..."
	@$(MAKE) build-riscv64 --no-print-directory 2>&1 | grep -v "^make"
	@echo "[3/4] Packaging binaries and configs..."
	@$(MAKE) cli-package-riscv --no-print-directory 2>&1 | grep -E "^  |^Package"
	@echo "[4/4] Finalizing..."
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""

cli-x86-ffmpeg44:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_cli                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  x86 (ffmpeg44)"
	@echo "  Platform: amd64 (GOARCH=amd64, ffmpeg 4.4)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/4] Building Web UI..."
	@$(MAKE) web --no-print-directory
	@echo "[2/4] Building Go binary..."
	@$(MAKE) build-x86-ffmpeg44 --no-print-directory 2>&1 | grep -v "^make"
	@echo "[3/4] Packaging binaries and configs..."
	@$(MAKE) cli-package-ffmpeg44 --no-print-directory 2>&1 | grep -E "^  |^Package"
	@echo "[4/4] Finalizing..."
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""

# 内部：x86_64 打包
cli-package: cli-package-amd64

# 内部：x86_64 打包流程（编译 → 打包到 dist/rivision-cli/）
cli-package-amd64: build-x86
	@echo "Packaging to $(DIST_DIR)/..."
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config $(DIST_DIR)/models $(DIST_DIR)/models/class_names $(DIST_DIR)/models/image-embed $(DIST_DIR)/third_party/image-embed $(DIST_DIR)/mp4path $(DIST_DIR)/data $(DIST_DIR)/logs $(DIST_DIR)/services

	# Binary
	@if [ ! -f $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ]; then \
		echo "ERROR: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 不存在，请先: make build-x86"; exit 1; fi
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(DIST_DIR)/bin/rivision-cli
	@chmod +x $(DIST_DIR)/bin/rivision-cli
	@echo "  rivision-cli: $$(du -sh $(DIST_DIR)/bin/rivision-cli | cut -f1)"

	# 嵌入的 go2rtc / rtsp_server 二进制（供运行时提取）
	@for arch in amd64 riscv64; do \
		if [ -f $(EMBED_BIN_DIR)/go2rtc-linux-$$arch ]; then \
			cp $(EMBED_BIN_DIR)/go2rtc-linux-$$arch $(DIST_DIR)/bin/go2rtc-linux-$$arch; \
			echo "  go2rtc-linux-$$arch: $$(du -sh $(DIST_DIR)/bin/go2rtc-linux-$$arch | cut -f1)"; \
		fi; \
		if [ -f $(EMBED_BIN_DIR)/rtsp_server-linux-$$arch ]; then \
			cp $(EMBED_BIN_DIR)/rtsp_server-linux-$$arch $(DIST_DIR)/bin/rtsp_server-linux-$$arch; \
			echo "  rtsp_server-linux-$$arch: $$(du -sh $(DIST_DIR)/bin/rtsp_server-linux-$$arch | cut -f1)"; \
		fi; \
	done

	# 配置文件
	@if [ -f $(MAKEFILE_DIR)/configs/rivision.yaml.example ]; then \
		cp $(MAKEFILE_DIR)/configs/rivision.yaml.example $(DIST_DIR)/config/; \
		echo "  config/rivision.yaml.example"; \
	fi
	@if [ -f $(MAKEFILE_DIR)/internal/embed/config/go2rtc.yaml ]; then \
		cp $(MAKEFILE_DIR)/internal/embed/config/go2rtc.yaml $(DIST_DIR)/config/go2rtc.yaml.example; \
		echo "  config/go2rtc.yaml.example"; \
	fi

	# 模型文件
	@if [ -d $(MAKEFILE_DIR)/models ]; then \
		for f in $(MAKEFILE_DIR)/models/*.onnx; do \
			[ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/ && echo "  models/$$(basename $$f)"; \
		done; \
		if [ -d $(MAKEFILE_DIR)/models/class_names ]; then \
			for f in $(MAKEFILE_DIR)/models/class_names/*; do \
				[ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/class_names/ && echo "  models/class_names/$$(basename $$f)"; \
			done; \
		fi; \
		if [ -d $(MAKEFILE_DIR)/models/image-embed ]; then \
			mkdir -p $(DIST_DIR)/models/image-embed; \
			for f in $(MAKEFILE_DIR)/models/image-embed/*.onnx; do \
				bn=$$(basename "$$f"); \
				case "$$bn" in \
					*_q4.onnx) ;; \
					*) [ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/image-embed/ && echo "  models/image-embed/$$bn" ;; \
				esac; \
			done; \
		fi \
	fi

	# MP4 测试视频（可选）
	@if [ -d $(MAKEFILE_DIR)/mp4path ]; then \
		mp4_count=$$(find $(MAKEFILE_DIR)/mp4path -maxdepth 1 -name "*.mp4" 2>/dev/null | wc -l); \
		if [ "$$mp4_count" -gt 0 ]; then \
			cp $(MAKEFILE_DIR)/mp4path/*.mp4 $(DIST_DIR)/mp4path/ 2>/dev/null || true; \
			echo "  mp4path/: $$mp4_count 个 MP4 文件"; \
		fi \
	fi

	# 安装脚本
	@if [ -f $(MAKEFILE_DIR)/install.sh ]; then \
		cp $(MAKEFILE_DIR)/install.sh $(DIST_DIR)/; \
		chmod +x $(DIST_DIR)/install.sh; \
		echo "  install.sh"; \
	fi

	# Systemd 服务（install.sh 运行时动态生成，跳过）
	# 内置 image-embed-server（二进制）
	@echo "Building image-embed-server..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST_DIR)/third_party/image-embed/image-embed-server ./cmd/image-embed-server
	@chmod +x $(DIST_DIR)/third_party/image-embed/image-embed-server
	@echo "  third_party/image-embed/image-embed-server"

	@echo "  services/: (由 install.sh 动态生成，跳过)"

	@echo ""
	@echo "Package complete: $(DIST_DIR)"
	@echo ""
	@echo "部署步骤:"
	@echo "  1. cp -r $(DIST_DIR)/* /opt/rivision-cli/"
	@echo "  2. 编辑 /opt/rivision-cli/config/rivision.yaml 填写真实值"
	@echo "  3. sudo systemctl daemon-reload"
	@echo "  4. sudo systemctl enable rivision-cli"
	@echo "  5. sudo systemctl start rivision-cli"

# 内部：RISC-V 打包流程
cli-package-riscv: build-riscv64
	@echo "Packaging to $(DIST_DIR)/..."
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config $(DIST_DIR)/models $(DIST_DIR)/models/class_names $(DIST_DIR)/models/image-embed $(DIST_DIR)/third_party/image-embed $(DIST_DIR)/mp4path $(DIST_DIR)/data $(DIST_DIR)/logs $(DIST_DIR)/services

	@if [ ! -f $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 ]; then \
		echo "ERROR: $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 不存在，请先: make build-riscv64"; exit 1; fi
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 $(DIST_DIR)/bin/rivision-cli
	@chmod +x $(DIST_DIR)/bin/rivision-cli
	@echo "  rivision-cli: $$(du -sh $(DIST_DIR)/bin/rivision-cli | cut -f1)"

	@for arch in amd64 riscv64; do \
		if [ -f $(EMBED_BIN_DIR)/go2rtc-linux-$$arch ]; then \
			cp $(EMBED_BIN_DIR)/go2rtc-linux-$$arch $(DIST_DIR)/bin/go2rtc-linux-$$arch; \
			echo "  go2rtc-linux-$$arch: $$(du -sh $(DIST_DIR)/bin/go2rtc-linux-$$arch | cut -f1)"; \
		fi; \
		if [ -f $(EMBED_BIN_DIR)/rtsp_server-linux-$$arch ]; then \
			cp $(EMBED_BIN_DIR)/rtsp_server-linux-$$arch $(DIST_DIR)/bin/rtsp_server-linux-$$arch; \
			echo "  rtsp_server-linux-$$arch: $$(du -sh $(DIST_DIR)/bin/rtsp_server-linux-$$arch | cut -f1)"; \
		fi; \
	done

	@if [ -f $(MAKEFILE_DIR)/configs/rivision.yaml.example ]; then \
		cp $(MAKEFILE_DIR)/configs/rivision.yaml.example $(DIST_DIR)/config/; \
		echo "  config/rivision.yaml.example"; \
	fi
	@if [ -f $(MAKEFILE_DIR)/internal/embed/config/go2rtc.yaml ]; then \
		cp $(MAKEFILE_DIR)/internal/embed/config/go2rtc.yaml $(DIST_DIR)/config/go2rtc.yaml.example; \
		echo "  config/go2rtc.yaml.example"; \
	fi

	@if [ -d $(MAKEFILE_DIR)/models ]; then \
		for f in $(MAKEFILE_DIR)/models/*.onnx; do \
			[ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/ && echo "  models/$$(basename $$f)"; \
		done; \
		if [ -d $(MAKEFILE_DIR)/models/class_names ]; then \
			for f in $(MAKEFILE_DIR)/models/class_names/*; do \
				[ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/class_names/ && echo "  models/class_names/$$(basename $$f)"; \
			done; \
		fi; \
		if [ -d $(MAKEFILE_DIR)/models/image-embed ]; then \
			mkdir -p $(DIST_DIR)/models/image-embed; \
			for f in $(MAKEFILE_DIR)/models/image-embed/*.onnx; do \
				if [ -f "$$f" ]; then \
					case "$$(basename $$f)" in \
						*_q4.onnx) ;; \
						*) cp "$$f" $(DIST_DIR)/models/image-embed/ && echo "  models/image-embed/$$(basename $$f)" ;; \
					esac; \
				fi; \
			done; \
		fi \
	fi

	@if [ -d $(MAKEFILE_DIR)/mp4path ]; then \
		mp4_count=$$(find $(MAKEFILE_DIR)/mp4path -maxdepth 1 -name "*.mp4" 2>/dev/null | wc -l); \
		if [ "$$mp4_count" -gt 0 ]; then \
			cp $(MAKEFILE_DIR)/mp4path/*.mp4 $(DIST_DIR)/mp4path/ 2>/dev/null || true; \
			echo "  mp4path/: $$mp4_count 个 MP4 文件"; \
		fi \
	fi

	@if [ -f $(MAKEFILE_DIR)/install.sh ]; then \
		cp $(MAKEFILE_DIR)/install.sh $(DIST_DIR)/; \
		chmod +x $(DIST_DIR)/install.sh; \
		echo "  install.sh"; \
	fi

	# 内置 image-embed-server（二进制）
	@echo "Building image-embed-server..."
	@GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST_DIR)/third_party/image-embed/image-embed-server ./cmd/image-embed-server
	@chmod +x $(DIST_DIR)/third_party/image-embed/image-embed-server
	@echo "  third_party/image-embed/image-embed-server"

	@echo "  services/: (由 install.sh 动态生成，跳过)"
	@echo ""
	@echo "Package complete: $(DIST_DIR)"

# 内部：x86_64 (ffmpeg 4.4) 打包流程
cli-package-ffmpeg44: build-x86-ffmpeg44
	@echo "Packaging to $(DIST_DIR)/..."
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config $(DIST_DIR)/models $(DIST_DIR)/models/class_names $(DIST_DIR)/mp4path $(DIST_DIR)/data $(DIST_DIR)/logs $(DIST_DIR)/services

	@if [ ! -f $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-ffmpeg44 ]; then \
		echo "ERROR: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-ffmpeg44 不存在，请先: make build-x86-ffmpeg44"; exit 1; fi
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-ffmpeg44 $(DIST_DIR)/bin/rivision-cli
	@chmod +x $(DIST_DIR)/bin/rivision-cli
	@echo "  rivision-cli: $$(du -sh $(DIST_DIR)/bin/rivision-cli | cut -f1)"

	@for arch in amd64 riscv64; do \
		if [ -f $(EMBED_BIN_DIR)/go2rtc-linux-$$arch ]; then \
			cp $(EMBED_BIN_DIR)/go2rtc-linux-$$arch $(DIST_DIR)/bin/go2rtc-linux-$$arch; \
			echo "  go2rtc-linux-$$arch: $$(du -sh $(DIST_DIR)/bin/go2rtc-linux-$$arch | cut -f1)"; \
		fi; \
		if [ -f $(EMBED_BIN_DIR)/rtsp_server-linux-$$arch ]; then \
			cp $(EMBED_BIN_DIR)/rtsp_server-linux-$$arch $(DIST_DIR)/bin/rtsp_server-linux-$$arch; \
			echo "  rtsp_server-linux-$$arch: $$(du -sh $(DIST_DIR)/bin/rtsp_server-linux-$$arch | cut -f1)"; \
		fi; \
	done

	@if [ -f $(MAKEFILE_DIR)/configs/rivision.yaml.example ]; then \
		cp $(MAKEFILE_DIR)/configs/rivision.yaml.example $(DIST_DIR)/config/; \
		echo "  config/rivision.yaml.example"; \
	fi
	@if [ -f $(MAKEFILE_DIR)/internal/embed/config/go2rtc.yaml ]; then \
		cp $(MAKEFILE_DIR)/internal/embed/config/go2rtc.yaml $(DIST_DIR)/config/go2rtc.yaml.example; \
		echo "  config/go2rtc.yaml.example"; \
	fi

	@if [ -d $(MAKEFILE_DIR)/models ]; then \
		for f in $(MAKEFILE_DIR)/models/*.onnx; do \
			[ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/ && echo "  models/$$(basename $$f)"; \
		done; \
		if [ -d $(MAKEFILE_DIR)/models/class_names ]; then \
			for f in $(MAKEFILE_DIR)/models/class_names/*; do \
				[ -f "$$f" ] && cp "$$f" $(DIST_DIR)/models/class_names/ && echo "  models/class_names/$$(basename $$f)"; \
			done; \
		fi \
	fi

	@if [ -d $(MAKEFILE_DIR)/mp4path ]; then \
		mp4_count=$$(find $(MAKEFILE_DIR)/mp4path -maxdepth 1 -name "*.mp4" 2>/dev/null | wc -l); \
		if [ "$$mp4_count" -gt 0 ]; then \
			cp $(MAKEFILE_DIR)/mp4path/*.mp4 $(DIST_DIR)/mp4path/ 2>/dev/null || true; \
			echo "  mp4path/: $$mp4_count 个 MP4 文件"; \
		fi \
	fi

	@if [ -f $(MAKEFILE_DIR)/install.sh ]; then \
		cp $(MAKEFILE_DIR)/install.sh $(DIST_DIR)/; \
		chmod +x $(DIST_DIR)/install.sh; \
		echo "  install.sh"; \
	fi

	@echo "  services/: (由 install.sh 动态生成，跳过)"
	@echo ""
	@echo "Package complete: $(DIST_DIR)"

clean:
	@rm -rf $(BUILD_DIR)
	@rm -rf $(WEB_DEST)
	@rm -rf $(WEB_SRC)
	@rm -rf $(WEB_DIR)/node_modules
	@rm -rf $(DIST_DIR)

help:
	@echo "RiVision CLI Build System"
	@echo ""
	@echo "Usage: make <target> [VERSION=x.y.z] [DIST_DIR=path]"
	@echo ""
	@echo "Packaging targets (recommended):"
	@echo "  cli-x86           - x86_64 完整构建+打包 → dist/rivision-cli/"
	@echo "  cli-riscv64       - RISC-V K3 完整构建+打包 → dist/rivision-cli/"
	@echo "  cli-x86-ffmpeg44  - x86_64 (ffmpeg 4.4) 完整构建+打包"
	@echo ""
	@echo "Build targets:"
	@echo "  build-riscv64     - 仅编译 RISC-V"
	@echo "  build-x86         - 仅编译 x86_64 (ffmpeg 6.1)"
	@echo "  build-x86-ffmpeg44 - 仅编译 x86_64 (ffmpeg 4.4)"
	@echo "  all-platforms     - 编译所有平台"
	@echo ""
	@echo "Web frontend:"
	@echo "  web               - 增量检测+编译+同步 Web 前端"
	@echo "  web-build         - 强制重新编译 Web 前端"
	@echo ""
	@echo "Utilities:"
	@echo "  deps              - 下载 Go 依赖（可选）"
	@echo "  clean             - 清理 build + dist + embedded Web 前端"

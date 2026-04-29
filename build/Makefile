# ============================================================
# RiVision K3 Deploy - Top-level Build System
# 统一构建组件: rivision_cli, rivision_gateway, rivision_embed, rivision_owl, rivision_node
# ============================================================

# 默认目标: 显示帮助
.DEFAULT_GOAL := help

# === 变量定义 ===
# VERSION 优先级: 1) 命令行指定 2) 环境变量 3) VERSION 文件 4) git tag 5) 默认值
# 用法示例:
#   make release VERSION=v1.0.0              # 手动指定版本
#   VERSION=v1.0.0 make release              # 环境变量指定
#   echo "1.0.0" > ../VERSION                # 修改 VERSION 文件
VERSION ?= $(shell cat ../VERSION 2>/dev/null || git describe --tags --always 2>/dev/null || echo "1.0.0")
TIMESTAMP := $(shell date +%Y%m%d-%H%M%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
NODE_PROFILE ?= riscv_b
FFMPEG_VARIANT ?= auto

# PACKAGE 控制打包内容:
#   all       - host + node (完整包，默认)
#   host      - cli, gateway, embed, owl (主机端)
#   node      - 完整 node (yolo + llama)
#   node_llama - 仅 llama (VLM 图像描述)
#   node_yolo  - 仅 yolo (目标检测)
#   node_all   - 同时打包 3 种 node 变体
PACKAGE ?= all

SRC_DIR := ../src
DIST_BASE := ../dist
CURDIR := $(shell pwd)

# === Go 环境统一配置 ===
# 所有子项目继承此设置，子项目 Makefile 不再单独设置
GO_MIN_VERSION := 1.25.0
export GOPROXY := https://goproxy.cn,https://proxy.golang.org,direct
export GONOSUMDB := *

# === .PHONY 声明 (集中管理) ===
.PHONY: help clean check-go
.PHONY: release release-riscv64 release-x86 release-arm64 _release-x86-do
.PHONY: cli-x86 _cli-x86-do cli-riscv64 cli-web cli-web-build
.PHONY: embed-x86 embed-riscv64
.PHONY: gateway-x86 gateway-riscv64
.PHONY: node-x86 node-riscv64 node-riscv64-llama node-riscv64-yolo node-arm64
.PHONY: yolo-server-x86 yolo-server-riscv64 yolo-server-arm64
.PHONY: update-third-party
.PHONY: owl-x86 owl-riscv owl-riscv64 owl-sim owl-gb28181-sim owl-onvif-sim owl-web check-zlm zlm-x86 zlm-riscv64

# === FFmpeg 版本检测脚本 (内联到 recipe 中使用) ===
# 设置 FFVAR 变量: ffmpeg44 或 ffmpeg61
# 用法: 在 recipe 开头加 $(DETECT_FFMPEG)
DETECT_FFMPEG = \
	FFVAR="$(FFMPEG_VARIANT)"; \
	if [ "$$FFVAR" = "auto" ]; then \
		_VER="$$(pkg-config --modversion libavcodec 2>/dev/null || true)"; \
		if [ -z "$$_VER" ]; then \
			_VER="$$(ffmpeg -version 2>/dev/null | head -1 | sed -E 's/.*version ([0-9]+(\.[0-9]+)?).*/\1/' || true)"; \
		fi; \
		_MAJOR="$${_VER%%.*}"; \
		case "$$_MAJOR" in \
			4) FFVAR=ffmpeg44 ;; \
			5|6|7|8|9) FFVAR=ffmpeg61 ;; \
			*) FFVAR=ffmpeg44 ;; \
		esac; \
	fi

# ============================================================
# Go 环境预检
# ============================================================
check-go:
	@echo "🔍 检查 Go 环境..."
	@command -v go >/dev/null 2>&1 || { \
		echo "  ❌ 未找到 go 命令"; \
		echo "     安装: https://mirrors.aliyun.com/golang/"; \
		exit 1; \
	}
	@GO_VER=$$(go version | grep -oP 'go\K[0-9]+\.[0-9]+(\.[0-9]+)?'); \
	REQ="$(GO_MIN_VERSION)"; \
	if [ "$$(printf '%s\n' "$$REQ" "$$GO_VER" | sort -V | head -1)" != "$$REQ" ]; then \
		echo "  ❌ Go 版本过低: $$GO_VER (需要 >= $$REQ)"; \
		echo "     当前: $$(which go)"; \
		echo "     升级: https://mirrors.aliyun.com/golang/"; \
		exit 1; \
	fi; \
	echo "  ✅ Go $$GO_VER (>= $$REQ)"
	@echo "  GOPROXY: $${GOPROXY}"
	@PROXY_URL=$$(echo "$${GOPROXY}" | cut -d, -f1); \
	if curl -sf --connect-timeout 5 "$$PROXY_URL" >/dev/null 2>&1; then \
		echo "  ✅ Go proxy 可达 ($$PROXY_URL)"; \
	else \
		echo "  ⚠️  Go proxy ($$PROXY_URL) 不可达，将尝试备用源"; \
	fi
	@echo ""

# ============================================================
# 完整发布 (Release)
# ============================================================

# RISC-V K3 发布 (默认目标，K3 上直接编译)
# 支持 PACKAGE 参数: all(默认), host, node
# - host: cli, gateway, embed, owl (输出到 riscv64/host/)
# - node: rivision_node (输出到 riscv64/node/)
# - all:  host + node (输出到 riscv64/，包含两者)
#
# 目录结构:
#   dist/v1.0.0/riscv64/           # PACKAGE=host 或 node 时 (可增量)
#   dist/v1.0.0-YYYYMMDD-HHMMSS/riscv64/  # PACKAGE=all 时 (带时间戳)
#   ├── host/       # 主机端组件
#   ├── node/       # 算力节点
#   ├── install.sh
#   └── checksums.txt
#
# 增量构建示例:
#   make release PACKAGE=host  # 先构建 host
#   make release PACKAGE=node  # 再构建 node (追加到同一目录)
release:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  🚀 RiVision Release Build                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: riscv64 (RISC-V K3)"
	@echo "  Package:  $(PACKAGE)"
	@echo "  Profile:  $(NODE_PROFILE)"
	@echo "  Go:       $$(go version 2>/dev/null | grep -oP 'go[0-9]+\.[0-9]+(\.[0-9]+)?' || echo 'N/A')"
	@echo "  Proxy:    $${GOPROXY}"
	@echo ""
	@$(MAKE) check-go --no-print-directory
	@echo "[0/5] Cleaning previous build artifacts..."
	@if [ "$(PACKAGE)" = "all" ] || [ "$(PACKAGE)" = "host" ]; then \
		cd $(CURDIR)/../src/rivision_cli && rm -rf build/ 2>/dev/null || true; \
		cd $(CURDIR)/../src/rivision_gateway/inference-gateway-go && rm -rf build/ 2>/dev/null || true; \
		cd $(CURDIR)/../src/rivision_embed && rm -rf build/ 2>/dev/null || true; \
		cd $(CURDIR)/../src/rivision_owl && rm -rf build/ 2>/dev/null || true; \
	fi
	@DIST_BASE=$(CURDIR)/../dist/$(VERSION)/riscv64 && \
	mkdir -p $$DIST_BASE && \
	if [ "$(PACKAGE)" = "all" ] || [ "$(PACKAGE)" = "host" ]; then \
		HOST_DIR=$$DIST_BASE/host; \
		mkdir -p $$HOST_DIR/bin $$HOST_DIR/lib $$HOST_DIR/models/embed $$HOST_DIR/services $$HOST_DIR/owl/bin $$HOST_DIR/owl/config $$HOST_DIR/config $$HOST_DIR/mp4path; \
		echo "=== Packaging host ==="; \
		echo "  Profile:  riscv64"; \
		echo "  Platform: riscv64 (GOARCH=riscv64)"; \
		echo "  Output:   $$HOST_DIR"; \
		echo ""; \
		echo "[1/5] Building rivision-cli (GOARCH=riscv64)..."; \
		cd $(CURDIR)/../src/rivision_cli && make build-riscv64 VERSION=$(VERSION) --no-print-directory && \
		cp build/rivision-cli-linux-riscv64 $$HOST_DIR/bin/rivision-cli && \
		file $$HOST_DIR/bin/rivision-cli | grep -q "RISC-V" || (echo "ERROR: rivision-cli is not RISC-V binary!"; exit 1); \
		echo "      ✅ 打包完成: rivision-cli"; \
		echo "[2/5] Building rivision_gateway (GOARCH=riscv64)..."; \
		cd $(CURDIR)/../src/rivision_gateway/inference-gateway-go && make build-riscv64 VERSION=$(VERSION) --no-print-directory && \
		cp build/rivision_gateway $$HOST_DIR/bin/ && \
		file $$HOST_DIR/bin/rivision_gateway | grep -q "RISC-V" || (echo "ERROR: rivision_gateway is not RISC-V binary!"; exit 1); \
		echo "      ✅ 打包完成: rivision_gateway"; \
		echo "[3/5] Building rivision-embed-server (GOARCH=riscv64)..."; \
		cd $(CURDIR)/../src/rivision_embed && make build-riscv64 VERSION=$(VERSION) --no-print-directory && \
		cp build/rivision-embed-server-linux-riscv64 $$HOST_DIR/bin/rivision-embed-server && \
		file $$HOST_DIR/bin/rivision-embed-server | grep -q "RISC-V" || (echo "ERROR: rivision-embed-server is not RISC-V binary!"; exit 1); \
		echo "      ✅ 打包完成: rivision-embed-server"; \
		echo "[4/5] Building rivision-owl (GOARCH=riscv64)..."; \
		cd $(CURDIR)/../src/rivision_owl && make owl-riscv-quiet PROFILE=riscv DIST_DIR=$$HOST_DIR/owl --no-print-directory; \
		file $$HOST_DIR/owl/bin/owl | grep -q "RISC-V" || (echo "ERROR: owl is not RISC-V binary!"; exit 1); \
		cd $(CURDIR); \
		cp $(CURDIR)/../src/rivision_embed/models/embed/*.onnx $$HOST_DIR/models/embed/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_embed/third_party/spacemit-ort-current/lib/libonnxruntime*.so* $$HOST_DIR/lib/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_embed/third_party/spacemit-ort-current/lib/libspacemit_ep*.so* $$HOST_DIR/lib/ 2>/dev/null || true; \
		cp templates/rivision-gateway.service templates/rivision-cli.service templates/rivision-embed.service templates/rivision-owl.service $$HOST_DIR/services/ 2>/dev/null || true; \
		cp templates/.env.template $$HOST_DIR/config/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_cli/mp4path/*.mp4 $$HOST_DIR/mp4path/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_cli/internal/embed/binaries/go2rtc-linux-riscv64 $$HOST_DIR/bin/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_cli/internal/embed/binaries/rtsp_server-linux-riscv64 $$HOST_DIR/bin/ 2>/dev/null || true; \
		echo "      ✅ 打包完成: rivision-owl"; \
		echo "[5/5] Finalizing host package..."; \
		./scripts/generate_manifest.sh $$HOST_DIR $(VERSION) riscv64 $(GIT_COMMIT); \
		./scripts/checksum.sh $$HOST_DIR; \
		cp templates/install-host.sh $$HOST_DIR/install.sh; \
		cp templates/uninstall.sh $$HOST_DIR/; \
		chmod +x $$HOST_DIR/install.sh $$HOST_DIR/uninstall.sh; \
		echo "      ✅ 打包完成: host"; \
	fi && \
	_build_yolo_if_needed() { \
		if [ "$(PACKAGE)" = "all" ] || [ "$(PACKAGE)" = "node" ] || [ "$(PACKAGE)" = "node_yolo" ] || [ "$(PACKAGE)" = "node_all" ]; then \
			echo "[+] Building yolo-server (riscv64)..."; \
			cd $(CURDIR)/../src/rivision_node && make build-yolo-riscv64 --no-print-directory 2>&1 | tail -5; \
			cd $(CURDIR); \
		fi; \
	}; \
	_build_yolo_if_needed && \
	_package_node() { \
		local profile=$$1 outdir=$$2 desc=$$3 env_type=$$4; \
		mkdir -p $$outdir/config; \
		echo "[+] Packaging $$desc ($$profile)..."; \
		cd $(CURDIR)/../src/rivision_node && make package PROFILE=$$profile DIST_DIR=$$outdir --no-print-directory 2>&1 | grep -v "^make\|Entering\|Leaving" || true; \
		cd $(CURDIR); \
		if [ -f "templates/.env.$$env_type.template" ]; then \
			cp "templates/.env.$$env_type.template" $$outdir/config/.env.node.template; \
		else \
			cp templates/.env.node.template $$outdir/config/ 2>/dev/null || true; \
		fi; \
		./scripts/generate_manifest.sh $$outdir $(VERSION) riscv64 $(GIT_COMMIT); \
		./scripts/checksum.sh $$outdir; \
		cp templates/install-node.sh $$outdir/install.sh; \
		cp templates/uninstall.sh $$outdir/; \
		chmod +x $$outdir/install.sh $$outdir/uninstall.sh; \
		echo "      ✅ 打包完成: $$desc"; \
	}; \
	if [ "$(PACKAGE)" = "all" ] || [ "$(PACKAGE)" = "node" ]; then \
		_package_node "$(NODE_PROFILE)" "$$DIST_BASE/node" "node (完整)" "node"; \
	fi && \
	if [ "$(PACKAGE)" = "node_llama" ]; then \
		_package_node "riscv_b_llama" "$$DIST_BASE/node_llama" "node_llama (仅VLM)" "node_llama"; \
	fi && \
	if [ "$(PACKAGE)" = "node_yolo" ]; then \
		_package_node "riscv_b_yolo" "$$DIST_BASE/node_yolo" "node_yolo (仅YOLO)" "node_yolo"; \
	fi && \
	if [ "$(PACKAGE)" = "node_all" ]; then \
		_package_node "$(NODE_PROFILE)" "$$DIST_BASE/node" "node (完整)" "node"; \
		_package_node "riscv_b_llama" "$$DIST_BASE/node_llama" "node_llama (仅VLM)" "node_llama"; \
		_package_node "riscv_b_yolo" "$$DIST_BASE/node_yolo" "node_yolo (仅YOLO)" "node_yolo"; \
	fi && \
	echo "README: host/, node/, node_llama/, node_yolo/ 为独立可分发包" > $$DIST_BASE/README.md && \
	ln -sfn $(VERSION) $(CURDIR)/../dist/latest 2>/dev/null || true; \
	echo "" && \
	echo "╔══════════════════════════════════════════════════════════════╗" && \
	echo "║  ✅ Release Complete                                         ║" && \
	echo "╚══════════════════════════════════════════════════════════════╝" && \
	echo "" && \
	echo "  Output Packages:" && \
	echo "  ┌─────────────────────────────────────────────────────────────┐" && \
	([ -d "$$DIST_BASE/host" ] && printf "  │  %-12s %-6s cli, gateway, embed, owl              │\n" "host/" "$$(du -sh $$DIST_BASE/host | cut -f1)" || true) && \
	([ -d "$$DIST_BASE/node" ] && printf "  │  %-12s %-6s node (yolo + llama)                   │\n" "node/" "$$(du -sh $$DIST_BASE/node | cut -f1)" || true) && \
	([ -d "$$DIST_BASE/node_llama" ] && printf "  │  %-12s %-6s node (仅 llama/VLM)                  │\n" "node_llama/" "$$(du -sh $$DIST_BASE/node_llama | cut -f1)" || true) && \
	([ -d "$$DIST_BASE/node_yolo" ] && printf "  │  %-12s %-6s node (仅 yolo/检测)                  │\n" "node_yolo/" "$$(du -sh $$DIST_BASE/node_yolo | cut -f1)" || true) && \
	echo "  └─────────────────────────────────────────────────────────────┘" && \
	echo "" && \
	echo "  Deploy Commands:" && \
	([ -d "$$DIST_BASE/host" ] && echo "    scp -r host/ target:/tmp/ && cd /tmp/host && ./install.sh" || true) && \
	([ -d "$$DIST_BASE/node" ] && echo "    scp -r node/ target:/tmp/ && cd /tmp/node && ./install.sh" || true) && \
	([ -d "$$DIST_BASE/node_llama" ] && echo "    scp -r node_llama/ target:/tmp/ && cd /tmp/node_llama && ./install.sh" || true) && \
	([ -d "$$DIST_BASE/node_yolo" ] && echo "    scp -r node_yolo/ target:/tmp/ && cd /tmp/node_yolo && ./install.sh" || true) && \
	echo ""

# 兼容别名
release-riscv64: release

# x86_64 发布 (内部执行器)
# 支持 PACKAGE 参数: all(默认), host, node
# 目录结构与 riscv64 一致
_release-x86-do:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  🚀 RiVision Release Build                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: x86_64"
	@echo "  Package:  $(PACKAGE)"
	@echo "  Go:       $$(go version 2>/dev/null | grep -oP 'go[0-9]+\.[0-9]+(\.[0-9]+)?' || echo 'N/A')"
	@echo "  Proxy:    $${GOPROXY}"
	@$(MAKE) check-go --no-print-directory
	@$(DETECT_FFMPEG); \
	echo "  FFmpeg:   $$FFVAR"; \
	echo ""; \
	DIST_BASE=$(CURDIR)/../dist/$(VERSION)/x86_64; \
	mkdir -p $$DIST_BASE; \
	if [ "$(PACKAGE)" = "all" ] || [ "$(PACKAGE)" = "host" ]; then \
		HOST_DIR=$$DIST_BASE/host; \
		mkdir -p $$HOST_DIR/bin $$HOST_DIR/lib $$HOST_DIR/models/embed $$HOST_DIR/services $$HOST_DIR/owl/bin $$HOST_DIR/owl/config $$HOST_DIR/config $$HOST_DIR/mp4path; \
		echo "=== Packaging host ==="; \
		echo "  Profile:  x86"; \
		echo "  Platform: amd64 (GOARCH=amd64)"; \
		echo "  Output:   $$HOST_DIR"; \
		echo ""; \
		echo "[1/5] Building rivision-cli..."; \
		case "$$FFVAR" in \
			ffmpeg44) cd $(CURDIR)/../src/rivision_cli && make _build-x86-ffmpeg44-do VERSION=$(VERSION) --no-print-directory && cp build/rivision-cli-linux-amd64-ffmpeg44 $$HOST_DIR/bin/rivision-cli ;; \
			ffmpeg61) cd $(CURDIR)/../src/rivision_cli && make _build-x86-do VERSION=$(VERSION) --no-print-directory && cp build/rivision-cli-linux-amd64 $$HOST_DIR/bin/rivision-cli ;; \
			*) echo "ERROR: unsupported FFMPEG_VARIANT='$$FFVAR'"; exit 1 ;; \
		esac; \
		echo "      ✅ 打包完成: rivision-cli"; \
		echo "[2/5] Building rivision_gateway..."; \
		cd $(CURDIR)/../src/rivision_gateway/inference-gateway-go && make build-x86 VERSION=$(VERSION) --no-print-directory && \
		cp build/rivision_gateway $$HOST_DIR/bin/; \
		echo "      ✅ 打包完成: rivision_gateway"; \
		echo "[3/5] Building rivision-embed-server..."; \
		cd $(CURDIR)/../src/rivision_embed && make build-x86 VERSION=$(VERSION) --no-print-directory && \
		cp build/rivision-embed-server-linux-amd64 $$HOST_DIR/bin/rivision-embed-server; \
		echo "      ✅ 打包完成: rivision-embed-server"; \
		echo "[4/5] Building rivision-owl..."; \
		cd $(CURDIR)/../src/rivision_owl && make owl-x86-quiet PROFILE=x86 DIST_DIR=$$HOST_DIR/owl --no-print-directory; \
		cd $(CURDIR); \
		cp $(CURDIR)/../src/rivision_embed/models/embed/*.onnx $$HOST_DIR/models/embed/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_embed/third_party/onnxruntime_x86/libonnxruntime*.so* $$HOST_DIR/lib/ 2>/dev/null || true; \
		cp templates/rivision-gateway.service templates/rivision-cli.service templates/rivision-embed.service templates/rivision-owl.service $$HOST_DIR/services/ 2>/dev/null || true; \
		cp templates/.env.template $$HOST_DIR/config/ 2>/dev/null || true; \
		cp $(CURDIR)/../src/rivision_cli/mp4path/*.mp4 $$HOST_DIR/mp4path/ 2>/dev/null || true; \
		case "$$FFVAR" in \
			ffmpeg44) cp $(CURDIR)/../src/rivision_cli/internal/embed/binaries/platform/x86_64-ffmpeg44/go2rtc-linux-amd64 $$HOST_DIR/bin/ 2>/dev/null || true; \
			          cp $(CURDIR)/../src/rivision_cli/internal/embed/binaries/platform/x86_64-ffmpeg44/rtsp_server-linux-amd64 $$HOST_DIR/bin/ 2>/dev/null || true ;; \
			ffmpeg61) cp $(CURDIR)/../src/rivision_cli/internal/embed/binaries/platform/x86_64-ffmpeg61/go2rtc-linux-amd64 $$HOST_DIR/bin/ 2>/dev/null || true; \
			          cp $(CURDIR)/../src/rivision_cli/internal/embed/binaries/platform/x86_64-ffmpeg61/rtsp_server-linux-amd64 $$HOST_DIR/bin/ 2>/dev/null || true ;; \
		esac; \
		echo "      ✅ 打包完成: rivision-owl"; \
		echo "[5/5] Finalizing host package..."; \
		./scripts/generate_manifest.sh $$HOST_DIR $(VERSION) x86_64 $(GIT_COMMIT); \
		./scripts/checksum.sh $$HOST_DIR; \
		cp templates/install-host.sh $$HOST_DIR/install.sh; \
		cp templates/uninstall.sh $$HOST_DIR/; \
		chmod +x $$HOST_DIR/install.sh $$HOST_DIR/uninstall.sh; \
		echo "      ✅ 打包完成: host"; \
	fi; \
	if [ "$(PACKAGE)" = "all" ] || [ "$(PACKAGE)" = "node" ]; then \
		NODE_DIR=$$DIST_BASE/node; \
		mkdir -p $$NODE_DIR/config; \
		echo "[+] Packaging rivision_node (x86)..."; \
		cd $(CURDIR)/../src/rivision_node && make package PROFILE=x86 DIST_DIR=$$NODE_DIR --no-print-directory 2>&1 | grep -v "^make\\|Entering\\|Leaving" || true; \
		cd $(CURDIR); \
		cp templates/.env.node.template $$NODE_DIR/config/ 2>/dev/null || true; \
		echo "      ✅ 打包完成: rivision_node"; \
		echo "[+] Finalizing node package..."; \
		./scripts/generate_manifest.sh $$NODE_DIR $(VERSION) x86_64 $(GIT_COMMIT); \
		./scripts/checksum.sh $$NODE_DIR; \
		cp templates/install-node.sh $$NODE_DIR/install.sh; \
		cp templates/uninstall.sh $$NODE_DIR/; \
		chmod +x $$NODE_DIR/install.sh $$NODE_DIR/uninstall.sh; \
	fi; \
	echo "README: host/ 和 node/ 为独立可分发包" > $$DIST_BASE/README.md; \
	ln -sfn $(VERSION) $(CURDIR)/../dist/latest 2>/dev/null || true; \
	echo ""; \
	echo "╔══════════════════════════════════════════════════════════════╗"; \
	echo "║  ✅ Release Complete                                         ║"; \
	echo "╚══════════════════════════════════════════════════════════════╝"; \
	echo ""; \
	echo "  Output Packages:"; \
	echo "  ┌─────────────────────────────────────────────────────────────┐"; \
	([ -d "$$DIST_BASE/host" ] && printf "  │  %-8s %-6s cli, gateway, embed, owl                   │\n" "host/" "$$(du -sh $$DIST_BASE/host | cut -f1)" || true); \
	([ -d "$$DIST_BASE/node" ] && printf "  │  %-8s %-6s node-agent, llama.cpp, yolo-server         │\n" "node/" "$$(du -sh $$DIST_BASE/node | cut -f1)" || true); \
	echo "  └─────────────────────────────────────────────────────────────┘"; \
	echo ""; \
	echo "  Deploy Commands:"; \
	([ -d "$$DIST_BASE/host" ] && echo "    scp -r host/ target:/tmp/ && cd /tmp/host && ./install.sh" || true); \
	([ -d "$$DIST_BASE/node" ] && echo "    scp -r node/ target:/tmp/ && cd /tmp/node && ./install.sh" || true); \
	echo ""

# x86_64 发布 (包含 cli-web 增量构建)
release-x86: cli-web _release-x86-do

# ARM64 发布 (NVIDIA Jetson Orin)
# 仅支持 node 组件，无 host 组件
# 工具链: Bootlin 2024.05 (GCC 14.2.0, GLIBC 2.39)
release-arm64:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  🚀 RiVision Release Build                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: arm64 (NVIDIA Jetson Orin)"
	@echo "  Package:  node only"
	@echo ""
	@NODE_SRC=$(CURDIR)/../src/rivision_node && \
	DIST_BASE=$(CURDIR)/../dist/$(VERSION)/arm64 && \
	NODE_DIR=$$DIST_BASE/node && \
	mkdir -p $$NODE_DIR/config && \
	echo "[1/3] Building yolo-server (cross-compilation)..." && \
	cd $$NODE_SRC && make build-yolo-arm64 && \
	echo "[2/3] Packaging rivision_node (arm_gpu)..." && \
	cd $$NODE_SRC && make package PROFILE=arm_gpu DIST_DIR=$$NODE_DIR && \
	cp $(CURDIR)/templates/.env.node.template $$NODE_DIR/config/ 2>/dev/null || true && \
	echo "[3/3] Finalizing node package..." && \
	$(CURDIR)/scripts/generate_manifest.sh $$NODE_DIR $(VERSION) arm64 $(GIT_COMMIT) && \
	$(CURDIR)/scripts/checksum.sh $$NODE_DIR && \
	cp $(CURDIR)/templates/install-node.sh $$NODE_DIR/install.sh && \
	cp $(CURDIR)/templates/uninstall.sh $$NODE_DIR/ && \
	chmod +x $$NODE_DIR/install.sh $$NODE_DIR/uninstall.sh && \
	ln -sfn $(VERSION) $(CURDIR)/../dist/latest 2>/dev/null || true && \
	echo "" && \
	echo "╔══════════════════════════════════════════════════════════════╗" && \
	echo "║  ✅ Release Complete                                         ║" && \
	echo "╚══════════════════════════════════════════════════════════════╝" && \
	echo "" && \
	echo "  Output Packages:" && \
	echo "  ┌─────────────────────────────────────────────────────────────┐" && \
	printf "  │  %-8s %-6s node-agent, llama.cpp, yolo-server         │\n" "node/" "$$(du -sh $$NODE_DIR | cut -f1)" && \
	echo "  └─────────────────────────────────────────────────────────────┘" && \
	echo "" && \
	echo "  Deploy Command:" && \
	echo "    scp -r node/ jetson:/tmp/ && cd /tmp/node && ./install.sh" && \
	echo ""

# 单独构建 ARM64 node
# 注: yolo-server.arm64 需在 Jetson 上编译后复制到 yolo-server/bin/
#     交叉编译会因 GLIBC 版本不匹配失败
node-arm64:
	@if [ ! -f "$(SRC_DIR)/rivision_node/yolo-server/bin/yolo-server.arm64" ]; then \
		echo "ERROR: yolo-server.arm64 not found. Please build on Jetson first."; \
		echo "  Or extract from: yolo-server/third_party/yolo-server_gpu.tgz"; \
		exit 1; \
	fi
	cd $(SRC_DIR)/rivision_node && make package PROFILE=arm_gpu DIST_DIR=$(CURDIR)/dist/rivision_node

# ============================================================
# 单组件构建: CLI
# ============================================================

cli-web:
	@cd $(SRC_DIR)/rivision_cli && make web --no-print-directory

cli-web-build:
	cd $(SRC_DIR)/rivision_cli && make web-build

_cli-x86-do:
	@$(DETECT_FFMPEG); \
	echo "[cli-x86] FFmpeg variant: $$FFVAR"; \
	DIST_DIR=$(CURDIR)/dist/rivision_cli; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services; \
	case "$$FFVAR" in \
		ffmpeg44) cd $(SRC_DIR)/rivision_cli && make cli-x86-ffmpeg44 DIST_DIR=$$DIST_DIR ;; \
		ffmpeg61) cd $(SRC_DIR)/rivision_cli && make cli-x86 DIST_DIR=$$DIST_DIR ;; \
		*) echo "ERROR: unsupported FFMPEG_VARIANT='$$FFVAR'"; exit 1 ;; \
	esac; \
	cp $(CURDIR)/templates/rivision-cli.service $$DIST_DIR/services/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true

cli-x86: cli-web _cli-x86-do

cli-riscv64:
	@DIST_DIR=$(CURDIR)/dist/rivision_cli; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services; \
	cd $(SRC_DIR)/rivision_cli && make cli-riscv64 DIST_DIR=$$DIST_DIR; \
	cp $(CURDIR)/templates/rivision-cli.service $$DIST_DIR/services/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true

# ============================================================
# 单组件构建: Embed
# ============================================================

embed-x86:
	@DIST_DIR=$(CURDIR)/dist/rivision_embed; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services $$DIST_DIR/models $$DIST_DIR/lib; \
	cd $(SRC_DIR)/rivision_embed && make embed-x86 VERSION=$(VERSION) DIST_DIR=$$DIST_DIR; \
	cp $(SRC_DIR)/rivision_embed/models/embed/*.onnx $$DIST_DIR/models/ 2>/dev/null || true; \
	cp $(SRC_DIR)/rivision_embed/third_party/onnxruntime_x86/libonnxruntime*.so* $$DIST_DIR/lib/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/rivision-embed.service $$DIST_DIR/services/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true

embed-riscv64:
	@DIST_DIR=$(CURDIR)/dist/rivision_embed; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services $$DIST_DIR/models $$DIST_DIR/lib; \
	cd $(SRC_DIR)/rivision_embed && make embed-riscv64 VERSION=$(VERSION) DIST_DIR=$$DIST_DIR; \
	cp $(SRC_DIR)/rivision_embed/models/embed/*.onnx $$DIST_DIR/models/ 2>/dev/null || true; \
	cp $(SRC_DIR)/rivision_embed/third_party/spacemit-ort-current/lib/libonnxruntime*.so* $$DIST_DIR/lib/ 2>/dev/null || true; \
	cp $(SRC_DIR)/rivision_embed/third_party/spacemit-ort-current/lib/libspacemit_ep*.so* $$DIST_DIR/lib/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/rivision-embed.service $$DIST_DIR/services/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true

# ============================================================
# 单组件构建: Gateway
# ============================================================

gateway-x86:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_gateway                                 ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  x86"
	@echo "  Platform: amd64 (GOARCH=amd64)"
	@echo "  Output:   $(CURDIR)/dist/rivision_gateway"
	@echo ""
	@DIST_DIR=$(CURDIR)/dist/rivision_gateway; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services; \
	echo "[1/3] Building Go binary..."; \
	cd $(SRC_DIR)/rivision_gateway/inference-gateway-go && make build-x86 VERSION=$(VERSION) --no-print-directory; \
	echo "[2/3] Copying files..."; \
	cp build/rivision_gateway $$DIST_DIR/bin/; \
	cp $(CURDIR)/templates/.env.gateway.template $$DIST_DIR/config/.env 2>/dev/null || true; \
	cp $(CURDIR)/templates/rivision-gateway.service $$DIST_DIR/services/ 2>/dev/null || true; \
	echo "[3/3] Generating install script..."; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true; \
	echo ""; \
	echo "╔══════════════════════════════════════════════════════════════╗"; \
	echo "║  ✅ Package Complete                                         ║"; \
	echo "╚══════════════════════════════════════════════════════════════╝"; \
	echo "  Output: $$DIST_DIR"; \
	echo "  Size:   $$(du -sh $$DIST_DIR | cut -f1)"; \
	echo ""

gateway-riscv64:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_gateway                                 ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  riscv64"
	@echo "  Platform: riscv64 (GOARCH=riscv64)"
	@echo "  Output:   $(CURDIR)/dist/rivision_gateway"
	@echo ""
	@DIST_DIR=$(CURDIR)/dist/rivision_gateway; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services; \
	echo "[1/3] Building Go binary..."; \
	cd $(SRC_DIR)/rivision_gateway/inference-gateway-go && make build-riscv64 VERSION=$(VERSION) --no-print-directory; \
	echo "[2/3] Copying files..."; \
	cp build/rivision_gateway $$DIST_DIR/bin/; \
	cp $(CURDIR)/templates/.env.gateway.template $$DIST_DIR/config/.env 2>/dev/null || true; \
	cp $(CURDIR)/templates/rivision-gateway.service $$DIST_DIR/services/ 2>/dev/null || true; \
	echo "[3/3] Generating install script..."; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true; \
	echo ""; \
	echo "╔══════════════════════════════════════════════════════════════╗"; \
	echo "║  ✅ Package Complete                                         ║"; \
	echo "╚══════════════════════════════════════════════════════════════╝"; \
	echo "  Output: $$DIST_DIR"; \
	echo "  Size:   $$(du -sh $$DIST_DIR | cut -f1)"; \
	echo ""

# ============================================================
# 单组件构建: Node
# ============================================================

node-x86:
	cd $(SRC_DIR)/rivision_node && make package PROFILE=x86 DIST_DIR=$(CURDIR)/dist/rivision_node

node-riscv64:
	cd $(SRC_DIR)/rivision_node && make package PROFILE=$(NODE_PROFILE) DIST_DIR=$(CURDIR)/dist/rivision_node

node-riscv64-llama:
	@echo "=== Packaging node (llama only) ==="
	cd $(SRC_DIR)/rivision_node && make package PROFILE=riscv_b_llama DIST_DIR=$(CURDIR)/dist/rivision_node_llama

node-riscv64-yolo:
	@echo "=== Packaging node (yolo only) ==="
	cd $(SRC_DIR)/rivision_node && make package PROFILE=riscv_b_yolo DIST_DIR=$(CURDIR)/dist/rivision_node_yolo

# ============================================================
# yolo-server 编译 (不含打包)
# ============================================================

yolo-server-x86:
	@echo "=== Building yolo-server for x86_64 ==="
	cd $(SRC_DIR)/rivision_node/yolo-server && mkdir -p build-x86 && cd build-x86 && \
		cmake .. -DCMAKE_BUILD_TYPE=Release && make -j$$(nproc)
	@mkdir -p $(SRC_DIR)/rivision_node/yolo-server/bin
	cp $(SRC_DIR)/rivision_node/yolo-server/build-x86/yolo-server $(SRC_DIR)/rivision_node/yolo-server/bin/yolo-server.x86_64
	@echo "✅ Output: src/rivision_node/yolo-server/bin/yolo-server.x86_64"

yolo-server-riscv64:
	@echo "=== Building yolo-server for RISC-V 64 ==="
	cd $(SRC_DIR)/rivision_node/yolo-server && ./scripts/build-riscv64.sh
	@echo "✅ Output: src/rivision_node/yolo-server/bin/yolo-server.riscv64"

yolo-server-arm64:
	@echo "=== Building yolo-server for ARM64 ==="
	cd $(SRC_DIR)/rivision_node/yolo-server && ./scripts/build-arm64.sh
	@echo "✅ Output: src/rivision_node/yolo-server/bin/yolo-server.arm64"

update-third-party:
	@../scripts/update-third-party.sh

# ============================================================
# 单组件构建: OWL
# ============================================================

owl-x86:
	cd $(SRC_DIR)/rivision_owl && make owl-x86 PROFILE=x86 DIST_DIR=$(CURDIR)/dist/rivision_owl
	cp $(CURDIR)/templates/component-install.sh $(CURDIR)/dist/rivision_owl/install.sh
	chmod +x $(CURDIR)/dist/rivision_owl/install.sh

owl-riscv owl-riscv64:
	cd $(SRC_DIR)/rivision_owl && make owl-riscv PROFILE=riscv DIST_DIR=$(CURDIR)/dist/rivision_owl
	cp $(CURDIR)/templates/component-install.sh $(CURDIR)/dist/rivision_owl/install.sh
	chmod +x $(CURDIR)/dist/rivision_owl/install.sh

owl-sim:
	cd $(SRC_DIR)/rivision_owl && make owl-sim PROFILE=x86 DIST_DIR=$(CURDIR)/dist/rivision_owl

owl-gb28181-sim:
	cd $(SRC_DIR)/rivision_owl && make owl-gb28181-sim PROFILE=x86 DIST_DIR=$(CURDIR)/dist/rivision_owl

owl-onvif-sim:
	cd $(SRC_DIR)/rivision_owl && make owl-onvif-sim PROFILE=x86 DIST_DIR=$(CURDIR)/dist/rivision_owl

owl-web:
	cd $(SRC_DIR)/rivision_owl && make owl-web

check-zlm:
	cd $(SRC_DIR)/rivision_owl && make $@

# ZLMediaKit 源码编译 (从 src/rivision_owl/ZLMediaKit/ 编译)
zlm-x86:
	cd $(SRC_DIR)/rivision_owl && make zlm-x86

zlm-riscv64:
	cd $(SRC_DIR)/rivision_owl && make zlm-riscv64

# ============================================================
# 工具目标
# ============================================================

clean:
	@echo "Cleaning all build artifacts..."
	@echo "  [1/6] rivision_cli..."
	@cd $(SRC_DIR)/rivision_cli && make clean 2>/dev/null || true
	@echo "  [2/6] rivision_embed..."
	@cd $(SRC_DIR)/rivision_embed && make clean 2>/dev/null || true
	@echo "  [3/6] rivision_gateway..."
	@cd $(SRC_DIR)/rivision_gateway/inference-gateway-go && make clean 2>/dev/null || true
	@echo "  [4/6] rivision_node..."
	@cd $(SRC_DIR)/rivision_node && make clean 2>/dev/null || true
	@echo "  [5/6] rivision_owl..."
	@cd $(SRC_DIR)/rivision_owl && make clean 2>/dev/null || true
	@echo "  [6/6] dist/..."
	@rm -rf dist/*
	@echo "  ✅ 清理完成"

help:
	@echo "============================================================"
	@echo "RiVision Deploy - 构建系统"
	@echo "============================================================"
	@echo ""
	@echo "用法: make <目标> [PACKAGE=all] [VERSION=vX.Y.Z] [NODE_PROFILE=riscv_b]"
	@echo ""
	@echo "完整发布 (支持 PACKAGE 参数: all|host|node):"
	@echo "  release                           - K3 RISC-V 发布包 (默认)"
	@echo "  release-riscv64                   - 同 release (兼容别名)"
	@echo "  release-x86                       - x86_64 发布包 (含 cli-web)"
	@echo "  release-arm64                     - ARM64 Jetson Orin 发布包 (仅 node)"
	@echo ""
	@echo "  PACKAGE 选项 (release/release-x86):"
	@echo "    all        (默认) - 完整包 (host + node)"
	@echo "    host              - 主机端 (cli, gateway, embed, owl)"
	@echo "    node              - 完整 node (yolo + llama)"
	@echo "    node_llama        - 仅 llama 的 node (VLM 图像描述)"
	@echo "    node_yolo         - 仅 yolo 的 node (目标检测)"
	@echo "    node_all          - 同时打包 3 种 node 变体"
	@echo ""
	@echo "  注: release-arm64 仅支持完整 node，无需 PACKAGE 参数"
	@echo ""
	@echo "单组件构建 (输出到 dist/rivision_xxx/, 含 install.sh):"
	@echo "  cli-x86 / cli-riscv64             - rivision_cli"
	@echo "  embed-x86 / embed-riscv64         - rivision_embed"
	@echo "  gateway-x86 / gateway-riscv64     - rivision_gateway"
	@echo "  owl-x86 / owl-riscv64             - rivision_owl (含 ZLMediaKit)"
	@echo "  node-x86 / node-riscv64 / node-arm64 - rivision_node (完整: yolo+llama)"
	@echo "  node-riscv64-llama                - rivision_node (仅 llama)"
	@echo "  node-riscv64-yolo                 - rivision_node (仅 yolo)"
	@echo ""
	@echo "CLI 相关:"
	@echo "  cli-web                           - 增量构建 Web 前端"
	@echo "  cli-web-build                     - 强制构建 Web 前端"
	@echo ""
	@echo "OWL 相关:"
	@echo "  owl-sim                           - 所有模拟器 (gb28181 + onvif)"
	@echo "  owl-gb28181-sim                   - GB28181 模拟器"
	@echo "  owl-onvif-sim                     - ONVIF 模拟器"
	@echo "  check-zlm                         - 检查 ZLMediaKit 状态 (owl-* 已自动包含)"
	@echo ""
	@echo "ZLMediaKit 源码编译 (从 src/rivision_owl/ZLMediaKit/):"
	@echo "  zlm-x86                           - x86_64 本地编译"
	@echo "  zlm-riscv64                       - RISC-V 交叉编译"
	@echo ""
	@echo "yolo-server 编译 (从 src/rivision_node/yolo-server/):"
	@echo "  yolo-server-x86                   - x86_64 本地编译"
	@echo "  yolo-server-riscv64               - RISC-V 交叉编译"
	@echo "  yolo-server-arm64                 - ARM64 交叉编译"
	@echo ""
	@echo "工具:"
	@echo "  clean                             - 清理所有构建产物"
	@echo "  update-third-party                - 更新 third_party 版本并在 ORT 变更时重编译 yolo"
	@echo "  help                              - 显示此帮助"
	@echo ""
	@echo "变量:"
	@echo "  PACKAGE        - 打包内容 (all|host|node|node_llama|node_yolo|node_all)"
	@echo "  VERSION        - 版本号 (默认: git tag)"
	@echo "  NODE_PROFILE   - 节点配置 (x86|riscv_a|riscv_b|riscv_b_llama|riscv_b_yolo)"
	@echo "  FFMPEG_VARIANT - FFmpeg 变体 (auto|ffmpeg44|ffmpeg61, 默认: auto)"
	@echo ""
	@echo "示例:"
	@echo "  make release                                 # 完整 K3 发布 (host + node)"
	@echo "  make release PACKAGE=host                    # K3 只打包主机端"
	@echo "  make release PACKAGE=node                    # K3 完整 node (yolo+llama)"
	@echo "  make release PACKAGE=node_llama              # K3 仅 VLM 节点"
	@echo "  make release PACKAGE=node_yolo               # K3 仅 YOLO 节点"
	@echo "  make release PACKAGE=node_all                # K3 3种节点全打包"
	@echo "  make release-arm64                           # ARM64 Jetson 发布"
	@echo "  make yolo-server-riscv64 node-riscv64        # 编译 yolo + 打包 node"
	@echo ""
	@echo "单组件安装:"
	@echo "  cd dist/rivision_cli && sudo ./install.sh    # 安装 CLI"
	@echo "  cd dist/rivision_embed && sudo ./install.sh  # 安装 Embed"

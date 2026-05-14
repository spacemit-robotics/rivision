# ============================================================
# RiVision K3 Deploy - Top-level Build System
# 统一构建组件:
#   rivision_cli, rivision_gateway, rivision_embed, rivision_owl, rivision_node
#   rivision_hub, rivision_worker (C++ with built-in inference), rivision_app_web
# ============================================================

# 默认目标: release (K3 RISC-V)
.DEFAULT_GOAL := release

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
GO_MIN_VERSION := 1.24.0
export GOPROXY := https://goproxy.cn,https://proxy.golang.org,direct
export GONOSUMDB := *

# === Server LDFLAGS (hub / worker) ===
SERVER_LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.gitCommit=$(GIT_COMMIT) \
	-X main.buildTime=$(TIMESTAMP)

# === .PHONY 声明 (集中管理) ===
.PHONY: help clean check-env check-env-hub check-env-worker check-go
.PHONY: release
.PHONY: cli cli-web cli-web-build
.PHONY: embed
.PHONY: gateway
.PHONY: node node-llama node-yolo
.PHONY: yolo-server
.PHONY: owl owl-sim owl-gb28181-sim owl-onvif-sim owl-web check-zlm zlm
.PHONY: hub worker
.PHONY: release-server-host release-server-node

# ============================================================
# check-env-hub — Hub 侧专项检查
#   检查: gcc(CGO) / Go≥MIN / libsqlite3-dev / Node.js(可选) / GOPROXY
#   触发: hub, release-server-host
# ============================================================
check-env-hub:
	@echo "🔍 [hub] 构建环境检查..."
	@ERRORS=0; \
	echo ""; \
	echo "  --- 编译工具 ---"; \
	command -v gcc >/dev/null 2>&1 && \
		echo "  ✅ gcc $$(gcc -dumpversion)  (CGO 必需)" || { \
		echo "  ❌ gcc 未安装 — sudo apt install -y build-essential"; \
		ERRORS=$$((ERRORS+1)); \
	}; \
	if command -v go >/dev/null 2>&1; then \
		GO_VER=$$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+(\.[0-9]+)?' | sed 's/^go//' | head -1); \
		REQ="$(GO_MIN_VERSION)"; \
		if [ -z "$$GO_VER" ]; then \
			echo "  ❌ Go 安装异常（go version 无法解析）"; ERRORS=$$((ERRORS+1)); \
		elif [ "$$(printf '%s\n%s\n' "$$REQ" "$$GO_VER" | sort -V | head -1)" != "$$REQ" ]; then \
			echo "  ❌ Go $$GO_VER 过低，需要 >= $$REQ"; \
			echo "     wget https://mirrors.aliyun.com/golang/go$$REQ.linux-riscv64.tar.gz"; \
			echo "     sudo tar -C /usr/local -xzf go$$REQ.linux-riscv64.tar.gz"; \
			ERRORS=$$((ERRORS+1)); \
		else \
			echo "  ✅ Go $$GO_VER"; \
		fi; \
	else \
		echo "  ❌ Go 未安装（hub 必需，>= $(GO_MIN_VERSION)）"; \
		echo "     wget https://mirrors.aliyun.com/golang/go$(GO_MIN_VERSION).linux-riscv64.tar.gz"; \
		echo "     sudo tar -C /usr/local -xzf go$(GO_MIN_VERSION).linux-riscv64.tar.gz"; \
		ERRORS=$$((ERRORS+1)); \
	fi; \
	dpkg -s libsqlite3-dev >/dev/null 2>&1 && \
		echo "  ✅ libsqlite3-dev" || { \
		echo "  ❌ libsqlite3-dev 未安装 — sudo apt install -y libsqlite3-dev"; \
		ERRORS=$$((ERRORS+1)); \
	}; \
	echo ""; \
	echo "  --- 可选（app_web） ---"; \
	command -v node >/dev/null 2>&1 && \
		echo "  ✅ Node.js $$(node --version) / npm $$(npm --version)" || \
		echo "  ⚠️  Node.js/npm 未安装（仅 app_web 需要）— sudo apt install -y nodejs npm"; \
	echo ""; \
	echo "  --- 网络 ---"; \
	PROXY_URL=$$(echo "$${GOPROXY}" | cut -d, -f1); \
	curl -sf --connect-timeout 5 "$$PROXY_URL" >/dev/null 2>&1 && \
		echo "  ✅ GOPROXY 可达 ($$PROXY_URL)" || \
		echo "  ⚠️  GOPROXY ($$PROXY_URL) 不可达，将尝试备用源"; \
	echo ""; \
	if [ $$ERRORS -gt 0 ]; then \
		echo "  ❌ Hub 侧 $$ERRORS 个缺失项，修复后再执行 make hub"; exit 1; \
	fi; \
	echo "  ✅ Hub 侧检查通过"
	@echo ""

# ============================================================
# check-env-worker — Worker 侧专项检查（自动 apt 安装缺失包）
#   检查: gcc/g++/cmake/make / apt 包（自动安装）/
#         OpenCV(/opt/opencv-spacemit) / bundled SDK(third_party/)
#         /dev/mpp /dev/npu（仅信息，不阻塞）
#   触发: worker, release-server-node
# ============================================================
_WORKER_APT := libyaml-cpp-dev libsqlite3-dev libcurl4-openssl-dev \
               libavformat-dev libavcodec-dev libavutil-dev \
               libswresample-dev libswscale-dev
_WORKER_SRC := $(CURDIR)/../src/rivision_worker

check-env-worker:
	@echo "🔍 [worker] 构建环境检查..."
	@ERRORS=0; \
	echo ""; \
	echo "  --- 编译工具链 ---"; \
	for tool in gcc g++ cmake make; do \
		command -v $$tool >/dev/null 2>&1 && { \
			v=$$($$tool --version 2>&1 | head -1); printf "  ✅ %-8s %s\n" $$tool "$$v"; \
		} || { printf "  ❌ %-8s 未安装\n" $$tool; ERRORS=$$((ERRORS+1)); }; \
	done; \
	if [ $$ERRORS -gt 0 ]; then \
		echo "  修复: sudo apt install -y build-essential cmake"; exit 1; \
	fi; \
	echo ""; \
	echo "  --- apt 开发包（自动安装缺失项） ---"; \
	MISSING_APT=""; \
	for pkg in $(_WORKER_APT); do \
		dpkg -s $$pkg >/dev/null 2>&1 && \
			printf "  ✅ %s\n" $$pkg || { \
			printf "  ⚠️  %-36s→ 将自动安装\n" $$pkg; \
			MISSING_APT="$$MISSING_APT $$pkg"; \
		}; \
	done; \
	if [ -n "$$MISSING_APT" ]; then \
		echo ""; \
		echo "  → sudo apt install -y $$MISSING_APT"; \
		sudo apt-get update -qq 2>&1 | tail -1; \
		sudo apt-get install -y -q $$MISSING_APT 2>&1 | \
			grep -v "^Reading\|^Building\|^Hit\|^Get\|^Ign\|^Fetched" | tail -5; \
		echo "  ✅ apt 安装完成"; \
	fi; \
	echo ""; \
	echo "  --- OpenCV (SpacemiT) ---"; \
	_CV_CFG=/opt/opencv-spacemit/lib/cmake/opencv4/OpenCVConfig.cmake; \
	if [ -f "$$_CV_CFG" ]; then \
		CV_VER=$$(grep -m1 'OpenCV_VERSION ' /opt/opencv-spacemit/lib/cmake/opencv4/OpenCVConfig-version.cmake 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1); \
		echo "  ✅ /opt/opencv-spacemit  (OpenCV $${CV_VER:-?})"; \
	else \
		echo "  ❌ /opt/opencv-spacemit 未找到（需从 SpacemiT 获取并安装到该路径）"; \
		ERRORS=$$((ERRORS+1)); \
	fi; \
	echo ""; \
	echo "  --- Bundled SDK 完整性 (third_party/) ---"; \
	SDK_ERR=0; \
	check_file() { \
		if [ -e "$$1" ]; then printf "  ✅ %s\n" "$$2"; \
		else printf "  ❌ %s  缺失: %s\n" "$$2" "$$1"; SDK_ERR=$$(($$SDK_ERR+1)); fi; \
	}; \
	check_file "$(_WORKER_SRC)/third_party/spacemit-ort/include/onnxruntime_cxx_api.h" "ORT headers"; \
	check_file "$(_WORKER_SRC)/third_party/spacemit-ort/lib/libonnxruntime.so" "libonnxruntime.so"; \
	check_file "$(_WORKER_SRC)/third_party/spacemit-ort/lib/libspacemit_ep.so" "libspacemit_ep.so"; \
	check_file "$(_WORKER_SRC)/third_party/spacemit-llama.cpp/include/llama.h" "llama headers"; \
	check_file "$(_WORKER_SRC)/third_party/spacemit-llama.cpp/lib/libllama.so" "libllama.so"; \
	check_file "$(_WORKER_SRC)/third_party/mpp/include/sys/sys_api.h" "MPP headers"; \
	check_file "$(_WORKER_SRC)/third_party/mpp/lib/libmpp_demux.a" "libmpp_demux.a"; \
	if [ $$SDK_ERR -gt 0 ]; then \
		echo "  ❌ $$SDK_ERR 个 bundled SDK 文件缺失（请从 SpacemiT 获取 tar.gz 并解压到 third_party/）"; \
		ERRORS=$$((ERRORS+$$SDK_ERR)); \
	fi; \
	echo ""; \
	if [ $$ERRORS -gt 0 ]; then \
		echo "  ❌ Worker 侧 $$ERRORS 个问题，修复后再执行 make worker"; exit 1; \
	fi; \
	echo "  ✅ Worker 侧检查通过"
	@echo ""

# check-env = 全量检查（两侧都跑）
check-env: check-env-hub check-env-worker

# 兼容旧目标名
check-go: check-env-hub

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
	@$(MAKE) check-env --no-print-directory
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
		cd $(CURDIR)/../src/rivision_owl && make owl-quiet DIST_DIR=$$HOST_DIR/owl --no-print-directory; \
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

# ============================================================
# 单组件构建: CLI
# ============================================================

cli-web:
	@cd $(SRC_DIR)/rivision_cli && make web --no-print-directory

cli-web-build:
	cd $(SRC_DIR)/rivision_cli && make web-build

cli:
	@DIST_DIR=$(CURDIR)/dist/rivision_cli; \
	mkdir -p $$DIST_DIR/bin $$DIST_DIR/config $$DIST_DIR/services; \
	cd $(SRC_DIR)/rivision_cli && make cli-riscv64 DIST_DIR=$$DIST_DIR; \
	cp $(CURDIR)/templates/rivision-cli.service $$DIST_DIR/services/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/component-install.sh $$DIST_DIR/install.sh 2>/dev/null || true; \
	chmod +x $$DIST_DIR/install.sh 2>/dev/null || true

# ============================================================
# 单组件构建: Embed
# ============================================================

embed:
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

gateway:
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

node:
	cd $(SRC_DIR)/rivision_node && make package PROFILE=$(NODE_PROFILE) DIST_DIR=$(CURDIR)/dist/rivision_node

node-llama:
	@echo "=== Packaging node (llama only) ==="
	cd $(SRC_DIR)/rivision_node && make package PROFILE=riscv_b_llama DIST_DIR=$(CURDIR)/dist/rivision_node_llama

node-yolo:
	@echo "=== Packaging node (yolo only) ==="
	cd $(SRC_DIR)/rivision_node && make package PROFILE=riscv_b_yolo DIST_DIR=$(CURDIR)/dist/rivision_node_yolo

# ============================================================
# yolo-server 编译 (不含打包)
# ============================================================

yolo-server:
	@echo "=== Building yolo-server for RISC-V 64 ==="
	cd $(SRC_DIR)/rivision_node/yolo-server && ./scripts/build-riscv64.sh
	@echo "✅ Output: src/rivision_node/yolo-server/bin/yolo-server.riscv64"

# ============================================================
# 单组件构建: OWL
# ============================================================

owl:
	cd $(SRC_DIR)/rivision_owl && make owl DIST_DIR=$(CURDIR)/dist/rivision_owl
	cp $(CURDIR)/templates/component-install.sh $(CURDIR)/dist/rivision_owl/install.sh
	chmod +x $(CURDIR)/dist/rivision_owl/install.sh

owl-sim:
	cd $(SRC_DIR)/rivision_owl && make owl-sim DIST_DIR=$(CURDIR)/dist/rivision_owl

owl-gb28181-sim:
	cd $(SRC_DIR)/rivision_owl && make owl-gb28181-sim DIST_DIR=$(CURDIR)/dist/rivision_owl

owl-onvif-sim:
	cd $(SRC_DIR)/rivision_owl && make owl-onvif-sim DIST_DIR=$(CURDIR)/dist/rivision_owl

owl-web:
	cd $(SRC_DIR)/rivision_owl && make owl-web

check-zlm:
	cd $(SRC_DIR)/rivision_owl && make $@

# ZLMediaKit 源码编译 (从 src/rivision_owl/ZLMediaKit/ 编译)
zlm:
	cd $(SRC_DIR)/rivision_owl && make zlm

# ============================================================
# 单组件构建: Hub (Go, K3 native)
# ============================================================

hub: check-env-hub
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_hub                                     ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: riscv64 (K3 native)"
	@echo "  Output:   $(CURDIR)/dist/rivision_hub"
	@echo ""
	@COMP_DIR=$(CURDIR)/dist/rivision_hub; \
	mkdir -p $$COMP_DIR/bin $$COMP_DIR/config $$COMP_DIR/web $$COMP_DIR/services $$COMP_DIR/logs; \
	echo "[1/4] Building Web UI (rivision_app_web)..."; \
	if [ -d "$(SRC_DIR)/rivision_app_web" ] && command -v npm >/dev/null 2>&1; then \
		cd $(SRC_DIR)/rivision_app_web && npm install --silent && npm run build && \
		cp -r dist/* $$COMP_DIR/web/ 2>/dev/null || true; \
	elif [ -d "$(SRC_DIR)/rivision_hub/web" ]; then \
		cp -r $(SRC_DIR)/rivision_hub/web/* $$COMP_DIR/web/ 2>/dev/null || true; \
	else \
		echo "  ⚠️  Web UI 跳过 (未找到 rivision_app_web 或 npm)"; \
	fi; \
	echo "[2/4] Building Go binary..."; \
	cd $(SRC_DIR)/rivision_hub && \
		go build -ldflags "$(SERVER_LDFLAGS)" -o $$COMP_DIR/bin/rivision-hub ./cmd/hub; \
	echo "[3/4] Copying config and services..."; \
	cp $(SRC_DIR)/rivision_hub/config/hub.yaml $$COMP_DIR/config/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/rivision-hub.service $$COMP_DIR/services/ 2>/dev/null || true; \
	echo "[4/4] Generating install/uninstall scripts..."; \
	cp $(CURDIR)/templates/component-install.sh $$COMP_DIR/install.sh 2>/dev/null || true; \
	cp $(CURDIR)/templates/uninstall.sh $$COMP_DIR/ 2>/dev/null || true; \
	chmod +x $$COMP_DIR/*.sh 2>/dev/null || true; \
	echo ""; \
	echo "  ✅ rivision_hub build complete"; \
	echo "  Output: $$COMP_DIR  Size: $$(du -sh $$COMP_DIR | cut -f1)"; \
	echo ""

# ============================================================
# 单组件构建: Worker (C++, K3 native, built-in YOLO+VLM)
# ============================================================

worker: check-env-worker
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_worker (C++)                            ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: riscv64 (K3 native, SpacemiT MPP)"
	@echo "  Output:   $(CURDIR)/dist/rivision_worker"
	@echo ""
	@COMP_DIR=$(CURDIR)/dist/rivision_worker; \
	mkdir -p $$COMP_DIR/bin $$COMP_DIR/config $$COMP_DIR/services $$COMP_DIR/lib $$COMP_DIR/models; \
	echo "[1/4] Building C++ binary..."; \
	cd $(CURDIR)/../src/rivision_worker && \
		mkdir -p build && cd build && \
		cmake .. -DCMAKE_BUILD_TYPE=Release -Wno-dev && \
		make -j$$(nproc); \
	cp $(CURDIR)/../src/rivision_worker/build/rivision-worker $$COMP_DIR/bin/; \
	echo "[2/4] Copying config and services..."; \
	cp $(SRC_DIR)/rivision_worker/config/*.yaml $$COMP_DIR/config/ 2>/dev/null || true; \
	cp $(CURDIR)/templates/rivision-worker.service $$COMP_DIR/services/ 2>/dev/null || true; \
	echo "[3/4] Generating install/uninstall scripts..."; \
	cp $(CURDIR)/templates/component-install.sh $$COMP_DIR/install.sh 2>/dev/null || true; \
	cp $(CURDIR)/templates/uninstall.sh $$COMP_DIR/ 2>/dev/null || true; \
	chmod +x $$COMP_DIR/*.sh 2>/dev/null || true; \
	echo "[4/4] Done."; \
	echo ""; \
	echo "  ✅ rivision_worker build complete"; \
	echo "  Output: $$COMP_DIR  Size: $$(du -sh $$COMP_DIR | cut -f1)"; \
	echo ""

# ============================================================
# Server 产品级打包: Host 端 (hub + gateway + owl + app_web)
# 输出: dist/<VERSION>/riscv64/server-host/
# ============================================================

release-server-host: check-env-hub
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  🚀 Server Host: hub + gateway + owl + app_web                ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: riscv64 (K3 native)"
	@echo ""
	@DIST_BASE=$(CURDIR)/../dist/$(VERSION)/riscv64 && \
	SVR_DIR=$$DIST_BASE/server-host && \
	mkdir -p $$SVR_DIR/bin $$SVR_DIR/web $$SVR_DIR/config $$SVR_DIR/services $$SVR_DIR/owl/bin $$SVR_DIR/owl/config && \
	echo "[1/4] Building rivision_hub (含 rivision_app_web)..." && \
	if [ -d "$(SRC_DIR)/rivision_app_web" ] && command -v npm >/dev/null 2>&1; then \
		cd $(SRC_DIR)/rivision_app_web && npm install --silent && npm run build && \
		cp -r dist/* $$SVR_DIR/web/ 2>/dev/null || true && \
		echo "      ✅ rivision_app_web 构建完成"; \
	fi && \
	cd $(SRC_DIR)/rivision_hub && \
		go build -ldflags "$(SERVER_LDFLAGS)" -o $$SVR_DIR/bin/rivision-hub ./cmd/hub && \
	echo "      ✅ rivision-hub 构建完成" && \
	echo "[2/4] Building rivision_gateway..." && \
	cd $(SRC_DIR)/rivision_gateway/inference-gateway-go && make build-riscv64 VERSION=$(VERSION) --no-print-directory && \
	cp build/rivision_gateway $$SVR_DIR/bin/ && \
	echo "      ✅ rivision_gateway 构建完成" && \
	echo "[3/4] Building rivision_owl (Hub OWL for SIP-media split)..." && \
	cd $(SRC_DIR)/rivision_owl && make owl-quiet DIST_DIR=$$SVR_DIR/owl --no-print-directory 2>/dev/null && \
	echo "      ✅ rivision-owl 构建完成" && \
	rm -f $$SVR_DIR/owl/config/config.toml $$SVR_DIR/owl/config/.env $$SVR_DIR/owl/config/gb28181-sim.yaml 2>/dev/null || true && \
	rm -f $$SVR_DIR/owl/bin/gb28181-sim 2>/dev/null || true && \
	rm -rf $$SVR_DIR/owl/services $$SVR_DIR/owl/mp4path 2>/dev/null || true && \
	rmdir $$SVR_DIR/owl/data $$SVR_DIR/owl/logs 2>/dev/null || true && \
	echo "[4/4] Finalizing server-host package..." && \
	cp $(SRC_DIR)/rivision_hub/config/hub.yaml $$SVR_DIR/config/ 2>/dev/null || true && \
	cp $(CURDIR)/templates/rivision-hub.service $(CURDIR)/templates/rivision-gateway.service $(CURDIR)/templates/rivision-owl.service $$SVR_DIR/services/ 2>/dev/null || true && \
	cp $(CURDIR)/templates/.env.server.template $$SVR_DIR/config/ 2>/dev/null || true && \
	$(CURDIR)/scripts/generate_manifest.sh $$SVR_DIR $(VERSION) riscv64 $(GIT_COMMIT) 2>/dev/null || true && \
	$(CURDIR)/scripts/checksum.sh $$SVR_DIR 2>/dev/null || true && \
	cp $(CURDIR)/templates/install-server-host.sh $$SVR_DIR/install.sh && \
	cp $(CURDIR)/templates/uninstall.sh $$SVR_DIR/ 2>/dev/null || true && \
	chmod +x $$SVR_DIR/install.sh $$SVR_DIR/uninstall.sh 2>/dev/null || true && \
	ln -sfn $(VERSION) $(CURDIR)/../dist/latest 2>/dev/null || true && \
	echo "" && \
	echo "  ✅ Server Host Complete"

# ============================================================
# Server 产品级打包: Node 端 (worker + embed + go2rtc + owl)
# 输出: dist/<VERSION>/riscv64/server-node/
# 注: C++ Worker 已内置 YOLO+VLM 推理
#     OWL+ZLM 在 worker_owl 模式下需要 (GB28181 同子网场景)
# ============================================================

release-server-node: check-env-worker
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  🚀 Server Node: worker + embed + go2rtc + owl                ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Version:  $(VERSION)"
	@echo "  Platform: riscv64 (K3 native)"
	@echo "  Note:     C++ Worker has built-in YOLO+VLM inference"
	@echo ""
	@DIST_BASE=$(CURDIR)/../dist/$(VERSION)/riscv64 && \
	NODE_DIR=$$DIST_BASE/server-node && \
	mkdir -p $$NODE_DIR/bin $$NODE_DIR/config $$NODE_DIR/services $$NODE_DIR/lib \
	         $$NODE_DIR/models/embed $$NODE_DIR/models/yolo $$NODE_DIR/models/vlm && \
	echo "[1/4] Building rivision_worker (C++ with built-in inference)..." && \
	cd $(CURDIR)/../src/rivision_worker && \
		mkdir -p build && cd build && \
		cmake .. -DCMAKE_BUILD_TYPE=Release -Wno-dev && \
		make -j$$(nproc) && \
	cp $(CURDIR)/../src/rivision_worker/build/rivision-worker $$NODE_DIR/bin/ && \
	echo "      ✅ rivision-worker (C++) 构建完成" && \
	echo "[2/4] Building rivision_embed..." && \
	cd $(SRC_DIR)/rivision_embed && make build-riscv64 VERSION=$(VERSION) --no-print-directory && \
	cp build/rivision-embed-server-linux-riscv64 $$NODE_DIR/bin/rivision-embed-server && \
	echo "      ✅ rivision-embed 构建完成" && \
	echo "  [+] Copying models (yolo / embed / vlm)..." && \
	mkdir -p $$NODE_DIR/models/yolo/yolo11/detection \
	         $$NODE_DIR/models/yolo/yolov8/pose \
	         $$NODE_DIR/models/yolo/yolov8/segment \
	         $$NODE_DIR/models/yolo/yolov5/face && \
	cp $(SRC_DIR)/rivision_worker/models/yolo/yolo11/detection/910byolo11n_b2.q.onnx $$NODE_DIR/models/yolo/yolo11/detection/ 2>/dev/null || true && \
	cp $(SRC_DIR)/rivision_worker/models/yolo/yolov8/pose/*.onnx $$NODE_DIR/models/yolo/yolov8/pose/ 2>/dev/null || true && \
	cp $(SRC_DIR)/rivision_worker/models/yolo/yolov8/segment/*.onnx $$NODE_DIR/models/yolo/yolov8/segment/ 2>/dev/null || true && \
	cp $(SRC_DIR)/rivision_worker/models/yolo/yolov5/face/*.onnx $$NODE_DIR/models/yolo/yolov5/face/ 2>/dev/null || true && \
	cp $(SRC_DIR)/rivision_worker/models/embed/*.onnx $$NODE_DIR/models/embed/ 2>/dev/null || true && \
	if [ -d "$(SRC_DIR)/rivision_worker/models/vlm/fastvlm-mm-0.5b-q4_1" ]; then \
		cp -r $(SRC_DIR)/rivision_worker/models/vlm/fastvlm-mm-0.5b-q4_1 $$NODE_DIR/models/vlm/; \
	fi && \
	echo "[3/4] Finalizing server-node package..." && \
	cp $(SRC_DIR)/rivision_worker/config/*.yaml $$NODE_DIR/config/ 2>/dev/null || true && \
	cp $(CURDIR)/templates/rivision-worker.service $(CURDIR)/templates/rivision-embed.service $(CURDIR)/templates/rivision-go2rtc.service $(CURDIR)/templates/rivision-vlm.service $$NODE_DIR/services/ 2>/dev/null || true && \
	cp $(CURDIR)/templates/.env.server-node.template $$NODE_DIR/config/ 2>/dev/null || true && \
	echo "  [+] Bundling OWL+ZLM (必选组件)..." && \
	if [ ! -d "$(CURDIR)/dist/rivision_owl/bin" ]; then \
		echo ""; \
		echo "  ❌ 错误: OWL 组件未找到"; \
		echo ""; \
		cd $(CURDIR) && $(MAKE) owl --no-print-directory || { \
			echo ""; \
			echo "  ❌ OWL 构建失败，请手动运行:"; \
			echo "    make owl"; \
			echo ""; \
			exit 1; \
		}; \
	fi && \
	mkdir -p $$NODE_DIR/owl/bin $$NODE_DIR/owl/config && \
	cp $(CURDIR)/dist/rivision_owl/bin/owl $$NODE_DIR/owl/bin/ && \
	cp $(CURDIR)/dist/rivision_owl/bin/ZLMediaKit $$NODE_DIR/owl/bin/ && \
	cp $(CURDIR)/dist/rivision_owl/config/owl.toml.template $$NODE_DIR/owl/config/ && \
	if [ -d "$(CURDIR)/dist/rivision_owl/www" ]; then cp -r $(CURDIR)/dist/rivision_owl/www $$NODE_DIR/owl/; fi && \
	cp $(CURDIR)/templates/rivision-owl.service $$NODE_DIR/services/ && \
	echo "      ✅ OWL+ZLM bundled" && \
	echo "[4/4] Generating install/manifest..." && \
	$(CURDIR)/scripts/generate_manifest.sh $$NODE_DIR $(VERSION) riscv64 $(GIT_COMMIT) 2>/dev/null || true && \
	$(CURDIR)/scripts/checksum.sh $$NODE_DIR 2>/dev/null || true && \
	cp $(CURDIR)/templates/install-server-node.sh $$NODE_DIR/install.sh && \
	cp $(CURDIR)/templates/uninstall.sh $$NODE_DIR/ 2>/dev/null || true && \
	chmod +x $$NODE_DIR/install.sh $$NODE_DIR/uninstall.sh 2>/dev/null || true && \
	echo "" && \
	echo "  ✅ Server Node Complete" && \
	echo "  组件: worker (C++, built-in YOLO+VLM) + embed + owl" && \
	echo "  输出: $$NODE_DIR  ($$(du -sh $$NODE_DIR | cut -f1))" && \
	echo "  部署: scp -r server-node/ k3:/tmp/ && cd /tmp/server-node && ./install.sh" && \
	echo ""

# ============================================================
# 工具目标
# ============================================================

clean:
	@echo "Cleaning all build artifacts..."
	@echo "  [1/8] rivision_cli..."
	@cd $(SRC_DIR)/rivision_cli && make clean 2>/dev/null || true
	@echo "  [2/8] rivision_embed..."
	@cd $(SRC_DIR)/rivision_embed && make clean 2>/dev/null || true
	@echo "  [3/8] rivision_gateway..."
	@cd $(SRC_DIR)/rivision_gateway/inference-gateway-go && make clean 2>/dev/null || true
	@echo "  [4/8] rivision_node..."
	@cd $(SRC_DIR)/rivision_node && make clean 2>/dev/null || true
	@echo "  [5/8] rivision_owl..."
	@cd $(SRC_DIR)/rivision_owl && make clean 2>/dev/null || true
	@echo "  [6/8] rivision_hub..."
	@rm -rf $(SRC_DIR)/rivision_hub/bin 2>/dev/null || true
	@echo "  [7/8] rivision_worker..."
	@rm -rf $(SRC_DIR)/rivision_worker/build 2>/dev/null || true
	@echo "  [8/8] dist/..."
	@rm -rf dist/*
	@echo "  ✅ 清理完成"

help:
	@echo "============================================================"
	@echo "RiVision Deploy - K3 RISC-V 构建系统"
	@echo "============================================================"
	@echo ""
	@echo "用法: make [目标] [PACKAGE=all] [VERSION=vX.Y.Z] [NODE_PROFILE=riscv_b]"
	@echo ""
	@echo "发布构建 (支持 PACKAGE 参数: all|host|node):"
	@echo "  release (默认)                     - K3 RISC-V 完整发布包"
	@echo ""
	@echo "  PACKAGE 选项:"
	@echo "    all        (默认) - 完整包 (host + node)"
	@echo "    host              - 主机端 (cli, gateway, embed, owl)"
	@echo "    node              - 完整 node (yolo + llama)"
	@echo "    node_llama        - 仅 llama 的 node (VLM 图像描述)"
	@echo "    node_yolo         - 仅 yolo 的 node (目标检测)"
	@echo "    node_all          - 同时打包 3 种 node 变体"
	@echo ""
	@echo "单组件构建 (输出到 dist/rivision_xxx/, 含 install.sh):"
	@echo "  cli                                - rivision_cli"
	@echo "  embed                              - rivision_embed"
	@echo "  gateway                            - rivision_gateway"
	@echo "  owl                                - rivision_owl (含 ZLMediaKit)"
	@echo "  node                               - rivision_node (完整: yolo+llama)"
	@echo "  node-llama                         - rivision_node (仅 llama)"
	@echo "  node-yolo                          - rivision_node (仅 yolo)"
	@echo "  hub                                - rivision_hub (Go + Web UI)"
	@echo "  worker                             - rivision_worker (C++, 内置 YOLO+VLM)"
	@echo ""
	@echo "Server 产品级打包 (输出到 dist/<VERSION>/riscv64/):"
	@echo "  release-server-host                - server-host (hub + gateway + owl + app_web)"
	@echo "  release-server-node                - server-node (worker + embed + go2rtc + owl)"
	@echo ""
	@echo "CLI 相关:"
	@echo "  cli-web                            - 增量构建 Web 前端"
	@echo "  cli-web-build                      - 强制构建 Web 前端"
	@echo ""
	@echo "OWL 相关:"
	@echo "  owl-sim                            - 所有模拟器 (gb28181 + onvif)"
	@echo "  owl-gb28181-sim                    - GB28181 模拟器"
	@echo "  owl-onvif-sim                      - ONVIF 模拟器"
	@echo "  check-zlm                          - 检查 ZLMediaKit 状态"
	@echo ""
	@echo "ZLMediaKit / yolo-server 编译:"
	@echo "  zlm                                - ZLMediaKit RISC-V 编译"
	@echo "  yolo-server                        - yolo-server RISC-V 编译"
	@echo ""
	@echo "工具:"
	@echo "  clean                              - 清理所有构建产物"
	@echo "  check-env                          - 检查构建/运行环境"
	@echo "  help                               - 显示此帮助"
	@echo ""
	@echo "变量:"
	@echo "  PACKAGE        - 打包内容 (all|host|node|node_llama|node_yolo|node_all)"
	@echo "  VERSION        - 版本号 (默认: git tag)"
	@echo "  NODE_PROFILE   - 节点配置 (riscv_b|riscv_b_llama|riscv_b_yolo)"
	@echo ""
	@echo "示例:"
	@echo "  make                                         # 完整 K3 发布 (host + node)"
	@echo "  make release PACKAGE=host                    # 只打包主机端"
	@echo "  make release PACKAGE=node                    # 完整 node (yolo+llama)"
	@echo "  make release PACKAGE=node_llama              # 仅 VLM 节点"
	@echo "  make release PACKAGE=node_yolo               # 仅 YOLO 节点"
	@echo "  make release PACKAGE=node_all                # 3种节点全打包"
	@echo "  make yolo-server node               # 编译 yolo + 打包 node"
	@echo ""

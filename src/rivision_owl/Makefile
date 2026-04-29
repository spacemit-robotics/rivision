# RiVision OWL Makefile
#
# ZLMediaKit 嵌入式编译说明:
#   - go:embed 要求二进制在 owl/internal/embed/binaries/ 中存在
#   - 因此 build 前必须先 build-zlm
#   - x86_64: 使用 ZLMediaKit/release/linux/Release/MediaServer
#   - RISC-V: 使用 ZLMediaKit/build-riscv64/release/linux/Release/MediaServer
#             如需重新编译: make zlm-x86 或 make zlm-riscv64

VERSION  ?= 1.0.0
PROFILE  ?= x86
GOARCH   ?= amd64
DIST_DIR ?= dist
EMBED_DIR := owl/internal/embed/binaries

# Makefile 所在目录（绝对路径，用于修复相对路径问题）
# 使用此变量而非 $(shell pwd)，因为 shell pwd 取决于调用者而非 Makefile 位置
MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# 内部变量（不要在命令行覆盖）
# 直接使用 ZLMediaKit 源码编译产物
_src_zlm_x86   := ZLMediaKit/build-x86_64/release/linux/Release/MediaServer
_src_zlm_riscv := ZLMediaKit/build-riscv64/release/linux/Release/MediaServer
_dst_zlm_x86   := $(EMBED_DIR)/ZLMediaKit-linux-amd64
_dst_zlm_riscv := $(EMBED_DIR)/ZLMediaKit-linux-riscv64

.PHONY: help
.PHONY: owl-build owl-package build-owl build-gb28181-sim build-onvif-sim owl-web clean
.PHONY: owl-sim owl-gb28181-sim owl-onvif-sim
.PHONY: build-zlm check-zlm zlm-x86 zlm-riscv64
.PHONY: owl-x86 owl-riscv

# === ZLMediaKit 源码编译 ===
# 从 ZLMediaKit/ 源码编译，输出到 zlmediakit/prebuilt/{arch}/MediaServer
#
# 用法:
#   make zlm-x86       # x86_64 本地编译
#   make zlm-riscv64   # RISC-V 交叉编译

zlm-x86:
	@echo "=== 从源码编译 ZLMediaKit (x86_64) ==="
	@if [ -d "ZLMediaKit" ]; then \
		cd ZLMediaKit && make -f Makefile.rivision build-x86; \
	else \
		echo "ERROR: ZLMediaKit 源码目录不存在"; \
		echo "  请确保 src/rivision_owl/ZLMediaKit/ 存在"; \
		exit 1; \
	fi

zlm-riscv64:
	@echo "=== 从源码编译 ZLMediaKit (RISC-V) ==="
	@if [ -d "ZLMediaKit" ]; then \
		cd ZLMediaKit && make -f Makefile.rivision build-riscv64; \
	else \
		echo "ERROR: ZLMediaKit 源码目录不存在"; \
		echo "  请确保 src/rivision_owl/ZLMediaKit/ 存在"; \
		exit 1; \
	fi

# === 快捷目标 ===

# === 统一格式输出目标 ===

# x86_64 简化构建（用于 release 流程，无 banner）
owl-x86-quiet:
	@$(MAKE) build-zlm --no-print-directory
	@$(MAKE) build-owl GOARCH=amd64 --no-print-directory 2>&1 | grep -v "^\[build" || true
	@$(MAKE) build-gb28181-sim GOARCH=amd64 --no-print-directory 2>&1 | grep -v "^\[build" || true
	@$(MAKE) owl-package PROFILE=x86 GOARCH=amd64 --no-print-directory 2>&1 | grep -E "^  |✅|IP" || true

# riscv64 简化构建（用于 release 流程，无 banner）
owl-riscv-quiet:
	@$(MAKE) build-zlm --no-print-directory
	@$(MAKE) build-owl GOARCH=riscv64 --no-print-directory 2>&1 | grep -v "^\[build" || true
	@$(MAKE) build-gb28181-sim GOARCH=riscv64 --no-print-directory 2>&1 | grep -v "^\[build" || true
	@$(MAKE) owl-package PROFILE=riscv GOARCH=riscv64 --no-print-directory 2>&1 | grep -E "^  |✅|IP" || true

# x86_64 完整构建：build-zlm → build → package
owl-x86:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_owl                                     ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  x86"
	@echo "  Platform: amd64 (GOARCH=amd64)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/4] Checking ZLMediaKit binary..."
	@$(MAKE) build-zlm --no-print-directory
	@echo "[2/4] Building owl binary..."
	@$(MAKE) build-owl GOARCH=amd64 --no-print-directory
	@echo "[3/4] Building gb28181-sim..."
	@$(MAKE) build-gb28181-sim GOARCH=amd64 --no-print-directory
	@echo "[4/4] Packaging..."
	@$(MAKE) owl-package PROFILE=x86 GOARCH=amd64 --no-print-directory 2>&1 | grep -E "^  |✅|IP" || true
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""
	@echo "  Usage:"
	@echo "    ./bin/owl -c ./config"
	@echo "    ./bin/gb28181-sim -c ./config/gb28181-sim.yaml"
	@echo ""

# RISC-V 完整构建：build-zlm → build → package
owl-riscv:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  📦 Building rivision_owl                                     ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Profile:  riscv64"
	@echo "  Platform: riscv64 (GOARCH=riscv64)"
	@echo "  Output:   $(DIST_DIR)"
	@echo ""
	@echo "[1/4] Checking ZLMediaKit binary..."
	@$(MAKE) build-zlm --no-print-directory
	@echo "[2/4] Building owl binary..."
	@$(MAKE) build-owl GOARCH=riscv64 --no-print-directory
	@echo "[3/4] Building gb28181-sim..."
	@$(MAKE) build-gb28181-sim GOARCH=riscv64 --no-print-directory
	@echo "[4/4] Packaging..."
	@$(MAKE) owl-package PROFILE=riscv GOARCH=riscv64 --no-print-directory 2>&1 | grep -E "^  |✅|IP" || true
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║  ✅ Package Complete                                         ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"
	@echo "  Output: $(DIST_DIR)"
	@echo "  Size:   $$(du -sh $(DIST_DIR) | cut -f1)"
	@echo ""
	@echo "  Usage:"
	@echo "    ./bin/owl -c ./config"
	@echo ""

# === 全平台构建 ===
owl-build: build-zlm build-owl build-gb28181-sim

# === 准备嵌入的 ZLMediaKit 二进制 ===
#
# 用法:
#   make build-zlm              # 复制所有存在的 ZLM 二进制
#   make build-zlm CHECK=1      # 仅检查，返回 exit code（0=全部存在, 1=有缺失）
#
build-zlm:
	@mkdir -p $(EMBED_DIR)
	@find $(EMBED_DIR) -name "*.placeholder" -delete 2>/dev/null || true
	@$(call copy_zlm,x86_64)
	@$(call copy_zlm,riscv64)
	@if [ "$(CHECK)" = "1" ]; then \
		if [ ! -f "$(_src_zlm_x86)" ] || [ ! -f "$(_src_zlm_riscv)" ]; then \
			echo "WARN: ZLMediaKit 二进制不完整"; exit 1; \
		else \
			echo "OK: 所有 ZLM 二进制就绪"; \
		fi \
	fi

# 内部宏：复制单个平台的 ZLM 二进制
# 参数: $(1) = x86_64 | riscv64
define copy_zlm
	@if [ "$(1)" = "x86_64" ]; then \
		src="$(_src_zlm_x86)"; dst="$(_dst_zlm_x86)"; \
	else \
		src="$(_src_zlm_riscv)"; dst="$(_dst_zlm_riscv)"; \
	fi; \
	if [ -f "$$src" ]; then \
		cp "$$src" "$$dst"; \
	else \
		if [ "$(CHECK)" = "1" ]; then :; else \
			touch "$$dst".placeholder; \
		fi \
	fi
endef

# === 检查 ZLMediaKit 二进制状态 ===
#
# 用法:
#   make check-zlm   # 输出每个平台的 ZLM 状态（存在/不存在/缺失原因）
#
check-zlm:
	@echo "ZLMediaKit 二进制状态检查："
	@for arch in x86_64 riscv64; do \
		if [ "$$arch" = "x86_64" ]; then \
			src="$(_src_zlm_x86)"; dst="$(_dst_zlm_x86)"; \
		else \
			src="$(_src_zlm_riscv)"; dst="$(_dst_zlm_riscv)"; \
		fi; \
		if [ -f "$$src" ]; then \
			echo "  $$arch: $$src ($$(du -sh $$src | cut -f1)) ✓"; \
		else \
			echo "  $$arch: 未找到（需: $$src） ✗"; \
		fi \
	done

# === Go 编译 ===
# 使用架构特定的输出文件名，避免 x86/riscv 混淆

build-owl:
	@echo "[build-owl] GOARCH=$(GOARCH)"
	@mkdir -p build
	@cd owl && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/owl-$(GOARCH) .
	@cp build/owl-$(GOARCH) build/owl
	@echo "[build-owl] 输出: build/owl-$(GOARCH)"

build-gb28181-sim:
	@echo "[build-gb28181-sim] GOARCH=$(GOARCH)"
	@mkdir -p build
	@cd gb28181-sim && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/gb28181-sim-$(GOARCH) .
	@cp build/gb28181-sim-$(GOARCH) build/gb28181-sim
	@echo "[build-gb28181-sim] 输出: build/gb28181-sim-$(GOARCH)"

build-onvif-sim:
	@echo "[build-onvif-sim] GOARCH=$(GOARCH)"
	@mkdir -p build
	@cd onvif-sim && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/onvif-sim-$(GOARCH) .
	@cp build/onvif-sim-$(GOARCH) build/onvif-sim
	@echo "[build-onvif-sim] 输出: build/onvif-sim-$(GOARCH)"

# === 模拟器打包目标 ===

# owl-sim: 构建所有模拟器 (gb28181 + onvif)
owl-sim: owl-gb28181-sim owl-onvif-sim
	@echo "✅ 所有模拟器构建完成"

# owl-gb28181-sim: 构建 GB28181 模拟器
owl-gb28181-sim:
	@cd gb28181-sim && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/gb28181-sim .
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config
	@cp build/gb28181-sim $(DIST_DIR)/bin/
	@cp gb28181-sim/config.example.yaml $(DIST_DIR)/config/gb28181-sim.yaml
	@# 自动替换本地 IP (获取物理网卡 IP，排除虚拟/隧道接口)
	@LOCAL_IP=$$(if [ -n "$$LOCAL_IP" ]; then echo "$$LOCAL_IP"; else \
		ip -4 addr show | grep -E '(eth|enp|wlan|wlp|ens)[0-9]' | \
		grep -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+' | head -1; \
	fi); \
	if [ -n "$$LOCAL_IP" ]; then \
		sed -i 's|^  ip: "[^"]*"|  ip: "'"$$LOCAL_IP"'"|' $(DIST_DIR)/config/gb28181-sim.yaml; \
		echo "  检测到本地 IP: $$LOCAL_IP"; \
	fi
	@echo "✅ gb28181-sim 构建完成"

# owl-onvif-sim: 构建 ONVIF 模拟器
owl-onvif-sim:
	@cd onvif-sim && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/onvif-sim .
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config
	@cp build/onvif-sim $(DIST_DIR)/bin/
	@cp onvif-sim/config.example.yaml $(DIST_DIR)/config/onvif-sim.yaml
	@echo "✅ onvif-sim 构建完成"

# === Web UI 构建 ===

# 构建前端页面（需要 Node.js 和 npm）
# npm install 仅在 node_modules 不存在时执行（增量化）
owl-web:
	@if [ ! -d "gb28181-web/node_modules" ]; then \
		echo "  首次构建，执行 npm install..."; \
		cd gb28181-web && npm install 2>/dev/null; \
	fi
	cd gb28181-web && npm run build
	@echo "  Web UI 构建完成: gb28181-web/dist/"

# === 自动检测本地 IP ===
# 优先级: 1) 环境变量 LOCAL_IP  2) 自动检测（排除 127.0.0.1 和 docker 网桥）
define detect_local_ip
$(shell if [ -n "$$LOCAL_IP" ]; then echo "$$LOCAL_IP"; else ip -4 addr show | grep -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+' | grep -v '^127\.' | grep -v '^172\.17\.' | head -1; fi)
endef

# === 打包（依赖 owl-build，确保二进制已编译）===
owl-package: owl-build
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/config $(DIST_DIR)/services $(DIST_DIR)/www $(DIST_DIR)/data $(DIST_DIR)/logs
	@if [ ! -f build/owl ]; then echo "ERROR: build/owl 不存在"; exit 1; fi
	@cp build/owl build/gb28181-sim $(DIST_DIR)/bin/ 2>/dev/null || cp build/owl $(DIST_DIR)/bin/
	@if [ "$(PROFILE)" = "x86" ] && [ -f "$(_src_zlm_x86)" ]; then \
		cp $(_src_zlm_x86) $(DIST_DIR)/bin/ZLMediaKit; \
	elif [ "$(PROFILE)" = "riscv" ] && [ -f "$(_src_zlm_riscv)" ]; then \
		cp $(_src_zlm_riscv) $(DIST_DIR)/bin/ZLMediaKit; \
	fi
	@cp $(MAKEFILE_DIR)/owl/configs/config.toml $(DIST_DIR)/config/ 2>/dev/null || true
	@cp $(MAKEFILE_DIR)/gb28181-sim/config.example.yaml $(DIST_DIR)/config/gb28181-sim.yaml 2>/dev/null || true
	@cp $(MAKEFILE_DIR)/config/.env.template $(DIST_DIR)/config/.env 2>/dev/null || true
	@cp $(MAKEFILE_DIR)/config/owl.toml.template $(DIST_DIR)/config/ 2>/dev/null || true
	@# 自动替换本地 IP（SDPIP 和 gb28181-sim server.ip）
	@# 获取物理网卡 IP (eth/enp/wlan/wlp/ens)，排除虚拟/隧道接口
	@LOCAL_IP=$$(if [ -n "$$LOCAL_IP" ]; then echo "$$LOCAL_IP"; else \
		ip -4 addr show | grep -E '(eth|enp|wlan|wlp|ens)[0-9]' | \
		grep -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+' | head -1; \
	fi); \
	if [ -n "$$LOCAL_IP" ]; then \
		echo "  检测到本地 IP: $$LOCAL_IP"; \
		sed -i "s/SDPIP = '[^']*'/SDPIP = '$$LOCAL_IP'/" $(DIST_DIR)/config/config.toml; \
		sed -i 's|^  ip: "[^"]*"|  ip: "'"$$LOCAL_IP"'"|' $(DIST_DIR)/config/gb28181-sim.yaml; \
		sed -i "s/^# OWL_SDP_IP=.*/OWL_SDP_IP=$$LOCAL_IP/" $(DIST_DIR)/config/.env 2>/dev/null || true; \
	else \
		echo "  ⚠️ 无法检测本地 IP，请手动修改 config.toml 中的 SDPIP"; \
	fi
	@if [ -d "gb28181-web/dist" ]; then cp -r gb28181-web/dist/* $(DIST_DIR)/www/; fi
	@cp services/rivision-owl.service $(DIST_DIR)/services/ 2>/dev/null || true
	@mkdir -p $(DIST_DIR)/mp4path && cp -r $(MAKEFILE_DIR)/mp4path/*.mp4 $(DIST_DIR)/mp4path/ 2>/dev/null || true
	@echo ""
	@echo "✅ 打包完成: $(DIST_DIR)/"
	@echo "   bin/    : owl, gb28181-sim, ZLMediaKit"
	@echo "   config/ : config.toml, .env, gb28181-sim.yaml (IP 已自动配置)"
	@echo "   mp4path/: 测试视频文件"

# === 清理 ===
clean:
	rm -rf build dist
	cd owl && rm -f owl
	cd gb28181-sim && rm -f gb28181-sim gb28181-sim-*
	rm -f $(EMBED_DIR)/ZLMediaKit-*
	rm -f $(EMBED_DIR)/*.placeholder
	rm -rf ZLMediaKit/build-x86_64 ZLMediaKit/build-riscv64 ZLMediaKit/release
	rm -rf onvif-sim/build
	rm -rf gb28181-web/dist gb28181-web/node_modules

# === 帮助 ===
help:
	@echo "OWL 构建系统"
	@echo ""
	@echo "用法: make <目标> [PROFILE=x86|riscv] [GOARCH=amd64|riscv64]"
	@echo ""
	@echo "快捷目标（推荐）:"
	@echo "  owl-x86   - x86_64 完整构建（build-zlm + build + package）"
	@echo "  owl-riscv - RISC-V K3 完整构建（build-zlm + build + package）"
	@echo ""
	@echo "精细控制:"
	@echo "  owl-build    - 编译所有 Go 二进制（ZLM 嵌入 + owl + sim）"
	@echo "  build-owl   - 仅编译 owl 二进制（需先 build-zlm）"
	@echo "  owl-sim     - 仅编译 GB28181 模拟器（独立，不重复构建 owl）"
	@echo "  owl-web    - 构建 Web UI 页面（首次自动 npm install）"
	@echo "  build-zlm  - 复制 ZLM 二进制到 embed/ 目录"
	@echo "  check-zlm  - 检查 ZLM 二进制是否就绪"
	@echo "  owl-package - 编译 + 打包（等同于 owl-x86）"
	@echo "  clean      - 清理所有构建产物"
	@echo ""
	@echo "示例:"
	@echo "  make owl-x86                          # 推荐：完整构建+打包"
	@echo "  make owl-riscv                        # RISC-V K3"
	@echo "  make owl-build                        # 仅编译 Go 二进制（不打包）"
	@echo "  make owl-web                         # 构建 Web UI（首次自动 npm install）"
	@echo "  make owl-package                      # 打包（依赖 owl-build）"
	@echo "  make build-zlm CHECK=1               # 仅检查 ZLM，返回 exit code"
	@echo ""
	@echo "ZLMediaKit 二进制说明:"
	@echo "  x86_64:  ZLMediaKit/release/linux/Release/MediaServer"
	@echo "  riscv64: ZLMediaKit/build-riscv64/release/linux/Release/MediaServer"
	@echo "  重新编译: make zlm-x86 或 make zlm-riscv64"

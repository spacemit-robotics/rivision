# RiVision OWL Makefile (K3 RISC-V 专用)
#
# ZLMediaKit 嵌入式编译说明:
#   - go:embed 要求二进制在 owl/internal/embed/binaries/ 中存在
#   - 因此 build 前必须先 build-zlm
#   - 编译产物: ZLMediaKit/release/linux/Release/MediaServer
#   - 如需重新编译: make zlm

VERSION  ?= 1.0.0
PROFILE  ?= riscv
GOARCH   ?= riscv64
DIST_DIR ?= dist
EMBED_DIR := owl/internal/embed/binaries

# Makefile 所在目录（绝对路径，用于修复相对路径问题）
MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# ZLMediaKit 二进制路径
# CMake 输出到源码根的 release/linux/Release/ 目录
_src_zlm := ZLMediaKit/release/linux/Release/MediaServer
_dst_zlm := $(EMBED_DIR)/ZLMediaKit-linux-riscv64

.PHONY: help
.PHONY: owl-build owl-package build-owl build-gb28181-sim build-onvif-sim owl-web clean
.PHONY: owl-sim owl-gb28181-sim owl-onvif-sim
.PHONY: build-zlm check-zlm zlm
.PHONY: owl owl-quiet

# === ZLMediaKit 源码编译 (RISC-V) ===
#
# 用法:
#   make zlm   # K3 原生编译 ZLMediaKit

zlm:
	@echo "=== 从源码编译 ZLMediaKit (RISC-V) ==="
	@if [ -d "ZLMediaKit" ]; then \
		cd ZLMediaKit && make -f Makefile.rivision build-riscv64; \
	else \
		echo "ERROR: ZLMediaKit 源码目录不存在"; \
		echo "  请确保 src/rivision_owl/ZLMediaKit/ 存在"; \
		echo "  运行: git submodule update --init src/rivision_owl/ZLMediaKit"; \
		exit 1; \
	fi

# === 快捷目标 ===

# 简化构建（用于 release 流程，无 banner）
owl-quiet:
	@$(MAKE) build-zlm --no-print-directory
	@$(MAKE) build-owl --no-print-directory 2>&1 | grep -v "^\[build" || true
	@$(MAKE) build-gb28181-sim --no-print-directory 2>&1 | grep -v "^\[build" || true
	@$(MAKE) owl-package --no-print-directory 2>&1 | grep -E "^  |✅|IP" || true

# RISC-V 完整构建：build-zlm → build → package
owl:
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
	@$(MAKE) build-owl --no-print-directory
	@echo "[3/4] Building gb28181-sim..."
	@$(MAKE) build-gb28181-sim --no-print-directory
	@echo "[4/4] Packaging..."
	@$(MAKE) owl-package --no-print-directory 2>&1 | grep -E "^  |✅|IP" || true
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
#   make build-zlm              # 复制 ZLM 二进制到 embed 目录（缺失则用 placeholder）
#   make build-zlm CHECK=1      # 仅检查，缺失则报错
#
build-zlm:
	@mkdir -p $(EMBED_DIR)
	@find $(EMBED_DIR) -name "*.placeholder" -delete 2>/dev/null || true
	@if [ -f "$(_src_zlm)" ]; then \
		cp "$(_src_zlm)" "$(_dst_zlm)"; \
	elif [ "$(CHECK)" = "1" ]; then \
		echo ""; \
		echo "  ❌ ZLMediaKit riscv64 二进制缺失!"; \
		echo "     需要: $(_src_zlm)"; \
		echo "     编译: cd src/rivision_owl && make zlm"; \
		echo ""; \
		exit 1; \
	else \
		touch "$(_dst_zlm).placeholder"; \
		echo "  ⚠️  ZLMediaKit 二进制不存在，已创建 placeholder"; \
		echo "     编译: make zlm"; \
	fi

# === 检查 ZLMediaKit 二进制状态 ===
check-zlm:
	@echo "ZLMediaKit 二进制状态检查："
	@if [ -f "$(_src_zlm)" ]; then \
		echo "  riscv64: $(_src_zlm) ($$(du -sh $(_src_zlm) | cut -f1)) ✓"; \
	else \
		echo "  riscv64: 未找到（需: $(_src_zlm)） ✗"; \
	fi

# === Go 编译 ===

build-owl:
	@echo "[build-owl] GOARCH=$(GOARCH)"
	@mkdir -p build
	@cd owl && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/owl .
	@echo "[build-owl] 输出: build/owl"

build-gb28181-sim:
	@echo "[build-gb28181-sim] GOARCH=$(GOARCH)"
	@mkdir -p build
	@cd gb28181-sim && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/gb28181-sim .
	@echo "[build-gb28181-sim] 输出: build/gb28181-sim"

build-onvif-sim:
	@echo "[build-onvif-sim] GOARCH=$(GOARCH)"
	@mkdir -p build
	@cd onvif-sim && GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../build/onvif-sim .
	@echo "[build-onvif-sim] 输出: build/onvif-sim"

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
	@if [ -f "$(_src_zlm)" ]; then \
		cp $(_src_zlm) $(DIST_DIR)/bin/ZLMediaKit; \
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
	cd gb28181-sim && rm -f gb28181-sim
	rm -f $(EMBED_DIR)/ZLMediaKit-*
	rm -f $(EMBED_DIR)/*.placeholder
	rm -rf ZLMediaKit/build-riscv64 ZLMediaKit/release
	rm -rf onvif-sim/build
	rm -rf gb28181-web/dist gb28181-web/node_modules

# === 帮助 ===
help:
	@echo "OWL 构建系统 (K3 RISC-V)"
	@echo ""
	@echo "用法: make <目标>"
	@echo ""
	@echo "快捷目标（推荐）:"
	@echo "  owl       - RISC-V K3 完整构建（build-zlm + build + package）"
	@echo ""
	@echo "精细控制:"
	@echo "  owl-build    - 编译所有 Go 二进制（ZLM 嵌入 + owl + sim）"
	@echo "  build-owl    - 仅编译 owl 二进制（需先 build-zlm）"
	@echo "  owl-sim      - 编译模拟器（gb28181 + onvif）"
	@echo "  owl-web      - 构建 Web UI 页面（首次自动 npm install）"
	@echo "  build-zlm    - 复制 ZLM 二进制到 embed/ 目录"
	@echo "  check-zlm    - 检查 ZLM 二进制是否就绪"
	@echo "  owl-package  - 编译 + 打包"
	@echo "  clean        - 清理所有构建产物"
	@echo ""
	@echo "ZLMediaKit:"
	@echo "  zlm          - 从源码编译 ZLMediaKit (RISC-V)"
	@echo "  编译产物: ZLMediaKit/release/linux/Release/MediaServer"
	@echo ""
	@echo "示例:"
	@echo "  make owl                              # 推荐：完整构建+打包"
	@echo "  make zlm                              # 编译 ZLMediaKit"
	@echo "  make owl-build                        # 仅编译 Go 二进制（不打包）"
	@echo "  make owl-web                          # 构建 Web UI"
	@echo "  make build-zlm CHECK=1                # 检查 ZLM 二进制是否存在"

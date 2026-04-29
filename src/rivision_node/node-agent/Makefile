# ============================================================
# RiVision Node Agent (Go) Makefile
# 支持本地编译和交叉编译 (K3 RISC-V / x86_64)
# ============================================================

BINARY_NAME := rivision_node
VERSION := 1.0.0
BUILD_DIR := .
BIN_DIR := bin

# 检测当前架构
ARCH := $(shell uname -m)
ifeq ($(ARCH),x86_64)
    ARCH_SUFFIX := x86_64
else ifeq ($(ARCH),riscv64)
    ARCH_SUFFIX := riscv64
else
    ARCH_SUFFIX := $(ARCH)
endif

# Go 编译参数
LDFLAGS := -s -w -X main.Version=$(VERSION)
GO_BUILD := go build -ldflags "$(LDFLAGS)"

.PHONY: all build clean install cross-k3 cross-x86 cache help

# 默认: 编译当前架构
all: build

# 编译当前架构
build:
	@echo "编译 $(BINARY_NAME) ($(ARCH_SUFFIX))..."
	$(GO_BUILD) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 交叉编译 K3 (RISC-V)
cross-k3:
	@echo "交叉编译 $(BINARY_NAME) (riscv64)..."
	GOOS=linux GOARCH=riscv64 $(GO_BUILD) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 交叉编译 x86_64
cross-x86:
	@echo "交叉编译 $(BINARY_NAME) (x86_64)..."
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 缓存预编译二进制 (用于部署包)
cache: build
	@mkdir -p $(BIN_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(BIN_DIR)/$(BINARY_NAME).$(ARCH_SUFFIX)
	@echo "已缓存: $(BIN_DIR)/$(BINARY_NAME).$(ARCH_SUFFIX)"

# 清理
clean:
	@rm -f $(BUILD_DIR)/$(BINARY_NAME)
	@echo "已清理"

# 完全清理 (包括缓存)
distclean: clean
	@rm -rf $(BIN_DIR)
	@echo "已完全清理"

# 安装到系统
install: build
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "已安装到 /usr/local/bin/$(BINARY_NAME)"

# 帮助
help:
	@echo "RiVision Node Agent (Go) 构建系统"
	@echo ""
	@echo "用法: make [目标]"
	@echo ""
	@echo "目标:"
	@echo "  build      编译当前架构 (默认)"
	@echo "  cross-k3   交叉编译 RISC-V (K3)"
	@echo "  cross-x86  交叉编译 x86_64"
	@echo "  cache      编译并缓存到 bin/ 目录"
	@echo "  clean      清理编译产物"
	@echo "  distclean  完全清理 (包括缓存)"
	@echo "  install    安装到系统"
	@echo "  help       显示此帮助"
	@echo ""
	@echo "当前架构: $(ARCH_SUFFIX)"

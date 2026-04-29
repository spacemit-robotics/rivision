# RiVision

RiVision 是一个面向 SpaceMIT K3 RISC-V 平台的智能视频监控系统，集成了 GB28181 协议、YOLO 目标检测、VLM 图像理解和流媒体服务。

## 系统架构

```
┌─────────────────────────────────────────────────────────┐
│                    RiVision 系统                         │
├──────────────┬──────────────┬───────────────────────────┤
│   Host 端    │   Node 端    │        说明               │
├──────────────┼──────────────┼───────────────────────────┤
│ rivision-cli │ rivision-node│ CLI 管理 / AI 推理节点     │
│ rivision-owl │              │ GB28181 + 流媒体           │
│ rivision-gw  │              │ API 网关                   │
│ rivision-embed│             │ 文本向量化服务              │
└──────────────┴──────────────┴───────────────────────────┘
```

## 组件说明

| 组件 | 目录 | 功能 |
|------|------|------|
| rivision-cli | `src/rivision_cli/` | 主控 CLI，集成 go2rtc 流媒体、Web UI、摄像头管理 |
| rivision-owl | `src/rivision_owl/` | GB28181 信令服务、ZLMediaKit 流媒体、设备管理 |
| rivision-gateway | `src/rivision_gateway/` | 推理网关，转发 VLM/Embedding 请求到后端模型 |
| rivision-embed | `src/rivision_embed/` | 文本向量化服务 (ONNX Runtime) |
| rivision-node | `src/rivision_node/` | AI 推理节点，包含 YOLO 目标检测和 llama.cpp VLM |

## 目标平台

当前仅支持 **SpaceMIT K3 RISC-V** 平台原生编译。

## 前置条件

- SpaceMIT K3 开发板 (Bianbu OS)
- Go 1.25+
- CMake 3.20+
- GCC / G++
- Node.js 18+ (Web UI 构建)

## 快速开始

### 1. 克隆仓库

```bash
git clone --recursive https://github.com/spacemit-robotics/rivision.git
cd rivision
```

### 2. 下载依赖

```bash
./scripts/setup-deps.sh
```

此脚本会下载模型文件、第三方库等运行时依赖。

### 3. 应用补丁

```bash
patch -p3 < patches/owl.patch
patch -p3 < patches/gb28181-web.patch
patch -p3 < patches/onvif-go.patch
```

### 4. 构建

```bash
cd build
make                    # 完整构建 (host + node)
make release PACKAGE=host   # 仅主机端
make release PACKAGE=node   # 仅推理节点
```

构建产物输出到 `dist/<version>/riscv64/` 目录。

### 5. 部署

```bash
# 主机端
scp -r dist/<version>/riscv64/host/ k3:/tmp/
ssh k3 "cd /tmp/host && sudo ./install.sh"

# 推理节点
scp -r dist/<version>/riscv64/node/ k3:/tmp/
ssh k3 "cd /tmp/node && sudo ./install.sh"
```

## 目录结构

```
rivision/
├── build/              # 构建系统 (Makefile, 模板, 脚本)
├── patches/            # 子模块本地修改补丁
├── scripts/            # 工具脚本
└── src/
    ├── rivision_cli/       # 主控 CLI
    ├── rivision_embed/     # 文本向量化
    ├── rivision_gateway/   # 推理网关
    ├── rivision_node/      # AI 推理节点
    └── rivision_owl/       # GB28181 + 流媒体
        ├── owl/            # (submodule) 核心服务
        ├── gb28181-web/    # (submodule) Web 前端
        ├── onvif-go/       # (submodule) ONVIF 协议
        └── ZLMediaKit/     # (submodule) 流媒体引擎
```

## 构建命令参考

```bash
make help               # 查看所有可用目标
make check-go           # 检查 Go 环境
make cli-riscv64        # 单独构建 CLI
make owl-riscv64        # 单独构建 OWL
make node-riscv64       # 单独构建 Node
make zlm-riscv64        # 编译 ZLMediaKit
make clean              # 清理构建产物
```

## License

MIT

# RiVision

RiVision 是面向视频流的分布式 AI 推理底座。基于 SpaceMIT K3 RISC-V 芯片，为 YOLO 目标检测、VLM 图像理解等视觉模型提供并行化、高性能的推理服务。

## 解决什么问题

传统视频 AI 方案依赖 GPU 服务器集中处理，成本高、延迟大、扩展难。RiVision 将推理能力下沉到低功耗的 RISC-V 边缘节点，通过多节点集群实现弹性扩展：

| 集群规模 | K3 节点数 | 并行处理能力 | 典型场景 |
|----------|----------|-------------|---------|
| 入门 | 1 | 1-4 路视频流 | 单店铺、小型仓库 |
| 标准 | 4 | 4-16 路视频流 | 园区、中型工厂 |
| 扩展 | 8 | 8-32 路视频流 | 大型园区、多楼层 |
| 集群 | 16-32 | 16-128 路视频流 | 城市级、大型工业园 |

每个 K3 节点成本低、功耗小，按需增减节点即可匹配业务规模，无需一次性投入高端硬件。

## 核心能力

- **YOLO 实时目标检测** — 人员、车辆、物体识别，支持多路视频流并行推理
- **VLM 图像理解** — 基于 llama.cpp 的视觉语言模型，对场景进行语义描述和异常分析，视频摘要等
- **GB28181 国标接入** — 兼容公安/交通行业标准协议，无缝对接现有监控体系
- **流媒体分发** — 集成 ZLMediaKit + go2rtc，支持 RTSP/WebRTC/HLS 多协议转发
- **文本向量化** — 内置 Embedding 服务，支持检测结果的语义检索

## 系统架构

```
                        ┌──────────────────────┐
                        │     rivision-cli     │  主控 CLI + Web UI
                        │     rivision-owl     │  GB28181 + 流媒体
                        │   rivision-gateway   │  推理网关
  摄像头/视频流 ──────▶  │   rivision-embed     │  文本向量化
                        │       (Host)         │
                        └──────────┬───────────┘
                                   │ 推理任务分发
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
             ┌────────────┐ ┌────────────┐ ┌────────────┐
             │ K3 Node #1 │ │ K3 Node #2 │ │ K3 Node #N │
             │ YOLO + VLM │ │ YOLO + VLM │ │ YOLO + VLM │
             └────────────┘ └────────────┘ └────────────┘
                    rivision-node (按需扩展)
```

- **Host 端**：接入视频流、管理设备、分发推理任务、聚合结果
- **Node 端**：运行 AI 模型，每个 K3 节点独立执行 YOLO 检测或 VLM 推理

## 组件说明

| 组件 | 目录 | 功能 |
|------|------|------|
| rivision-cli | `src/rivision_cli/` | 主控 CLI，集成 go2rtc 流媒体、Web UI、摄像头管理 |
| rivision-owl | `src/rivision_owl/` | GB28181 信令服务、ZLMediaKit 流媒体、设备管理 |
| rivision-gateway | `src/rivision_gateway/` | 推理网关，转发 VLM/Embedding 请求到后端模型 |
| rivision-embed | `src/rivision_embed/` | 文本向量化服务 (ONNX Runtime) |
| rivision-node | `src/rivision_node/` | AI 推理节点，包含 YOLO 目标检测和 llama.cpp VLM |

## 目标平台

当前支持 **SpaceMIT K3 RISC-V** 平台原生编译。

## 快速开始

### 前置条件

- SpaceMIT K3 开发板 (Bianbu OS)
- Go 1.25+
- CMake 3.20+
- GCC / G++
- Node.js 18+ (Web UI 构建)

可运行一键安装脚本自动检测并安装缺失依赖：

```bash
./scripts/setup-env.sh
```

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
make                         # 完整构建 (host + node)
make release PACKAGE=host    # 仅主机端
make release PACKAGE=node    # 仅推理节点
```

构建产物输出到 `dist/<version>/riscv64/` 目录。

### 5. 部署

```bash
# 主机端
scp -r dist/<version>/riscv64/host/ k3:/tmp/
ssh k3 "cd /tmp/host && sudo ./install.sh"

# 推理节点 (每个 K3 节点重复此步骤)
scp -r dist/<version>/riscv64/node/ k3-node:/tmp/
ssh k3-node "cd /tmp/node && sudo ./install.sh"
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
make check-env          # 检查构建/运行环境
make cli                # 单独构建 CLI
make owl                # 单独构建 OWL
make node               # 单独构建 Node
make zlm                # 编译 ZLMediaKit
make clean              # 清理构建产物
```

## 贡献方式

欢迎通过 issue、文档修订、参数优化、功能增强和问题修复等方式参与贡献。

建议贡献前：
- 明确修改目标与适用硬件范围；
- 补充必要的配置说明与验证结果；
- 避免引入与现有构建体系不兼容的改动。

## License

本组件源码文件头声明为 Apache-2.0，最终以本目录 `LICENSE` 文件为准。

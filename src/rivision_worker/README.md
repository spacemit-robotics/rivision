# RiVision Worker (C++)

**基于 SpaceMIT K3 的高性能边缘视频分析服务**

## 概述

rivision_worker 是 RiVision 系统的边缘计算节点，基于 SpaceMIT K3 RISC-V 芯片，负责：
- 多路 RTSP 流拉取与硬件解码（K3 VPU）
- YOLO 目标检测（SpaceMIT NPU 加速，2TOPS）
- ByteTrack 目标跟踪
- 规则引擎（区域入侵、越线、停留等）
- 向量搜索（CLIP 嵌入）
- 统计分析（热力图、人流计数）
- Hub 通信（WebSocket）
- REST API

## 技术栈

| 组件 | 选型 | 说明 |
|------|------|------|
| 语言 | C++17 | 高性能、无 GC |
| HTTP | oatpp | 轻量级异步框架 |
| 日志 | spdlog | 高性能日志 |
| JSON | nlohmann/json | Header-only |
| 数据库 | SQLite3 | 嵌入式 |
| 构建 | CMake | 跨平台 |
| 推理 | ONNX Runtime + SpaceMIT NPU EP | K3 NPU 硬件加速 |
| 视频 | SpaceMIT MPP | K3 VPU 硬件编解码 |

## 目标平台

| 平台 | 后端 | 特点 |
|------|------|------|
| **SpaceMIT K3** | MPP + NPU | 8 核 RISC-V, 2TOPS NPU, <5W |

## 快速开始

### K3 本地编译

```bash
# K3 开发板 (Bianbu OS) 上直接编译
make

# 运行
./build/bin/rivision_worker -c config/worker.yaml
```

### 打包部署

```bash
make package
# 输出: rivision_worker-YYYYMMDD.tar.gz
```

## 目录结构

```
src/rivision_worker/
├── CMakeLists.txt          # 顶层构建
├── Makefile                # 便捷命令
├── cmake/                  # CMake 模块 (FindSpacemiTMPP, FindSpacemiTORT)
├── include/rivision/       # 公共头文件
├── src/
│   ├── main.cpp            # 入口
│   ├── api/                # HTTP/WS 服务
│   ├── core/               # 业务逻辑
│   ├── pipeline/           # 流处理管线
│   ├── hal/                # 硬件抽象层
│   ├── backend/            # 平台实现 (MPP/NPU)
│   ├── storage/            # 数据存储
│   ├── hub/                # Hub 通信
│   ├── utils/              # 工具函数
│   └── vlm/                # VLM 推理服务
├── config/                 # 配置文件
│   ├── worker.yaml
│   ├── yolo.yaml
│   ├── pipeline.yaml
│   ├── rules.yaml
│   ├── vlm.yaml
│   └── embed.yaml
└── test/                   # 测试
```

## 配置

### worker.yaml

```yaml
node_id: "worker-001"

server:
  host: "0.0.0.0"
  http_port: 8080

hub:
  url: "ws://hub:8081/ws/worker"
  heartbeat_ms: 10000

inference:
  device: "npu"  # SpaceMIT K3 NPU
```

### yolo.yaml

```yaml
model:
  path: "models/yolo/yolo11n.q.onnx"
  input_width: 640
  input_height: 640

detection:
  confidence_threshold: 0.5
  nms_threshold: 0.45

tracker:
  enabled: true
  max_age: 30
```

## API

### REST API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| GET | `/api/v1/streams` | 列出流 |
| POST | `/api/v1/streams` | 添加流 |
| DELETE | `/api/v1/streams/{id}` | 删除流 |
| POST | `/api/v1/search/text` | 文本搜索 |
| POST | `/api/v1/search/image` | 图像搜索 |
| GET | `/api/v1/analytics/stats` | 统计数据 |
| GET | `/metrics` | Prometheus 指标 |

### WebSocket (Hub)

```json
// 心跳
{"type": "heartbeat", "node_id": "worker-001", ...}

// 检测事件
{"type": "detection", "stream_id": "...", "detections": [...]}

// 告警事件
{"type": "alert", "alert_id": "...", "level": "critical", ...}
```

## 性能指标 (SpaceMIT K3)

| 指标 | 数值 | 说明 |
|------|------|------|
| YOLO 推理 | 12ms | NPU 加速 (INT8 量化) |
| 端到端延迟 | ~26ms | 含 VPU 解码 + NPU 推理 + 后处理 |
| 最大路数 | 9 路 | 1080p@5fps |
| 功耗 | ~5W | 含 NPU 满载 |
| 内存占用 | <512MB | 4 路视频流 |

## 文档

- 架构设计: `docs/rivision_arch_v2.md`
- 设计文档: `docs/rivision_worker_v6_design.md`
- API 规范: `docs/RiVision_Product_Spec_Technical.md`

## License

Copyright © 2026 SpacemiT (Hangzhou) Technology Co. Ltd.

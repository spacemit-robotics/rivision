# RiVision 算力节点部署包

## 概述

自包含的算力节点部署包，支持 K3 (SpacemiT RISC-V) 和 x86_64：
- **Node Agent** — 节点代理服务（Go，注册/心跳/进程管理）
- **YOLO Server** — 目标检测服务（C++，OnnxRuntime + SpacemiT NPU EP）
- **llama.cpp** — 多模态推理引擎（RVV 向量优化 / NPU SMT 模式）

## 快速开始

```bash
# 1. 构建（在开发机上）
make package PROFILE=riscv_b    # 或 riscv_a / x86

# 2. 部署到目标机器
sudo ./install.sh --node

# 3. 配置
vi /opt/rivision/.env           # 至少修改 NODE_ID, GATEWAY_URL, REGISTRATION_TOKEN

# 4. 选择模型并启动
/opt/rivision/scripts/switch-model.sh fastvlm-mm-0.5b-q4_1
sudo systemctl start rivision-llama
sudo systemctl start rivision-yolo
sudo systemctl start rivision-node-agent

# 5. 验证
curl http://localhost:9080/health      # llama 推理
curl http://localhost:9081/health      # YOLO 推理
curl http://localhost:9090/health      # Node Agent
/opt/rivision/scripts/test.sh test.jpg # 推理测试
```

## 安装后目录结构

```
/opt/rivision/
├── bin/
│   ├── rivision_node             # 节点 Agent (Go)
│   └── yolo-server               # YOLO 推理服务 (C++)
├── lib/                          # 运行时库 (.so)
│   ├── libonnxruntime.so.*       # OnnxRuntime
│   ├── libspacemit_ep.so.*       # SpacemiT NPU EP (riscv_b)
│   ├── libspine_tcm.so.*        # Spine-TCM (riscv_b)
│   ├── libllama.so.*            # llama.cpp
│   └── libggml*.so.*            # GGML
├── engines/llama.cpp/            # llama.cpp 工具
│   ├── llama-server              # VLM 推理服务
│   ├── llama-mtmd-cli            # 多模态 CLI
│   └── ...
├── models/                       # 模型文件
├── config/
│   ├── .env.template             # 环境变量模板
│   ├── models.conf               # VLM 模型注册表
│   ├── yolo-models.conf          # YOLO 模型注册表
│   └── yolo.yaml                 # YOLO 服务配置
├── scripts/                      # 运行时脚本
├── .env                          # 用户配置（首次安装自动生成）
└── manifest.json                 # 版本信息
```

## 支持的模型

### VLM 模型

| 模型 | Profile | 模式 | 大小 | K3 性能 |
|------|---------|------|------|---------|
| fastvlm-mm-0.5b-q4_1 | riscv_b | NPU/SMT | ~500MB | TBD |
| fastvlm-0.5b-q8 | x86/riscv_a | CPU/mmproj | ~900MB | 6-8 tok/s |
| miniCPM-V 4.5 Q4 | 全部 | CPU/mmproj | ~4GB | ~0.5 tok/s |

### YOLO 模型

| 模型 | Profile | 说明 |
|------|---------|------|
| 910byolo11n_b2.q.onnx | riscv_a/riscv_b | SpacemiT NPU 量化 |
| yolov8n.onnx | x86 | 标准 ONNX |

## 模型切换

```bash
# VLM 模型
/opt/rivision/scripts/switch-model.sh --list
/opt/rivision/scripts/switch-model.sh fastvlm-mm-0.5b-q4_1
sudo systemctl restart rivision-llama

# YOLO 模型
/opt/rivision/scripts/switch-yolo-model.sh --list
/opt/rivision/scripts/switch-yolo-model.sh 910byolo11n_b2.q.onnx
sudo systemctl restart rivision-yolo
```

## 运行时脚本

| 脚本 | 用途 |
|------|------|
| start-llama.sh | systemd 启动 llama 推理（支持 CPU/NPU 自动切换） |
| start-yolo.sh | systemd 启动 YOLO 推理 |
| start-node-agent.sh | systemd 启动节点 Agent |
| switch-model.sh | VLM 模型切换（生成 active-model.sh） |
| switch-yolo-model.sh | YOLO 模型切换 |
| test.sh | 快速推理测试（支持 CPU/NPU） |
| benchmark.sh | 性能基准测试（多轮平均） |
| bench-threads.sh | 线程调优测试 |
| bench-repack.sh | Repack 隔离测试（CPU 专用） |
| optimize.sh | K3 系统优化 |

## 配置

编辑 `/opt/rivision/.env`：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| GATEWAY_URL | http://192.168.1.100:8081 | Gateway 地址（必须修改） |
| LLAMA_THREADS | 8 | 推理线程 (K3=8) |
| LLAMA_PORT | 9080 | llama 推理端口 |
| LLAMA_CPU_AFFINITY | (空) | CPU 绑核 (K3=0-7) |
| YOLO_HOST | 0.0.0.0 | YOLO 监听地址 |
| YOLO_PORT | 9081 | YOLO 推理端口 |
| YOLO_THREADS | 4 | YOLO 每 Worker 线程数 |
| AGENT_PORT | 9090 | Agent 端口 |

完整变量列表见 `config/.env.template`。

## 故障排除

```bash
# 查看服务状态
sudo systemctl status rivision-llama
sudo systemctl status rivision-yolo
sudo systemctl status rivision-node-agent

# 查看日志
journalctl -u rivision-llama -f
journalctl -u rivision-yolo -f
journalctl -u rivision-node-agent -f

# 手动测试推理
/opt/rivision/scripts/test.sh test.jpg

# 性能测试
/opt/rivision/scripts/benchmark.sh test.jpg
```

# rivision-cli

RiVision 主控 CLI 服务，负责摄像头管理、流媒体分发、YOLO 检测调度和 VLM 图像理解触发。

## 功能

- 摄像头 RTSP 流接入与管理
- 集成 go2rtc 流媒体引擎 (WebRTC, HLS, RTSP 转发)
- YOLO 目标检测调度 (通过 rivision-node)
- VLM 图像理解触发 (通过 rivision-gateway)
- Web UI 管理界面
- 检测结果存储与查询

## 目录结构

```
rivision_cli/
├── cmd/                    # CLI 入口
├── internal/
│   ├── embed/binaries/     # 嵌入的二进制 (go2rtc, rtsp_server)
│   ├── go2rtc/             # go2rtc 集成
│   ├── server/             # HTTP API 服务
│   └── yolo/               # YOLO 调度逻辑
├── go2rtc-vue/             # Web UI 前端 (Vue 3)
├── configs/                # 配置文件示例
├── mp4path/                # 测试视频文件
└── Makefile
```

## 构建

```bash
# 从 build/ 目录
make cli-riscv64

# 或直接在 src/rivision_cli/ 目录
make cli-riscv64
```

## 配置

参考 `configs/rivision.yaml.example` 了解完整配置项。

## 嵌入二进制

CLI 通过 `go:embed` 嵌入 go2rtc 和 rtsp_server 二进制。构建前需要运行 `prepare_build.sh` 准备对应平台的二进制文件：

```bash
./prepare_build.sh riscv64
```

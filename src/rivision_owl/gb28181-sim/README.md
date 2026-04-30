# GB28181 Simulator (Go版本)

GB28181 设备模拟器，用于测试 OWL/GoWVP 等 GB28181 平台。

## 功能特性

- **SIP 信令**: 支持 REGISTER、MESSAGE、INVITE、BYE、SUBSCRIBE
- **Digest 认证**: 支持 RFC 2617 Digest 认证
- **多种视频源**: 测试图案、RTSP、文件
- **跨平台编译**: 支持 x86_64、RISC-V 等平台

## 编译

```bash
# 通过父级 Makefile 编译
cd src/rivision_owl
make owl-sim

# 或直接在 gb28181-sim 目录编译
cd gb28181-sim && go build -o ../build/gb28181-sim .
```

## 使用方法

```bash
# 单通道测试（测试图案）
./build/gb28181-sim \
    --server-ip 127.0.0.1 \
    --server-port 15060 \
    --server-id 34020000002000000001 \
    --agent-id 34020000001320000001 \
    --agent-password 12345678 \
    --channel "34020000001310000001:test" \
    --verbose

# 多通道测试
./build/gb28181-sim \
    --server-ip 127.0.0.1 \
    --server-port 15060 \
    --server-id 34020000002000000001 \
    --agent-id 34020000001320000001 \
    --agent-password 12345678 \
    --channel "cam1:test" \
    --channel "cam2:rtsp://admin:admin@192.168.1.200/stream" \
    --channel "cam3:file:///tmp/test.h264"
```

> **必填参数**：`--server-ip`、`--server-id`、`--agent-id`（至少一个 `--channel`）

### 参数说明

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `--server-ip` | ✅ | - | GB28181 平台 IP |
| `--server-port` | - | 5060 | SIP 端口 |
| `--server-id` | ✅ | - | 平台 ID (20位) |
| `--domain` | - | server-id 前10位 | SIP 域 |
| `--agent-id` | ✅ | - | 设备 ID (20位) |
| `--agent-password` | - | 000000 | 设备密码 |
| `--channel` | ✅ | - | 通道配置，格式 `channelID:source`（可指定多个） |
| `--local-ip` | - | 自动检测 | 本地 IP |
| `--tcp` | - | false | 使用 TCP 信令 |
| `--verbose` | - | false | 详细日志 |

### 视频源格式

| 格式 | 说明 |
|------|------|
| `test` | 内置测试图案（无需外部依赖） |
| `rtsp://user:pass@host/path` | RTSP 实时流 |
| `file:///path/to/video.h264` | H.264 原始文件 |
| `file:///path/to/video.h265` | H.265 原始文件 |
| `file:///path/to/video.mp4` | MP4 容器 |

### 与 Python 版本对比

| 特性 | Python 版本 | Go 版本 |
|------|-------------|---------|
| 依赖 | Python3 + GStreamer | 单一二进制 |
| 体积 | ~40KB + 运行时 | ~10MB |
| 部署 | 需要 Python 环境 | 直接运行 |
| 跨平台 | 依赖系统 Python | 交叉编译 |
| PS-RTP | 自研实现 | FFmpeg/GStreamer |

## License

MIT

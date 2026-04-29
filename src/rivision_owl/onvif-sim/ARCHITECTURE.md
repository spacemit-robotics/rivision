# onvif-sim 系统架构设计

## 1. 概述

onvif-sim 是一个 ONVIF 设备模拟器，用于模拟多路 IP 摄像头，供 OWL 平台发现和接入。

### 1.1 与 gb28181-sim 的对比

| 特性 | gb28181-sim | onvif-sim |
|------|-------------|-----------|
| 协议 | GB28181 (SIP + RTP) | ONVIF (SOAP + RTSP) |
| 流媒体传输 | RTP 主动推送到 OWL | RTSP 被动等待拉取 |
| 设备发现 | SIP REGISTER | WS-Discovery |
| 视频源 | file:// / rtsp:// / test | file:// / rtsp:// / test |
| 流输出 | RTP → OWL → zlmediakit → go2rtc | RTSP Server → OWL → go2rtc |

### 1.2 核心设计理念

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           onvif-sim                                      │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐      │
│  │   RTSP Server   │    │  ONVIF Server   │    │  WS-Discovery   │      │
│  │  (视频流输出)    │    │  (设备服务)      │    │  (设备发现)      │      │
│  └────────┬────────┘    └────────┬────────┘    └────────┬────────┘      │
│           │                      │                      │                │
│           └──────────────────────┼──────────────────────┘                │
│                                  │                                       │
│                    ┌─────────────┴─────────────┐                         │
│                    │      Config Manager       │                         │
│                    │   (配置文件: YAML/CLI)     │                         │
│                    └─────────────┬─────────────┘                         │
│                                  │                                       │
│                    ┌─────────────┴─────────────┐                         │
│                    │     Media Source          │                         │
│                    │  (FFmpeg 读取视频文件)     │                         │
│                    └───────────────────────────┘                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## 2. 系统架构

### 2.1 整体数据流

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              完整数据流程                                         │
└─────────────────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐
  │  视频文件     │  file:///opt/rivision-owl/mp4path/cam1.mp4
  │  cam1.mp4    │
  └──────┬───────┘
         │
         ▼
  ┌──────────────┐
  │   FFmpeg     │  读取视频文件，转码/复制
  │  (onvif-sim) │
  └──────┬───────┘
         │
         ▼
  ┌──────────────┐
  │ RTSP Server  │  rtsp://192.168.1.x:18554/cam1
  │  (onvif-sim) │  内置 RTSP 服务器，提供视频流
  └──────┬───────┘
         │
         │  ① WS-Discovery 发现设备
         │  ② ONVIF GetCapabilities / GetProfiles
         │  ③ ONVIF GetStreamUri → 返回 RTSP 地址
         ▼
  ┌──────────────┐
  │     OWL      │  ONVIF 客户端，发现并管理设备
  │   (平台)     │
  └──────┬───────┘
         │
         │  配置 go2rtc 从 RTSP 地址拉流
         ▼
  ┌──────────────┐
  │   go2rtc     │  rtsp://localhost:8554/onvif_cam1
  │  (流媒体)    │  统一流媒体服务
  └──────┬───────┘
         │
         ▼
  ┌──────────────┐
  │   播放器     │  WebRTC / HLS / RTSP
  │  (前端)      │
  └──────────────┘
```

### 2.2 模块设计

```
onvif-sim/
├── main.go                 # 入口，命令行解析
├── config.example.yaml     # 配置文件示例
├── internal/
│   ├── config/             # 配置管理
│   │   └── config.go       # YAML 配置解析
│   ├── rtsp/               # RTSP 服务器模块
│   │   ├── server.go       # RTSP 服务器实现
│   │   └── session.go      # RTSP 会话管理
│   ├── media/              # 媒体源模块
│   │   ├── source.go       # 视频源抽象
│   │   ├── file.go         # 文件读取 (FFmpeg)
│   │   └── test.go         # 测试图案生成
│   └── onvif/              # ONVIF 服务模块
│       ├── server.go       # ONVIF HTTP 服务
│       ├── device.go       # Device Service
│       ├── media.go        # Media Service
│       ├── ptz.go          # PTZ Service
│       └── discovery.go    # WS-Discovery
└── onvif-go/               # ONVIF 协议库 (现有)
```

## 3. 详细设计

### 3.1 RTSP Server 模块

**职责**: 读取视频文件，提供 RTSP 流输出

```go
// RTSPServer 管理多路 RTSP 流
type RTSPServer struct {
    Port     int                    // RTSP 端口 (默认 18554)
    Streams  map[string]*Stream     // 流名 -> 流配置
}

// Stream 单路视频流
type Stream struct {
    Name       string   // 流名 (如 cam1)
    Source     string   // 视频源 (file:///path/to/video.mp4)
    RTSPPath   string   // RTSP 路径 (/cam1)
    FFmpegCmd  *exec.Cmd
}

// 启动流程:
// 1. 解析配置文件，获取视频源列表
// 2. 为每个视频源启动 FFmpeg 进程
// 3. FFmpeg 输出到本地 RTSP 服务器
// 4. ONVIF GetStreamUri 返回对应的 RTSP 地址
```

**FFmpeg 命令示例**:
```bash
# 读取 MP4 文件，输出 RTSP 流
ffmpeg -re -stream_loop -1 -i /path/to/cam1.mp4 \
    -c:v copy -an \
    -f rtsp rtsp://localhost:18554/cam1
```

### 3.2 ONVIF Server 模块

**职责**: 响应 ONVIF 协议请求，返回设备信息和流地址

```go
// GetStreamUri 返回视频流地址
func (s *MediaService) GetStreamUri(profileToken string) string {
    // 从配置获取对应的 RTSP 流地址
    stream := s.server.GetStream(profileToken)
    
    // 返回完整的 RTSP 地址
    // 例: rtsp://192.168.1.100:18554/cam1
    return fmt.Sprintf("rtsp://%s:%d%s", 
        s.server.ExternalIP, 
        s.server.RTSPPort, 
        stream.RTSPPath)
}
```

### 3.3 配置文件设计

```yaml
# onvif-sim.yaml

server:
  host: "0.0.0.0"
  port: 15188              # ONVIF HTTP 端口
  rtsp_port: 18554         # RTSP 服务端口
  verbose: false

auth:
  username: "admin"
  password: "admin"

device:
  manufacturer: "RiVision"
  model: "ONVIF-Sim Virtual Camera"
  serial: "ONVIF-SIM-001"

features:
  ptz: true
  imaging: true
  discovery: true

# 通道配置 (与 gb28181-sim 格式一致)
profiles:
  - name: "大门"
    source: "file:///opt/rivision-owl/mp4path/cam1.mp4"
    width: 1920
    height: 1080
    framerate: 30
    ptz: true

  - name: "停车场"
    source: "file:///opt/rivision-owl/mp4path/cam2.mp4"
    width: 1280
    height: 720
    framerate: 25
    ptz: false
```

## 4. 与 OWL/CLI 集成

### 4.1 从 CLI 角度的工作流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        CLI 视角的集成流程                                 │
└─────────────────────────────────────────────────────────────────────────┘

1. 启动 onvif-sim
   $ ./onvif-sim -c /opt/rivision/rivision_owl/config/onvif-sim.yaml
   
   onvif-sim 执行:
   ├── 启动 RTSP Server (18554)
   ├── 为每个 profile 启动 FFmpeg (读取视频文件)
   ├── 启动 ONVIF HTTP Server (15188)
   └── 启动 WS-Discovery 服务

2. OWL 发现设备
   OWL Discovery → WS-Discovery Probe → onvif-sim 响应 ProbeMatch
   
3. OWL 添加设备
   OWL → ONVIF GetCapabilities → onvif-sim
   OWL → ONVIF GetProfiles → onvif-sim (返回通道列表)
   
4. OWL 获取流地址
   OWL → ONVIF GetStreamUri(profile_0) → onvif-sim
   返回: rtsp://192.168.1.x:18554/cam1
   
5. OWL 配置 go2rtc
   OWL 将流地址添加到 go2rtc 配置:
   streams:
     onvif_大门: rtsp://192.168.1.x:18554/cam1
     onvif_停车场: rtsp://192.168.1.x:18554/cam2

6. 播放视频
   前端 → go2rtc (8554) → onvif-sim RTSP (18554) → 视频文件
```

### 4.2 go2rtc 配置自动生成

当 OWL 通过 ONVIF 发现并添加设备后，会自动更新 go2rtc 配置：

```yaml
# go2rtc.yaml (由 OWL 自动管理)
streams:
  # ONVIF 设备自动添加的流
  onvif_大门: rtsp://192.168.1.100:18554/cam1
  onvif_停车场: rtsp://192.168.1.100:18554/cam2
  onvif_楼梯口: rtsp://192.168.1.100:18554/cam3
```

## 5. 实现方案

### 5.1 方案一：使用 FFmpeg RTSP Server (推荐)

```bash
# FFmpeg 直接作为 RTSP 服务器
ffmpeg -re -stream_loop -1 -i cam1.mp4 \
    -c:v copy -f rtsp \
    -rtsp_transport tcp \
    rtsp://0.0.0.0:18554/cam1
```

**优点**: 简单，无需额外依赖
**缺点**: 每路流一个 FFmpeg 进程

### 5.2 方案二：使用 MediaMTX (rtsp-simple-server)

```yaml
# mediamtx.yml
paths:
  cam1:
    source: publisher
  cam2:
    source: publisher
```

```bash
# FFmpeg 推送到 MediaMTX
ffmpeg -re -stream_loop -1 -i cam1.mp4 \
    -c:v copy -f rtsp \
    rtsp://localhost:8554/cam1
```

**优点**: 专业 RTSP 服务器，支持多客户端
**缺点**: 需要额外部署 MediaMTX

### 5.3 方案三：内嵌 Go RTSP Server

使用 Go 语言实现简单的 RTSP 服务器：

```go
import "github.com/bluenviron/gortsplib/v4"

// 内嵌 RTSP 服务器
server := &gortsplib.Server{
    Handler: &serverHandler{},
    RTSPAddress: ":18554",
}
```

**优点**: 单一进程，易于管理
**缺点**: 实现复杂度高

## 6. 对比 gb28181-sim

| 方面 | gb28181-sim | onvif-sim (新设计) |
|------|-------------|-------------------|
| 视频读取 | FFmpeg | FFmpeg |
| 流输出方式 | RTP 主动推送 | RTSP 被动拉取 |
| 目标地址 | OWL SIP 指定 | 本地 RTSP Server |
| OWL 接收 | zlmediakit | go2rtc |
| 配置文件 | source: file:// | source: file:// |

## 7. 下一步实施计划

1. **修改 onvif-sim**: 添加 RTSP Server 模块
2. **更新配置格式**: source 使用 file:// 格式
3. **修改 GetStreamUri**: 返回内置 RTSP Server 地址
4. **测试完整流程**: OWL 发现 → 获取流 → go2rtc 播放

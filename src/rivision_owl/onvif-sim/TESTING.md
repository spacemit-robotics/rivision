# onvif-sim 测试指南

## 1. 端口配置说明

### 系统端口分配

| 服务 | 端口 | 用途 | 备注 |
|------|------|------|------|
| **OWL** | 15123 | HTTP API | Web 管理界面 |
| **OWL** | 15060 | SIP | GB28181 信令 |
| **ZLMediaKit** | 8220 | HTTP API | 媒体服务器 |
| **go2rtc** | 8554 | RTSP 输出 | 统一流媒体出口 |
| **go2rtc** | 1984 | HTTP API | WebRTC/API |
| **onvif-sim** | 15188 | ONVIF HTTP | 设备发现/控制 |
| **onvif-sim** | 18555 | RTSP | 内置视频流输出 |
| **rivision-cli** | 18554 | RTSP | 独立服务 (不相关) |

### 重要说明

```
┌─────────────────────────────────────────────────────────────────────────┐
│  go2rtc.yaml (/opt/rivision/rivision_cli/go2rtc.yaml)                   │
│  属于 rivision-cli 系统，与 onvif-sim/gb28181-sim 完全独立              │
│  请勿修改此文件！                                                        │
└─────────────────────────────────────────────────────────────────────────┘
```

## 2. 架构对比

### onvif-sim vs gb28181-sim

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        gb28181-sim (主动推流)                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   视频文件 ─────→ FFmpeg ─────→ RTP/GB28181 ─────→ OWL/ZLM ────→ go2rtc │
│   cam1.mp4       读取编码        主动推送           接收处理      输出    │
│                                                                         │
│   特点: 模拟器主动推流，OWL 被动接收                                     │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                        onvif-sim (被动响应)                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   视频文件 ─────→ FFmpeg ─────→ RTSP Server ←───── OWL ←───── go2rtc   │
│   cam1.mp4       读取编码        (18555)           发现设备    拉流播放  │
│                                                                         │
│   特点: 模拟器提供 RTSP 流，OWL 通过 ONVIF 发现并主动拉取                │
└─────────────────────────────────────────────────────────────────────────┘
```

## 3. 测试步骤

### 3.1 准备工作

#### 确认视频文件存在
```bash
# 检查视频文件目录
ls -la /opt/rivision-owl/mp4path/

# 如果目录不存在，创建并放入测试视频
sudo mkdir -p /opt/rivision-owl/mp4path/
# 复制测试视频到此目录
```

#### 检查 FFmpeg 可用
```bash
which ffmpeg
ffmpeg -version
```

### 3.2 启动 onvif-sim

#### 方式一：使用测试图案（无需视频文件）
```bash
# 创建测试配置
cat > /tmp/onvif-test.yaml << 'EOF'
server:
  host: "0.0.0.0"
  port: 15188
  rtsp_port: 18555
  verbose: true

auth:
  username: "admin"
  password: "admin"

device:
  manufacturer: "RiVision"
  model: "ONVIF-Sim Test"
  serial: "TEST-001"

features:
  ptz: true
  imaging: true
  discovery: true

profiles:
  - name: "测试通道1"
    width: 1920
    height: 1080
    framerate: 25
    bitrate: 4096
    source: "test"
    ptz: true

  - name: "测试通道2"
    width: 1280
    height: 720
    framerate: 25
    bitrate: 2048
    source: "test"
    ptz: false
EOF

# 启动
./build/onvif-sim -c /tmp/onvif-test.yaml
```

#### 方式二：使用视频文件
```bash
# 使用配置文件
./build/onvif-sim -c config.example.yaml
```

#### 预期输出
```
📄 使用配置文件: /tmp/onvif-test.yaml
╔═══════════════════════════════════════════════════════════╗
║           🎥  ONVIF Simulator (onvif-sim)  🎥            ║
╚═══════════════════════════════════════════════════════════╝

  地址: http://0.0.0.0:15188/onvif/device_service
  认证: admin / admin
  配置: 2 个摄像头配置文件
  PTZ:  true | Imaging: true | Events: false
  WS-Discovery: true

  💡 设备可被自动发现 (WS-Discovery)
     OWL Discovery 页面将自动发现此设备

  📹 RTSP 服务器已启动 (端口 18555)
     - profile_0: rtsp://192.168.x.x:18555/profile_0
     - profile_1: rtsp://192.168.x.x:18555/profile_1
```

### 3.3 验证 RTSP 流

```bash
# 使用 ffprobe 测试 RTSP 流
ffprobe rtsp://127.0.0.1:18555/profile_0

# 使用 ffplay 播放（需要图形界面）
ffplay rtsp://127.0.0.1:18555/profile_0

# 使用 VLC 播放
vlc rtsp://192.168.x.x:18555/profile_0
```

### 3.4 验证 ONVIF 服务

```bash
# 测试 ONVIF 设备服务
curl -s http://127.0.0.1:15188/onvif/device_service | head -20

# 使用 ONVIF Device Manager (Windows) 或 python-onvif 测试
```

## 4. OWL 集成测试

### 4.1 确保 OWL 正在运行

```bash
# 检查 OWL 进程
ps aux | grep owl

# 检查 OWL 端口
ss -tlnp | grep -E ':(15123|15060)'
```

### 4.2 通过 OWL Web 界面添加设备

1. **打开 OWL 管理界面**
   ```
   http://192.168.x.x:15123
   ```

2. **进入设备发现页面**
   - 导航到 "设备管理" → "ONVIF 发现"
   - 点击 "扫描设备"

3. **发现 onvif-sim 设备**
   - 应该看到 "ONVIF-Sim Virtual Camera" 或配置的设备名称
   - IP: onvif-sim 运行的机器 IP
   - 端口: 15188

4. **添加设备**
   - 点击设备，输入认证信息：
     - 用户名: admin
     - 密码: admin
   - 点击 "添加"

5. **查看通道**
   - 添加成功后，在设备列表中展开设备
   - 应该看到配置的摄像头通道

### 4.3 播放视频流

1. **在 OWL 中点击通道播放**
   - OWL 会自动：
     1. 调用 ONVIF GetStreamUri 获取 RTSP 地址
     2. 配置 go2rtc 从 onvif-sim 的 RTSP 拉流
     3. 通过 WebRTC/HLS 在前端播放

2. **直接通过 go2rtc 播放**
   ```
   # WebRTC 播放
   http://192.168.x.x:1984/stream.html?src=onvif_通道名

   # RTSP 直接播放
   rtsp://192.168.x.x:8554/onvif_通道名
   ```

## 5. 故障排除

### 5.1 WS-Discovery 未发现设备

```bash
# 检查组播是否正常
sudo tcpdump -i any port 3702

# 检查防火墙
sudo iptables -L | grep 3702

# 手动添加设备（绕过发现）
# 在 OWL 中使用 "手动添加" 功能，输入 IP 和端口
```

### 5.2 RTSP 流无法播放

```bash
# 检查 FFmpeg 进程是否在运行
ps aux | grep ffmpeg

# 检查 RTSP 端口是否监听
ss -tlnp | grep 18555

# 查看 onvif-sim 日志（启用 verbose）
./build/onvif-sim -c config.yaml  # 确保 verbose: true
```

### 5.3 端口冲突

```bash
# 检查端口占用
ss -tlnp | grep -E ':(15188|18555)'

# 杀死占用进程
kill -9 $(lsof -t -i:15188)
```

## 6. 配置文件示例

### 完整配置 (生产环境)
```yaml
server:
  host: "0.0.0.0"
  port: 15188
  rtsp_port: 18555
  verbose: false

auth:
  username: "admin"
  password: "your_password"

device:
  manufacturer: "RiVision"
  model: "ONVIF-Sim Virtual Camera"
  serial: "PROD-001"

features:
  ptz: true
  imaging: true
  discovery: true

profiles:
  - name: "大门"
    width: 1920
    height: 1080
    framerate: 25
    bitrate: 4096
    source: "file:///opt/rivision-owl/mp4path/cam1.mp4"
    ptz: true

  - name: "停车场"
    width: 1920
    height: 1080
    framerate: 25
    bitrate: 4096
    source: "file:///opt/rivision-owl/mp4path/cam2.mp4"
    ptz: false
```

## 7. 数据流总结

```
┌────────────────────────────────────────────────────────────────────────────┐
│                          完整数据流                                         │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│   [视频文件]                                                               │
│   cam1.mp4                                                                 │
│       │                                                                    │
│       ▼                                                                    │
│   [onvif-sim]                                                              │
│   ├── FFmpeg 读取视频文件                                                  │
│   ├── RTSP Server (18555) 输出流                                           │
│   └── ONVIF Service (15188) 返回流地址                                     │
│                    │                                                       │
│                    │ WS-Discovery / GetStreamUri                           │
│                    ▼                                                       │
│   [OWL]                                                                    │
│   ├── ONVIF 客户端发现设备                                                 │
│   ├── 获取 RTSP 地址: rtsp://x.x.x.x:18555/profile_0                       │
│   └── 配置 go2rtc 拉流                                                     │
│                    │                                                       │
│                    ▼                                                       │
│   [go2rtc (8554)]                                                          │
│   └── 从 onvif-sim RTSP 拉流，输出 WebRTC/HLS                              │
│                    │                                                       │
│                    ▼                                                       │
│   [播放器/前端]                                                             │
│   └── 播放视频                                                             │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘
```

# ONVIF Simulator (onvif-sim)

虚拟 ONVIF 摄像头模拟器，用于测试 OWL 等 ONVIF 客户端。

## 功能特性

- **ONVIF Profile S** - Device、Media、PTZ、Imaging 服务
- **多摄像头配置** - 支持 1-10 个虚拟摄像头配置文件
- **PTZ 控制** - 云台控制模拟（连续/绝对/相对移动）
- **WS-Security** - UsernameToken 认证
- **跨平台** - 支持 x86_64、RISC-V
- **配置文件** - 支持 YAML 配置文件 (参考 gb28181-sim)

> ⚠️ **注意**: onvif-sim 暂不支持 WS-Discovery，OWL 的 Discovery 页面无法自动发现。请使用「手动添加」方式。

## 编译

```bash
# 通过父级 Makefile 编译
cd src/rivision_owl
make onvif-sim

# 或直接编译
cd onvif-sim && go build -o ../build/onvif-sim .
```

## 使用方法

### 方式 1: 命令行参数

```bash
# 默认启动 (端口 15188, 3 个配置文件)
./bin/onvif-sim

# 自定义端口和配置
./bin/onvif-sim -port 15200 -profiles 5

# 自定义认证
./bin/onvif-sim -username myuser -password mypass
```

### 方式 2: 配置文件 (推荐)

```bash
# 使用配置文件启动
./bin/onvif-sim -c config/onvif-sim.yaml
```

配置文件示例 (`config.example.yaml`):

```yaml
server:
  host: "0.0.0.0"
  port: 15188

auth:
  username: "admin"
  password: "admin"

profiles:
  - name: "主摄像头"
    width: 1920
    height: 1080
    framerate: 30
    source: "rtsp://127.0.0.1:8554/stream0"
```

## 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-host` | 0.0.0.0 | 监听地址 |
| `-port` | 15188 | 监听端口 |
| `-username` | admin | 认证用户名 |
| `-password` | admin | 认证密码 |
| `-profiles` | 3 | 摄像头配置文件数量 (1-10) |
| `-ptz` | true | 启用 PTZ 云台控制 |
| `-imaging` | true | 启用 Imaging 图像控制 |
| `-verbose` | false | 详细日志 |
| `-c` | - | 配置文件路径 (YAML) |

## 与 OWL 配合使用

**步骤 1: 启动 onvif-sim**

```bash
cd build/dist/rivision_owl
./bin/onvif-sim
```

启动后会显示手动添加提示:
```
💡 在 OWL 中手动添加此设备:
   IP: 192.168.1.107, 端口: 15188, 用户: admin, 密码: admin
```

**步骤 2: 在 OWL 中手动添加**

方式 A - Web 界面:
1. 打开 OWL Web UI (`http://localhost:15123`)
2. 进入 **Devices** 页面
3. 点击 **添加设备**
4. 填写: IP=`127.0.0.1`, 端口=`15188`, 用户名=`admin`, 密码=`admin`

方式 B - API:
```bash
curl -X POST http://localhost:15123/api/devices \
  -H "Content-Type: application/json" \
  -d '{"type":"ONVIF","name":"ONVIF-Sim","ip":"127.0.0.1","port":15188,"username":"admin","password":"admin"}'
```

## ONVIF 服务端点

- **Device Service**: `http://127.0.0.1:15188/onvif/device_service`
- **Media Service**: `http://127.0.0.1:15188/onvif/media_service`
- **PTZ Service**: `http://127.0.0.1:15188/onvif/ptz_service`
- **Imaging Service**: `http://127.0.0.1:15188/onvif/imaging_service`

## 与 gb28181-sim 对比

| 特性 | gb28181-sim | onvif-sim |
|------|-------------|-----------|
| 协议 | GB/T 28181 (SIP) | ONVIF (HTTP/SOAP) |
| 视频 | PS-RTP 流 | RTSP 流地址 |
| 认证 | SIP Digest | WS-Security |
| 用途 | 测试 GB28181 平台 | 测试 ONVIF 客户端 |

## License

MIT

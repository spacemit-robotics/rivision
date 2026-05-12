# K3 MPP 多媒体处理库

SpacemiT K3 RISC-V 芯片的 MPP (Multimedia Processing Platform) 库。

## 功能

- **K1_JPU**: 硬件 JPEG 解码 (VDEC)
- **K1_V2D**: 硬件图像缩放/色彩转换 (G2D)

## 目录结构

```
mpp/
├── include/                    # MPP 头文件
│   ├── vdec.h                 # 视频解码 API (JPEG)
│   ├── g2d.h                  # 2D 图形 API (resize, CSC)
│   ├── frame.h                # 帧数据结构
│   └── ...
├── lib/                        # MPP 库文件
│   ├── libspacemit_mpp.so     # 主库
│   ├── libspacemit_mpp.so.0
│   ├── libspacemit_mpp.so.0.0.15
│   └── libspacemit_mpp.tgz    # 压缩包 (自动解压)
└── README.md
```

## 在 K3 上编译 MPP 库

### 前提条件

K3 设备上需要安装以下依赖:
- libopenh264
- ffmpeg (libavcodec)
- spacemit video codec libs (libsfdec, libsfenc, libsf-omx-il)

### 编译步骤

```bash
# 1. 在 K3 设备上克隆 MPP 源码
git clone https://gitee.com/AlaricReject/mpp.git
cd mpp

# 2. 编译
mkdir -p build && cd build
cmake ..
make -j4

# 3. 安装到系统 (可选)
sudo make install

# 4. 或者手动拷贝库文件
cp libspacemit_mpp.so /path/to/rivision/third_party/mpp/lib/
```

### 使用预编译库

如果 K3 系统已安装 MPP，库文件通常位于:
- `/usr/local/lib/libspacemit_mpp.so`
- `/usr/lib/libspacemit_mpp.so`

## 在项目中使用

CMake 会自动检测 MPP 库:
1. 首先检查 `third_party/mpp/lib`
2. 然后检查系统路径 `/usr/local/lib`, `/usr/lib`

启用 MPP:
```bash
# 自动启用 (K3 平台自动检测)
cmake -DCMAKE_SYSTEM_PROCESSOR=riscv64 ..

# 手动启用
cmake -DUSE_K3_MPP=ON -DK3_MPP_ROOT=/path/to/mpp ..

# 禁用 MPP
cmake -DUSE_K3_MPP=OFF ..
```

运行时控制:
```bash
# 禁用 MPP 预处理 (回退到 OpenCV)
export USE_MPP_PREPROCESS=0
./rivision-benchmark ...
```

## API 使用示例

### JPEG 解码 (K1_JPU)

```c
#include "vdec.h"

// 创建解码器
MppVdecCtx *ctx = VDEC_CreateChannel();
ctx->eCodecType = CODEC_K1_JPU;
ctx->stVdecPara.eCodingType = CODING_JPEG;
ctx->stVdecPara.eOutputPixelFormat = PIXEL_FORMAT_NV12;
VDEC_Init(ctx);

// 解码
VDEC_Process(ctx, input_packet, output_frame);

// 清理
VDEC_DestoryChannel(ctx);
```

### 图像缩放 (K1_V2D)

```c
#include "g2d.h"

// 创建处理器
MppG2dCtx *ctx = G2D_CreateChannel();
ctx->eVpsType = VPS_K1_V2D;
ctx->stG2dPara.eG2dCmd = MPP_G2D_CMD_SCALE;
ctx->stG2dPara.eInputPixelFormat = PIXEL_FORMAT_NV12;
ctx->stG2dPara.eOutputPixelFormat = PIXEL_FORMAT_RGB_888;
G2D_Init(ctx);

// 处理
G2D_Process(ctx, input_frame, output_frame);

// 清理
G2D_DestoryChannel(ctx);
```

## 性能

| 操作 | OpenCV (CPU) | MPP (HW) | 提升 |
|------|-------------|----------|------|
| JPEG 解码 (2048x1365) | 20-30ms | 3-5ms | 6x |
| Resize (2048→640) | 10-15ms | 1-2ms | 5x |
| 色彩转换 (NV12→RGB) | 3-5ms | (合并) | 免费 |

## 许可证

BSD-style License (见 MPP 源码 LICENSE 文件)

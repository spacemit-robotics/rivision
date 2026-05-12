// SpacemiT MPP Common utilities
#include "mpp_common.h"
#include "utils/logger.h"

#ifdef USE_SPACEMIT_MPP
extern "C" {
#include "sys/vb_api.h"
#include "sys/sys_api.h"
#include "sys/dma_alloc.h"
}
#endif

#include <cstdlib>

namespace rivision::spacemit {

bool mppInit() {
#ifdef USE_SPACEMIT_MPP
    // Initialize SYS module
    S32 ret = SYS_Init();
    if (ret != 0) {
        LOG_ERROR("Failed to initialize SYS module: {}", ret);
        return false;
    }
    
    // Initialize VB (Video Buffer) module
    ret = VB_Init();
    if (ret != 0) {
        LOG_ERROR("Failed to initialize VB module: {}", ret);
        SYS_Exit();
        return false;
    }
    
    LOG_INFO("SpacemiT MPP initialized");
    return true;
#else
    LOG_WARN("SpacemiT MPP not available");
    return false;
#endif
}

void mppCleanup() {
#ifdef USE_SPACEMIT_MPP
    VB_Exit();
    SYS_Exit();
    LOG_INFO("SpacemiT MPP cleanup complete");
#endif
}

bool nv12ToRgb24(const uint8_t* nv12, int width, int height, uint8_t* rgb24, int stride) {
    if (!nv12 || !rgb24) return false;
    if (stride == 0) stride = width * 3;
    
    const uint8_t* y_plane = nv12;
    const uint8_t* uv_plane = nv12 + width * height;
    
    for (int j = 0; j < height; j++) {
        for (int i = 0; i < width; i++) {
            int y_idx = j * width + i;
            int uv_idx = (j / 2) * width + (i & ~1);
            
            int y = y_plane[y_idx];
            int u = uv_plane[uv_idx] - 128;
            int v = uv_plane[uv_idx + 1] - 128;
            
            int r = y + ((359 * v) >> 8);
            int g = y - ((88 * u + 183 * v) >> 8);
            int b = y + ((454 * u) >> 8);
            
            r = r < 0 ? 0 : (r > 255 ? 255 : r);
            g = g < 0 ? 0 : (g > 255 ? 255 : g);
            b = b < 0 ? 0 : (b > 255 ? 255 : b);
            
            int rgb_idx = j * stride + i * 3;
            rgb24[rgb_idx + 0] = static_cast<uint8_t>(r);
            rgb24[rgb_idx + 1] = static_cast<uint8_t>(g);
            rgb24[rgb_idx + 2] = static_cast<uint8_t>(b);
        }
    }
    
    return true;
}

bool rgb24ToNv12(const uint8_t* rgb24, int width, int height, uint8_t* nv12, int stride) {
    if (!rgb24 || !nv12) return false;
    if (stride == 0) stride = width * 3;
    
    uint8_t* y_plane = nv12;
    uint8_t* uv_plane = nv12 + width * height;
    
    for (int j = 0; j < height; j++) {
        for (int i = 0; i < width; i++) {
            int rgb_idx = j * stride + i * 3;
            int r = rgb24[rgb_idx + 0];
            int g = rgb24[rgb_idx + 1];
            int b = rgb24[rgb_idx + 2];
            
            // RGB to YUV BT.601
            int y = ((66 * r + 129 * g + 25 * b + 128) >> 8) + 16;
            y_plane[j * width + i] = static_cast<uint8_t>(y < 0 ? 0 : (y > 255 ? 255 : y));
            
            // Subsample UV
            if ((j % 2 == 0) && (i % 2 == 0)) {
                int u = ((-38 * r - 74 * g + 112 * b + 128) >> 8) + 128;
                int v = ((112 * r - 94 * g - 18 * b + 128) >> 8) + 128;
                
                int uv_idx = (j / 2) * width + i;
                uv_plane[uv_idx] = static_cast<uint8_t>(u < 0 ? 0 : (u > 255 ? 255 : u));
                uv_plane[uv_idx + 1] = static_cast<uint8_t>(v < 0 ? 0 : (v > 255 ? 255 : v));
            }
        }
    }
    
    return true;
}

bool isMppAvailable() {
#ifdef USE_SPACEMIT_MPP
    return true;
#else
    return false;
#endif
}

VbBuffer allocVbBuffer(size_t size) {
    VbBuffer buf = {};
#ifdef USE_SPACEMIT_MPP
    int fd = -1;
    U64 phys = 0;
    void* virt = nullptr;
    
    S32 ret = dma_alloc_buf(static_cast<U32>(size), &fd, &phys, &virt);
    if (ret != 0 || virt == nullptr) {
        LOG_ERROR("Failed to allocate DMA buffer: {} bytes", size);
        return buf;
    }
    
    buf.size = size;
    buf.phys_addr = phys;
    buf.virt_addr = static_cast<uint8_t*>(virt);
    buf.dma_fd = fd;
#endif
    return buf;
}

void freeVbBuffer(VbBuffer& buf) {
#ifdef USE_SPACEMIT_MPP
    if (buf.virt_addr && buf.dma_fd >= 0) {
        dma_free_buf(buf.dma_fd, buf.virt_addr, static_cast<U32>(buf.size));
        buf.virt_addr = nullptr;
        buf.phys_addr = 0;
        buf.size = 0;
        buf.dma_fd = -1;
    }
#endif
}

} // namespace rivision::spacemit

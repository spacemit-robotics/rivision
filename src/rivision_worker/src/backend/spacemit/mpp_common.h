#pragma once

// SpacemiT MPP common definitions and utilities

#include <cstdint>
#include <string>
#include "hal/interface/error_code.h"

namespace rivision::spacemit {

// =============================================================================
// MPP Error Code Mapping to HAL Error Codes
// =============================================================================

// MPP SDK error codes (from demux_api.h, vdec_api.h, etc.)
constexpr int ERR_MPP_OK = 0;
constexpr int ERR_MPP_NULL_PTR = -1;
constexpr int ERR_MPP_INVALID_CHN = -2;
constexpr int ERR_MPP_OPEN_FAIL = -3;
constexpr int ERR_MPP_DECODE_FAIL = -4;
constexpr int ERR_MPP_ENCODE_FAIL = -5;
constexpr int ERR_MPP_TIMEOUT = -6;
constexpr int ERR_MPP_NO_MEMORY = -7;
constexpr int ERR_MPP_NOT_READY = -8;
constexpr int ERR_MPP_BUSY = -9;

// Map MPP error code to HAL error code
inline hal::ErrorCode mppErrorToHal(int mpp_error) {
    switch (mpp_error) {
        case ERR_MPP_OK:           return hal::ErrorCode::OK;
        case ERR_MPP_NULL_PTR:     return hal::ErrorCode::ERROR_NULL_POINTER;
        case ERR_MPP_INVALID_CHN:  return hal::ErrorCode::ERROR_HW_CHANNEL_INVALID;
        case ERR_MPP_OPEN_FAIL:    return hal::ErrorCode::ERROR_STREAM_OPEN_FAILED;
        case ERR_MPP_DECODE_FAIL:  return hal::ErrorCode::ERROR_CODEC_DECODE_FAILED;
        case ERR_MPP_ENCODE_FAIL:  return hal::ErrorCode::ERROR_CODEC_ENCODE_FAILED;
        case ERR_MPP_TIMEOUT:      return hal::ErrorCode::ERROR_TIMEOUT;
        case ERR_MPP_NO_MEMORY:    return hal::ErrorCode::ERROR_NO_MEMORY;
        case ERR_MPP_NOT_READY:    return hal::ErrorCode::ERROR_NOT_INITIALIZED;
        case ERR_MPP_BUSY:         return hal::ErrorCode::ERROR_BUSY;
        default:                   return hal::ErrorCode::ERROR_UNKNOWN;
    }
}

// Get error message for MPP error code
inline const char* mppErrorString(int mpp_error) {
    return hal::errorCodeToString(mppErrorToHal(mpp_error));
}

// =============================================================================
// Error handling macros with HAL error code support
// =============================================================================

#define MPP_CHECK(call) \
    do { \
        int ret = (call); \
        if (ret != 0) { \
            LOG_ERROR("MPP error: {} ({}) at {}:{}", mppErrorString(ret), ret, __FILE__, __LINE__); \
            return false; \
        } \
    } while (0)

#define MPP_CHECK_RET(call, hal_result) \
    do { \
        int ret = (call); \
        if (ret != 0) { \
            LOG_ERROR("MPP error: {} ({}) at {}:{}", mppErrorString(ret), ret, __FILE__, __LINE__); \
            return mppErrorToHal(ret); \
        } \
    } while (0)

#define MPP_CHECK_NULL(ptr, msg) \
    do { \
        if ((ptr) == nullptr) { \
            LOG_ERROR("Null pointer: {} at {}:{}", msg, __FILE__, __LINE__); \
            return false; \
        } \
    } while (0)

// =============================================================================
// Buffer management
// =============================================================================

struct VbBuffer {
    uint64_t phys_addr = 0;
    uint8_t* virt_addr = nullptr;
    size_t size = 0;
    int dma_fd = -1;
    
    bool isValid() const { return virt_addr != nullptr && size > 0; }
    void reset() { phys_addr = 0; virt_addr = nullptr; size = 0; dma_fd = -1; }
};

// Allocate VB buffer using DMA heap
VbBuffer allocVbBuffer(size_t size);

// Free VB buffer
void freeVbBuffer(VbBuffer& buf);

// =============================================================================
// Frame conversion
// =============================================================================

// NV12 to RGB24
bool nv12ToRgb24(const uint8_t* nv12, int width, int height,
                 uint8_t* rgb24, int stride = 0);

// RGB24 to NV12
bool rgb24ToNv12(const uint8_t* rgb24, int width, int height,
                 uint8_t* nv12, int stride = 0);

// =============================================================================
// MPP initialization
// =============================================================================

// Initialize MPP subsystem (SYS + VB modules)
bool mppInit();

// Cleanup MPP subsystem
void mppCleanup();

// Check if MPP is available
bool isMppAvailable();

} // namespace rivision::spacemit

#pragma once

// HAL Unified Error Codes
// Backend implementations (spacemit/MPP+NPU) map to these codes

#include <string>
#include <cstdint>

namespace rivision::hal {

// =============================================================================
// Error Code Definitions
// =============================================================================

enum class ErrorCode : int32_t {
    // Success
    OK = 0,
    
    // Generic errors (1-99)
    ERROR_UNKNOWN = 1,
    ERROR_INVALID_PARAM = 2,
    ERROR_NULL_POINTER = 3,
    ERROR_NOT_INITIALIZED = 4,
    ERROR_ALREADY_INITIALIZED = 5,
    ERROR_NOT_SUPPORTED = 6,
    ERROR_TIMEOUT = 7,
    ERROR_BUSY = 8,
    ERROR_NO_MEMORY = 9,
    ERROR_PERMISSION_DENIED = 10,
    
    // Stream/Connection errors (100-199)
    ERROR_STREAM_NOT_FOUND = 100,
    ERROR_STREAM_OPEN_FAILED = 101,
    ERROR_STREAM_READ_FAILED = 102,
    ERROR_STREAM_WRITE_FAILED = 103,
    ERROR_STREAM_EOF = 104,
    ERROR_STREAM_DISCONNECTED = 105,
    ERROR_STREAM_RECONNECTING = 106,
    ERROR_STREAM_INVALID_URL = 107,
    ERROR_STREAM_CONNECTION_REFUSED = 108,
    ERROR_STREAM_NETWORK_UNREACHABLE = 109,
    
    // Codec errors (200-299)
    ERROR_CODEC_NOT_FOUND = 200,
    ERROR_CODEC_OPEN_FAILED = 201,
    ERROR_CODEC_DECODE_FAILED = 202,
    ERROR_CODEC_ENCODE_FAILED = 203,
    ERROR_CODEC_INVALID_DATA = 204,
    ERROR_CODEC_UNSUPPORTED_FORMAT = 205,
    ERROR_CODEC_BUFFER_FULL = 206,
    ERROR_CODEC_NEED_MORE_DATA = 207,
    
    // Hardware errors (300-399) - SpacemiT MPP specific
    ERROR_HW_NOT_AVAILABLE = 300,
    ERROR_HW_INIT_FAILED = 301,
    ERROR_HW_RESOURCE_BUSY = 302,
    ERROR_HW_CHANNEL_FULL = 303,
    ERROR_HW_CHANNEL_INVALID = 304,
    ERROR_HW_BUFFER_ALLOC_FAILED = 305,
    ERROR_HW_DMA_FAILED = 306,
    ERROR_HW_VPU_ERROR = 307,
    ERROR_HW_NPU_ERROR = 308,
    ERROR_HW_V2D_ERROR = 309,
    
    // Inference errors (400-499)
    ERROR_INFERENCE_MODEL_NOT_FOUND = 400,
    ERROR_INFERENCE_MODEL_LOAD_FAILED = 401,
    ERROR_INFERENCE_INVALID_INPUT = 402,
    ERROR_INFERENCE_RUNTIME_ERROR = 403,
    ERROR_INFERENCE_OUTPUT_ERROR = 404,
    ERROR_INFERENCE_PROVIDER_NOT_FOUND = 405,
    
    // Graphics errors (500-599)
    ERROR_GRAPHICS_INVALID_FRAME = 500,
    ERROR_GRAPHICS_CONVERSION_FAILED = 501,
    ERROR_GRAPHICS_DRAW_FAILED = 502,
    ERROR_GRAPHICS_RESIZE_FAILED = 503,
};

// =============================================================================
// Error Code Utilities
// =============================================================================

inline const char* errorCodeToString(ErrorCode code) {
    switch (code) {
        case ErrorCode::OK: return "OK";
        case ErrorCode::ERROR_UNKNOWN: return "Unknown error";
        case ErrorCode::ERROR_INVALID_PARAM: return "Invalid parameter";
        case ErrorCode::ERROR_NULL_POINTER: return "Null pointer";
        case ErrorCode::ERROR_NOT_INITIALIZED: return "Not initialized";
        case ErrorCode::ERROR_ALREADY_INITIALIZED: return "Already initialized";
        case ErrorCode::ERROR_NOT_SUPPORTED: return "Not supported";
        case ErrorCode::ERROR_TIMEOUT: return "Timeout";
        case ErrorCode::ERROR_BUSY: return "Busy";
        case ErrorCode::ERROR_NO_MEMORY: return "No memory";
        case ErrorCode::ERROR_PERMISSION_DENIED: return "Permission denied";
        
        case ErrorCode::ERROR_STREAM_NOT_FOUND: return "Stream not found";
        case ErrorCode::ERROR_STREAM_OPEN_FAILED: return "Stream open failed";
        case ErrorCode::ERROR_STREAM_READ_FAILED: return "Stream read failed";
        case ErrorCode::ERROR_STREAM_WRITE_FAILED: return "Stream write failed";
        case ErrorCode::ERROR_STREAM_EOF: return "End of stream";
        case ErrorCode::ERROR_STREAM_DISCONNECTED: return "Stream disconnected";
        case ErrorCode::ERROR_STREAM_RECONNECTING: return "Stream reconnecting";
        case ErrorCode::ERROR_STREAM_INVALID_URL: return "Invalid URL";
        case ErrorCode::ERROR_STREAM_CONNECTION_REFUSED: return "Connection refused";
        case ErrorCode::ERROR_STREAM_NETWORK_UNREACHABLE: return "Network unreachable";
        
        case ErrorCode::ERROR_CODEC_NOT_FOUND: return "Codec not found";
        case ErrorCode::ERROR_CODEC_OPEN_FAILED: return "Codec open failed";
        case ErrorCode::ERROR_CODEC_DECODE_FAILED: return "Decode failed";
        case ErrorCode::ERROR_CODEC_ENCODE_FAILED: return "Encode failed";
        case ErrorCode::ERROR_CODEC_INVALID_DATA: return "Invalid codec data";
        case ErrorCode::ERROR_CODEC_UNSUPPORTED_FORMAT: return "Unsupported format";
        case ErrorCode::ERROR_CODEC_BUFFER_FULL: return "Codec buffer full";
        case ErrorCode::ERROR_CODEC_NEED_MORE_DATA: return "Need more data";
        
        case ErrorCode::ERROR_HW_NOT_AVAILABLE: return "Hardware not available";
        case ErrorCode::ERROR_HW_INIT_FAILED: return "Hardware init failed";
        case ErrorCode::ERROR_HW_RESOURCE_BUSY: return "Hardware resource busy";
        case ErrorCode::ERROR_HW_CHANNEL_FULL: return "Hardware channel full";
        case ErrorCode::ERROR_HW_CHANNEL_INVALID: return "Invalid hardware channel";
        case ErrorCode::ERROR_HW_BUFFER_ALLOC_FAILED: return "Hardware buffer alloc failed";
        case ErrorCode::ERROR_HW_DMA_FAILED: return "DMA failed";
        case ErrorCode::ERROR_HW_VPU_ERROR: return "VPU error";
        case ErrorCode::ERROR_HW_NPU_ERROR: return "NPU error";
        case ErrorCode::ERROR_HW_V2D_ERROR: return "V2D error";
        
        case ErrorCode::ERROR_INFERENCE_MODEL_NOT_FOUND: return "Model not found";
        case ErrorCode::ERROR_INFERENCE_MODEL_LOAD_FAILED: return "Model load failed";
        case ErrorCode::ERROR_INFERENCE_INVALID_INPUT: return "Invalid inference input";
        case ErrorCode::ERROR_INFERENCE_RUNTIME_ERROR: return "Inference runtime error";
        case ErrorCode::ERROR_INFERENCE_OUTPUT_ERROR: return "Inference output error";
        case ErrorCode::ERROR_INFERENCE_PROVIDER_NOT_FOUND: return "Provider not found";
        
        case ErrorCode::ERROR_GRAPHICS_INVALID_FRAME: return "Invalid frame";
        case ErrorCode::ERROR_GRAPHICS_CONVERSION_FAILED: return "Conversion failed";
        case ErrorCode::ERROR_GRAPHICS_DRAW_FAILED: return "Draw failed";
        case ErrorCode::ERROR_GRAPHICS_RESIZE_FAILED: return "Resize failed";
        
        default: return "Unknown error code";
    }
}

inline bool isSuccess(ErrorCode code) {
    return code == ErrorCode::OK;
}

inline bool isRetryable(ErrorCode code) {
    switch (code) {
        case ErrorCode::ERROR_TIMEOUT:
        case ErrorCode::ERROR_BUSY:
        case ErrorCode::ERROR_STREAM_RECONNECTING:
        case ErrorCode::ERROR_CODEC_NEED_MORE_DATA:
        case ErrorCode::ERROR_HW_RESOURCE_BUSY:
            return true;
        default:
            return false;
    }
}

// =============================================================================
// Result type for operations that can fail
// =============================================================================

template<typename T>
struct Result {
    ErrorCode error = ErrorCode::OK;
    T value;
    
    bool ok() const { return error == ErrorCode::OK; }
    explicit operator bool() const { return ok(); }
    
    static Result<T> success(T&& v) { return {ErrorCode::OK, std::forward<T>(v)}; }
    static Result<T> failure(ErrorCode e) { return {e, T{}}; }
};

// Specialization for void
template<>
struct Result<void> {
    ErrorCode error = ErrorCode::OK;
    
    bool ok() const { return error == ErrorCode::OK; }
    explicit operator bool() const { return ok(); }
    
    static Result<void> success() { return {ErrorCode::OK}; }
    static Result<void> failure(ErrorCode e) { return {e}; }
};

} // namespace rivision::hal

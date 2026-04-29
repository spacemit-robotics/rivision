#pragma once

#include <cstdint>
#include <cstddef>
#include <string>

namespace rivision {
namespace benchmark {

/**
 * Preprocessor interface for JPEG decoding and image preprocessing
 * 
 * Implementations:
 * - FFmpegPreprocessor: FFmpeg-based with hardware JPEG decode (K3 mjpeg_stcodec)
 * - K3MppPreprocessor: MPP-based (K1 JPU/V2D, limited K3 support)
 * - OpenCV fallback: Software decode (built into local_yolo_backend)
 */
class IPreprocessor {
public:
    virtual ~IPreprocessor() = default;
    
    /**
     * Preprocess JPEG data to normalized float tensor
     * 
     * @param jpeg_data      Input JPEG data
     * @param jpeg_size      Size of JPEG data in bytes
     * @param output_buffer  Output buffer for float32 tensor (NCHW format, normalized [0,1])
     * @param target_width   Target width (e.g., 640 for YOLO)
     * @param target_height  Target height (e.g., 640 for YOLO)
     * @return true on success, false on failure
     */
    virtual bool preprocess(const uint8_t* jpeg_data, size_t jpeg_size,
                           float* output_buffer,
                           int target_width, int target_height) = 0;
    
    /**
     * Get preprocessor name
     */
    virtual std::string getName() const = 0;
    
    /**
     * Check if hardware acceleration is enabled
     */
    virtual bool isHardwareAccelerated() const = 0;
};

}  // namespace benchmark
}  // namespace rivision

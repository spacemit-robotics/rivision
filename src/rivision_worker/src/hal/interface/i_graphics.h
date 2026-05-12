#pragma once

#include "rivision/types.h"
#include <memory>
#include <string>

namespace rivision::hal {

// =============================================================================
// Color structure
// =============================================================================

struct Color {
    uint8_t r = 0;
    uint8_t g = 0;
    uint8_t b = 0;
    uint8_t a = 255;
    
    Color() = default;
    Color(uint8_t r, uint8_t g, uint8_t b, uint8_t a = 255) 
        : r(r), g(g), b(b), a(a) {}
    
    static Color red() { return {255, 0, 0}; }
    static Color green() { return {0, 255, 0}; }
    static Color blue() { return {0, 0, 255}; }
    static Color yellow() { return {255, 255, 0}; }
    static Color cyan() { return {0, 255, 255}; }
    static Color magenta() { return {255, 0, 255}; }
    static Color white() { return {255, 255, 255}; }
    static Color black() { return {0, 0, 0}; }
};

// =============================================================================
// IGraphics - 2D graphics/OSD interface
// =============================================================================

class IGraphics {
public:
    virtual ~IGraphics() = default;
    
    struct Config {
        int width = 0;
        int height = 0;
        PixelFormat format = PixelFormat::NV12;
        
        // Font settings
        std::string font_path;
        int font_size = 16;
    };
    
    // Initialize
    virtual bool init(const Config& cfg) = 0;
    
    // Release
    virtual void release() = 0;
    
    // Draw rectangle
    virtual void drawRect(Frame& frame, const BBox& bbox, 
                         const Color& color, int thickness = 2) = 0;
    
    // Draw filled rectangle
    virtual void fillRect(Frame& frame, const BBox& bbox, 
                         const Color& color, float alpha = 1.0f) = 0;
    
    // Draw text
    virtual void drawText(Frame& frame, const std::string& text,
                         int x, int y, const Color& color) = 0;
    
    // Draw line
    virtual void drawLine(Frame& frame, int x1, int y1, int x2, int y2,
                         const Color& color, int thickness = 1) = 0;
    
    // Draw detection boxes with labels
    void drawDetections(Frame& frame, 
                       const std::vector<Detection>& detections,
                       bool show_labels = true,
                       bool show_confidence = true);
    
    // Draw tracks with IDs
    void drawTracks(Frame& frame,
                   const std::vector<Track>& tracks,
                   bool show_trajectory = false);
    
    // Draw zone overlay
    void drawZone(Frame& frame,
                 const std::vector<std::pair<float, float>>& polygon,
                 const Color& color,
                 float alpha = 0.3f);
    
protected:
    Config config_;
    
    // Get color by class ID
    Color getClassColor(int class_id) const {
        static const Color colors[] = {
            {255, 0, 0}, {0, 255, 0}, {0, 0, 255},
            {255, 255, 0}, {255, 0, 255}, {0, 255, 255},
            {128, 0, 0}, {0, 128, 0}, {0, 0, 128},
            {128, 128, 0}, {128, 0, 128}, {0, 128, 128}
        };
        return colors[class_id % 12];
    }
};

// Factory function
std::unique_ptr<IGraphics> createGraphics();

} // namespace rivision::hal

// SpacemiT MPP Graphics - V2D hardware accelerated graphics
#pragma once

#include "hal/interface/i_graphics.h"
#include <memory>
#include <string>

namespace rivision::spacemit {

class MppGraphics : public hal::IGraphics {
public:
    MppGraphics();
    ~MppGraphics() override;
    
    // IGraphics interface - exact signature match
    bool init(const hal::IGraphics::Config& cfg) override;
    void release() override;
    
    void drawRect(Frame& frame, const rivision::BBox& bbox, 
                  const hal::Color& color, int thickness = 2) override;
    void fillRect(Frame& frame, const rivision::BBox& bbox, 
                  const hal::Color& color, float alpha = 1.0f) override;
    void drawText(Frame& frame, const std::string& text,
                  int x, int y, const hal::Color& color) override;
    void drawLine(Frame& frame, int x1, int y1, int x2, int y2,
                  const hal::Color& color, int thickness = 1) override;
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    int width_ = 0;
    int height_ = 0;
    PixelFormat format_ = PixelFormat::NV12;
    std::string last_error_;
    bool initialized_ = false;
};

} // namespace rivision::spacemit

// SpacemiT MPP Graphics implementation (using OpenCV with optional V2D)
#include "mpp_graphics.h"
#include "mpp_common.h"
#include "utils/logger.h"

#include <opencv2/opencv.hpp>

namespace rivision::spacemit {

class MppGraphics::Impl {
public:
    VbBuffer work_buffer;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
        freeVbBuffer(work_buffer);
    }
};

MppGraphics::MppGraphics() : impl_(std::make_unique<Impl>()) {}
MppGraphics::~MppGraphics() = default;

bool MppGraphics::init(const hal::IGraphics::Config& cfg) {
    config_ = cfg;
    width_ = cfg.width;
    height_ = cfg.height;
    format_ = cfg.format;
    
#ifdef USE_SPACEMIT_MPP
    // Allocate work buffer
    size_t buf_size = cfg.width * cfg.height * 4; // RGBA
    impl_->work_buffer = allocVbBuffer(buf_size);
#endif
    
    initialized_ = true;
    LOG_INFO("MPP Graphics initialized: {}x{}", cfg.width, cfg.height);
    return true;
}

void MppGraphics::release() {
    impl_->cleanup();
    initialized_ = false;
}

void MppGraphics::drawRect(Frame& frame, const rivision::BBox& bbox, const hal::Color& color, int thickness) {
    if (!initialized_) return;
    
    // Use OpenCV for drawing (V2D primarily for color conversion/resize)
    try {
        cv::Mat img;
        if (frame.format == PixelFormat::NV12) {
            cv::Mat yuv(frame.height * 3 / 2, frame.width, CV_8UC1, frame.cpuData());
            cv::cvtColor(yuv, img, cv::COLOR_YUV2BGR_NV12);
        } else if (frame.format == PixelFormat::BGR24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3, frame.cpuData());
        } else {
            return;
        }
        
        cv::Scalar cv_color(color.b, color.g, color.r);
        cv::rectangle(img, cv::Point(static_cast<int>(bbox.x1), static_cast<int>(bbox.y1)),
                      cv::Point(static_cast<int>(bbox.x2), static_cast<int>(bbox.y2)),
                      cv_color, thickness);
    } catch (...) {
    }
}

void MppGraphics::fillRect(Frame& frame, const rivision::BBox& bbox, const hal::Color& color, float alpha) {
    if (!initialized_) return;
    
    try {
        cv::Mat img(frame.height, frame.width, CV_8UC3, frame.cpuData());
        cv::Scalar cv_color(color.b, color.g, color.r);
        
        if (alpha >= 1.0f) {
            cv::rectangle(img, 
                cv::Point(static_cast<int>(bbox.x1), static_cast<int>(bbox.y1)),
                cv::Point(static_cast<int>(bbox.x2), static_cast<int>(bbox.y2)),
                cv_color, cv::FILLED);
        } else {
            cv::Mat overlay = img.clone();
            cv::rectangle(overlay,
                cv::Point(static_cast<int>(bbox.x1), static_cast<int>(bbox.y1)),
                cv::Point(static_cast<int>(bbox.x2), static_cast<int>(bbox.y2)),
                cv_color, cv::FILLED);
            cv::addWeighted(overlay, alpha, img, 1.0 - alpha, 0, img);
        }
    } catch (...) {
    }
}

void MppGraphics::drawText(Frame& frame, const std::string& text, int x, int y,
                           const hal::Color& color) {
    if (!initialized_) return;
    
    try {
        cv::Mat img(frame.height, frame.width, CV_8UC3, frame.cpuData());
        cv::Scalar cv_color(color.b, color.g, color.r);
        cv::putText(img, text, cv::Point(x, y),
                    cv::FONT_HERSHEY_SIMPLEX, 0.5, cv_color, 1);
    } catch (...) {
    }
}

void MppGraphics::drawLine(Frame& frame, int x1, int y1, int x2, int y2,
                           const hal::Color& color, int thickness) {
    if (!initialized_) return;
    
    try {
        cv::Mat img(frame.height, frame.width, CV_8UC3, frame.cpuData());
        cv::Scalar cv_color(color.b, color.g, color.r);
        cv::line(img, cv::Point(x1, y1), cv::Point(x2, y2), cv_color, thickness);
    } catch (...) {
    }
}

} // namespace rivision::spacemit

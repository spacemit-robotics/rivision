#include "logger.h"

namespace rivision::utils {

LogLevel Logger::level_ = LogLevel::INFO;
std::mutex Logger::mutex_;

void Logger::init(const std::string& name,
                 const std::string& level,
                 const std::string& file,
                 int max_size_mb,
                 int max_files) {
    setLevel(level);
    LOG_INFO("Logger initialized: {} level={}", name, level);
}

void Logger::setLevel(const std::string& level) {
    if (level == "trace") {
        level_ = LogLevel::TRACE;
    } else if (level == "debug") {
        level_ = LogLevel::DEBUG;
    } else if (level == "info") {
        level_ = LogLevel::INFO;
    } else if (level == "warn" || level == "warning") {
        level_ = LogLevel::WARN;
    } else if (level == "error") {
        level_ = LogLevel::ERROR;
    } else if (level == "critical") {
        level_ = LogLevel::CRITICAL;
    } else {
        level_ = LogLevel::INFO;
    }
}

} // namespace rivision::utils

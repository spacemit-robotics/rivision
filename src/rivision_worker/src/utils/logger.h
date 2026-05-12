#pragma once

#include <string>
#include <iostream>
#include <sstream>
#include <chrono>
#include <iomanip>
#include <mutex>

namespace rivision::utils {

enum class LogLevel { TRACE, DEBUG, INFO, WARN, ERROR, CRITICAL };

class Logger {
public:
    static void init(const std::string& name,
                    const std::string& level = "info",
                    const std::string& file = "",
                    int max_size_mb = 100,
                    int max_files = 5);
    
    static void setLevel(const std::string& level);
    static void setLevel(LogLevel level) { level_ = level; }
    static LogLevel getLevel() { return level_; }
    static void flush() { std::cout.flush(); std::cerr.flush(); }
    static void shutdown() {}
    
    template<typename... Args>
    static void log(LogLevel level, const char* fmt, Args&&... args) {
        if (level < level_) return;
        
        std::lock_guard<std::mutex> lock(mutex_);
        
        auto now = std::chrono::system_clock::now();
        auto time = std::chrono::system_clock::to_time_t(now);
        auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
            now.time_since_epoch()) % 1000;
        
        std::ostringstream oss;
        oss << std::put_time(std::localtime(&time), "%Y-%m-%d %H:%M:%S");
        oss << '.' << std::setfill('0') << std::setw(3) << ms.count();
        oss << " [" << levelStr(level) << "] ";
        
        formatTo(oss, fmt, std::forward<Args>(args)...);
        oss << '\n';
        
        if (level >= LogLevel::ERROR) {
            std::cerr << oss.str();
        } else {
            std::cout << oss.str();
        }
    }
    
private:
    static LogLevel level_;
    static std::mutex mutex_;
    
    static const char* levelStr(LogLevel level) {
        switch (level) {
            case LogLevel::TRACE: return "TRACE";
            case LogLevel::DEBUG: return "DEBUG";
            case LogLevel::INFO:  return "INFO";
            case LogLevel::WARN:  return "WARN";
            case LogLevel::ERROR: return "ERROR";
            case LogLevel::CRITICAL: return "CRITICAL";
            default: return "?";
        }
    }
    
    static void formatTo(std::ostringstream& oss, const char* fmt) {
        oss << fmt;
    }
    
    template<typename T, typename... Args>
    static void formatTo(std::ostringstream& oss, const char* fmt, T&& val, Args&&... args) {
        while (*fmt) {
            if (*fmt == '{' && *(fmt + 1) == '}') {
                oss << std::forward<T>(val);
                formatTo(oss, fmt + 2, std::forward<Args>(args)...);
                return;
            }
            oss << *fmt++;
        }
    }
};

} // namespace rivision::utils

#define LOG_TRACE(...) rivision::utils::Logger::log(rivision::utils::LogLevel::TRACE, __VA_ARGS__)
#define LOG_DEBUG(...) rivision::utils::Logger::log(rivision::utils::LogLevel::DEBUG, __VA_ARGS__)
#define LOG_INFO(...)  rivision::utils::Logger::log(rivision::utils::LogLevel::INFO, __VA_ARGS__)
#define LOG_WARN(...)  rivision::utils::Logger::log(rivision::utils::LogLevel::WARN, __VA_ARGS__)
#define LOG_ERROR(...) rivision::utils::Logger::log(rivision::utils::LogLevel::ERROR, __VA_ARGS__)
#define LOG_CRITICAL(...) rivision::utils::Logger::log(rivision::utils::LogLevel::CRITICAL, __VA_ARGS__)

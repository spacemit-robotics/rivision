#include "core/worker.h"
#include "utils/logger.h"
#include "rivision/export.h"

#include <iostream>
#include <csignal>
#include <atomic>
#include <thread>

using namespace rivision;

// Global worker instance for signal handling
static std::atomic<bool> g_running{true};
static core::Worker* g_worker = nullptr;

void signalHandler(int signum) {
    LOG_INFO("Received signal {}, shutting down...", signum);
    g_running = false;
    if (g_worker) {
        g_worker->stop();
    }
}

void printBanner() {
    std::cout << R"(
  ____  _ __     ___     _             __        __         _             
 |  _ \(_)\ \   / (_)___(_) ___  _ __  \ \      / /__  _ __| | _____ _ __ 
 | |_) | |\ \ / /| / __| |/ _ \| '_ \  \ \ /\ / / _ \| '__| |/ / _ \ '__|
 |  _ <| | \ V / | \__ \ | (_) | | | |  \ V  V / (_) | |  |   <  __/ |   
 |_| \_\_|  \_/  |_|___/_|\___/|_| |_|   \_/\_/ \___/|_|  |_|\_\___|_|   
                                                                          
)" << std::endl;
    std::cout << "Version: " << RIVISION_VERSION_STRING << std::endl;
    std::cout << "=========================================" << std::endl;
}

void printUsage(const char* prog) {
    std::cout << "Usage: " << prog << " [options]\n"
              << "Options:\n"
              << "  -c, --config <path>   Config file path (default: config/worker.yaml)\n"
              << "  -d, --data-dir <path> Data directory (overrides config)\n"
              << "  -l, --log-level <lvl> Log level: trace|debug|info|warn|error\n"
              << "  -h, --help            Show this help\n"
              << "  -v, --version         Show version\n"
              << std::endl;
}

int main(int argc, char* argv[]) {
    // Parse command line arguments
    std::string config_path = "config/worker.yaml";
    std::string data_dir;
    std::string log_level;
    
    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        
        if (arg == "-h" || arg == "--help") {
            printUsage(argv[0]);
            return 0;
        }
        
        if (arg == "-v" || arg == "--version") {
            std::cout << "rivision_worker " << RIVISION_VERSION_STRING << std::endl;
            return 0;
        }
        
        if ((arg == "-c" || arg == "--config") && i + 1 < argc) {
            config_path = argv[++i];
        }
        
        if ((arg == "-d" || arg == "--data-dir") && i + 1 < argc) {
            data_dir = argv[++i];
        }
        
        if ((arg == "-l" || arg == "--log-level") && i + 1 < argc) {
            log_level = argv[++i];
        }
    }
    
    // Print banner
    printBanner();
    
    // Initialize logger (early, before config load)
    utils::Logger::init("rivision_worker", log_level.empty() ? "info" : log_level);
    
    LOG_INFO("Starting rivision_worker...");
    LOG_INFO("Config: {}", config_path);
    
    // Load configuration
    WorkerConfig config;
    try {
        config = WorkerConfig::load(config_path);
        
        // Override with command line
        if (!data_dir.empty()) {
            config.storage.data_dir = data_dir;
        }
        if (!log_level.empty()) {
            config.log.level = log_level;
        }
        
        // Initialize paths
        config.initPaths();
        
        // Re-init logger with config
        utils::Logger::init("rivision_worker", config.log.level, 
                           config.log.file, config.log.max_size_mb, config.log.max_files);
        
    } catch (const std::exception& e) {
        LOG_ERROR("Failed to load config: {}", e.what());
        LOG_WARN("Using default configuration");
        config.initPaths();
    }
    
    // Setup signal handlers
    std::signal(SIGINT, signalHandler);
    std::signal(SIGTERM, signalHandler);
    
    // Create and initialize worker
    core::Worker worker;
    g_worker = &worker;
    
    if (!worker.init(config)) {
        LOG_ERROR("Failed to initialize worker");
        return 1;
    }
    
    LOG_INFO("Worker initialized successfully");
    
    // Start worker
    if (!worker.start()) {
        LOG_ERROR("Failed to start worker");
        return 1;
    }
    
    LOG_INFO("Worker started, node_id={}", config.node_id);
    LOG_INFO("HTTP server listening on {}:{}", config.server.host, config.server.http_port);
    
    // Wait for shutdown signal
    while (g_running && worker.isRunning()) {
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
    }
    
    // Graceful shutdown
    LOG_INFO("Shutting down...");
    worker.stop();
    
    LOG_INFO("Worker stopped");
    g_worker = nullptr;
    
    return 0;
}

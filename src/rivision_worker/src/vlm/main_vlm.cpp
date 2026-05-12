#include "vlm_server.h"
#include "utils/logger.h"
#include <iostream>
#include <csignal>
#include <atomic>
#include <thread>
#include <chrono>

using namespace rivision;

static std::atomic<bool> g_running{true};

static void signalHandler(int sig) {
    LOG_INFO("Received signal {}, shutting down...", sig);
    g_running = false;
}

static void printUsage(const char* prog) {
    std::cout << "Usage: " << prog << " [options]\n"
              << "Options:\n"
              << "  -c, --config <file>   Config file path (default: config/vlm.yaml)\n"
              << "  -p, --port <port>     Server port (default: 8090)\n"
              << "  -m, --model <path>    Model directory path\n"
              << "  -h, --help            Show this help\n";
}

int main(int argc, char* argv[]) {
    // Parse command line arguments
    std::string config_path = "config/vlm.yaml";
    std::string model_path;
    int port = 8090;
    
    for (int i = 1; i < argc; i++) {
        std::string arg = argv[i];
        if ((arg == "-c" || arg == "--config") && i + 1 < argc) {
            config_path = argv[++i];
        } else if ((arg == "-p" || arg == "--port") && i + 1 < argc) {
            port = std::stoi(argv[++i]);
        } else if ((arg == "-m" || arg == "--model") && i + 1 < argc) {
            model_path = argv[++i];
        } else if (arg == "-h" || arg == "--help") {
            printUsage(argv[0]);
            return 0;
        }
    }
    
    // Setup signal handlers
    std::signal(SIGINT, signalHandler);
    std::signal(SIGTERM, signalHandler);
    
    // Initialize logger
    LOG_INFO("RiVision VLM Server starting...");
    
    // Load config
    VlmConfig config;
    config.server.port = port;
    if (!model_path.empty()) {
        config.model.path = model_path;
    }
    
    // Create and init server
    vlm::VlmServer server;
    
    if (!server.init(config)) {
        LOG_ERROR("Failed to initialize VLM server");
        return 1;
    }
    
    // Start server
    if (!server.start(config.server.port)) {
        LOG_ERROR("Failed to start VLM server");
        return 1;
    }
    
    LOG_INFO("VLM Server running on port {}. Press Ctrl+C to stop.", config.server.port);
    
    // Main loop
    while (g_running) {
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
    }
    
    // Cleanup
    server.stop();
    LOG_INFO("VLM Server stopped");
    
    return 0;
}

// spacemit_cgo.cpp - C++ implementation of SpaceMIT EP wrapper
// Compiled only on RISC-V K3 platform with SpaceMIT toolchain

#include "spacemit_cgo.h"
#include "spacemit_ort_env.h"
#include <iostream>
#include <cstdlib>

// Global flag to track initialization
static bool g_spacemit_initialized = false;

extern "C" {

// Initialize SpaceMIT EP environment (call once at startup)
int SpaceMIT_EnvInit(void) {
    if (g_spacemit_initialized) {
        return 0;  // Already initialized
    }
    
    // Check for NPU device
    const char* npu_disable = std::getenv("SPACEMIT_NPU_DISABLE");
    if (npu_disable && strcmp(npu_disable, "1") == 0) {
        std::cout << "[SpaceMIT] NPU disabled by environment variable" << std::endl;
        return -1;
    }
    
    std::cout << "[SpaceMIT] A100 NPU environment ready (EP will be attached to sessions)" << std::endl;
    g_spacemit_initialized = true;
    return 0;
}

// Attach SpaceMIT EP to session options
// 根据官方文档: SPACEMIT_EP_INTRA_THREAD_NUM=N 会启动 2*N 个计算线程
// 要使用 8 个 A100 核心，应设置 SPACEMIT_EP_INTRA_THREAD_NUM=4
int SpaceMIT_AttachToSessionOptions(OrtSessionOptions* options, int num_threads) {
    if (options == nullptr) {
        std::cerr << "[SpaceMIT] Invalid session options pointer" << std::endl;
        return -1;
    }
    
    if (!g_spacemit_initialized) {
        SpaceMIT_EnvInit();
    }
    
    try {
        // Create C++ wrapper around C handle
        Ort::SessionOptions session_opts(options);
        
        // ★ 根据官方文档优化配置:
        // 1. ORT 线程设为 1，完全由 EP 控制线程
        session_opts.SetIntraOpNumThreads(1);
        session_opts.SetInterOpNumThreads(1);
        
        // 2. 禁止线程缓存清理，提高连续推理性能
        session_opts.AddConfigEntry("session.use_env_allocators", "1");
        
        // Configure provider options
        std::unordered_map<std::string, std::string> provider_options;
        
        // ★ 关键: EP 线程数 = num_threads/2，因为 EP 会创建 2×N 线程
        // 要使用 8 核，设置 SPACEMIT_EP_INTRA_THREAD_NUM=4 (创建 8 线程)
        int ep_threads = (num_threads + 1) / 2;  // 8->4, 4->2
        if (ep_threads < 1) ep_threads = 1;
        provider_options["SPACEMIT_EP_INTRA_THREAD_NUM"] = std::to_string(ep_threads);
        
        // ★ 多 Session 共享线程池 (embed 有 text/vision 两个 session)
        provider_options["SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD"] = "1";
        
        std::cout << "[SpaceMIT] EP config: INTRA_THREAD_NUM=" << ep_threads 
                  << " (actual threads=" << ep_threads * 2 << ")" << std::endl;
        
        // Initialize SpaceMIT EP
        OrtStatus* status = Ort::SessionOptionsSpaceMITEnvInit(session_opts, provider_options);
        
        if (status == nullptr) {
            std::cout << "[SpaceMIT] A100 NPU EP attached successfully" << std::endl;
            return 0;
        } else {
            Ort::Status s(status);
            std::cerr << "[SpaceMIT] EP attach failed: " << s.GetErrorMessage() << std::endl;
            return -2;
        }
    } catch (const Ort::Exception& e) {
        std::cerr << "[SpaceMIT] ORT exception: " << e.what() << std::endl;
        return -3;
    } catch (const std::exception& e) {
        std::cerr << "[SpaceMIT] Exception: " << e.what() << std::endl;
        return -4;
    }
}

int SpaceMIT_IsAvailable(void) {
    return g_spacemit_initialized ? 1 : 0;
}

} // extern "C"

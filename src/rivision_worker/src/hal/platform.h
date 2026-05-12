#pragma once

#include <string>

namespace rivision::hal {

// K3-only build: single supported platform.
enum class Platform {
    UNKNOWN,
    RISCV64_SPACEMIT
};

class PlatformDetector {
public:
    static Platform detect();

    static std::string platformToString(Platform p);

    static bool isNpuAvailable();

    static bool isSpacemiTMppAvailable();

    static std::string getCpuInfo();

    static int getCpuCount();

    static int64_t getTotalMemory();

    static std::string getKernelVersion();
};

} // namespace rivision::hal

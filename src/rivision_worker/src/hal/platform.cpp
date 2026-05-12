#include "platform.h"
#include <fstream>
#include <sstream>
#include <cstring>
#include <unistd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>

namespace rivision::hal {

Platform PlatformDetector::detect() {
#if defined(__riscv) && (__riscv_xlen == 64)
    if (isSpacemiTMppAvailable()) {
        return Platform::RISCV64_SPACEMIT;
    }
#endif
    return Platform::UNKNOWN;
}

std::string PlatformDetector::platformToString(Platform p) {
    switch (p) {
        case Platform::RISCV64_SPACEMIT: return "riscv64-spacemit";
        default: return "unknown";
    }
}

bool PlatformDetector::isNpuAvailable() {
    std::ifstream f("/dev/npu");
    return f.good();
}

bool PlatformDetector::isSpacemiTMppAvailable() {
    std::ifstream f("/dev/mpp");
    return f.good();
}

std::string PlatformDetector::getCpuInfo() {
    std::ifstream f("/proc/cpuinfo");
    if (!f.is_open()) return "unknown";

    std::string line;
    while (std::getline(f, line)) {
        if (line.find("model name") != std::string::npos ||
            line.find("cpu model") != std::string::npos) {
            size_t pos = line.find(':');
            if (pos != std::string::npos) {
                std::string value = line.substr(pos + 1);
                while (!value.empty() && value[0] == ' ') {
                    value.erase(0, 1);
                }
                return value;
            }
        }
    }

    return "unknown";
}

int PlatformDetector::getCpuCount() {
    return sysconf(_SC_NPROCESSORS_ONLN);
}

int64_t PlatformDetector::getTotalMemory() {
    struct sysinfo info;
    if (sysinfo(&info) == 0) {
        return static_cast<int64_t>(info.totalram) * info.mem_unit;
    }
    return 0;
}

std::string PlatformDetector::getKernelVersion() {
    struct utsname buf;
    if (uname(&buf) == 0) {
        return buf.release;
    }
    return "unknown";
}

} // namespace rivision::hal

# CMake Toolchain File for RISC-V 64 (K3 SpacemiT)
# Usage: cmake -DCMAKE_TOOLCHAIN_FILE=cmake/toolchain-riscv64.cmake ..

set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR riscv64)

# 获取项目根目录
if(NOT DEFINED PROJECT_ROOT)
    # benchmark -> rivision_node -> src -> rivision-k3-deploy
    get_filename_component(PROJECT_ROOT "${CMAKE_CURRENT_LIST_DIR}/../../../.." ABSOLUTE)
endif()

# SpacemiT 工具链路径
set(SPACEMIT_TOOLCHAIN "${PROJECT_ROOT}/tools/spacemit-toolchain-linux-glibc-x86_64-v1.2.2")

# 验证工具链存在
if(NOT EXISTS "${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-gcc")
    message(FATAL_ERROR "SpacemiT toolchain not found at: ${SPACEMIT_TOOLCHAIN}")
endif()

# 设置编译器
set(CMAKE_C_COMPILER "${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-gcc")
set(CMAKE_CXX_COMPILER "${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-g++")
set(CMAKE_AR "${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-ar")
set(CMAKE_RANLIB "${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-ranlib")
set(CMAKE_STRIP "${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-strip")

# 设置 sysroot
set(CMAKE_SYSROOT "${SPACEMIT_TOOLCHAIN}/sysroot")
set(CMAKE_FIND_ROOT_PATH "${CMAKE_SYSROOT}")

# 搜索模式
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)

# RISC-V 编译标志
# rv64gcv: RV64 + G (IMAFD) + C (压缩指令) + V (向量扩展)
set(CMAKE_C_FLAGS_INIT "-march=rv64gcv -O3 -ffast-math")
set(CMAKE_CXX_FLAGS_INIT "-march=rv64gcv -O3 -ffast-math")

# 平台标识
add_compile_definitions(
    PLATFORM_RISCV64
    USE_SPACEMIT_NPU
    __riscv
    __riscv_xlen=64
)

# ONNX Runtime SpacemiT NPU 路径
set(SPACEMIT_ORT_ROOT "${PROJECT_ROOT}/src/rivision_node/yolo-server/third_party/spacemit-ort-current")
if(EXISTS "${SPACEMIT_ORT_ROOT}")
    set(ONNXRUNTIME_ROOT "${SPACEMIT_ORT_ROOT}")
    set(ONNXRUNTIME_INCLUDE_DIRS "${SPACEMIT_ORT_ROOT}/include")
    set(ONNXRUNTIME_LIBRARIES "${SPACEMIT_ORT_ROOT}/lib/libonnxruntime.so")
    message(STATUS "Found SpacemiT ONNX Runtime: ${SPACEMIT_ORT_ROOT}")
endif()

# llama.cpp 路径
set(LLAMA_CPP_ROOT "${PROJECT_ROOT}/src/rivision_node/third_party/llama.cpp/riscv64-current")

message(STATUS "=== RISC-V 64 Cross-Compilation ===")
message(STATUS "Toolchain: ${SPACEMIT_TOOLCHAIN}")
message(STATUS "Sysroot: ${CMAKE_SYSROOT}")
message(STATUS "C Compiler: ${CMAKE_C_COMPILER}")
message(STATUS "CXX Compiler: ${CMAKE_CXX_COMPILER}")

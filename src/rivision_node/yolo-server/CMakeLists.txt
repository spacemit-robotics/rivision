cmake_minimum_required(VERSION 3.14)
project(yolo-server VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Architecture-specific optimization flags
if(CMAKE_SYSTEM_PROCESSOR MATCHES "riscv64")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -march=rv64gcv -O3 -ffast-math")
    message(STATUS "Building for RISC-V with RVV optimization")
elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -march=armv8-a+simd+crypto -mtune=cortex-a78 -O3 -ffast-math")
    message(STATUS "Building for ARM64 with Cortex-A78 optimization (Jetson Orin)")
else()
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -O3 -ffast-math")
endif()

# ============================================================
# 按平台选择 third_party 中的 ONNX Runtime
# 重要: 只从 third_party 查找，禁止 fallback 到系统库
# ============================================================
set(SPACEMIT_ORT_DIR "" CACHE PATH "SpacemiT ONNX Runtime directory")

if(NOT SPACEMIT_ORT_DIR)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "riscv64")
        # RISC-V: 优先使用 spacemit-ort-current 符号链接
        set(_CURRENT_LINK "${CMAKE_SOURCE_DIR}/third_party/spacemit-ort-current")
        if(EXISTS "${_CURRENT_LINK}/lib")
            set(SPACEMIT_ORT_DIR "${_CURRENT_LINK}")
            message(STATUS "Using ORT symlink: spacemit-ort-current")
        else()
            # 回退: 匹配目录（排除 .tar.gz 文件）
            file(GLOB _ORT_CANDIDATES
                "${CMAKE_SOURCE_DIR}/third_party/spacemit-ort.riscv64*"
                "${CMAKE_SOURCE_DIR}/third_party/onnxruntime_spacemit*"
            )
            set(_ORT_DIRS "")
            foreach(_CANDIDATE ${_ORT_CANDIDATES})
                if(IS_DIRECTORY "${_CANDIDATE}")
                    list(APPEND _ORT_DIRS "${_CANDIDATE}")
                endif()
            endforeach()
            if(_ORT_DIRS)
                list(SORT _ORT_DIRS)
                list(GET _ORT_DIRS -1 SPACEMIT_ORT_DIR)
            endif()
        endif()
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64")
        # ARM64: 使用 ONNX Runtime ARM64 GPU（含 CUDA/TensorRT EP）
        file(GLOB _ORT_CANDIDATES
            "${CMAKE_SOURCE_DIR}/third_party/onnxruntime_arm64*"
            "${CMAKE_SOURCE_DIR}/third_party/onnxruntime-linux-aarch64*"
        )
        set(_ORT_DIRS "")
        foreach(_CANDIDATE ${_ORT_CANDIDATES})
            if(IS_DIRECTORY "${_CANDIDATE}")
                list(APPEND _ORT_DIRS "${_CANDIDATE}")
            endif()
        endforeach()
        if(_ORT_DIRS)
            list(SORT _ORT_DIRS)
            list(GET _ORT_DIRS -1 SPACEMIT_ORT_DIR)
        endif()
    else()
        # x86: 使用标准 ORT
        file(GLOB _ORT_CANDIDATES "${CMAKE_SOURCE_DIR}/third_party/onnxruntime_x86*")
        set(_ORT_DIRS "")
        foreach(_CANDIDATE ${_ORT_CANDIDATES})
            if(IS_DIRECTORY "${_CANDIDATE}")
                list(APPEND _ORT_DIRS "${_CANDIDATE}")
            endif()
        endforeach()
        if(_ORT_DIRS)
            list(SORT _ORT_DIRS)
            list(GET _ORT_DIRS -1 SPACEMIT_ORT_DIR)
        endif()
    endif()
    if(SPACEMIT_ORT_DIR)
        message(STATUS "Auto-detected ORT directory: ${SPACEMIT_ORT_DIR}")
    endif()
endif()

if(NOT SPACEMIT_ORT_DIR OR NOT EXISTS "${SPACEMIT_ORT_DIR}/lib")
    message(FATAL_ERROR
        "ONNX Runtime not found in third_party/!\n"
        "  Expected: third_party/spacemit-ort.riscv64.* (RISC-V) or third_party/onnxruntime_x86* (x86)\n"
        "  Or specify: -DSPACEMIT_ORT_DIR=/path/to/ort\n"
        "  DO NOT use system libraries — build must use bundled third_party.")
endif()

message(STATUS "Using ORT from: ${SPACEMIT_ORT_DIR}")

# 查找 ONNX Runtime 库 (仅从 third_party，NO_DEFAULT_PATH 禁止系统路径)
# 交叉编译时禁用 CMAKE_FIND_ROOT_PATH 限制
find_library(ONNXRUNTIME_LIB onnxruntime
    PATHS ${SPACEMIT_ORT_DIR}/lib
    NO_DEFAULT_PATH
    NO_CMAKE_FIND_ROOT_PATH
)

# 查找 ONNX Runtime 头文件 (仅从 third_party)
find_path(ONNXRUNTIME_INCLUDE_DIR onnxruntime_cxx_api.h
    PATHS ${SPACEMIT_ORT_DIR}/include
    NO_DEFAULT_PATH
    NO_CMAKE_FIND_ROOT_PATH
)

# 查找 SpacemiT EP 库（仅 K3 RISC-V）
find_library(SPACEMIT_EP_LIB spacemit_ep
    PATHS ${SPACEMIT_ORT_DIR}/lib
    NO_DEFAULT_PATH
    NO_CMAKE_FIND_ROOT_PATH
)

if(NOT ONNXRUNTIME_LIB)
    message(FATAL_ERROR "libonnxruntime.so not found in ${SPACEMIT_ORT_DIR}/lib/")
endif()
if(NOT ONNXRUNTIME_INCLUDE_DIR)
    message(FATAL_ERROR "onnxruntime headers not found in ${SPACEMIT_ORT_DIR}/include/")
endif()

set(USE_ONNX ON)
message(STATUS "Found ONNX Runtime: ${ONNXRUNTIME_LIB}")
message(STATUS "Found ONNX Runtime headers: ${ONNXRUNTIME_INCLUDE_DIR}")
if(SPACEMIT_EP_LIB)
    message(STATUS "Found SpacemiT EP: ${SPACEMIT_EP_LIB} (K3 hardware acceleration)")
endif()

# 源文件
set(SOURCES
    src/main.cpp
    src/yolo_detector.cpp
    src/yolo_pipeline.cpp
    src/http_server.cpp
    src/image_utils.cpp
)

# 可执行文件
add_executable(yolo-server ${SOURCES})

# 半静态链接：系统库静态，ORT 动态
# 减少部署依赖，只需要 ORT .so 文件即可运行
target_link_options(yolo-server PRIVATE
    -static-libgcc
    -static-libstdc++
)

# 嵌入 rpath：让二进制自动查找 ../lib/ 下的 ORT 动态库
set_target_properties(yolo-server PROPERTIES
    INSTALL_RPATH "$ORIGIN/../lib"
    BUILD_RPATH "$ORIGIN/../lib"
    INSTALL_RPATH_USE_LINK_PATH FALSE
)

target_include_directories(yolo-server PRIVATE
    ${CMAKE_SOURCE_DIR}/include
)

if(USE_ONNX)
    target_include_directories(yolo-server PRIVATE ${ONNXRUNTIME_INCLUDE_DIR})
    target_link_libraries(yolo-server PRIVATE ${ONNXRUNTIME_LIB})
    target_compile_definitions(yolo-server PRIVATE USE_ONNX=1)

    # CUDA EP 硬件加速（ARM64 Jetson Orin）
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64")
        find_library(ORT_PROVIDERS_CUDA onnxruntime_providers_cuda
            PATHS ${SPACEMIT_ORT_DIR}/lib
            NO_DEFAULT_PATH
            NO_CMAKE_FIND_ROOT_PATH
        )
        find_library(ORT_PROVIDERS_SHARED onnxruntime_providers_shared
            PATHS ${SPACEMIT_ORT_DIR}/lib
            NO_DEFAULT_PATH
            NO_CMAKE_FIND_ROOT_PATH
        )
        if(ORT_PROVIDERS_CUDA)
            target_compile_definitions(yolo-server PRIVATE USE_CUDA_EP=1)
            message(STATUS "Found CUDA provider: ${ORT_PROVIDERS_CUDA}")
            message(STATUS "  -> CUDA EP enabled for Jetson Orin GPU")
        endif()
        if(ORT_PROVIDERS_SHARED)
            target_link_libraries(yolo-server PRIVATE ${ORT_PROVIDERS_SHARED})
            message(STATUS "Found providers_shared: ${ORT_PROVIDERS_SHARED}")
        endif()
    endif()

    # SpacemiT EP 硬件加速（K3 RISC-V）
    if(SPACEMIT_EP_LIB)
        target_link_libraries(yolo-server PRIVATE ${SPACEMIT_EP_LIB})
        target_compile_definitions(yolo-server PRIVATE USE_SPACEMIT_EP=1)

        # providers_shared 是 SpacemiT EP 的必要依赖
        find_library(ORT_PROVIDERS_SHARED onnxruntime_providers_shared
            PATHS ${SPACEMIT_ORT_DIR}/lib
            NO_DEFAULT_PATH
        )
        if(ORT_PROVIDERS_SHARED)
            target_link_libraries(yolo-server PRIVATE ${ORT_PROVIDERS_SHARED})
            message(STATUS "Found providers_shared: ${ORT_PROVIDERS_SHARED}")
        endif()
    endif()
endif()

# 链接 pthread
find_package(Threads REQUIRED)
target_link_libraries(yolo-server PRIVATE Threads::Threads)

# 安装
install(TARGETS yolo-server DESTINATION bin)

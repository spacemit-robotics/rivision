cmake_minimum_required(VERSION 3.14)
project(yolo-server VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# RISC-V optimization flags (K3 target platform)
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -march=rv64gcv -O3 -ffast-math")
message(STATUS "Building for RISC-V with RVV optimization")

# ============================================================
# ONNX Runtime — 仅使用系统库 (/usr/lib, /usr/include)
# K3 上通过 apt 安装: sudo apt install libonnxruntime-dev spacemit-ort
# ============================================================
find_library(ONNXRUNTIME_LIB onnxruntime)
find_path(ONNXRUNTIME_INCLUDE_DIR onnxruntime_cxx_api.h)

if(NOT ONNXRUNTIME_LIB)
    message(FATAL_ERROR
        "libonnxruntime.so not found!\n"
        "  Install: sudo apt install libonnxruntime-dev spacemit-ort")
endif()
if(NOT ONNXRUNTIME_INCLUDE_DIR)
    message(FATAL_ERROR
        "onnxruntime headers not found!\n"
        "  Install: sudo apt install libonnxruntime-dev")
endif()

set(USE_ONNX ON)
message(STATUS "Found ONNX Runtime: ${ONNXRUNTIME_LIB}")
message(STATUS "Found ONNX Runtime headers: ${ONNXRUNTIME_INCLUDE_DIR}")

# SpacemiT EP 硬件加速（K3 RISC-V）
find_library(SPACEMIT_EP_LIB spacemit_ep)
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

target_include_directories(yolo-server PRIVATE
    ${CMAKE_SOURCE_DIR}/include
)

if(USE_ONNX)
    target_include_directories(yolo-server PRIVATE ${ONNXRUNTIME_INCLUDE_DIR})
    target_link_libraries(yolo-server PRIVATE ${ONNXRUNTIME_LIB})
    target_compile_definitions(yolo-server PRIVATE USE_ONNX=1)

    # SpacemiT EP 硬件加速（K3 RISC-V）
    if(SPACEMIT_EP_LIB)
        target_link_libraries(yolo-server PRIVATE ${SPACEMIT_EP_LIB})
        target_compile_definitions(yolo-server PRIVATE USE_SPACEMIT_EP=1)

        find_library(ORT_PROVIDERS_SHARED onnxruntime_providers_shared)
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

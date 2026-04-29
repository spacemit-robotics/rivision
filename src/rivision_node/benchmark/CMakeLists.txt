cmake_minimum_required(VERSION 3.14)
project(rivision_benchmark VERSION 2.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# 编译选项
add_compile_options(-Wall -Wextra -O3)

# ============================================================
# 平台检测与配置
# ============================================================
option(PLATFORM_X86 "Build for x86_64" OFF)
option(PLATFORM_RISCV64 "Build for RISC-V 64" OFF)
option(PLATFORM_ARM64_GPU "Build for ARM64 with GPU" OFF)
option(USE_SPACEMIT_NPU "Enable SpacemiT NPU support" OFF)
option(USE_CUDA "Enable CUDA support" OFF)
option(USE_TENSORRT "Enable TensorRT support" OFF)

# 架构检测
if(CMAKE_SYSTEM_PROCESSOR MATCHES "riscv64")
    message(STATUS "Building for RISC-V 64-bit (K3)")
    add_compile_definitions(RISCV64 PLATFORM_RISCV64)
    set(USE_SPACEMIT_NPU ON)
elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64")
    message(STATUS "Building for ARM64")
    add_compile_definitions(ARM64 PLATFORM_ARM64)
    if(USE_CUDA)
        add_compile_definitions(PLATFORM_ARM64_GPU USE_CUDA)
    endif()
elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "x86_64")
    message(STATUS "Building for x86_64")
    add_compile_definitions(X86_64 PLATFORM_X86_64)
endif()

# SpacemiT NPU 支持
if(USE_SPACEMIT_NPU)
    add_compile_definitions(USE_SPACEMIT_NPU)
    # 启用 SpacemiT EP (Execution Provider) 用于 A100 NPU
    add_compile_definitions(USE_SPACEMIT_EP)
    message(STATUS "SpacemiT NPU support enabled (USE_SPACEMIT_EP)")
endif()

# TensorRT 支持
if(USE_TENSORRT)
    add_compile_definitions(USE_TENSORRT)
    message(STATUS "TensorRT support enabled")
endif()

# ============================================================
# K3 硬件加速预处理支持
# ============================================================
# K3 硬件加速预处理方案
# 方案1: FFmpeg (推荐) - 软件 MJPEG 解码 + swscale 缩放
#        设置 FFMPEG_USE_HW=1 可启用 mjpeg_stcodec 硬件解码 (需要 video 组权限)
# 方案2: MPP (已废弃) - K1 JPU + V2D，K3 不完全支持
option(USE_K3_FFMPEG "Enable K3 FFmpeg JPEG decoding (recommended)" ON)
option(USE_K3_MPP "Enable K3 MPP hardware acceleration (deprecated, use FFmpeg instead)" OFF)

# K3 平台 MPP 配置 (仅当 USE_K3_MPP=ON 时才检测)
# 默认禁用 MPP，使用 FFmpeg 方案
if(USE_K3_MPP AND CMAKE_SYSTEM_PROCESSOR MATCHES "riscv64")
    # 查找 MPP 头文件路径
    set(K3_MPP_CANDIDATES
        "${CMAKE_CURRENT_SOURCE_DIR}/third_party/riscv64/mpp"  # ★ 优先: 本地 third_party/riscv64
        "/usr/local"                                            # 系统安装位置
        "/opt/spacemit/mpp"
    )
    foreach(MPP_PATH ${K3_MPP_CANDIDATES})
        if(EXISTS "${MPP_PATH}/include/vdec.h" AND EXISTS "${MPP_PATH}/include/g2d.h")
            set(K3_MPP_ROOT "${MPP_PATH}")
            message(STATUS "Found K3 MPP headers: ${K3_MPP_ROOT}/include")
            break()
        endif()
    endforeach()
    
    # 查找 MPP 库文件 (交叉编译时必须存在本地库)
    if(K3_MPP_ROOT)
        set(K3_MPP_LIB_CANDIDATES
            "${K3_MPP_ROOT}/lib"
            "${CMAKE_SYSROOT}/usr/lib"
            "${CMAKE_SYSROOT}/usr/local/lib"
        )
        # 本地编译时添加系统路径
        if(NOT CMAKE_CROSSCOMPILING)
            list(APPEND K3_MPP_LIB_CANDIDATES "/usr/local/lib" "/usr/lib")
        endif()
        
        foreach(LIB_PATH ${K3_MPP_LIB_CANDIDATES})
            if(EXISTS "${LIB_PATH}/libspacemit_mpp.so")
                set(K3_MPP_LIB_DIR "${LIB_PATH}")
                set(USE_K3_MPP ON)
                message(STATUS "Found K3 MPP library: ${K3_MPP_LIB_DIR}/libspacemit_mpp.so")
                break()
            endif()
        endforeach()
        
        if(NOT USE_K3_MPP)
            if(CMAKE_CROSSCOMPILING)
                message(STATUS "K3 MPP: headers found but library not available for cross-compile")
                message(STATUS "K3 MPP: disabling hardware acceleration (will use OpenCV)")
                message(STATUS "K3 MPP: to enable, copy libspacemit_mpp.so to third_party/riscv64/mpp/lib/")
            else()
                # 本地编译: 假设运行时库可用
                set(USE_K3_MPP ON)
                set(K3_MPP_LIB_DIR "/usr/lib")
                message(STATUS "K3 MPP: will link system library at runtime")
            endif()
        endif()
    endif()
endif()

if(USE_K3_MPP)
    add_compile_definitions(USE_K3_MPP)
    message(STATUS "K3 MPP hardware preprocess enabled")
else()
    message(STATUS "K3 MPP hardware preprocess disabled (using OpenCV fallback)")
endif()

# ============================================================
# K3 FFmpeg 硬件 JPEG 解码 (mjpeg_stcodec)
# ============================================================
# K3 平台推荐使用 FFmpeg 进行硬件 JPEG 解码
# FFmpeg 使用 mjpeg_stcodec 通过 VPU 硬件加速
if(CMAKE_SYSTEM_PROCESSOR MATCHES "riscv64" AND USE_K3_FFMPEG)
    set(K3_FFMPEG_DIR "${CMAKE_CURRENT_SOURCE_DIR}/third_party/riscv64/ffmpeg")
    
    # 检查 FFmpeg 头文件
    if(EXISTS "${K3_FFMPEG_DIR}/include/libavcodec/avcodec.h")
        set(FFMPEG_INCLUDE_DIR "${K3_FFMPEG_DIR}/include")
        message(STATUS "Found K3 FFmpeg headers: ${FFMPEG_INCLUDE_DIR}")
        
        # 检查 FFmpeg 库文件
        if(EXISTS "${K3_FFMPEG_DIR}/lib/libavcodec.so")
            set(FFMPEG_LIB_DIR "${K3_FFMPEG_DIR}/lib")
            set(K3_FFMPEG_FOUND TRUE)
            add_compile_definitions(USE_K3_FFMPEG)
            message(STATUS "Found K3 FFmpeg libraries: ${FFMPEG_LIB_DIR}")
        else()
            message(STATUS "K3 FFmpeg: headers found but libraries missing")
            message(STATUS "K3 FFmpeg: copy libraries from K3:")
            message(STATUS "  scp node@k3:/usr/lib/riscv64-linux-gnu/libav*.so* ${K3_FFMPEG_DIR}/lib/")
            message(STATUS "  scp node@k3:/usr/lib/riscv64-linux-gnu/libsw*.so* ${K3_FFMPEG_DIR}/lib/")
        endif()
    else()
        message(STATUS "K3 FFmpeg: headers not found, hardware JPEG decode disabled")
        message(STATUS "K3 FFmpeg: copy headers from K3:")
        message(STATUS "  scp -r node@k3:/usr/include/libav* ${K3_FFMPEG_DIR}/include/")
        message(STATUS "  scp -r node@k3:/usr/include/libsw* ${K3_FFMPEG_DIR}/include/")
    endif()
endif()

if(K3_FFMPEG_FOUND)
    message(STATUS "K3 FFmpeg hardware JPEG decode enabled (mjpeg_stcodec)")
endif()

# ============================================================
# libyuv - 高性能 YUV 处理库 (RVV 优化)
# ============================================================
if(CMAKE_SYSTEM_PROCESSOR MATCHES "riscv64")
    set(LIBYUV_DIR "${CMAKE_CURRENT_SOURCE_DIR}/third_party/riscv64/libyuv")
    
    if(EXISTS "${LIBYUV_DIR}/include/libyuv.h")
        set(LIBYUV_INCLUDE_DIR "${LIBYUV_DIR}/include")
        
        # 优先使用静态库
        if(EXISTS "${LIBYUV_DIR}/lib/libyuv.a")
            set(LIBYUV_LIBRARY "${LIBYUV_DIR}/lib/libyuv.a")
            set(LIBYUV_FOUND TRUE)
            message(STATUS "Found libyuv (static): ${LIBYUV_LIBRARY}")
        elseif(EXISTS "${LIBYUV_DIR}/lib/libyuv.so")
            set(LIBYUV_LIBRARY "${LIBYUV_DIR}/lib/libyuv.so")
            set(LIBYUV_LIB_DIR "${LIBYUV_DIR}/lib")
            set(LIBYUV_FOUND TRUE)
            message(STATUS "Found libyuv (shared): ${LIBYUV_LIBRARY}")
        endif()
        
        if(LIBYUV_FOUND)
            # 禁用 libyuv: I420Scale/I420ToRGB24 无 RVV 优化，性能与 swscale 持平
            # 保留 swscale 作为默认方案
            # add_compile_definitions(USE_LIBYUV)
            message(STATUS "libyuv found but disabled (no RVV for I420Scale), using swscale")
        endif()
    else()
        message(STATUS "libyuv not found. To enable RVV-optimized YUV processing:")
        message(STATUS "  Run: ./scripts/build-libyuv-riscv64.sh")
    endif()
endif()

# ============================================================
# 依赖配置 - 支持交叉编译和本地编译
# ============================================================
#
# 第三方库目录结构:
#   third_party/riscv64/
#     ├── opencv/        # OpenCV for K3 (SpacemiT build)
#     │   ├── include/opencv4/
#     │   └── lib/
#     └── curl/          # CURL headers + static binary
#         ├── include/curl/
#         └── bin/curl-static
#
# K3 运行时要求:
#   - OpenCV: 使用打包的 libs 或系统 libopencv (apt install libopencv-dev)
#   - CURL: 使用系统 libcurl (apt install libcurl4)
# ============================================================

set(THIRD_PARTY_DIR "${CMAKE_CURRENT_SOURCE_DIR}/third_party")

if(CMAKE_CROSSCOMPILING AND CMAKE_SYSTEM_PROCESSOR STREQUAL "riscv64")
    message(STATUS "Cross-compilation: K3 RISC-V full build mode")
    
    # OpenCV: 使用预编译的 SpacemiT OpenCV
    set(RISCV64_OPENCV_DIR "${THIRD_PARTY_DIR}/riscv64/opencv")
    if(EXISTS "${RISCV64_OPENCV_DIR}/include/opencv4")
        set(OpenCV_FOUND TRUE)
        set(OpenCV_INCLUDE_DIRS "${RISCV64_OPENCV_DIR}/include/opencv4")
        set(OpenCV_LIBS 
            "${RISCV64_OPENCV_DIR}/lib/libopencv_core.so"
            "${RISCV64_OPENCV_DIR}/lib/libopencv_imgproc.so"
            "${RISCV64_OPENCV_DIR}/lib/libopencv_imgcodecs.so"
            "${RISCV64_OPENCV_DIR}/lib/libopencv_videoio.so"
            "${RISCV64_OPENCV_DIR}/lib/libopencv_dnn.so"
        )
        set(OpenCV_LIB_DIR "${RISCV64_OPENCV_DIR}/lib")
        message(STATUS "OpenCV: Using SpacemiT build at ${RISCV64_OPENCV_DIR}")
    else()
        message(FATAL_ERROR "OpenCV for RISC-V not found at ${RISCV64_OPENCV_DIR}")
    endif()
    
    # CURL: 使用头文件，运行时链接系统 libcurl
    set(RISCV64_CURL_DIR "${THIRD_PARTY_DIR}/riscv64/curl")
    if(EXISTS "${RISCV64_CURL_DIR}/include/curl/curl.h")
        set(CURL_FOUND TRUE)
        set(CURL_INCLUDE_DIRS "${RISCV64_CURL_DIR}/include")
        # 检查是否有预编译的 libcurl
        if(EXISTS "${RISCV64_CURL_DIR}/lib/libcurl.so")
            set(CURL_LIBRARIES "${RISCV64_CURL_DIR}/lib/libcurl.so")
            set(CURL_LIB_DIR "${RISCV64_CURL_DIR}/lib")
            message(STATUS "CURL: Using prebuilt library at ${RISCV64_CURL_DIR}/lib")
        else()
            # 没有预编译库，禁用 HTTP 后端
            message(WARNING "CURL library not found at ${RISCV64_CURL_DIR}/lib")
            message(WARNING "To enable HTTP backends, copy libcurl.so from K3:")
            message(WARNING "  scp user@k3:/usr/lib/riscv64-linux-gnu/libcurl.so* ${RISCV64_CURL_DIR}/lib/")
            set(NO_HTTP_BACKENDS ON)
        endif()
    else()
        message(WARNING "CURL headers not found, HTTP backends disabled")
        set(NO_HTTP_BACKENDS ON)
    endif()
    
elseif(CMAKE_CROSSCOMPILING)
    # 其他交叉编译目标 (ARM64 等)
    message(STATUS "Cross-compilation: Generic mode")
    find_package(OpenCV QUIET)
    find_package(CURL QUIET)
    if(NOT OpenCV_FOUND)
        set(HTTP_ONLY_MODE ON)
    endif()
    if(NOT CURL_FOUND)
        set(NO_HTTP_BACKENDS ON)
    endif()
else()
    # 本地编译: 标准查找
    find_package(OpenCV REQUIRED)
    find_package(CURL REQUIRED)
endif()

if(HTTP_ONLY_MODE)
    message(STATUS "HTTP_ONLY_MODE enabled")
    add_compile_definitions(HTTP_ONLY_MODE)
endif()

# ONNX Runtime (可选)
set(ONNXRUNTIME_ROOT "" CACHE PATH "ONNX Runtime root directory")

# 为 RISC-V 交叉编译查找 SpacemiT ORT
if(CMAKE_CROSSCOMPILING AND CMAKE_SYSTEM_PROCESSOR STREQUAL "riscv64")
    # 查找 SpacemiT ORT 的多个可能位置
    # 优先使用 spacemit-ort-current 符号链接，便于版本切换
    set(SPACEMIT_ORT_CANDIDATES
        "${CMAKE_CURRENT_SOURCE_DIR}/../yolo-server/third_party/spacemit-ort-current"
        "${CMAKE_CURRENT_SOURCE_DIR}/../yolo-server/third_party/spacemit-ort.riscv64.2.0.2+rc3"
        "${CMAKE_CURRENT_SOURCE_DIR}/../third_party/spacemit-ort.riscv64"
        "${THIRD_PARTY_DIR}/riscv64/onnxruntime"
    )
    foreach(ORT_PATH ${SPACEMIT_ORT_CANDIDATES})
        if(EXISTS "${ORT_PATH}/include" AND EXISTS "${ORT_PATH}/lib")
            # 解析符号链接获取真实路径
            get_filename_component(ONNXRUNTIME_ROOT "${ORT_PATH}" REALPATH)
            message(STATUS "Found SpacemiT ORT for RISC-V: ${ONNXRUNTIME_ROOT}")
            break()
        endif()
    endforeach()
endif()

# 默认路径
if(NOT ONNXRUNTIME_ROOT)
    set(ONNXRUNTIME_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../third_party/onnxruntime")
endif()

if(EXISTS "${ONNXRUNTIME_ROOT}/include")
    set(ONNXRUNTIME_INCLUDE_DIRS "${ONNXRUNTIME_ROOT}/include")
    if(EXISTS "${ONNXRUNTIME_ROOT}/lib/libonnxruntime.so")
        set(ONNXRUNTIME_LIBRARIES "${ONNXRUNTIME_ROOT}/lib/libonnxruntime.so")
        set(ONNXRUNTIME_LIB_DIR "${ONNXRUNTIME_ROOT}/lib")
    elseif(EXISTS "${ONNXRUNTIME_ROOT}/lib64/libonnxruntime.so")
        set(ONNXRUNTIME_LIBRARIES "${ONNXRUNTIME_ROOT}/lib64/libonnxruntime.so")
        set(ONNXRUNTIME_LIB_DIR "${ONNXRUNTIME_ROOT}/lib64")
    endif()
    set(ONNXRUNTIME_FOUND TRUE)
    message(STATUS "Found ONNX Runtime: ${ONNXRUNTIME_ROOT}")
    
    # 查找 SpacemiT EP 库 (K3 A100 NPU)
    if(USE_SPACEMIT_NPU AND EXISTS "${ONNXRUNTIME_LIB_DIR}/libspacemit_ep.so")
        set(SPACEMIT_EP_LIB "${ONNXRUNTIME_LIB_DIR}/libspacemit_ep.so")
        set(SPACEMIT_EP_PROVIDERS "${ONNXRUNTIME_LIB_DIR}/libonnxruntime_providers_shared.so")
        message(STATUS "Found SpacemiT EP: ${SPACEMIT_EP_LIB}")
    endif()
else()
    set(ONNXRUNTIME_FOUND FALSE)
    message(WARNING "ONNX Runtime not found, local YOLO backend will be disabled")
endif()

# 头文件目录
include_directories(
    ${CMAKE_CURRENT_SOURCE_DIR}/include
    ${CURL_INCLUDE_DIRS}
)

if(OpenCV_FOUND)
    include_directories(${OpenCV_INCLUDE_DIRS})
endif()

if(ONNXRUNTIME_FOUND)
    include_directories(${ONNXRUNTIME_INCLUDE_DIRS})
endif()

# K3 MPP 头文件和库
if(USE_K3_MPP AND K3_MPP_ROOT)
    include_directories(${K3_MPP_ROOT}/include)
    if(K3_MPP_LIB_DIR)
        link_directories(${K3_MPP_LIB_DIR})
    endif()
endif()

# K3 FFmpeg 头文件和库
if(K3_FFMPEG_FOUND)
    include_directories(${FFMPEG_INCLUDE_DIR})
    link_directories(${FFMPEG_LIB_DIR})
endif()

# libyuv 头文件
if(LIBYUV_FOUND)
    include_directories(${LIBYUV_INCLUDE_DIR})
    if(LIBYUV_LIB_DIR)
        link_directories(${LIBYUV_LIB_DIR})
    endif()
endif()

# ============================================================
# 源文件配置
# ============================================================

# 核心模块 (始终编译)
set(BENCHMARK_SOURCES
    src/platform/platform_detector.cpp
    src/platform/backend_factory.cpp
    src/metrics_collector.cpp
    src/benchmark_runner.cpp
    src/inference_backend/inference_backend_base.cpp
)

# HTTP 后端
if(NOT NO_HTTP_BACKENDS AND CURL_FOUND)
    list(APPEND BENCHMARK_SOURCES
        src/inference_backend/http_yolo_backend.cpp
        src/inference_backend/http_vlm_backend.cpp
    )
    message(STATUS "HTTP backends: Enabled")
else()
    message(STATUS "HTTP backends: Disabled (no CURL)")
    add_compile_definitions(NO_HTTP_BACKENDS)
endif()

# 桩实现 (用于禁用的后端)
list(APPEND BENCHMARK_SOURCES src/stubs/cross_compile_stubs.cpp)

# 本地后端需要 OpenCV
if(NOT HTTP_ONLY_MODE AND OpenCV_FOUND)
    list(APPEND BENCHMARK_SOURCES
        # 数据源
        src/data_source/video_file_source.cpp
        src/data_source/video_stream_source.cpp
        src/data_source/image_folder_source.cpp
        src/data_source/rtsp_source.cpp
        # 本地推理后端
        src/inference_backend/local_vlm_backend.cpp
    )
    
    # 本地 YOLO 后端 (需要 ONNX Runtime)
    if(ONNXRUNTIME_FOUND)
        list(APPEND BENCHMARK_SOURCES src/inference_backend/local_yolo_backend.cpp)
    else()
        add_compile_definitions(NO_ONNXRUNTIME)
    endif()
else()
    message(STATUS "Local backends disabled (HTTP_ONLY_MODE or no OpenCV)")
    add_compile_definitions(NO_ONNXRUNTIME NO_LOCAL_BACKENDS)
endif()

# 平台特定源文件
if(USE_SPACEMIT_NPU)
    message(STATUS "Building with SpacemiT NPU support")
endif()

# K3 MPP 硬件加速预处理
if(USE_K3_MPP)
    list(APPEND BENCHMARK_SOURCES src/preprocessing/k3_mpp_preprocessor.cpp)
    message(STATUS "K3 MPP preprocessor source added")
endif()

# K3 FFmpeg 硬件 JPEG 解码预处理
if(K3_FFMPEG_FOUND)
    list(APPEND BENCHMARK_SOURCES src/preprocessing/ffmpeg_preprocessor.cpp)
    message(STATUS "K3 FFmpeg preprocessor source added")
endif()

# TensorRT 后端 (Jetson Orin)
if(USE_TENSORRT AND NOT HTTP_ONLY_MODE)
    find_package(CUDA REQUIRED)
    find_library(NVINFER_LIB nvinfer)
    find_library(NVONNXPARSER_LIB nvonnxparser)
    
    if(NVINFER_LIB AND NVONNXPARSER_LIB)
        list(APPEND BENCHMARK_SOURCES src/inference_backend/yolo_tensorrt.cpp)
        include_directories(${CUDA_INCLUDE_DIRS})
        message(STATUS "TensorRT support enabled")
    else()
        message(WARNING "TensorRT libraries not found, disabling TensorRT backend")
        set(USE_TENSORRT OFF)
    endif()
endif()

# 库
add_library(benchmark_lib STATIC ${BENCHMARK_SOURCES})

# 交叉编译时允许共享库的未定义符号（运行时由 K3 系统解析）
if(CMAKE_CROSSCOMPILING)
    target_link_options(benchmark_lib PUBLIC "-Wl,--allow-shlib-undefined")
endif()

target_link_libraries(benchmark_lib pthread)

if(NOT NO_HTTP_BACKENDS AND CURL_FOUND)
    target_link_libraries(benchmark_lib ${CURL_LIBRARIES})
endif()

if(OpenCV_FOUND AND NOT HTTP_ONLY_MODE)
    target_link_libraries(benchmark_lib ${OpenCV_LIBS})
endif()

if(ONNXRUNTIME_FOUND)
    target_link_libraries(benchmark_lib ${ONNXRUNTIME_LIBRARIES})
    # SpacemiT EP (K3 A100 NPU)
    if(SPACEMIT_EP_LIB)
        target_link_libraries(benchmark_lib ${SPACEMIT_EP_LIB} ${SPACEMIT_EP_PROVIDERS})
    endif()
endif()

# K3 FFmpeg 库链接
if(K3_FFMPEG_FOUND)
    target_link_libraries(benchmark_lib
        "${FFMPEG_LIB_DIR}/libavcodec.so"
        "${FFMPEG_LIB_DIR}/libavformat.so"
        "${FFMPEG_LIB_DIR}/libavutil.so"
        "${FFMPEG_LIB_DIR}/libswscale.so"
    )
    message(STATUS "K3 FFmpeg: linking libraries from ${FFMPEG_LIB_DIR}")
endif()

# libyuv 库链接 (RVV 优化的 YUV 处理)
if(LIBYUV_FOUND)
    target_link_libraries(benchmark_lib ${LIBYUV_LIBRARY})
    message(STATUS "libyuv: linking ${LIBYUV_LIBRARY}")
endif()

# K3 MPP 库链接
if(USE_K3_MPP)
    # MPP 库名: libspacemit_mpp.so
    set(MPP_LIB_FOUND FALSE)
    
    # 检查本地库 (third_party/riscv64/mpp/lib)
    if(K3_MPP_LIB_DIR AND EXISTS "${K3_MPP_LIB_DIR}/libspacemit_mpp.so")
        target_link_libraries(benchmark_lib "${K3_MPP_LIB_DIR}/libspacemit_mpp.so")
        set(MPP_LIB_FOUND TRUE)
        message(STATUS "K3 MPP: linking ${K3_MPP_LIB_DIR}/libspacemit_mpp.so")
    endif()
    
    # 检查系统库路径 (用于交叉编译时的 sysroot)
    if(NOT MPP_LIB_FOUND)
        foreach(SYS_LIB_PATH "/usr/lib" "/usr/local/lib" "${CMAKE_SYSROOT}/usr/lib")
            if(EXISTS "${SYS_LIB_PATH}/libspacemit_mpp.so")
                target_link_libraries(benchmark_lib "${SYS_LIB_PATH}/libspacemit_mpp.so")
                set(MPP_LIB_FOUND TRUE)
                message(STATUS "K3 MPP: linking ${SYS_LIB_PATH}/libspacemit_mpp.so")
                break()
            endif()
        endforeach()
    endif()
    
    # 交叉编译且无本地库: 禁用 MPP 编译，运行时回退到 OpenCV
    if(NOT MPP_LIB_FOUND)
        if(CMAKE_CROSSCOMPILING)
            message(STATUS "K3 MPP: library not found for cross-compile, disabling MPP (will use OpenCV)")
            remove_definitions(-DUSE_K3_MPP)
            set(USE_K3_MPP OFF)
        else()
            # 本地编译: 尝试链接系统库
            target_link_libraries(benchmark_lib spacemit_mpp)
            message(STATUS "K3 MPP: linking system libspacemit_mpp.so")
        endif()
    endif()
endif()

# TensorRT 链接
if(USE_TENSORRT AND NVINFER_LIB AND NVONNXPARSER_LIB)
    target_link_libraries(benchmark_lib
        ${NVINFER_LIB}
        ${NVONNXPARSER_LIB}
        ${CUDA_LIBRARIES}
        ${CUDA_CUDART_LIBRARY}
    )
endif()

# 可执行文件
add_executable(rivision-benchmark src/main.cpp)
target_link_libraries(rivision-benchmark benchmark_lib)

# ★ 模型检查工具
if(ONNXRUNTIME_FOUND)
    add_executable(check-model-output tools/check_model_output.cpp)
    target_link_libraries(check-model-output ${ONNXRUNTIME_LIBRARIES})
    target_include_directories(check-model-output PRIVATE ${ONNXRUNTIME_INCLUDE_DIRS})
endif()

# ★ RPATH 配置: 确保运行时能找到 ./lib/ 中的库
# $ORIGIN 表示可执行文件所在目录
set_target_properties(rivision-benchmark PROPERTIES
    BUILD_RPATH_USE_ORIGIN TRUE
    INSTALL_RPATH "$ORIGIN/../lib:$ORIGIN/lib"
    BUILD_RPATH "$ORIGIN/../lib:$ORIGIN/lib"
)

if(ONNXRUNTIME_FOUND)
    set_target_properties(check-model-output PROPERTIES
        BUILD_RPATH_USE_ORIGIN TRUE
        INSTALL_RPATH "$ORIGIN/../lib:$ORIGIN/lib"
        BUILD_RPATH "$ORIGIN/../lib:$ORIGIN/lib"
    )
    install(TARGETS check-model-output RUNTIME DESTINATION bin)
endif()

# 安装
install(TARGETS rivision-benchmark RUNTIME DESTINATION bin)
install(DIRECTORY include/ DESTINATION include/rivision/benchmark)

# 输出信息
message(STATUS "")
message(STATUS "RiVision Benchmark Configuration:")
message(STATUS "  OpenCV:        ${OpenCV_VERSION}")
message(STATUS "  CURL:          ${CURL_VERSION_STRING}")
message(STATUS "  ONNX Runtime:  ${ONNXRUNTIME_FOUND}")
message(STATUS "  Architecture:  ${CMAKE_SYSTEM_PROCESSOR}")
message(STATUS "")

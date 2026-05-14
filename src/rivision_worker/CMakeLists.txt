cmake_minimum_required(VERSION 3.16)
project(rivision_worker VERSION 1.0.0 LANGUAGES CXX C)

# =============================================================================
# RiVision Worker
# =============================================================================

# C++ Standard
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# =============================================================================
# 平台守卫：仅支持 SpacemiT K3 (riscv64) 本机编译
# =============================================================================
if(NOT CMAKE_SYSTEM_PROCESSOR STREQUAL "riscv64")
    message(FATAL_ERROR
        "\n  RiVision Worker is K3-only (SpacemiT riscv64).\n"
        "  Current CMAKE_SYSTEM_PROCESSOR = '${CMAKE_SYSTEM_PROCESSOR}'\n"
        "  Build on a K3 device (Bianbu OS / Ubuntu riscv64)\n"
    )
endif()

# =============================================================================
# Build Options (功能开关，非平台开关)
# =============================================================================
option(BUILD_VLM "Build VLM inference service" ON)
option(USE_VLM_CLIENT "Enable VLM HTTP client for AI verification" ON)
option(BUILD_TESTS "Build unit tests" OFF)
option(BUILD_BENCHMARKS "Build benchmarks" OFF)
option(USE_OATPP "Build with oatpp HTTP server" ON)

# =============================================================================
# 自检报告收集（configure 末尾统一汇报缺失项 + apt 安装命令）
# =============================================================================
set(_RV_MISSING_APT "" CACHE INTERNAL "")
set(_RV_MISSING_OTHER "" CACHE INTERNAL "")

function(rv_mark_missing_apt pkg)
    list(APPEND _RV_MISSING_APT "${pkg}")
    list(REMOVE_DUPLICATES _RV_MISSING_APT)
    set(_RV_MISSING_APT "${_RV_MISSING_APT}" CACHE INTERNAL "" FORCE)
endfunction()

function(rv_mark_missing_other msg)
    list(APPEND _RV_MISSING_OTHER "${msg}")
    set(_RV_MISSING_OTHER "${_RV_MISSING_OTHER}" CACHE INTERNAL "" FORCE)
endfunction()

# Helper：多路径查找一个 CMake 包，返回首个匹配的 *Config.cmake 目录
function(rv_find_cmake_config var_dir config_name)
    if(${var_dir})
        return()  # 用户已通过 -D${var_dir}=... 指定
    endif()
    foreach(_d ${ARGN})
        if(EXISTS "${_d}/${config_name}")
            set(${var_dir} "${_d}" CACHE PATH "Detected ${config_name} dir" FORCE)
            return()
        endif()
    endforeach()
endfunction()

# Helper：检查 apt 包是否安装（仅 Debian/Ubuntu 系），不强制 FATAL
function(rv_check_apt_pkg pkg out_var)
    execute_process(
        COMMAND dpkg-query -W -f=$\{Status\} ${pkg}
        OUTPUT_VARIABLE _dpkg_status
        ERROR_QUIET
        RESULT_VARIABLE _dpkg_rc
    )
    if(_dpkg_rc EQUAL 0 AND _dpkg_status MATCHES "install ok installed")
        set(${out_var} TRUE PARENT_SCOPE)
    else()
        set(${out_var} FALSE PARENT_SCOPE)
    endif()
endfunction()

# =============================================================================
# Include paths
# =============================================================================
include_directories(${CMAKE_CURRENT_SOURCE_DIR}/include)
include_directories(${CMAKE_CURRENT_SOURCE_DIR}/src)
list(APPEND CMAKE_MODULE_PATH ${CMAKE_CURRENT_SOURCE_DIR}/cmake)

# =============================================================================
# 标准开发依赖（apt 系统包）
# =============================================================================

# spdlog (optional)
find_package(spdlog QUIET)
if(spdlog_FOUND)
    add_definitions(-DUSE_SPDLOG)
    message(STATUS "spdlog: system")
else()
    message(STATUS "spdlog: not found (using built-in logger)")
endif()

# nlohmann_json (header-only) — system → FetchContent fallback
find_package(nlohmann_json QUIET)
if(NOT nlohmann_json_FOUND)
    message(STATUS "nlohmann_json: system not found, will FetchContent (apt: sudo apt install nlohmann-json3-dev to avoid network)")
    include(FetchContent)
    FetchContent_Declare(
        json
        URL https://github.com/nlohmann/json/releases/download/v3.11.3/json.tar.xz
        DOWNLOAD_EXTRACT_TIMESTAMP TRUE
    )
    FetchContent_MakeAvailable(json)
    include_directories(${json_SOURCE_DIR}/include)
else()
    message(STATUS "nlohmann_json: system")
endif()

# yaml-cpp (REQUIRED)
find_package(yaml-cpp QUIET)
if(NOT yaml-cpp_FOUND AND NOT TARGET yaml-cpp)
    rv_mark_missing_apt("libyaml-cpp-dev")
    message(STATUS "yaml-cpp: ❌ NOT FOUND — will add libyaml-cpp-dev to apt list")
else()
    message(STATUS "yaml-cpp: system")
endif()

# SQLite3 (REQUIRED)
find_package(SQLite3 QUIET)
if(NOT SQLite3_FOUND)
    rv_mark_missing_apt("libsqlite3-dev")
    message(STATUS "SQLite3: ❌ NOT FOUND — will add libsqlite3-dev to apt list")
else()
    message(STATUS "SQLite3: ${SQLite3_VERSION}")
endif()

# =============================================================================
# OpenCV — 多路径自适应探测
#   优先级:
#     1) -DOpenCV_DIR=... 显式覆盖
#     2) /opt/opencv-spacemit (SpacemiT 定制版，K3 NPU 优化)
#     3) /usr/local 自编安装
#     4) 系统 apt 默认位置 (/usr/lib/riscv64-linux-gnu/cmake/opencv4)
# =============================================================================
rv_find_cmake_config(OpenCV_DIR "OpenCVConfig.cmake"
    "/opt/opencv-spacemit/lib/cmake/opencv4"
    "/usr/local/lib/cmake/opencv4"
    "/usr/lib/riscv64-linux-gnu/cmake/opencv4"
    "/usr/lib/cmake/opencv4"
)
find_package(OpenCV QUIET COMPONENTS core imgproc imgcodecs dnn)
if(NOT OpenCV_FOUND)
    rv_mark_missing_other("OpenCV not found. Either install SpacemiT OpenCV to /opt/opencv-spacemit/ or run: sudo apt install -y libopencv-dev")
    message(STATUS "OpenCV: ❌ NOT FOUND in any standard location")
else()
    message(STATUS "OpenCV: ${OpenCV_VERSION} from ${OpenCV_DIR}")
endif()

# =============================================================================
# FFmpeg dev headers (libavformat / libavcodec / libavutil / libswresample / libswscale)
# =============================================================================
find_package(PkgConfig QUIET)
if(PkgConfig_FOUND)
    pkg_check_modules(FFMPEG QUIET libavformat libavcodec libavutil libswresample libswscale)
endif()
if(NOT FFMPEG_FOUND)
    rv_mark_missing_apt("libavformat-dev")
    rv_mark_missing_apt("libavcodec-dev")
    rv_mark_missing_apt("libavutil-dev")
    rv_mark_missing_apt("libswresample-dev")
    rv_mark_missing_apt("libswscale-dev")
    message(STATUS "FFmpeg dev: ❌ NOT FOUND — will add libav*-dev to apt list")
else()
    message(STATUS "FFmpeg dev: ${FFMPEG_VERSION}")
endif()

# =============================================================================
# libcurl (VLM client HTTP)
# =============================================================================
find_package(CURL QUIET)
if(NOT CURL_FOUND AND USE_VLM_CLIENT)
    rv_mark_missing_apt("libcurl4-openssl-dev")
    message(STATUS "libcurl: ❌ NOT FOUND — will add libcurl4-openssl-dev to apt list")
elseif(CURL_FOUND)
    message(STATUS "libcurl: ${CURL_VERSION_STRING}")
endif()

# =============================================================================
# oatpp HTTP/WebSocket server framework
# =============================================================================
set(OATPP_SOURCE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/third_party/oatpp")
set(OATPP_WS_SOURCE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/third_party/oatpp-websocket")

if(USE_OATPP)
    find_package(oatpp QUIET)
    find_package(oatpp-websocket QUIET)

    if(oatpp_FOUND AND oatpp-websocket_FOUND)
        add_definitions(-DUSE_OATPP)
        message(STATUS "oatpp: system")
    elseif(EXISTS "${OATPP_SOURCE_DIR}/CMakeLists.txt" AND EXISTS "${OATPP_WS_SOURCE_DIR}/CMakeLists.txt")
        message(STATUS "oatpp: building from third_party source")
        # oatpp 0.x 的 CMakeLists 声明 cmake_minimum_required(VERSION 3.1)，
        # CMake ≥ 4.0 拒绝 < 3.5 的兼容性，注入兼容策略
        set(CMAKE_POLICY_VERSION_MINIMUM 3.5 CACHE STRING "" FORCE)
        set(OATPP_BUILD_TESTS OFF CACHE BOOL "" FORCE)
        set(OATPP_INSTALL OFF CACHE BOOL "" FORCE)
        add_subdirectory(${OATPP_SOURCE_DIR} ${CMAKE_BINARY_DIR}/oatpp)

        set(OATPP_MODULES_LOCATION "CUSTOM" CACHE STRING "" FORCE)
        set(OATPP_DIR_SRC "${OATPP_SOURCE_DIR}" CACHE PATH "" FORCE)
        set(oatpp_INCLUDE "${OATPP_SOURCE_DIR}/src" CACHE PATH "" FORCE)
        set(OATPP_WEBSOCKET_BUILD_TESTS OFF CACHE BOOL "" FORCE)
        set(OATPP_WEBSOCKET_INSTALL OFF CACHE BOOL "" FORCE)
        add_subdirectory(${OATPP_WS_SOURCE_DIR} ${CMAKE_BINARY_DIR}/oatpp-websocket)

        add_definitions(-DUSE_OATPP)
        set(oatpp_FOUND TRUE)
        set(oatpp-websocket_FOUND TRUE)
    else()
        message(WARNING "oatpp not found — HTTP API will use stub fallback")
        set(USE_OATPP OFF)
    endif()
endif()

# =============================================================================
# ONNX Runtime + SpacemiT EP — 固定使用 third_party/spacemit-ort/（bundled）
# =============================================================================
set(SPACEMIT_ORT_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/third_party/spacemit-ort")

if(NOT EXISTS "${SPACEMIT_ORT_ROOT}/include/onnxruntime_cxx_api.h")
    rv_mark_missing_other(
        "SpacemiT ORT SDK not found at third_party/spacemit-ort/. "
        "Unpack spacemit-ort.riscv64.*.tar.gz to that directory.")
else()
    set(ONNXRUNTIME_INCLUDE_DIR "${SPACEMIT_ORT_ROOT}/include")
    set(ONNXRUNTIME_LIB_DIR     "${SPACEMIT_ORT_ROOT}/lib")
    set(ONNXRUNTIME_FOUND TRUE)
    message(STATUS "ONNX Runtime: ${SPACEMIT_ORT_ROOT}/lib (bundled)")
endif()

if(ONNXRUNTIME_FOUND AND NOT EXISTS "${SPACEMIT_ORT_ROOT}/lib/libspacemit_ep.so")
    rv_mark_missing_other("libspacemit_ep.so missing from third_party/spacemit-ort/lib/")
else()
    set(SPACEMIT_EP_FOUND TRUE)
    message(STATUS "SpacemiT EP:  ${SPACEMIT_ORT_ROOT}/lib/libspacemit_ep.so (bundled)")
endif()

# =============================================================================
# llama.cpp — 固定使用 third_party/spacemit-llama.cpp/（bundled）
# =============================================================================
if(BUILD_VLM)
    set(SPACEMIT_LLAMA_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/third_party/spacemit-llama.cpp")
    if(NOT EXISTS "${SPACEMIT_LLAMA_ROOT}/include/llama.h")
        rv_mark_missing_other(
            "SpacemiT llama.cpp SDK not found at third_party/spacemit-llama.cpp/. "
            "Unpack spacemit-llama.cpp.riscv64.*.tar.gz to that directory.")
    else()
        set(LLAMA_INCLUDE_DIR "${SPACEMIT_LLAMA_ROOT}/include")
        set(LLAMA_LIB_DIR     "${SPACEMIT_LLAMA_ROOT}/lib")
        set(LLAMA_FOUND TRUE)
        message(STATUS "llama.cpp:    ${SPACEMIT_LLAMA_ROOT}/lib (bundled)")
    endif()
endif()

# =============================================================================
# SpacemiT MPP (always ON for K3 — required platform backend, bundled)
# =============================================================================
include(FindSpacemiTMPP)

if(NOT SpacemiTMPP_FOUND)
    rv_mark_missing_other(
        "SpacemiT MPP SDK not found at third_party/mpp/. Layout required: third_party/mpp/include/sys/sys_api.h + third_party/mpp/lib/libmpp_*.a — obtain SDK from SpacemiT and unpack to third_party/mpp/")
endif()
add_definitions(-DUSE_SPACEMIT_MPP)

# =============================================================================
# 统一报告：configure 末尾汇报所有缺失项 + 一键 apt 命令
# =============================================================================
if(_RV_MISSING_APT OR _RV_MISSING_OTHER)
    message("")
    message("═══════════════════════════════════════════════════════════════")
    message(" RiVision Worker — 配置失败，缺失依赖如下：")
    message("═══════════════════════════════════════════════════════════════")
    if(_RV_MISSING_APT)
        message("")
        message(" 缺失 apt 开发包：")
        foreach(_p ${_RV_MISSING_APT})
            message("   - ${_p}")
        endforeach()
        string(REPLACE ";" " " _apt_install_args "${_RV_MISSING_APT}")
        message("")
        message(" 修复命令（K3 上执行）：")
        message("   sudo apt update && sudo apt install -y ${_apt_install_args}")
    endif()
    if(_RV_MISSING_OTHER)
        message("")
        message(" 其他问题：")
        foreach(_x ${_RV_MISSING_OTHER})
            message("   - ${_x}")
        endforeach()
    endif()
    message("")
    message("═══════════════════════════════════════════════════════════════")
    message(FATAL_ERROR "请先解决以上依赖再重新 cmake ..")
endif()
message(STATUS "✅ All required dependencies satisfied")

# =============================================================================
# Source files
# =============================================================================

# HAL interfaces (header-only)
set(HAL_HEADERS
    src/hal/interface/i_demux.h
    src/hal/interface/i_decoder.h
    src/hal/interface/i_encoder.h
    src/hal/interface/i_graphics.h
    src/hal/interface/i_mux.h
    src/hal/interface/i_inference.h
)

# Core sources
set(CORE_SOURCES
    src/core/worker.cpp
    src/core/stream_manager.cpp
    src/core/config_manager.cpp
    src/core/notify_dispatcher.cpp
    src/core/rules/engine.cpp
    src/core/rules/triggers.cpp
    src/core/rules/actions.cpp
    src/core/analytics/collector.cpp
    src/core/analytics/heatmap.cpp
    src/core/search/engine.cpp
    src/core/search/vectorize.cpp
    src/core/search/smart_engine.cpp
    src/core/search/rrf_fusion.cpp
    src/core/search/vlm_reranker.cpp
    src/core/search/intent_parser.cpp
    src/core/search/query_cache.cpp
    src/core/search/session_manager.cpp
    src/core/search/async_precompute.cpp
    src/core/search/federated_search.cpp
    src/core/tracker/byte_track.cpp
)

set(CORE_HEADERS
    src/core/config_manager.h
    src/core/rules/engine.h
    src/core/rules/triggers.h
    src/core/rules/actions.h
    src/core/analytics/collector.h
    src/core/analytics/heatmap.h
    src/core/search/engine.h
    src/core/search/vectorize.h
    src/core/search/smart_engine.h
    src/core/search/rrf_fusion.h
    src/core/search/vlm_reranker.h
    src/core/search/intent_parser.h
    src/core/search/query_cache.h
    src/core/search/session_manager.h
    src/core/search/async_precompute.h
    src/core/search/federated_search.h
)

# Pipeline sources
set(PIPELINE_SOURCES
    src/pipeline/pipeline_manager.cpp
    src/pipeline/stream_context.cpp
    src/pipeline/inference/yolo_service.cpp
    src/pipeline/inference/yolo_config.cpp
    src/pipeline/inference/embedder.cpp
    src/pipeline/output/snapshot.cpp
    src/pipeline/output/recording.cpp
)

# Output sources
set(OUTPUT_SOURCES
    src/output/rtsp_output.cpp
)

# HAL sources
set(HAL_SOURCES
    src/hal/factory.cpp
    src/hal/platform.cpp
)

set(HAL_HEADERS
    src/hal/factory.h
    src/hal/platform.h
    src/hal/interface/i_demux.h
    src/hal/interface/i_decoder.h
    src/hal/interface/i_encoder.h
    src/hal/interface/i_graphics.h
    src/hal/interface/i_mux.h
    src/hal/interface/i_inference.h
)

# Storage sources
set(STORAGE_SOURCES
    src/storage/meta_db.cpp
    src/storage/event_cache.cpp
    src/storage/vector_store.cpp
    src/storage/recording_store.cpp
    src/storage/stream_store.cpp
    src/storage/fts5_index.cpp
    src/storage/tracklet_store.cpp
    src/storage/hnsw_store.cpp
)

# Hub sources
set(HUB_SOURCES
    src/hub/hub_client.cpp
    src/hub/heartbeat.cpp
    src/hub/event_publisher.cpp
)

# API sources (handlers + websocket) - requires oatpp
if(USE_OATPP)
    set(API_SOURCES
        src/api/server.cpp
        src/api/router.cpp
        src/api/handlers/health_handler.cpp
        src/api/handlers/stream_handler.cpp
        src/api/handlers/search_handler.cpp
        src/api/handlers/analytics_handler.cpp
        src/api/handlers/config_handler.cpp
        src/api/handlers/rules_handler.cpp
        src/api/handlers/thumbnail_handler.cpp
        src/api/websocket/ws_server.cpp
        src/api/websocket/ws_hub_client.cpp
    )
    set(API_HEADERS
        src/api/server.h
        src/api/router.h
        src/api/handlers/health_handler.h
        src/api/handlers/stream_handler.h
        src/api/handlers/search_handler.h
        src/api/handlers/analytics_handler.h
        src/api/handlers/config_handler.h
        src/api/handlers/rules_handler.h
        src/api/handlers/thumbnail_handler.h
        src/api/websocket/ws_server.h
        src/api/websocket/ws_hub_client.h
    )
else()
    set(API_SOURCES src/api/server.cpp)
    set(API_HEADERS src/api/server.h)
endif()

# Utils sources
set(UTILS_SOURCES
    src/utils/metrics.cpp
    src/utils/system_monitor.cpp
    src/utils/color_cvt.cpp
)

# Backend sources — SpacemiT MPP only (DEMUX, VDEC, VENC, V2D, MUX, NPU)
set(BACKEND_SOURCES
    src/backend/spacemit/mpp_common.cpp
    src/backend/spacemit/mpp_demux.cpp
    src/backend/spacemit/mpp_decoder.cpp
    src/backend/spacemit/mpp_encoder.cpp
    src/backend/spacemit/mpp_graphics.cpp
    src/backend/spacemit/mpp_mux.cpp
    src/backend/spacemit/npu_inference.cpp
)

# VLM client sources (conditional)
if(USE_VLM_CLIENT)
    set(VLM_CLIENT_SOURCES 
        src/vlm/vlm_client.cpp
        src/vlm/vlm_batch_client.cpp
        src/vlm/llama_engine.cpp
        src/vlm/llama_server_manager.cpp
        src/vlm/vision_encoder.cpp
    )
else()
    set(VLM_SOURCES "")
endif()

# Embed sources (built-in CLIP embedding, same pattern as YOLO)
set(EMBED_SOURCES
    src/embed/embed_service.cpp
)

# =============================================================================
# Main executable: rivision_worker
# =============================================================================
add_executable(rivision_worker
    src/main.cpp
    ${VLM_SOURCES}
    ${EMBED_SOURCES}
    ${CORE_SOURCES}
    ${CORE_HEADERS}
    ${PIPELINE_SOURCES}
    ${OUTPUT_SOURCES}
    ${HAL_SOURCES}
    ${HAL_HEADERS}
    ${STORAGE_SOURCES}
    ${HUB_SOURCES}
    ${API_SOURCES}
    ${API_HEADERS}
    ${UTILS_SOURCES}
    ${BACKEND_SOURCES}
)

set_target_properties(rivision_worker PROPERTIES OUTPUT_NAME "rivision-worker")

# SQLite3 linking
if(TARGET SQLite::SQLite3)
    target_link_libraries(rivision_worker PRIVATE SQLite::SQLite3)
elseif(SQLite3_LIBRARIES)
    target_link_libraries(rivision_worker PRIVATE ${SQLite3_LIBRARIES})
endif()
if(SQLite3_INCLUDE_DIRS)
    target_include_directories(rivision_worker PRIVATE ${SQLite3_INCLUDE_DIRS})
endif()

# =============================================================================
# CURL (system, for VLM client)
# =============================================================================
if(USE_VLM_CLIENT)
    find_package(CURL REQUIRED)
    target_link_libraries(rivision_worker PRIVATE CURL::libcurl)
    add_definitions(-DUSE_VLM_CLIENT)
    message(STATUS "VLM Client: Enabled (system libcurl)")
endif()

# OpenCV + pthread
target_link_libraries(rivision_worker PRIVATE
    ${OpenCV_LIBS}
    pthread
)

# yaml-cpp
if(yaml-cpp_FOUND OR TARGET yaml-cpp)
    target_link_libraries(rivision_worker PRIVATE yaml-cpp)
endif()

# oatpp
if(USE_OATPP)
    if(TARGET oatpp::oatpp)
        target_link_libraries(rivision_worker PRIVATE oatpp::oatpp oatpp::oatpp-websocket)
    else()
        target_link_libraries(rivision_worker PRIVATE oatpp oatpp-websocket)
        target_include_directories(rivision_worker PRIVATE
            ${OATPP_SOURCE_DIR}/src
            ${OATPP_WS_SOURCE_DIR}/src)
    endif()
endif()

# SpacemiT MPP backend (always linked) + system FFmpeg dynamic libs
target_include_directories(rivision_worker PRIVATE ${SPACEMIT_MPP_INCLUDE_DIRS})
target_link_libraries(rivision_worker PRIVATE
    ${SPACEMIT_MPP_LIBRARIES}
    ${SPACEMIT_ORT_LIBRARIES}
    avformat avcodec swresample avutil   # system FFmpeg (apt: libavformat-dev ...)
)

# ONNX Runtime + SpacemiT EP — 两者必须同时链接（spacemit_ep 含 NPU 加速入口）
if(ONNXRUNTIME_FOUND)
    target_include_directories(rivision_worker PRIVATE ${ONNXRUNTIME_INCLUDE_DIR})
    target_link_directories(rivision_worker PRIVATE ${ONNXRUNTIME_LIB_DIR})
    target_link_libraries(rivision_worker PRIVATE onnxruntime spacemit_ep)
    # BUILD_RPATH: 让编译出的二进制在 K3 本地运行时能找到 bundled .so
    set_target_properties(rivision_worker PROPERTIES
        BUILD_RPATH  "${ONNXRUNTIME_LIB_DIR}"
        INSTALL_RPATH "$ORIGIN/../lib"
    )
    add_definitions(-DUSE_ONNXRUNTIME)
endif()

# =============================================================================
# VLM executable (optional)
# =============================================================================
if(BUILD_VLM)
    set(VLM_SOURCES
        src/vlm/main_vlm.cpp
        src/vlm/vlm_server.cpp
        src/vlm/llm_engine.cpp
        src/vlm/vision_encoder.cpp
    )

    set(VLM_HEADERS
        src/vlm/vlm_server.h
        src/vlm/llm_engine.h
        src/vlm/vision_encoder.h
        src/vlm/vlm_client.h
        src/vlm/prompt_template.h
    )

    add_executable(rivision_vlm
        ${VLM_SOURCES}
        ${VLM_HEADERS}
        src/utils/logger.cpp
        src/utils/color_cvt.cpp
    )

    target_include_directories(rivision_vlm PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}/include
        ${CMAKE_CURRENT_SOURCE_DIR}/src
    )

    target_link_libraries(rivision_vlm PRIVATE
        pthread
        ${OpenCV_LIBS}
    )

    # llama.cpp — bundled at third_party/spacemit-llama.cpp/
    if(LLAMA_FOUND)
        target_include_directories(rivision_vlm PRIVATE ${LLAMA_INCLUDE_DIR})
        target_link_directories(rivision_vlm PRIVATE ${LLAMA_LIB_DIR})
        target_link_libraries(rivision_vlm PRIVATE llama ggml ggml-base)
        set_target_properties(rivision_vlm PROPERTIES
            BUILD_RPATH  "${LLAMA_LIB_DIR}"
            INSTALL_RPATH "$ORIGIN/../lib"
        )
    endif()
endif()

# =============================================================================
# Tests
# =============================================================================
if(BUILD_TESTS)
    enable_testing()
    find_package(GTest QUIET)

    if(GTest_FOUND)
        add_executable(test_rules test/unit/test_rules_engine.cpp)
        target_link_libraries(test_rules PRIVATE GTest::gtest GTest::gtest_main)
        add_test(NAME test_rules COMMAND test_rules)

        add_executable(test_search test/unit/test_search.cpp)
        target_link_libraries(test_search PRIVATE GTest::gtest GTest::gtest_main)
        add_test(NAME test_search COMMAND test_search)

        add_executable(test_analytics test/unit/test_analytics.cpp)
        target_link_libraries(test_analytics PRIVATE GTest::gtest GTest::gtest_main)
        add_test(NAME test_analytics COMMAND test_analytics)
    endif()
endif()

# =============================================================================
# Install
# =============================================================================
install(TARGETS rivision_worker DESTINATION bin)
if(BUILD_VLM)
    install(TARGETS rivision_vlm DESTINATION bin)
endif()
install(DIRECTORY config/ DESTINATION config)

# =============================================================================
# Summary
# =============================================================================
message(STATUS "=== rivision_worker Build Configuration ===")
message(STATUS "  Platform:    riscv64 (SpacemiT K3)")
message(STATUS "  BUILD_TYPE:  ${CMAKE_BUILD_TYPE}")
message(STATUS "  BUILD_VLM:   ${BUILD_VLM}")
message(STATUS "  BUILD_TESTS: ${BUILD_TESTS}")
message(STATUS "  USE_OATPP:   ${USE_OATPP}")
message(STATUS "============================================")

# SpacemiT MPP backend for K1/K3 RISC-V
# SDK Paths:
#   SPACEMIT_ORT_DIR    - SpacemiT ONNX Runtime (NPU)
#   SPACEMIT_OPENCV_DIR - SpacemiT OpenCV (RISC-V)
#   MODEL_ZOO_VISION_DIR - Reference YOLO implementations

set(SPACEMIT_SOURCES
    mpp_common.cpp
    mpp_demux.cpp
    mpp_decoder.cpp
    mpp_encoder.cpp
    mpp_graphics.cpp
    npu_inference.cpp
)

set(SPACEMIT_HEADERS
    mpp_common.h
    mpp_demux.h
    mpp_decoder.h
    mpp_encoder.h
    mpp_graphics.h
    npu_inference.h
)

add_library(rivision_backend_spacemit STATIC
    ${SPACEMIT_SOURCES}
    ${SPACEMIT_HEADERS}
)

target_include_directories(rivision_backend_spacemit PUBLIC
    ${CMAKE_SOURCE_DIR}/src
    ${CMAKE_SOURCE_DIR}/include
)

# =============================================================================
# SpacemiT ONNX Runtime SDK (for NPU inference)
# =============================================================================
if(SPACEMIT_ORT_DIR AND EXISTS "${SPACEMIT_ORT_DIR}")
    message(STATUS "SpacemiT ORT: ${SPACEMIT_ORT_DIR}")
    target_include_directories(rivision_backend_spacemit PRIVATE
        ${SPACEMIT_ORT_DIR}/include
    )
    target_link_directories(rivision_backend_spacemit PRIVATE
        ${SPACEMIT_ORT_DIR}/lib
    )
    target_link_libraries(rivision_backend_spacemit PRIVATE
        onnxruntime
        spacemit_ep
    )
else()
    find_package(SpacemiTORT REQUIRED)
    target_include_directories(rivision_backend_spacemit PRIVATE ${SPACEMIT_ORT_INCLUDE_DIRS})
    target_link_libraries(rivision_backend_spacemit PRIVATE ${SPACEMIT_ORT_LIBRARIES})
endif()

# =============================================================================
# SpacemiT OpenCV (RISC-V)
# =============================================================================
if(SPACEMIT_OPENCV_DIR AND EXISTS "${SPACEMIT_OPENCV_DIR}")
    message(STATUS "SpacemiT OpenCV: ${SPACEMIT_OPENCV_DIR}")
    set(OpenCV_DIR "${SPACEMIT_OPENCV_DIR}/lib/cmake/opencv4")
    find_package(OpenCV REQUIRED COMPONENTS core imgproc dnn NO_DEFAULT_PATH)
    target_include_directories(rivision_backend_spacemit PRIVATE ${OpenCV_INCLUDE_DIRS})
    target_link_libraries(rivision_backend_spacemit PRIVATE ${OpenCV_LIBS})
endif()

# =============================================================================
# SpacemiT MPP SDK (for hardware codec)
# =============================================================================
if(SPACEMIT_MPP_DIR AND EXISTS "${SPACEMIT_MPP_DIR}")
    message(STATUS "SpacemiT MPP: ${SPACEMIT_MPP_DIR}")
    target_include_directories(rivision_backend_spacemit PRIVATE
        ${SPACEMIT_MPP_DIR}/include
    )
    target_link_directories(rivision_backend_spacemit PRIVATE
        ${SPACEMIT_MPP_DIR}/lib
    )
    target_link_libraries(rivision_backend_spacemit PRIVATE
        vb_mgr
        vpu_api
        cam_mgr
        v2d
    )
else()
    find_package(SpacemiTMPP QUIET)
    if(SpacemiTMPP_FOUND)
        target_include_directories(rivision_backend_spacemit PRIVATE ${SPACEMIT_MPP_INCLUDE_DIRS})
        target_link_libraries(rivision_backend_spacemit PRIVATE ${SPACEMIT_MPP_LIBRARIES})
    else()
        message(WARNING "SpacemiT MPP not found - hardware codec disabled")
    endif()
endif()

# MODEL_ZOO_VISION removed (参考实现已内化到 npu_inference.cpp 和 yolo_service.cpp)

# Enable NPU by default
target_compile_definitions(rivision_backend_spacemit PRIVATE USE_NPU=1)

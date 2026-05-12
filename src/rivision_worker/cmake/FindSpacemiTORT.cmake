# Find SpacemiT ONNX Runtime SDK
#
# This module defines:
#   SPACEMIT_ORT_FOUND        - SpacemiT ORT SDK found
#   SPACEMIT_ORT_INCLUDE_DIRS - Include directories
#   SPACEMIT_ORT_LIBRARIES    - Libraries to link

# Try explicit SDK path first
if(SPACEMIT_ORT_DIR AND EXISTS "${SPACEMIT_ORT_DIR}")
    set(SPACEMIT_ORT_INCLUDE_DIR "${SPACEMIT_ORT_DIR}/include")
    
    find_library(SPACEMIT_ORT_LIBRARY
        NAMES onnxruntime
        PATHS "${SPACEMIT_ORT_DIR}/lib"
        NO_DEFAULT_PATH
    )
    
    find_library(SPACEMIT_EP_LIBRARY
        NAMES spacemit_ep
        PATHS "${SPACEMIT_ORT_DIR}/lib"
        NO_DEFAULT_PATH
    )
else()
    # Fallback to system paths
    find_path(SPACEMIT_ORT_INCLUDE_DIR
        NAMES onnxruntime_cxx_api.h spine_vision_engine.h
        PATHS
            /usr/include
            /usr/local/include
            /opt/spacemit-ort/include
        PATH_SUFFIXES onnxruntime
    )
    
    find_library(SPACEMIT_ORT_LIBRARY
        NAMES onnxruntime
        PATHS
            /usr/lib
            /usr/local/lib
            /opt/spacemit-ort/lib
    )
    
    find_library(SPACEMIT_EP_LIBRARY
        NAMES spacemit_ep
        PATHS
            /usr/lib
            /usr/local/lib
            /opt/spacemit-ort/lib
    )
endif()

include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(SpacemiTORT
    REQUIRED_VARS
        SPACEMIT_ORT_INCLUDE_DIR
        SPACEMIT_ORT_LIBRARY
)

if(SpacemiTORT_FOUND)
    set(SPACEMIT_ORT_INCLUDE_DIRS ${SPACEMIT_ORT_INCLUDE_DIR})
    set(SPACEMIT_ORT_LIBRARIES ${SPACEMIT_ORT_LIBRARY})
    
    if(SPACEMIT_EP_LIBRARY)
        list(APPEND SPACEMIT_ORT_LIBRARIES ${SPACEMIT_EP_LIBRARY})
    endif()
    
    message(STATUS "Found SpacemiT ORT:")
    message(STATUS "  Include: ${SPACEMIT_ORT_INCLUDE_DIRS}")
    message(STATUS "  Libraries: ${SPACEMIT_ORT_LIBRARIES}")
endif()

mark_as_advanced(
    SPACEMIT_ORT_INCLUDE_DIR
    SPACEMIT_ORT_LIBRARY
    SPACEMIT_EP_LIBRARY
)

# Find SpacemiT MPP (Media Processing Platform)
#
# K3-only: paths flattened — third_party/mpp/include + third_party/mpp/lib
# (no riscv64/spacemit subdirectory, since the whole project is K3-RISC-V only)
#
# This module defines:
#   SpacemiTMPP_FOUND          - System has SpacemiT MPP SDK
#   SPACEMIT_MPP_INCLUDE_DIRS  - Include directories
#   SPACEMIT_MPP_LIBRARIES     - Static libraries to link

set(SPACEMIT_MPP_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/third_party/mpp")
set(SPACEMIT_MPP_INCLUDE_DIR "${SPACEMIT_MPP_ROOT}/include")
set(SPACEMIT_MPP_LIB_DIR     "${SPACEMIT_MPP_ROOT}/lib")

if(EXISTS "${SPACEMIT_MPP_INCLUDE_DIR}/sys/sys_api.h")
    set(SpacemiTMPP_FOUND TRUE)
else()
    set(SpacemiTMPP_FOUND FALSE)
endif()

if(SpacemiTMPP_FOUND)
    set(SPACEMIT_MPP_INCLUDE_DIRS ${SPACEMIT_MPP_INCLUDE_DIR})

    # Static MPP libraries (order matters: dependent libs after their dependencies)
    set(SPACEMIT_MPP_LIBRARIES
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_demux.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_mux.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_vdec.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_venc.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_vi.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_vo.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_v2d.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_uvc.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_al_module.a"
        "${SPACEMIT_MPP_LIB_DIR}/libmpp_sys.a"
        "${SPACEMIT_MPP_LIB_DIR}/libutils.a"
    )

    message(STATUS "Found SpacemiT MPP:")
    message(STATUS "  Include: ${SPACEMIT_MPP_INCLUDE_DIRS}")
    message(STATUS "  Lib dir: ${SPACEMIT_MPP_LIB_DIR}")
endif()

mark_as_advanced(
    SPACEMIT_MPP_INCLUDE_DIR
    SPACEMIT_MPP_LIB_DIR
)

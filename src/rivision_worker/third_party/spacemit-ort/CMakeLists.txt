cmake_minimum_required(VERSION 3.13)

include(CheckCXXCompilerFlag)

project(spacemit_ort_samples)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

if(CMAKE_BUILD_TYPE STREQUAL "Release")
  set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -Wno-builtin-macro-redefined -U__FILE__ -D__FILE__='\"$(subst $(dir $<),,$<)\"'")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Wno-builtin-macro-redefined -U__FILE__ -D__FILE__='\"$(subst $(dir $<),,$<)\"'")
endif()

if (DEFINED ONNXRUNTIME_PUBLIC_INCLUDE_DIRS)
    add_executable(imagenet_test imagenet_test.cc)
    add_executable(run_demo demo.cc)
    add_executable(concurrency_demo concurrency_demo.cc)
    spine_configure_target(imagenet_test)
    spine_configure_target(run_demo)
    spine_configure_target(concurrency_demo)
else()
    add_executable(imagenet_test samples/imagenet_test.cc)
    add_executable(run_demo samples/demo.cc)
    add_executable(concurrency_demo samples/concurrency_demo.cc)
    set(DEMO_INCLUDE_DIRS
        ${CMAKE_CURRENT_LIST_DIR}/include/
    )
    set(DEMO_LIB_DIRS
        ${CMAKE_CURRENT_LIST_DIR}/lib/
    )
endif()

target_include_directories(
    imagenet_test PRIVATE
    ${DEMO_INCLUDE_DIRS}
)

target_include_directories(
    run_demo PRIVATE
    ${DEMO_INCLUDE_DIRS}
)

target_include_directories(
    concurrency_demo PRIVATE
    ${DEMO_INCLUDE_DIRS}
)

target_link_directories(
    imagenet_test PRIVATE
    ${DEMO_LIB_DIRS}
)

target_link_directories(
    run_demo PRIVATE
    ${DEMO_LIB_DIRS}
)

target_link_directories(
    concurrency_demo PRIVATE
    ${DEMO_LIB_DIRS}
)

set(SPACEMIT_ORT_LIBS onnxruntime spacemit_ep)

target_link_libraries(imagenet_test PRIVATE ${SPACEMIT_ORT_LIBS})
target_link_libraries(run_demo PRIVATE ${SPACEMIT_ORT_LIBS})
target_link_libraries(concurrency_demo PRIVATE ${SPACEMIT_ORT_LIBS})

install(TARGETS imagenet_test
        ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
        LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
        RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
        FRAMEWORK DESTINATION ${CMAKE_INSTALL_BINDIR})


install(TARGETS run_demo
        ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
        LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
        RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
        FRAMEWORK DESTINATION ${CMAKE_INSTALL_BINDIR})

install(TARGETS concurrency_demo
        ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
        LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
        RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
        FRAMEWORK DESTINATION ${CMAKE_INSTALL_BINDIR})

if(SPACEMIT_EP_BUILD_WITH_LLAMACPP)
    add_executable(multimodal_qa_demo multimodal_qa_demo.cc)
    set(LLAMACPP_LIB_DIR   "${LLAMACPP_INSTALL_DIR}/lib")
    set(LLAMACPP_INC_DIR   "${LLAMACPP_INSTALL_DIR}/include")

    target_include_directories(multimodal_qa_demo PRIVATE
        ${ONNXRUNTIME_PUBLIC_INCLUDE_DIRS}
        ${GSL_INCLUDE_DIR}
        ${SPACEMIT_EP_ROOT_DIR}/src/llm
        ${LLAMACPP_INC_DIR}
    )

    target_link_directories(
        multimodal_qa_demo PRIVATE
        ${PUBLIC_LIB_DIRS}
        ${LLAMACPP_LIB_DIR}
    )

    target_link_libraries(multimodal_qa_demo PRIVATE
        spine_llm
        llama
        ggml
        ggml-cpu
        ggml-base
        ${SPACEMIT_ORT_LIBS}
    )

    install(TARGETS multimodal_qa_demo
            ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
            LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
            RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
            FRAMEWORK DESTINATION ${CMAKE_INSTALL_BINDIR})
endif()

#!/usr/bin/env bash
set -e
mkdir -p build/riscv64
CMAKE_ARGS=()
CMAKE_ARGS+=("-DCMAKE_BUILD_TYPE=$1")
CMAKE_ARGS+=("-DCMAKE_TOOLCHAIN_FILE=$PWD/scripts/linux_riscv64.toolchain.cmake")
CMAKE_ARGS+=($@)
cd build/riscv64 && cmake ../.. \
    "${CMAKE_ARGS[@]}"
if [ "$(uname)" == "Darwin" ]
then
  cmake --build . -- "-j$(sysctl -n hw.ncpu)"
else
  cmake --build . -- "-j$(nproc)"
fi

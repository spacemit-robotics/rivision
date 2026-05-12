#!/usr/bin/env bash
set -e
mkdir -p build/local
CMAKE_ARGS=()
CMAKE_ARGS+=("-DCMAKE_BUILD_TYPE=$1")
CMAKE_ARGS+=($@)
cd build/local && cmake ../.. \
    "${CMAKE_ARGS[@]}"
if [ "$(uname)" == "Darwin" ]
then
  cmake --build . -- "-j$(sysctl -n hw.ncpu)"
else
  cmake --build . -- "-j$(nproc)"
fi

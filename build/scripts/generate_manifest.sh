#!/bin/bash
# generate_manifest.sh — 自动扫描 dist 目录生成 manifest.json
# 不再硬编码组件列表，新增模块自动出现在 manifest 中
set -e

DIST_DIR=$1
VERSION=$2
PLATFORM=$3

if [ -z "$DIST_DIR" ] || [ -z "$VERSION" ] || [ -z "$PLATFORM" ]; then
    echo "Usage: $0 <dist_dir> <version> <platform>"
    exit 1
fi

BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 自动扫描所有可执行文件
COMPONENTS=""
FIRST=true
for bin_dir in "$DIST_DIR/bin" "$DIST_DIR/node/bin"; do
    [ -d "$bin_dir" ] || continue
    for f in "$bin_dir"/*; do
        [ -f "$f" ] || continue
        NAME=$(basename "$f")
        REL_PATH=$(realpath --relative-to="$DIST_DIR" "$f" 2>/dev/null || echo "$f" | sed "s|$DIST_DIR/||")
        SIZE=$(stat -c%s "$f" 2>/dev/null || echo 0)
        CHECKSUM=$(sha256sum "$f" | cut -d' ' -f1)

        if [ "$FIRST" = true ]; then FIRST=false; else COMPONENTS="$COMPONENTS,"; fi
        COMPONENTS="$COMPONENTS
    \"$NAME\": {
      \"path\": \"$REL_PATH\",
      \"size\": $SIZE,
      \"sha256\": \"$CHECKSUM\"
    }"
    done
done

# 扫描引擎目录
ENGINES=""
if [ -d "$DIST_DIR/node/engines" ]; then
    ENGINE_COUNT=$(find "$DIST_DIR/node/engines" -type f -executable 2>/dev/null | wc -l)
    ENGINES="\"engines\": {\"path\": \"node/engines/\", \"count\": $ENGINE_COUNT},"
fi

# 扫描运行时库
LIB_COUNT=0
if [ -d "$DIST_DIR/node/lib" ]; then
    LIB_COUNT=$(find "$DIST_DIR/node/lib" -name "*.so*" 2>/dev/null | wc -l)
fi

# 扫描模型
MODEL_LIST=""
if [ -d "$DIST_DIR/node/models" ]; then
    MODEL_LIST=$(find "$DIST_DIR/node/models" -maxdepth 1 -mindepth 1 -type d -exec basename {} \; 2>/dev/null | sort | tr '\n' ',' | sed 's/,$//')
fi

cat > "$DIST_DIR/manifest.json" <<EOF
{
  "version": "$VERSION",
  "platform": "$PLATFORM",
  "build_time": "$BUILD_TIME",
  "git_commit": "$GIT_COMMIT",
  "components": {$COMPONENTS
  },
  $ENGINES
  "runtime_libs": {"path": "node/lib/", "count": $LIB_COUNT},
  "models": ["$(echo "$MODEL_LIST" | sed 's/,/", "/g')"]
}
EOF

COMP_COUNT=$(echo "$COMPONENTS" | grep -c '"path"' || echo 0)
echo "Manifest generated: $COMP_COUNT components, $LIB_COUNT libs, models: [$MODEL_LIST]"

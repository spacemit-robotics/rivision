# RiVision Node - Profile-based Build System

PROFILE ?= riscv_b
VERSION ?= 1.0.0
DIST_DIR ?= dist/$(PROFILE)

.PHONY: build package-x86 package-riscv-a package-riscv-b package-arm-gpu clean help update-third-party
.PHONY: build-yolo-riscv64 build-yolo-arm64
.PHONY: build-benchmark benchmark-yolo benchmark-vlm benchmark-all

# 按 profile 打包
package:
	@./package_profile.sh $(PROFILE) $(DIST_DIR)

package-x86:
	@$(MAKE) package PROFILE=x86

package-riscv-a:
	@$(MAKE) package PROFILE=riscv_a

package-riscv-b:
	@$(MAKE) package PROFILE=riscv_b

package-arm-gpu:
	@$(MAKE) package PROFILE=arm_gpu

# 仅构建 node-agent 二进制
build-agent-riscv64:
	@mkdir -p dist/bin
	cd node-agent && GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 \
		go build -ldflags "-s -w" -o ../dist/bin/rivision_node .

build-agent-x86:
	@mkdir -p dist/bin
	cd node-agent && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags "-s -w" -o ../dist/bin/rivision_node .

build-agent-arm64:
	@mkdir -p dist/bin
	cd node-agent && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags "-s -w" -o ../dist/bin/rivision_node .

# yolo-server  cross-compilation
build-yolo-riscv64:
	@cd yolo-server && ./scripts/build-riscv64.sh

build-yolo-arm64:
	@cd yolo-server && ./scripts/build-arm64.sh

update-third-party:
	@../../scripts/update-third-party.sh

all-profiles: package-x86 package-riscv-a package-riscv-b package-arm-gpu
	@echo "All profiles packaged."

clean:
	@rm -rf dist/
	@rm -rf benchmark/build/ benchmark/build-riscv64/
	@rm -rf yolo-server/bin/
	@rm -rf yolo-server/build-native/ yolo-server/build-riscv64/ yolo-server/build-arm64/

help:
	@echo "RiVision Node Build System"
	@echo ""
	@echo "Usage: make <target> [PROFILE=x86|riscv_a|riscv_b|arm_gpu]"
	@echo ""
	@echo "Profiles:"
	@echo "  x86       - x86_64 (onnxruntime_x86 + llama.cpp standard)"
	@echo "  riscv_a   - RISC-V CPU (beta2 + llama.cpp legacy)"
	@echo "  riscv_b   - RISC-V NPU (beta3 + llama.cpp 0.0.7+a0)"
	@echo "  arm_gpu   - ARM64 + NVIDIA GPU (Jetson Orin, CUDA)"
	@echo ""
	@echo "Model mapping:"
	@echo "  x86/riscv_a/arm_gpu: Qwen3VL (default) + miniCPM_v4.5 (optional)"
	@echo "  riscv_b:             Qwen3VL (default) + fastvlm-mm (NPU)"
	@echo ""
	@echo "Targets:"
	@echo "  package            - Package for PROFILE"
	@echo "  package-x86        - Package for x86_64"
	@echo "  package-riscv-a    - Package for RISC-V CPU mode"
	@echo "  package-riscv-b    - Package for RISC-V NPU mode"
	@echo "  package-arm-gpu    - Package for ARM64 + NVIDIA GPU"
	@echo "  build-yolo-riscv64 - Build yolo-server for RISC-V (cross-compilation)"
	@echo "  build-yolo-arm64   - Build yolo-server for ARM64 (cross-compilation)"
	@echo "  update-third-party - Update third_party versions and rebuild yolo if ORT changes"
	@echo "  build-agent-riscv64- Build node-agent only (riscv64)"
	@echo "  build-agent-x86    - Build node-agent only (x86)"
	@echo "  build-agent-arm64  - Build node-agent only (arm64)"
	@echo "  all-profiles       - Package all profiles"
	@echo "  build-benchmark    - Build benchmark tool"
	@echo "  benchmark-yolo     - Run YOLO benchmark (HTTP)"
	@echo "  benchmark-vlm      - Run VLM benchmark"
	@echo "  benchmark-all      - Run all benchmarks"
	@echo "  clean              - Remove build artifacts"

# Benchmark 模块
build-benchmark:
	@mkdir -p benchmark/build
	@cd benchmark/build && cmake .. -DCMAKE_BUILD_TYPE=Release && make -j$$(nproc)
	@echo "Benchmark tool built: benchmark/build/rivision-benchmark"

benchmark-yolo:
	@./benchmark/scripts/run_yolo_http.sh

benchmark-vlm:
	@./benchmark/scripts/run_vlm_local.sh

benchmark-all:
	@./benchmark/scripts/run_all.sh

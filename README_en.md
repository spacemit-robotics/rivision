# RiVision

RiVision is a distributed AI inference platform for video streams. Built on the SpaceMIT K3 RISC-V chip, it provides parallel, high-performance inference services for visual models such as YOLO object detection and VLM image understanding.

## Problem Statement

Traditional video AI solutions rely on centralized GPU servers, which are costly, high-latency, and difficult to scale. RiVision pushes inference capability down to low-power RISC-V edge nodes, enabling elastic scaling through multi-node clusters:

| Cluster Size | K3 Nodes | Parallel Capacity | Typical Use Case |
|--------------|----------|-------------------|------------------|
| Entry | 1 | 1-4 video streams | Single store, small warehouse |
| Standard | 4 | 4-16 video streams | Campus, medium factory |
| Extended | 8 | 8-32 video streams | Large campus, multi-floor |
| Cluster | 16-32 | 16-128 video streams | City-level, large industrial park |

Each K3 node is low-cost and low-power. Simply add or remove nodes to match your business scale—no upfront investment in expensive hardware required.

## Core Capabilities

- **YOLO Real-time Object Detection** — Person, vehicle, and object recognition with parallel inference across multiple video streams
- **VLM Image Understanding** — Visual language model based on llama.cpp for semantic scene description, anomaly analysis, and video summarization
- **GB28181 Standard Access** — Compatible with public security/transportation industry protocols, seamlessly integrates with existing surveillance systems
- **Streaming Distribution** — Integrated ZLMediaKit + go2rtc, supporting RTSP/WebRTC/HLS multi-protocol forwarding
- **Text Vectorization** — Built-in Embedding service for semantic retrieval of detection results

## System Architecture

```
                        ┌──────────────────────┐
                        │     rivision-cli     │  Host CLI + Web UI
                        │     rivision-owl     │  GB28181 + Streaming
                        │   rivision-gateway   │  Inference Gateway
  Camera/Video Stream ──▶  │   rivision-embed     │  Text Vectorization
                        │       (Host)         │
                        └──────────┬───────────┘
                                   │ Inference Task Distribution
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
             ┌────────────┐ ┌────────────┐ ┌────────────┐
             │ K3 Node #1 │ │ K3 Node #2 │ │ K3 Node #N │
             │ YOLO + VLM │ │ YOLO + VLM │ │ YOLO + VLM │
             └────────────┘ └────────────┘ └────────────┘
                    rivision-node (Scale as needed)
```

- **Host**: Ingests video streams, manages devices, distributes inference tasks, aggregates results
- **Node**: Runs AI models; each K3 node independently executes YOLO detection or VLM inference

## Components

| Component | Directory | Function |
|-----------|-----------|----------|
| rivision-cli | `src/rivision_cli/` | Host CLI with integrated go2rtc streaming, Web UI, and camera management |
| rivision-owl | `src/rivision_owl/` | GB28181 signaling service, ZLMediaKit streaming, device management |
| rivision-gateway | `src/rivision_gateway/` | Inference gateway, forwards VLM/Embedding requests to backend models |
| rivision-embed | `src/rivision_embed/` | Text vectorization service (ONNX Runtime) |
| rivision-node | `src/rivision_node/` | AI inference node with YOLO object detection and llama.cpp VLM |

## Target Platform

Currently supports native compilation on **SpaceMIT K3 RISC-V** platform.

## Quick Start

### Prerequisites

- SpaceMIT K3 development board (Bianbu OS)
- Go 1.25+
- CMake 3.20+
- GCC / G++
- Node.js 18+ (for Web UI build)

Run the one-click setup script to automatically detect and install missing dependencies:

```bash
./scripts/setup-env.sh
```

### 1. Clone the Repository

```bash
git clone --recursive https://github.com/spacemit-robotics/rivision.git
cd rivision
```

### 2. Download Dependencies

```bash
./scripts/setup-deps.sh
```

This script downloads model files, third-party libraries, and other runtime dependencies.

### 3. Apply Patches

```bash
patch -p3 < patches/owl.patch
patch -p3 < patches/gb28181-web.patch
patch -p3 < patches/onvif-go.patch
```

### 4. Build

```bash
cd build
make                         # Full build (host + node)
make release PACKAGE=host    # Host only
make release PACKAGE=node    # Inference node only
```

Build artifacts are output to the `dist/<version>/riscv64/` directory.

### 5. Deploy

```bash
# Host
scp -r dist/<version>/riscv64/host/ k3:/tmp/
ssh k3 "cd /tmp/host && sudo ./install.sh"

# Inference node (repeat for each K3 node)
scp -r dist/<version>/riscv64/node/ k3-node:/tmp/
ssh k3-node "cd /tmp/node && sudo ./install.sh"
```

## Directory Structure

```
rivision/
├── build/              # Build system (Makefile, templates, scripts)
├── patches/            # Local modification patches for submodules
├── scripts/            # Utility scripts
└── src/
    ├── rivision_cli/       # Host CLI
    ├── rivision_embed/     # Text vectorization
    ├── rivision_gateway/   # Inference gateway
    ├── rivision_node/      # AI inference node
    └── rivision_owl/       # GB28181 + Streaming
        ├── owl/            # (submodule) Core service
        ├── gb28181-web/    # (submodule) Web frontend
        ├── onvif-go/       # (submodule) ONVIF protocol
        └── ZLMediaKit/     # (submodule) Streaming engine
```

## Build Command Reference

```bash
make help               # Show all available targets
make check-env          # Check build/runtime environment
make cli                # Build CLI only
make owl                # Build OWL only
make node               # Build Node only
make zlm                # Compile ZLMediaKit
make clean              # Clean build artifacts
```

## Contributing

Contributions are welcome through issues, documentation revisions, parameter optimization, feature enhancements, and bug fixes.

Before contributing:
- Clarify your modification goals and applicable hardware scope
- Include necessary configuration instructions and verification results
- Avoid introducing changes incompatible with the existing build system

## License

Source files in this component are declared as Apache-2.0 in their headers. The `LICENSE` file in this directory is the authoritative source.

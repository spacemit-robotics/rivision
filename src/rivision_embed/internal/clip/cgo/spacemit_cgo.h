/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

// spacemit_cgo.h - C wrapper for SpaceMIT EP initialization
// Only used on RISC-V K3 platform with A100 NPU

#ifndef SPACEMIT_CGO_H
#define SPACEMIT_CGO_H

#include <onnxruntime_c_api.h>

#ifdef __cplusplus
extern "C" {
#endif

// Initialize SpaceMIT NPU environment (call once at startup)
// Returns 0 on success, non-zero on failure
int SpaceMIT_EnvInit(void);

// Attach SpaceMIT EP to session options
// options: ORT session options handle
// num_threads: number of threads for EP (0 for default)
// Returns 0 on success, non-zero on failure
int SpaceMIT_AttachToSessionOptions(OrtSessionOptions *options, int num_threads);

// Check if SpaceMIT EP is available
// Returns 1 if available, 0 otherwise
int SpaceMIT_IsAvailable(void);

#ifdef __cplusplus
}
#endif

#endif  // SPACEMIT_CGO_H

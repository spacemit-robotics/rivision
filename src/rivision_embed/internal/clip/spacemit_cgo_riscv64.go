// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

//go:build linux && riscv64 && cgo

package clip

/*
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}/../../third_party/spacemit-ort-current/include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/spacemit-ort-current/lib -lonnxruntime -lspacemit_ep -lstdc++

#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// ORT C API types
typedef struct OrtSessionOptions OrtSessionOptions;
typedef struct OrtStatus OrtStatus;

// SpaceMIT EP initialization function from libspacemit_ep.so
// This attaches the SpaceMIT EP to the session options
extern OrtStatus* OrtSessionOptionsSpaceMITEnvInit(OrtSessionOptions* options, const char** keys, const char** values, int num_options);

static int g_spacemit_ready = 0;

static int SpaceMIT_EnvInit(void) {
    if (g_spacemit_ready) return 0;
    g_spacemit_ready = 1;
    return 0;
}

static int SpaceMIT_IsAvailable(void) {
    return g_spacemit_ready;
}

// Attach SpaceMIT EP to OrtSessionOptions
// 根据官方文档: SPACEMIT_EP_INTRA_THREAD_NUM=N 会创建 2*N 线程
// 要使用 8 个 A100 核心，应设置 N=4
static int SpaceMIT_AttachEP(void* options_ptr, int num_threads) {
    if (options_ptr == NULL) {
        fprintf(stderr, "[SpaceMIT] Invalid session options pointer\n");
        return -1;
    }
    
    if (!g_spacemit_ready) {
        SpaceMIT_EnvInit();
    }
    
    OrtSessionOptions* options = (OrtSessionOptions*)options_ptr;
    
    // ★ 关键: EP 线程数 = num_threads/2，因为 EP 会创建 2×N 线程
    // 要使用 8 核，设置 SPACEMIT_EP_INTRA_THREAD_NUM=4 (创建 8 线程)
    int ep_threads = (num_threads + 1) / 2;
    if (ep_threads < 1) ep_threads = 1;
    
    char thread_str[16];
    snprintf(thread_str, sizeof(thread_str), "%d", ep_threads);
    
    // ★ 多 Session 共享线程池 (embed 有 text/vision 两个 session)
    const char* keys[] = {"SPACEMIT_EP_INTRA_THREAD_NUM", "SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD"};
    const char* values[] = {thread_str, "1"};
    
    OrtStatus* status = OrtSessionOptionsSpaceMITEnvInit(options, keys, values, 2);
    
    if (status == NULL) {
        printf("[SpaceMIT] ✓ A100 NPU EP attached (EP_THREADS=%d, actual=%d)\n", ep_threads, ep_threads * 2);
        return 0;
    } else {
        fprintf(stderr, "[SpaceMIT] EP attach failed\n");
        return -2;
    }
}
*/
import "C"
import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"unsafe"
)

var (
	spacemitInitOnce sync.Once
	spacemitReady    bool
)

// InitSpaceMITNPU initializes the SpaceMIT A100 NPU environment
func InitSpaceMITNPU() bool {
	spacemitInitOnce.Do(func() {
		ret := C.SpaceMIT_EnvInit()
		spacemitReady = (ret == 0)
		if spacemitReady {
			log.Println("[SpaceMIT] A100 NPU library loaded (libspacemit_ep.so)")
		} else {
			log.Printf("[SpaceMIT] NPU initialization failed (code=%d)", ret)
		}
	})
	return spacemitReady
}

// IsSpaceMITReady returns true if SpaceMIT NPU is available
func IsSpaceMITReady() bool {
	return spacemitReady
}

// AttachSpaceMITEP attaches the SpaceMIT A100 NPU EP to session options
// sessionOptionsPtr should be the raw pointer from ort.SessionOptions
func AttachSpaceMITEP(sessionOptionsPtr unsafe.Pointer, numThreads int) error {
	if sessionOptionsPtr == nil {
		return fmt.Errorf("session options pointer is nil")
	}
	
	ret := C.SpaceMIT_AttachEP(sessionOptionsPtr, C.int(numThreads))
	if ret != 0 {
		return fmt.Errorf("SpaceMIT EP attach failed (code=%d)", ret)
	}
	return nil
}

// GetSessionOptionsPtr extracts the raw OrtSessionOptions pointer from ort.SessionOptions
// This uses unsafe to access the private field 'o' which is the first field in the struct
func GetSessionOptionsPtr(opts interface{}) unsafe.Pointer {
	// SessionOptions struct layout: type SessionOptions struct { o *C.OrtSessionOptions }
	// The 'o' field is the first (and only) field, so we can directly access it
	v := reflect.ValueOf(opts)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		log.Println("[SpaceMIT] Warning: opts is not a valid pointer")
		return nil
	}
	
	// Get the struct value
	structVal := v.Elem()
	if structVal.NumField() == 0 {
		log.Println("[SpaceMIT] Warning: SessionOptions has no fields")
		return nil
	}
	
	// The first field 'o' is *C.OrtSessionOptions
	// Use unsafe to read the pointer value directly from memory
	structPtr := unsafe.Pointer(structVal.UnsafeAddr())
	// First field is at offset 0, read the pointer value
	ptrVal := *(*unsafe.Pointer)(structPtr)
	
	if ptrVal == nil {
		log.Println("[SpaceMIT] Warning: SessionOptions.o is nil")
		return nil
	}
	
	return ptrVal
}

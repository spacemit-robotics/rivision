// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

//go:build !linux || !riscv64 || !cgo

package clip

import "unsafe"

// InitSpaceMITNPU is a stub for non-RISC-V platforms
func InitSpaceMITNPU() bool {
	return false
}

// AttachSpaceMITEP is a stub for non-RISC-V platforms
func AttachSpaceMITEP(sessionOptionsPtr unsafe.Pointer, numThreads int) error {
	return nil
}

// IsSpaceMITReady is a stub for non-RISC-V platforms
func IsSpaceMITReady() bool {
	return false
}

// GetSessionOptionsPtr is a stub for non-RISC-V platforms
func GetSessionOptionsPtr(opts interface{}) unsafe.Pointer {
	return nil
}

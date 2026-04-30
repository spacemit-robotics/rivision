//go:build linux && riscv64

package clip

// SpaceMITEPAvailable 检查是否在 RISC-V K3 平台上运行
// 此文件仅在 linux/riscv64 编译时包含
func SpaceMITEPAvailable() bool {
	return true
}

// GetSpaceMITProviderOptions 返回 SpaceMIT EP 的配置选项
func GetSpaceMITProviderOptions() map[string]string {
	return map[string]string{
		// 可选配置（通过环境变量设置）:
		// SPACEMIT_EP_DISABLE_FLOAT16_EPILOGUE=0  启用 FP16 后处理加速
		// SPACEMIT_EP_DEBUG_PROFILE=embed         导出性能分析
	}
}

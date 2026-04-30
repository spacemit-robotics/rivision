//go:build !linux || !riscv64

package clip

// SpaceMITEPAvailable 在非 K3 平台返回 false
func SpaceMITEPAvailable() bool {
	return false
}

// GetSpaceMITProviderOptions 返回空配置（非 K3 平台）
func GetSpaceMITProviderOptions() map[string]string {
	return nil
}

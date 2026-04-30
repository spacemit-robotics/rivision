package embed

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// 嵌入 go2rtc 和 rtsp_server 二进制文件
// 构建前由 prepare_build.sh 将对应平台的二进制复制到 binaries/ 根目录
// 只嵌入根目录下的二进制，不嵌入 platform/ 子目录（避免体积膨胀）
//
//go:embed binaries/go2rtc-* binaries/rtsp_server-*
var binariesFS embed.FS

// 嵌入 go2rtc 配置文件
//
//go:embed config/*
var configFS embed.FS

// BinaryInfo 二进制文件信息
type BinaryInfo struct {
	Name     string
	Platform string
	Arch     string
	Path     string
}

// GetGo2rtcBinaryName 根据当前平台返回 go2rtc 二进制文件名
func GetGo2rtcBinaryName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	name := fmt.Sprintf("go2rtc-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// GetRtspServerBinaryName 根据当前平台返回 RTSP Server 二进制文件名
func GetRtspServerBinaryName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	name := fmt.Sprintf("rtsp_server-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// ExtractGo2rtc 提取 go2rtc 二进制到指定目录
// 如果目标文件已存在且可执行，则直接使用，不再写入
func ExtractGo2rtc(destDir string) (string, error) {
	binaryName := GetGo2rtcBinaryName()
	destPath := filepath.Join(destDir, binaryName)

	// 检查文件是否已存在且可执行
	if info, err := os.Stat(destPath); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
		return destPath, nil // 已存在，直接使用
	}

	srcPath := filepath.Join("binaries", binaryName)

	// 读取嵌入的二进制
	data, err := binariesFS.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("未找到当前平台的 go2rtc 二进制 (%s): %w", binaryName, err)
	}

	// 确保目标目录存在
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 写入目标文件
	if err := os.WriteFile(destPath, data, 0755); err != nil {
		// "text file busy": 可执行文件正在被使用，跳过更新
		// "permission denied": 无写权限，检查是否已存在可用文件
		if strings.Contains(err.Error(), "text file busy") || strings.Contains(err.Error(), "permission denied") {
			if _, statErr := os.Stat(destPath); statErr == nil {
				return destPath, nil
			}
		}
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return destPath, nil
}

// ExtractConfig 提取配置文件到指定目录
func ExtractConfig(destDir string) error {
	return fs.WalkDir(configFS, "config", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录
		if path == "config" {
			return nil
		}

		// 计算目标路径
		relPath, _ := filepath.Rel("config", path)
		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// 复制文件
		data, err := configFS.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(destPath, data, 0644)
	})
}

// ListAvailableBinaries 列出所有可用的嵌入二进制
func ListAvailableBinaries() ([]BinaryInfo, error) {
	var binaries []BinaryInfo

	entries, err := binariesFS.ReadDir("binaries")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		info := BinaryInfo{
			Name: name,
			Path: filepath.Join("binaries", name),
		}

		// 解析平台和架构
		// 格式: go2rtc-linux-amd64, go2rtc-windows-amd64.exe
		if len(name) > 7 && name[:7] == "go2rtc-" {
			rest := name[7:]
			// 移除 .exe 后缀
			if len(rest) > 4 && rest[len(rest)-4:] == ".exe" {
				rest = rest[:len(rest)-4]
			}
			// 分割 platform-arch
			for i := len(rest) - 1; i >= 0; i-- {
				if rest[i] == '-' {
					info.Platform = rest[:i]
					info.Arch = rest[i+1:]
					break
				}
			}
		}

		binaries = append(binaries, info)
	}

	return binaries, nil
}

// HasEmbeddedBinary 检查是否有当前平台的嵌入 go2rtc 二进制
func HasEmbeddedBinary() bool {
	binaryName := GetGo2rtcBinaryName()
	srcPath := filepath.Join("binaries", binaryName)
	_, err := binariesFS.ReadFile(srcPath)
	return err == nil
}

// HasEmbeddedRtspServer 检查是否有当前平台的嵌入 RTSP Server 二进制
func HasEmbeddedRtspServer() bool {
	binaryName := GetRtspServerBinaryName()
	srcPath := filepath.Join("binaries", binaryName)
	_, err := binariesFS.ReadFile(srcPath)
	return err == nil
}

// ExtractRtspServer 提取 RTSP Server 二进制到指定目录
// 如果目标文件已存在且可执行，则直接使用，不再写入
func ExtractRtspServer(destDir string) (string, error) {
	binaryName := GetRtspServerBinaryName()
	destPath := filepath.Join(destDir, binaryName)

	// 检查文件是否已存在且可执行
	if info, err := os.Stat(destPath); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
		return destPath, nil // 已存在，直接使用
	}

	srcPath := filepath.Join("binaries", binaryName)

	// 读取嵌入的二进制
	data, err := binariesFS.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("未找到当前平台的 RTSP Server 二进制 (%s): %w", binaryName, err)
	}

	// 确保目标目录存在
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 写入目标文件
	if err := os.WriteFile(destPath, data, 0755); err != nil {
		// "text file busy": 可执行文件正在被使用，跳过更新
		// "permission denied": 无写权限，检查是否已存在可用文件
		if strings.Contains(err.Error(), "text file busy") || strings.Contains(err.Error(), "permission denied") {
			if _, statErr := os.Stat(destPath); statErr == nil {
				return destPath, nil
			}
		}
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return destPath, nil
}

// CopyFile 复制文件
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SystemStats 系统状态
type SystemStats struct {
	CPUPercent       float64 `json:"cpu_percent"`
	CPUCount         int     `json:"cpu_count"`
	MemoryTotalMB    float64 `json:"memory_total_mb"`
	MemoryUsedMB     float64 `json:"memory_used_mb"`
	MemoryPercent    float64 `json:"memory_percent"`
	MemoryAvailGB    float64 `json:"memory_available_gb"`
	DiskTotalGB      float64 `json:"disk_total_gb"`
	DiskUsedGB       float64 `json:"disk_used_gb"`
	DiskPercent      float64 `json:"disk_percent"`
	LoadAvg1m        float64 `json:"load_avg_1m"`
	LoadAvg5m        float64 `json:"load_avg_5m"`
	LoadAvg15m       float64 `json:"load_avg_15m"`
}

// ToMap 转换为 map
func (s *SystemStats) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"cpu_percent":     s.CPUPercent,
		"cpu_count":       s.CPUCount,
		"memory_total_mb": s.MemoryTotalMB,
		"memory_used_mb":  s.MemoryUsedMB,
		"memory_percent":      s.MemoryPercent,
		"memory_available_gb": s.MemoryAvailGB,
		"disk_total_gb":   s.DiskTotalGB,
		"disk_used_gb":    s.DiskUsedGB,
		"disk_percent":    s.DiskPercent,
		"load_avg_1m":     s.LoadAvg1m,
		"load_avg_5m":     s.LoadAvg5m,
		"load_avg_15m":    s.LoadAvg15m,
	}
}

// SystemMonitor 系统监控器
type SystemMonitor struct{}

// GetStats 获取系统状态
func (m *SystemMonitor) GetStats() *SystemStats {
	stats := &SystemStats{
		CPUCount: runtime.NumCPU(),
	}

	// CPU percent
	stats.CPUPercent = m.getCPUPercent()

	// Memory
	m.getMemoryStats(stats)

	// Disk
	m.getDiskStats(stats)

	// Load average
	m.getLoadAvg(stats)

	return stats
}

// getCPUPercent 获取 CPU 使用率
func (m *SystemMonitor) getCPUPercent() float64 {
	// 读取 /proc/stat 两次，计算差值
	idle1, total1 := m.readCPUStat()
	time.Sleep(100 * time.Millisecond)
	idle2, total2 := m.readCPUStat()

	idleDelta := idle2 - idle1
	totalDelta := total2 - total1

	if totalDelta == 0 {
		return 0.0
	}

	cpuPercent := (1.0 - float64(idleDelta)/float64(totalDelta)) * 100.0
	return round2(cpuPercent)
}

// readCPUStat 读取 /proc/stat 第一行
func (m *SystemMonitor) readCPUStat() (idle, total uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, 0
	}

	// cpu  user nice system idle iowait irq softirq steal
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}

	var values []uint64
	for _, f := range fields[1:] {
		v, _ := strconv.ParseUint(f, 10, 64)
		values = append(values, v)
		total += v
	}

	if len(values) >= 4 {
		idle = values[3] // idle 是第4个值
	}

	return idle, total
}

// getMemoryStats 获取内存状态
func (m *SystemMonitor) getMemoryStats(stats *SystemStats) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}

	memInfo := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, _ := strconv.ParseUint(valStr, 10, 64)
		memInfo[key] = val
	}

	totalKB := memInfo["MemTotal"]
	availableKB := memInfo["MemAvailable"]
	usedKB := totalKB - availableKB

	stats.MemoryTotalMB = round2(float64(totalKB) / 1024.0)
	stats.MemoryUsedMB = round2(float64(usedKB) / 1024.0)
	stats.MemoryAvailGB = round2(float64(availableKB) / (1024.0 * 1024.0))
	if totalKB > 0 {
		stats.MemoryPercent = round2(float64(usedKB) / float64(totalKB) * 100.0)
	}
}

// getDiskStats 获取磁盘状态
func (m *SystemMonitor) getDiskStats(stats *SystemStats) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	stats.DiskTotalGB = round2(float64(totalBytes) / (1024 * 1024 * 1024))
	stats.DiskUsedGB = round2(float64(usedBytes) / (1024 * 1024 * 1024))
	if totalBytes > 0 {
		stats.DiskPercent = round2(float64(usedBytes) / float64(totalBytes) * 100.0)
	}
}

// getLoadAvg 获取负载平均值
func (m *SystemMonitor) getLoadAvg(stats *SystemStats) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}

	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		stats.LoadAvg1m, _ = strconv.ParseFloat(fields[0], 64)
		stats.LoadAvg5m, _ = strconv.ParseFloat(fields[1], 64)
		stats.LoadAvg15m, _ = strconv.ParseFloat(fields[2], 64)
		stats.LoadAvg1m = round2(stats.LoadAvg1m)
		stats.LoadAvg5m = round2(stats.LoadAvg5m)
		stats.LoadAvg15m = round2(stats.LoadAvg15m)
	}
}

// GetGPUStats 获取 GPU 状态 (如果有)
func (m *SystemMonitor) GetGPUStats() map[string]interface{} {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}

	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) < 4 {
		return nil
	}

	gpuUtil, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	memUsed, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	memTotal, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	temp, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)

	return map[string]interface{}{
		"accelerator_type": "gpu",
		"gpu_utilization":  gpuUtil,
		"memory_used_mb":   memUsed,
		"memory_total_mb":  memTotal,
		"temperature":      temp,
	}
}

// GetAcceleratorStats 获取加速器状态（GPU/NPU/RVV）
func (m *SystemMonitor) GetAcceleratorStats() map[string]interface{} {
	// 1. 尝试 NVIDIA GPU
	gpuStats := m.GetGPUStats()
	if gpuStats != nil {
		return gpuStats
	}

	// 2. 尝试 NPU (多厂商支持)
	npuStats := m.getNPUStats()
	if npuStats != nil {
		return npuStats
	}

	// 3. 尝试 RISC-V RVV
	rvvStats := m.getRVVStats()
	if rvvStats != nil {
		return rvvStats
	}

	// 无加速器，使用纯CPU
	return map[string]interface{}{
		"accelerator_type": "cpu",
		"cpu_utilization":  m.getCPUUtilization(),
		"memory_used_mb":   0,
		"memory_total_mb":  0,
	}
}

// getNPUStats 获取 NPU 状态
func (m *SystemMonitor) getNPUStats() map[string]interface{} {
	// 尝试华为昇腾 NPU
	out, err := exec.Command("npu-smi", "info", "-t", "usages").Output()
	if err == nil {
		return map[string]interface{}{
			"accelerator_type": "npu_ascend",
			"npu_utilization":  m.parseNPUUtilization(string(out)),
			"memory_used_mb":   0,
			"memory_total_mb":  0,
		}
	}

	// 尝试算能 NPU
	out, err = exec.Command("bm-smi", "--json").Output()
	if err == nil {
		return map[string]interface{}{
			"accelerator_type": "npu_sophon",
			"npu_utilization":  0,
			"memory_used_mb":   0,
			"memory_total_mb":  0,
		}
	}

	return nil
}

// getRVVStats 获取 RISC-V RVV 状态
func (m *SystemMonitor) getRVVStats() map[string]interface{} {
	// 检查是否是 RISC-V 架构
	if runtime.GOARCH != "riscv64" {
		return nil
	}

	// 检查 RVV 支持
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return nil
	}

	hasRVV := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "isa") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "_v") || strings.Contains(lower, "rv64gcv") {
				hasRVV = true
				break
			}
		}
	}

	if hasRVV {
		return map[string]interface{}{
			"accelerator_type": "rvv",
			"rvv_enabled":      true,
			"vlen_bits":        m.detectVLEN(),
			"memory_used_mb":   0,
			"memory_total_mb":  0,
		}
	}

	return nil
}

// detectVLEN 检测 RISC-V 向量寄存器长度
func (m *SystemMonitor) detectVLEN() int {
	return 256 // 默认值
}

// parseNPUUtilization 解析 NPU 利用率
func (m *SystemMonitor) parseNPUUtilization(output string) float64 {
	re := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "utilization") || strings.Contains(lower, "usage") {
			match := re.FindStringSubmatch(line)
			if len(match) >= 2 {
				val, _ := strconv.ParseFloat(match[1], 64)
				return val
			}
		}
	}
	return 0.0
}

// getCPUUtilization 获取 CPU 利用率
func (m *SystemMonitor) getCPUUtilization() float64 {
	return m.getCPUPercent()
}

// round2 四舍五入到2位小数
func round2(val float64) float64 {
	return float64(int(val*100+0.5)) / 100
}

// 全局单例
var (
	systemMonitorInstance *SystemMonitor
	systemMonitorOnce     sync.Once
)

// GetSystemMonitor 获取系统监控器单例
func GetSystemMonitor() *SystemMonitor {
	systemMonitorOnce.Do(func() {
		systemMonitorInstance = &SystemMonitor{}
	})
	return systemMonitorInstance
}

// formatUptime 格式化运行时间
func formatUptime(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, secs)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

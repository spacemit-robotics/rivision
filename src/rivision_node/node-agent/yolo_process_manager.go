// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// YOLOProcessManager YOLO 进程管理器
type YOLOProcessManager struct {
	serviceName string
	processName string
	binaryPath  string
	modelPath   string
	port        int
	enabled     bool
}

// NewYOLOProcessManager 创建 YOLO 进程管理器
func NewYOLOProcessManager(cfg *Settings) *YOLOProcessManager {
	return &YOLOProcessManager{
		serviceName: cfg.YoloServiceName,
		processName: cfg.YoloProcessName,
		binaryPath:  cfg.YoloBinaryPath,
		modelPath:   cfg.YoloModelPath,
		port:        cfg.YoloPort,
		enabled:     cfg.YoloEnabled,
	}
}

// GetProcessInfo 获取 yolo-server 进程信息
func (pm *YOLOProcessManager) GetProcessInfo() *ProcessInfo {
	procs := pm.findProcesses()
	if len(procs) > 0 {
		return procs[0]
	}
	return nil
}

// GetAllProcesses 获取所有 yolo-server 进程
func (pm *YOLOProcessManager) GetAllProcesses() []*ProcessInfo {
	return pm.findProcesses()
}

// IsRunning 检查 yolo-server 是否运行
func (pm *YOLOProcessManager) IsRunning() bool {
	info := pm.GetProcessInfo()
	return info != nil && (info.Status == "running" || info.Status == "sleeping" || info.Status == "S" || info.Status == "R")
}

// IsEnabled 检查 YOLO 是否启用
func (pm *YOLOProcessManager) IsEnabled() bool {
	return pm.enabled
}

// Start 启动 yolo-server 服务
func (pm *YOLOProcessManager) Start() ActionResult {
	if !pm.enabled {
		return ActionResult{
			Success: false,
			Message: "YOLO 服务未启用",
			Action:  "start",
		}
	}

	// 检查是否已运行
	if pm.IsRunning() {
		return ActionResult{
			Success: true,
			Message: "yolo-server 已在运行",
			Action:  "start",
		}
	}

	// 尝试使用 systemctl 启动
	result := pm.systemctlAction("start")
	if result.Success {
		// 等待服务启动
		time.Sleep(2 * time.Second)
		if pm.IsRunning() {
			log.Printf("[YOLOProcessManager] yolo-server 服务已启动")
			return result
		}
	}

	return result
}

// Stop 停止 yolo-server 服务
func (pm *YOLOProcessManager) Stop() ActionResult {
	// 检查是否在运行
	if !pm.IsRunning() {
		return ActionResult{
			Success: true,
			Message: "yolo-server 未在运行",
			Action:  "stop",
		}
	}

	// 尝试使用 systemctl 停止
	result := pm.systemctlAction("stop")
	if result.Success {
		log.Printf("[YOLOProcessManager] yolo-server 服务已停止")
		return result
	}

	// systemctl 失败，尝试直接 kill 进程
	return pm.killProcesses(false)
}

// Restart 重启 yolo-server 服务
func (pm *YOLOProcessManager) Restart() ActionResult {
	// 尝试使用 systemctl 重启
	result := pm.systemctlAction("restart")
	if result.Success {
		// 等待服务重启
		time.Sleep(3 * time.Second)
		if pm.IsRunning() {
			log.Printf("[YOLOProcessManager] yolo-server 服务已重启")
			return result
		}
	}

	// systemctl 失败，手动 stop + start
	stopResult := pm.Stop()
	if !stopResult.Success {
		return stopResult
	}

	time.Sleep(1 * time.Second)
	return pm.Start()
}

// EnsureRunning 确保 YOLO 服务运行（自动启动）
func (pm *YOLOProcessManager) EnsureRunning() bool {
	if !pm.enabled {
		return false
	}

	if pm.IsRunning() {
		return true
	}

	log.Printf("[YOLOProcessManager] yolo-server 未运行，尝试自动启动...")
	result := pm.Start()
	if result.Success {
		log.Printf("[YOLOProcessManager] yolo-server 自动启动成功")
		return true
	}

	log.Printf("[YOLOProcessManager] yolo-server 自动启动失败: %s", result.Message)
	return false
}

// findProcesses 查找匹配的进程
func (pm *YOLOProcessManager) findProcesses() []*ProcessInfo {
	var result []*ProcessInfo

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
	}

	// 获取系统总内存用于计算百分比
	totalMemKB := getTotalMemoryKB()

	// 获取系统启动时间
	bootTime := getBootTime()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// 读取进程名
		commData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(commData))

		if !strings.Contains(comm, pm.processName) {
			continue
		}

		info := &ProcessInfo{
			PID:  pid,
			Name: comm,
		}

		// 读取 /proc/[pid]/stat
		statData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err == nil {
			pm.parseProcStat(string(statData), info, totalMemKB, bootTime)
		}

		// 读取 /proc/[pid]/cmdline
		cmdlineData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err == nil {
			info.Command = strings.ReplaceAll(string(cmdlineData), "\x00", " ")
			info.Command = strings.TrimSpace(info.Command)
		}

		result = append(result, info)
	}

	return result
}

// parseProcStat 解析 /proc/[pid]/stat
func (pm *YOLOProcessManager) parseProcStat(stat string, info *ProcessInfo, totalMemKB uint64, bootTime uint64) {
	idx := strings.LastIndex(stat, ")")
	if idx < 0 {
		return
	}

	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 22 {
		return
	}

	// 状态
	info.Status = fields[0]

	// RSS (第 21 个字段，从 ) 后算起是第 21 个)
	if len(fields) > 21 {
		rssPages, _ := strconv.ParseUint(fields[21], 10, 64)
		info.MemoryMB = float64(rssPages*4096) / 1024 / 1024
		if totalMemKB > 0 {
			info.MemoryPercent = float64(rssPages*4) / float64(totalMemKB) * 100
		}
	}

	// 启动时间 (第 19 个字段)
	if len(fields) > 19 && bootTime > 0 {
		startTicks, _ := strconv.ParseUint(fields[19], 10, 64)
		startTime := bootTime + startTicks/100
		info.UptimeSeconds = float64(time.Now().Unix()) - float64(startTime)
	}
}

// systemctlAction 执行 systemctl 操作
func (pm *YOLOProcessManager) systemctlAction(action string) ActionResult {
	cmd := exec.Command("systemctl", action, pm.serviceName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return ActionResult{
			Success: false,
			Message: fmt.Sprintf("systemctl %s %s 失败: %v, 输出: %s", action, pm.serviceName, err, string(output)),
			Action:  action,
		}
	}

	return ActionResult{
		Success: true,
		Message: fmt.Sprintf("systemctl %s %s 成功", action, pm.serviceName),
		Action:  action,
	}
}

// killProcesses 终止所有 yolo 进程
func (pm *YOLOProcessManager) killProcesses(force bool) ActionResult {
	procs := pm.findProcesses()
	if len(procs) == 0 {
		return ActionResult{
			Success: true,
			Message: "没有找到 yolo-server 进程",
			Action:  "kill",
		}
	}

	var killedPIDs []int
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}

	for _, proc := range procs {
		p, err := os.FindProcess(proc.PID)
		if err != nil {
			continue
		}
		if err := p.Signal(sig); err == nil {
			killedPIDs = append(killedPIDs, proc.PID)
		}
	}

	return ActionResult{
		Success: len(killedPIDs) > 0,
		Message: fmt.Sprintf("已终止 %d 个 yolo-server 进程", len(killedPIDs)),
		Action:  "kill",
		PIDs:    killedPIDs,
	}
}

// 全局单例
var (
	yoloProcessManager     *YOLOProcessManager
	yoloProcessManagerOnce = false
)

// GetYOLOProcessManager 获取 YOLO 进程管理器单例
func GetYOLOProcessManager() *YOLOProcessManager {
	if !yoloProcessManagerOnce && settings != nil {
		yoloProcessManager = NewYOLOProcessManager(settings)
		yoloProcessManagerOnce = true
	}
	return yoloProcessManager
}

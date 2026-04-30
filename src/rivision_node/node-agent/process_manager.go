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
	"sync"
	"syscall"
	"time"
)

// ProcessInfo 进程信息
type ProcessInfo struct {
	PID           int     `json:"pid"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	MemoryMB      float64 `json:"memory_mb"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	Command       string  `json:"command"`
}

// ActionResult 操作结果
type ActionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Action  string `json:"action"`
	PIDs    []int  `json:"pids,omitempty"`
}

// ProcessManager 进程管理器
type ProcessManager struct {
	serviceName string
	processName string
	binaryPath  string
	modelPath   string
	port        int
}

// NewProcessManager 创建进程管理器
func NewProcessManager(cfg *Settings) *ProcessManager {
	return &ProcessManager{
		serviceName: cfg.LlamaServiceName,
		processName: cfg.LlamaProcessName,
		binaryPath:  cfg.LlamaBinaryPath,
		modelPath:   cfg.LlamaModelPath,
		port:        cfg.LlamaPort,
	}
}

// GetProcessInfo 获取 llama.cpp 进程信息
func (pm *ProcessManager) GetProcessInfo() *ProcessInfo {
	procs := pm.findProcesses()
	if len(procs) > 0 {
		return procs[0]
	}
	return nil
}

// GetAllProcesses 获取所有 llama.cpp 进程
func (pm *ProcessManager) GetAllProcesses() []*ProcessInfo {
	return pm.findProcesses()
}

// IsRunning 检查 llama.cpp 是否运行
func (pm *ProcessManager) IsRunning() bool {
	info := pm.GetProcessInfo()
	return info != nil && (info.Status == "running" || info.Status == "sleeping" || info.Status == "S" || info.Status == "R")
}

// Start 启动 llama.cpp 服务
func (pm *ProcessManager) Start() ActionResult {
	// 检查是否已运行
	if pm.IsRunning() {
		return ActionResult{
			Success: false,
			Message: "llama.cpp 已在运行",
			Action:  "start",
		}
	}

	// 尝试使用 systemctl 启动
	result := pm.systemctlAction("start")
	if result.Success {
		// 等待服务启动
		time.Sleep(2 * time.Second)
		if pm.IsRunning() {
			log.Printf("[ProcessManager] llama.cpp 服务已启动")
			return result
		}
	}

	return result
}

// Stop 停止 llama.cpp 服务
func (pm *ProcessManager) Stop() ActionResult {
	// 检查是否在运行
	if !pm.IsRunning() {
		return ActionResult{
			Success: true,
			Message: "llama.cpp 未在运行",
			Action:  "stop",
		}
	}

	// 尝试使用 systemctl 停止
	result := pm.systemctlAction("stop")
	if result.Success {
		log.Printf("[ProcessManager] llama.cpp 服务已停止")
		return result
	}

	// systemctl 失败，尝试直接 kill 进程
	return pm.killProcesses(false)
}

// Restart 重启 llama.cpp 服务
func (pm *ProcessManager) Restart() ActionResult {
	// 尝试使用 systemctl 重启
	result := pm.systemctlAction("restart")
	if result.Success {
		// 等待服务重启
		time.Sleep(3 * time.Second)
		if pm.IsRunning() {
			log.Printf("[ProcessManager] llama.cpp 服务已重启")
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

// EnsureRunning 确保 llama.cpp 服务运行（自动启动）
func (pm *ProcessManager) EnsureRunning() bool {
	if pm.IsRunning() {
		return true
	}

	log.Printf("[ProcessManager] llama.cpp 未运行，尝试自动启动...")
	result := pm.Start()
	if result.Success {
		log.Printf("[ProcessManager] llama.cpp 自动启动成功")
		return true
	}

	log.Printf("[ProcessManager] llama.cpp 自动启动失败: %s", result.Message)
	return false
}

// Kill 终止 llama.cpp 进程
func (pm *ProcessManager) Kill(pid int, force bool) ActionResult {
	if pid > 0 {
		// 终止指定进程
		proc, err := os.FindProcess(pid)
		if err != nil {
			return ActionResult{
				Success: false,
				Message: fmt.Sprintf("进程 %d 不存在", pid),
				Action:  "kill",
			}
		}

		sig := syscall.SIGTERM
		if force {
			sig = syscall.SIGKILL
		}

		err = proc.Signal(sig)
		if err != nil {
			return ActionResult{
				Success: false,
				Message: fmt.Sprintf("终止进程 %d 失败: %v", pid, err),
				Action:  "kill",
			}
		}

		return ActionResult{
			Success: true,
			Message: fmt.Sprintf("已发送信号到进程: [%d]", pid),
			Action:  "kill",
			PIDs:    []int{pid},
		}
	}

	// 终止所有 llama 进程
	return pm.killProcesses(force)
}

// findProcesses 查找匹配的进程
func (pm *ProcessManager) findProcesses() []*ProcessInfo {
	var result []*ProcessInfo

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
	}

	// 获取系统总内存用于计算百分比
	totalMemKB := getTotalMemoryKB()

	// 获取系统启动时间
	bootTime := getBootTime()

	// ★ 支持多个进程名：llama-server (标准) 或 llama-mtmd-cli (NPU模式)
	processNames := []string{pm.processName}
	if pm.processName == "llama-server" {
		processNames = append(processNames, "llama-mtmd-cli", "llama-mtmd")
	}

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

		// 检查是否匹配任一进程名
		matched := false
		for _, name := range processNames {
			if strings.Contains(comm, name) {
				matched = true
				break
			}
		}
		if !matched {
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
func (pm *ProcessManager) parseProcStat(stat string, info *ProcessInfo, totalMemKB uint64, bootTime uint64) {
	// 格式: pid (comm) state utime stime ... starttime ... rss ...
	// 找到最后一个 ')' 的位置，之后的字段才是可靠的
	idx := strings.LastIndex(stat, ")")
	if idx < 0 {
		return
	}

	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 22 {
		return
	}

	// state 是第一个字段
	switch fields[0] {
	case "R":
		info.Status = "running"
	case "S":
		info.Status = "sleeping"
	case "D":
		info.Status = "disk-sleep"
	case "Z":
		info.Status = "zombie"
	case "T":
		info.Status = "stopped"
	default:
		info.Status = fields[0]
	}

	// rss 是第 21 个字段 (0-indexed from after ')')
	// fields[0]=state, fields[1]=ppid, ... fields[21]=rss
	if len(fields) > 21 {
		rssPages, _ := strconv.ParseUint(fields[21], 10, 64)
		pageSize := uint64(os.Getpagesize())
		rssMB := float64(rssPages*pageSize) / (1024 * 1024)
		info.MemoryMB = round2(rssMB)

		if totalMemKB > 0 {
			rssKB := rssPages * pageSize / 1024
			info.MemoryPercent = round2(float64(rssKB) / float64(totalMemKB) * 100.0)
		}
	}

	// starttime 是第 19 个字段 (0-indexed from after ')')
	if len(fields) > 19 && bootTime > 0 {
		startTicks, _ := strconv.ParseUint(fields[19], 10, 64)
		clkTck := uint64(100) // sysconf(_SC_CLK_TCK), 通常是 100
		startTimeSec := bootTime + startTicks/clkTck
		info.UptimeSeconds = round2(float64(time.Now().Unix()) - float64(startTimeSec))
	}
}

// getTotalMemoryKB 获取系统总内存 (KB)
func getTotalMemoryKB() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				return val
			}
		}
	}
	return 0
}

// getBootTime 获取系统启动时间 (unix timestamp)
func getBootTime() uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				return val
			}
		}
	}
	return 0
}

// systemctlAction 执行 systemctl 操作
func (pm *ProcessManager) systemctlAction(action string) ActionResult {
	cmd := exec.Command("sudo", "systemctl", action, pm.serviceName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return ActionResult{
			Success: false,
			Message: fmt.Sprintf("systemctl %s 失败: %s", action, strings.TrimSpace(string(output))),
			Action:  action,
		}
	}

	return ActionResult{
		Success: true,
		Message: fmt.Sprintf("systemctl %s 成功", action),
		Action:  action,
	}
}

// killProcesses 终止所有 llama.cpp 进程
func (pm *ProcessManager) killProcesses(force bool) ActionResult {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}

	procs := pm.findProcesses()
	var killed []int

	for _, p := range procs {
		proc, err := os.FindProcess(p.PID)
		if err != nil {
			continue
		}
		err = proc.Signal(sig)
		if err == nil {
			killed = append(killed, p.PID)
		}
	}

	if len(killed) > 0 {
		time.Sleep(1 * time.Second)
		return ActionResult{
			Success: true,
			Message: fmt.Sprintf("已终止 %d 个进程", len(killed)),
			Action:  "kill",
			PIDs:    killed,
		}
	}

	return ActionResult{
		Success: true,
		Message: "没有找到运行中的进程",
		Action:  "kill",
		PIDs:    []int{},
	}
}

// 全局单例
var (
	processManagerInstance *ProcessManager
	processManagerOnce     sync.Once
)

// GetProcessManager 获取进程管理器单例
func GetProcessManager() *ProcessManager {
	processManagerOnce.Do(func() {
		processManagerInstance = NewProcessManager(settings)
	})
	return processManagerInstance
}

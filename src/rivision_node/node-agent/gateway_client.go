// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GatewayClient Gateway 客户端
type GatewayClient struct {
	gatewayURL        string
	nodeID            string
	llamaPort         int
	agentPort         int
	registrationToken string
	heartbeatInterval int
	host              string
	running           bool
	stopCh            chan struct{}
	// ★ 状态标志，避免重复打印相同警告
	llamaWarnLogged   bool
	yoloWarnLogged    bool
	// ★ 启动时间，用于启动宽限期
	startTime         time.Time
}

// NewGatewayClient 创建 Gateway 客户端
func NewGatewayClient(cfg *Settings) *GatewayClient {
	gc := &GatewayClient{
		gatewayURL:        strings.TrimRight(cfg.GatewayURL, "/"),
		nodeID:            cfg.NodeID,
		llamaPort:         cfg.LlamaPort,
		agentPort:         cfg.AgentPort,
		registrationToken: cfg.RegistrationToken,
		heartbeatInterval: cfg.HeartbeatInterval,
		stopCh:            make(chan struct{}),
		startTime:         time.Now(),
	}

	// 获取本机 IP（带重试，等待网络就绪）
	if cfg.NodeHost == "auto" || cfg.NodeHost == "" {
		gc.host = gc.getLocalIPWithRetry()
	} else {
		gc.host = cfg.NodeHost
	}

	log.Printf("[GatewayClient] 初始化: gateway=%s, node=%s, host=%s", gc.gatewayURL, gc.nodeID, gc.host)
	return gc
}

// getLocalIPWithRetry 带重试的 IP 获取（等待网络就绪）
func (gc *GatewayClient) getLocalIPWithRetry() string {
	maxRetries := 10
	retryInterval := 3 * time.Second

	for i := 0; i < maxRetries; i++ {
		ip := gc.getLocalIP()
		if ip != "" && ip != "127.0.0.1" {
			return ip
		}

		if i < maxRetries-1 {
			log.Printf("[GatewayClient] 网络未就绪，等待 %v 后重试 (%d/%d)...", retryInterval, i+1, maxRetries)
			time.Sleep(retryInterval)
		}
	}

	log.Printf("⚠️ 网络等待超时，使用 127.0.0.1（将在心跳中修复）")
	return "127.0.0.1"
}

// getLocalIP 获取本机 IP 地址
func (gc *GatewayClient) getLocalIP() string {
	// 方法1：连接 Gateway 获取 IP (最可靠)
	parsed, err := url.Parse(gc.gatewayURL)
	if err == nil {
		gatewayHost := parsed.Hostname()
		gatewayPort := parsed.Port()
		if gatewayPort == "" {
			gatewayPort = "80"
		}

		// 使用 TCP 而非 UDP，更可靠
		conn, err := net.DialTimeout("tcp", gatewayHost+":"+gatewayPort, 5*time.Second)
		if err == nil {
			defer conn.Close()
			localAddr := conn.LocalAddr().(*net.TCPAddr)
			ip := localAddr.IP.String()
			if ip != "" && ip != "127.0.0.1" && !strings.HasPrefix(ip, "::") {
				log.Printf("通过 Gateway TCP 连接获取本机 IP: %s", ip)
				return ip
			}
		} else {
			log.Printf("无法连接 Gateway %s:%s: %v", gatewayHost, gatewayPort, err)
		}
	}

	// 方法2：遍历网卡获取非 lo 接口的 IP
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			// 跳过 lo, docker, veth 等虚拟接口
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "veth") ||
				strings.HasPrefix(iface.Name, "br-") || strings.HasPrefix(iface.Name, "virbr") {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				// 只要 IPv4，排除 127.x
				if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
					log.Printf("通过网卡 %s 获取本机 IP: %s", iface.Name, ip.String())
					return ip.String()
				}
			}
		}
	}

	// 方法3：hostname -I
	out, err := exec.Command("hostname", "-I").Output()
	if err == nil {
		ips := strings.Fields(strings.TrimSpace(string(out)))
		for _, ip := range ips {
			if ip != "" && !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
				log.Printf("通过 hostname -I 获取本机 IP: %s", ip)
				return ip
			}
		}
	}

	// 方法4：连接外网获取 IP
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 3*time.Second)
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		ip := localAddr.IP.String()
		if ip != "" && ip != "127.0.0.1" {
			log.Printf("通过外网连接获取本机 IP: %s", ip)
			return ip
		}
	}

	log.Printf("⚠️ 无法自动获取本机 IP，请在 .env 中设置 NODE_HOST=<实际IP>")
	return "127.0.0.1"
}

// Start 启动客户端
func (gc *GatewayClient) Start() {
	gc.running = true

	// 注册节点
	gc.register()

	// 启动心跳循环
	ticker := time.NewTicker(time.Duration(gc.heartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-gc.stopCh:
			return
		case <-ticker.C:
			gc.heartbeat()
		}
	}
}

// Stop 停止客户端
func (gc *GatewayClient) Stop() {
	gc.running = false
	// 注销节点
	gc.unregister()
	close(gc.stopCh)
}

// register 向 Gateway 注册节点
func (gc *GatewayClient) register() bool {
	maxRetries := 5
	retryDelay := 5 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		token := gc.getRegistrationToken()

		payload := map[string]interface{}{
			"id":                 gc.nodeID,
			"host":               gc.host,
			"port":               gc.llamaPort,
			"agent_port":         gc.agentPort,
			"yolo_port":          settings.YoloPort,
			"weight":             1,
			"tags":               []string{"auto-registered"},
			"registration_token": token,
		}

		body, _ := json.Marshal(payload)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(
			gc.gatewayURL+"/api/v1/nodes/register",
			"application/json",
			bytes.NewReader(body),
		)

		if err != nil {
			log.Printf("节点注册异常 (尝试 %d/%d): %v", attempt+1, maxRetries, err)
		} else {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == 200 {
				log.Printf("节点注册成功: %s (%s:%d)", gc.nodeID, gc.host, gc.llamaPort)
				return true
			}
			log.Printf("节点注册失败: %d - %s", resp.StatusCode, string(respBody))
		}

		// 非最后一次尝试，等待后重试
		if attempt < maxRetries-1 {
			log.Printf("等待 %v 后重试注册...", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	log.Printf("节点注册失败，已达最大重试次数 (%d)", maxRetries)
	return false
}

// unregister 从 Gateway 注销节点
func (gc *GatewayClient) unregister() {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("DELETE", gc.gatewayURL+"/api/v1/nodes/"+gc.nodeID, nil)
	if err != nil {
		log.Printf("节点注销异常: %v", err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("节点注销异常: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("节点注销成功: %s", gc.nodeID)
	} else {
		log.Printf("节点注销失败: %d", resp.StatusCode)
	}
}

// heartbeat 发送心跳
func (gc *GatewayClient) heartbeat() {
	// ★ 修复：如果当前 IP 是 127.0.0.1（启动时网络未就绪），尝试重新获取
	if gc.host == "127.0.0.1" {
		newIP := gc.getLocalIP()
		if newIP != "127.0.0.1" {
			log.Printf("[GatewayClient] IP 更新: %s -> %s，触发重新注册", gc.host, newIP)
			gc.host = newIP
			// 重新注册以更新 Gateway 中的节点地址
			go gc.register()
		}
	}

	pm := GetProcessManager()
	sm := GetSystemMonitor()

	// 获取系统状态
	systemStats := sm.GetStats()

	// 获取加速器状态
	acceleratorStats := sm.GetAcceleratorStats()
	gpuStats := sm.GetGPUStats()

	// ★ 根据配置决定是否检测 llama 服务
	var processInfo *ProcessInfo
	llamaHealthy := false
	llamaBusy := false

	if settings.LlamaEnabled {
		// 1. 检测 llama.cpp 实际状态
		processInfo = pm.GetProcessInfo()

		if processInfo != nil && processInfo.PID > 0 {
			// 进程存在，尝试 /health 检查
			llamaHealthy = true
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", gc.llamaPort))
			if err != nil {
				llamaBusy = true
				LogDebug("llama /health 超时，进程存在，视为繁忙")
			} else {
				resp.Body.Close()
				if resp.StatusCode != 200 {
					llamaBusy = true
				}
			}
		} else {
			// 进程不存在，尝试直接检查端口（可能是 systemd 启动的）
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", gc.llamaPort))
			if err == nil {
				defer resp.Body.Close()
				llamaHealthy = resp.StatusCode == 200
				if llamaHealthy && gc.llamaWarnLogged {
					log.Printf("[node-agent] llama 服务已恢复")
					gc.llamaWarnLogged = false
				}
			} else {
				// ★ 启动宽限期：前 60 秒内不打印警告（等待模型加载）
				gracePeriod := 60 * time.Second
				if time.Since(gc.startTime) < gracePeriod {
					LogDebug("llama 服务启动中，等待模型加载...")
				} else if !gc.llamaWarnLogged {
					// 超过宽限期后，仅首次打印警告
					log.Printf("[node-agent] llama 服务不可用 (进程未找到且 /health 无响应)")
					gc.llamaWarnLogged = true
				}
			}
		}
	}

	// 构建资源状态
	resources := map[string]interface{}{}
	if systemStats != nil {
		resources["memory_percent"] = systemStats.MemoryPercent
		resources["cpu_percent"] = systemStats.CPUPercent
	}

	// 加速器资源
	if acceleratorStats != nil {
		accType, _ := acceleratorStats["accelerator_type"].(string)
		resources["accelerator_type"] = accType

		accMemUsed, _ := acceleratorStats["memory_used_mb"].(float64)
		accMemTotal, _ := acceleratorStats["memory_total_mb"].(float64)
		if accMemTotal == 0 {
			accMemTotal = 1
		}
		if accMemTotal > 0 {
			resources["accelerator_memory_percent"] = accMemUsed / accMemTotal * 100
		}

		if accType == "gpu" {
			resources["gpu_memory_percent"] = resources["accelerator_memory_percent"]
			if gpuUtil, ok := acceleratorStats["gpu_utilization"]; ok {
				resources["gpu_utilization"] = gpuUtil
			}
		}
	} else if gpuStats != nil {
		gpuMemUsed, _ := gpuStats["memory_used_mb"].(float64)
		gpuMemTotal, _ := gpuStats["memory_total_mb"].(float64)
		if gpuMemTotal == 0 {
			gpuMemTotal = 1
		}
		if gpuMemTotal > 0 {
			resources["gpu_memory_percent"] = gpuMemUsed / gpuMemTotal * 100
		}
		if gpuUtil, ok := gpuStats["gpu_utilization"]; ok {
			resources["gpu_utilization"] = gpuUtil
		}
	}

	// 构建心跳数据
	// ★ 修复: load 直接使用 CPUPercent (0-100)，不再除以100
	// 之前 load = CPUPercent/100 导致显示 1% 实际是 100%
	var load float64
	if systemStats != nil {
		load = systemStats.CPUPercent
		if load > 100.0 {
			load = 100.0
		}
	}

	// ★ 构建 llama 状态 (包含 enabled 标志)
	llamaStatus := map[string]interface{}{
		"enabled":     settings.LlamaEnabled,
		"running":     settings.LlamaEnabled && (processInfo != nil || llamaHealthy),
		"healthy":     llamaHealthy,
		"busy":        llamaBusy,
		"pid":         nil,
		"cpu_percent": float64(0),
		"memory_mb":   float64(0),
	}
	if processInfo != nil {
		llamaStatus["pid"] = processInfo.PID
		llamaStatus["cpu_percent"] = processInfo.CPUPercent
		llamaStatus["memory_mb"] = processInfo.MemoryMB
	}

	var systemStatsMap map[string]interface{}
	if systemStats != nil {
		systemStatsMap = systemStats.ToMap()
	}

	if gpuStats == nil {
		gpuStats = map[string]interface{}{}
	}

	// ★ 根据配置决定是否检测 YOLO 服务
	yoloStatus := map[string]interface{}{
		"enabled": settings.YoloEnabled,
		"healthy": false,
		"running": false,
	}

	if settings.YoloEnabled {
		// 检测 YOLO 实际状态（优先 HTTP 检测，进程检测作为补充）
		yoloPM := GetYOLOProcessManager()
		yoloProc := yoloPM.GetProcessInfo()

		// 通过 HTTP 检测 YOLO 服务健康状态
		yoloSvc := GetYOLOService()
		if yoloSvc != nil {
			yoloStatus = yoloSvc.GetStatus()
			yoloStatus["enabled"] = settings.YoloEnabled
		}

		// running = 进程存在 或 服务响应健康
		yoloHealthy, _ := yoloStatus["healthy"].(bool)
		yoloStatus["running"] = yoloProc != nil || yoloHealthy

		if yoloProc != nil {
			yoloStatus["pid"] = yoloProc.PID
			yoloStatus["cpu_percent"] = yoloProc.CPUPercent
			yoloStatus["memory_mb"] = yoloProc.MemoryMB
		}
	}

	heartbeatData := map[string]interface{}{
		"host":         gc.host, // ★ 携带当前 host，Gateway 可检测 IP 变化
		"connections":  0,
		"load":         round2(load),
		"llama_status": llamaStatus,
		"yolo_status":  yoloStatus,
		"system_stats": systemStatsMap,
		"resources":    resources,
		"gpu_stats":    gpuStats,
	}

	// 发送心跳
	body, _ := json.Marshal(heartbeatData)
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("PUT",
		gc.gatewayURL+"/api/v1/nodes/"+gc.nodeID+"/heartbeat",
		bytes.NewReader(body),
	)
	if err != nil {
		log.Printf("心跳发送异常: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("心跳发送异常: %v", err)
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		LogDebug("心跳发送成功: %s", gc.nodeID)
	case 404:
		log.Printf("节点未注册 (404)，尝试重新注册...")
		gc.register()
	default:
		log.Printf("心跳发送失败: %d", resp.StatusCode)
		if resp.StatusCode == 422 {
			respBody, _ := io.ReadAll(resp.Body)
			log.Printf("422验证错误详情: %s", string(respBody))
		}
	}
}

// getRegistrationToken 获取注册Token
func (gc *GatewayClient) getRegistrationToken() string {
	// 1. 优先使用构造函数传入的Token
	if gc.registrationToken != "" {
		return gc.registrationToken
	}

	// 2. 从配置读取Token
	if settings.RegistrationToken != "" {
		return settings.RegistrationToken
	}

	// 3. 从环境变量读取Token
	envToken := os.Getenv("REGISTRATION_TOKEN")
	if envToken != "" {
		return envToken
	}

	// 4. 如果没有配置Token，抛出致命错误（与Python版一致）
	log.Fatalf("未配置注册Token。请设置以下任一项:\n" +
		"1. 环境变量: export REGISTRATION_TOKEN=your_token\n" +
		"2. .env文件: REGISTRATION_TOKEN=your_token")
	return ""
}

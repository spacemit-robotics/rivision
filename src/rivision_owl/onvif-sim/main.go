// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// ONVIF Simulator - 虚拟 ONVIF 摄像头模拟器
//
// 用于测试 OWL 等 ONVIF 客户端，无需真实摄像头硬件。
// 基于 github.com/0x524a/onvif-go/server 实现。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/0x524a/onvif-go/server"
	"github.com/sirupsen/logrus"
)

var version = "1.1.0"

func main() {
	// 命令行参数
	configFile := flag.String("c", "", "配置文件路径 (YAML)")
	host := flag.String("host", "0.0.0.0", "监听地址")
	port := flag.Int("port", 15188, "监听端口")
	username := flag.String("username", "admin", "认证用户名")
	password := flag.String("password", "admin", "认证密码")
	profiles := flag.Int("profiles", 3, "摄像头配置文件数量 (1-10)")
	ptz := flag.Bool("ptz", true, "启用 PTZ 云台控制")
	imaging := flag.Bool("imaging", true, "启用 Imaging 图像控制")
	events := flag.Bool("events", true, "启用 Event 事件服务")
	discovery := flag.Bool("discovery", true, "启用 WS-Discovery 自动发现")
	showVersion := flag.Bool("version", false, "显示版本")
	verbose := flag.Bool("verbose", false, "详细日志")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ONVIF Simulator - 虚拟 ONVIF 摄像头\n\n")
		fmt.Fprintf(os.Stderr, "用法: %s [选项]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s                              # 默认配置启动\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -c config.yaml               # 使用配置文件\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -port 15188 -profiles 5       # 自定义端口和配置数\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -discovery=false              # 禁用 WS-Discovery\n", os.Args[0])
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("onvif-sim version %s\n", version)
		os.Exit(0)
	}

	// 配置来源: 配置文件 > 命令行参数 > 默认值
	var config *server.Config
	var localIP string

	// RTSP 服务器（用于 file:// 源）
	var rtspServer *RTSPServer
	rtspPort := 18555 // 默认 RTSP 端口 (避免与 rivision-cli 18554 冲突)

	if *configFile != "" {
		cfg, err := LoadConfig(*configFile)
		if err != nil {
			log.Fatalf("加载配置文件失败: %v", err)
		}
		*host = cfg.Server.Host
		*port = cfg.Server.Port
		rtspPort = cfg.Server.RTSPPort
		*username = cfg.Auth.Username
		*password = cfg.Auth.Password
		*verbose = cfg.Server.Verbose
		*ptz = cfg.Features.PTZ
		*imaging = cfg.Features.Imaging
		localIP = detectLocalIP()

		// 检查是否有 file:// 源，需要启动 RTSP 服务器
		hasFileSource := false
		for _, p := range cfg.Profiles {
			if strings.HasPrefix(p.Source, "file://") || 
			   (p.Source != "" && !strings.HasPrefix(p.Source, "rtsp://") && p.Source != "test") {
				hasFileSource = true
				break
			}
		}

		if hasFileSource {
			// 创建 RTSP 服务器
			extIP := localIP
			if extIP == "" {
				extIP = "127.0.0.1"
			}
			rtspServer = NewRTSPServer(rtspPort, extIP)

			// 为每个 file:// 源添加流
			for i, p := range cfg.Profiles {
				if p.Source != "" {
					streamName := fmt.Sprintf("profile_%d", i)
					if _, err := rtspServer.AddStream(streamName, p.Source); err != nil {
						log.Printf("警告: 添加流 %s 失败: %v", p.Name, err)
					}
				}
			}
		}

		// 构建配置，使用 RTSP 服务器地址
		config = buildConfigFromFileWithRTSP(cfg, rtspServer, localIP, rtspPort)
		fmt.Printf("📄 使用配置文件: %s\n", *configFile)
	} else {
		if *profiles < 1 || *profiles > 10 {
			log.Fatal("profiles 必须在 1-10 之间")
		}
		config = buildConfig(*host, *port, *username, *password, *profiles, *ptz, *imaging, *events)
		localIP = detectLocalIP()
	}

	srv, err := server.New(config)
	if err != nil {
		log.Fatalf("创建服务器失败: %v", err)
	}

	// 打印启动信息
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║           🎥  ONVIF Simulator (onvif-sim)  🎥            ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Printf("\n  地址: http://%s:%d/onvif/device_service\n", *host, *port)
	fmt.Printf("  认证: %s / %s\n", *username, *password)
	fmt.Printf("  配置: %d 个摄像头配置文件\n", len(config.Profiles))
	fmt.Printf("  PTZ:  %v | Imaging: %v | Events: %v\n", *ptz, *imaging, *events)
	fmt.Printf("  WS-Discovery: %v\n", *discovery)
	if localIP != "" && !*discovery {
		fmt.Printf("\n  💡 在 OWL 中手动添加此设备:\n")
		fmt.Printf("     IP: %s, 端口: %d, 用户: %s, 密码: %s\n\n", localIP, *port, *username, *password)
	}

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	// 信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 启动 WS-Discovery 响应器
	var discoveryResponder *server.DiscoveryResponder
	if *discovery {
		var err error
		discoveryResponder, err = server.NewDiscoveryResponder(srv, nil)
		if err != nil {
			log.Printf("警告: 无法创建 Discovery 响应器: %v", err)
		} else {
			if err := discoveryResponder.Start(ctx); err != nil {
				log.Printf("警告: 无法启动 Discovery 响应器: %v", err)
			} else {
				fmt.Printf("\n  💡 设备可被自动发现 (WS-Discovery)\n")
				fmt.Printf("     OWL Discovery 页面将自动发现此设备\n\n")
			}
		}
	}

	// 启动 RTSP 服务器（如果有 file:// 源）
	if rtspServer != nil && len(rtspServer.Streams) > 0 {
		if err := rtspServer.Start(); err != nil {
			log.Fatalf("RTSP 服务器启动失败: %v", err)
		} else {
			fmt.Printf("\n  📹 RTSP 服务器已启动 (端口 %d)\n", rtspPort)
			for name, stream := range rtspServer.Streams {
				fmt.Printf("     - %s: %s\n", name, stream.FullURI)
			}
		}
	}

	go func() {
		if err := srv.Start(ctx); err != nil {
			log.Printf("服务器错误: %v", err)
			cancel()
		}
	}()

	<-sigChan
	fmt.Println("\n🛑 收到停止信号，正在关闭...")
	cancel()
	if discoveryResponder != nil {
		_ = discoveryResponder.Stop()
	}
	if rtspServer != nil {
		rtspServer.Stop()
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Println("✅ 服务器已停止")
}

func buildConfig(host string, port int, username, password string,
	numProfiles int, ptz, imaging, events bool) *server.Config {

	config := &server.Config{
		Host:     host,
		Port:     port,
		BasePath: "/onvif",
		Timeout:  30 * time.Second,
		DeviceInfo: server.DeviceInfo{
			Manufacturer:    "RiVision",
			Model:           "ONVIF-Sim Virtual Camera",
			FirmwareVersion: version,
			SerialNumber:    "ONVIF-SIM-001",
			HardwareID:      "HW-ONVIF-SIM",
		},
		Username:       username,
		Password:       password,
		SupportPTZ:     ptz,
		SupportImaging: imaging,
		SupportEvents:  events,
		Profiles:       make([]server.ProfileConfig, numProfiles),
	}

	// 预定义配置模板
	templates := []struct {
		name      string
		width     int
		height    int
		framerate int
		bitrate   int
		hasPTZ    bool
	}{
		{"主摄像头 - 高清", 1920, 1080, 30, 4096, true},
		{"广角摄像头", 1280, 720, 30, 2048, false},
		{"长焦摄像头", 1920, 1080, 25, 6144, true},
		{"夜视摄像头", 1920, 1080, 30, 4096, false},
		{"4K 超清摄像头", 3840, 2160, 30, 16384, true},
	}

	for i := 0; i < numProfiles; i++ {
		t := templates[i%len(templates)]

		profile := server.ProfileConfig{
			Token: fmt.Sprintf("profile_%d", i),
			Name:  t.name,
			VideoSource: server.VideoSourceConfig{
				Token:      fmt.Sprintf("video_source_%d", i),
				Name:       t.name,
				Resolution: server.Resolution{Width: t.width, Height: t.height},
				Framerate:  t.framerate,
				Bounds:     server.Bounds{X: 0, Y: 0, Width: t.width, Height: t.height},
			},
			VideoEncoder: server.VideoEncoderConfig{
				Encoding:   "H264",
				Resolution: server.Resolution{Width: t.width, Height: t.height},
				Quality:    80,
				Framerate:  t.framerate,
				Bitrate:    t.bitrate,
				GovLength:  t.framerate,
			},
			Snapshot: server.SnapshotConfig{
				Enabled:    true,
				Resolution: server.Resolution{Width: t.width, Height: t.height},
				Quality:    85,
			},
		}

		if ptz && t.hasPTZ {
			profile.PTZ = &server.PTZConfig{
				NodeToken:          fmt.Sprintf("ptz_node_%d", i),
				PanRange:           server.Range{Min: -180, Max: 180},
				TiltRange:          server.Range{Min: -90, Max: 90},
				ZoomRange:          server.Range{Min: 0, Max: 1},
				DefaultSpeed:       server.PTZSpeed{Pan: 0.5, Tilt: 0.5, Zoom: 0.5},
				SupportsContinuous: true,
				SupportsAbsolute:   true,
				SupportsRelative:   true,
				Presets: []server.Preset{
					{Token: fmt.Sprintf("preset_%d_home", i), Name: "Home",
						Position: server.PTZPosition{Pan: 0, Tilt: 0, Zoom: 0}},
				},
			}
		}

		config.Profiles[i] = profile
	}

	return config
}

// buildConfigFromFile 从配置文件构建服务器配置
func buildConfigFromFile(cfg *Config) *server.Config {
	config := &server.Config{
		Host:     cfg.Server.Host,
		Port:     cfg.Server.Port,
		BasePath: "/onvif",
		Timeout:  30 * time.Second,
		DeviceInfo: server.DeviceInfo{
			Manufacturer:    cfg.Device.Manufacturer,
			Model:           cfg.Device.Model,
			FirmwareVersion: version,
			SerialNumber:    cfg.Device.Serial,
			HardwareID:      "HW-" + cfg.Device.Serial,
		},
		Username:       cfg.Auth.Username,
		Password:       cfg.Auth.Password,
		SupportPTZ:     cfg.Features.PTZ,
		SupportImaging: cfg.Features.Imaging,
		SupportEvents:  false,
		Profiles:       make([]server.ProfileConfig, len(cfg.Profiles)),
	}

	for i, p := range cfg.Profiles {
		profile := server.ProfileConfig{
			Token:     fmt.Sprintf("profile_%d", i),
			Name:      p.Name,
			StreamURI: p.Source, // Use source from config file
			VideoSource: server.VideoSourceConfig{
				Token:      fmt.Sprintf("video_source_%d", i),
				Name:       p.Name,
				Resolution: server.Resolution{Width: p.Width, Height: p.Height},
				Framerate:  p.Framerate,
				Bounds:     server.Bounds{X: 0, Y: 0, Width: p.Width, Height: p.Height},
			},
			VideoEncoder: server.VideoEncoderConfig{
				Encoding:   "H264",
				Resolution: server.Resolution{Width: p.Width, Height: p.Height},
				Quality:    80,
				Framerate:  p.Framerate,
				Bitrate:    p.Bitrate,
				GovLength:  p.Framerate,
			},
			Snapshot: server.SnapshotConfig{
				Enabled:    true,
				Resolution: server.Resolution{Width: p.Width, Height: p.Height},
				Quality:    85,
			},
		}

		if cfg.Features.PTZ && p.PTZ {
			profile.PTZ = &server.PTZConfig{
				NodeToken:          fmt.Sprintf("ptz_node_%d", i),
				PanRange:           server.Range{Min: -180, Max: 180},
				TiltRange:          server.Range{Min: -90, Max: 90},
				ZoomRange:          server.Range{Min: 0, Max: 1},
				DefaultSpeed:       server.PTZSpeed{Pan: 0.5, Tilt: 0.5, Zoom: 0.5},
				SupportsContinuous: true,
				SupportsAbsolute:   true,
				SupportsRelative:   true,
				Presets: []server.Preset{
					{Token: fmt.Sprintf("preset_%d_home", i), Name: "Home",
						Position: server.PTZPosition{Pan: 0, Tilt: 0, Zoom: 0}},
				},
			}
		}

		config.Profiles[i] = profile
	}

	return config
}

// buildConfigFromFileWithRTSP 从配置文件构建服务器配置，使用 RTSP 服务器地址
func buildConfigFromFileWithRTSP(cfg *Config, rtspServer *RTSPServer, localIP string, rtspPort int) *server.Config {
	config := &server.Config{
		Host:     cfg.Server.Host,
		Port:     cfg.Server.Port,
		BasePath: "/onvif",
		Timeout:  30 * time.Second,
		DeviceInfo: server.DeviceInfo{
			Manufacturer:    cfg.Device.Manufacturer,
			Model:           cfg.Device.Model,
			FirmwareVersion: version,
			SerialNumber:    cfg.Device.Serial,
			HardwareID:      "HW-" + cfg.Device.Serial,
		},
		Username:       cfg.Auth.Username,
		Password:       cfg.Auth.Password,
		SupportPTZ:     cfg.Features.PTZ,
		SupportImaging: cfg.Features.Imaging,
		SupportEvents:  false,
		Profiles:       make([]server.ProfileConfig, len(cfg.Profiles)),
	}

	// 确定外部 IP
	extIP := localIP
	if extIP == "" {
		extIP = "127.0.0.1"
	}

	for i, p := range cfg.Profiles {
		// 确定 StreamURI
		var streamURI string
		streamName := fmt.Sprintf("profile_%d", i)

		if p.Source == "" {
			// 无视频源，使用默认
			streamURI = fmt.Sprintf("rtsp://%s:8554/stream%d", extIP, i)
		} else if strings.HasPrefix(p.Source, "rtsp://") {
			// 已经是 RTSP 地址，直接使用
			streamURI = p.Source
		} else if rtspServer != nil {
			// file:// 或本地文件，使用内置 RTSP 服务器地址
			streamURI = rtspServer.GetStreamURI(streamName)
			if streamURI == "" {
				streamURI = fmt.Sprintf("rtsp://%s:%d/%s", extIP, rtspPort, streamName)
			}
		} else {
			// 无 RTSP 服务器，构建默认地址
			streamURI = fmt.Sprintf("rtsp://%s:%d/%s", extIP, rtspPort, streamName)
		}

		profile := server.ProfileConfig{
			Token:     streamName,
			Name:      p.Name,
			StreamURI: streamURI,
			VideoSource: server.VideoSourceConfig{
				Token:      fmt.Sprintf("video_source_%d", i),
				Name:       p.Name,
				Resolution: server.Resolution{Width: p.Width, Height: p.Height},
				Framerate:  p.Framerate,
				Bounds:     server.Bounds{X: 0, Y: 0, Width: p.Width, Height: p.Height},
			},
			VideoEncoder: server.VideoEncoderConfig{
				Encoding:   "H264",
				Resolution: server.Resolution{Width: p.Width, Height: p.Height},
				Quality:    80,
				Framerate:  p.Framerate,
				Bitrate:    p.Bitrate,
				GovLength:  p.Framerate,
			},
			Snapshot: server.SnapshotConfig{
				Enabled:    true,
				Resolution: server.Resolution{Width: p.Width, Height: p.Height},
				Quality:    85,
			},
		}

		if cfg.Features.PTZ && p.PTZ {
			profile.PTZ = &server.PTZConfig{
				NodeToken:          fmt.Sprintf("ptz_node_%d", i),
				PanRange:           server.Range{Min: -180, Max: 180},
				TiltRange:          server.Range{Min: -90, Max: 90},
				ZoomRange:          server.Range{Min: 0, Max: 1},
				DefaultSpeed:       server.PTZSpeed{Pan: 0.5, Tilt: 0.5, Zoom: 0.5},
				SupportsContinuous: true,
				SupportsAbsolute:   true,
				SupportsRelative:   true,
				Presets: []server.Preset{
					{Token: fmt.Sprintf("preset_%d_home", i), Name: "Home",
						Position: server.PTZPosition{Pan: 0, Tilt: 0, Zoom: 0}},
				},
			}
		}

		config.Profiles[i] = profile
	}

	return config
}

// detectLocalIP 检测本机物理网卡 IP
func detectLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		// 跳过虚拟网卡
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := iface.Name
		// 只匹配物理网卡: eth*, enp*, wlan*, wlp*, ens*
		if len(name) < 3 {
			continue
		}
		prefix := name[:3]
		if prefix != "eth" && prefix != "enp" && prefix != "wla" && prefix != "wlp" && prefix != "ens" {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

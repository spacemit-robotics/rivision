// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// RTSP Server 模块 - 使用 mediamtx 作为 RTSP 服务器，FFmpeg 推流
package main

import (
	"context"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// RTSPServer 管理多路 RTSP 流
type RTSPServer struct {
	Port       int                    // RTSP 端口
	Host       string                 // 外部访问 IP
	Streams    map[string]*RTSPStream // 流名 -> 流配置
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	mediamtx   *exec.Cmd // mediamtx 进程
	mtxCfgFile string
	mtxStdout  bytes.Buffer
	mtxStderr  bytes.Buffer
	mtxExitCh  chan error
}

// RTSPStream 单路视频流
type RTSPStream struct {
	Name     string    // 流名 (如 profile_0)
	Source   string    // 视频源 (file:///path/to/video.mp4 或 rtsp://...)
	RTSPPath string    // RTSP 路径 (/profile_0)
	FullURI  string    // 完整 RTSP URI
	cmd      *exec.Cmd // FFmpeg 进程
	running  bool
}

// NewRTSPServer 创建 RTSP 服务器
func NewRTSPServer(port int, host string) *RTSPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &RTSPServer{
		Port:    port,
		Host:    host,
		Streams: make(map[string]*RTSPStream),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// AddStream 添加一路视频流
func (s *RTSPServer) AddStream(name, source string) (*RTSPStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rtspPath := "/" + name
	fullURI := fmt.Sprintf("rtsp://%s:%d%s", s.Host, s.Port, rtspPath)

	stream := &RTSPStream{
		Name:     name,
		Source:   source,
		RTSPPath: rtspPath,
		FullURI:  fullURI,
	}

	s.Streams[name] = stream
	return stream, nil
}

// GetStreamURI 获取流的 RTSP 地址
func (s *RTSPServer) GetStreamURI(profileToken string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stream, ok := s.Streams[profileToken]; ok {
		return stream.FullURI
	}
	return ""
}

// findMediamtx 查找 mediamtx 可执行文件
func findMediamtx() string {
	// 优先查找同目录下的 mediamtx
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	
	candidates := []string{
		filepath.Join(exeDir, "mediamtx"),
		"/opt/rivision/rivision_owl/bin/mediamtx",
		"/usr/local/bin/mediamtx",
		"/usr/bin/mediamtx",
	}
	
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	// 尝试 PATH
	if path, err := exec.LookPath("mediamtx"); err == nil {
		return path
	}
	
	return ""
}

// Start 启动 RTSP 服务器和所有流
func (s *RTSPServer) Start() error {
	// 查找 mediamtx
	mediamtxPath := findMediamtx()
	if mediamtxPath == "" {
		logrus.Warn("未找到 mediamtx，RTSP 服务器功能不可用")
		logrus.Warn("请将 mediamtx 放置到 bin/ 目录或安装到系统路径")
		return fmt.Errorf("未找到 mediamtx")
	}

	// 使用显式配置文件，避免 mediamtx 默认端口冲突 (如 UDP 8000, TCP 1935 等)
	// mediamtx v1.9.3 的 RTSP 传输协议字段为 protocols（不是 rtspTransports）
	// 这里仅启用 TCP，避免占用 UDP 端口。
	cfg := fmt.Sprintf(`logLevel: warn

rtsp: yes
protocols: [tcp]
rtspAddress: :%d

paths:
  all: {}

rtmp: no
hls: no
webrtc: no
srt: no
api: no
metrics: no
pprof: no
`, s.Port)

	f, err := os.CreateTemp("", "onvif-sim-mediamtx-*.yml")
	if err != nil {
		return fmt.Errorf("创建 mediamtx 配置文件失败: %w", err)
	}
	if _, err := f.WriteString(cfg); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return fmt.Errorf("写入 mediamtx 配置文件失败: %w", err)
	}
	_ = f.Close()
	s.mtxCfgFile = f.Name()

	s.mtxStderr.Reset()
	s.mtxStdout.Reset()
	s.mediamtx = exec.CommandContext(s.ctx, mediamtxPath, s.mtxCfgFile)
	s.mediamtx.Stdout = &s.mtxStdout
	s.mediamtx.Stderr = &s.mtxStderr

	if logrus.GetLevel() >= logrus.DebugLevel {
		s.mediamtx.Stdout = io.MultiWriter(os.Stdout, &s.mtxStdout)
		s.mediamtx.Stderr = io.MultiWriter(os.Stderr, &s.mtxStderr)
	}

	if err := s.mediamtx.Start(); err != nil {
		return fmt.Errorf("启动 mediamtx 失败: %w", err)
	}

	// 立即启动 Wait() 以避免僵尸进程，并能检测启动后快速退出
	s.mtxExitCh = make(chan error, 1)
	go func(cmd *exec.Cmd) {
		s.mtxExitCh <- cmd.Wait()
	}(s.mediamtx)

	logrus.Infof("mediamtx 已启动 (RTSP 端口: %d)", s.Port)

	// 等待 mediamtx 就绪（TCP 监听可连接）
	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			stderrStr := strings.TrimSpace(s.mtxStderr.String())
			stdoutStr := strings.TrimSpace(s.mtxStdout.String())
			if stderrStr != "" || stdoutStr != "" {
				if stderrStr != "" {
					lines := strings.Split(stderrStr, "\n")
					if len(lines) > 50 {
						lines = lines[len(lines)-50:]
					}
					stderrStr = strings.Join(lines, "\n")
				}
				if stdoutStr != "" {
					lines := strings.Split(stdoutStr, "\n")
					if len(lines) > 50 {
						lines = lines[len(lines)-50:]
					}
					stdoutStr = strings.Join(lines, "\n")
				}
				return fmt.Errorf("mediamtx 未就绪(端口 %d 未监听/不可连接)\nmediamtx stdout(last):\n%s\nmediamtx stderr(last):\n%s", s.Port, stdoutStr, stderrStr)
			}
			return fmt.Errorf("mediamtx 未就绪(端口 %d 未监听/不可连接)", s.Port)
		}

		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		// 如果进程已退出，提前失败
		select {
		case exitErr := <-s.mtxExitCh:
			stderrStr := strings.TrimSpace(s.mtxStderr.String())
			stdoutStr := strings.TrimSpace(s.mtxStdout.String())
			if stderrStr != "" {
				lines := strings.Split(stderrStr, "\n")
				if len(lines) > 50 {
					lines = lines[len(lines)-50:]
				}
				stderrStr = strings.Join(lines, "\n")
			}
			if stdoutStr != "" {
				lines := strings.Split(stdoutStr, "\n")
				if len(lines) > 50 {
					lines = lines[len(lines)-50:]
				}
				stdoutStr = strings.Join(lines, "\n")
			}
			return fmt.Errorf("mediamtx 启动后退出: %v\nmediamtx stdout(last):\n%s\nmediamtx stderr(last):\n%s", exitErr, stdoutStr, stderrStr)
		default:
		}

		time.Sleep(100 * time.Millisecond)
	}

	// 启动所有配置的流
	s.mu.Lock()
	for name, stream := range s.Streams {
		if err := s.startStream(stream); err != nil {
			logrus.Warnf("启动流 %s 失败: %v", name, err)
		}
	}
	s.mu.Unlock()

	return nil
}

// Stop 停止所有流和服务器
func (s *RTSPServer) Stop() {
	s.cancel()

	s.mu.Lock()
	for _, stream := range s.Streams {
		s.stopStream(stream)
	}
	s.mu.Unlock()

	if s.mediamtx != nil && s.mediamtx.Process != nil {
		_ = s.mediamtx.Process.Kill()
		if s.mtxExitCh != nil {
			select {
			case <-s.mtxExitCh:
			case <-time.After(2 * time.Second):
			}
			s.mtxExitCh = nil
		}
		logrus.Info("mediamtx 已停止")
	}
	if s.mtxCfgFile != "" {
		_ = os.Remove(s.mtxCfgFile)
		s.mtxCfgFile = ""
	}
}

// startStream 启动单路流 (FFmpeg 推流到 mediamtx)
func (s *RTSPServer) startStream(stream *RTSPStream) error {
	if stream.running {
		return nil
	}

	source := stream.Source
	var inputArgs []string
	var isTest bool

	switch {
	case strings.HasPrefix(source, "file://"):
		inputPath := strings.TrimPrefix(source, "file://")
		if _, err := os.Stat(inputPath); err != nil {
			return fmt.Errorf("视频文件不存在: %s", inputPath)
		}
		inputArgs = []string{
			"-re",
			"-stream_loop", "-1",
			"-i", inputPath,
		}

	case strings.HasPrefix(source, "rtsp://"):
		inputArgs = []string{
			"-rtsp_transport", "tcp",
			"-i", source,
		}

	case source == "test":
		isTest = true
		inputArgs = []string{
			"-re",
			"-f", "lavfi",
			"-i", fmt.Sprintf("testsrc=size=1920x1080:rate=25,format=yuv420p,drawtext=fontsize=72:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2:text='%s'", stream.Name),
		}

	default:
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("未知视频源: %s", source)
		}
		inputArgs = []string{
			"-re",
			"-stream_loop", "-1",
			"-i", source,
		}
	}

	// FFmpeg 输出到 mediamtx
	rtspDest := fmt.Sprintf("rtsp://127.0.0.1:%d%s", s.Port, stream.RTSPPath)

	var outputArgs []string
	// 说明：
	// - 生产环境视频文件可能是 HEVC(H.265)，直接 copy 推 RTSP 往往会被服务器拒绝 (400 Bad Request)
	// - 为了与 OWL/go2rtc/VLC 等最大兼容，统一转码为 H.264 baseline + yuv420p
	// - test 源也采用相同编码参数
	if isTest {
		outputArgs = []string{
			"-c:v", "libx264",
			"-pix_fmt", "yuv420p",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-g", "25",
			"-bf", "0",
			"-an",
			"-f", "rtsp",
			"-rtsp_transport", "tcp",
			rtspDest,
		}
	} else {
		outputArgs = []string{
			"-c:v", "libx264",
			"-pix_fmt", "yuv420p",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-g", "25",
			"-bf", "0",
			"-an",
			"-f", "rtsp",
			"-rtsp_transport", "tcp",
			rtspDest,
		}
	}

	args := append(inputArgs, outputArgs...)

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("未找到 FFmpeg: %w", err)
	}

	cmd := exec.CommandContext(s.ctx, ffmpegPath, args...)
	cmd.Stdout = nil
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if logrus.GetLevel() >= logrus.DebugLevel {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	}

	logrus.Infof("启动 RTSP 流: %s -> %s", source, stream.FullURI)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 FFmpeg 失败: %w", err)
	}

	stream.cmd = cmd
	stream.running = true

	// 监控进程并自动重启
	go s.monitorStream(stream, ffmpegPath, args, &stderr)

	return nil
}

// monitorStream 监控流进程并自动重启
func (s *RTSPServer) monitorStream(stream *RTSPStream, ffmpegPath string, args []string, stderrBuf *bytes.Buffer) {
	for {
		if stream.cmd == nil {
			return
		}

		err := stream.cmd.Wait()
		stream.running = false

		if s.ctx.Err() != nil {
			return // 服务器已停止
		}

		if err != nil {
			stderrStr := strings.TrimSpace(stderrBuf.String())
			if stderrStr != "" {
				// 避免日志过长，只保留最后 30 行
				lines := strings.Split(stderrStr, "\n")
				if len(lines) > 30 {
					lines = lines[len(lines)-30:]
				}
				stderrStr = strings.Join(lines, "\n")
			}

			if stderrStr != "" {
				logrus.Warnf("FFmpeg 进程退出: %s, 错误: %v\nFFmpeg stderr(last):\n%s\n3秒后重启...", stream.Name, err, stderrStr)
			} else {
				logrus.Warnf("FFmpeg 进程退出: %s, 错误: %v, 3秒后重启...", stream.Name, err)
			}
			time.Sleep(3 * time.Second)

			if s.ctx.Err() != nil {
				return
			}

			// 重启
			newCmd := exec.CommandContext(s.ctx, ffmpegPath, args...)
			newCmd.Stdout = nil
			stderrBuf.Reset()
			newCmd.Stderr = stderrBuf
			if logrus.GetLevel() >= logrus.DebugLevel {
				newCmd.Stderr = io.MultiWriter(os.Stderr, stderrBuf)
			}

			if err := newCmd.Start(); err != nil {
				logrus.Errorf("重启 FFmpeg 失败: %s, %v", stream.Name, err)
				return
			}
			stream.cmd = newCmd
			stream.running = true
		} else {
			return // 正常退出
		}
	}
}

// stopStream 停止单路流
func (s *RTSPServer) stopStream(stream *RTSPStream) {
	if !stream.running || stream.cmd == nil {
		return
	}

	if stream.cmd.Process != nil {
		stream.cmd.Process.Kill()
	}
	stream.running = false
	logrus.Infof("停止 RTSP 流: %s", stream.Name)
}

// extractStreamName 从源路径提取流名
func extractStreamName(source string) string {
	if strings.HasPrefix(source, "file://") {
		path := strings.TrimPrefix(source, "file://")
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		return strings.TrimSuffix(base, ext)
	}

	if strings.HasPrefix(source, "rtsp://") {
		parts := strings.Split(source, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	return source
}

// getLocalIP 获取本机 IP
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			ip := ipNet.IP.String()
			if !strings.HasPrefix(ip, "172.") && !strings.HasPrefix(ip, "198.18.") {
				return ip
			}
		}
	}
	return "127.0.0.1"
}

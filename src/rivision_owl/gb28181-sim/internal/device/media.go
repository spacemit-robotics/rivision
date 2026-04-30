package device

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// GB28181-2015 标准常量
const (
	// RTP payload types (RFC 3551 / GB28181)
	PayloadTypePS   = 96  // PS (Program Stream) - GB28181 主要格式
	PayloadTypeMPEG = 97  // MPEG-4
	PayloadTypeH264 = 98  // H.264 ES
	PayloadTypeH265 = 99  // H.265 ES

	// RTP 时间戳频率
	RTPTimestampFrequency = 90000 // 90kHz for video

	// RTP 包相关
	RTPVersion           = 2
	RTPMaxPayloadSize    = 1300 // MTU - IP/UDP/RTP header
	RTPHeaderSize        = 12
	RTCPReportInterval   = 5 * time.Second
)

// mp4H264StreamCopy 为 true 时 MP4/MKV 等走 -c:v copy（省 CPU）。
// 默认 false：对容器内 H.264 重编码为无 B 帧流，避免 B 帧在 RTP 实时推送下出现
// 解码/显示顺序与 RTP 时间戳错位（画面“回跳”、1-2-3/2-3-4 感）。
// 环境变量: GB28181_SIM_MP4_COPY=1 启用 copy。
func mp4H264StreamCopy() bool {
	return strings.TrimSpace(os.Getenv("GB28181_SIM_MP4_COPY")) == "1"
}

// probeVideoCodec returns the video codec name of a file using ffprobe.
// Returns "" on error (fallback to H.264 default).
func probeVideoCodec(file string) string {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0", "-show_entries", "stream=codec_name",
		"-of", "csv=p=0", file).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// mp4H265StreamCopy 同上，针对 H.265 容器
func mp4H265StreamCopy() bool {
	return strings.TrimSpace(os.Getenv("GB28181_SIM_MP4_COPY")) == "1"
}

// StreamStats holds streaming statistics for RTCP reports
type StreamStats struct {
	PacketsSent atomic.Uint64
	BytesSent   atomic.Uint64
	FramesSent  atomic.Uint64
	OctetsSent  atomic.Uint64 // RTCP uses this
	LastFrameTs atomic.Int64

	// RTCP statistics
	PacketsLost  atomic.Uint64
	LastSeq      atomic.Uint64
	Jitter       atomic.Uint64

	// NTP timestamp for RTCP SR
	BaseTimeNTP uint64
	BaseTimeRTP uint32
}

func newStreamStats() *StreamStats {
	s := &StreamStats{}
	s.LastFrameTs.Store(time.Now().UnixMilli())
	s.BaseTimeNTP = 0
	s.BaseTimeRTP = 0
	return s
}

var (
	statsMu  sync.RWMutex
	statsMap = make(map[string]*StreamStats)
)

func getStats(sessionID string) *StreamStats {
	statsMu.RLock()
	s, ok := statsMap[sessionID]
	statsMu.RUnlock()
	if ok {
		return s
	}
	statsMu.Lock()
	defer statsMu.Unlock()
	s = newStreamStats()
	statsMap[sessionID] = s
	return s
}

// StartMediaStream starts streaming media to the destination
// 符合 GB28181-2015 标准
func StartMediaStream(session *MediaSession) error {
	source := session.Source

	// 根据源文件扩展名并结合容器探测判断编码格式：
	//   扩展名为 .h265/.265/.hevc  → H.265 ES (PT=99)
	//   扩展名为 .mp4/.mkv/.avi/.mov → 探测实际编码（可能是 HEVC）
	//   其他（H.264 / 其他）        → H.264 ES (PT=98)
	session.PayloadType = PayloadTypeH264

	// 容器文件需要探测实际编码，因为 .mp4 可能内含 HEVC
	var fileToProbe string
	if strings.HasPrefix(source, "file://") {
		fileToProbe = strings.TrimPrefix(source, "file://")
	} else if !strings.HasPrefix(source, "rtsp://") && source != "test" {
		fileToProbe = source
	}

	if fileToProbe != "" {
		ext := strings.ToLower(filepath.Ext(fileToProbe))
		if ext == ".h265" || ext == ".265" || ext == ".hevc" {
			session.PayloadType = PayloadTypeH265
		} else if ext == ".mp4" || ext == ".mkv" || ext == ".avi" || ext == ".mov" {
			codec := probeVideoCodec(fileToProbe)
			if codec == "hevc" {
				session.PayloadType = PayloadTypeH265
				logrus.Infof("Detected HEVC inside container: %s", fileToProbe)
			}
		}
	}

	switch {
	case source == "test":
		return streamTestPattern(session)
	case strings.HasPrefix(source, "rtsp://"):
		return streamRTSP(session, source)
	case strings.HasPrefix(source, "file://"):
		path := strings.TrimPrefix(source, "file://")
		return streamFile(session, path)
	default:
		if _, err := os.Stat(source); err == nil {
			return streamFile(session, source)
		}
		return fmt.Errorf("unknown source type: %s", source)
	}
}

// streamTestPattern generates a test video pattern
// 使用 GB28181 标准的 PS over RTP 封装
func streamTestPattern(session *MediaSession) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		logrus.Warn("FFmpeg not found, falling back to native RTP")
		return nativeRTPSend(session)
	}

	stats := getStats(session.ChannelID)
	chLabel := session.ChannelID
	if len(chLabel) > 8 {
		chLabel = chLabel[:8]
	}

	// 根据请求的 Payload Type 选择封装格式
	if session.PayloadType == PayloadTypePS || session.PayloadType == 0 {
		return streamTestPatternPS(session, ffmpegPath, chLabel, stats)
	}
	return streamTestPatternH264(session, ffmpegPath, chLabel, stats)
}

// streamTestPatternPS 使用 PS 封装 (GB28181 标准格式)
func streamTestPatternPS(session *MediaSession, ffmpegPath, chLabel string, stats *StreamStats) error {
	// 对于 PS (MPEG-TS)，使用 udp:// 而不是 rtp://
	udpDest := fmt.Sprintf("udp://%s:%d?pkt_size=1316", session.DestIP, session.DestPort)

	args := []string{
		"-re",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=size=640x480:rate=25,format=yuv420p"),
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-profile:v", "baseline",
		"-level", "3.0",
		"-g", "25",
		"-keyint_min", "25",
		"-sc_threshold", "0",
		"-b:v", "2000k",
		"-maxrate", "2000k",
		"-bufsize", "2000k",
		"-bf", "0",
		"-threads", "4",
		// PS 封装 (GB28181 标准)
		"-f", "mpegts",
	}

	if session.SSRC != 0 {
		args = append(args, "-ssrc", fmt.Sprintf("%d", session.SSRC))
	}
	args = append(args, udpDest)

	logrus.Infof("Streaming MPEG-TS/PS (GB28181): %s:%d (PT=96)", session.DestIP, session.DestPort)
	logrus.Infof("UDP destination: %s", udpDest)
	logrus.Debugf("FFmpeg cmd: %s %v", ffmpegPath, args)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	go func() {
		<-session.stopCh
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	go startRTCPReports(session, stats)

	return cmd.Wait()
}

// streamTestPatternH264 使用 H.264 ES over RTP
func streamTestPatternH264(session *MediaSession, ffmpegPath, chLabel string, stats *StreamStats) error {
	dest := fmt.Sprintf("rtp://%s:%d", session.DestIP, session.DestPort)

	args := []string{
		"-re",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=size=640x480:rate=25,format=yuv420p,drawtext=fontsize=30:fontcolor=white:x=(w-text_w)/2:y=h-50:text='CH %s'", chLabel),
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-profile:v", "baseline",
		"-level", "3.0",
		"-g", "25",
		"-keyint_min", "25",
		"-sc_threshold", "0",
		"-b:v", "2000k",
		"-maxrate", "2000k",
		"-bufsize", "2000k",
		"-bf", "0",
		"-threads", "4",
		// H.264 ES over RTP
		"-f", "rtp",
		"-payload_type", fmt.Sprintf("%d", PayloadTypeH264),
	}

	if session.SSRC != 0 {
		args = append(args, "-ssrc", fmt.Sprintf("%d", session.SSRC))
	}
	args = append(args, dest)

	logrus.Infof("Streaming H.264 ES: %s:%d (PT=%d)", session.DestIP, session.DestPort, PayloadTypeH264)
	logrus.Debugf("FFmpeg cmd: %s %v", ffmpegPath, args)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	go func() {
		<-session.stopCh
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	go startRTCPReports(session, stats)

	return cmd.Wait()
}

// streamRTSP re-streams an RTSP source
func streamRTSP(session *MediaSession, rtspURL string) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("FFmpeg required for RTSP streaming: %w", err)
	}

	stats := getStats(session.ChannelID)

	// 根据 Payload Type 选择输出格式
	var args []string

	if session.PayloadType == PayloadTypeH265 {
		// H.265 ES over RTP：RTSP 源 → 接收 → H.265 RTP (PT=99)
		args = []string{
			"-rtsp_transport", "tcp",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-max_delay", "500000",
			"-i", rtspURL,
			"-an", // RTP muxer 仅支持单视频流
			"-c:v", "copy",
			"-bsf:v", "hevc_mp4toannexb",
			"-f", "rtp",
			"-payload_type", fmt.Sprintf("%d", PayloadTypeH265),
		}
	} else if session.PayloadType == PayloadTypePS || session.PayloadType == 0 {
		// PS over MPEG-TS
		udpDest := fmt.Sprintf("udp://%s:%d?pkt_size=1316", session.DestIP, session.DestPort)
		args = []string{
			"-rtsp_transport", "tcp",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-max_delay", "500000",
			"-i", rtspURL,
			"-c:v", "copy",
			"-an",
			"-f", "mpegts",
			udpDest,
		}
	} else {
		// H.264 ES over RTP (PT=98)
		args = []string{
			"-rtsp_transport", "tcp",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-max_delay", "500000",
			"-i", rtspURL,
			"-c:v", "copy",
			"-an",
			"-bsf:v", "h264_mp4toannexb",
			"-f", "rtp",
			"-payload_type", fmt.Sprintf("%d", session.PayloadType),
		}
	}

	logrus.Infof("Streaming RTSP: %s -> %s:%d (PT=%d)", rtspURL, session.DestIP, session.DestPort, session.PayloadType)
	logrus.Debugf("FFmpeg cmd: %s %v", ffmpegPath, args)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	go func() {
		<-session.stopCh
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	go startRTCPReports(session, stats)

	return cmd.Wait()
}

// streamFile streams a local video file
func streamFile(session *MediaSession, filePath string) error {
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("FFmpeg required for file streaming: %w", err)
	}

	stats := getStats(session.ChannelID)
	ext := strings.ToLower(filepath.Ext(filePath))

	// 容器文件 (.mp4/.mkv/.avi/.mov): 探测实际视频编码，而非仅凭扩展名判断。
	// 例如 test_h264_aac.mp4 扩展名是 .mp4，但视频流实际上是 HEVC。
	if session.PayloadType == PayloadTypeH265 {
		return streamFileH265(session, ffmpegPath, filePath, stats)
	}
	return streamFileH264(session, ffmpegPath, filePath, ext, stats)
}

// streamFilePS 使用 PS 封装流式传输文件
func streamFilePS(session *MediaSession, ffmpegPath, filePath, ext, dest string, stats *StreamStats) error {
	var args []string

	// 对于 PS (MPEG-TS)，使用 udp:// 而不是 rtp://
	// rtp:// 期望 RTP 包，而 MPEG-TS 是直接流
	udpDest := fmt.Sprintf("udp://%s:%d?pkt_size=1316", session.DestIP, session.DestPort)

	switch ext {
	case ".h264", ".264":
		// H.264 文件需要先解码再封装为 PS
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-f", "h264",
			"-i", filePath,
			"-c:v", "copy",
			"-f", "mpegts",
		}
	case ".mp4", ".mkv", ".avi", ".mov":
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-i", filePath,
			"-c:v", "copy",
			"-an",
			"-f", "mpegts",
		}
	case ".h265", ".265", ".hevc":
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-i", filePath,
			"-c:v", "copy",
			"-f", "mpegts",
		}
	default:
		// 转码为 H.264
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-i", filePath,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-g", "25",
			"-bf", "0",
			"-f", "mpegts",
		}
	}

	if session.SSRC != 0 {
		args = append(args, "-ssrc", fmt.Sprintf("%d", session.SSRC))
	}
	args = append(args, udpDest)

	logrus.Infof("Streaming MPEG-TS/PS: %s (format: %s)", filePath, ext)
	logrus.Infof("UDP destination: %s", udpDest)
	logrus.Debugf("FFmpeg cmd: %s %v", ffmpegPath, args)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	go func() {
		<-session.stopCh
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	go startRTCPReports(session, stats)

	return cmd.Wait()
}

// streamFileH265 使用 H.265 ES over RTP 流式传输文件
// 符合 GB28181-2015 标准
// 支持：.h265/.265/.hevc 原始 AnnexB 文件（-c:v copy + hevc_mp4toannexb）
//      MP4/MKV/AVI/MOV 等容器（H.265 视频）：libx265 重编码 → 无 B 帧
//      其他格式：libx265 转码
func streamFileH265(session *MediaSession, ffmpegPath, filePath string, stats *StreamStats) error {
	var args []string

	dest := fmt.Sprintf("rtp://%s:%d", session.DestIP, session.DestPort)

	// GB28181-SIM_MP4_COPY=1 可选：对容器内 H.265 也走 copy（省 CPU）
	if mp4H265StreamCopy() {
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-i", filePath,
			"-an", // RTP muxer 仅支持单视频流
			"-c:v", "copy",
			"-bsf:v", "hevc_mp4toannexb",
			"-f", "rtp",
			"-payload_type", fmt.Sprintf("%d", PayloadTypeH265),
		}
		logrus.Infof("MP4 H.265: stream copy (GB28181_SIM_MP4_COPY=1); may have HEVC B-frames jitter")
	} else {
		// 默认：libx265 重编码，-bf 0（无 B 帧），确保 RTP 时间戳单调
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-i", filePath,
			"-an", // RTP muxer 仅支持单视频流
			"-c:v", "libx265",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-x265-params", "ctu=32:bframes=0:sao=0",
			"-f", "rtp",
			"-payload_type", fmt.Sprintf("%d", PayloadTypeH265),
		}
		logrus.Infof("MP4 H.265: transcode libx265 -bframes=0 (no B-frames) for smooth RTP playback")
	}

	if session.SSRC != 0 {
		args = append(args, "-ssrc", fmt.Sprintf("%d", session.SSRC))
	}
	args = append(args, dest)

	logrus.Infof("Starting FFmpeg HEVC stream: %s -> %s (PT=%d)", filePath, dest, PayloadTypeH265)
	logrus.Debugf("FFmpeg cmd: %s %v", ffmpegPath, args)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	go func() {
		<-session.stopCh
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	go startRTCPReports(session, stats)

	return cmd.Wait()
}

// streamFileH264 使用 H.264 ES over RTP 流式传输文件
// 符合 GB28181-2015 / RFC 6184 标准
func streamFileH264(session *MediaSession, ffmpegPath, filePath, ext string, stats *StreamStats) error {
	var args []string

	dest := fmt.Sprintf("rtp://%s:%d", session.DestIP, session.DestPort)

	switch ext {
	case ".h264", ".264":
		// 原始 H.264 文件 - 直接发送 AnnexB 格式
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-f", "h264",
			"-i", filePath,
			"-c:v", "copy",
			"-f", "rtp",
			"-payload_type", fmt.Sprintf("%d", PayloadTypeH264),
		}
	case ".mp4", ".mkv", ".avi", ".mov":
		// MP4 等容器常见为 Main/High + B 帧 (has_b_frames>0)。-c:v copy 时 NAL 为解码顺序，
		// RTP 时间戳若按显示顺序打包易与 ZLM/播放器缓冲策略冲突，表现为运动物体“回跳”、
		// 似 1-2-3→2-3-4 重复。默认重编码为 -bf 0 + zerolatency，GOP 与时间戳单调，利于直播。
		// 低性能机器可设环境变量 GB28181_SIM_MP4_COPY=1 恢复 copy（可能仍有 B 帧抖动）。
		if mp4H264StreamCopy() {
			args = []string{
				"-re",
				"-stream_loop", "-1",
				"-i", filePath,
				"-c:v", "copy",
				"-an",
				"-bsf:v", "h264_mp4toannexb",
				"-f", "rtp",
				"-payload_type", fmt.Sprintf("%d", PayloadTypeH264),
			}
			logrus.Infof("MP4 H.264: stream copy (GB28181_SIM_MP4_COPY=1); B-frames may cause playback jitter")
		} else {
			args = []string{
				"-re",
				"-stream_loop", "-1",
				"-i", filePath,
				"-an",
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-tune", "zerolatency",
				"-pix_fmt", "yuv420p",
				"-profile:v", "main",
				"-level", "4.2",
				"-bf", "0",
				"-g", "60",
				"-keyint_min", "15",
				"-sc_threshold", "0",
				"-f", "rtp",
				"-payload_type", fmt.Sprintf("%d", PayloadTypeH264),
			}
			logrus.Infof("MP4 H.264: transcode libx264 -bf=0 (no B-frames) for smooth RTP playback")
		}
	default:
		args = []string{
			"-re",
			"-stream_loop", "-1",
			"-i", filePath,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-r", "25",
			"-g", "25",
			"-bf", "0",
			"-f", "rtp",
			"-payload_type", fmt.Sprintf("%d", PayloadTypeH264),
		}
	}

	// 添加 SSRC 和目标地址
	if session.SSRC != 0 {
		args = append(args, "-ssrc", fmt.Sprintf("%d", session.SSRC))
	}
	args = append(args, dest)

	logrus.Infof("Starting FFmpeg stream: %s -> %s (PT=%d)", filePath, dest, PayloadTypeH264)
	logrus.Debugf("FFmpeg cmd: %s %v", ffmpegPath, args)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	go func() {
		<-session.stopCh
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	go startRTCPReports(session, stats)

	return cmd.Wait()
}

// startRTCPReports 启动 RTCP 报告 (RFC 3550 §6)
func startRTCPReports(session *MediaSession, stats *StreamStats) {
	if session.UseTCP {
		// TCP 模式下 RTCP 通过同一连接发送
		logrus.Debug("RTCP reports via TCP not implemented")
		return
	}

	// 启动 RTCP 接收和发送
	go rtcpReceiver(session, stats)
	go rtcpSender(session, stats)
}

// rtcpReceiver 接收 RTCP 报告
func rtcpReceiver(session *MediaSession, stats *StreamStats) {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(session.DestIP),
		Port: session.DestPort + 1, // RTCP port = RTP port + 1
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		logrus.Warnf("RTCP receiver failed to listen: %v", err)
		return
	}
	defer conn.Close()

	buf := make([]byte, 1500)
	for {
		select {
		case <-session.stopCh:
			return
		default:
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			parseRTCPReport(buf[:n], stats)
		}
	}
}

// rtcpSender 发送 RTCP SR/RR 报告 (RFC 3550 §6.4)
func rtcpSender(session *MediaSession, stats *StreamStats) {
	ticker := time.NewTicker(RTCPReportInterval)
	defer ticker.Stop()

	ssrc := session.SSRC
	if ssrc == 0 {
		ssrc = uint32(time.Now().UnixNano() & 0xFFFFFFFF)
	}

	rtcpPort := session.DestPort + 1
	if !session.UseTCP {
		rtcpAddr := &net.UDPAddr{
			IP:   net.ParseIP(session.DestIP),
			Port: rtcpPort,
		}
		conn, err := net.DialUDP("udp", nil, rtcpAddr)
		if err != nil {
			logrus.Warnf("RTCP sender failed to connect: %v", err)
			return
		}
		defer conn.Close()

		for {
			select {
			case <-session.stopCh:
				return
			case <-ticker.C:
				sendRTCPReport(conn, ssrc, stats)
			}
		}
	}
}

// sendRTCPReport 发送 RTCP Sender Report (SR)
func sendRTCPReport(conn *net.UDPConn, ssrc uint32, stats *StreamStats) {
	// RTCP SR packet
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// | V=2 |P|   RC   |   PT=SR=200   |             length            |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                         SSRC of sender                        |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |              NTP timestamp, most significant word             |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |             NTP timestamp, least significant word             |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                       RTP timestamp                           |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                     packet's count                           |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                      octet's count                           |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

	// Get NTP timestamp
	ntp := getNTPTime()

	// Build SR packet
	pkt := make([]byte, 28) // SR header without report blocks

	// Header
	pkt[0] = 0x80 // V=2, P=0, RC=0
	pkt[1] = 200  // PT=SR
	binary.BigEndian.PutUint16(pkt[2:4], 6) // length = 6 words (no report blocks)

	// SSRC
	binary.BigEndian.PutUint32(pkt[4:8], ssrc)

	// NTP timestamp
	binary.BigEndian.PutUint32(pkt[8:12], uint32(ntp>>32))            // MSW
	binary.BigEndian.PutUint32(pkt[12:16], uint32(ntp&0xFFFFFFFF))     // LSW

	// RTP timestamp (approximate)
	rtpTs := uint32(time.Now().UnixNano() / 1e6 * 90) // 90kHz
	binary.BigEndian.PutUint32(pkt[16:20], rtpTs)

	// Packet count
	binary.BigEndian.PutUint32(pkt[20:24], uint32(stats.PacketsSent.Load()))

	// Octet count
	binary.BigEndian.PutUint32(pkt[24:28], uint32(stats.OctetsSent.Load()))

	conn.Write(pkt)
}

// parseRTCPReport 解析 RTCP 报告
func parseRTCPReport(data []byte, stats *StreamStats) {
	if len(data) < 8 {
		return
	}

	// Check version and payload type
	version := (data[0] >> 6) & 0x03
	if version != 2 {
		return
	}

	pt := data[1]
	switch pt {
	case 200: // SR
		logrus.Debug("Received RTCP SR")
	case 201: // RR
		logrus.Debug("Received RTCP RR")
	case 203: // BYE
		logrus.Debug("Received RTCP BYE")
	}
}

// getNTPTime 获取当前 NTP 时间戳
func getNTPTime() uint64 {
	now := time.Now()
	// NTP epoch: 1900-01-01
	// Unix epoch: 1970-01-01
	// Difference: 70 years (including 17 leap years)
	const ntpEpoch = 2208988800

	sec := uint64(now.Unix()) + ntpEpoch
	frac := uint64((now.Nanosecond() * 0xFFFFFFFF) / 1e9)

	return (sec << 32) | frac
}

// nativeRTPSend 使用原生 Go 实现发送 RTP 数据
// 注意：这是简化实现，仅用于无 FFmpeg 的情况
func nativeRTPSend(session *MediaSession) error {
	logrus.Info("Native RTP mode: sending RTP packets")

	stats := getStats(session.ChannelID)
	conn, err := createUDPConn(session.DestIP, session.DestPort)
	if err != nil {
		return fmt.Errorf("failed to create UDP sender: %w", err)
	}
	defer conn.Close()

	ssrc := session.SSRC
	if ssrc == 0 {
		ssrc = uint32(time.Now().UnixNano() & 0xFFFFFFFF)
	}

	// Initialize NTP base time
	stats.BaseTimeNTP = getNTPTime()
	stats.BaseTimeRTP = 0

	frameInterval := 40 * time.Millisecond // 25 fps
	frameWidth := 640
	frameHeight := 480
	frameNum := 0
	seq := uint16(0)
	ts := uint32(0)

	for {
		select {
		case <-session.stopCh:
			logrus.Infof("Native stream stopped: frames=%d, packets=%d",
				stats.FramesSent.Load(), stats.PacketsSent.Load())
			return nil
		case <-time.After(frameInterval):
			frameNum++
			seq++
			ts += 3600 // 90kHz / 25fps = 3600
			stats.BaseTimeRTP = ts

			// Generate H.264 NAL unit (simplified - SPS/PPS + SEI + IDR slice)
			frameData := generateH264IDRFrame(frameWidth, frameHeight, frameNum)

			// Split into RTP packets with correct marker bit
			rtpPackets := buildRTPPackets(frameData, ssrc, seq, ts)

			for _, pkt := range rtpPackets {
				n, err := conn.Write(pkt)
				if err != nil {
					logrus.Warnf("Write error: %v", err)
					continue
				}
				stats.PacketsSent.Add(1)
				stats.BytesSent.Add(uint64(n))
				stats.OctetsSent.Add(uint64(n))
			}

			stats.FramesSent.Add(1)
			stats.LastFrameTs.Store(time.Now().UnixMilli())

			if frameNum%250 == 0 {
				logrus.Debugf("Native RTP: frames=%d seq=%d ts=%d ssrc=%d",
					frameNum, seq, ts, ssrc)
			}
		}
	}
}

// buildRTPPackets 构建符合 RFC 6184 的 RTP 包
func buildRTPPackets(data []byte, ssrc uint32, seq uint16, ts uint32) [][]byte {
	packets := make([][]byte, 0)

	for i := 0; i < len(data); i += RTPMaxPayloadSize {
		end := i + RTPMaxPayloadSize
		if end > len(data) {
			end = len(data)
		}
		isLast := end == len(data)

		// RFC 6184 RTP header
		pkt := make([]byte, RTPHeaderSize+len(data[i:end]))
		pkt[0] = 0x80 // V=2, P=0, X=0, CC=0
		pkt[1] = 0x60 // M=0 (set below for last packet), PT=96 (H.264)

		// Set marker bit for last packet of frame (RFC 6184 §4.4)
		if isLast {
			pkt[1] = 0xE0 // M=1, PT=96
		}

		binary.BigEndian.PutUint16(pkt[2:4], seq)
		binary.BigEndian.PutUint32(pkt[4:8], ts)
		binary.BigEndian.PutUint32(pkt[8:12], ssrc)
		copy(pkt[RTPHeaderSize:], data[i:end])

		packets = append(packets, pkt)
	}

	return packets
}

// generateH264IDRFrame 生成简化的 H.264 IDR 帧
// 包含 SPS, PPS, SEI 和 IDR 切片
func generateH264IDRFrame(width, height, frameNum int) []byte {
	// H.264 SPS (Sequence Parameter Set)
	sps := []byte{
		0x67, 0x42, 0xC0, 0x1F, 0xDA, 0x0F, 0x0A, 0x69,
		0xA8, 0x00, 0x78, 0x02, 0x27, 0xE5, 0xC0,
	}

	// Update SPS with correct dimensions
	sps[5] = byte(width >> 8)
	sps[6] = byte(width & 0xFF)
	sps[7] = byte(height >> 8)
	sps[8] = byte(height & 0xFF)

	// H.264 PPS (Picture Parameter Set)
	pps := []byte{
		0x68, 0xCE, 0x38, 0x80,
	}

	// Add start code prefix
	nalUnits := make([]byte, 0)
	nalUnits = append(nalUnits, 0x00, 0x00, 0x00, 0x01)
	nalUnits = append(nalUnits, sps...)
	nalUnits = append(nalUnits, 0x00, 0x00, 0x00, 0x01)
	nalUnits = append(nalUnits, pps...)

	// Generate simple IDR slice data
	frameSize := width * height * 3 / 2
	idrData := make([]byte, frameSize)

	// Generate test pattern
	barWidth := width / 8
	for y := 0; y < height; y++ {
		bar := (y / barWidth) % 8
		var yVal byte
		switch bar {
		case 0:
			yVal = 180
		case 1:
			yVal = 164
		case 2:
			yVal = 145
		case 3:
			yVal = 126
		case 4:
			yVal = 106
		case 5:
			yVal = 82
		case 6:
			yVal = 51
		case 7:
			yVal = 35
		}
		for x := 0; x < width; x++ {
			idrData[y*width+x] = yVal
		}
	}

	// Add motion
	offset := (frameNum * 20) % width
	for y := 0; y < height; y++ {
		for x := 0; x < width/4; x++ {
			realX := (x + offset) % width
			idrData[y*width+realX] = 255 - idrData[y*width+realX]
		}
	}

	// Chroma
	for y := 0; y < height/2; y++ {
		for x := 0; x < width/2; x++ {
			idx := width*height + y*width/2+x
			idrData[idx] = 128 + byte(frameNum%32)
		}
	}

	// Simple slice header + data (IDR frame, slice 0)
	idrNAL := []byte{0x65} // IDR NAL unit type
	idrNAL = append(idrNAL, 0x88, 0x84, 0x00) // Slice header
	idrNAL = append(idrNAL, idrData[:width*height/4]...) // Simplified data

	nalUnits = append(nalUnits, 0x00, 0x00, 0x00, 0x01)
	nalUnits = append(nalUnits, idrNAL...)

	return nalUnits
}

func createUDPConn(addr string, port int) (*net.UDPConn, error) {
	udpAddr := &net.UDPAddr{
		IP:   net.ParseIP(addr),
		Port: port,
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
	if err := conn.SetWriteBuffer(65536); err != nil {
		logrus.Warnf("SetWriteBuffer failed: %v", err)
	}
	return conn, nil
}

// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package video

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rivision/rivision-cli/internal/gateway"
)

type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) GetVideoInfo(videoPath string) (*gateway.VideoInfo, error) {
	// 使用 ffprobe 获取视频信息
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,nb_frames",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 执行失败: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	info := &gateway.VideoInfo{}

	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) >= 4 {
			// stream info: width,height,r_frame_rate,nb_frames
			info.Width, _ = strconv.Atoi(parts[0])
			info.Height, _ = strconv.Atoi(parts[1])
			// 解析帧率 (如 "30/1")
			if fps := parseFPS(parts[2]); fps > 0 {
				info.FPS = fps
			}
			info.FrameCount, _ = strconv.Atoi(parts[3])
		} else if len(parts) == 1 {
			// format info: duration
			info.Duration, _ = strconv.ParseFloat(parts[0], 64)
		}
	}

	// 如果没有获取到帧数，通过时长和帧率计算
	if info.FrameCount == 0 && info.Duration > 0 && info.FPS > 0 {
		info.FrameCount = int(info.Duration * info.FPS)
	}

	return info, nil
}

func (e *Extractor) ExtractFrames(videoPath string, interval int) ([]gateway.Frame, error) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "rivision-frames-")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 获取视频信息
	info, err := e.GetVideoInfo(videoPath)
	if err != nil {
		return nil, err
	}

	// 计算需要提取的帧
	fps := info.FPS
	if fps <= 0 {
		fps = 30 // 默认帧率
	}

	// 使用 ffmpeg 提取帧
	// -vf "select='not(mod(n,interval))'" 每隔 interval 帧提取一帧
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vf", fmt.Sprintf("select='not(mod(n,%d))'", interval),
		"-vsync", "vfr",
		"-q:v", "2",
		filepath.Join(tmpDir, "frame_%04d.jpg"),
	)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg 提取帧失败: %w", err)
	}

	// 读取提取的帧
	files, err := filepath.Glob(filepath.Join(tmpDir, "frame_*.jpg"))
	if err != nil {
		return nil, err
	}

	var frames []gateway.Frame
	for i, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		frameIndex := i * interval
		timestamp := float64(frameIndex) / fps

		frames = append(frames, gateway.Frame{
			Base64:    base64.StdEncoding.EncodeToString(data),
			Timestamp: timestamp,
			Index:     frameIndex,
		})
	}

	return frames, nil
}

func (e *Extractor) ExtractKeyFrames(videoPath string, count int) ([]gateway.Frame, error) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "rivision-keyframes-")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 获取视频信息
	info, err := e.GetVideoInfo(videoPath)
	if err != nil {
		return nil, err
	}

	// 计算时间间隔
	interval := info.Duration / float64(count+1)

	var frames []gateway.Frame
	for i := 1; i <= count; i++ {
		timestamp := interval * float64(i)
		outputFile := filepath.Join(tmpDir, fmt.Sprintf("keyframe_%02d.jpg", i))

		// 使用 ffmpeg 提取指定时间点的帧
		cmd := exec.Command("ffmpeg",
			"-ss", fmt.Sprintf("%.2f", timestamp),
			"-i", videoPath,
			"-vframes", "1",
			"-q:v", "2",
			outputFile,
		)

		if err := cmd.Run(); err != nil {
			continue
		}

		data, err := os.ReadFile(outputFile)
		if err != nil {
			continue
		}

		frames = append(frames, gateway.Frame{
			Base64:    base64.StdEncoding.EncodeToString(data),
			Timestamp: timestamp,
			Index:     int(timestamp * info.FPS),
		})
	}

	return frames, nil
}

func parseFPS(fpsStr string) float64 {
	// 解析帧率字符串，如 "30/1" 或 "29.97"
	re := regexp.MustCompile(`(\d+)/(\d+)`)
	matches := re.FindStringSubmatch(fpsStr)
	if len(matches) == 3 {
		num, _ := strconv.ParseFloat(matches[1], 64)
		den, _ := strconv.ParseFloat(matches[2], 64)
		if den > 0 {
			return num / den
		}
	}

	// 尝试直接解析为浮点数
	fps, _ := strconv.ParseFloat(fpsStr, 64)
	return fps
}

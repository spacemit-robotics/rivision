package cmd

import (
	"fmt"
	"strings"

	"github.com/rivision/rivision-cli/internal/gateway"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/rivision/rivision-cli/internal/video"
	"github.com/spf13/cobra"
)

var (
	detectQuery         string
	detectFrameInterval int
	detectConfidence    float64
)

var detectCmd = &cobra.Command{
	Use:   "detect [视频文件]",
	Short: "视频目标检测 (YOLO)",
	Long:  `使用YOLO模型对视频进行目标检测，支持指定检测目标`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		videoPath := args[0]
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "视频目标检测")

		client := gateway.NewClient(gatewayURL)

		// 健康检查
		term.PrintStatus("检查Gateway状态...")
		if !client.HealthCheck() {
			term.PrintError("Gateway连接失败")
			return fmt.Errorf("gateway不可用")
		}
		term.PrintSuccess("Gateway连接成功")

		// 获取视频信息
		term.PrintStatus("获取视频信息...")
		extractor := video.NewExtractor()
		info, err := extractor.GetVideoInfo(videoPath)
		if err != nil {
			term.PrintError(fmt.Sprintf("获取视频信息失败: %v", err))
			return err
		}
		term.PrintInfo(fmt.Sprintf("📹 视频时长: %.1fs, FPS: %.0f", info.Duration, info.FPS))

		// 提取关键帧
		term.PrintStatus("提取关键帧...")
		frames, err := extractor.ExtractFrames(videoPath, detectFrameInterval)
		if err != nil {
			term.PrintError(fmt.Sprintf("关键帧提取失败: %v", err))
			return err
		}
		term.PrintInfo(fmt.Sprintf("提取了 %d 个关键帧", len(frames)))

		// 检测目标
		targets := strings.Split(detectQuery, ",")
		term.PrintInfo(fmt.Sprintf("🎯 检测目标: %s", strings.Join(targets, ", ")))

		detectedCount := 0
		for i, frame := range frames {
			term.PrintStatus(fmt.Sprintf("检测帧 %d/%d...", i+1, len(frames)))

			result, err := client.DetectYOLOBase64(frame.Base64)
			if err != nil {
				term.PrintWarning(fmt.Sprintf("帧 %d 检测失败: %v", i+1, err))
				continue
			}

			if !result.Success {
				continue
			}

			// 过滤匹配的目标
			for _, det := range result.Detections {
				for _, target := range targets {
					if strings.EqualFold(strings.TrimSpace(target), det.ClassName) && det.Confidence >= detectConfidence {
						detectedCount++
						term.PrintSuccess(fmt.Sprintf("  ✅ 帧 %d (%.1fs): %s (%.1f%%)",
							i+1, frame.Timestamp, det.ClassName, det.Confidence*100))
					}
				}
			}
		}

		term.PrintPanel("检测结果", fmt.Sprintf(
			"检测帧数: %d\n匹配目标: %d\n检测目标: %s",
			len(frames), detectedCount, detectQuery))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().StringVar(&detectQuery, "query", "", "检测目标 (逗号分隔, 如: person,car,bicycle)")
	detectCmd.Flags().IntVar(&detectFrameInterval, "frame-interval", 30, "帧采样间隔")
	detectCmd.Flags().Float64Var(&detectConfidence, "confidence", 0.5, "置信度阈值")
	detectCmd.MarkFlagRequired("query")
}

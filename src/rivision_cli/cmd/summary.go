package cmd

import (
	"fmt"

	"github.com/rivision/rivision-cli/internal/gateway"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/rivision/rivision-cli/internal/video"
	"github.com/spf13/cobra"
)

var summaryCmd = &cobra.Command{
	Use:   "summary [视频文件]",
	Short: "视频摘要生成",
	Long:  `分析视频内容并生成摘要`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		videoPath := args[0]

		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "视频摘要")

		term.PrintInfo(fmt.Sprintf("📝 生成视频摘要: %s", videoPath))

		// 创建客户端
		client := gateway.NewClient(gatewayURL)

		// 健康检查
		term.PrintStatus("检查Gateway状态...")
		if !client.HealthCheck() {
			term.PrintError("Gateway连接失败!")
			return fmt.Errorf("gateway连接失败")
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
		term.PrintInfo(fmt.Sprintf("📹 视频时长: %s", formatDuration(info.Duration)))

		// 提取关键帧
		term.PrintStatus("提取关键帧...")
		frames, err := extractor.ExtractKeyFrames(videoPath, 10) // 提取10个关键帧
		if err != nil {
			term.PrintError(fmt.Sprintf("关键帧提取失败: %v", err))
			return err
		}
		term.PrintSuccess(fmt.Sprintf("提取了 %d 个关键帧", len(frames)))

		// 生成摘要
		term.PrintStatus("正在生成摘要...")
		summary, err := client.GenerateSummary(frames, info)
		if err != nil {
			term.PrintError(fmt.Sprintf("摘要生成失败: %v", err))
			return err
		}

		// 显示结果
		term.PrintInfo(fmt.Sprintf("📹 视频时长: %s", formatDuration(summary.TotalDuration)))
		term.PrintInfo(fmt.Sprintf("🎞️ 分析片段: %d 个", summary.SegmentCount))
		term.PrintPanel("整体摘要", summary.OverallSummary)

		if len(summary.KeyScenes) > 0 {
			term.PrintInfo("\n🎬 关键场景")
			for i, scene := range summary.KeyScenes {
				term.PrintInfo(fmt.Sprintf("%d. %s - %s", i+1, formatDuration(scene.Timestamp), scene.Description))
			}
		}

		// 保存结果
		if output != "" {
			if err := saveSummary(summary, output); err != nil {
				term.PrintError(fmt.Sprintf("保存文件失败: %v", err))
			} else {
				term.PrintSuccess(fmt.Sprintf("📄 结果已保存到: %s", output))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(summaryCmd)
}

func formatDuration(seconds float64) string {
	minutes := int(seconds) / 60
	secs := int(seconds) % 60
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

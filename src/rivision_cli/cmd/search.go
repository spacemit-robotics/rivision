// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/rivision/rivision-cli/internal/gateway"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/rivision/rivision-cli/internal/video"
	"github.com/spf13/cobra"
)

var (
	searchQuery      string
	searchConfidence float64
	searchInterval   int
	searchMode       string
)

var searchCmd = &cobra.Command{
	Use:   "search [视频文件]",
	Short: "视频内容搜索",
	Long:  `在视频中搜索特定内容，支持快速和精确两种模式`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		videoPath := args[0]

		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "视频搜索")

		term.PrintInfo(fmt.Sprintf("🎬 搜索视频: %s", videoPath))
		term.PrintInfo(fmt.Sprintf("🔍 查询内容: %s", searchQuery))
		term.PrintInfo(fmt.Sprintf("⚙️ 搜索模式: %s", searchMode))

		// 创建客户端
		client := gateway.NewClient(gatewayURL)

		// 健康检查
		term.PrintStatus("检查Gateway状态...")
		if !client.HealthCheck() {
			term.PrintError("Gateway连接失败!")
			return fmt.Errorf("gateway连接失败")
		}
		term.PrintSuccess("Gateway连接成功")

		// 调整帧间隔
		interval := searchInterval
		if searchMode == "fast" {
			interval = max(interval*2, 60)
			term.PrintWarning(fmt.Sprintf("💡 快速模式: 帧间隔调整为 %d", interval))
		}

		// 提取视频帧
		term.PrintStatus("提取视频帧...")
		extractor := video.NewExtractor()
		frames, err := extractor.ExtractFrames(videoPath, interval)
		if err != nil {
			term.PrintError(fmt.Sprintf("视频帧提取失败: %v", err))
			return err
		}
		term.PrintSuccess(fmt.Sprintf("提取了 %d 帧", len(frames)))

		// 搜索分析
		term.PrintStatus("正在搜索视频内容...")
		results, err := client.SearchVideo(frames, searchQuery, searchConfidence)
		if err != nil {
			term.PrintError(fmt.Sprintf("视频搜索失败: %v", err))
			return err
		}

		// 显示结果
		if len(results) == 0 {
			term.PrintWarning("🔍 未找到匹配结果")
			term.PrintInfo("建议：")
			term.PrintInfo("• 尝试降低置信度阈值 --confidence 0.5")
			term.PrintInfo("• 使用更通用的查询词汇")
		} else {
			term.PrintSuccess(fmt.Sprintf("🎯 搜索完成！共找到 %d 个匹配结果", len(results)))
			for _, r := range results {
				term.PrintResult(r.Timestamp, r.Description, r.Confidence)
			}
		}

		// 保存结果
		if output != "" {
			if err := saveSearchResults(results, output); err != nil {
				term.PrintError(fmt.Sprintf("保存文件失败: %v", err))
			} else {
				term.PrintSuccess(fmt.Sprintf("📄 结果已保存到: %s", output))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVar(&searchQuery, "query", "", "搜索查询内容 (必需)")
	searchCmd.Flags().Float64Var(&searchConfidence, "confidence", 0.7, "置信度阈值")
	searchCmd.Flags().IntVar(&searchInterval, "frame-interval", 30, "视频抽帧间隔")
	searchCmd.Flags().StringVar(&searchMode, "mode", "precise", "搜索模式: fast 或 precise")
	searchCmd.MarkFlagRequired("query")
}

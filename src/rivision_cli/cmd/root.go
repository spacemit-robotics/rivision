// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	gatewayURL string
	verbose    bool
	output     string
)

var rootCmd = &cobra.Command{
	Use:   "rivision-cli",
	Short: "RiVision CLI - AI视频推理分析工具",
	Long: `RiVision CLI 是一个完整的命令行工具，用于AI视频推理分析。

支持功能:
  - 图片分析 (image)
  - 视频内容搜索 (search)
  - 视频摘要生成 (summary)
  - 目标检测 (detect)
  - 集成Web界面 (webui)
  - 节点管理 (node)
  - Token管理 (token)
  - 黑名单管理 (blacklist)

使用示例:
  rivision-cli image photo.jpg --query "描述这张图片"
  rivision-cli search video.mp4 --query "戴帽子的人"
  rivision-cli webui --with-go2rtc`,
	Version: "1.0.0",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&gatewayURL, "gateway-url", "http://localhost:8081", "Gateway服务地址")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "详细输出")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "", "输出文件路径")
}

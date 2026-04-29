package cmd

import (
	"fmt"

	"github.com/rivision/rivision-cli/internal/gateway"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/spf13/cobra"
)

var imageQuery string

var imageCmd = &cobra.Command{
	Use:   "image [图片文件]",
	Short: "单张图片分析",
	Long:  `使用AI分析单张图片内容`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imagePath := args[0]

		// 初始化UI
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "图片分析")

		// 检查文件
		term.PrintInfo(fmt.Sprintf("🖼️ 分析图片: %s", imagePath))
		term.PrintInfo(fmt.Sprintf("💭 查询内容: %s", imageQuery))

		// 创建客户端
		client := gateway.NewClient(gatewayURL)

		// 健康检查
		term.PrintStatus("检查Gateway状态...")
		if !client.HealthCheck() {
			term.PrintError("Gateway连接失败!")
			return fmt.Errorf("gateway连接失败")
		}
		term.PrintSuccess("Gateway连接成功")

		// 分析图片
		term.PrintStatus("正在分析图片...")
		result, err := client.AnalyzeImage(imagePath, imageQuery)
		if err != nil {
			term.PrintError(fmt.Sprintf("图片分析失败: %v", err))
			return err
		}

		// 显示结果
		term.PrintPanel("分析结果", result.Content)

		// 保存结果
		if output != "" {
			if err := saveResult(result, output); err != nil {
				term.PrintError(fmt.Sprintf("保存文件失败: %v", err))
			} else {
				term.PrintSuccess(fmt.Sprintf("📄 结果已保存到: %s", output))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(imageCmd)
	imageCmd.Flags().StringVar(&imageQuery, "query", "", "分析查询内容 (必需)")
	imageCmd.MarkFlagRequired("query")
}

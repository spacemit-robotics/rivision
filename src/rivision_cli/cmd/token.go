package cmd

import (
	"fmt"

	"github.com/rivision/rivision-cli/internal/gateway"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Token管理",
	Long:  `管理节点注册Token：生成、列表、撤销、清理、统计`,
}

var (
	tokenNodeID     string
	tokenHours      int
	tokenMaxUses    int
	tokenDesc       string
)

var tokenGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "生成节点注册Token",
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "生成Token")

		client := gateway.NewClient(gatewayURL)

		term.PrintInfo(fmt.Sprintf("🔐 为节点 %s 生成Token... (hours=%d, max_uses=%d)", tokenNodeID, tokenHours, tokenMaxUses))

		result, err := client.GenerateToken(tokenNodeID, tokenHours, tokenMaxUses, tokenDesc)
		if err != nil {
			term.PrintError(fmt.Sprintf("生成Token失败: %v", err))
			return err
		}

		term.PrintPanel("Token信息", fmt.Sprintf(
			"节点ID:       %s\n"+
				"Token:        %s\n"+
				"有效期:       %d小时\n"+
				"过期时间:     %s\n"+
				"最大使用次数: %d",
			tokenNodeID, result.Token, tokenHours, result.ExpiresAt, tokenMaxUses,
		))

		term.PrintInfo("\n将此Token配置到node-agent的REGISTRATION_TOKEN环境变量")

		return nil
	},
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有Token",
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "Token列表")

		client := gateway.NewClient(gatewayURL)

		term.PrintStatus("获取Token列表...")
		tokens, err := client.ListTokens()
		if err != nil {
			term.PrintError(fmt.Sprintf("获取Token列表失败: %v", err))
			return err
		}

		if len(tokens) == 0 {
			term.PrintWarning("暂无Token")
			return nil
		}

		term.PrintTable(
			[]string{"节点ID", "Token", "状态", "使用次数", "过期时间", "描述"},
			tokensToRows(tokens),
		)

		term.PrintInfo(fmt.Sprintf("\n共 %d 个Token", len(tokens)))

		return nil
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [节点ID]",
	Short: "撤销Token（仅有效Token）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID := args[0]
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "撤销Token")

		client := gateway.NewClient(gatewayURL)

		term.PrintInfo(fmt.Sprintf("🗑️ 撤销节点 %s 的Token...", nodeID))

		err := client.RevokeTokenByNodeID(nodeID)
		if err != nil {
			term.PrintError(fmt.Sprintf("撤销Token失败: %v", err))
			return err
		}

		term.PrintSuccess("✅ Token已撤销")

		return nil
	},
}

var tokenCleanupCmd = &cobra.Command{
	Use:   "cleanup [节点ID]",
	Short: "清理所有Token（包括无效的）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID := args[0]
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "清理Token")

		client := gateway.NewClient(gatewayURL)

		term.PrintInfo(fmt.Sprintf("🧹 清理节点 %s 的所有Token...", nodeID))

		err := client.CleanupTokens(nodeID)
		if err != nil {
			term.PrintError(fmt.Sprintf("清理Token失败: %v", err))
			return err
		}

		term.PrintSuccess("✅ Token已清理")

		return nil
	},
}

var tokenStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "查看注册统计",
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "注册统计")

		client := gateway.NewClient(gatewayURL)

		term.PrintStatus("获取注册统计...")
		stats, err := client.GetRegistrationStats()
		if err != nil {
			term.PrintError(fmt.Sprintf("获取统计失败: %v", err))
			return err
		}

		term.PrintPanel("节点注册统计", fmt.Sprintf(
			"已注册节点: %d\n"+
				"有效Token:  %d\n"+
				"黑名单IP:   %d\n"+
				"失败IP数:   %d",
			stats.RegisteredNodes, stats.ActiveTokens, stats.BlacklistedIPs, stats.TotalFailureIPs,
		))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)
	tokenCmd.AddCommand(tokenGenerateCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	tokenCmd.AddCommand(tokenCleanupCmd)
	tokenCmd.AddCommand(tokenStatsCmd)

	tokenGenerateCmd.Flags().StringVar(&tokenNodeID, "node-id", "", "节点ID (必需)")
	tokenGenerateCmd.Flags().IntVar(&tokenHours, "hours", 8760, "有效期（小时，默认1年）")
	tokenGenerateCmd.Flags().IntVar(&tokenMaxUses, "max-uses", 999, "最大使用次数")
	tokenGenerateCmd.Flags().StringVar(&tokenDesc, "description", "", "Token描述")
	tokenGenerateCmd.MarkFlagRequired("node-id")
}

func tokensToRows(tokens []gateway.TokenInfo) [][]string {
	rows := make([][]string, len(tokens))
	for i, t := range tokens {
		tokenDisplay := t.Token
		status := "✅ 有效"
		if !t.IsValid {
			status = "❌ 无效"
		}
		desc := t.Description
		if len(desc) > 20 {
			desc = desc[:20]
		}

		rows[i] = []string{
			t.NodeID,
			tokenDisplay,
			status,
			fmt.Sprintf("%d/%d", t.UsedCount, t.MaxUses),
			t.ExpiresAt,
			desc,
		}
	}
	return rows
}

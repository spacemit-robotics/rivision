// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/rivision/rivision-cli/internal/gateway"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/spf13/cobra"
)

var blacklistCmd = &cobra.Command{
	Use:   "blacklist",
	Short: "IP黑名单管理",
	Long:  `管理IP黑名单：查看、添加、移除`,
}

var blacklistReason string

var blacklistListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看黑名单",
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "黑名单列表")

		client := gateway.NewClient(gatewayURL)

		term.PrintStatus("获取黑名单列表...")
		items, err := client.ListBlacklist()
		if err != nil {
			term.PrintError(fmt.Sprintf("获取黑名单失败: %v", err))
			return err
		}

		if len(items) == 0 {
			term.PrintSuccess("✅ 黑名单为空")
			return nil
		}

		term.PrintTable(
			[]string{"IP地址", "原因", "添加时间", "失败次数"},
			blacklistToRows(items),
		)

		term.PrintInfo(fmt.Sprintf("\n共 %d 个黑名单IP", len(items)))

		return nil
	},
}

var blacklistAddCmd = &cobra.Command{
	Use:   "add [IP地址]",
	Short: "添加IP到黑名单",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := args[0]
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "添加黑名单")

		client := gateway.NewClient(gatewayURL)

		term.PrintInfo(fmt.Sprintf("🔒 添加到黑名单: %s", ip))

		err := client.AddToBlacklist(ip, blacklistReason)
		if err != nil {
			term.PrintError(fmt.Sprintf("添加黑名单失败: %v", err))
			return err
		}

		term.PrintSuccess("✅ 已添加到黑名单")

		return nil
	},
}

var blacklistRemoveCmd = &cobra.Command{
	Use:   "remove [IP地址]",
	Short: "从黑名单移除IP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := args[0]
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "移除黑名单")

		client := gateway.NewClient(gatewayURL)

		term.PrintInfo(fmt.Sprintf("🔓 从黑名单移除: %s", ip))

		err := client.RemoveFromBlacklist(ip)
		if err != nil {
			term.PrintError(fmt.Sprintf("移除黑名单失败: %v", err))
			return err
		}

		term.PrintSuccess("✅ 已从黑名单移除")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(blacklistCmd)
	blacklistCmd.AddCommand(blacklistListCmd)
	blacklistCmd.AddCommand(blacklistAddCmd)
	blacklistCmd.AddCommand(blacklistRemoveCmd)

	blacklistAddCmd.Flags().StringVar(&blacklistReason, "reason", "", "添加原因")
}

func blacklistToRows(items []gateway.BlacklistItem) [][]string {
	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = []string{
			item.IP,
			item.Reason,
			item.AddedAt,
			fmt.Sprintf("%d", item.FailureCount),
		}
	}
	return rows
}

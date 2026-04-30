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

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "节点管理",
	Long:  `管理计算节点：列表、状态、详情`,
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有节点",
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "节点列表")

		client := gateway.NewClient(gatewayURL)

		term.PrintStatus("获取节点列表...")
		nodes, err := client.GetNodes()
		if err != nil {
			term.PrintError(fmt.Sprintf("获取节点列表失败: %v", err))
			return err
		}

		if len(nodes) == 0 {
			term.PrintWarning("暂无可用节点")
			return nil
		}

		// 打印表格
		term.PrintTable(
			[]string{"#", "节点ID", "地址", "状态", "工作状态", "负载", "处理时间(s)"},
			nodesToRows(nodes),
		)

		// 汇总
		healthyCount := 0
		var totalLoad float64
		for _, n := range nodes {
			if n.Healthy {
				healthyCount++
				totalLoad += n.Load
			}
		}
		avgLoad := float64(0)
		if healthyCount > 0 {
			avgLoad = totalLoad / float64(healthyCount)
		}

		term.PrintInfo(fmt.Sprintf("\n在线节点: %d/%d", healthyCount, len(nodes)))
		term.PrintInfo(fmt.Sprintf("平均负载: %.1f%%", avgLoad))

		return nil
	},
}

var nodeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示节点状态概览",
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "节点状态")

		client := gateway.NewClient(gatewayURL)

		term.PrintStatus("获取节点状态...")
		nodes, err := client.GetNodes()
		if err != nil {
			term.PrintError(fmt.Sprintf("获取节点状态失败: %v", err))
			return err
		}

		if len(nodes) == 0 {
			term.PrintWarning("暂无可用节点")
			return nil
		}

		// 统计
		healthyCount := 0
		var totalLoad, totalTime float64
		timeCount := 0
		for _, n := range nodes {
			if n.Healthy {
				healthyCount++
				totalLoad += n.Load
			}
			if n.AvgProcessingTime > 0 {
				totalTime += n.AvgProcessingTime
				timeCount++
			}
		}

		avgLoad := float64(0)
		avgTime := float64(0)
		if healthyCount > 0 {
			avgLoad = totalLoad / float64(healthyCount)
		}
		if timeCount > 0 {
			avgTime = totalTime / float64(timeCount)
		}

		term.PrintPanel("节点状态概览", fmt.Sprintf(
			"在线节点:     %d/%d\n"+
				"平均负载:     %.1f%%\n"+
				"平均处理时间: %.1fs",
			healthyCount, len(nodes), avgLoad, avgTime,
		))

		// 节点详情
		for _, n := range nodes {
			status := "🟢"
			if !n.Healthy {
				status = "🔴"
			}
			workStatus := "空闲"
			if n.Load > 0 {
				workStatus = "处理中"
			}
			term.PrintInfo(fmt.Sprintf("  %s %s: %s (负载: %.1f%%)", status, n.ID, workStatus, n.Load))
		}

		return nil
	},
}

var nodeInfoNodeID string

var nodeInfoCmd = &cobra.Command{
	Use:   "info [节点ID]",
	Short: "查看单个节点详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID := args[0]
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "节点详情")

		client := gateway.NewClient(gatewayURL)

		term.PrintStatus(fmt.Sprintf("获取节点 %s 详情...", nodeID))
		node, err := client.GetNodeInfo(nodeID)
		if err != nil {
			term.PrintError(fmt.Sprintf("获取节点详情失败: %v", err))
			return err
		}

		if node == nil {
			term.PrintError(fmt.Sprintf("节点 %s 不存在", nodeID))
			return fmt.Errorf("节点不存在")
		}

		status := "🟢 在线"
		if !node.Healthy {
			status = "🔴 离线"
		}
		workStatus := "空闲"
		if node.Load > 0 {
			workStatus = "处理中"
		}

		term.PrintPanel(fmt.Sprintf("节点: %s", nodeID), fmt.Sprintf(
			"节点ID:       %s\n"+
				"地址:         %s:%d\n"+
				"状态:         %s\n"+
				"工作状态:     %s (负载: %.1f%%)\n"+
				"处理时间:     %.1fs\n"+
				"总处理数:     %d\n"+
				"失败数:       %d",
			node.ID, node.Host, node.Port, status, workStatus, node.Load,
			node.AvgProcessingTime, node.TotalProcessed, node.TotalFailed,
		))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
	nodeCmd.AddCommand(nodeListCmd)
	nodeCmd.AddCommand(nodeStatusCmd)
	nodeCmd.AddCommand(nodeInfoCmd)
}

func nodesToRows(nodes []gateway.Node) [][]string {
	rows := make([][]string, len(nodes))
	for i, n := range nodes {
		status := "🟢 在线"
		if !n.Healthy {
			status = "🔴 离线"
		}
		workStatus := "空闲"
		if n.Load > 0 {
			workStatus = "处理中"
		}
		avgTime := "-"
		if n.AvgProcessingTime > 0 {
			avgTime = fmt.Sprintf("%.1f", n.AvgProcessingTime)
		}

		rows[i] = []string{
			fmt.Sprintf("%d", i+1),
			n.ID,
			fmt.Sprintf("%s:%d", n.Host, n.Port),
			status,
			workStatus,
			fmt.Sprintf("%.1f%%", n.Load),
			avgTime,
		}
	}
	return rows
}

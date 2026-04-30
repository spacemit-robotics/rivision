// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("240")).
				Padding(0, 1)

	tableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)
)

type Terminal struct{}

func NewTerminal() *Terminal {
	return &Terminal{}
}

func (t *Terminal) PrintHeader(title, subtitle string) {
	header := headerStyle.Render(fmt.Sprintf("🎬 %s", title))
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(subtitle)
	fmt.Printf("%s %s\n\n", header, sub)
}

func (t *Terminal) PrintInfo(msg string) {
	fmt.Println(infoStyle.Render(msg))
}

func (t *Terminal) PrintSuccess(msg string) {
	fmt.Println(successStyle.Render(msg))
}

func (t *Terminal) PrintWarning(msg string) {
	fmt.Println(warningStyle.Render(msg))
}

func (t *Terminal) PrintError(msg string) {
	fmt.Println(errorStyle.Render("❌ " + msg))
}

func (t *Terminal) PrintStatus(msg string) {
	fmt.Println(statusStyle.Render("⏳ " + msg))
}

func (t *Terminal) PrintPanel(title, content string) {
	titleRendered := panelTitleStyle.Render(title)
	panel := panelStyle.Render(content)
	fmt.Printf("\n%s\n%s\n", titleRendered, panel)
}

func (t *Terminal) PrintSeparator() {
	fmt.Println(strings.Repeat("─", 60))
}

func (t *Terminal) PrintResult(timestamp float64, description string, confidence float64) {
	timeStr := fmt.Sprintf("%02d:%02d", int(timestamp)/60, int(timestamp)%60)
	fmt.Printf("  %s %s - %s (置信度: %.2f)\n",
		successStyle.Render("⏰"),
		successStyle.Render(timeStr),
		infoStyle.Render(description),
		confidence,
	)
}

func (t *Terminal) PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	// 计算每列宽度
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// 打印表头
	var headerCells []string
	for i, h := range headers {
		headerCells = append(headerCells, tableHeaderStyle.Width(colWidths[i]+2).Render(h))
	}
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))

	// 打印行
	for _, row := range rows {
		var cells []string
		for i, cell := range row {
			if i < len(colWidths) {
				cells = append(cells, tableCellStyle.Width(colWidths[i]+2).Render(cell))
			}
		}
		fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
}

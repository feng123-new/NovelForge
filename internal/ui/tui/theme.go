package tui

import "github.com/charmbracelet/lipgloss"

// 主题色板 — 暖调书卷气
var (
	colorText    = lipgloss.Color("#e8e0d0") // 羊皮纸白（略暖）
	colorDim     = lipgloss.Color("#5c5545") // 墨灰（偏暖）
	colorMuted   = lipgloss.Color("#a09880") // 柔和但可读（介于 dim 和 text 之间）
	colorAccent  = lipgloss.Color("#c9953c") // 古金（比琥珀黄更沉稳）
	colorAccent2 = lipgloss.Color("#7a9e7e") // 青竹（辅助色，用于装饰）
	colorSuccess = lipgloss.Color("#7a9e7e") // 竹绿（与 accent2 统一）
	colorError   = lipgloss.Color("#c45c4a") // 砖红（比朱红柔和）
	colorReview  = lipgloss.Color("#cc8844") // 赭橙
	colorContext = lipgloss.Color("#8b7bb5") // 藤紫（偏暖）
)

// 状态标签颜色映射
var statusColors = map[string]lipgloss.Color{
	"READY":    colorDim,
	"RUNNING":  colorSuccess,
	"REVIEW":   colorReview,
	"REWRITE":  colorReview,
	"COMPLETE": colorSuccess,
	"ERROR":    colorError,
}

// 事件分类颜色映射
var categoryColors = map[string]lipgloss.Color{
	"TOOL":    colorText,
	"SYSTEM":  colorAccent,
	"REVIEW":  colorReview,
	"CHECK":   colorSuccess,
	"ERROR":   colorError,
	"AGENT":   colorDim,
	"CONTEXT": colorContext,
}

// 基础样式
var (
	baseBorder = lipgloss.RoundedBorder()

	topBarStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(0, 1)

	statusCapsule = lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	fieldLabelStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Width(10)

	fieldValueStyle = lipgloss.NewStyle().
			Foreground(colorText)

	highlightValueStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	cardTitleStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true)

	cardContentStyle = lipgloss.NewStyle().
				Foreground(colorText)
)

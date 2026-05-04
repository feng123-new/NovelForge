package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/entry/startup"
	"github.com/voocel/ainovel-cli/internal/host"
)

type startupMode int

const (
	startupModeQuick startupMode = iota
	startupModeCoCreate
)

func (m startupMode) label() string {
	switch m {
	case startupModeCoCreate:
		return "共创规划"
	default:
		return "快速开始"
	}
}

func (m startupMode) subtitle() string {
	switch m {
	case startupModeCoCreate:
		return "先与 AI 对话澄清，再开始创作"
	default:
		return "一句话直接开始写"
	}
}

func placeholderForNewMode(mode startupMode) string {
	switch mode {
	case startupModeCoCreate:
		return "先输入你的核心想法，Enter 开始与 AI 共创"
	default:
		return "输入一句小说需求，Enter 直接开始创作"
	}
}

func placeholderForCoCreate(state *cocreateState) string {
	if state == nil {
		return placeholderForNewMode(startupModeCoCreate)
	}
	switch {
	case state.awaiting:
		return "AI 正在整理你的要求..."
	case state.canStart():
		return "继续补充，或按 Ctrl+S 开始创作"
	default:
		return "继续补充你的要求，Enter 发送给 AI"
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

type cocreateState struct {
	session  *startup.CoCreateSession
	awaiting bool
	reqID    int
	cancel   context.CancelFunc // 取消当前 LLM 请求
	deltaCh  chan cocreateStreamItem
	doneCh   chan cocreateDoneMsg
	promptVP viewport.Model
}

func newCoCreateState(initial string) *cocreateState {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	return &cocreateState{
		session:  startup.NewCoCreateSession(strings.TrimSpace(initial)),
		awaiting: true,
		promptVP: vp,
	}
}

func (s *cocreateState) appendUser(text string) {
	s.session.AppendUser(text)
}

func (s *cocreateState) apply(reply host.CoCreateReply) {
	s.awaiting = false
	s.session.ApplyReply(reply)
}

func (s *cocreateState) applyDelta(kind, text string) {
	s.session.ApplyDelta(kind, text)
}

func (s *cocreateState) canStart() bool {
	return s.session.CanStart()
}

func (s *cocreateState) initialInput() string {
	return s.session.InitialInput()
}

func (s *cocreateState) streamReply() string {
	return s.session.StreamReply()
}

func (s *cocreateState) draftPrompt() string {
	return s.session.DraftPrompt()
}

func (s *cocreateState) ready() bool {
	return s.session.Ready()
}

func (s *cocreateState) buildPlan() (startup.Plan, error) {
	return s.session.BuildPlan()
}

func renderStartupModeBar(width int, mode startupMode) string {
	quick := renderStartupModePill(mode == startupModeQuick, "快速开始")
	cocreate := renderStartupModePill(mode == startupModeCoCreate, "共创规划")
	title := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("启动模式")
	divider := lipgloss.NewStyle().
		Foreground(colorDim).
		Render("·")
	line := title + " " + divider + " " + quick + "  " + cocreate
	return lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		Render(line)
}

func renderStartupModePill(active bool, label string) string {
	style := lipgloss.NewStyle().Padding(0, 1)
	if active {
		style = style.Foreground(lipgloss.Color("#1c1a14")).Background(colorAccent).Bold(true)
	} else {
		style = style.Foreground(colorMuted)
	}
	return style.Render(label)
}

// coCreateColumns 把 modal 内容区切成左右两栏宽度。
// 左栏承载对话与输入框（上下叠），右栏承载创作指令草稿；总和等于 modal 内容宽。
func coCreateColumns(bodyW int) (leftW, rightW int) {
	leftW = bodyW * 58 / 100
	if leftW < 42 {
		leftW = bodyW / 2
	}
	rightW = bodyW - leftW
	if rightW < 28 {
		rightW = 28
		leftW = bodyW - rightW
	}
	return leftW, rightW
}

func renderCoCreateBody(width, height int, state *cocreateState, errMsg, inputView string, spinnerFrame int) string {
	if state == nil {
		return ""
	}
	leftW, rightW := coCreateColumns(width)

	// 右 border 由外层 leftCol 容器画，贯穿 body 顶到底；conversation 与 input
	// 都不画自己的右 border。input 仍是完整圆角框，左右各 1 列 margin 与
	// conversation 的 padding 对齐，看起来与两侧边线距离一致。
	// 共创模式下 textarea 固定 1 行（见 model.refitTextareaHeight 分支），
	// input 高度 = 1 (textarea) + 2 (top/bottom border) = 3 行，永不漂移。
	innerW := leftW - 1 // 给外层右竖线留 1 列

	inputBox := lipgloss.NewStyle().
		Width(innerW - 6). // -2 margin -2 padding -2 border
		Border(baseBorder).
		BorderForeground(colorDim).
		Padding(0, 1).
		Margin(0, 1).
		Render(inputView)

	convH := height - lipgloss.Height(inputBox)
	if convH < 4 {
		convH = 4
	}

	convPanel := renderCoCreateConversationPanel(innerW, convH, state, errMsg, spinnerFrame)
	leftCol := lipgloss.NewStyle().
		Border(baseBorder, false, true, false, false).
		BorderForeground(colorDim).
		Render(lipgloss.JoinVertical(lipgloss.Left, convPanel, inputBox))

	rightPanel := renderCoCreatePromptPanel(rightW, height, state)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightPanel)
}

func coCreateModalSize(width, height int) (boxW, boxH int) {
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 24
	}
	boxW = minInt(maxInt(width*76/100, 88), width-4)
	boxH = minInt(maxInt(height*72/100, 22), height-4)
	if boxW < 64 {
		boxW = maxInt(width-2, 42)
	}
	if boxH < 14 {
		boxH = maxInt(height-2, 12)
	}
	return boxW, boxH
}

// coCreateInputWidth 算出 textarea 实际可输入的字符宽度。
// 左栏装饰：外层右竖线 1 + input 左右 margin 2 + border 2 + padding 2 = 7 列；
// textarea 自身 prompt+cursor 占 2 列；所以 textareaW = leftW - 9。
func coCreateInputWidth(width, height int) int {
	boxW, _ := coCreateModalSize(width, height)
	bodyW := boxW - 4
	leftW, _ := coCreateColumns(bodyW)
	inputW := leftW - 9
	if inputW < 20 {
		inputW = 20
	}
	return inputW
}

func renderCoCreateModal(width, height int, state *cocreateState, errMsg, inputView string, spinnerFrame int, quitPending bool) string {
	if state == nil {
		return ""
	}

	boxW, boxH := coCreateModalSize(width, height)

	// title / subtitle / hint 放在 modal 外（上方与下方居中），让 modal 内部
	// 完全交给 body —— 左栏右竖线与右栏从 modal 顶贯穿到底。
	// modal 实际占用 = boxH (content) + 2 (padding 1*2) + 2 (border) = boxH+4 行；
	// 整体 stack = title(1) + subtitle(1) + 空(1) + modal(boxH+4) + 空(1) + hint(1) = boxH+9。
	// 因此把 boxH 减 5 行预算给 modal 外的装饰，避免溢出终端。
	contentH := boxH - 5
	if contentH < 10 {
		contentH = 10
	}

	headerStyle := lipgloss.NewStyle().Width(boxW).AlignHorizontal(lipgloss.Center)
	title := headerStyle.Foreground(colorMuted).Bold(true).Render("共创规划")
	subtitle := headerStyle.Foreground(colorDim).Italic(true).Render("先把需求聊清楚，再开始创作")

	var hintLine string
	hintStyle := lipgloss.NewStyle().Width(boxW).AlignHorizontal(lipgloss.Center)
	if quitPending {
		// quitPending 与 inputHints() 一致；否则共创 modal 盖住底栏，用户感受不到"再按一次 Ctrl+C"。
		hintLine = hintStyle.Foreground(lipgloss.Color("243")).Bold(true).Render("Press Ctrl+C again to exit")
	} else {
		hintLine = hintStyle.Foreground(colorDim).Italic(true).Render(coCreateHint(state))
	}

	body := renderCoCreateBody(boxW-4, contentH, state, errMsg, inputView, spinnerFrame)
	box := lipgloss.NewStyle().
		Width(boxW).
		Height(contentH).
		Border(baseBorder).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Render(body)

	stack := lipgloss.JoinVertical(lipgloss.Center, title, subtitle, "", box, "", hintLine)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, stack)
}

// coCreateHint 根据状态生成简短键位提示，避免与 placeholder 重复语义。
func coCreateHint(state *cocreateState) string {
	switch {
	case state == nil:
		return "Enter 发送 · Esc 退出"
	case state.awaiting:
		return "AI 回复中 · ↑↓ 滚动右侧 · Esc 退出"
	case state.canStart():
		return "Enter 继续补充 · Ctrl+S 开始创作 · ↑↓ 滚动 · Esc 退出"
	default:
		return "Enter 发送 · ↑↓ 滚动 · Esc 退出"
	}
}

func renderCoCreateConversationPanel(width, height int, state *cocreateState, errMsg string, spinnerFrame int) string {
	// 不画自己的 border —— 右竖线由外层 leftCol 容器统一画。
	// 内容宽 = 列总宽 - 2（仅 padding 0,1）；行内再扣 2 列前缀（"▌ "/"  "）。
	contentW := width - 2
	if contentW < 12 {
		contentW = 12
	}
	wrapW := max(12, contentW-2)

	userRole := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render("你")
	aiRole := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("AI")
	userBody := lipgloss.NewStyle().Foreground(colorAccent2)
	aiBody := lipgloss.NewStyle().Foreground(bodyTextColor)
	thinkingStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)
	thinkingTag := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Render("AI 思考")

	var lines []string
	for _, item := range state.session.History() {
		isUser := item.Role != "assistant"
		if isUser {
			lines = append(lines, userRole)
			for _, line := range wrapStreamText(strings.TrimSpace(item.Content), wrapW) {
				// 整行一次 Render，避免前缀颜色 reset 与正文颜色拼接处的 ANSI 控制符 bleed。
				lines = append(lines, userBody.Render("▌ "+line))
			}
		} else {
			lines = append(lines, aiRole)
			for _, line := range wrapStreamText(strings.TrimSpace(item.Content), wrapW) {
				lines = append(lines, aiBody.Render("  "+line))
			}
		}
		lines = append(lines, "")
	}

	if state.awaiting {
		if t := state.session.StreamThinking(); t != "" {
			lines = append(lines, thinkingTag)
			for _, line := range wrapStreamText(t, wrapW) {
				lines = append(lines, thinkingStyle.Render("  "+line))
			}
			lines = append(lines, "")
		}
		if state.streamReply() != "" {
			lines = append(lines, aiRole)
			for _, line := range wrapStreamText(state.streamReply(), wrapW) {
				lines = append(lines, aiBody.Render("  "+line))
			}
			lines = append(lines, "")
		}
		// sparkle 装饰：让用户始终看到"AI 在工作"
		lines = append(lines, strings.TrimLeft(renderEventSparkle(spinnerFrame, contentW), " "))
	}
	if errMsg != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render("! "+errMsg))
	}

	contentH := max(4, height-2)
	if len(lines) > contentH {
		lines = lines[len(lines)-contentH:]
	}
	content := strings.Join(lines, "\n")

	style := lipgloss.NewStyle().
		Width(contentW).
		Height(height).
		Padding(0, 1)
	return style.Render(panelTitleStyle.Render(":: 共创对话") + "\n" + content)
}

func renderCoCreatePromptPanel(width, height int, state *cocreateState) string {
	status := lipgloss.NewStyle().Foreground(colorDim).Render("继续对话中")
	if state.ready() {
		status = lipgloss.NewStyle().Foreground(colorAccent).Render("已可开始创作")
	}
	if state.awaiting {
		status = lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render("AI 整理中")
	}

	// 内容宽 = 列总宽 - 2（padding 0,1 占用 2 列，无 border）。
	contentW := width - 2
	if contentW < 8 {
		contentW = 8
	}

	text := strings.TrimSpace(state.draftPrompt())
	if text == "" {
		text = "AI 会在这里持续整理出一段可直接进入创作的最终指令。"
		text = lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render(text)
	} else {
		text = renderMarkdownPreview(text, max(12, contentW-2))
	}
	vpHeight := height - 5
	if vpHeight < 3 {
		vpHeight = 3
	}
	if state.promptVP.Width != contentW || state.promptVP.Height != vpHeight {
		state.promptVP.Width = contentW
		state.promptVP.Height = vpHeight
	}
	state.promptVP.MouseWheelEnabled = true
	state.promptVP.SetContent(text)

	hint := ""
	if state.promptVP.TotalLineCount() > state.promptVP.VisibleLineCount() {
		switch {
		case state.promptVP.AtTop():
			hint = "↓ 下方还有内容，可滚轮或 PgDn 查看"
		case state.promptVP.AtBottom():
			hint = "↑ 上方还有内容，可滚轮或 PgUp 查看"
		default:
			hint = "↑↓ 可继续滚动查看"
		}
	}

	style := lipgloss.NewStyle().
		Width(contentW).
		Height(height).
		Padding(0, 1)

	body := panelTitleStyle.Render(":: 当前创作指令") + "\n" + status + "\n\n" + state.promptVP.View()
	if hint != "" {
		body += "\n\n" + lipgloss.NewStyle().
			Width(contentW).
			AlignHorizontal(lipgloss.Center).
			Foreground(colorDim).
			Italic(true).
			Render(hint)
	}
	return style.Render(body)
}

func renderMarkdownPreview(text string, width int) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}

	h1Style := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	h2Style := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	h3Style := lipgloss.NewStyle().Foreground(colorMuted).Bold(true)
	bulletStyle := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	codeStyle := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	var out []string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			out = append(out, "")
			continue
		}

		switch {
		case strings.HasPrefix(line, "# "):
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			out = append(out, h1Style.Render(title))
		case strings.HasPrefix(line, "## "):
			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			out = append(out, h2Style.Render(title))
		case strings.HasPrefix(line, "### "):
			title := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			out = append(out, h3Style.Render(title))
		case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "):
			body := strings.TrimSpace(line[2:])
			wrapped := wrapStreamText(body, max(8, width-4))
			for i, item := range wrapped {
				if i == 0 {
					out = append(out, bulletStyle.Render("• ")+cardContentStyle.Render(item))
				} else {
					out = append(out, "  "+cardContentStyle.Render(item))
				}
			}
		case isOrderedMarkdownItem(line):
			prefix, body := splitOrderedMarkdownItem(line)
			wrapped := wrapStreamText(body, max(8, width-len(prefix)-2))
			for i, item := range wrapped {
				if i == 0 {
					out = append(out, bulletStyle.Render(prefix+" ")+cardContentStyle.Render(item))
				} else {
					out = append(out, strings.Repeat(" ", len(prefix)+1)+cardContentStyle.Render(item))
				}
			}
		case strings.HasPrefix(line, "> "):
			body := strings.TrimSpace(strings.TrimPrefix(line, "> "))
			for _, item := range wrapStreamText(body, max(8, width-4)) {
				out = append(out, codeStyle.Render("│ "+item))
			}
		default:
			for _, item := range wrapStreamText(line, width) {
				out = append(out, cardContentStyle.Render(item))
			}
		}
	}
	return strings.Join(out, "\n")
}

func isOrderedMarkdownItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
}

func splitOrderedMarkdownItem(line string) (prefix, body string) {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return "", strings.TrimSpace(line)
	}
	return line[:i+1], strings.TrimSpace(line[i+2:])
}

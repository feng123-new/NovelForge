package orchestrator

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/logger"
	"github.com/voocel/ainovel-cli/internal/tools"
)

// setupFileLogger 设置 CLI 模式日志，同时输出到 stderr 和日志文件。
func setupFileLogger(outputDir string) func() {
	return logger.SetupFile(outputDir, "cli.log", true)
}

// cliAskUserHandler 是 CLI 模式下的交互式选择器，上下键选择，回车确认。
func cliAskUserHandler(_ context.Context, questions []tools.Question) (*tools.AskUserResponse, error) {
	resp := &tools.AskUserResponse{
		Answers: make(map[string]string),
		Notes:   make(map[string]string),
	}
	for _, q := range questions {
		m := newSelectModel(q)
		p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
		final, err := p.Run()
		if err != nil {
			return resp, err
		}
		result := final.(selectModel)
		if result.cancelled {
			continue
		}
		resp.Answers[q.Question] = result.answer
		if result.isCustom {
			resp.Notes[q.Question] = result.answer
		}
	}
	return resp, nil
}

// ---------- 交互式选择器（bubbletea mini program）----------

var (
	selectCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	selectDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	selectInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

type selectModel struct {
	question  tools.Question
	items     []string // label 列表，最后一项是"自由输入"
	descs     []string // 描述列表
	cursor    int
	answer    string
	isCustom  bool
	cancelled bool
	typing    bool   // 是否进入自由输入模式
	input     string // 自由输入缓冲
}

func newSelectModel(q tools.Question) selectModel {
	items := make([]string, 0, len(q.Options)+1)
	descs := make([]string, 0, len(q.Options)+1)
	for _, opt := range q.Options {
		items = append(items, opt.Label)
		descs = append(descs, opt.Description)
	}
	items = append(items, "自由输入")
	descs = append(descs, "以上都不合适，我自己写")
	return selectModel{question: q, items: items, descs: descs}
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.typing {
		return m.updateTyping(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor == len(m.items)-1 {
				m.typing = true
				return m, nil
			}
			m.answer = m.items[m.cursor]
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectModel) updateTyping(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := strings.TrimSpace(m.input)
			if text == "" {
				return m, nil
			}
			m.answer = text
			m.isCustom = true
			return m, tea.Quit
		case "esc":
			m.typing = false
			m.input = ""
			return m, nil
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "backspace":
			if len(m.input) > 0 {
				runes := []rune(m.input)
				m.input = string(runes[:len(runes)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.input += " "
			}
		}
	}
	return m, nil
}

func (m selectModel) View() string {
	var b strings.Builder
	b.WriteString(selectHeaderStyle.Render(fmt.Sprintf("[%s] %s", m.question.Header, m.question.Question)))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = selectCursorStyle.Render("❯ ")
		}
		label := item
		if i == m.cursor {
			label = selectCursorStyle.Render(item)
		}
		desc := selectDescStyle.Render(" " + m.descs[i])
		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, label, desc))
	}

	if m.typing {
		b.WriteString("\n")
		b.WriteString(selectInputStyle.Render("  ✎ "))
		b.WriteString(m.input)
		b.WriteString(selectCursorStyle.Render("▌"))
		b.WriteString(selectDescStyle.Render("  (Enter 确认, Esc 返回)"))
	} else {
		b.WriteString(selectDescStyle.Render("\n  ↑↓ 选择  Enter 确认  Esc 取消"))
	}

	return b.String()
}

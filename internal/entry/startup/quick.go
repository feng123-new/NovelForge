package startup

import (
	"fmt"
	"os"
	"strings"
)

// LoadPromptFile 读取文件作为初始创作要求。
func LoadPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取 prompt 失败: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// PrepareQuick 将直接输入整理为可进入 Engine 的快速启动计划。
func PrepareQuick(req Request) (Plan, error) {
	prompt := strings.TrimSpace(req.UserPrompt)
	if prompt == "" {
		return Plan{}, fmt.Errorf("prompt is required")
	}
	return Plan{
		Mode:        ModeQuick,
		DisplayName: "快速开始",
		RawPrompt:   prompt,
	}, nil
}

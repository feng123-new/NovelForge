package main

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/app"
	"github.com/voocel/ainovel-cli/tools"
	"github.com/voocel/ainovel-cli/tui"
)

//go:embed prompts/*.md
var promptsFS embed.FS

//go:embed references
var referencesFS embed.FS

//go:embed styles/*.md
var stylesFS embed.FS

func main() {
	configPath, args := parseFlags()

	// 首次引导
	if app.NeedsSetup(configPath) {
		setupCfg, err := app.RunSetup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "setup: %v\n", err)
			os.Exit(1)
		}
		// 引导完成后使用生成的配置继续
		runWithConfig(setupCfg, args)
		return
	}

	// 加载配置
	cfg, err := app.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	runWithConfig(cfg, args)
}

func runWithConfig(cfg app.Config, args []string) {
	refs := loadReferences(cfg.Style)
	prompts := loadPrompts()
	styles := loadStyles()

	prompt := strings.Join(args, " ")
	if prompt != "" {
		// CLI 模式：有命令行参数，直接运行
		cfg.Prompt = prompt
		if err := app.Run(cfg, refs, prompts, styles); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// TUI 模式：无命令行参数，启动交互界面
	if err := tui.Run(cfg, refs, prompts, styles); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// parseFlags 提取 --config 参数，返回配置路径和剩余参数。
func parseFlags() (configPath string, args []string) {
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
			continue
		}
		args = append(args, os.Args[i])
	}
	return
}

func loadReferences(style string) tools.References {
	if style == "" {
		style = "default"
	}
	refs := tools.References{
		ChapterGuide:      mustRead(referencesFS, "references/chapter-guide.md"),
		HookTechniques:    mustRead(referencesFS, "references/hook-techniques.md"),
		QualityChecklist:  mustRead(referencesFS, "references/quality-checklist.md"),
		OutlineTemplate:   mustRead(referencesFS, "references/outline-template.md"),
		CharacterTemplate: mustRead(referencesFS, "references/character-template.md"),
		ChapterTemplate:   mustRead(referencesFS, "references/chapter-template.md"),
		Consistency:       mustRead(referencesFS, "references/consistency.md"),
		ContentExpansion:  mustRead(referencesFS, "references/content-expansion.md"),
		DialogueWriting:   mustRead(referencesFS, "references/dialogue-writing.md"),
		LongformPlanning:  mustRead(referencesFS, "references/longform-planning.md"),
		Differentiation:   mustRead(referencesFS, "references/differentiation.md"),
	}
	if style != "" && style != "default" {
		path := "references/" + style + "/style-references.md"
		if data, err := referencesFS.ReadFile(path); err == nil {
			refs.StyleReference = string(data)
		}
	}
	return refs
}

func loadPrompts() app.Prompts {
	return app.Prompts{
		Coordinator:    mustRead(promptsFS, "prompts/coordinator.md"),
		ArchitectShort: mustRead(promptsFS, "prompts/architect-short.md"),
		ArchitectMid:   mustRead(promptsFS, "prompts/architect-mid.md"),
		ArchitectLong:  mustRead(promptsFS, "prompts/architect-long.md"),
		Writer:         mustRead(promptsFS, "prompts/writer.md"),
		Editor:         mustRead(promptsFS, "prompts/editor.md"),
	}
}

func mustRead(fs embed.FS, path string) string {
	data, err := fs.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("embed read %s: %v", path, err))
	}
	return string(data)
}

func loadStyles() map[string]string {
	styles := make(map[string]string)
	entries, err := stylesFS.ReadDir("styles")
	if err != nil {
		return styles
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := stylesFS.ReadFile("styles/" + e.Name())
		if err != nil {
			continue
		}
		styles[name] = string(data)
	}
	return styles
}

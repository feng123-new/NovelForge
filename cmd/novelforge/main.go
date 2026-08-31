// NovelForge is derived from the ainovel-cli command under the Apache License 2.0.
// The legacy command remains available at cmd/ainovel-cli for upstream compatibility.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/entry/headless"
	"github.com/voocel/ainovel-cli/internal/entry/startup"
	"github.com/voocel/ainovel-cli/internal/entry/tui"
	"github.com/voocel/ainovel-cli/internal/eval"
	"github.com/voocel/ainovel-cli/internal/rules"
	buildversion "github.com/voocel/ainovel-cli/internal/version"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// headlessMode records whether the process uses non-interactive execution so
// fatal errors never wait for input in automation or containers.
var headlessMode bool

func main() {
	// Commands with their own flag grammar are intercepted before legacy option
	// parsing. The original TUI/headless behavior remains the default path.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			os.Exit(runServerCommand(os.Args[2:]))
		case "eval":
			os.Exit(eval.Command(os.Args[2:]))
		}
	}

	opts, args, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		die("flags: %v", err)
	}
	if opts.Version {
		printVersion(os.Stdout, versionInfo())
		return
	}
	if opts.Update {
		if err := runSelfUpdate(opts.UpdateVersion); err != nil {
			fmt.Fprintf(os.Stderr, "update: %v\n", err)
			os.Exit(1)
		}
		return
	}
	headlessMode = opts.Headless

	// Keep the mature ainovel first-run setup and ~/.ainovel configuration path
	// intact in Phase 1. This lets existing users run NovelForge without moving
	// credentials or project settings; a backed-up ~/.novelforge migration is a
	// separate compatibility phase.
	if bootstrap.NeedsSetup() {
		if opts.Headless {
			die("error: headless 模式不支持首次引导，请先运行一次 TUI 完成配置")
		}
		setupCfg, err := bootstrap.RunSetup()
		if err != nil {
			die("setup: %v", err)
		}
		runWithConfig(setupCfg, opts, args)
		return
	}

	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		die("config: %v", err)
	}
	runWithConfig(cfg, opts, args)
}

func die(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, msg)
	if path := bootstrap.WriteStartupError(msg); path != "" {
		fmt.Fprintf(os.Stderr, "（详细错误已记录到 %s）\n", path)
	}
	if !headlessMode && stdinIsTerminal() {
		fmt.Fprint(os.Stderr, "\n按回车键退出...")
		fmt.Fscanln(os.Stdin)
	}
	os.Exit(1)
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runWithConfig(cfg bootstrap.Config, opts cliOptions, args []string) {
	rules.EnsureHomeRulesDir()

	if len(args) > 0 {
		die("error: 不再支持命令行直接传入小说需求，请启动后在 TUI 输入框中输入")
	}

	cfg.FillDefaults()
	bundle := assets.Load(cfg.Style, assets.DefaultLoadOptions(cfg.OutputDir))
	if opts.Headless {
		prompt, err := loadPrompt(opts)
		if err != nil {
			die("error: %v", err)
		}
		if err := headless.Run(cfg, bundle, headless.Options{Prompt: prompt}); err != nil {
			die("error: %v", err)
		}
		return
	}
	if opts.Prompt != "" || opts.PromptFile != "" {
		die("error: --prompt/--prompt-file 仅能在 --headless 模式下使用")
	}
	if err := tui.Run(cfg, bundle, versionInfo()); err != nil {
		die("error: %v", err)
	}
}

type cliOptions struct {
	Headless      bool
	Prompt        string
	PromptFile    string
	Version       bool
	Update        bool
	UpdateVersion string
}

func parseCLIOptions(argv []string) (cliOptions, []string, error) {
	var opts cliOptions
	var args []string
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--version", "-v":
			opts.Version = true
		case "version":
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("version 不接受参数")
			}
			opts.Version = true
		case "update":
			if opts.Update {
				return opts, nil, fmt.Errorf("update 只能指定一次")
			}
			opts.Update = true
			if i+1 < len(argv) {
				if strings.HasPrefix(argv[i+1], "-") {
					return opts, nil, fmt.Errorf("update 只接受一个可选版本参数")
				}
				opts.UpdateVersion = argv[i+1]
				i++
			}
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("update 只接受一个可选版本参数")
			}
		case "--headless":
			opts.Headless = true
		case "--prompt":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--prompt 缺少值")
			}
			opts.Prompt = argv[i+1]
			i++
		case "--prompt-file":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--prompt-file 缺少值")
			}
			opts.PromptFile = argv[i+1]
			i++
		default:
			args = append(args, argv[i])
		}
	}
	if opts.Prompt != "" && opts.PromptFile != "" {
		return opts, nil, fmt.Errorf("--prompt 和 --prompt-file 不能同时使用")
	}
	if opts.Version && (opts.Update || opts.Headless || opts.Prompt != "" || opts.PromptFile != "" || len(args) > 0) {
		return opts, nil, fmt.Errorf("version 不能与其他启动参数混用")
	}
	if opts.Update && (opts.Headless || opts.Prompt != "" || opts.PromptFile != "" || len(args) > 0) {
		return opts, nil, fmt.Errorf("update 不能与其他启动参数混用")
	}
	return opts, args, nil
}

func versionInfo() buildversion.Info {
	return buildversion.Resolve(buildversion.Info{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
}

func printVersion(w io.Writer, info buildversion.Info) {
	info = buildversion.Resolve(info)
	fmt.Fprintf(w, "novelforge %s\ncommit: %s\nbuilt: %s\n", info.Version, info.Commit, info.Date)
}

func runSelfUpdate(target string) error {
	info := versionInfo()
	result, err := buildversion.Update(context.Background(), buildversion.UpdateOptions{
		Repo:           "feng123-new/NovelForge",
		BinaryName:     "novelforge",
		TargetVersion:  target,
		CurrentVersion: info.Version,
	})
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Printf("NovelForge 已是最新版本 %s\n", result.Version)
		return nil
	}
	fmt.Printf("NovelForge 已更新到 %s\n", result.Version)
	fmt.Printf("安装位置：%s\n", result.Path)
	return nil
}

func loadPrompt(opts cliOptions) (string, error) {
	return loadPromptFrom(opts, os.Stdin)
}

func loadPromptFrom(opts cliOptions, stdin io.Reader) (string, error) {
	if opts.PromptFile == "" {
		return strings.TrimSpace(opts.Prompt), nil
	}
	if opts.PromptFile == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("读取 prompt 失败: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return startup.LoadPromptFile(opts.PromptFile)
}

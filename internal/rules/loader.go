package rules

import (
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/compat"
)

// LoadOptions enumerates rule source directories for RawFileSources.
type LoadOptions struct {
	HomeRulesDir    string
	ProjectRulesDir string
}

// DefaultProjectRulesDir returns the selected project rules directory. The
// NovelForge command prefers .novelforge/rules, falls back to .ainovel/rules,
// and creates neither directory while resolving. The legacy command remains on
// .ainovel/rules.
func DefaultProjectRulesDir(projectDir string) string {
	if projectDir == "" {
		return ""
	}
	if !compat.NovelForgeRuntimeActive() {
		return filepath.Join(projectDir, compat.LegacyDirName, "rules")
	}
	return preferredRulesDir(
		filepath.Join(projectDir, compat.ProductDirName, "rules"),
		filepath.Join(projectDir, compat.LegacyDirName, "rules"),
	)
}

// DefaultHomeRulesDir returns the selected global rules directory using the
// same new-path-first compatibility contract.
func DefaultHomeRulesDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	if !compat.NovelForgeRuntimeActive() {
		return filepath.Join(home, compat.LegacyDirName, "rules")
	}
	return preferredRulesDir(
		filepath.Join(home, compat.ProductDirName, "rules"),
		filepath.Join(home, compat.LegacyDirName, "rules"),
	)
}

func preferredRulesDir(preferred, legacy string) string {
	if info, err := os.Stat(preferred); err == nil && info.IsDir() {
		return preferred
	}
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		return legacy
	}
	return preferred
}

const legacyHomeRulesReadme = `这里放全局写作偏好，跨所有书生效。

新建一个 .md 文件（如 my-style.md），用大白话写要求就行——
不需要任何格式、不需要 YAML：

    # 角色
    - 主角林尘别写成圣母，外冷内热即可
    # 风格
    - 多用身体感知（指节发白）替代情绪标签（紧张）
    - 对话别太书面，每章 3000 字左右
    - 不要出现"某种程度上"这种 AI 腔

写完不用管格式：系统会用模型把这些自然语言要求归一化成结构化约束
（字数范围、禁用词、疲劳词阈值等），写作时自动遵循、提交时自动自检。

多个 .md 按文件名字典序合并；点开头的隐藏文件、非 .md 文件都会被忽略
（所以这份 README.txt 不会被当成规则）。

常见 AI 套句、疲劳词的机械基线已内置，开箱即用，不写也没关系。

加载优先级（高 → 低）：./.ainovel/rules/*.md（本书） > ~/.ainovel/rules/*.md（这里） > 内置默认
`

const homeRulesReadme = legacyHomeRulesReadme

const novelForgeHomeRulesReadme = `这里放 NovelForge 全局写作偏好，跨所有书生效。

新建一个 .md 文件（如 my-style.md），用大白话写要求就行——
不需要任何格式、不需要 YAML：

    # 角色
    - 主角林尘别写成圣母，外冷内热即可
    # 风格
    - 多用身体感知（指节发白）替代情绪标签（紧张）
    - 对话别太书面，每章 3000 字左右
    - 不要出现"某种程度上"这种 AI 腔

写完不用管格式：系统会用模型把这些自然语言要求归一化成结构化约束
（字数范围、禁用词、疲劳词阈值等），写作时自动遵循、提交时自动自检。

多个 .md 按文件名字典序合并；点开头的隐藏文件、非 .md 文件都会被忽略
（所以这份 README.txt 不会被当成规则）。

常见 AI 套句、疲劳词的机械基线已内置，开箱即用，不写也没关系。

NovelForge 在每个层级优先使用 .novelforge/rules；目录不存在时兼容读取 .ainovel/rules。
项目规则优先于全局规则。新旧同层级不会合并，避免规则来源含糊。
`

func currentHomeRulesReadme() string {
	if compat.NovelForgeRuntimeActive() {
		return novelForgeHomeRulesReadme
	}
	return legacyHomeRulesReadme
}

// EnsureHomeRulesDir best-effort creates the selected global rules directory
// and refreshes its generated README. It never copies legacy user rules.
func EnsureHomeRulesDir() {
	if dir := DefaultHomeRulesDir(); dir != "" {
		_ = ensureRulesDirAt(dir)
	}
}

func ensureRulesDirAt(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.txt"), []byte(currentHomeRulesReadme()), 0o644)
}

// DefaultOptions binds project rules to cwd and applies the active command's
// compatibility profile once at startup.
func DefaultOptions() LoadOptions {
	cwd, _ := os.Getwd()
	return LoadOptions{
		HomeRulesDir:    DefaultHomeRulesDir(),
		ProjectRulesDir: DefaultProjectRulesDir(cwd),
	}
}

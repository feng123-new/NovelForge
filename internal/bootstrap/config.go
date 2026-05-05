package bootstrap

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/apperr"
	"github.com/voocel/ainovel-cli/internal/models"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// DefaultContextWindow 模型未在 registry 登记时的兜底窗口大小。
const DefaultContextWindow = 128000

// ProviderConfig 定义单个 LLM 提供商的凭证。
type ProviderConfig struct {
	Type    string   `json:"type,omitempty"`     // API 协议类型（openai/anthropic/gemini），自定义代理时指定
	APIKey  string   `json:"api_key,omitempty"`  // API Key
	BaseURL string   `json:"base_url,omitempty"` // API Base URL
	Models  []string `json:"models,omitempty"`   // 可选模型列表，供 TUI 切换时展示
}

// RequiresAPIKey 返回该 provider 是否必须显式配置 api_key。
// 约定：
// 1. ollama / bedrock 允许无 key；
// 2. 显式指定 Type 的配置视为自定义代理，允许无 key；
// 3. 其他 provider 默认要求 key，保持对官方托管接口的保守校验。
func (pc ProviderConfig) RequiresAPIKey(name string) bool {
	switch name {
	case "ollama", "bedrock":
		return false
	}
	if pc.Type != "" {
		return false
	}
	return true
}

// ProviderType 返回有效的 API 协议类型。
// 优先使用显式 Type，否则从 provider 名称推断。
func (pc ProviderConfig) ProviderType(name string) (string, error) {
	if pc.Type != "" {
		return pc.Type, nil
	}
	if _, ok := knownProviderTypes[name]; ok {
		return name, nil
	}
	return "", apperr.New(
		apperr.CodeProviderInvalid,
		"bootstrap.provider_type",
		fmt.Sprintf("provider %q 缺少 type，且不在已知 provider 列表中", name),
	)
}

// knownProviderTypes 已知 provider 名称到 API 协议类型的映射。
var knownProviderTypes = map[string]bool{
	"openai":     true,
	"anthropic":  true,
	"gemini":     true,
	"openrouter": true,
	"deepseek":   true,
	"qwen":       true,
	"glm":        true,
	"grok":       true,
	"mimo":       true,
	"ollama":     true,
	"bedrock":    true,
}

// ModelRef 表示一个 provider/model 组合。
type ModelRef struct {
	Provider string `json:"provider"` // provider 名称（Providers map 中的 key）
	Model    string `json:"model"`    // 模型名（原样透传，不做任何解析）
}

// RoleConfig 定义单个角色的模型覆盖。
type RoleConfig struct {
	Provider  string     `json:"provider"`            // 主 provider 名称（Providers map 中的 key）
	Model     string     `json:"model"`               // 主模型名（原样透传，不做任何解析）
	Fallbacks []ModelRef `json:"fallbacks,omitempty"` // 显式备用 provider/model 列表
}

// knownRoles 支持的角色名。
var knownRoles = map[string]bool{
	"coordinator": true,
	"architect":   true,
	"writer":      true,
	"editor":      true,
}

// Config 小说应用配置。
type Config struct {
	// 运行时字段（不序列化到 JSON）
	OutputDir string `json:"-"` // 输出根目录

	// 默认 LLM 配置
	Provider  string `json:"provider"` // 默认 provider（Providers map 中的 key）
	ModelName string `json:"model"`    // 默认模型名

	// Provider 凭证库
	Providers map[string]ProviderConfig `json:"providers,omitempty"`

	// 角色级模型覆盖
	Roles map[string]RoleConfig `json:"roles,omitempty"`

	// 创作参数
	Style string `json:"style,omitempty"`
}

// ValidateBase 校验基础配置。
func (c *Config) ValidateBase() error {
	if err := validateConfigText("provider", c.Provider); err != nil {
		return err
	}
	if err := validateConfigText("model", c.ModelName); err != nil {
		return err
	}

	if c.Provider == "" {
		return apperr.New(apperr.CodeConfigInvalid, "bootstrap.validate_base", "provider is required")
	}
	if c.ModelName == "" {
		return apperr.New(apperr.CodeConfigInvalid, "bootstrap.validate_base", "model is required")
	}

	// 默认 provider 必须有凭证
	pc, ok := c.Providers[c.Provider]
	if !ok {
		return apperr.New(
			apperr.CodeConfigInvalid,
			"bootstrap.validate_base",
			fmt.Sprintf("provider %q is not configured in providers", c.Provider),
		)
	}
	if pc.RequiresAPIKey(c.Provider) && pc.APIKey == "" {
		return apperr.New(
			apperr.CodeConfigInvalid,
			"bootstrap.validate_base",
			fmt.Sprintf("provider %q has no api_key configured", c.Provider),
		)
	}
	if err := validateProviderConfigText(c.Provider, pc); err != nil {
		return err
	}
	for name, provider := range c.Providers {
		if err := validateConfigText("provider name", name); err != nil {
			return err
		}
		if err := validateProviderConfigText(name, provider); err != nil {
			return err
		}
	}

	// 校验角色覆盖
	for role, rc := range c.Roles {
		if err := validateConfigText("role name", role); err != nil {
			return err
		}
		if err := validateConfigText(fmt.Sprintf("role %q provider", role), rc.Provider); err != nil {
			return err
		}
		if err := validateConfigText(fmt.Sprintf("role %q model", role), rc.Model); err != nil {
			return err
		}
		if !knownRoles[role] {
			return apperr.New(
				apperr.CodeConfigInvalid,
				"bootstrap.validate_base",
				fmt.Sprintf("unknown role %q in roles config (valid: coordinator/architect/writer/editor)", role),
			)
		}
		if rc.Provider == "" || rc.Model == "" {
			return apperr.New(
				apperr.CodeConfigInvalid,
				"bootstrap.validate_base",
				fmt.Sprintf("role %q must have both provider and model", role),
			)
		}
		if err := c.validateModelRef(
			fmt.Sprintf("role %q", role),
			ModelRef{Provider: rc.Provider, Model: rc.Model},
		); err != nil {
			return err
		}
		for i, fallback := range rc.Fallbacks {
			if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] provider", role, i), fallback.Provider); err != nil {
				return err
			}
			if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] model", role, i), fallback.Model); err != nil {
				return err
			}
			if err := c.validateModelRef(
				fmt.Sprintf("role %q fallback[%d]", role, i),
				fallback,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateProviderConfigText(name string, pc ProviderConfig) error {
	fields := []struct {
		label string
		value string
	}{
		{label: fmt.Sprintf("provider %q type", name), value: pc.Type},
		{label: fmt.Sprintf("provider %q api_key", name), value: pc.APIKey},
		{label: fmt.Sprintf("provider %q base_url", name), value: pc.BaseURL},
	}
	for _, field := range fields {
		if err := validateConfigText(field.label, field.value); err != nil {
			return err
		}
	}
	for i, model := range pc.Models {
		if err := validateConfigText(fmt.Sprintf("provider %q models[%d]", name, i), model); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigText(name, value string) error {
	if utils.ContainsControl(value) {
		return apperr.New(
			apperr.CodeConfigInvalid,
			"bootstrap.validate_base",
			fmt.Sprintf("%s contains control character", name),
		)
	}
	return nil
}

// DefaultProviderConfig 返回默认 provider 的凭证配置。
func (c *Config) DefaultProviderConfig() ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[c.Provider]
}

// FillDefaults 填充默认值。
func (c *Config) FillDefaults() {
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join("output", "novel")
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	if c.Roles == nil {
		c.Roles = make(map[string]RoleConfig)
	}
	if c.Style == "" {
		c.Style = "default"
	}
}

// ContextWindowSource 标记窗口取值的来源，供日志/诊断使用。
type ContextWindowSource string

const (
	CtxWindowRegistry ContextWindowSource = "registry" // OpenRouter 基线命中
	CtxWindowDefault  ContextWindowSource = "default"  // 兜底（自定义代理/未知模型）
)

// ResolveContextWindow 按模型名解析上下文窗口：
//  1. models.DefaultRegistry 按模型名查询（OpenRouter 基线 + 24h 刷新）
//  2. 兜底 DefaultContextWindow（自定义代理 / 未知模型）
//
// 不再支持配置文件全局覆盖：本应用是多模型架构，运行时随 /model 切换；
// 用一个静态 context_window 钉死所有模型违背设计意图。自定义代理 / 未登记
// 模型如需精确窗口，应在 models.DefaultRegistry 注册条目，而非配置全局值。
func (c Config) ResolveContextWindow(modelName string) (int, ContextWindowSource) {
	if w := models.DefaultRegistry().ResolveContextWindow(modelName); w > 0 {
		return w, CtxWindowRegistry
	}
	return DefaultContextWindow, CtxWindowDefault
}

// LogContextWindowChoice 打印某个角色的窗口决策。source=default 时发 Warn 提示
// 该模型未在 registry 命中（OpenRouter 也未收录），后续上下文压缩会按 128k 兜底
// 触发——若模型实际窗口更大，长篇可能被提前压缩、丢史。
func LogContextWindowChoice(role, model string, window int, source ContextWindowSource) {
	attrs := []any{"module", "context", "role", role, "model", model, "window", window, "source", source}
	if source == CtxWindowDefault {
		slog.Warn("未识别的模型，使用 128k 兜底窗口（自定义代理或 OpenRouter 未收录）", attrs...)
		return
	}
	slog.Info("上下文窗口", attrs...)
}

// CandidateModels 返回某个 provider 下可供切换的模型列表。
// 优先使用 provider 显式声明的 models；同时补充当前配置中已出现过的该 provider 模型。
func (c Config) CandidateModels(provider string) []string {
	if provider == "" {
		return nil
	}

	seen := make(map[string]bool)
	models := make([]string, 0, 4)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		models = append(models, model)
	}

	if pc, ok := c.Providers[provider]; ok {
		for _, model := range pc.Models {
			add(model)
		}
	}
	if c.Provider == provider {
		add(c.ModelName)
	}
	for _, rc := range c.Roles {
		if rc.Provider == provider {
			add(rc.Model)
		}
		for _, fallback := range rc.Fallbacks {
			if fallback.Provider == provider {
				add(fallback.Model)
			}
		}
	}
	return models
}

func (c Config) validateModelRef(owner string, ref ModelRef) error {
	if ref.Provider == "" || ref.Model == "" {
		return apperr.New(
			apperr.CodeConfigInvalid,
			"bootstrap.validate_base",
			fmt.Sprintf("%s must have both provider and model", owner),
		)
	}

	pc, ok := c.Providers[ref.Provider]
	if !ok {
		return apperr.New(
			apperr.CodeConfigInvalid,
			"bootstrap.validate_base",
			fmt.Sprintf("%s references provider %q which is not configured", owner, ref.Provider),
		)
	}
	if pc.RequiresAPIKey(ref.Provider) && pc.APIKey == "" {
		return apperr.New(
			apperr.CodeConfigInvalid,
			"bootstrap.validate_base",
			fmt.Sprintf("%s references provider %q which has no api_key", owner, ref.Provider),
		)
	}
	return nil
}

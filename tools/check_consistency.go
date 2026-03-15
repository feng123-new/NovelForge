package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/state"
)

// CheckConsistencyTool 返回章节内容和全部状态数据，供 Agent 自行对照判断。
// 纯 IO 工具：只负责加载数据，不注入指令。
type CheckConsistencyTool struct {
	store *state.Store
}

func NewCheckConsistencyTool(store *state.Store) *CheckConsistencyTool {
	return &CheckConsistencyTool{store: store}
}

func (t *CheckConsistencyTool) Name() string { return "check_consistency" }
func (t *CheckConsistencyTool) Description() string {
	return "加载章节内容和全部状态数据（时间线、伏笔、关系、世界规则、角色状态），供你自行对照检查一致性"
}
func (t *CheckConsistencyTool) Label() string { return "一致性检查" }

func (t *CheckConsistencyTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("要检查的章节号")).Required(),
	)
}

func (t *CheckConsistencyTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter int `json:"chapter"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0")
	}

	result := map[string]any{"chapter": a.Chapter}

	// 章节内容
	content, wordCount, err := t.store.LoadChapterContent(a.Chapter)
	if err != nil {
		return nil, fmt.Errorf("load chapter content: %w", err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d", a.Chapter)
	}
	result["content"] = content
	result["word_count"] = wordCount

	// 状态数据（全部加载，Agent 自行决定怎么用）
	if timeline, _ := t.store.LoadTimeline(); len(timeline) > 0 {
		result["timeline"] = timeline
	}
	if foreshadow, _ := t.store.LoadForeshadowLedger(); len(foreshadow) > 0 {
		result["foreshadow_ledger"] = foreshadow
	}
	if relationships, _ := t.store.LoadRelationships(); len(relationships) > 0 {
		result["relationships"] = relationships
	}
	if chars, _ := t.store.LoadCharacters(); len(chars) > 0 {
		result["characters"] = chars
		aliasMap := make(map[string]string)
		for _, c := range chars {
			for _, alias := range c.Aliases {
				aliasMap[alias] = c.Name
			}
		}
		if len(aliasMap) > 0 {
			result["alias_map"] = aliasMap
		}
	}
	if changes, _ := t.store.LoadRecentStateChanges(a.Chapter, 5); len(changes) > 0 {
		result["recent_state_changes"] = changes
	}
	if rules, _ := t.store.LoadWorldRules(); len(rules) > 0 {
		result["world_rules"] = rules
	}
	if summaries, _ := t.store.LoadRecentSummaries(a.Chapter, 2); len(summaries) > 0 {
		result["recent_summaries"] = summaries
	}

	return json.Marshal(result)
}

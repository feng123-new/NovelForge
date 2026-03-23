package orchestrator

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// handleFoundationCheck 在 SubAgent 完成后检查基础设定是否完备。
// 如果 phase 仍在 premise（有 premise 但无 outline），注入确定性提醒。
func handleFoundationCheck(coordinator *agentcore.Agent, store *storepkg.Store, emit emitFn) {
	progress, _ := store.LoadProgress()
	if progress == nil {
		return
	}
	// 只在规划阶段检查（premise 已保存但 outline 未保存）
	if progress.Phase != domain.PhasePremise {
		return
	}
	var missing []string
	if o, _ := store.LoadOutline(); len(o) == 0 {
		missing = append(missing, "outline")
	}
	if c, _ := store.LoadCharacters(); len(c) == 0 {
		missing = append(missing, "characters")
	}
	if r, _ := store.LoadWorldRules(); len(r) == 0 {
		missing = append(missing, "world_rules")
	}
	if len(missing) == 0 {
		return
	}
	slog.Warn("基础设定不完整", "module", "host", "missing", missing)
	if emit != nil {
		emit(UIEvent{Time: time.Now(), Category: "SYSTEM",
			Summary: fmt.Sprintf("基础设定不完整，缺失: %v", missing), Level: "warn"})
	}
	runMeta, _ := store.LoadRunMeta()
	guidance := planningTierGuidance(runMeta)
	msg := fmt.Sprintf(
		"[系统] 基础设定不完整，以下项目尚未保存：%v。请重新调用对应规划师补全这些设定。在基础设定全部完备前，不要调用 writer。",
		missing)
	if guidance != "" {
		msg += "\n" + guidance
	}
	coordinator.FollowUp(agentcore.UserMsg(msg))
}

// handleUncommittedDraft 在 Writer 结束但没有 commit 时检测是否存在未提交的草稿。
func handleUncommittedDraft(coordinator *agentcore.Agent, store *storepkg.Store, emit emitFn) {
	progress, _ := store.LoadProgress()
	if progress == nil || progress.Phase == domain.PhaseComplete {
		return
	}
	next := 1
	if progress.InProgressChapter > 0 {
		next = progress.InProgressChapter
	} else if len(progress.CompletedChapters) > 0 {
		next = progress.NextChapter()
	}
	draft, _ := store.LoadDraft(next)
	if draft == "" {
		return
	}
	slog.Warn("Writer 结束但草稿未提交", "module", "host", "chapter", next)
	if emit != nil {
		emit(UIEvent{Time: time.Now(), Category: "SYSTEM",
			Summary: fmt.Sprintf("第 %d 章有草稿但未提交", next), Level: "warn"})
	}
	coordinator.FollowUp(agentcore.UserMsg(fmt.Sprintf(
		"[系统] Writer 结束但第 %d 章草稿未提交。请重新调用 writer 完成该章的自审和提交（commit_chapter）。",
		next)))
}

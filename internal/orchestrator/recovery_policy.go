package orchestrator

import (
	"fmt"
	"log/slog"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func planningTierGuidance(runMeta *domain.RunMeta) string {
	if runMeta == nil {
		return ""
	}
	switch runMeta.PlanningTier {
	case domain.PlanningTierShort:
		return "当前规划级别：short。如需调整设定或重做大纲，优先调用 architect_short。"
	case domain.PlanningTierMid:
		return "当前规划级别：mid。如需调整设定或重做大纲，优先调用 architect_mid。"
	case domain.PlanningTierLong:
		return "当前规划级别：long。如需调整设定或重做大纲，优先调用 architect_long，并保持分层大纲的一致性。"
	default:
		return ""
	}
}

// recoveryResult 恢复链的判断结果。
type recoveryResult struct {
	PromptText string
	Label      string
	IsNew      bool
}

// determineRecovery 根据 Progress 和 RunMeta 判断恢复类型和 Prompt 文本。
func determineRecovery(progress *domain.Progress, runMeta *domain.RunMeta, store ...*storepkg.Store) recoveryResult {
	if progress == nil {
		return recoveryResult{IsNew: true}
	}
	guidance := planningTierGuidance(runMeta)
	withGuidance := func(prompt string) string {
		if guidance == "" {
			return prompt
		}
		return prompt + "\n" + guidance
	}

	if progress.Phase == domain.PhasePremise || progress.Phase == domain.PhaseOutline {
		return recoveryResult{
			PromptText: withGuidance(
				"上次在规划阶段中断。请调用 novel_context 检查当前基础设定状态，补全缺失的设定项（premise/outline/characters/world_rules），然后开始写作。"),
			Label: fmt.Sprintf("恢复：规划阶段（%s）", progress.Phase),
		}
	}

	if progress.InProgressChapter > 0 {
		ch := progress.InProgressChapter
		return recoveryResult{
			PromptText: withGuidance(fmt.Sprintf(
				"第 %d 章正在进行中，已有部分草稿。请调用 writer 继续完成该章（可用 read_chapter 读取已有草稿）。总共需要写 %d 章。",
				ch, progress.TotalChapters)),
			Label: fmt.Sprintf("恢复：第 %d 章进行中", ch),
		}
	}

	if len(progress.PendingRewrites) > 0 {
		verb := "重写"
		if progress.Flow == domain.FlowPolishing {
			verb = "打磨"
		}
		return recoveryResult{
			PromptText: withGuidance(fmt.Sprintf(
				"上次审阅后有 %d 章被标记为待%s（受影响章节：%v）。原因：%s。\n"+
					"请先调用 novel_context 读取相关章节原文，重新评估是否真的需要%s。如果问题不严重或已在后续章节中自然修正，可以跳过%s直接继续写第 %d 章。\n"+
					"确实需要%s的章节请逐章调用 writer 处理。总共需要写 %d 章。",
				len(progress.PendingRewrites), verb, progress.PendingRewrites, progress.RewriteReason,
				verb, verb, progress.NextChapter(), verb, progress.TotalChapters)),
			Label: fmt.Sprintf("%s恢复：%d 章待处理 %v", verb, len(progress.PendingRewrites), progress.PendingRewrites),
		}
	}

	if progress.Flow == domain.FlowReviewing {
		return recoveryResult{
			PromptText: withGuidance(fmt.Sprintf(
				"上次审阅中断，请重新调用 editor 对已完成章节进行全局审阅。已完成 %d 章，共 %d 字。总共需要写 %d 章。",
				len(progress.CompletedChapters), progress.TotalWordCount, progress.TotalChapters)),
			Label: "审阅恢复：上次审阅中断",
		}
	}

	if progress.Flow == domain.FlowSteering && (runMeta == nil || runMeta.PendingSteer == "") {
		if progress.IsResumable() {
			next := progress.NextChapter()
			return recoveryResult{
				PromptText: withGuidance(fmt.Sprintf(
					"从第 %d 章继续写作。之前已完成 %d 章，共 %d 字。总共需要写 %d 章。",
					next, len(progress.CompletedChapters), progress.TotalWordCount, progress.TotalChapters)),
				Label: fmt.Sprintf("恢复模式：从第 %d 章继续（干预状态已重置）", next),
			}
		}
	}

	if progress.IsResumable() && runMeta != nil && runMeta.PendingSteer != "" {
		next := progress.NextChapter()
		return recoveryResult{
			PromptText: withGuidance(fmt.Sprintf(
				"从第 %d 章继续写作。之前已完成 %d 章，共 %d 字。总共需要写 %d 章。\n\n[用户干预-恢复] %s\n请评估影响范围，决定是否需要修改设定或重写已有章节。",
				next, len(progress.CompletedChapters), progress.TotalWordCount, progress.TotalChapters, runMeta.PendingSteer)),
			Label: "Steer 恢复：上次干预未完成，重新注入",
		}
	}

	// 滚动规划恢复：评审已完成但下一弧/卷尚未就绪
	if progress.IsResumable() && progress.Layered && len(store) > 0 && store[0] != nil {
		s := store[0]
		next := progress.NextChapter()
		// 检查下一章是否在大纲中，如果不在说明需要展开弧或创建新卷
		if _, err := s.GetChapterOutline(next); err != nil {
			volumes := mustLoadLayered(s)
			// 先检查是否有骨架弧需要展开
			if vol, arc := domain.NextSkeletonArc(volumes, progress.CurrentVolume, progress.CurrentArc); vol > 0 {
				return recoveryResult{
					PromptText: withGuidance(fmt.Sprintf(
						"上次弧级评审已完成，但第 %d 卷第 %d 弧尚未展开章节。请调用 architect_long 为该弧展开详细章节规划（save_foundation type=expand_arc, volume=%d, arc=%d），然后继续写作。已完成 %d 章，共 %d 字。",
						vol, arc, vol, arc, len(progress.CompletedChapters), progress.TotalWordCount)),
					Label: fmt.Sprintf("恢复模式：展开第 %d 卷第 %d 弧", vol, arc),
				}
			}
			// 无骨架弧且当前卷非 Final → 需要创建下一卷
			currentFinal := false
			for _, v := range volumes {
				if v.Index == progress.CurrentVolume {
					currentFinal = v.Final
					break
				}
			}
			if !currentFinal {
				return recoveryResult{
					PromptText: withGuidance(fmt.Sprintf(
						"上次卷级评审已完成，需要创建下一卷。请调用 architect_long 自主规划下一卷（save_foundation type=append_volume），参考终局方向和已写内容决定方向，同时更新指南针（save_foundation type=update_compass），然后继续写作。已完成 %d 章，共 %d 字。",
						len(progress.CompletedChapters), progress.TotalWordCount)),
					Label: "恢复模式：创建下一卷",
				}
			}
		}
	}

	if progress.IsResumable() {
		next := progress.NextChapter()
		return recoveryResult{
			PromptText: withGuidance(fmt.Sprintf(
				"从第 %d 章继续写作。之前已完成 %d 章，共 %d 字。总共需要写 %d 章。",
				next, len(progress.CompletedChapters), progress.TotalWordCount, progress.TotalChapters)),
			Label: fmt.Sprintf("恢复模式：从第 %d 章继续（已完成 %d 章，共 %d 字）",
				next, len(progress.CompletedChapters), progress.TotalWordCount),
		}
	}

	return recoveryResult{IsNew: true}
}

func mustLoadLayered(s *storepkg.Store) []domain.VolumeOutline {
	v, err := s.LoadLayeredOutline()
	if err != nil {
		slog.Warn("加载分层大纲失败", "module", "recovery", "err", err)
	}
	return v
}

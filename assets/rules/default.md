---
# 项目内置默认规则（Phase 1 安全版）
#
# 这里只放"机械可检 + 低争议"的默认约束。非机械化审美偏好（如风格倾向）
# 当前仍由 writer.md / editor.md 承载，待 Phase 1.5（F1 手测验证
# working_memory 约束力后）再决定是否搬入本文件。
#
# 用户可在项目根 ./rules.md 或 ~/.ainovel/rules.md 覆盖普通字段；
# fatigue_words 按词合并，同一词由更近来源覆盖阈值。
# 详细字段语义参见项目根 rules.md.example。

# 章节字数范围：偏差 <20% 警告；≥20% 错误。
chapter_words: 3000-6000

# 疲劳词软限制：commit_chapter 会检查每章出现次数，超过阈值报 warning。
# 这些是网文/小说常见过度使用的词，writer.md 也有同方向的提示——双源信号一致。
fatigue_words:
  不禁: 1
  竟然: 1
  仿佛: 2
  此外: 1
  然而: 2
---

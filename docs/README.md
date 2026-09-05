# NovelForge 文档导航

当前维护 Phase 1–9，Phase 9 已由用户明确恢复实施；Phase 11–13 继续暂停。源码功能交付、局部验证与正式发布是不同状态。

## 当前使用与设计

| 主题 | 当前入口 |
| --- | --- |
| 安装、配置与使用 | [项目 README](../README.md)、[Web 工作台](WEB.md)、[配置与迁移](MIGRATION.md) |
| 项目和接口 | [Project API](PROJECT_API.md)、[API 说明](API.md) |
| 事实与叙事 | [Truth Store](TRUTH_STORE.md)、[Narrative Ledger](NARRATIVE_LEDGER.md) |
| 创作与版本 | [Quality Gate](QUALITY_GATE.md)、[ChapterVersion](CHAPTER_VERSIONS.md)、[Agent 边界](AGENTS.md) |
| 上下文 | [Context Compiler](CONTEXT_COMPILER.md) |
| 持续创作 | [Phase 9 Autopilot](AUTOPILOT.md) |
| 架构与演进 | [NovelForge 架构](ARCHITECTURE.md)、[保留的上游运行时](upstream/ainovel-runtime-architecture.md)、[上游同步](UPSTREAM_SYNC.md) |
| 当前范围与修复 | [路线图](ROADMAP.md)、[五项源码修复](PHASE_01_08_FIXES.md) |
| 历史证据 | [归档导航](archive/README.md)；不以历史 CI 冒充当前运行结果 |

## 2026-09-05 目录与冗余清理

基线：`48fb03aa1d9d91ee994edc935317d6bbfe6d8b06`。本轮不改变生成、审核、定稿、迁移、模型配置或后续阶段功能。

将旧运行时架构移动到独立名称，消除仅大小写不同的文件路径；历史交付、早期复核与 Phase 8 验收归档；两份上游演示素材移动到 `docs/assets/upstream/`，内容不删减。同步仓库内文档链接和源码注释中的显式路径。删除经全仓 Go 引用检查确认只有定义的旧诊断辅助函数，保留现用编译、诊断、字节预算与测试。

清理前业务仓库共有 88 个分支；静态规则选出 76 个候选。删除前将完整分支清单、提交对象和理由保存到独立归档标签 `archive/phase-01-08-cleanup-20260905`。只有本 PR 合并后才执行删除，并重新核对 HEAD、保护状态和未关闭 PR；发生变化的分支保留。独有且未确认可清理的实现以及 Phase 9 分支不动。实际删除数量以清理 PR 的操作记录为准，不把计划数量冒充已完成。

验证限于文件路径大小写检查、移动后链接检查、素材字节一致性、无引用检查、入口编译及两个上下文命名测试；实际结果在清理 PR 记录。没有全量测试、前端重建、数据库迁移或历史改写。`web/dist`、兼容入口、依赖锁、许可证、旧迁移及既有 CI 均保留。临时清理控制器不进入主分支。


## Phase 10 authoring systems

Markdown Writing/Review/Polish/Planning Skills, separate style and reference libraries with Chinese-capable FTS, and configurable advisory phrase/repetition rules are connected to the Web/Autopilot model requests. See [Authoring systems](AUTHORING.md) for limits, storage and actual runtime behavior. Phase 11 is now connected; Phase 12–13 remain paused; no full-suite or long-book acceptance is claimed.

## Phase 11 lifecycle

[Manuscript import, export, backup and restore](LIFECYCLE.md) now connect to immutable versions and the existing semantic review path. Upload and restore do not start paid work. Phase 12–13 remain paused. Verification is bounded to named short flows and affected builds; no whole-book or full-suite acceptance is implied.


## Phase 12 — diagnostics and costs

Project-scoped SDK-attempt accounting, conservative pre-call quotas, immutable price snapshots, explicit unknown-cost reconciliation, provider observations and the Diagnostics & Cost page are implemented. See [diagnostics](DIAGNOSTICS.md). Phase 13A delivery preparation and Phase 13B local full acceptance remain separate; no full-suite or long-book result is implied.

# 历史档案

[返回当前文档导航](../README.md)。本目录保留当时的事实和验证范围，不将旧的 complete、未修复或后续交接说明当作当前操作指令。

| 档案 | 用途 |
| --- | --- |
| [阶段交付记录](phase-01-08/IMPLEMENTATION_STATUS.md) | 保留历史 PR、提交和 CI 证据 |
| [前八阶段原始复核](phase-01-08/PHASE_01_08_REVIEW.md) | 保留 `5dbcadfa` 基线的断点与回补计划 |
| [Phase 8 验收](phase-01-08/PHASE_08_ACCEPTANCE.md) | 保留章节版本阶段证据 |

当前五项修复状态见 [PHASE_01_08_FIXES.md](../PHASE_01_08_FIXES.md)。归档只调整位置和本地引用，不删除验收内容。上游运行时架构位于 [upstream](../upstream/ainovel-runtime-architecture.md)，仍用于解释保留的 TUI/Headless 路径。

## 分支快照与恢复

清理使用独立 Git 标签 `archive/phase-01-08-cleanup-20260905`，不创建 Release，也不改写 main 历史。标签指向专用档案提交：树中保存 `branch-snapshot.json`，父提交保留全部原分支 HEAD 以及本轮清理提交，避免 squash 合并或删除最后一个引用导致旧实现失去持久引用。

读取档案：

```sh
git fetch origin refs/tags/archive/phase-01-08-cleanup-20260905:refs/tags/archive/phase-01-08-cleanup-20260905
git show archive/phase-01-08-cleanup-20260905:branch-snapshot.json
```

按快照中记录的完整 SHA 新建恢复分支即可。不要强制覆盖已有同名分支。没有确认可清理的独有实现、受保护分支、仍被开放 PR 使用的分支和冻结的后续阶段分支均不删除。实际分支操作结果保存在清理 PR 讨论记录中。

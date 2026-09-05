# Phase 1–8 五项连接问题修复记录

日期：2026-09-05。生产基线：`5dbcadfa272f70f6e4cf1049ce12d40c3dcd8460`；承接 PR #34 的整理提交 `f64726ecbfd5eacefec3826a7fceeabe894cf6a0`。

本文描述本次代码改动，不改写历史阶段交付和 CI 证据。只有 PR 的真实合并状态才能证明进入 main；本文和旧 CI 成功均不代替新代码的运行验收。

## 范围

继续暂停 Phase 9–13。五项问题属于 Phase 1–8 的默认入口、模块连接、中文检索、本地部署和状态说明；“正式发行尚未发生”涉及后续发布，因此仅纠正说明，不在本次创建 Release。

## 1. 默认质量模型服务连接（Phase 1/2/5/8）

- `server --config FILE` 和已有环境变量进入同一配置解析流程；不改变进程工作目录，不隐式迁移。
- `qualityruntime.QualityRuntime` 将 opaque project ID 映射到受控项目根，按项目读取配置，复用工作区默认模型。
- `qualityruntime.QualityModel` 复用已有 ModelSet / Provider / Fallback；支持 librarian 角色覆盖，传递角色推理配置并检查估算的请求空间。
- 无配置时管理功能保留，生成/审核不可用；非法选中配置不静默切换到其他凭据。
- Quality 协调器和 ChapterVersion 协调器共用同一模型服务构造链。仍执行 Librarian / Continuity / Editor，不给 Writer 任何事实写权限。
- 使用生产 DTO 生成模型输出 Schema 说明，返回结果仍由既有严格 Decoder 验证。拒绝空结果、截断和工具调用结果。
- Provider 原始错误不直接进入 API 或调用记录；返回安全错误。配置可用不等于网络、密钥或额度健康已验证。

## 2. 让编译结果进入实际请求（Phase 5/6/7）

Web Writer：`Repository.WriterContext → Compiler → ModelWriterService payload → IdempotentModelCaller request hash → Provider`。实际上下文包含 Current Chapter Plan、Truth、独立查询的 POV 状态、Ledger 的已知秘密/无真相未知边界、近期 Final 和真实 FTS 结果。请求哈希包含编译结果；不能在缓存哈希之后另加一份隐藏上下文。

旧工具：`novel_context → SelectLegacyMap → selected map → JSON tool response`。编译先于旧式可选字段裁剪；保留旧非可裁剪字段的约束，编译失败直接返回错误，不再返回旧内容并仅标记 unavailable。原始输入不被修改，也不附送一份未受预算控制的副本。

这是两个兼容路径：没有将所有旧文件自动导入 SQLite。旧适配器保留既有字段的时间来源假设；项目提供者沿用现有有界查询，通用 Truth 查询上限和 Ledger 上限没有在本轮重做规模设计。Token 计数仍是估算，不是各 Provider 精确 tokenizer。模型最终是否遵守知识边界仍需真实样本验证；不能据此宣称文学效果或百万字容量通过。

## 3. 中文检索（Phase 7）

新增 **数据库 Migration 8**，不是启动 Phase 9：

- 新建 `context_documents_cjk_fts`，汉字按单字存储，查询按词组要求相邻顺序；支持两字人名和连续中文短语。
- 不改变原始 `context_documents.content`，也不改已经应用的 Migration 5 SQL。
- 回填已有文档，INSERT/UPDATE/DELETE 触发器保持索引同步，包括 ChapterVersion 直接更新索引源表的路径。
- 保留原有英文索引；含汉字查询使用新索引，项目/章节筛选、参数绑定、稳定排序和结果上限保留。
- `novelforge_cjk_v1` 在应用的 SQLite 驱动连接创建前注册。外部 SQLite 程序直接写 `context_documents` 也需要提供该函数；不支持绕过应用直接写内部派生表。

这解决无空格中文句中词组的索引匹配，不宣称解决别名、同义词、语义相似度或文学检索质量。

## 4. 本地端口发布（Phase 2/3）

Compose 与 README Docker 示例统一为 `127.0.0.1:48090:48090`。不改变项目挂载、容器内部监听或原有 CLI。只缩小默认宿主机发布范围，不声称新增认证、权限系统或完成公网部署验收。

## 5. 文档与能力状态

README 统一说明历史交付、代码连接、配置可用、运行验证和发行状态。旧 `PHASE_01_08_REVIEW.md` 是固定基线复核；以本文理解本次回补。`IMPLEMENTATION_STATUS.md` 的既有 PR/SHA/CI 证据保持不变。

截至本次读取 Releases API 返回空数组。当前提供源码/Docker 构建入口；安装脚本仍依赖未来真实发行资产。本轮不发布、不打版本、不恢复 Phase 9–13。

## 实际验证记录

| 检查 | 本轮结果 | 边界 |
| --- | --- | --- |
| Go 汉字切分/查询转义命名测试 | 通过，使用本地 Go 1.23.2，仅编译 `cjk.go` 与 `cjk_test.go` | 不是整个包或项目测试 |
| SQLite 词组索引小样本 | SQLite 3.46.1，13 项断言通过 | 直接使用本次迁移/查询 SQL 和对应的 Python 注册函数；不是 modernc 驱动集成测试 |
| SQL 样本范围 | 旧文档回填、原文不变、张三/青云宗/玄铁剑、顺序、项目/章节隔离、更新、删除、英文索引 | 没有规模基准 |
| 格式/语法 | 对本次所有 Go 文件执行 gofmt，并检查 AST 解析 | 不等于依赖链接或完整类型检查 |
| 连接链 | 静态复核 Config → Runtime → ModelSet → Quality/Version，以及 Context → Payload → Hash → Model | 未调用真实 Provider |

新增但**未在本轮执行**的仓库测试：`qualityruntime/model_test.go`、`model_context_test.go`、`legacy_selection_test.go`、`novel_context_compilation_test.go`、`fts_cjk_test.go`。这些文件不能计入“已通过”数量。

未执行：项目完整编译、完整 Go/前端/race 测试、浏览器流程、付费模型生成、Windows/Docker 矩阵、长篇模拟和规模测试。当前环境只有 Go 1.23.2，仓库声明 Go 1.25.5，且未获取完整依赖源码；不将语法检查冒充完整构建。

不修改已有 CI、不删除测试、不放宽保护；本次提交使用 `[skip ci]`。后续只对受影响链执行命名测试或短流程，是否扩大全量验证由用户另行改变范围。

# Phase 1–8 逻辑链复核与维护范围

决策日期：2026-09-05。

复核基线：`main@5dbcadfa272f70f6e4cf1049ce12d40c3dcd8460`。本文只描述该基线及本次文档整理；后续代码变化需要另记实际提交，不沿用旧结论冒充新验证。

## 1. 当前决定

只重新梳理 Phase 1–8 的职责、依赖、入口、状态流转与写入边界。Phase 9–13 暂停功能推进和专项维护：不开发 Worker/任务队列，不扩展后续 Skills/生命周期/成本系统，不做发布工程或长篇规模验收，不继续操作后续阶段的临时工作流和分支。

本轮是文档与逻辑链复核，不是重写前八阶段，也没有把下面的回补项实现为代码。既有模块、历史 PR、合并提交及 CI 证据继续保留。冻结后续阶段不删除已有 CLI/Headless 的导入导出、规则、诊断或恢复能力。

后续验证采用静态追踪优先，必要时仅对改变的逻辑链执行一个命名测试或短流程；不进行全量测试。轻量复核不等于完整运行验收，也不等于数据安全、文学效果或百万字容量已被证明。

## 2. 状态口径

| 口径 | 含义 | 不代表什么 |
| --- | --- | --- |
| 历史已交付 | 已有阶段代码、合并与当时的验收记录 | 不代表本轮重新执行过测试 |
| 静态已核对 | 读取实际入口、构造函数和调用关系 | 不代表已调用真实模型或走完运行流程 |
| 已知接线缺口 | 当前代码可定位的调用或依赖断点 | 不代表整个阶段从未完成 |
| 待小范围验证 | 尚需受影响路径的命名测试或短流程确认 | 不允许直接标为通过 |
| 已修复 | 必须同时记录真实代码提交和采用的验证范围 | 不能因更新了本文而改成已修复 |

[IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) 继续作为历史交付档案；其中的 complete、CI success 和 Phase 9 handoff 是历史语境。当前维护范围以本文和 [ROADMAP.md](ROADMAP.md) 为准，不自动开始下一阶段。

## 3. Phase 1–8 分阶段复核矩阵

以下“历史”来自已有交付记录和模块契约；“本轮”只包含静态核对或明确的待核对项，没有执行八阶段全量回归。

| 阶段 | 需要保留的职责 | 当前逻辑链与本轮结论 | 后续限定动作 |
| --- | --- | --- | --- |
| 1 品牌与兼容 | 新旧入口、配置优先级、显式迁移和凭据边界 | 历史兼容验收保留；默认 server 构造仅传 Host/Port/Workspace/Version，不能把 CLI 配置支持等同于 Web 模型已注入 | 回补时复用既有配置解析，区分 workspace 与 project，不隐式迁移、不混合新旧同层凭据 |
| 2 Project/API | 项目生命周期、路径封装、SQLite、幂等、持久化 SSE | 历史实现保留；追踪 server 配置到 Quality/ChapterVersion 服务，Engine 适配边界存在不等于 Worker 已运行 | 只检查相关入口、统一错误和幂等边界；不建立新任务系统 |
| 3 Web Workspace | 页面、类型化客户端、服务器权威状态 | capabilities 已区分模块存在、quality_model_available 和两个 worker_available=false | 逐操作检查禁用条件与错误提示；Models 列表不作连接健康证明；本轮未运行浏览器 |
| 4 Truth Store | 不可变事件、来源、权威等级、Chapter-N 查询和投影 | 历史契约保留；复核生成/人工修订路径的提案与正式提交边界，不重新宣布所有时间查询正确 | 涉及时只追踪一个事实的提交和前后章节读取，不跑十万事实或整书重建基准 |
| 5 Quality Gate | Draft → 提案 → Continuity → Editor → 有限重写 → Final | 已确认 qualityConfigured 依赖服务或 QualityModel；默认入口没有注入。新生成/审核链存在连接缺口 | 先接通已有模型适配与服务，保持 FAIL/HOLD、Schema 和重试上限，不让草稿直接写 Truth |
| 6 Narrative Ledger | 伏笔状态、计算 OVERDUE、秘密持有范围和角色边界 | 历史 Ledger 与页面交付保留；其提供者必须被实际上下文链消费，不能仅凭接口存在宣称全面接入 | 只核对 accepted-Final 的模型来源写入、幂等、Chapter-N/POV 过滤；显式人工管理操作与模型提案分开 |
| 7 Context Compiler | 分层预算、必需项、时序过滤、结构化/FTS 检索 | 已确认旧 novel_context 先裁剪 legacy map，再附加 _context_compiler 诊断；编译失败被记录为 unavailable，旧返回路径仍继续 | 回补真实模型输入连接与项目提供者；中文 FTS 召回作为单独小样本待验证项 |
| 8 ChapterVersion | 不可变版本、人工修改、审核、定稿、同步、派生重建与恢复 | 已确认其协调器复用 Quality 的 Librarian/Continuity/Editor；Save/Restore 不等于 Check/Accept/Finalize 全链已可用 | 沿既有协调器追踪新修订到 Final 和检查点；服务缺失先解决依赖，不绕开审核 |

## 4. 只追踪四条现有逻辑链

### A. 启动、配置与项目服务

```text
novelforge server
  → runServerCommand
  → server.New(server.Config)
  → Workspace / Project Repository / routes
  → qualityConfigured / qualityCoordinator
  → ChapterVersion Coordinator
```

已确认断点：默认命令只传入基础服务器参数，未注入 QualityModel 或三个质量服务。配置解析与 Provider 绑定应在现有入口体系内回补；不以虚构成功状态、测试专用注入或新增 Autopilot 代替。

证据：[server command](../cmd/novelforge/server.go)、[quality coordinator](../internal/server/quality.go)、[version coordinator wiring](../internal/server/chapter_versions_routes.go)、[capabilities](../internal/server/workspace.go)。

### B. 上下文到 Writer 的输入

```text
当前旧路径：旧数据读取 → legacy map → trimByBudget
            → loading summary → 编译器诊断 → 返回原 map

待回补连接：真实项目/Chapter-N/POV 数据 → Context Compiler
            → 必需项与预算结果 → Writer 实际请求
```

第二行是回补目标，不是已经观察到的运行结果。需要检查真实 Truth/Ledger/FTS 提供者与实际发送的输入；不能只检查 context_sha 或诊断字段出现了就宣布接通。

证据：[novel_context](../internal/tools/novel_context.go)、[legacy adapter](../internal/contextcompiler/legacy.go)、[compiler](../internal/contextcompiler/compiler.go)、[FTS store](../internal/contextcompiler/fts.go)。

中文检索目前登记为风险，本文不重复宣称已完成运行复现。后续必要时只选一个无空格中文句子和句中人名/地点做小样本核对；不以英文命中或全文整句命中替代中文关键词召回验证，也不为此引入外部向量服务。

### C. 单章生成与接受后的提交

```text
已有章节计划
  → Writer / Draft 持久化
  → Librarian / Fact Proposal
  → Continuity
  → Editor / 有上限的候选选择
  → 接受的候选 / Final
  → Truth / Ledger / 章节文件 / Active Final / 派生索引 / Checkpoint
```

这是待逐段确认的跨模块关系，不宣称默认入口已经贯通。计划以已有显式 ChapterPlan 为起点，不要求实现 Foundation Worker 或持续自动规划。质量事务与版本协调器的具体提交桥接要分别读取，不能把两套流程画在一起便认为同一入口已执行全部步骤。

所有失败分支都要写清：草稿或修订保留在哪里、是否允许继续、哪个键用于重放。禁止把 Continuity FAIL 降级为可 Finalize，也禁止 Writer/Librarian 直接修改权威事实。

证据：[quality routes](../internal/server/quality.go)、[Phase 8 quality bridge](../internal/server/quality_phase8.go)、[Final coordinator](../internal/chapterversion/coordinator_finalize.go)、[agent boundaries](AGENTS.md)。

### D. 人工修改与后续读取

```text
Save Human Revision / Restore as New Version / Explicit External Sync
  → 新版本（不是 Active Final）
  → Librarian + Continuity + Truth 冲突评估
  → Accept
  → Finalize
  → 有效正文 / Truth / Ledger / Active Final
  → 受影响派生状态 / FTS / Checkpoint
  → 后续读取使用已接受状态
```

Save/Restore 与需要语义服务的审核不是同一个能力。新审核依赖默认服务接线；已有持久化评估的恢复路径应单独判断。外部文件变动不能悄悄覆盖 Active Final；恢复历史内容也不能删除历史。

证据：[version actions](../internal/server/chapter_versions_actions.go)、[evaluation](../internal/chapterversion/coordinator_evaluate.go)、[finalization](../internal/chapterversion/coordinator_finalize.go)、[ChapterVersion contract](CHAPTER_VERSIONS.md)。

## 5. 有序回补清单（本轮没有实施业务代码修复）

| ID | 优先级 / 阶段 | 待处理内容 | 最小核对方式 | 状态 |
| --- | --- | --- | --- | --- |
| R01 | 首先；1/2/5/8 | 默认 server 的配置加载、角色模型绑定与质量服务注入 | 读实际构造链；必要时一个使用假模型的启动级短流程，同时看未配置时是否明确报错 | 已定位，未修复 |
| R02 | 随 R01；3/8 | 页面与每个操作的真实前置条件一致，不以模块存在替代服务可用 | 看 capabilities 到 action 的判断；必要时一项受影响组件测试 | 待逐操作核对，未修复 |
| R03 | 其次；4/6/7 | Context Compiler 结果进入实际模型输入，并使用真实项目的 Truth/Ledger/FTS 提供者 | 截获一份假模型请求；检查必需项、未来信息排除和失败去向 | legacy 诊断连接已定位，完整接管未修复 |
| R04 | 随 R03；7 | 中文正文关键词的 FTS 召回 | 一个无空格中文样例及人名/地点查询；仍保留项目与章节过滤 | 风险登记，本轮未执行样例 |
| R05 | 然后；5/6/8 | 生成与人工修订两条链使用正确的提交/恢复边界 | 读接受、定稿、重放和重建的调用顺序；必要时一个修订或故障点短流程 | 待接线后小范围核对 |
| R06 | 配套；1/2/3 | 默认本地部署、能力说明与文档一致 | 核对端口映射和未配置说明；不运行 Docker/跨平台矩阵 | README 已补限制说明；Compose 文件未修改 |

顺序为 R01/R02 → R03/R04 → R05，R06 随相关文档整理。每次只修一个明确断点，不为完成一个编号扩展到后续阶段，也不承诺这份清单已经穷尽所有缺陷。

## 6. 轻量验证规则

每次记录六个要素：入口、输入、调用对象、状态转换、权威写入边界、失败/重放去向。必要的动态验证只选择受改动影响的单一命名测试或短流程，使用临时项目与假模型，不需要真实付费调用或长篇生成。

本维护轮不执行 `go test ./...`、全项目 race、完整 Vitest、Windows/Docker 构建矩阵、300 章模拟、100/500/1000 章基准或十万事实规模门禁。已有 CI、测试文件、依赖锁和安全门禁不删除、不放宽；未运行或跳过的检查不标记为成功。

如果一个问题无法在该验证范围内获得足够证据，保留“待验证”，不要为了追求全部绿色擅自扩成全量验证。后续确需扩大范围时，应由用户明确改变当前范围。

纯文档提交可使用 `[skip ci]` 避免自动触发既有全量 `push`/`pull_request` 工作流；不修改 `.github/workflows/ci.yml`，不生成伪造成功状态。如果必要检查因此处于 Pending 并阻止合并，保持未合并，不绕过分支保护。参考 [GitHub skipping workflow runs](https://docs.github.com/actions/managing-workflow-runs/skipping-workflow-runs)。

### 本次实际记录

- 已读取主分支 ref，固定上述 SHA；重读阶段档案、README、Roadmap、Agent 边界及关键 server/context/version 调用点。
- 本次变更只包含 README、Roadmap 和本文；历史验收档案、业务源码、数据库迁移、前端产物、依赖和工作流均未修改。
- 本次没有执行 Go 测试、前端测试、编译、浏览器流程、真实模型调用或规模测试。运行验收状态为“未执行”，不是“通过”。
- 本地源码克隆因当前执行环境无法解析 GitHub 域名而未完成；实际源码核对通过已连接的 GitHub 读取进行，不据此报告任何本地运行结果。
- 后续修复记录必须写明实际提交、做过的静态核对/命名验证、仍未覆盖的范围以及是否合并，避免“已提交”等同于“已进入 main”。

## 7. 收口条件

完成梳理的标准是：八阶段职责与已知断点清楚、四条逻辑链有代码落点、回补项与未验证内容可追踪、README 与 Roadmap 不再把旧状态当当前可用性。完成梳理不等于回补已经完成。

后续只有在实际代码修复后，才逐项更新 R01–R06 的状态。Phase 9–13 继续暂停；不因前八阶段的某次静态复核结束而自动恢复后续开发、专项维护或全量测试。

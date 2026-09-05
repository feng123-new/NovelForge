# NovelForge

## Current delivery: Phase 12 + Phase 13A

Phase 1–12 functionality is delivered; Phase 13A supplies default-entry smoke, local verification tools and an explicit release-candidate pipeline. Phase 13B full/platform/scale/real-provider acceptance is pending local execution.

Start with [local deployment](docs/DEPLOYMENT.md), follow [local acceptance](docs/LOCAL_ACCEPTANCE.md), and use the [candidate process](docs/RELEASING.md). A prerelease is not stable/latest, and cross-compilation is not target-platform runtime validation.


> 面向长篇小说的本地 AI 创作与人机共创工程平台。百万字规模是设计目标，不是本轮验证结论。

NovelForge 基于 Apache-2.0 上游 [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli)。保留原有 TUI、Headless、Engine、Agent Runtime、Checkpoint、滚动规划、模型路由、导入导出和诊断能力，新增 Web 工作台、Truth Store、质量门禁、叙事账本和章节版本。

## 当前维护范围

[文档导航与当前维护记录](docs/README.md) · [历史交付档案](docs/archive/README.md)

**Phase 9 已按本次明确请求接入；Phase 11–13 继续暂停。** 默认 Web 启用可恢复 Autopilot，复用前八阶段的质量门禁与版本定稿。新建向导仍只保存请求，用户在 Autopilot 页面单独启动有界任务，不会自动产生模型费用。当前只做定向验证，不发布新 Release。详见 [Autopilot](docs/AUTOPILOT.md)。

历史 PR、合并与 CI 证据保留在 [IMPLEMENTATION_STATUS.md](docs/archive/phase-01-08/IMPLEMENTATION_STATUS.md)。当前源码修复和有限验证边界见 [PHASE_01_08_FIXES.md](docs/PHASE_01_08_FIXES.md)。模块存在、配置满足、定向测试通过和全量验收是不同状态。本轮没有全量回归、付费模型验收或规模测试。

## 已有能力

| 范围 | 实现与边界 |
| --- | --- |
| 兼容与入口 | `novelforge` 与旧 `cmd/ainovel-cli`；新旧目录兼容；显式配置、诊断和 copy-only 迁移 |
| Project/API | 项目生命周期、路径边界、SQLite 迁移、幂等写 API、持久化 SSE |
| Web Workspace | Dashboard、Projects、New Novel、Chapters、Models、Logs、Settings |
| Truth Store | 不可变事实事件、来源、冲突与 Chapter-N 投影 |
| Quality Gate | Draft、Librarian 提案、Continuity、Editor、有限重写与定稿 |
| Narrative Ledger | 伏笔生命周期、计算 OVERDUE、角色秘密持有范围及管理页面 |
| Context Compiler | 分层预算、必需项、确定性选择、历史 FTS 与中文字符检索 |
| ChapterVersion | 版本历史、Diff、人工修订、同步、接受、定稿、重建与恢复 |
| Autopilot | 持久化任务、Foundation/章节规划、连续生成、审阅暂停、停止、重启恢复；只复用已接受的定稿路径 |

## 安装与启动

需要满足 `go.mod` 声明的 Go 版本（当前 Go 1.25.5）或 Docker。

```sh
git clone https://github.com/feng123-new/NovelForge.git
cd NovelForge
go build -o novelforge ./cmd/novelforge

# 原有 TUI / Headless
novelforge
novelforge --headless --prompt-file prompt.txt

# 配置标志位于 server 子命令之后
novelforge server --workspace /path/to/novels
novelforge server --workspace /path/to/novels --config /secure/novelforge.json
NOVELFORGE_CONFIG=/secure/novelforge.json novelforge server --workspace /path/to/novels
```

浏览器访问 `http://127.0.0.1:48090`。无模型配置时仍可管理项目和查看/保存版本；生成和新语义审核需要有效 Provider 配置。Models 列表不是连接健康检查。

**当前不将 Release 安装脚本作为已有发行渠道。** 核对时仓库尚无 Release；本轮不发布发行资产。源码和 Docker 构建是当前入口。未来有对应平台资产及校验文件后才使用 `scripts/install.sh`；离线烟测不等于发布完成。

卸载只删除可执行文件，不删除配置和小说：

```sh
NOVELFORGE_INSTALL_DIR="$HOME/.local/bin" sh scripts/uninstall.sh
```

## Web 模型与上下文

默认 Web 入口启用配置加载器，各项目由 Project Repository 解析配置，不修改进程工作目录，不混用不同项目的配置。Writer、Librarian、Editor 复用已有模型集合和显式 Fallback；`roles.librarian` 可独立配置，缺省使用顶层默认模型。

生成前读取 Truth、POV 过滤后的 Ledger、近期索引与 FTS，通过 Context Compiler 生成 `compiled_context`。该结果在模型调用哈希计算前进入 Writer 请求；编译失败不回退未裁剪原始输入。旧 `novel_context` 保留选中字段的形状，但只返回编译器选中的记录，不再仅附加诊断。

Token 预算仍是估算，不是提供商计费 Token。旧适配器未重新解释所有旧文件的时序元数据；有界 SQL 查询、长篇召回和全部角色路径未经过本轮规模验收。

### 配置优先级

```text
server --config / 顶层 CLI --config
NOVELFORGE_CONFIG
<项目>/.novelforge/config.json
<项目>/.ainovel/config.json
~/.novelforge/config.json
~/.ainovel/config.json
built-in defaults
```

显式配置独立使用；同层新旧目录不合并，项目覆盖选中的全局层。旧 `cmd/ainovel-cli` 保持原兼容行为。Web `quality_model_available` 表示工作区默认配置条件；具体项目以其质量状态接口为准，不代表 Provider 已联网验证；Worker 就绪状态由独立能力字段报告。

```json
{
  "provider": "local-proxy",
  "model": "your-model",
  "providers": {
    "local-proxy": {
      "type": "openai",
      "api": "chat",
      "api_key": "${NOVELFORGE_API_KEY}",
      "base_url": "https://your-provider.example/v1"
    }
  },
  "roles": {
    "writer": {"provider": "local-proxy", "model": "your-model"},
    "librarian": {"provider": "local-proxy", "model": "your-model"},
    "editor": {"provider": "local-proxy", "model": "your-model"}
  }
}
```

Web 适配器支持 `api_key` 的完整 `${ENV_NAME}` 引用，变量缺失时拒绝配置；不把展开后的凭据写回文件或返回浏览器。不要提交真实密钥。配置及迁移规则见 [MIGRATION.md](docs/MIGRATION.md)。

## Docker 与访问边界

```sh
docker compose up --build

# 直接运行也限定宿主机回环地址
docker build -t novelforge .
docker run --rm -p 127.0.0.1:48090:48090 \
  -v "$PWD/config:/root/.novelforge" \
  -v "$PWD/workspace:/workspace" \
  novelforge server --host 0.0.0.0 --workspace /workspace
```

Compose 映射默认为 `127.0.0.1:48090:48090`。容器内监听 `0.0.0.0` 供 Docker 转发。当前服务没有完整多用户认证；非回环告警不是认证，远程访问需要受保护的网络入口，不应直接向公网开放。

## 数据与 API

工作区控制数据在 `.novelforge/server.db`，小说数据在各项目的 `.novelforge/project.db`。业务写 API 要求 `Idempotency-Key`，集合采用有界分页。完整路由以 `/api/openapi.json` 为准。

人工保存和恢复创建新版本，不覆盖 Active Final。Check / Accept 需要语义服务；只有接受并完成 Finalize 后才提交权威状态。Writer、Librarian 不直接写 Truth，严重一致性失败不可被文学评分抵消。

新增 Migration 8 提供中文字符索引，保留英文词索引和原文。回填与触发器共用 Go 注册的确定性函数。普通外部 SQLite 客户端未注册该函数时不应直接写 `context_documents`。升级沿用迁移备份与校验机制，不修改旧 Migration 5；源码更新本身不立即打开或迁移用户数据库。

## 开发与验证

本轮限于静态调用链核对、相关入口编译和少量命名测试，不运行全量 Go/前端/race、平台矩阵、长篇模拟或真实付费调用。既有 CI、锁文件、前端产物和历史验收记录保留。实际运行编号与精确提交见修复 PR，未执行的检查不标记为通过。

[Architecture](docs/ARCHITECTURE.md) · [Web](docs/WEB.md) · [Roadmap](docs/ROADMAP.md) · [Upstream Sync](docs/UPSTREAM_SYNC.md)

## License and credits

Apache License 2.0；保留 ainovel-cli 的原始版权与来源。详见 [LICENSE](LICENSE)、[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)、[docs/LICENSES.md](docs/LICENSES.md)。

- **voocel/ainovel-cli** — Apache-2.0，核心代码与运行时上游。
- **Nigh/show-me-the-story** — MIT，体验与部署设计参考。
- **Hurricane0698/novelwriter** — AGPL-3.0，clean-room 架构参考，不复制源码。
- **EthanYoQ/AI-Novel-Writer** — GPL-3.0，clean-room 工作流参考，不复制源码。


## Phase 10 authoring systems

Markdown Writing/Review/Polish/Planning Skills, separate style and reference libraries with Chinese-capable FTS, and configurable advisory phrase/repetition rules are connected to the Web/Autopilot model requests. See [Authoring systems](docs/AUTHORING.md) for limits, storage and actual runtime behavior. Phase 11 is now connected; Phase 12 is delivered; Phase 13B remains local; no full-suite or long-book acceptance is claimed.

## Phase 11 lifecycle

[Manuscript import, export, backup and restore](docs/LIFECYCLE.md) now connect to immutable versions and the existing semantic review path. Upload and restore do not start paid work. Phase 12 is delivered. Phase 13A supplies candidate delivery; Phase 13B full acceptance remains local. Verification is bounded to named short flows and affected builds; no whole-book or full-suite acceptance is implied.


## Phase 12 — diagnostics and costs

Project-scoped SDK-attempt accounting, conservative pre-call quotas, immutable price snapshots, explicit unknown-cost reconciliation, provider observations and the Diagnostics & Cost page are implemented. See [diagnostics](docs/DIAGNOSTICS.md). Phase 13A delivery preparation and Phase 13B local full acceptance remain separate; no full-suite or long-book result is implied.

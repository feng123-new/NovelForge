# NovelForge

> 面向长篇小说的 AI 创作与人机共创平台。百万字规模是设计目标，不是当前轻量验证的结论。

NovelForge 以 [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli) 为 Apache-2.0 核心上游，保留 TUI、Headless、Engine、Agent Runtime、Checkpoint、滚动规划、摘要压缩、模型路由与已有导入导出，并增量增加本地 Web、项目管理、Truth Store、质量门禁、伏笔与秘密账本、上下文编译和章节版本。

## 当前范围与状态

**2026-09-05：只维护 Phase 1–8；Phase 9–13 暂停功能推进和专项维护。** 不开发 Durable Autopilot、后台任务队列或后续发布功能，也不删除已有 CLI 功能。

历史交付证据见 [IMPLEMENTATION_STATUS](docs/IMPLEMENTATION_STATUS.md)。针对前八阶段连接缺口的代码变更和实际验证范围见 [PHASE_01_08_FIXES](docs/PHASE_01_08_FIXES.md)。[PHASE_01_08_REVIEW](docs/PHASE_01_08_REVIEW.md) 是固定旧基线的复核记录，不应将其中“待回补”当作本次代码提交后的状态。

| 能力 | 当前代码边界 |
| --- | --- |
| 品牌、配置与迁移 | 新入口 `.novelforge` 优先，保留 `.ainovel` 兼容读取；显式、备份、copy-only 迁移 |
| 项目与 API | 项目生命周期、SQLite 迁移、写入幂等、持久化 SSE、安全错误与 OpenAPI |
| Web 工作台 | Dashboard、Projects、New Novel、Chapters、Models、Logs、Settings、Foreshadows、Secrets、Versions |
| Truth / Quality / Ledger | 不可变事实事件、章节时序投影、提案审核、有限重写、伏笔生命周期和角色知识边界 |
| 默认 Web 模型连接 | `server` 加载配置并注入按项目解析的 Writer / Librarian / Editor 模型适配器；未配置时管理功能保留，语义操作不可用 |
| 实际模型上下文 | Web Writer 将项目 Truth、角色过滤 Ledger、近期 Final 与 FTS 检索编译进请求，并参与幂等哈希；旧 `novel_context` 返回编译器选中字段，编译失败向上传递 |
| 中文检索 | 增量 Migration 8 增加按汉字切分的词组索引；保留原文、旧英文索引和既有迁移校验和 |
| 章节版本 | 不可变版本、Diff、人工保存、审核、接受、Final、外部同步、检查点和恢复；仍由既有协调器控制写入 |
| Foundation / Autopilot | Foundation 只保存请求；Worker 不可用，不显示为已运行 |
| 分发 | 优先从源码或本地 Docker 构建；截至本次核对尚无 GitHub Release，不提供“已发布”的暗示 |

代码连接已补充不等于真实 Provider 已验收。配置可用性标记表示配置和客户端能够建立，不表示 API Key、额度、网络或模型健康已通过在线检查；本轮没有执行全量回归、浏览器端到端或付费模型生成。

## 架构

```text
CLI / TUI / Headless → 原有 Host / Engine / Tools
                                      └─ novel_context → 编译器选择后的工具结果

Web → REST / SSE → Project Repository
                         ├─ 按项目配置 → 原有 ModelSet / Provider / Fallback
                         ├─ Quality：Writer → Librarian → Continuity → Editor
                         │     └─ 实际 Writer 请求含编译上下文与请求哈希
                         └─ ChapterVersion → Final / Truth / Ledger / FTS / Checkpoint
```

LLM 负责语义生成与提案，Go 负责验证、状态、版本、幂等与提交。普通草稿和检索结果不能直接成为权威事实；严重 Continuity FAIL 不能被文学评分覆盖。

旧 CLI 数据不会因读取上下文而隐式转换为新的项目数据库。Web 的项目提供者连接与旧工具的字段预算适配是两条明确的兼容路径，并非所有旧数据已经迁移到 Truth Store。详细设计见 [ARCHITECTURE](docs/ARCHITECTURE.md)、[WEB](docs/WEB.md)、[TRUTH_STORE](docs/TRUTH_STORE.md)、[CHAPTER_VERSIONS](docs/CHAPTER_VERSIONS.md)。

## 安装与启动

使用满足 `go.mod` 的 Go 版本（当前声明 `1.25.5`）构建：

```bash
git clone https://github.com/feng123-new/NovelForge.git
cd NovelForge
go build -o novelforge ./cmd/novelforge
```

TUI 与 Headless 保留：

```bash
novelforge
novelforge --headless --prompt-file prompt.txt
cat prompt.txt | novelforge --headless --prompt-file -
novelforge --config /secure/novelforge.json --headless --prompt-file prompt.txt
```

启动 Web；注意 `--config` 放在 `server` 子命令后：

```bash
novelforge server --workspace /path/to/novels
novelforge server --workspace /path/to/novels --config /secure/novelforge.json
NOVELFORGE_CONFIG=/secure/novelforge.json novelforge server --workspace /path/to/novels
```

访问 `http://127.0.0.1:48090`。默认只监听本机。监听非回环地址会告警；当前不是可直接开放公网的已认证多用户服务。

安装/升级脚本仍保留，但依赖真实发行资产；离线脚本烟测不证明已发布。正式 Release 不在本轮范围内。卸载只移除可执行文件，不自动删除配置或小说项目：

```bash
NOVELFORGE_INSTALL_DIR="$HOME/.local/bin" sh scripts/uninstall.sh
```

## 配置

沿用已有确定性优先级：显式 `--config`、`NOVELFORGE_CONFIG`、项目新/旧目录、全局新/旧目录、默认值。同一范围的新旧配置不混合凭据；项目配置覆盖所选全局层。旧 `cmd/ainovel-cli` 保持原有目录语义。

Web 按 opaque project ID 由 Repository 找到项目配置；显式文件优先。没有项目/全局模型时可以使用工作区默认模型。无配置不等于配置错误：无配置允许管理；错误的选中配置不静默切换到其他凭据。

下面仅为结构示例，`your-model`、地址与 Key 都需要替换为自己的实际值；不要提交真实凭据：

```json
{
  "provider": "my-provider",
  "model": "your-model",
  "providers": {
    "my-provider": {
      "type": "openai",
      "api": "chat",
      "base_url": "https://your-provider.example/v1",
      "api_key": "REPLACE_WITH_YOUR_KEY"
    }
  },
  "roles": {
    "writer": { "provider": "my-provider", "model": "your-model" },
    "librarian": { "provider": "my-provider", "model": "your-model" },
    "editor": { "provider": "my-provider", "model": "your-model" }
  }
}
```

角色未单独配置时使用默认模型。模型适配器复用原有 ModelSet、Provider 类型和角色 Fallback，并传递角色推理配置；结构化返回仍须经过严格解码与来源检查。模型文本不能授权定稿。模型调用失败对 Web 和调用账本使用安全错误，不持久化原始 Provider 错误中的请求头或凭据。

```bash
novelforge doctor
novelforge doctor --json
novelforge migrate --dry-run
novelforge migrate
```

完整迁移与配置规则见 [MIGRATION](docs/MIGRATION.md)。

## API 与数据

基础读取包括 `/api/health`、`/api/projects`、`/api/models`、`/api/settings`、`/api/events`。项目章节、Foundation、Quality、Ledger 与 ChapterVersion 的完整契约见 `/api/openapi.json`。写接口要求 `Idempotency-Key`，重复相同请求重放结果，不同内容复用同一 Key 返回冲突。

每个项目拥有 `.novelforge/project.db`；工作区 `.novelforge/server.db` 保存 API 幂等与事件。路径由 Repository 解析，不直接暴露给浏览器。Provider 凭据不经 Foundation 请求、设置接口或浏览器存储传递。

Versions 的 Save/Restore 创建新版本，不会直接替换 Active Final。Check/Accept 需要模型服务与确定性检查；Finalize 沿已有恢复链提交。已持久化评估的恢复不应仅因为暂时缺少模型配置而被迫重做付费步骤。

## Docker

```bash
docker compose up --build
```

Compose 默认端口已绑定 `127.0.0.1:48090:48090`。容器内部仍监听 `0.0.0.0`，不等于向所有宿主机接口发布。配置与项目挂载保持 `./config` → `/root/.novelforge`、`./workspace` → `/workspace`。

```bash
docker build -t novelforge .
docker run --rm -p 127.0.0.1:48090:48090 \
  -v "$PWD/config:/root/.novelforge" \
  -v "$PWD/workspace:/workspace" \
  novelforge server --host 0.0.0.0 --workspace /workspace
```

本轮没有启动容器验证网络可达性，也没有增加认证系统。远程访问须另行保护，不能把端口映射放宽当作安全部署方案。

## 开发与验证范围

当前只追踪改变的逻辑链，必要时做命名测试/短流程，不运行全量 Go、前端、race、Windows/Docker 矩阵或长篇规模测试。已运行与未运行的项目分别列在 [修复记录](docs/PHASE_01_08_FIXES.md)。已有测试、依赖锁、许可证清单与 CI 配置保留，没有删除检查或伪造通过结果。

本轮提交使用 `[skip ci]` 避免触发现有全量 CI。若必要检查因此阻止合并，保持未合并，不绕过保护。提交到 PR、合并到 main、通过测试是三个不同状态。

前端源码与 `web/dist` 保持一致性规则；本轮不修改前端组件或构建产物。已有 TUI 导入导出和诊断保留；Web EPUB、ZIP Backup/Restore、任务队列和发布工程继续暂缓。路线图见 [ROADMAP](docs/ROADMAP.md)。

## License 与上游

采用 [Apache License 2.0](LICENSE)，保留 ainovel-cli 版权和来源信息。Go 模块路径暂时保留上游路径，不是新旧凭据或项目状态的混合依据。第三方依赖与 clean-room 边界见 [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES.md)、[LICENSES](docs/LICENSES.md)、[UPSTREAM_BASE](UPSTREAM_BASE.md) 与 [UPSTREAM_SYNC](docs/UPSTREAM_SYNC.md)。

设计参考包括 `Nigh/show-me-the-story`（MIT）、`Hurricane0698/novelwriter`（AGPL-3.0）和 `EthanYoQ/AI-Novel-Writer`（GPL-3.0）；后两者仅作 clean-room 架构/流程参考，不复制源码、SQL、测试或提示词。本轮不新增 Go/npm 依赖。

# NovelForge

> 面向百万字级长篇小说的 AI 自动创作与人机共创平台；百万字规模是设计目标，不是本轮运行验收结论。

NovelForge 以 [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli) 为 Apache-2.0 核心上游。项目保留确定性 Engine、Agent Runtime、TUI、Headless、Checkpoint、滚动规划、上下文压缩、模型路由、导入导出与诊断能力，并在其上增加安全项目平台、单二进制 Web 工作台、结构化 Truth Store、一致性门禁和章节版本。Durable Autopilot 属于尚未实现的后续范围。

> **当前维护范围（2026-09-05）：只重新梳理 Phase 1–8，Phase 9–13 暂停功能推进和专项维护。** 已有阶段验收记录保留，但不等于默认入口全部可用。本轮采用逻辑链静态核对，必要时仅做受影响路径的小范围验证，不进行全量测试。当前状态、接线缺口和回补顺序见 [`docs/PHASE_01_08_REVIEW.md`](docs/PHASE_01_08_REVIEW.md)；[`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md) 保留历史交付与 CI 证据，不作为本轮新验收结论。

## Features

### 已继承并保持兼容

- TUI 与 `--headless` 两种运行方式
- Architect / Writer / Editor / Arbiter 工作流
- Checkpoint、恢复、重试、Fallback、预算哨兵
- 长篇分卷、Arc、Compass、滚动规划和摘要压缩
- OpenAI-compatible、OpenAI Responses、Anthropic、Gemini、OpenRouter 等模型通道
- `/import`、`/export`、`/sync`、`/diag`、`/simulate` 等能力
- Docker 与跨平台构建基础

### NovelForge 已新增

- `novelforge` 品牌入口，同时保留 `cmd/ainovel-cli`
- `.novelforge` 优先、`.ainovel` 零搬迁兼容读取
- `--config`、`NOVELFORGE_CONFIG`、项目层和全局层的确定性优先级
- `novelforge doctor` 安全诊断，不输出凭据
- `novelforge migrate` copy-only migration、备份、校验清单和失败回滚
- 安装、升级、卸载离线烟测；卸载永不自动删除配置
- 安全项目生命周期：创建、导入骨架、修改、归档、恢复、复制、回收站删除和显式永久删除
- 每项目 `.novelforge/project.db` 与 Workspace `.novelforge/server.db`
- SQLite migration checksum、迁移前备份、事务回滚和 CGO-free 驱动
- 所有写 API 的 `Idempotency-Key` 与统一安全错误 Envelope
- 可重放、重启可恢复、慢客户端不阻塞生产者的持久化 SSE
- Svelte 5、TypeScript、Vite、Tailwind 和 DaisyUI 正式 Web Workspace
- Dashboard、Projects、New Novel、Chapters、Models、Logs、Settings 真实页面
- Truth Store 的不可变事件、来源、冲突和 Chapter-N 投影
- Quality Gate 的候选、事实提案、一致性检查和有上限的重写机制
- Narrative Ledger 的伏笔、秘密和 Chapter-N 知识边界，以及 Foreshadows / Secrets 页面
- Context Compiler 的分层预算和 FTS5 模块；旧 `novel_context` 路径目前主要附加编译诊断，尚未全面替代模型输入
- ChapterVersion 的版本历史、Diff、人工修订、显式同步与可恢复定稿，以及 Versions 页面
- OpenAPI 3.1 与实现漂移测试

**默认 Web 的运行限制：** 当前 `cmd/novelforge/server.go` 未向 `server.Config` 注入 `QualityModel` 或 Writer / Librarian / Editor 服务。因此，模块和路由存在，不代表标准 `novelforge server` 已能执行新的生成、语义审核及人工修订接受流程。查看、保存版本与依赖模型的 Check / Accept 应分别判断；这不是仅填写 API Key 就已解决的接线问题。详见 [Phase 1–8 复核](docs/PHASE_01_08_REVIEW.md)。

## Architecture

```text
CLI / TUI / Headless
    └── 已有 Host / Engine / Agent Runtime / Tools

Web Workspace
    └── REST + SSE
          ├── Project Repository / Durable Events / Idempotency
          ├── Truth Store / Quality Gate / Narrative Ledger
          └── ChapterVersion / Context Index

Engine Adapter 已有边界；默认质量模型注入和完整上下文接管仍待回补。
Durable Autopilot 未接入，且不在当前维护范围。
```

核心原则是 **事实层确定，语义层自主**：LLM 负责创意、文学表达、模糊语义判断和审稿；Go 负责状态迁移、验证、幂等、版本、API、文件、迁移与恢复。

详细设计见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)，Web 边界见 [`docs/WEB.md`](docs/WEB.md)。设计目标与当前入口可用性应分开阅读。

## Installation

需要满足 `go.mod` 声明的 Go 版本，或使用 Docker。

```bash
git clone https://github.com/feng123-new/NovelForge.git
cd NovelForge
go build -o novelforge ./cmd/novelforge
```

Release 安装脚本需要已有对应平台发行资产，并校验 SHA-256；离线安装烟测不等于已完成正式发布。使用前应核对仓库 Releases：

```bash
curl -fsSL https://raw.githubusercontent.com/feng123-new/NovelForge/main/scripts/install.sh | sh
```

卸载只删除可执行文件，不删除任何配置、凭据或小说项目：

```bash
NOVELFORGE_INSTALL_DIR="$HOME/.local/bin" sh scripts/uninstall.sh
```

## Quick Start

### TUI

```bash
novelforge
```

### Headless

```bash
novelforge --headless --prompt "创作一部东方玄幻长篇，主角从边城开始追查失落王朝。"
```

也可以从文件或标准输入读取：

```bash
novelforge --headless --prompt-file prompt.txt
cat prompt.txt | novelforge --headless --prompt-file -
```

使用独立配置：

```bash
novelforge --config /secure/novelforge.json --headless --prompt-file prompt.txt
NOVELFORGE_CONFIG=/secure/novelforge.json novelforge
```

### Web UI

```bash
novelforge server --workspace /path/to/novels
```

浏览器访问 `http://127.0.0.1:48090`。工作区可以是一本既有 ainovel 小说目录，也可以是包含多本小说的 Library。

工作台提供：

- **Dashboard**：真实项目数、章节、字数和服务状态
- **Projects**：筛选、归档、恢复、安全复制和回收站删除
- **New Novel**：六步创建真实项目并保存 Foundation 请求
- **Chapters**：读取项目中的真实章节元数据
- **Foreshadows / Secrets**：伏笔生命周期和秘密持有范围管理
- **Versions**：版本历史、Diff、保存人工修订、恢复与显式同步入口；语义审核仍受模型服务接线限制
- **Models**：读取 Go 运行时模型注册表，不代表 Provider 已配置可用
- **Logs**：显示持久化与重放 SSE 事件
- **Settings**：显示脱敏服务配置、能力与主题

New Novel 向导保存真实 Foundation 请求并明确返回 `worker_available=false`。Durable Worker 与 START / PAUSE / STOP / RESUME 属于已暂缓的 Phase 9，不在本轮接入。

监听 `0.0.0.0` 或其他非回环地址时，程序会打印安全告警。不要把未认证的本地工作区直接暴露到公网。

## Web API

主要基础端点：

```text
GET    /api/health
GET    /api/openapi.json
GET    /api/events
GET    /api/models
GET    /api/settings

GET    /api/projects
POST   /api/projects
GET    /api/projects/{id}
PATCH  /api/projects/{id}
POST   /api/projects/{id}/archive
POST   /api/projects/{id}/unarchive
POST   /api/projects/{id}/duplicate
DELETE /api/projects/{id}

GET    /api/projects/{id}/chapters
GET    /api/projects/{id}/foundation
POST   /api/projects/{id}/foundation
```

Truth、Quality、Ledger 与 ChapterVersion 的完整契约以 `/api/openapi.json` 及对应模块文档为准。路由存在与模型服务已配置是不同状态。

所有写接口要求 `Idempotency-Key`。同一 key 和相同请求会重放原始状态码与响应；同一 key 配合不同请求会返回冲突。集合接口使用稳定排序与有界分页，具体上限以对应 OpenAPI 契约为准。

项目 ID 是不泄露绝对路径的 opaque ID。API、SSE、浏览器资产和错误详情均不得包含 Provider Secret。

## Configuration precedence

NovelForge 的配置兼容规则从高到低为：

```text
--config
NOVELFORGE_CONFIG
./.novelforge/config.json
./.ainovel/config.json
~/.novelforge/config.json
~/.ainovel/config.json
built-in defaults
```

同一层级的新旧配置不会合并；`.novelforge` 完整遮蔽 `.ainovel`。项目层可以覆盖选中的全局层。`cmd/ainovel-cli` 继续使用原来的 `.ainovel` 行为。这些兼容能力不等于 Web 质量模型服务已经从配置中完成注入。

```bash
novelforge doctor
novelforge doctor --json
novelforge migrate --dry-run
novelforge migrate
```

详细规则见 [`docs/MIGRATION.md`](docs/MIGRATION.md)。

## Docker

仓库保留 Compose 构建入口：

```bash
docker compose up --build
```

默认挂载：

- `./config` → `/root/.novelforge`
- `./workspace` → `/workspace`
- Web 端口 `48090`

**本地使用前检查端口映射。** 当前 `docker-compose.yml` 仍写作 `48090:48090`，本轮文档整理没有修改该文件。需要限制为宿主机回环访问时，将其映射明确设为 `127.0.0.1:48090:48090`；下面的直接运行示例已显式限定回环地址：

```bash
docker build -t novelforge .
docker run --rm -p 127.0.0.1:48090:48090 \
  -v "$PWD/config:/root/.novelforge" \
  -v "$PWD/workspace:/workspace" \
  novelforge server --host 0.0.0.0 --workspace /workspace
```

## Model Configuration

已有运行时配置支持每个角色选择不同 Provider、Model、推理强度和 Fallback。下面是配置结构示例，不是默认 Web 质量服务已接通的证明：

```json
{
  "provider": "my-proxy",
  "model": "gpt-5-mini",
  "providers": {
    "my-proxy": {
      "type": "openai",
      "api": "responses",
      "api_key": "${YOUR_API_KEY}",
      "base_url": "https://proxy.example.com/v1",
      "models": [
        { "name": "gpt-5-mini", "context_window": 400000, "json_schema": true }
      ]
    }
  },
  "roles": {
    "architect": { "provider": "my-proxy", "model": "gpt-5-mini" },
    "writer": { "provider": "my-proxy", "model": "gpt-5-mini" }
  }
}
```

不要把真实 API Key 提交到 Git。Web 设置接口、项目复制、项目数据库与 Foundation 请求都会排除或拒绝凭据。

## Import and Export

现有 TUI 保留导入、导出和人工修改同步流程。Web 上传、EPUB、项目 ZIP Backup/Restore 与可恢复长任务属于后续路线图，当前暂不推进或专项维护。

## Development

### 当前轻量维护

本轮以 [Phase 1–8 复核清单](docs/PHASE_01_08_REVIEW.md) 为执行入口：检查调用关系、输入输出、状态转换、写入边界和失败去向。必要时只选择一个受影响的命名测试或短流程，不运行全量 Go / 前端 / race 测试、Windows / Docker 矩阵、长篇模拟或规模基准。不把未运行的检查标成通过，也不删除已有测试来降低标准。

### 既有完整开发检查（保留参考，本轮不执行）

```bash
gofmt -w ./cmd ./internal ./scripts
go vet ./...
go test ./...
go build ./cmd/novelforge

cd web
npm ci
npm run check
npm test
npm run build
cd ..

docker build .
```

既有 CI 包含 Go、Windows、Docker、Frontend、OpenAPI、CGO-disabled 构建、依赖锁文件与 Go/npm 许可证清单检查。本轮不修改 CI 配置、不伪造成功状态、不绕过必要的合并保护；纯文档复核提交使用 `[skip ci]` 避免触发全量运行。

当前维护状态见 [`docs/PHASE_01_08_REVIEW.md`](docs/PHASE_01_08_REVIEW.md)，历史实施证据见 [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md)，路线图见 [`docs/ROADMAP.md`](docs/ROADMAP.md)，上游同步流程见 [`docs/UPSTREAM_SYNC.md`](docs/UPSTREAM_SYNC.md)。

## License

NovelForge 采用 [Apache License 2.0](LICENSE)。ainovel-cli 的原始版权、提交历史和许可证信息予以保留。第三方代码复用与设计参考边界见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) 和 [`docs/LICENSES.md`](docs/LICENSES.md)。

## Credits

- **voocel/ainovel-cli** — Apache-2.0；核心代码与运行时上游
- **Nigh/show-me-the-story** — MIT；产品体验和单二进制部署设计参考，不复制源码
- **Hurricane0698/novelwriter** — AGPL-3.0；clean-room 架构参考，不复制源码
- **EthanYoQ/AI-Novel-Writer** — GPL-3.0；clean-room 工作流参考，不复制源码

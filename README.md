# NovelForge

> 面向百万字级长篇小说的 AI 自动创作与人机共创平台。

NovelForge 以 [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli) 为核心上游进行二次开发。项目保留其成熟的确定性 Engine、Agent Runtime、TUI、Headless、Checkpoint、滚动规划、上下文压缩、模型路由、导入导出与诊断能力，并在其上逐步增加单二进制 Web 工作台、结构化 Truth Store、一致性门禁、章节版本和可恢复 Autopilot。

> 当前完成了品牌与配置兼容阶段，以及只读 Server/API Foundation。原 ainovel-cli 能力可继续使用；完整 Svelte Workspace、Truth Store 和 Web Autopilot 仍按 Roadmap 实施，本文不会把未完成能力标成可用。

## Features

### 已继承并保持兼容

- TUI 与 `--headless` 两种运行方式
- Architect / Writer / Editor / Arbiter 工作流
- Checkpoint、恢复、重试、Fallback、预算哨兵
- 长篇分卷、Arc、Compass、滚动规划和摘要压缩
- OpenAI-compatible、OpenAI Responses、Anthropic、Gemini、OpenRouter 等模型通道
- `/import`、`/export`、`/sync`、`/diag`、`/simulate` 等成熟能力
- Docker 与跨平台构建基础

### NovelForge 已新增

- `novelforge` 品牌入口，同时保留 `cmd/ainovel-cli` 以降低上游同步成本
- `.novelforge` 新配置目录和 `.ainovel` 零搬迁兼容读取
- `--config`、`NOVELFORGE_CONFIG`、项目层和全局层的确定性优先级
- `novelforge doctor` 安全诊断，不输出 API Key 或配置内容
- `novelforge migrate` 显式 copy-only migration、备份、校验清单和失败回滚
- 安装、升级、卸载离线烟测；卸载永不自动删除配置
- `novelforge server`，默认监听 `127.0.0.1:48090`
- 单二进制嵌入式 Web 总览页
- `/api/health`、`/api/projects`、`/api/projects/:id` REST API
- `/api/events` Server-Sent Events 实时通道
- OpenAPI 3.1 文档 `/api/openapi.json`
- 对既有 ainovel 小说目录的真实只读发现与进度聚合
- 非回环地址监听安全告警及默认安全响应头

## Architecture

```text
User
  │
  ├── Web / CLI / Headless
  │
NovelForge API
  │
Deterministic Engine ── Agent Runtime ── Context Compiler
  │                                      │
Quality Gate ── Truth Store ── Checkpoint ── LLM Router
```

核心原则是 **事实层确定，语义层自主**：LLM 负责创意、文学表达、模糊语义判断和审稿；Go 代码负责状态迁移、验证、幂等、版本、任务、API、文件与恢复。

详细设计见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。

## Screenshots

<!-- Dashboard / Chapter Workspace / Atlas screenshots will be replaced with real release screenshots after the Svelte workspace phase. -->

- Dashboard：真实 Project、章节、字数和运行状态总览
- Chapter Workspace：章节树、Markdown 编辑器、AI Inspector、版本与 Diff
- Atlas：角色、地点、组织、关系和时间线

## Installation

需要 Go 1.25 或使用 Docker。

```bash
git clone https://github.com/feng123-new/NovelForge.git
cd NovelForge
go build -o novelforge ./cmd/novelforge
```

一行安装脚本会下载对应平台的 Release 包并校验 SHA-256：

```bash
curl -fsSL https://raw.githubusercontent.com/feng123-new/NovelForge/main/scripts/install.sh | sh
```

自定义目录：

```bash
NOVELFORGE_INSTALL_DIR="$HOME/.local/bin" \
  curl -fsSL https://raw.githubusercontent.com/feng123-new/NovelForge/main/scripts/install.sh | sh
```

卸载只删除可执行文件，不删除任何配置、API Key 或项目：

```bash
NOVELFORGE_INSTALL_DIR="$HOME/.local/bin" sh scripts/uninstall.sh
```

## Quick Start

### TUI

```bash
novelforge
```

首次设置写入 `~/.novelforge/config.json`。已有 `~/.ainovel` 或项目级 `./.ainovel` 的用户可以直接启动；NovelForge 会按兼容优先级读取，不会静默移动凭据。

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
novelforge server
```

浏览器访问 `http://127.0.0.1:48090`。指定工作区：

```bash
novelforge server --workspace /path/to/novels --host 127.0.0.1 --port 48090
```

工作区可以是一本现有 ainovel 小说目录，也可以是包含多本小说子目录的 Library。当前 Web API 为只读 Foundation，不会修改项目状态。

监听 `0.0.0.0` 或其他非回环地址时，程序会打印安全告警。不要把未认证的本地工作区直接暴露到公网。

## Configuration precedence

NovelForge 的读取顺序从高到低为：

```text
--config
NOVELFORGE_CONFIG
./.novelforge/config.json
./.ainovel/config.json
~/.novelforge/config.json
~/.ainovel/config.json
built-in defaults
```

同一层级的新旧配置不会合并；`.novelforge` 完整遮蔽 `.ainovel`。项目层可以覆盖选中的全局层。`cmd/ainovel-cli` 继续使用原来的 `.ainovel` 行为。

检查来源：

```bash
novelforge doctor
novelforge doctor --json
```

安全迁移：

```bash
novelforge migrate --dry-run
novelforge migrate
```

详细规则和回滚流程见 [`docs/MIGRATION.md`](docs/MIGRATION.md)。

## Docker

```bash
docker compose up --build
```

默认挂载：

- `./config` → `/root/.novelforge`
- `./workspace` → `/workspace`
- Web 端口 `48090`

直接运行镜像：

```bash
docker build -t novelforge .
docker run --rm -p 48090:48090 \
  -v "$PWD/config:/root/.novelforge" \
  -v "$PWD/workspace:/workspace" \
  novelforge server --host 0.0.0.0 --workspace /workspace
```

挂载旧路径 `/root/.ainovel` 仍可兼容读取，但新部署应使用 `.novelforge`。

## Model Configuration

配置支持每个角色选择不同 Provider、Model、推理强度和 Fallback。

### OpenAI-compatible example

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
      ],
      "extra": {
        "headers": { "X-Custom-Client": "novelforge" }
      },
      "extra_body": { "temperature": 0.8 }
    }
  },
  "roles": {
    "architect": { "provider": "my-proxy", "model": "gpt-5-mini" },
    "writer": { "provider": "my-proxy", "model": "gpt-5-mini" }
  }
}
```

不要把真实 API Key 提交到 Git。项目备份与导出功能将明确排除全局凭据。

## Web API

当前可用端点：

```text
GET /api/health
GET /api/projects
GET /api/projects/:id
GET /api/events
GET /api/openapi.json
```

`/api/events` 首个事件为 `connected`，后续运输层已经为 job、agent、chapter、checkpoint 与 automation 事件预留统一 Envelope。Durable Job Queue 接入后，事件将由持久化任务状态驱动，而不是依赖浏览器内状态。

## Autopilot

CLI/Headless 的既有自动创作能力继续可用。Web 中 START / PAUSE / STOP / CONTINUE 的持久化控制尚未宣称完成；它将在 Durable Job Queue、Quality Gate、Truth Store 和 Chapter Version 落地后接入同一 Engine，避免制作无后端的按钮。

## Import and Export

现有 TUI 继续提供成熟的导入、导出和人工修改同步流程。Roadmap 中将增加 Web 上传、EPUB、项目 ZIP Backup/Restore 以及可恢复的长任务状态。

## Migration from ainovel-cli

- 现有小说目录不会被删除或就地重写。
- NovelForge 优先读取 `.novelforge`，缺失时兼容读取 `.ainovel`。
- `novelforge migrate` 在复制前创建时间戳备份和 SHA-256 manifest。
- Migration 使用同级 staging + 原子 rename；失败会清理 staging，保留原目录和已完成备份。
- 新旧凭据目录不会在同一层级自动合并。
- 卸载不会删除任何配置目录。
- 项目数据格式升级必须通过 migration tests，不能由 UI 隐式执行破坏性转换。

## Development

```bash
gofmt -w ./cmd ./internal ./web
go vet ./...
go test ./...
go build ./cmd/novelforge
sh scripts/install_lifecycle_smoke.sh
sh scripts/brand_audit.sh
docker build .
```

上游同步流程见 [`docs/UPSTREAM_SYNC.md`](docs/UPSTREAM_SYNC.md)，路线图见 [`docs/ROADMAP.md`](docs/ROADMAP.md)，当前实施状态见 [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md)。新增能力优先放入新 package，通过 adapter/interface 接入，不无意义重写 ainovel-cli 核心。

## License

NovelForge 采用 [Apache License 2.0](LICENSE)。ainovel-cli 的原始版权、提交历史和许可证信息予以保留。第三方代码复用与设计参考边界见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) 和 [`docs/LICENSES.md`](docs/LICENSES.md)。

## Credits

- **voocel/ainovel-cli** — Apache-2.0；NovelForge 的核心代码与运行时上游
- **Nigh/show-me-the-story** — MIT；Web 产品体验、伏笔、Skills 和单二进制部署设计参考
- **Hurricane0698/novelwriter** — AGPL-3.0；Structured World Model、Atlas/Studio、事实审核理念参考；不复制源码
- **EthanYoQ/AI-Novel-Writer** — GPL-3.0；版本化人工审稿工作流参考；不复制源码

感谢上述项目作者和贡献者。代码复用与设计参考在法律和工程上严格区分。

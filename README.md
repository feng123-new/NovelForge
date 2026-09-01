# NovelForge

> 面向百万字级长篇小说的 AI 自动创作与人机共创平台。

NovelForge 以 [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli) 为 Apache-2.0 核心上游。项目保留成熟的确定性 Engine、Agent Runtime、TUI、Headless、Checkpoint、滚动规划、上下文压缩、模型路由、导入导出与诊断能力，并在其上增加安全项目平台、单二进制 Web 工作台、结构化 Truth Store、一致性门禁、章节版本和可恢复 Autopilot。

> 当前已完成品牌与配置兼容、可写 Project/API Foundation 和正式 Web Workspace。Truth Store、Web 章节版本与 Durable Autopilot 仍按 Roadmap 实施，本文不会把未完成能力标成可用。

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
- OpenAPI 3.1 与实现漂移测试

## Architecture

```text
Web / CLI / Headless
          │
NovelForge REST + SSE API
          │
Project Repository ─ Engine Adapter ─ Durable Events / Idempotency
          │
Deterministic Engine ─ Agent Runtime ─ Context Compiler
          │                                      │
Quality Gate ─ Truth Store ─ Checkpoint ─ LLM Router
```

核心原则是 **事实层确定，语义层自主**：LLM 负责创意、文学表达、模糊语义判断和审稿；Go 负责状态迁移、验证、幂等、版本、任务、API、文件、迁移与恢复。

详细设计见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)，Web 边界见 [`docs/WEB.md`](docs/WEB.md)。

## Installation

需要 Go 1.25 或 Docker。

```bash
git clone https://github.com/feng123-new/NovelForge.git
cd NovelForge
go build -o novelforge ./cmd/novelforge
```

Release 安装脚本会下载对应平台资产并校验 SHA-256：

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

正式工作台提供：

- **Dashboard**：真实项目数、章节、字数和服务状态
- **Projects**：筛选、归档、恢复、安全复制和回收站删除
- **New Novel**：六步创建真实项目并保存 Foundation 请求
- **Chapters**：读取项目中的真实章节元数据
- **Models**：读取 Go 运行时模型注册表
- **Logs**：显示持久化与重放 SSE 事件
- **Settings**：显示脱敏服务配置、能力与主题

New Novel 向导不会伪装 Autopilot 已运行。它保存真实 Foundation 请求并明确返回 `worker_available=false`；Durable Worker 与 START / PAUSE / STOP / RESUME 在 Phase 9 接入。

监听 `0.0.0.0` 或其他非回环地址时，程序会打印安全告警。不要把未认证的本地工作区直接暴露到公网。

## Web API

主要端点：

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

所有写接口要求 `Idempotency-Key`。同一 key 和相同请求会重放原始状态码与响应；同一 key 配合不同请求会返回冲突。集合接口使用稳定排序、分页和 `limit <= 100`。

项目 ID 是不泄露绝对路径的 opaque ID。API、SSE、浏览器资产和错误详情均不得包含 Provider Secret。

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

```bash
novelforge doctor
novelforge doctor --json
novelforge migrate --dry-run
novelforge migrate
```

详细规则见 [`docs/MIGRATION.md`](docs/MIGRATION.md)。

## Docker

```bash
docker compose up --build
```

默认挂载：

- `./config` → `/root/.novelforge`
- `./workspace` → `/workspace`
- Web 端口 `48090`

```bash
docker build -t novelforge .
docker run --rm -p 48090:48090 \
  -v "$PWD/config:/root/.novelforge" \
  -v "$PWD/workspace:/workspace" \
  novelforge server --host 0.0.0.0 --workspace /workspace
```

## Model Configuration

配置支持每个角色选择不同 Provider、Model、推理强度和 Fallback。

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

现有 TUI 继续提供导入、导出和人工修改同步流程。Roadmap 将增加 Web 上传、EPUB、项目 ZIP Backup/Restore 以及可恢复长任务状态。

## Development

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

CI 验证 Go、Windows、Docker、Frontend、OpenAPI、CGO-disabled 构建、依赖锁文件与 Go/npm 许可证清单。

当前实施状态见 [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md)，路线图见 [`docs/ROADMAP.md`](docs/ROADMAP.md)，上游同步流程见 [`docs/UPSTREAM_SYNC.md`](docs/UPSTREAM_SYNC.md)。

## License

NovelForge 采用 [Apache License 2.0](LICENSE)。ainovel-cli 的原始版权、提交历史和许可证信息予以保留。第三方代码复用与设计参考边界见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) 和 [`docs/LICENSES.md`](docs/LICENSES.md)。

## Credits

- **voocel/ainovel-cli** — Apache-2.0；核心代码与运行时上游
- **Nigh/show-me-the-story** — MIT；产品体验和单二进制部署设计参考，不复制源码
- **Hurricane0698/novelwriter** — AGPL-3.0；clean-room 架构参考，不复制源码
- **EthanYoQ/AI-Novel-Writer** — GPL-3.0；clean-room 工作流参考，不复制源码

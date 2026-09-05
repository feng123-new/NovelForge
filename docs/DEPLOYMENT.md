# 本地部署与首次使用

当前交付目标是可部署候选版，不是已完成 Phase 13B 全量验收的稳定版。请先使用测试工作区，确认平台、模型和恢复流程后再接入正式作品。发行清单区分“交叉编译成功”和“目标平台运行验证”。普通用户运行二进制不需要 Go、Node 或 Python；源码开发和验证工具有单独依赖。

## 1. 获取并核对候选包

从仓库 Releases 选择明确的 `v0.1.0-rc.N`，按操作系统与架构下载。Windows 为 ZIP，Linux/macOS 为 tar.gz；`x86_64` 与 `amd64` 对应，Apple Silicon 使用 Darwin arm64。对比 `novelforge_checksums.txt` 中该文件的 SHA-256。Linux 可用 `sha256sum <file>`，macOS 用 `shasum -a 256 <file>`，PowerShell 用 `Get-FileHash <file> -Algorithm SHA256`。SHA-256 不是系统级代码签名；没有 notarization/签名证书的保证。

解压后检查 `BUILD_INFO.json` 和 `novelforge --version`，两者应与 release-manifest 中的提交、标签一致。不要混用不同候选版的程序、前端产物或数据库副本。候选版不设为 latest；已有安装脚本需要明确版本参数，例如 `sh scripts/install.sh v0.1.0-rc.1`，或直接使用解压后的程序。不要指望只有预发布版本时 `/releases/latest` 返回该版本。

## 2. 无模型查看工作台

Linux/macOS，在解压目录：

```sh
./novelforge server --workspace ./workspace-test --no-autopilot
```

Windows PowerShell：

```powershell
.\novelforge.exe server --workspace .\workspace-test --no-autopilot
```

浏览器打开 `http://127.0.0.1:48090`。`--no-autopilot` 不启动任务 Worker，不自动恢复旧任务；可以检查项目、版本、诊断和备份。但它不是全局只读模式，用户仍可显式执行有模型依赖的手工审核操作。正常启动默认启用 Worker，可能恢复该工作区已有的未结束任务；首次部署请用新目录，不要误指向正在创作的工作区。

不带子命令的 `novelforge` 保留旧 TUI；本文及新阶段功能以 `novelforge server` 为入口。启动器 `scripts/start-web.sh`、`scripts/start-web.ps1` 使用解压目录下的程序和 workspace。可用 `NOVELFORGE_WORKSPACE`、`NOVELFORGE_CONFIG`、`NOVELFORGE_PORT` 指定位置。PowerShell 启动器不改变系统执行策略；受限制时直接运行 exe 命令。

## 3. 配置真实模型

复制 `examples/server-config.json` 到源码仓库外的本地文件，替换 provider endpoint 和 model ID。示例仅演示 OpenAI-compatible chat 接口，不代表其他服务商类型也使用同一参数。真实 provider 类型、角色路由、fallback 的完整配置以源码 `config.example.jsonc` 为准。

示例使用 `${NOVELFORGE_API_KEY}` 环境变量占位。Linux/macOS 可通过不回显输入设置当前终端环境变量；PowerShell 可用安全提示或本地凭据管理器注入。不要把 Key 写入命令示例、Git 提交、截图或诊断附件。占位符未解析时相关质量服务不可用，系统不会把模型目录存在视为已配置成功。

```sh
./novelforge server --workspace ./workspace-test --config /absolute/path/models.json
```

```powershell
.\novelforge.exe server --workspace .\workspace-test --config C:\Private\models.json
```

默认仅监听本机，未提供完整多用户认证。不要直接开放公网端口。云模型会收到被选择的正文和参考资料，本地优先不等于模型调用永不离开电脑。关闭终端时先暂停任务、确认当前步骤结束，再使用 Ctrl+C。

## 4. 第一个完整流程

创建 New Novel → 填写基础设定请求 → 在 Skills & Libraries 保存适用章节/视角的资料 → 在 Diagnostics & Cost 配置准确价格和小额预算/次数上限 → 在 Autopilot 显式启动有限章节任务。先设置逐章人工审阅和小范围目标，查看候选稿后批准，不直接从百章规模开始。

写作中查看任务、调用明细和诊断；金额未知不等于免费。存在不确定请求时先核对服务商记录，不能盲目对账为 0。模型价格是用户维护的估算表，缓存或其他特殊计费尚未分项折算，最终账单以服务商为准。

人工修改在 Versions 保存候选，Check/Accept/Finalize 后才成为有效定稿。Import & Backup 的上传只暂存，分析需要明确同意；导入分析不会自动接受事实。按连续有效定稿范围导出 TXT/Markdown/EPUB。详细边界见 [诊断](DIAGNOSTICS.md)、[作品生命周期](LIFECYCLE.md)、[自动创作](AUTOPILOT.md)。

## 5. Docker（本地可选）

在匹配候选标签的源码目录中，使用已有 Dockerfile/Compose 构建。先审阅 Compose 的 workspace 挂载和配置位置，再运行 `docker compose up --build`。端口默认应为 `127.0.0.1:48090:48090`。模型配置应放入挂载的项目配置位置或通过你明确管理的只读挂载传入，不能把凭据写进镜像。不同系统的挂载权限、原生模型地址和容器内 `localhost` 的含义需要本地验证。

此候选交付不等于已经运行 Docker/Windows/macOS 完整验收。对应二进制已构建与否看 release-manifest；容器测试可以从 Full acceptance 工作流显式发起。

## 6. 升级、备份和回滚

升级前停止任务，保存项目备份，并另外私下备份模型配置。项目 ZIP 会排除配置凭据和工作区任务，不能用它证明全部运行环境也已备份。保留旧程序、旧提交、迁移前数据与验证记录。

新版本在独立副本/工作区打开，完成 [本地验收](LOCAL_ACCEPTANCE.md) 的短流程后再迁移正式工作区。若数据库已经升级，不要只换回旧程序强行打开；应在新工作区恢复对应旧备份。恢复保留原项目 ID，不覆盖同 ID 项目，也不会自动复活写作任务。出现未完成 Final/rebuild 时先恢复原操作，不删除数据库或历史来“修复”。

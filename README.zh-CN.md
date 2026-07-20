<p align="center">
  <img alt="Grok2API" src="./frontend/public/grok2api.png" width="720" />
</p>

<p align="center">
  <strong>面向 Grok Build、Grok Web 与 Grok Console 的多账号 API 网关</strong>
</p>

<p align="center">
  <a href="./README.md">English</a> | 简体中文
</p>

<p align="center">
  <a href="./backend/go.mod"><img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" /></a>
  <a href="./frontend/package.json"><img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=111827" /></a>
  <a href="https://github.com/chenyme/grok2api/pkgs/container/grok2api"><img alt="Docker" src="https://img.shields.io/badge/Docker-amd64%20%7C%20arm64-2496ED?logo=docker&logoColor=white" /></a>
</p>

本项目为对 [chenyme/grok2api](https://github.com/chenyme/grok2api) 的**二次修改与增强**。

> [!NOTE]
> 本项目仅供学习与研究。使用者必须在遵循 Grok 的**使用条款**以及**法律法规**的情况下使用，不得用于非法用途。

基于上游 Go + React 架构继续演进，保留 Build / Web / Console 三 Provider 账号池、OpenAI / Anthropic 兼容接口与管理后台，并在此基础上增强**逻辑账号组、IP 管理、外部批量导入、Build 账号巡检与响应恢复**等运维能力。

## 本仓库相对上游的增强

- **逻辑账号组**：将同一登录身份下的 Web / Build / Console 凭据归入同一逻辑账号组，统一展示成员与出口身份
- **IP 管理**：独立「IP 管理」菜单维护可复用代理资源（HTTP / HTTPS / SOCKS5 / SOCKS5H），支持连接测试、启停与引用计数
- **组级代理绑定**：逻辑账号组最多绑定一个固定代理，三类 Provider 上游请求共用；已绑定代理不可用时严格失败，不自动换 IP 或直连
- **批量绑定代理**：逻辑账号列表支持当前页勾选，在事务内批量绑定 / 切换 / 解绑代理
- **外部账号导入**：`POST /api/admin/v1/account-imports` 一次导入邮箱、Build OAuth、Web/Console SSO 与代理绑定，按条事务保证完整落库
- **逻辑账号删除**：按组原子删除全部 Provider 成员及关联运行态，保留可复用代理与已生成媒体资产
- **Build 账号巡检台**：管理端提供 Build 账号巡检工作台，汇总候选、分类状态与巡检结果
- **响应恢复增强**：改进 Build 侧 reasoning recovery 与 compaction 转发，提升多轮与压缩场景稳定性

设计说明见：

- [逻辑账号组统一代理](./docs/design/account-family-proxy-binding.md)
- [逻辑账号组删除](./docs/design/account-family-deletion.md)
- [外部系统逻辑账号导入](./docs/design/external-account-family-import.md)

## 功能概览（含上游能力）

- **三 Provider**：Build、Web、Console 分别维护凭据、额度、健康、冷却、并发和模型能力
- **兼容接口**：Responses、Chat Completions、Anthropic Messages、Images 与异步 Videos
- **模型路由**：动态模型发现、静态目录、来源限定、客户端权限和账号能力过滤
- **多账号调度**：优先级、额度门控、会话粘滞、并发租约、冷却和有界故障切换
- **多轮兼容**：stored response 归属、compaction，以及可选的服务端 reasoning replay
- **媒体链路**：图片生成、图片编辑、视频任务、本地归档和 URL/Base64/SSE 输出
- **账号关联与逻辑账号组**：跨 Provider 弱关联 + 本仓库逻辑账号组与组级代理
- **运行基础设施**：SQLite/PostgreSQL、Memory/Redis、HTTP/SOCKS5/Resin 出口
- **管理后台**：Dashboard、账号、逻辑账号、IP 管理、模型、密钥、图库、视频库、请求审计、Build 巡检、运行设置和版本检查
- **可选账号自动清理**（默认关闭）：运行设置可按间隔硬删除已标记 `reauthRequired` 且 `reauth_marked_at` 超过最短保留时长的账号。不会选中纯冷却账号，也不会打断仍可 drain 的永久 refresh 账号；仍有推理租约或排队中/进行中视频任务的账号会被跳过。共享运行态下使用分布式维护锁避免多实例重复执行，每次 tick 采用有限删除预算。启用后与进程启动后的首次扫描均等待一个间隔，且只有清理策略实际变化时才重排下一次扫描。

## 架构设计

```mermaid
flowchart TB
    Client["OpenAI / Anthropic Clients"] --> Compat["Compatibility API"]
    Admin["React Admin"] --> AdminAPI["Admin API"]

    Compat --> App["Application Services"]
    AdminAPI --> App

    App --> Router["Model Router"]
    Router --> Selector["Account Selector"]
    Selector --> Registry["Provider Registry"]

    Registry --> Build["Grok Build Adapter"]
    Registry --> Web["Grok Web Adapter"]
    Registry --> Console["Grok Console Adapter"]

    App --> DB["SQLite / PostgreSQL"]
    App --> Runtime["Memory / Redis"]
    Build --> Egress["Egress Manager"]
    Web --> Egress
    Console --> Egress
    App --> Media["Media Storage"]
    App --> Family["Account Family + Proxy"]
```

请求不会在三个 Provider 之间混用账号状态：

1. HTTP 层完成鉴权、输入上限和协议识别。
2. 模型路由将公开模型名解析为 Provider 限定的内部路由。
3. Provider Registry 根据声明式能力判断是否支持当前协议或媒体操作。
4. 账号选择器在目标 Provider 内按模型能力、额度、粘滞、冷却和并发选号。
5. 对应 Adapter 完成上游协议转换与转发；若逻辑账号组已绑定代理，出口严格使用该代理。
6. 审计、额度、计费、响应归属和并发租约在请求结束时统一结算。

### Provider 能力边界

| Provider | 认证 | 模型目录 | 额度来源 | 对外能力 |
| :-- | :-- | :-- | :-- | :-- |
| Grok Build | OAuth / Device OAuth | 按账号从上游发现 | Billing | Responses、Chat、Messages、Compact、stored responses、Video |
| Grok Web | SSO | 内置目录并按账号等级过滤 | 上游额度窗口 | Responses、Chat、Messages、Images、Image Edit、Video |
| Grok Console | SSO | 内置目录 | 本地窗口 | 无状态 Responses、Chat、Messages |

### 技术栈

| 层 | 主要技术 |
| :-- | :-- |
| Backend | Go 1.26、Gin、GORM |
| Frontend | React 19、TypeScript、Vite、Tailwind CSS、shadcn/ui |
| Database | SQLite / PostgreSQL |
| Runtime | Memory / Redis |

### 工程结构

```text
backend/
  cmd/grok2api/          进程入口
  internal/domain/      领域模型与稳定规则
  internal/application/ 用例、调度与结算
  internal/infra/       Provider、数据库、运行态、出口与安全实现
  internal/transport/   HTTP 路由、鉴权和 DTO
frontend/
  src/app/              路由、布局和全局 Provider
  src/features/         按业务能力组织的页面与交互
  src/entities/         跨功能领域对象
  src/shared/           API、鉴权、组件与通用工具
docs/design/            本仓库增强相关设计文档
```

## 快速部署

### Docker Compose（推荐）

```bash
git clone <your-fork-url>
cd grok2api
cp config.example.yaml config.yaml
```

生成安全密钥：

```bash
openssl rand -hex 32
openssl rand -base64 32
```

将结果写入 `config.yaml`，并修改首次管理员密码：

```yaml
secrets:
  jwtSecret: "替换为 hex 随机值"
  credentialEncryptionKey: "替换为 Base64 随机密钥"

bootstrapAdmin:
  username: "admin"
  password: "替换为强密码"
```

启动服务：

```bash
docker compose pull
docker compose up -d
docker compose logs -f grok2api
```

管理端默认地址：`http://127.0.0.1:8000`。

### 源码运行

```bash
cp config.example.yaml config.yaml
make run
```

单独启动前端开发服务器：

```bash
cd frontend
pnpm install
pnpm dev
```

前端默认运行在 `http://127.0.0.1:5173`，并将 API 请求代理到 `http://127.0.0.1:8000`。

## 首次使用

1. 使用 `bootstrapAdmin` 创建的管理员登录。
2. 在「IP 管理」中按需创建代理资源（可选）。
3. 在「上游账号」中接入 Build、Web 或 Console 账号；或通过外部导入接口批量写入逻辑账号组。
4. 在「逻辑账号」中绑定 / 批量绑定组级代理（可选）。
5. 等待账号额度和模型能力完成首次同步。
6. 在「模型路由」中确认公开模型名、来源和启用状态。
7. 在「客户端密钥」中创建 `g2a_` API Key。
8. 使用该密钥调用 `/v1/*`。

管理员创建成功后，建议修改密码并从配置中删除 `bootstrapAdmin`。`credentialEncryptionKey` 必须长期保留，更换后已有凭据将无法解密。

## 模型与路由

公开模型名默认不带来源前缀。内部使用 `Build/`、`Web/`、`Console/` 作为稳定路由 ID。请始终以管理端模型页或以下接口为准：

```http
GET /v1/models
```

### Grok Web 内置模型

| 模型 | 能力 | 最低等级 |
| :-- | :-- | :-- |
| `grok-chat-fast` | Chat / Responses / Messages | Basic |
| `grok-chat-auto` | Chat / Responses / Messages | Super |
| `grok-chat-expert` | Chat / Responses / Messages | Super |
| `grok-chat-heavy` | Chat / Responses / Messages | Heavy |
| `grok-imagine-image` | 图片生成 | Basic |
| `grok-imagine-image-quality` | 高质量图片生成 | Super |
| `grok-imagine-image-edit` | 图片编辑 | Super |
| `grok-imagine-video` | 视频生成 | Super |

### Grok Console 内置模型

| 模型 | 说明 |
| :-- | :-- |
| `grok-4.3` | 支持 reasoning effort 与搜索工具 |
| `grok-4.20-0309` | 通用 Responses 模型 |
| `grok-4.20-0309-reasoning` | Reasoning 版本 |
| `grok-4.20-0309-non-reasoning` | Non-reasoning 版本 |
| `grok-4.20-multi-agent-0309` | Multi-agent 版本 |
| `grok-build-0.1` | Build 系列模型 |

Console 还提供兼容别名和 reasoning effort 别名。Console 保持无状态语义，不支持 `previous_response_id`、Response 查询/删除或 compact。

Build 模型按账号真实能力动态发现，不属于 Console 静态目录。

## API

客户端推理接口需要 API Key：

```http
Authorization: Bearer g2a_xxx_xxx
```

| 方法 | 路径 | 说明 |
| :-- | :-- | :-- |
| `GET` | `/healthz` | 存活检查 |
| `GET` | `/readyz` | 分层就绪状态 |
| `GET` | `/v1/models` | 当前可服务模型 |
| `POST` | `/v1/responses` | Responses JSON / SSE |
| `POST` | `/v1/responses/compact` | Responses compact |
| `GET` | `/v1/responses/{id}` | 查询 stored response |
| `DELETE` | `/v1/responses/{id}` | 删除 stored response |
| `POST` | `/v1/chat/completions` | Chat Completions JSON / SSE |
| `POST` | `/v1/messages` | Anthropic Messages JSON / SSE |
| `POST` | `/v1/images/generations` | 图片生成 |
| `POST` | `/v1/images/edits` | 图片编辑，支持 JSON 与 multipart |
| `POST` | `/v1/videos/generations` | 创建异步视频任务 |
| `GET` | `/v1/videos/{request_id}` | 查询视频任务 |
| `GET` | `/v1/videos/{request_id}/content` | 获取视频任务内容 |
| `GET` | `/v1/media/images/{asset_id}` | 读取归档图片 |
| `GET` | `/v1/media/videos/{asset_id}` | 读取归档视频 |
| `PUT` | `/v1/media/uploads/{token}` | 使用一次性票据接收视频上传 |

### 管理端增强接口（本仓库）

| 方法 | 路径 | 说明 |
| :-- | :-- | :-- |
| `GET/POST/PATCH/DELETE` | `/api/admin/v1/proxies` | IP / 代理资源管理 |
| `POST` | `/api/admin/v1/proxies/:id/test` | 代理连接测试 |
| `GET` | `/api/admin/v1/account-families` | 逻辑账号组分页查询 |
| `PUT` | `/api/admin/v1/account-families/:id/proxy` | 绑定 / 切换 / 解绑组级代理 |
| `POST` | `/api/admin/v1/account-families/batch/proxy` | 当前页批量绑定代理 |
| `DELETE` | `/api/admin/v1/account-families/:id` | 按组删除逻辑账号 |
| `POST` | `/api/admin/v1/account-imports` | 外部系统批量导入逻辑账号 |

最小调用示例：

```bash
export GROK2API_API_KEY="g2a_xxx_xxx"

curl http://127.0.0.1:8000/v1/responses \
  -H "Authorization: Bearer $GROK2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-chat-auto",
    "input": "用三句话解释量子隧穿",
    "stream": true
  }'
```

## 配置、运行态与多实例

`config.yaml` 只保存启动所需配置：

| 分组 | 说明 |
| :-- | :-- |
| `server` | 监听地址、请求体限制、超时和 Swagger |
| `auth` | 管理端 Token 与安全 Cookie |
| `secrets` | JWT 与凭据加密密钥 |
| `frontend` | 静态资源目录和可选公开地址 |
| `database` | SQLite 或 PostgreSQL |
| `runtimeStore` | Memory 或 Redis |
| `media` | 媒体存储驱动与路径 |
| `routing` | 服务端多轮回放缓存 |

| 场景 | 数据库 | 运行态 | 媒体 |
| :-- | :-- | :-- | :-- |
| 单实例 | SQLite | Memory | 本地目录 |
| 多实例 | PostgreSQL | Redis | 共享卷或实例亲和 |

### 账号调度与逻辑账号组

- 会话粘滞命中时优先复用原账号；账号暂时满载时会短暂等待，再按规则借用其它可用账号。
- Web 可以与对应的 Build、Console 建立一对一弱关联；本仓库进一步以逻辑账号组统一成员与代理绑定。
- 逻辑账号组已绑定代理时，三类 Provider 共用该出口，绑定失败则请求失败。
- 未绑定代理时，继续使用现有按 Provider 作用域划分的出口节点池（含 Resin `{account}` 占位符）。
- Email 仅用于展示和检索，不作为代理身份。

### FlareSolverr 自动维护 Clearance

如需自动维护 Grok Web / Console 的 Cloudflare Clearance，可启动可选的 FlareSolverr Compose 服务：

```bash
docker compose --profile flaresolverr up -d
# 或
podman compose --profile flaresolverr up -d
```

随后在管理端打开 **运行设置 → 媒体与网络 → Clearance**，选择 `FlareSolverr`，并将服务地址设为 `http://flaresolverr:8191`。FlareSolverr 不会暴露到宿主机；每个 Web 或 Console 出口节点均使用自身代理获取匹配的 Cookie 与 User-Agent。

### Resin 粘性代理

```text
socks5h://Default.{account}:RESIN_PROXY_TOKEN@resin:2260
```

运行时会将 `{account}` 替换为稳定、匿名的账号身份。已关联 / 同组账号可复用同一身份。

## 安全与生产建议

- 使用 HTTPS，并在 HTTPS 管理地址下启用 `auth.secureCookies`
- 使用强随机 `jwtSecret` 和 `credentialEncryptionKey`
- 生产环境保持 `server.swaggerEnabled: false`
- 不要将 OAuth、SSO、Cookie、账号导出或真实数据库提交到 Git
- 多实例使用 PostgreSQL 与 Redis，并为媒体配置共享卷或实例亲和
- 备份 `config.yaml`、关系型数据库和媒体目录
- 公网部署建议使用反向代理、访问控制和基础网络防护
- 外部导入接口暴露面较大，生产环境务必限制访问来源

## 开发与验证

后端：

```bash
cd backend
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/grok2api
```

前端：

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm lint
pnpm build
```

修改公开 API 注释后，在仓库根目录执行：

```bash
make swagger
```

## 进一步阅读

- [English README](./README.md)
- [后端说明](./backend/README.md)
- [前端说明](./frontend/README.md)
- [上游项目 chenyme/grok2api](https://github.com/chenyme/grok2api)

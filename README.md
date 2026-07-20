<p align="center">
  <img alt="Grok2API" src="./frontend/public/grok2api.png" width="720" />
</p>

<p align="center">
  <strong>A multi-account API gateway for Grok Build, Grok Web, and Grok Console</strong>
</p>

<p align="center">
  English | <a href="./README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="./backend/go.mod"><img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" /></a>
  <a href="./frontend/package.json"><img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=111827" /></a>
  <a href="https://github.com/chenyme/grok2api/pkgs/container/grok2api"><img alt="Docker" src="https://img.shields.io/badge/Docker-amd64%20%7C%20arm64-2496ED?logo=docker&logoColor=white" /></a>
</p>

This project is a **secondary modification and enhancement** of [chenyme/grok2api](https://github.com/chenyme/grok2api).

> [!NOTE]
> This project is for learning and research only. You must follow Grok's **Terms of Use** and applicable **laws and regulations**. Do not use it for illegal purposes.

Built on the upstream Go + React architecture, it keeps independent Build / Web / Console account pools, OpenAI- and Anthropic-compatible APIs, and the admin console, and adds **logical account families, IP management, external bulk import, Build account inspection, and response recovery** improvements.

## Enhancements in this fork

- **Logical account families**: Group Web / Build / Console credentials that share one login identity; manage members and egress identity together
- **IP management**: Dedicated proxy resource page for reusable HTTP / HTTPS / SOCKS5 / SOCKS5H proxies, with connection tests, enable/disable, and reference counts
- **Family-level proxy binding**: Each logical family binds at most one fixed proxy shared by all three Providers; bound proxies fail closed (no auto IP switch or direct fallback)
- **Batch proxy binding**: Select families on the current page and bind / switch / unbind proxies in a single transaction
- **External account import**: `POST /api/admin/v1/account-imports` imports email, Build OAuth, Web/Console SSO, and proxy binding as one atomic per-row operation
- **Family deletion**: Atomically delete a family and all Provider members plus related runtime state; reusable proxies and generated media assets are retained
- **Build inspection workbench**: Admin console workbench that classifies Build accounts and surfaces inspection results
- **Response recovery improvements**: Stronger Build-side reasoning recovery and compaction forwarding for multi-turn / compact stability

Design docs:

- [Account family proxy binding](./docs/design/account-family-proxy-binding.md)
- [Account family deletion](./docs/design/account-family-deletion.md)
- [External account family import](./docs/design/external-account-family-import.md)

## Highlights (including upstream)

- **Three Providers**: Build, Web, and Console keep credentials, quotas, health, cooldowns, concurrency, and model capabilities separate
- **Compatible APIs**: Responses, Chat Completions, Anthropic Messages, Images, and asynchronous Videos
- **Model routing**: remote discovery, static catalogs, source pinning, client permissions, and per-account capability filtering
- **Multi-account scheduling**: priorities, quota gates, sticky sessions, concurrency leases, cooldowns, and bounded failover
- **Multi-turn compatibility**: stored-response ownership, compaction, and optional server-side reasoning replay
- **Media pipeline**: image generation, image editing, video jobs, local archiving, and URL/Base64/SSE output
- **Account relationships & families**: cross-Provider weak links plus this fork's logical families and family-level proxies
- **Runtime infrastructure**: SQLite/PostgreSQL, Memory/Redis, and HTTP/SOCKS5/Resin egress
- **Admin console**: dashboard, accounts, logical families, IP management, model routes, client keys, media libraries, audits, Build inspection, runtime settings, and update checks

## Architecture

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

Requests never mix account state across Providers:

1. The HTTP layer handles authentication, request limits, and protocol detection.
2. The model router resolves a public model name to a Provider-qualified internal route.
3. The Provider Registry verifies that the selected source supports the requested protocol or media operation.
4. The account selector chooses an eligible account from that Provider using capability, quota, stickiness, cooldown, and concurrency state.
5. The matching Adapter performs upstream protocol conversion and forwarding; if the logical family has a bound proxy, egress uses that proxy strictly.
6. Audit, quota, billing, response ownership, and concurrency leases are finalized once at the end of the request.

### Provider boundaries

| Provider | Authentication | Model catalog | Quota authority | Exposed capabilities |
| :-- | :-- | :-- | :-- | :-- |
| Grok Build | OAuth / Device OAuth | Discovered per account | Billing | Responses, Chat, Messages, Compact, stored responses, Video |
| Grok Web | SSO | Built in and filtered by account tier | Upstream quota windows | Responses, Chat, Messages, Images, Image Edit, Video |
| Grok Console | SSO | Built in | Local window | Stateless Responses, Chat, Messages |

### Technology stack

| Layer | Technology |
| :-- | :-- |
| Backend | Go 1.26, Gin, GORM |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS, shadcn/ui |
| Database | SQLite / PostgreSQL |
| Runtime | Memory / Redis |

### Repository layout

```text
backend/
  cmd/grok2api/          Process entry point
  internal/domain/      Domain models and stable rules
  internal/application/ Use cases, scheduling, and finalization
  internal/infra/       Providers, persistence, runtime, egress, and security
  internal/transport/   HTTP routes, authentication, and DTOs
frontend/
  src/app/              Routing, application shell, and global providers
  src/features/         Feature-oriented pages and interactions
  src/entities/         Shared domain objects
  src/shared/           API client, auth, components, and utilities
docs/design/            Design notes for fork enhancements
```

## Quick start

### Docker Compose (recommended)

```bash
git clone <your-fork-url>
cd grok2api
cp config.example.yaml config.yaml
```

Generate secure secrets:

```bash
openssl rand -hex 32
openssl rand -base64 32
```

Write the generated values to `config.yaml` and replace the bootstrap password:

```yaml
secrets:
  jwtSecret: "replace-with-the-generated-hex-value"
  credentialEncryptionKey: "replace-with-the-generated-base64-key"

bootstrapAdmin:
  username: "admin"
  password: "replace-with-a-strong-password"
```

Start the service:

```bash
docker compose pull
docker compose up -d
docker compose logs -f grok2api
```

The admin console is available at `http://127.0.0.1:8000` by default.

### Run from source

```bash
cp config.example.yaml config.yaml
make run
```

To run the frontend development server separately:

```bash
cd frontend
pnpm install
pnpm dev
```

The frontend runs at `http://127.0.0.1:5173` by default and proxies API requests to `http://127.0.0.1:8000`.

## First-time setup

1. Sign in with the administrator created from `bootstrapAdmin`.
2. Optionally create proxy resources under **IP Management**.
3. Add Build, Web, or Console accounts under **Upstream Accounts**, or bulk-import logical families via the external import API.
4. Optionally bind / batch-bind family-level proxies under **Logical Accounts**.
5. Wait for the initial quota and model-capability sync to complete.
6. Review public model names, sources, and enabled routes under **Model Routes**.
7. Create a `g2a_` API key under **Client Keys**.
8. Use that key to call `/v1/*`.

After the administrator has been created, change its password and remove `bootstrapAdmin` from the configuration. Keep `credentialEncryptionKey` permanently: changing it makes existing encrypted credentials unreadable.

## Models and routing

Public model names are unqualified by default. Internally, `Build/`, `Web/`, and `Console/` are used as stable route IDs. Always use the model page or this endpoint as the source of truth:

```http
GET /v1/models
```

### Built-in Grok Web models

| Model | Capability | Minimum tier |
| :-- | :-- | :-- |
| `grok-chat-fast` | Chat / Responses / Messages | Basic |
| `grok-chat-auto` | Chat / Responses / Messages | Super |
| `grok-chat-expert` | Chat / Responses / Messages | Super |
| `grok-chat-heavy` | Chat / Responses / Messages | Heavy |
| `grok-imagine-image` | Image generation | Basic |
| `grok-imagine-image-quality` | High-quality image generation | Super |
| `grok-imagine-image-edit` | Image editing | Super |
| `grok-imagine-video` | Video generation | Super |

### Built-in Grok Console models

| Model | Description |
| :-- | :-- |
| `grok-4.3` | Supports reasoning effort and search tools |
| `grok-4.20-0309` | General Responses model |
| `grok-4.20-0309-reasoning` | Reasoning variant |
| `grok-4.20-0309-non-reasoning` | Non-reasoning variant |
| `grok-4.20-multi-agent-0309` | Multi-agent variant |
| `grok-build-0.1` | Build-family model |

Console also exposes compatibility and reasoning-effort aliases. Console is stateless and does not support `previous_response_id`, Response retrieval/deletion, or compact.

Build models are discovered from account capabilities and are not part of the Console static catalog.

## API

Client inference endpoints require an API key:

```http
Authorization: Bearer g2a_xxx_xxx
```

| Method | Path | Description |
| :-- | :-- | :-- |
| `GET` | `/healthz` | Liveness check |
| `GET` | `/readyz` | Layered readiness status |
| `GET` | `/v1/models` | Currently serviceable models |
| `POST` | `/v1/responses` | Responses JSON / SSE |
| `POST` | `/v1/responses/compact` | Responses compact |
| `GET` | `/v1/responses/{id}` | Retrieve a stored response |
| `DELETE` | `/v1/responses/{id}` | Delete a stored response |
| `POST` | `/v1/chat/completions` | Chat Completions JSON / SSE |
| `POST` | `/v1/messages` | Anthropic Messages JSON / SSE |
| `POST` | `/v1/images/generations` | Image generation |
| `POST` | `/v1/images/edits` | Image editing with JSON or multipart input |
| `POST` | `/v1/videos/generations` | Create an asynchronous video job |
| `GET` | `/v1/videos/{request_id}` | Inspect a video job |
| `GET` | `/v1/videos/{request_id}/content` | Retrieve video job content |
| `GET` | `/v1/media/images/{asset_id}` | Read an archived image |
| `GET` | `/v1/media/videos/{asset_id}` | Read an archived video |
| `PUT` | `/v1/media/uploads/{token}` | Receive a video through a one-time upload ticket |

### Admin enhancements in this fork

| Method | Path | Description |
| :-- | :-- | :-- |
| `GET/POST/PATCH/DELETE` | `/api/admin/v1/proxies` | IP / proxy resource management |
| `POST` | `/api/admin/v1/proxies/:id/test` | Test a proxy connection |
| `GET` | `/api/admin/v1/account-families` | List logical account families |
| `PUT` | `/api/admin/v1/account-families/:id/proxy` | Bind / switch / unbind a family proxy |
| `POST` | `/api/admin/v1/account-families/batch/proxy` | Batch-bind proxies for selected families |
| `DELETE` | `/api/admin/v1/account-families/:id` | Delete a logical family |
| `POST` | `/api/admin/v1/account-imports` | Bulk-import logical families from an external system |

Minimal request example:

```bash
export GROK2API_API_KEY="g2a_xxx_xxx"

curl http://127.0.0.1:8000/v1/responses \
  -H "Authorization: Bearer $GROK2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-chat-auto",
    "input": "Explain quantum tunneling in three sentences.",
    "stream": true
  }'
```

## Configuration, runtime state, and multi-instance deployments

`config.yaml` contains startup configuration only:

| Group | Description |
| :-- | :-- |
| `server` | Listen address, request limits, timeouts, and Swagger |
| `auth` | Admin token lifetime and secure cookies |
| `secrets` | JWT and credential-encryption keys |
| `frontend` | Static assets and the optional public address |
| `database` | SQLite or PostgreSQL |
| `runtimeStore` | Memory or Redis |
| `media` | Media storage driver and path |
| `routing` | Server-side multi-turn replay cache |

| Deployment | Database | Runtime store | Media |
| :-- | :-- | :-- | :-- |
| Single instance | SQLite | Memory | Local directory |
| Multiple instances | PostgreSQL | Redis | Shared volume or instance affinity |

### Account scheduling and logical families

- A sticky-session hit prefers the account already bound to the conversation. If that account is temporarily full, the selector waits briefly before borrowing another eligible account.
- Web accounts can form one-to-one weak links with corresponding Build and Console accounts; this fork further unifies members and proxy binding through logical families.
- When a family has a bound proxy, all three Providers share that egress endpoint; binding failures fail the request.
- Unbound families continue to use the existing Provider-scoped egress node pools (including Resin `{account}` placeholders).
- Email addresses are used only for display and search, never as proxy identities.

### Resin sticky proxies

```text
socks5h://Default.{account}:RESIN_PROXY_TOKEN@resin:2260
```

At runtime, `{account}` is replaced with a stable anonymous account identity. Linked / same-family accounts can reuse the same identity.

## Security and production guidance

- Serve the application over HTTPS and enable `auth.secureCookies` for an HTTPS admin address
- Generate strong random values for `jwtSecret` and `credentialEncryptionKey`
- Keep `server.swaggerEnabled: false` in production
- Never commit OAuth data, SSO tokens, cookies, account exports, or real databases
- Use PostgreSQL and Redis for multi-instance deployments, plus shared media storage or instance affinity
- Back up `config.yaml`, the relational database, and the media directory
- Place a reverse proxy, access controls, and basic network protections in front of public deployments
- The external import endpoint has a large attack surface — restrict network access in production

## Development and verification

Backend:

```bash
cd backend
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/grok2api
```

Frontend:

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm lint
pnpm build
```

After changing public API annotations, regenerate Swagger from the repository root:

```bash
make swagger
```

## Further reading

- [简体中文 README](./README.zh-CN.md)
- [Backend guide](./backend/README.md)
- [Frontend guide](./frontend/README.md)
- [Upstream chenyme/grok2api](https://github.com/chenyme/grok2api)

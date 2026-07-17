# 外部系统逻辑账号导入接口设计

## 目标理解

- 外部系统通过一个 JSON 接口批量导入上游账号。
- 每条数据包含邮箱、Build OAuth 凭据、Web SSO 凭据和代理地址。
- `email` 作为逻辑账号列表及三种 Provider 成员的展示名称。
- OAuth 凭据生成或更新 Grok Build，SSO 凭据生成或更新 Grok Web 和 Grok Console。
- 三种 Provider 凭据必须归入同一个逻辑账号组，并统一绑定同一个代理。
- `proxy_url` 仅匹配“IP 管理”中的现有代理，不允许接口临时创建代理。

## 方案比较

### 方案一：外部系统分别调用三个现有导入接口

不需要新增统一接口，但外部系统需要自行维护三次调用、账号组关联和代理绑定，任一步骤失败都可能产生不完整数据。

### 方案二：新增同步批量导入接口

由服务端统一解析身份、匹配代理，并按单条账号事务创建三种凭据及逻辑账号组。接口调用简单，能够保证每条数据要么完整落库、要么完全不落库。本次采用该方案。

### 方案三：新增异步导入任务

适合超大规模导入，但需要任务表、状态查询、重试和清理机制；当前单批最多 1000 条，无需增加这部分复杂度。

## 接口契约

- 路径：`POST /api/admin/v1/account-imports`
- 鉴权：复用现有管理员鉴权。调用方先通过 `/api/admin/v1/auth/login` 使用管理员账号密码获取短期 `accessToken`，再以 Bearer Token 调用导入接口。
- 请求体：

```json
{
  "accounts": [
    {
      "email": "user@example.com",
      "access_token": "<Build Access Token>",
      "refresh_token": "<Build Refresh Token>",
      "sso_token": "<Web SSO Token>",
      "proxy_url": "socks5://172.18.0.1:12084"
    }
  ]
}
```

- 成功响应由管理端统一的 `data` 字段包裹：

```json
{
  "data": {
    "total": 1,
    "succeeded": 1,
    "failed": 0,
    "results": [
      {
        "index": 0,
        "status": "created",
        "family_id": "99",
        "proxy": {
          "id": "2",
          "name": "clash-verge-123"
        },
        "accounts": {
          "grok_build": {"id": "301", "status": "created"},
          "grok_web": {"id": "302", "status": "created"},
          "grok_console": {"id": "303", "status": "created"}
        }
      }
    ]
  }
}
```

## 字段与默认值

- `email`：必填，用作逻辑账号组展示名称；不作为唯一身份依据。
- `access_token`：必填，用于 Grok Build 调用，并从 JWT 解析 `sub`、团队和到期时间。
- `refresh_token`：必填，用于 Grok Build 自动续期。
- `sso_token`：必填，用于 Grok Web 和 Grok Console。
- `proxy_url`：必填，规范化后与“IP 管理”中的代理完整地址精确匹配。
- 调度优先级、并发数、启用状态、OAuth 客户端和上游地址均使用系统默认配置，不由外部系统传入。

## 处理流程

1. 校验批次数量、字段大小、邮箱格式和凭据完整性。
2. 从 Build JWT 解析稳定身份 `sub`；请求中的 `email` 只作为名称和展示信息。
3. 规范化 `proxy_url`，解密比对“IP 管理”中的现有代理；匹配不到或匹配多个时拒绝该条数据。
4. 按 `sub` 查找或创建逻辑账号组，并将组内三种 Provider 成员的展示名更新为本次 `email`。
5. 在同一事务内创建或更新 Build、Web、Console 三条凭据，三者写入相同 `family_id`。
6. 将匹配到的代理绑定到逻辑账号组。
7. 返回每条数据的创建、更新或失败结果，不在响应或日志中回显凭据。

## 原子性与重复导入

- 单条账号是最小事务边界：三种凭据和代理绑定全部成功才提交。
- 批次允许部分成功，每条结果带原始数组下标和稳定错误码。
- 相同 `sub` 重复导入时更新原逻辑账号组及三种凭据，不重复创建。
- 相同邮箱但 `sub` 不同的账号不得合并，避免共享邮箱或邮箱复用导致串号。

## 错误规则

- `400`：JSON 非法、`accounts` 为空或批次数量超限等整个请求错误。
- `401 adminUnauthorized`：管理员访问令牌缺失或失效。
- `500 accountFamilyImportFailed`：代理索引加载等批次级基础设施错误。
- 批次被正常受理时 HTTP 状态为 `200`；单条校验或写入失败通过 `results[index].status = "failed"` 返回，其 `code` 可为 `invalid_credential`、`proxy_not_found`、`proxy_ambiguous`、`account_family_conflict` 或 `import_failed`。

## 决策日志

- 决定采用单一同步批量接口，而不是让外部系统编排三个现有接口；原因是逻辑账号组和代理绑定必须由服务端保证一致性。
- 决定复用管理员登录和 Bearer JWT，不增加独立导入密钥；原因是调用方明确要求直接使用管理员密码完成鉴权。
- 决定管理员密码只提交给现有登录接口，不放入导入请求；原因是避免在每次导入和业务日志中重复暴露长期凭据。
- 决定 `email` 作为逻辑账号名称但不作为唯一键；原因是 `sub` 更稳定，且邮箱可能变化或重复。
- 决定 `priority` 等调度字段不进入接口；原因是它们属于本系统运行策略，导入时使用默认值即可。
- 决定代理不存在时不自动创建；原因是代理资源必须先在“IP 管理”中受控维护。
- 决定代理匹配失败时整条账号不落库；原因是禁止账号意外使用共享出口。

## 验收标准

- 外部系统能够使用管理员登录返回的 `accessToken` 调用导入接口。
- 成功导入后，逻辑账号名称等于请求中的 `email`。
- Build、Web、Console 三条凭据属于同一个逻辑账号组。
- 逻辑账号组绑定的代理与 `proxy_url` 对应的“IP 管理”代理一致。
- 重复提交同一身份只更新数据，不增加逻辑账号组数量。
- 代理不存在、凭据无效或任一 Provider 写入失败时，该条数据不产生残留记录。
- API 响应和服务日志不包含 access token、refresh token、SSO token 或完整带认证代理地址。

## 实现验收记录

- [x] 导入路由注册在现有管理端 Bearer 鉴权分组下。
- [x] `email` 写入 Build、Web、Console 三种成员的展示名。
- [x] Build、Web、Console 在单个仓储事务内写入同一 `family_id`。
- [x] `proxy_url` 仅在规范化后匹配 IP 管理中的启用代理，并绑定到逻辑账号组。
- [x] 逻辑账号组仅由 Build `sub` 稳定身份决定，重复导入按 Provider 更新原成员。
- [x] 单条导入在一个事务内完成，匹配失败在进入仓储前返回。
- [x] 响应 DTO 仅返回 ID、状态和代理名，错误日志仅记录数组下标及内部错误。
- [ ] 未做本地或服务器端到端调用验证；调用方将自行部署后测试。

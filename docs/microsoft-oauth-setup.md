# 数据源 · Microsoft 授权接入(含大陆区 21Vianet）

Microsoft（Entra ID / Graph）数据源采用 **授权码 + PKCE 公共客户端**，不在服务端保存
`client_secret`。**大陆区（世纪互联 / 21Vianet）是与国际版物理隔离的主权云**：身份颁发端
（`login.partner.microsoftonline.cn`）与 Graph 资源（`microsoftgraph.chinacloudapi.cn`）都与
国际版不同，国际版的应用注册无法用于大陆区，反之亦然，因此客户端配置按区域各写一份。

## 1. 注册 Azure 应用

- 国际区：`portal.azure.com` → Microsoft Entra ID → 应用注册。
- 大陆区：`portal.azure.cn`（Azure 中国）→ Microsoft Entra ID → 应用注册。**必须在此注册，
  不能用国际区应用。**

对每个要接入的区域：

1. 新建应用注册（单租户或多租户按需）。
2. **平台**选「移动和桌面应用程序」(Mobile and desktop applications)，添加重定向 URI：
   `http://localhost:8080/api/sources/oauth/microsoft/callback`
   （端口/域名要与实际运行的 1agents 服务一致；反代下用对外可达的 https 地址。）
3. 「身份验证」页勾选 **允许公共客户端流** (Allow public client flows) = 是 —— 这样用 PKCE
   即可完成授权码换取，无需 secret。
4. 「API 权限」添加 **委托权限** Microsoft Graph：`User.Read`、`Contacts.Read`、`Mail.Read`、
   `offline_access`，并按租户策略「授予管理员同意」。

记下：**应用(客户端) ID**、**租户 ID 或域名**（大陆区通常为单租户，填租户 GUID 或
`xxx.partner.onmschina.cn`）、以及你注册的**重定向 URI**。

## 2. 写配置

编辑 `~/.1agents/sources/microsoft_oauth.json`（`ONEAGENTS_HOME` 生效时为
`$ONEAGENTS_HOME/.1agents/sources/microsoft_oauth.json`），按区域填入：

```json
{
  "cn": {
    "clientId": "<Azure 中国区应用的 client id>",
    "tenant": "<租户 GUID 或 xxx.partner.onmschina.cn>",
    "redirectUri": "http://localhost:8080/api/sources/oauth/microsoft/callback"
  },
  "intl": {
    "clientId": "<国际区应用的 client id>",
    "tenant": "common",
    "redirectUri": "http://localhost:8080/api/sources/oauth/microsoft/callback"
  }
}
```

- 只测大陆区时，只填 `cn` 块即可；`intl` 留空则国际区显示「未配置」。
- 公共客户端不需要 `clientSecret`；若你的应用被注册成 Web 类型且强制要 secret，可加
  `"clientSecret": "..."`（服务端会一并带上）。
- 文件权限建议 `600`。改完需重启后端使配置生效。

## 3. 连接与测试

1. 前端「数据源 → 添加数据源 → Microsoft」，**区域选「大陆（世纪互联 21Vianet）」**，填账号
   名后添加（此时只是登记占位账号）。
2. 进入该账号的「认证」页 →「连接 Microsoft」。会弹出 21Vianet 登录页，用**中国版
   Microsoft 365 组织账号**登录并同意授权。
3. 回调页显示「已连接」后弹窗自动关闭；「认证」页刷新为**已连接**并显示登录邮箱与令牌有效期。
4. 「采集配置」中 `联系人 / 邮件` 已是「已实现」；在数据源同步里对 `ms_contact` 触发一次立刻
   同步，到「数据」页应能看到拉取到的原始记录（bronze）。

## 端点对照（代码已按区域固定）

| 区域 | 授权/令牌端点 | Graph 资源 |
| --- | --- | --- |
| 国际 | `https://login.microsoftonline.com/{tenant}/oauth2/v2.0/...` | `https://graph.microsoft.com/v1.0` |
| 大陆 | `https://login.partner.microsoftonline.cn/{tenant}/oauth2/v2.0/...` | `https://microsoftgraph.chinacloudapi.cn/v1.0` |

Graph 权限 scope 会按区域加资源前缀（大陆用 `https://microsoftgraph.chinacloudapi.cn/...`），
否则主权云不会颁发令牌。

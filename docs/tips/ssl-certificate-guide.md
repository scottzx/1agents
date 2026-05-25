# Remote Agents SSL/TLS 证书配置指南

为了确保浏览器端的高级 Web 功能（如 Web Terminals 中的 Clipboard 剪贴板、Media 媒体设备、Service Workers 缓存等）能够顺利且安全地在局域网内运行，`remote-agents` 强制要求在安全上下文（HTTPS / WSS）下提供服务。

目前，`remote-agents` 完美支持以下 **两种** 灵活的 SSL/TLS 证书方案：

---

## 方案一：零配置自动生成自签名证书（极速启动 ⚡）

这是最简单的开箱即用方案。当你在启动命令中添加 `--ssl` 标志，且当前系统没有安装证书时，系统将**自动在后台利用 Go 标准库完成所有证书的生成和配置**。

### 1. 它是如何工作的？
- **自动创建与存储**：自动在用户家目录创建 `~/.remote-agents/certs/` 并生成现代、高强度的 **ECDSA P-256** 证书文件（`cert.pem` 和 `key.pem`），有效期为 **10 年**。
- **局域网多 IP 自动适配 (SAN)**：自动扫描本机的全部网络接口，将所有活跃的局域网 IP（如 `192.168.x.x`）写入证书的 Subject Alternative Names (SAN)。
- **Tailscale 自动注入**：如果系统运行了 Tailscale，系统会通过本地查询，将你的 Tailscale 节点域名（如 `*.ts.net`）和 Tailscale IP 也自动注入到自签名证书中。

### 2. 使用方法
只需在启动时增加 `--ssl` 标志：
```bash
remote-agents --ssl
```

### 3. 如何在浏览器中消除“不安全”警告？
由于证书是本地自签名的，浏览器会拦截并提示“不受信任的根证书”。你可以：
- **临时使用**：在浏览器拦截界面点击“高级” -> “继续前往（不安全）”。
- **永久信任**：双击生成的 `~/.remote-agents/certs/cert.pem` 导入到你本机的系统证书链（如 macOS 钥匙串访问、Windows 受信任根证书）并将其信任关系设置为“始终信任”，即可绿锁通过。

---

## 方案二：Tailscale 官方 Let's Encrypt 证书（完美信任 & 绿锁 🔒）

如果你组建了 Tailscale 虚拟局域网，你可以非常轻松地通过 Tailscale 官方获取由 **Let's Encrypt（全球权威 CA 机构）** 签发的高级 SSL 证书。
使用此方案，**全球任何浏览器、任何设备访问你的域名时，都会直接显示安全绿锁 🔒，不会有任何不安全警告**。

### 1. 在 Tailscale 控制台开启功能
1. 浏览器登录并进入 [Tailscale 控制台管理员设置 (Admin Console Settings)](https://login.tailscale.com/admin/settings/features)。
2. 找到 **“HTTPS Certificates”**，点击 **“Enable HTTPS Certificates...”** 开启此服务。

### 2. 在终端申请官方证书
回到你需要运行 `remote-agents` 的机器终端，运行以下命令（将域名替换为你自己的节点域名，可通过 `tailscale status` 获得）：
```bash
tailscale cert macbook-air-5.tailfb4720.ts.net
```
此时 Tailscale 会自动在当前目录下生成两个文件：
- `macbook-air-5.tailfb4720.ts.net.crt` (公共证书)
- `macbook-air-5.tailfb4720.ts.net.key` (私钥)

### 3. 部署并启动
我们将申请好的证书移动到 `remote-agents` 的持久化存储目录中，以便在任何位置启动都能被自动识别：
```bash
mkdir -p ~/.remote-agents/certs/
cp macbook-air-5.tailfb4720.ts.net.crt macbook-air-5.tailfb4720.ts.net.key ~/.remote-agents/certs/
```

现在直接运行：
```bash
remote-agents --ssl
```
系统启动时将输出如下日志，表明已成功**自动检测并加载**你的官方证书：
```
[main] Discovered official Tailscale certificate files. Using: /Users/scott/.remote-agents/certs/macbook-air-5.tailfb4720.ts.net.crt
[main] HTTPS / SSL enabled (using cert: /Users/scott/.remote-agents/certs/macbook-air-5.tailfb4720.ts.net.crt)
```

---

## 常见问题与排查 (Troubleshooting)

### 1. 为什么用浏览器访问时报 Service Worker (sw.js) 注册错误？
- **原因**：高级 Web 特性如 Service Worker 强制要求浏览器运行在“强安全上下文”中。即使在自签名证书下通过了“高级访问”，如果客户端系统没有彻底导入并信任该证书，浏览器依然会将其标记为 `Untrusted` 并拦截 Service Worker。
- **解决办法**：请按照**方案二**部署 Tailscale 官方证书，或者按照**方案一**中的“永久信任”方式，将自签名证书导入并信任在你的客户端操作系统中。

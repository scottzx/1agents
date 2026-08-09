# 领域所有权、跨域访问与依赖架构门禁

> **状态：C0 已实现（#326），随 `go test` / CI 强制执行。** 本文是 [v1.0.0 企业底座架构](./enterprise-foundation-v1.0.0.md) §3.4 边界规则、§6.3、§7 同库分域与 §13.3 发布闸门的开发侧落地规范。追溯：顶层需求 `#319`，交付任务 `#326`。

---

## 1. 一句话规则

**每个表和写 API 只有一个所有者；跨域只能走 Command / Query / Event 契约；应用只依赖内核 SDK 接口，不互相导入实现。** 违规由架构门禁测试在 CI 拦截，运行期越权写入被受控执行器拒绝并审计。

---

## 2. 命名空间（kernel_ / enterprise_ / presales_ / commerce_）

SQLite 没有 schema namespace，所有权用「表名前缀 + 所有权注册表 + 代码门禁」共同执行（总纲 §7.1）。

| 命名空间 | 所有者 | 表前缀 | 说明 |
|---|---|---|---|
| `kernel` | 运行时内核 | 新表用 `kernel_`；既有内核表（`projects`、`work_cases`、`command_executions`…）按**所有权清单**（ledger）纳管，不改名 | Workspace/Identity/WorkCase/Task/Session/Artifact/Agent/权限/审计 + Command/Query/Event 基础设施 |
| `enterprise` | 无（预留） | `enterprise_` | 只有通过晋升闸门（§6.1 双领域证据）的共享能力才可在此建表；C0 阶段注册表拒绝任何 `enterprise_` 建表 |
| `presales` | 售前与交付应用 | `presales_` | Opportunity/Evidence/SolutionVersion…（C1） |
| `commerce` | 电商运营应用 | `commerce_` | Product/SKU/Listing/PublishRecord…（C2） |

- 新增领域应用再增加一个命名空间：**应用 id 必须等于命名空间**（合法标识符 `[a-z0-9_]+`），该应用拥有 `<id>_` 前缀的全部表。
- `backend/internal/domainownership.RegisterKernelLedger` 持有清单；`TestKernelLedgerCoversAllDDL` 保证内核包新建的表要么进清单、要么带 `kernel_` 前缀，清单不会悄悄漂移。

## 3. 唯一所有者：表与写 API

实现：`backend/internal/domainownership`（L1 内核包，仅标准库）。

1. **表**：`Registry.RegisterTable(ns, table)` 强制 `ns_` 前缀且每表唯一所有者；重复登记同一所有者幂等，不同所有者返回 `duplicate_owner`。应用建表的唯一入口 `appregistry.EnsureDomainTables`（→ `domainstore.EnsureTables`）在 DDL 执行成功后自动调用 `RegisterAppTables` 登记所有权——所以**新表在写入第一行之前就有唯一所有者**。
2. **写 API（Command 契约）**：`Registry.RegisterWriteAPI(ns, contract)` 每契约唯一所有者；`commandbus.Registry` 本身也拒绝重复契约注册。内核拥有 `workcase.create/update/transition/delete/link/unlink/set_phase`（见清单）；领域应用的契约必须以 `<ns>.` 开头（如 `presales.opportunity.create`）。
3. **结构化错误**：违规返回 `*domainownership.Error{Code,…}`，码集：`unknown_namespace` / `namespace_owned` / `duplicate_owner` / `cross_domain_write` / `cross_domain_read` / `unowned_table` / `repository_access` / `permission_denied` / `invalid_declaration`。

## 4. 跨域访问的合法路径（§5 D3）

| 需求 | 合法路径 | 禁止 |
|---|---|---|
| 让别的域改状态 | 发 Command：`commandbus.Gateway.Dispatch`，所有者 Handler 在自己的事务里写自己的表 | 直接写对方表 / 绕过 Gateway 改状态 |
| 读别的域权威状态 | 发 Query：`domainref.Registry.Resolve(DomainRef)`，所有者 Provider 内做对象级鉴权 | 按 DomainRef 拼表查询、直读对方表 |
| 获知已发生事实 | 订阅 Outbox Event（`outbox.Dispatcher`），消费者幂等 | 把 Event 当写指令、投影冒充权威真相 |

端到端正例测试：`domainownership.TestLegitCrossDomainPathsPass`（Command → Guard 写本域表 → Outbox Event 同事务落库 → Query 解析 → 权限拒绝被审计）。

## 5. Repository 暴露边界

每个域的 Repository（持有该域 SQL 访问的对象）**只对本域可见**：

- **静态**：架构门禁规则 `foreign_repository_import` 禁止 apps/A 之外的任何包导入 apps/A 的 repository/store 子包；`app_imports_app` 禁止应用间实现互引。
- **运行期**：共享缝隙中传递的 repository 用 `Registry.CheckRepositoryAccess(caller, ns, name)` 校验，跨域访问返回 `repository_access` 并审计。
- 域内代码用 `domainownership.GuardDB/GuardTx`（受控执行器）访问 SQL：Guard 在执行前解析语句，写/读非本域注册的表即拒绝（`cross_domain_write` / `cross_domain_read`）。**新领域应用的 Handler 必须通过 Guard 写表**（见 `paths_test.go` 的 `presales.lead.create` 示例）。

## 6. 权限拒绝审计（§13.3）

所有拒绝都会留痕：

| 拒绝类型 | 审计位置 |
|---|---|
| Command 鉴权/校验/并发拒绝 | `command_executions`（status=`rejected`/`failed` + error_code） |
| Query 对象级权限拒绝 | `kernel_access_denials`（action=`query_permission`，经 `domainref.SetDenialHook` → `domainownership.WireQueryDenialAudit`） |
| Guard/注册表拦截的跨域读写 | `kernel_access_denials`（action=`table_write`/`table_read`） |
| Repository 越权访问 | `kernel_access_denials`（action=`repository_access`） |

`kernel_access_denials` 本身是新内核表，按规则带 `kernel_` 前缀。审计写入尽力而为，绝不阻塞主路径。

## 7. 架构门禁规则清单（`ScanBackend`）

扫描 `backend/`（跳过 `*_test.go` 与 `testdata/`），规则 id 稳定可引用：

| Rule | 含义 |
|---|---|
| `app_imports_app` | 应用导入另一应用的实现包 |
| `app_imports_non_sdk` | 应用导入 SDK 白名单之外的 internal 包 |
| `foreign_repository_import` | 域外包导入某应用的 repository/store 包（含内核导入应用实现） |
| `app_imported_by_kernel` | 非聚合器/非 cmd 的内核包导入应用根包（依赖方向反转） |
| `cross_domain_sql_write` | SQL 写入非本域表（应用写非本前缀表；内核写领域前缀表） |
| `cross_domain_sql_read` | 应用直读其他域前缀表（应走 Query） |
| `parse_error` | 无法解析的 Go 文件（门禁盲区，必须修复） |

**SDK 白名单**（应用唯一可依赖的内核接口）：`appkit`、`appregistry`、`taskapi`、`domainref`、`commandbus`、`domainownership`、`domainstore`、`templateregistry`。扩白名单是架构决策，需更新本文与 `scan.go`。

**聚合器例外**：只有 `internal/apps` 聚合包与 `cmd/` 可以 blank-import 应用根包（注册用）；应用的子包（尤其 repository）永远不被域外导入。

门禁自带「捕获能力」证据：`testdata/fixture` 内置违规样例，`TestScanFixtureCatchesViolations` 断言每类违规恰好被抓到、合法文件不被误报；`TestRealTreeIsClean` 断言真实代码树零违规。

## 8. 新增一个领域应用（C1/C2 操作清单）

1. 建包 `backend/internal/apps/<ns>/`（`<ns>` 即命名空间，合法标识符）；repository 放 `apps/<ns>/repository/`。
2. `init()` 里 `appregistry.Register(...)` 声明 Manifest；`provides`/`requires` 走能力契约（#325）。
3. 建表只走 `appregistry.EnsureDomainTables(<ns>, ddls)`，表名全部 `<ns>_` 前缀（所有权自动登记）。
4. 写路径：`commandbus` 注册 `<ns>.…` 契约，Handler 内用 `domainownership.GuardTx` 写表；产生事实时在 Result 里带 Event 信封，Gateway 在同事务追加 Outbox Event。
5. 读路径：实现 `domainref.QueryProvider`（含对象级鉴权）并 `RegisterProvider`。
6. 在 `internal/apps` 聚合包 blank-import 应用根包。
7. 跑 `make archgate` 直至全绿。

## 9. 运行门禁

```bash
make archgate                    # 推荐：门禁 + 契约相关包
cd backend && go test ./internal/domainownership/...   # 仅门禁本体
```

CI：`.github/workflows/backend-archgate.yml` 在 `backend/**` 变更时运行 `make archgate`（纯 `go test`，modernc sqlite 无 CGO，不需要额外工具链）。

## 10. 明确的 C0 边界（不做范围）

- 不做运行时 SQL 代理/全量 SQL 拦截：Guard 是新领域应用的受控入口，存量内核路径由清单 + 门禁测试治理。
- 不做动态插件、远程服务发现、完整 RBAC（总纲 §10 统一延后）。
- `*_test.go` 不受门禁约束（测试可搭跨域夹具）；生产代码受约束。

# AST 压测台 · API Stress Test

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](ast/)
[![Electron](https://img.shields.io/badge/Electron-44-47848F?logo=electron&logoColor=white)](desktop/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)](desktop/)
[![Platform](https://img.shields.io/badge/Windows-10%20%2F%2011%20x64-0078D6?logo=windows&logoColor=white)](#平台支持)
[![Version](https://img.shields.io/badge/version-1.3.1-blue)]()
[![SQLite](https://img.shields.io/badge/SQLite-modernc%20(no%20CGO)-003B57?logo=sqlite&logoColor=white)](ast/analysis/)

一个**本地运行的 API 接口压力测试工具**：Go 写的并发压测引擎 + Electron 桌面端。

它不只是「发 N 个请求」，还会把每一次返回**按业务语义归成一类**：HTTP 状态码 + 响应体里的业务码 + 任务 ID 组成指纹，
同类只保留一份完整样本、其余只累加次数——所以 1000 次请求的结果通常只有几十 KB，
而不是一份几十万行的日志。

<!-- 界面截图（可选）：把截图放到 docs/screenshots/ 后取消注释
![运行监控](docs/screenshots/dashboard.png)
![返回类型分析](docs/screenshots/analysis.png)
-->

---

## 功能特性

| 能力 | 说明 |
|---|---|
| **异步压测任务** | `POST /getData` 立即返回 `202 + jobId`，后台并发执行，前端每秒轮询进度（QPS、成功/失败、状态码分布） |
| **可控并发** | 信号量限流 + `context` 取消 + 连接复用；单任务可设超时、并发数、总请求数 |
| **返回类型聚合** | 按 `jobId + HTTP 状态码 + 业务码` 去重计数，每类只留一份样本，写入量是 O(类型数) 而不是 O(请求数) |
| **SQLite 分析库** | 每个任务一个 `result.db`（纯 Go 驱动 `modernc.org/sqlite`，无需 CGO / gcc），存计数、首次/末次出现时间、样本索引 |
| **延迟分位** | `min / p50 / p95 / p99 / max`（nearest-rank），外加平均 QPS 与总耗时 |
| **参数存档** | 每次压测的完整参数写到 `config_test/<jobID>.json`，历史详情页可查看并**一键重新测试** |
| **多种请求体** | `json` / `form` / `multipart`（支持上传文件）/ `text` / `raw` |
| **认证** | `Bearer` / `Basic` / `X-API-Key`；结果文件里的凭证自动打码（只留前 4 位） |
| **历史记录** | 扫描 `Data/*/summary.json`，进程重启后依然可查 |
| **任务管理** | 取消运行中任务；删除任务记录（可选连同本地数据一起删） |

---

## 平台支持

| 平台 | 状态 | 说明 |
|---|---|---|
| **Windows 10 / 11 x64** | ✅ 提供安装包 | Electron 桌面端 + 内置后端进程，开箱即用 |
| macOS / Linux | ⚠️ 源码可用，未提供打包 | 后端可 `go run .` 独立运行；前端 `npm run dev` 用浏览器访问，或用 `electron-builder` 自行打包 |

> 后端本身跨平台（Go），桌面端打包目前只做了 Windows。

---

## 安装与运行（Windows）

1. 到 [Releases](https://github.com/uxiaoheACG/ApiStressTest/releases) 下载 `AST压测台 Setup <版本>.exe`（若暂无 Release，见下方 [从源码构建](#从源码构建)）；
2. 双击安装（可选择安装目录）；
3. 启动后会看到「运行监控」页，顶部副标题显示**后端已连接**即就绪。

应用启动时 Electron 主进程会自动拉起随包分发的后端 `ast.exe`：

- 若本机 `127.0.0.1:3010` 已有服务（比如你自己起的后端），**直接复用**，不会重复启动；
- 结果数据默认写在**安装目录同级的 `ast-data/`**（卸载 / 覆盖安装不会丢），
  该位置不可写时自动回退到 `%APPDATA%\ast-stress-desktop`；
- 想换位置：设置环境变量 `AST_DATA_DIR=<目录>`。

---

## 快速上手

1. **新建压测**：填请求地址、选方法，按需填 Header / URL 参数 / 请求体 / 认证；
2. **设并发**：单请求超时（ms）、最大并发、总请求数；
3. **点「开始压测」**：跳到任务详情，实时进度每秒刷新；
4. **看结果**：任务结束后看「汇总结果」（QPS、分位耗时）、「返回类型分析」（每类返回的次数与占比），
   点任意一类的「查看」能看到该类的**完整请求/响应快照**；
5. 想换参数再跑一次 → 详情页点 **↻ 重新测试**，参数会自动填回新建页。

---

## 界面与功能

### 运行监控
轮询 `/jobs`，展示进行中任务的进度条、实时 QPS、成功/失败拆分、状态码分布，可一键取消。
下方是最近结束的任务列表。

### 新建压测
- **请求目标**：URL、方法、URL 查询参数
- **请求体**：`json`（实时显示每个字段的类型，`"abc"` 是字符串、`18` 是数字）、
  `form`（urlencoded，值一律字符串）、`multipart`（可选本地文件，读成 base64 提交）、`text` / `raw`
  > 只有**当前选中的页签**会被发送，其它页签里填的内容不会发送（界面有标注）
- **请求头 / 认证**：键值对编辑；`Bearer` / `Basic` / `X-API-Key`
- **并发与结果**：
  - 单请求超时、最大并发、总请求数
  - **业务码字段（可选）**：留空时自动探测 `code / errcode / errno / status` 等，
    也可写 `error.code` 这种点号路径
  - 每秒请求数（`rate`）目前**尚未实现**，填了不生效

### 任务详情
- **实时进度**：进度条、QPS 趋势、状态码分布
- **汇总结果**：总请求、成功率、平均 QPS、总耗时、`min/p50/p95/p99/max`
- **当次请求参数**：该次压测的 URL、并发、业务码字段、Header、Params、请求体与文件，以及 **↻ 重新测试**
- **返回类型分析**：状态码 / 业务码 / 分类 / 错误归类 / 次数 / 占比 / 首次出现时间，点「查看」打开样本
- **样本快照**：`samples/` 目录里每类返回的样本卡片
- **删除**：右上角「删除」，勾选「同时删除本地存储数据」会连 `Data` 目录与 `config_test` 存档一起删除

### 历史记录
扫描 `Data/*/summary.json`，可按任务 ID / 目录筛选；缺少 `summary.json` 的任务会被标记。

### 设置
后端地址（持久化在本地）、连接状态、内置后端启停、**结果数据目录的真实路径与「打开目录」**。

---

## 结果存储与「返回类型」聚合

每个任务一个目录：

```
Data/2026-10-19_15-34-50_e6bdd719/
├── result.db              # SQLite：每类返回的计数、首末时间、样本索引
├── summary.json           # 汇总：QPS、分位耗时、状态码分布、类型数
└── samples/<hash>.json    # 每种返回类型的第一份完整请求/响应快照（响应体截断 4 KiB）
```

指纹规则：

```
指纹 = jobId | HTTP 状态码 | 业务码
      ├ 业务码：从响应体前 4 KiB 的 JSON 里取（默认探测 code/errcode/errno/status…，可用 bizCodeField 指定）
      └ 没有响应时（超时 / DNS / 连接拒绝）：状态码记 0，业务码位置记 err:<errKind>
```

约束与效果：

- 所有类型的次数之和 = **实际完成的请求数**（`done`）；取消任务时未执行的请求不计入
- 业务码超过 64 字符记为 `long`；单任务类型上限 5000，超出归入 `overflow`（防止 traceId 撑爆）
- 容量对比：200 次请求 → `result.db` 24 KB + 3 份样本 ≈ 26 KB；
  同样量级旧方案（每请求一行 NDJSON）在 1000 次时可达数百 KB

---

## 从源码构建

**前置**：Go 1.26+、Node.js 20+（Windows 上打包还需要能访问 npm / electron 镜像）。

```powershell
# 1) 构建后端（产物会被打进安装包）
cd ast
$env:CGO_ENABLED = '0'
go build -trimpath -ldflags "-s -w" -o ../desktop/resources/backend/ast.exe .

# 2) 装依赖并开发调试（Vite 热更新 + Electron 窗口，自动拉起后端）
cd ../desktop
npm install
npm run dev

# 3) 打包 Windows 安装包 → desktop/release/AST压测台 Setup <版本>.exe
npm run dist
```

其它脚本：`npm run build`（只构建前端）、`npm run pack`（免安装目录版）、`npm run typecheck`。

**只跑后端**（不用桌面端）：

```powershell
cd ast
go run .
# 监听 :3010，浏览器直接访问 http://127.0.0.1:3010/ 可见 {"status":"OK"}
```

---

## 后端接口

基础地址 `http://127.0.0.1:3010`（改端口：`serverConfig.yaml` 的 `addr` 或环境变量 `AST_PORT`）。
跨域只放行本机来源：`Origin` 为空 / `null`（Electron 的 file:// 页面）/ `localhost` / `127.0.0.1` / `::1`。

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/` | 健康检查 |
| `POST` | `/getData` | 启动压测，返回 `202 {jobId, status, total, dir}` |
| `GET` | `/jobs` | 内存中的任务列表（新→旧，最多 50 条，重启清空） |
| `GET` | `/jobs/:id` | 单个任务进度；结束后带 `summary` |
| `DELETE` | `/jobs/:id` | **取消**运行中的任务（202 = 信号已发出） |
| `GET` | `/jobs/:id/analysis` | 返回类型分析（读任务目录下的 `result.db`） |
| `GET` | `/jobs/:id/samples/:file` | 某类返回的样本原文 |
| `GET` | `/jobs/:id/config` | 当次压测的完整参数（`config_test/<jobID>.json`） |
| `DELETE` | `/jobs/:id/record?purge=` | **删除**任务记录；`purge=true` 时连 `Data` 目录与参数存档一起删 |
| `GET` | `/jobs/:id/records?kind=&offset=&limit=` | 旧版本任务的 jsonl 明细分页（新任务不再产生） |
| `GET` | `/history?limit=` | 磁盘上的历史汇总（重启后仍可查） |

`POST /getData` 请求体示例：

```json
{
  "url": "https://api.example.com/users",
  "method": "POST",
  "body": { "type": "json", "data": { "name": "tester", "age": 18 } },
  "header": { "Accept": "application/json" },
  "params": { "source": "ast" },
  "authorization": { "type": "bearer", "token": "<token>" },
  "concurrency": { "timeout": 3000, "count": 50, "totalRequests": 1000, "rate": 0 },
  "bizCodeField": "code"
}
```

`multipart` 的 `body.data` 结构（文件内容 base64，单文件前端限 10 MB、后端限 16 MB）：

```json
{
  "type": "multipart",
  "data": {
    "fields": { "name": "tester" },
    "textParts": [{ "field": "metadata", "value": "{\"scene\":\"test\"}", "contentType": "application/json" }],
    "files": [{ "field": "file", "filename": "a.txt", "contentType": "text/plain", "contentBase64": "..." }]
  }
}
```

---

## 配置

`ast/serverConfig.yaml`（环境变量优先）：

| 配置 | 环境变量 | 默认 |
|---|---|---|
| 监听地址 | `AST_PORT` | `:3010` |
| gin 模式 | `GIN_MODE` | `debug` |
| 优雅关闭等待 | `AST_SHUTDOWN_TIMEOUT` | `10s` |

桌面端另支持：`AST_DATA_DIR`（数据目录）、`AST_BACKEND_URL`（内置后端地址）。

---

## 已知限制

- `concurrency.rate`（每秒限速）**未实现**，界面标注为「不生效」；
- 同时最多 **4 个**压测任务，超出返回 429；内存只保留最近 50 条任务；
- 业务码只从响应体**前 4 KiB** 提取，响应体不是 JSON 时退化为「只按状态码」归类；
- 取消是协作式的：信号发出后要等在飞请求返回，状态才变成 `canceled`；
- 1.3.0 之前的旧任务没有 `result.db`，只能看 `summary.json` 与旧的 jsonl 明细。

---

## 目录结构

```
.
├── ast/                       # Go 后端（Gin）
│   ├── controller/            # 路由、handler、请求构造与压测执行
│   │   └── SendAndWrite/      #   并发派发后的结果聚合、样本落盘、SQLite 写入
│   ├── job/                   # 任务状态机、注册表、结果目录、参数存档
│   ├── analysis/              # SQLite 分析库（schema / 写入 / 查询）
│   ├── model/                 # TestConfig 等数据结构
│   ├── service/               # 服务器启动、配置、CORS、优雅关闭
│   └── serverConfig.yaml
├── desktop/                   # Electron + Vue 3 + TypeScript 桌面端
│   ├── electron/              # 主进程（窗口、请求代理、内置后端进程管理）
│   └── src/                   # 页面、组件、API 封装
└── README.md
```

更多桌面端细节（构建、请求链路、页面功能）见 [`desktop/README.md`](desktop/README.md)。

---

## 许可

本仓库目前未附带 `LICENSE` 文件。如需二次分发或商用，请先与作者联系确认授权方式。

# AST 压测台 · 桌面端

`ast`（Go + Gin 压测后端）的桌面图形界面：Electron 外壳 + Vue 3 + TypeScript + Vite。

```
desktop/
├── electron/
│   ├── main.cjs         Electron 主进程：窗口、请求代理、内置后端进程管理
│   └── preload.cjs      仅暴露 ast.request / ast.runtime / ast.backend 三个入口
├── src/
│   ├── api/             后端接口封装与类型（对齐 model.TestConfig、job.Snapshot）
│   ├── components/      任务卡片、状态码分布、耗时分布、明细表、详情抽屉等
│   ├── composables/     GET /jobs 的 1 秒轮询（模块级单例，多页面共用）
│   ├── views/           运行监控 / 新建压测 / 任务详情 / 历史记录 / 设置
│   ├── utils/format.ts  时间、耗时、字节、状态码等格式化
│   ├── App.vue          侧边导航 + 后端在线状态
│   └── router.ts        hash 路由（打包后 file:// 加载所需）
└── resources/backend/   随包分发的 ast.exe（打包时进 resources/backend）
```

## 快速开始

```powershell
cd desktop
npm install                 # 已装过可跳过

# 1) 开发模式（Vite 热更新 + Electron 窗口）
npm run dev

# 2) 构建前端 + 用构建产物启动窗口
npm start

# 3) 打包成 Windows 免安装目录：release/win-unpacked/AST压测台.exe
npm run pack

# 4) 打包成安装包（NSIS）
npm run dist
```

类型检查：`npm run typecheck`（vue-tsc）。

### 重新构建随包后端

改了 Go 代码后需要重新生成 `resources/backend/ast.exe`：

```powershell
cd ast
go build -trimpath -ldflags "-s -w" -o ../desktop/resources/backend/ast.exe .
```

## 后端接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/` | 健康检查，返回 `{"status":"OK"}` |
| POST | `/getData` | 启动压测，返回 `202 + jobId + dir` |
| GET | `/jobs` | 内存中的任务列表（新→旧，最多 50 条，重启清空） |
| GET | `/jobs/:id` | 单个任务进度，结束后带 `summary` |
| GET | `/history?limit=` | 磁盘上的历史汇总（重启后仍可查） |
| **DELETE** | `/jobs/:id` | **新增**：取消正在执行的任务（202 = 信号已发出） |
| **GET** | `/jobs/:id/records?kind=&offset=&limit=` | **新增**：分页读取 `success.jsonl` / `failures.jsonl` |
| **GET** | `/jobs/:id/config` | **新增**：当次压测的完整参数（`config_test/<jobID>.json`） |

`/jobs/:id/records` 的 `kind` 取 `success` / `failures`（默认），`limit` 上限 500、默认 100；
数据直接来自磁盘，所以历史任务（进程重启后）也能看明细。

**端口**：默认监听 `3010`（旧版是 8080），可用 `serverConfig.yaml` 的 `addr`
或环境变量 `AST_PORT` 覆盖。

**配置存档**：每次压测成功创建任务后会写 `config_test/<jobID>.json`，内容就是完整的
`TestConfig`（含 auth token 与上传文件内容），文件权限 0600。它是「历史详情展示参数」和
「重新测试」的数据来源，进程重启后仍可读。

**请求体类型**：`json` / `form` / `text` / `raw` / `multipart`。
`multipart` 的文件以 base64 随配置传入（`{"fields":{...},"files":[{field,filename,contentType,contentBase64}]}`），
后端解码后拼成 `multipart/form-data`，单文件上限 16 MB、整体 32 MB，前端限制单文件 10 MB。

后端另外新增了 CORS 中间件（`service/cors.go`），只放行本机来源：`Origin` 为空、
`null`（Electron 的 file:// 页面）、`localhost` / `127.0.0.1` / `::1` 的任意端口。

## 图标

应用图标是 `resources/icon.png`（763×763 PNG），electron-builder 打包时自动转成多尺寸 `.ico`
嵌入 exe（`build.win.icon`）；开发模式下同一个文件用作窗口图标。

## 请求链路

```
Vue 页面
  └─ src/api/client.ts
       ├─ window.ast.request()   → preload → 主进程 fetch（默认，绕开 file:// 的跨域限制）
       └─ window.fetch()         → 浏览器环境 / 直接用浏览器打开时，走后端 CORS
```

桌面应用启动时会自动尝试 `resources/backend/ast.exe`（开发模式下回退到 `ast/ast.exe`）：
- 3010 上已有服务 → 直接复用，不重复启动；
- 没有服务且找到 exe → 拉起后端，结果写到 `<工作目录>/Data`，参数存档写到 `<工作目录>/config_test`：
  - 开发模式：仓库 `ast/`（与手动 `go run` 一致）
  - 打包版：**安装目录同级的 `ast-data/`**（例如 `E:\test\ast-data\Data`）——
    NSIS 卸载/覆盖安装只清理安装目录本身，数据不会跟着丢；
    该位置不可写时自动回退 `%APPDATA%/ast-stress-desktop`
  - 想固定到别处：设置环境变量 `AST_DATA_DIR=<目录>`，Data 会写在该目录下
  - 设置页会显示真实路径，并提供「打开目录」按钮
- 找不到 exe → 界面提示，可在设置页填写外部后端地址。

## 页面功能

- **运行监控**：轮询 `/jobs`，实时进度条、QPS、成功/失败拆分、状态码分布，一键取消。
- **新建压测**：URL / 方法 / 请求体（json、form、multipart、text、raw）/ Header / URL 参数 /
  认证 / 并发（timeout、count、totalRequests、rate）/ 落盘模式。
  - JSON 编辑器实时显示解析结果与每个顶层字段的类型（`"abc"`→string、`18`→number、`true`→boolean）
  - multipart 可选本地文件，读成 base64 随配置提交，单文件 ≤ 10 MB
  - 支持从历史任务带参数跳转（`#/new?from=<jobID>`）自动填表
- **任务详情**：实时进度与 QPS 趋势、`summary.json` 汇总、耗时分位（P50/P95/P99）、
  状态码分布、明细分页表格（每页 100 条，点击行看完整请求/响应，凭证已由后端打码）；
  **「当次请求参数」卡片**展示该次压测的 URL、方法、并发、Header、URL 参数、请求体与文件，
  并提供 `↻ 重新测试` 一键带着这些参数跳到新建压测页。
- **历史记录**：扫描 `Data/*/summary.json`，可按任务 ID / 目录筛选，缺少 summary 的任务会标记。
- **设置**：后端地址（持久化在 localStorage）、连接测试、内置后端启停、结果目录与打开按钮。

## 已知限制（来自后端）

- `concurrency.rate`（每秒限速）后端尚未实现，界面标注为「不生效」。
- 同时最多 4 个压测任务，超限返回 429；`/jobs` 只保留最近 50 条。
- `saveData=0` 不产生明细文件，`saveData=2` 的成功记录只有简写（无请求/响应体）。
- 取消是协作式的：信号发出后任务要等在飞请求返回才变 `canceled`。
- 结果目录里的 `success.jsonl` / `failures.jsonl` 是 NDJSON，不能整体 `JSON.parse`。

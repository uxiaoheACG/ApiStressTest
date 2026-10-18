const { app, BrowserWindow, ipcMain, shell } = require('electron')
const path = require('node:path')
const fs = require('node:fs')
const { spawn } = require('node:child_process')

const DEV_URL = process.env.AST_DEV_URL || 'http://127.0.0.1:5173'
const BACKEND_URL = process.env.AST_BACKEND_URL || 'http://127.0.0.1:3010'
const isDev = process.argv.includes('--dev') || process.env.AST_DEV === '1'

/** @type {{ proc: import('node:child_process').ChildProcess, url: string } | null} */
let backend = null

/** 后端的工作目录（结果写在 <workDir>/Data），启动后端时确定 */
let workDir = null

/* ------------------------------------------------------------------ */
/* 后端进程管理：优先拉起随包分发的 ast.exe，找不到就让用户连外部后端   */
/* ------------------------------------------------------------------ */

function backendCandidates() {
  const list = []
  if (app.isPackaged) {
    list.push(path.join(process.resourcesPath, 'backend', 'ast.exe'))
  }
  list.push(path.join(__dirname, '..', 'resources', 'backend', 'ast.exe'))
  list.push(path.join(__dirname, '..', '..', 'ast', 'ast.exe'))
  return list
}

function findBackendBinary() {
  for (const candidate of backendCandidates()) {
    if (fs.existsSync(candidate)) return candidate
  }
  return null
}

async function pingBackend(url, timeoutMs = 1200) {
  try {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    const res = await fetch(`${url}/`, { signal: controller.signal })
    clearTimeout(timer)
    return res.ok
  } catch {
    return false
  }
}

function wait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** 开发模式下的后端工作目录：优先仓库里的 ast/（与手动 go run 一致） */
function devWorkingDir(exe) {
  const repoAst = path.join(__dirname, '..', '..', 'ast')
  if (fs.existsSync(path.join(repoAst, 'go.mod'))) return repoAst
  return exe ? path.dirname(exe) : app.getPath('userData')
}

/** 目录是否真的可写（用临时文件探测，比 access 判断可靠） */
function isWritable(dir) {
  try {
    fs.mkdirSync(dir, { recursive: true })
    const probe = path.join(dir, `.ast-write-test-${process.pid}`)
    fs.writeFileSync(probe, 'ok')
    fs.unlinkSync(probe)
    return true
  } catch {
    return false
  }
}

/** 打包版默认数据工作目录：与安装目录平级（卸载/覆盖安装不会动它） */
function defaultWorkDir() {
  if (!app.isPackaged) return devWorkingDir(null)
  const exeDir = path.dirname(process.execPath)
  return path.join(path.dirname(exeDir), 'ast-data')
}

/**
 * 后端的工作目录，结果会写到 <工作目录>/Data 下。
 *
 * 优先级：
 *   1. 环境变量 AST_DATA_DIR（自定义结果目录）
 *   2. 打包版：安装目录的**兄弟目录** `<安装目录父级>/ast-data`
 *      —— NSIS 卸载/覆盖安装只清理安装目录本身，数据不会跟着丢；
 *      该位置不可写时（例如装在 Program Files）回退用户数据目录
 *   3. 开发版：仓库里的 ast/
 */
function resolveWorkDir(exe) {
  if (workDir) return workDir

  const custom = process.env.AST_DATA_DIR
  if (custom) {
    workDir = path.resolve(custom)
    fs.mkdirSync(workDir, { recursive: true })
    return workDir
  }

  if (app.isPackaged) {
    const preferred = path.join(path.dirname(path.dirname(process.execPath)), 'ast-data')
    if (isWritable(preferred)) {
      workDir = preferred
      return workDir
    }
    const exeDir = path.dirname(process.execPath)
    workDir = isWritable(exeDir) ? exeDir : app.getPath('userData')
    return workDir
  }

  workDir = devWorkingDir(exe)
  return workDir
}

/** 结果目录（供界面显示与“打开目录”） */
function dataDir() {
  return path.join(workDir || defaultWorkDir(), 'Data')
}

async function startBackend() {
  if (backend && backend.proc.exitCode === null) {
    return { running: true, pid: backend.proc.pid, message: '后端已在运行' }
  }

  // 端口上已经有服务（例如用户自己起的）就直接复用
  if (await pingBackend(BACKEND_URL)) {
    return { running: true, message: '已连接现有后端' }
  }

  const exe = findBackendBinary()
  if (!exe) {
    return {
      running: false,
      message: '没有找到 ast.exe，请手动启动后端，或在设置页填写已有的后端地址',
    }
  }

  // 结果目录默认放在与安装目录平级的 ast-data/Data 下（卸载/重装不受影响），
  // 该位置不可写时回退用户数据目录
  const cwd = resolveWorkDir(exe)
  fs.mkdirSync(cwd, { recursive: true })

  const proc = spawn(exe, [], {
    cwd,
    windowsHide: true,
    // 打包后默认按 release 模式跑（日志更干净、无调试开销）；开发模式沿用 serverConfig.yaml
    env: {
      ...process.env,
      ...(app.isPackaged && !process.env.GIN_MODE ? { GIN_MODE: 'release' } : {}),
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  proc.stdout?.on('data', (chunk) => process.stdout.write(`[backend] ${chunk}`))
  proc.stderr?.on('data', (chunk) => process.stderr.write(`[backend] ${chunk}`))
  proc.on('exit', (code) => {
    console.log(`[backend] 进程退出，code=${code}`)
    backend = null
  })

  backend = { proc, url: BACKEND_URL }

  for (let i = 0; i < 40; i += 1) {
    if (await pingBackend(BACKEND_URL, 500)) {
      return { running: true, pid: proc.pid, message: `后端已启动（${exe}）` }
    }
    await wait(250)
  }

  return { running: false, pid: proc.pid, message: '后端启动超时，请检查端口 8080 是否被占用' }
}

function stopBackend() {
  if (!backend || backend.proc.exitCode !== null) {
    backend = null
    return { running: false, message: '后端未在运行' }
  }
  const { proc } = backend
  backend = null
  try {
    proc.kill()
  } catch (err) {
    return { running: false, message: `停止后端失败: ${err.message}` }
  }
  return { running: false, message: '后端已停止' }
}

/* ------------------------------------------------------------------ */
/* 窗口                                                                */
/* ------------------------------------------------------------------ */

function createWindow() {
  // 开发模式用仓库里的 PNG 作窗口图标；打包后 exe 已嵌入图标，走系统默认即可
  const iconPath = path.join(__dirname, '..', 'resources', 'icon.png')

  const win = new BrowserWindow({
    width: 1320,
    height: 860,
    minWidth: 1000,
    minHeight: 640,
    backgroundColor: '#0a1120',
    title: 'AST 压测台',
    autoHideMenuBar: true,
    ...(fs.existsSync(iconPath) ? { icon: iconPath } : {}),
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  })

  if (isDev) {
    win.loadURL(DEV_URL)
    if (process.env.AST_DEVTOOLS === '1') {
      win.webContents.openDevTools({ mode: 'detach' })
    }
  } else {
    win.loadFile(path.join(__dirname, '..', 'dist', 'index.html'))
  }

  // 页面里的外链交给系统浏览器，不在应用内开新窗口
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:/i.test(url)) shell.openExternal(url)
    return { action: 'deny' }
  })

  // 渲染进程的错误打到主进程日志，方便排查白屏
  win.webContents.on('console-message', (...args) => {
    const details = args[1]
    if (details && typeof details === 'object') {
      if (details.level === 'error' || details.level === 3) {
        console.log(`[renderer:error] ${details.message}`)
      }
    }
  })
  win.webContents.on('did-fail-load', (_e, code, desc) => {
    console.log(`[renderer] 页面加载失败 ${code}: ${desc}`)
  })

  return win
}

/* ------------------------------------------------------------------ */
/* IPC                                                                 */
/* ------------------------------------------------------------------ */

// 请求代理：由主进程发请求，彻底绕开 file:// 页面的跨域与私有网络限制
ipcMain.handle('ast:request', async (_event, payload) => {
  const { method = 'GET', url, body } = payload || {}
  if (!/^https?:\/\//i.test(String(url))) {
    return { ok: false, status: 0, body: JSON.stringify({ error: `非法地址: ${url}` }) }
  }

  const headers = {}
  const init = { method, headers }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }

  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 30_000)
  init.signal = controller.signal

  try {
    const res = await fetch(url, init)
    const text = await res.text()
    return { ok: res.ok, status: res.status, body: text }
  } catch (err) {
    return {
      ok: false,
      status: 0,
      body: JSON.stringify({ error: `主进程请求失败: ${err.message}` }),
    }
  } finally {
    clearTimeout(timer)
  }
})

ipcMain.handle('ast:runtime', () => ({
  electron: process.versions.electron,
  platform: `${process.platform} ${process.arch}`,
  backendUrl: BACKEND_URL,
  dataDir: dataDir(),
  dataDirKnown: workDir !== null,
}))

// 打开结果目录（先在资源管理器里暴露 Data 目录，方便找结果）
ipcMain.handle('ast:open-data-dir', async () => {
  const dir = dataDir()
  try {
    fs.mkdirSync(dir, { recursive: true })
  } catch (err) {
    return { ok: false, message: `创建目录失败: ${err.message}` }
  }
  const err = await shell.openPath(dir)
  return err ? { ok: false, message: err } : { ok: true, message: `已打开 ${dir}` }
})

ipcMain.handle('ast:backend-start', () => startBackend())
ipcMain.handle('ast:backend-stop', () => stopBackend())
ipcMain.handle('ast:backend-status', async () => ({
  running: (backend && backend.proc.exitCode === null) || (await pingBackend(BACKEND_URL)),
  pid: backend?.proc.exitCode === null ? backend.proc.pid : undefined,
  exists: Boolean(findBackendBinary()),
}))

/* ------------------------------------------------------------------ */
/* 生命周期                                                            */
/* ------------------------------------------------------------------ */

// 单实例：重复启动时聚焦已有窗口
if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  app.on('second-instance', () => {
    const [win] = BrowserWindow.getAllWindows()
    if (win) {
      if (win.isMinimized()) win.restore()
      win.focus()
    }
  })

  app.whenReady().then(async () => {
    createWindow()
    await startBackend()

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) createWindow()
    })
  })
}

app.on('window-all-closed', () => {
  stopBackend()
  if (process.platform !== 'darwin') app.quit()
})

app.on('before-quit', () => {
  stopBackend()
})

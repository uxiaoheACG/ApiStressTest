const { contextBridge, ipcRenderer } = require('electron')

/**
 * 渲染进程与主进程之间的最小桥：
 *
 * - request：代理 HTTP 请求（Node 侧发，不受页面来源限制）
 * - runtime：Electron / 平台信息
 * - backend：内置 Go 后端的启停与状态
 *
 * 只暴露这几个方法，不把 ipcRenderer 整体交给页面。
 */
contextBridge.exposeInMainWorld('ast', {
  request: (payload) => ipcRenderer.invoke('ast:request', payload),
  runtime: () => ipcRenderer.invoke('ast:runtime'),
  openDataDir: () => ipcRenderer.invoke('ast:open-data-dir'),
  backend: {
    start: () => ipcRenderer.invoke('ast:backend-start'),
    stop: () => ipcRenderer.invoke('ast:backend-stop'),
    status: () => ipcRenderer.invoke('ast:backend-status'),
  },
})

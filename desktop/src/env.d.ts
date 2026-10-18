/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}

/** preload 暴露的主进程桥（非 Electron 环境为 undefined） */
interface AstBridge {
  request(payload: {
    method: string
    url: string
    body?: unknown
  }): Promise<{ ok: boolean; status: number; body: string }>
  runtime(): Promise<{
    electron: string
    platform: string
    backendUrl: string
    dataDir: string
    dataDirKnown: boolean
  }>
  openDataDir(): Promise<{ ok: boolean; message?: string }>
  backend: {
    start(): Promise<{ running: boolean; pid?: number; message: string }>
    stop(): Promise<{ running: boolean; message: string }>
    status(): Promise<{ running: boolean; pid?: number; exists: boolean }>
  }
}

interface Window {
  ast?: AstBridge
}

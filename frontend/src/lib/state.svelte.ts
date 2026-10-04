// 全局前端状态：Svelte 5 runes，不引入额外状态库。
import { Events } from "@wailsio/runtime";
import {
  EV,
  TunnelService,
  HostService,
  type Host,
  type StatusEvent,
  type StatsEvent,
  type Tunnel,
} from "./api";
import type { Locale } from "./i18n";
import { loadTheme, applyTheme, type ThemeMode } from "./theme";

export type Page = "dashboard" | "hosts" | "tunnels" | "logs" | "settings";
export type LogEntry = { id: number; time: string; message: string; tunnelId?: string };

let logIdSeq = 0;
const statusEventRevisions = new Map<string, number>();

export const app = $state({
  page: "dashboard" as Page,
  language: loadLanguage(),
  theme: loadTheme(),
  tunnels: [] as Tunnel[],
  hosts: [] as Host[],
  loading: false,
  tunnelBusy: {} as Record<string, boolean>,
  tunnelOperationErrors: {} as Record<string, string>,

  // 每隧道 sparkline 历史（最近 60 个采样点的 tx/rx 增量）。
  txHistory: {} as Record<string, number[]>,
  rxHistory: {} as Record<string, number[]>,

  // 侧栏底部总速率（字节/秒）。
  totalTx: 0,
  totalRx: 0,

  logs: [] as LogEntry[],

  modalOpen: false,
});

function loadLanguage(): Locale {
  const storage = getStorage();
  return storage?.getItem("sshnat.language") === "en" ? "en" : "zh";
}

export function setLanguage(language: Locale) {
  app.language = language;
  if (typeof document !== "undefined") document.documentElement.lang = language === "en" ? "en" : "zh-CN";
  getStorage()?.setItem("sshnat.language", language);
}

/** Browsers provide Storage, while SSR and some test runners expose a partial global. */
function getStorage(): Storage | undefined {
  const candidate = (globalThis as { localStorage?: unknown }).localStorage;
  if (
    candidate &&
    typeof (candidate as Storage).getItem === "function" &&
    typeof (candidate as Storage).setItem === "function"
  ) {
    return candidate as Storage;
  }
  return undefined;
}

export function setTheme(mode: ThemeMode) {
  app.theme = mode;
  applyTheme(mode);
}

export function clearLogs() {
  app.logs = [];
}

const HISTORY_LEN = 60;

function pushHistory(map: Record<string, number[]>, id: string, v: number) {
  const arr = map[id] ?? (map[id] = []);
  arr.push(v);
  if (arr.length > HISTORY_LEN) arr.shift();
}

function recalculateTotalRates() {
  let tx = 0;
  let rx = 0;
  for (const t of app.tunnels) {
    if (t.running && t.status === "connected") {
      tx += last(app.txHistory[t.id]);
      rx += last(app.rxHistory[t.id]);
    }
  }
  app.totalTx = tx;
  app.totalRx = rx;
}

/** 初始化事件订阅并拉取首屏数据。返回清理函数。 */
export async function initApp(): Promise<() => void> {
  if (typeof document !== "undefined") document.documentElement.lang = app.language === "en" ? "en" : "zh-CN";
  // 确保主题立即挂载到 DOM
  applyTheme(app.theme);

  // 后端不可用（如纯浏览器 dev 模式）时也能渲染布局。
  await Promise.all([refreshTunnels(), refreshHosts()]).catch(() => {});

  const offStatus = Events.On(EV.tunnelStatus, (ev: any) => {
    const d: StatusEvent = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
    const t = app.tunnels.find((x) => x.id === d.tunnelId);
    if (!t) return;
    statusEventRevisions.set(d.tunnelId, (statusEventRevisions.get(d.tunnelId) ?? 0) + 1);
    t.status = d.status;
    t.running = d.status !== "stopped" && d.status !== "error";
    t.error = d.error ?? "";
    // 状态翻转时清空历史，避免折线跳变。
    if (d.status !== "connected") {
      app.txHistory[d.tunnelId] = [];
      app.rxHistory[d.tunnelId] = [];
    }
    recalculateTotalRates();
  });

  const offStats = Events.On(EV.tunnelStats, (ev: any) => {
    const d: StatsEvent = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
    pushHistory(app.txHistory, d.tunnelId, d.tx);
    pushHistory(app.rxHistory, d.tunnelId, d.rx);
    const t = app.tunnels.find((x) => x.id === d.tunnelId);
    if (t) {
      t.stats = {
        tx: d.txTotal,
        rx: d.rxTotal,
        activeConn: d.conns,
        totalConn: d.totalConns,
      };
    }
    recalculateTotalRates();
  });

  const offLog = Events.On(EV.log, (ev: any) => {
    const d = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
    const newEntry: LogEntry = {
      id: ++logIdSeq,
      time: String(d?.time ?? new Date().toISOString()),
      message: String(d?.message ?? ""),
      tunnelId: d?.tunnelId,
    };
    app.logs = [newEntry, ...app.logs.slice(0, 199)];
  });

  return () => {
    offStatus();
    offStats();
    offLog();
  };
}

function last(arr?: number[]): number {
  return arr && arr.length > 0 ? arr[arr.length - 1] : 0;
}

export async function refreshTunnels() {
  app.loading = true;
  try {
    app.tunnels = (await TunnelService.List()) ?? [];
    recalculateTotalRates();
  } finally {
    app.loading = false;
  }
}

export async function refreshHosts() {
  app.hosts = (await HostService.List()) ?? [];
}

const pendingTunnelOperations = new Map<string, Promise<void>>();

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function runTunnelOperation(id: string, operation: () => Promise<void>): Promise<void> {
  const pending = pendingTunnelOperations.get(id);
  if (pending) return pending;
  app.tunnelBusy[id] = true;
  delete app.tunnelOperationErrors[id];
  const task = Promise.resolve().then(operation).catch((error: unknown) => {
    app.tunnelOperationErrors[id] = errorMessage(error);
    throw error;
  }).finally(() => {
    pendingTunnelOperations.delete(id);
    delete app.tunnelBusy[id];
  });
  pendingTunnelOperations.set(id, task);
  return task;
}

export function startTunnel(id: string): Promise<void> {
  return runTunnelOperation(id, async () => {
    const eventRevision = statusEventRevisions.get(id) ?? 0;
    await TunnelService.Start(id);
    const tunnel = app.tunnels.find((entry) => entry.id === id);
    if (tunnel && (statusEventRevisions.get(id) ?? 0) === eventRevision) {
      tunnel.running = true;
      if (tunnel.status === "stopped" || tunnel.status === "error") tunnel.status = "starting";
      tunnel.error = "";
    }
  });
}

export function stopTunnel(id: string): Promise<void> {
  return runTunnelOperation(id, async () => {
    await TunnelService.Stop(id);
    const tunnel = app.tunnels.find((entry) => entry.id === id);
    if (tunnel) {
      tunnel.running = false;
      tunnel.status = "stopped";
      tunnel.error = "";
    }
    app.txHistory[id] = [];
    app.rxHistory[id] = [];
    recalculateTotalRates();
  });
}

export function deleteTunnel(id: string): Promise<void> {
  return runTunnelOperation(id, async () => {
    await TunnelService.Delete(id);
    app.tunnels = app.tunnels.filter((entry) => entry.id !== id);
    delete app.txHistory[id];
    delete app.rxHistory[id];
    statusEventRevisions.delete(id);
    recalculateTotalRates();
  });
}

export type TunnelOperationFailure = { id: string; name: string; error: string };

/** Wait for every operation, keeping each failure visible alongside its tunnel. */
export async function setAllTunnelsRunning(running: boolean): Promise<TunnelOperationFailure[]> {
  const targets = app.tunnels.filter((tunnel) => tunnel.running !== running);
  const results = await Promise.allSettled(targets.map((tunnel) => running ? startTunnel(tunnel.id) : stopTunnel(tunnel.id)));
  return results.flatMap((result, index) => result.status === "rejected"
    ? [{ id: targets[index].id, name: targets[index].name, error: errorMessage(result.reason) }]
    : []);
}

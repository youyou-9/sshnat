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

export const app = $state({
  page: "dashboard" as Page,
  language: loadLanguage(),
  theme: loadTheme(),
  tunnels: [] as Tunnel[],
  hosts: [] as Host[],
  loading: false,

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
  if (typeof localStorage === "undefined") return "zh";
  return localStorage.getItem("sshnat.language") === "en" ? "en" : "zh";
}

export function setLanguage(language: Locale) {
  app.language = language;
  if (typeof localStorage !== "undefined") localStorage.setItem("sshnat.language", language);
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
  // 确保主题立即挂载到 DOM
  applyTheme(app.theme);

  // 后端不可用（如纯浏览器 dev 模式）时也能渲染布局。
  await Promise.all([refreshTunnels(), refreshHosts()]).catch(() => {});

  const offStatus = Events.On(EV.tunnelStatus, (ev: any) => {
    const d: StatusEvent = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
    const t = app.tunnels.find((x) => x.id === d.tunnelId);
    if (!t) return;
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

export async function startTunnel(id: string) {
  await TunnelService.Start(id);
}

export async function stopTunnel(id: string) {
  await TunnelService.Stop(id);
}

export async function deleteTunnel(id: string) {
  await TunnelService.Delete(id);
  delete app.txHistory[id];
  delete app.rxHistory[id];
  await refreshTunnels();
}

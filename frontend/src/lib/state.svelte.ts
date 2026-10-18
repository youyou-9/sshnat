// 全局前端状态：Svelte 5 runes，不引入额外状态库。
import { Events } from "@wailsio/runtime";
import {
  EV,
  TunnelService,
  HostService,
  SettingsService,
  type Host,
  type StatusEvent,
  type StatsEvent,
  type Tunnel,
} from "./api";
import { t, type Locale } from "./i18n";
import { loadTheme, applyTheme, type ThemeMode } from "./theme";

export type Page = "workbench" | "hosts" | "logs" | "settings";
export type LogEntry = { id: number; time: string; message: string; tunnelId?: string };
export type Toast = { id: number; kind: "success" | "error" | "info"; message: string };
export type TerminalTab = { id: string; hostId: string; title: string; subtitle: string };

let logIdSeq = 0;
const statusEventRevisions = new Map<string, number>();

export const app = $state({
  page: "workbench" as Page,
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

  // 右上角瞬时通知。
  toasts: [] as Toast[],

  // 重连中隧道的下次重试时间戳（毫秒），供倒计时展示。
  nextRetryAt: {} as Record<string, number>,

  // 多终端标签：底部 Dock 中的会话。
  terminals: [] as TerminalTab[],
  activeTerminalId: "",

  // 命令面板显隐；新建隧道弹窗由命令面板/快捷键共用的全局开关驱动。
  paletteOpen: false,
  newTunnelOpen: false,

  // server 构建没有原生文件对话框，前端据此隐藏相关入口。
  serverMode: false,

  // SSH 终端外观（等宽字体族 + 字号），在终端内可 Ctrl+滚轮/加减号实时缩放。
  terminalFontFamily: loadTerminalFont(),
  terminalFontSize: loadTerminalFontSize(),
});

/** 可选等宽字体：都带系统回退，缺失时自动落到下一项。 */
export const TERMINAL_FONTS = [
  "JetBrains Mono",
  "Cascadia Code",
  "Fira Code",
  "Consolas",
  "monospace",
] as const;

export const TERMINAL_FONT_MIN = 9;
export const TERMINAL_FONT_MAX = 28;

function loadTerminalFont(): string {
  const saved = getStorage()?.getItem("sshnat.termFont");
  return saved && (TERMINAL_FONTS as readonly string[]).includes(saved) ? saved : "JetBrains Mono";
}

function loadTerminalFontSize(): number {
  const saved = Number(getStorage()?.getItem("sshnat.termFontSize"));
  return Number.isFinite(saved) && saved >= TERMINAL_FONT_MIN && saved <= TERMINAL_FONT_MAX ? saved : 13;
}

export function setTerminalFont(family: string) {
  if (!(TERMINAL_FONTS as readonly string[]).includes(family)) return;
  app.terminalFontFamily = family;
  getStorage()?.setItem("sshnat.termFont", family);
}

export function setTerminalFontSize(size: number) {
  const clamped = Math.min(TERMINAL_FONT_MAX, Math.max(TERMINAL_FONT_MIN, Math.round(size)));
  app.terminalFontSize = clamped;
  getStorage()?.setItem("sshnat.termFontSize", String(clamped));
  return clamped;
}

/** 终端内 Ctrl+滚轮 / Ctrl+加减号：按步长缩放并持久化。 */
export function zoomTerminalFont(delta: number): number {
  return setTerminalFontSize(app.terminalFontSize + delta);
}

// 打开的模态框计数刻意不放进 $state：它只在键盘事件里按需读取，
// 放进响应式图会让 $effect 自触发（先读后写同一状态）导致死循环。
let dialogOpenCount = 0;

export function dialogOpened() {
  dialogOpenCount += 1;
}

export function dialogClosed() {
  dialogOpenCount = Math.max(0, dialogOpenCount - 1);
}

export function anyDialogOpen(): boolean {
  return dialogOpenCount > 0;
}

/** 统一页面跳转：离开当前页时收口「新建隧道」弹窗，命令面板可安全调用。 */
export function gotoPage(page: Page) {
  if (app.page === page) return;
  app.newTunnelOpen = false;
  app.page = page;
}

/** 主机被删除后，清掉属于它的终端标签（其 pane 已随 host 消失而销毁）。 */
/** 配置导入/替换后统一对账：清掉已不存在主机的终端标签与已删除隧道的重试时间戳。 */
export function pruneTerminalState() {
  const hostIds = new Set(app.hosts.map((h) => h.id));
  const remaining = app.terminals.filter((tab) => hostIds.has(tab.hostId));
  if (remaining.length !== app.terminals.length) {
    if (!remaining.some((tab) => tab.id === app.activeTerminalId)) {
      app.activeTerminalId = remaining.at(-1)?.id ?? "";
    }
    app.terminals = remaining;
  }
  const tunnelIds = new Set(app.tunnels.map((entry) => entry.id));
  for (const id of Object.keys(app.nextRetryAt)) {
    if (!tunnelIds.has(id)) delete app.nextRetryAt[id];
  }
}

export function dropTerminalTabsForHost(hostId: string) {
  const remaining = app.terminals.filter((tab) => tab.hostId !== hostId);
  if (remaining.length === app.terminals.length) return;
  if (!remaining.some((tab) => tab.id === app.activeTerminalId)) {
    app.activeTerminalId = remaining.at(-1)?.id ?? "";
  }
  app.terminals = remaining;
}

let terminalIdSeq = 0;

/** 打开（或聚焦）某台主机的终端标签。 */
export function openTerminal(host: Host) {
  const existing = app.terminals.find((tab) => tab.hostId === host.id);
  if (existing) {
    app.activeTerminalId = existing.id;
    return;
  }
  const tab: TerminalTab = {
    id: `term-tab-${++terminalIdSeq}`,
    hostId: host.id,
    title: host.name,
    subtitle: `${host.user}@${host.host}:${host.port || 22}`,
  };
  app.terminals = [...app.terminals, tab];
  app.activeTerminalId = tab.id;
}

export function closeTerminalTab(id: string) {
  app.terminals = app.terminals.filter((tab) => tab.id !== id);
  if (app.activeTerminalId === id) {
    app.activeTerminalId = app.terminals.at(-1)?.id ?? "";
  }
}

let toastIdSeq = 0;

/** 弹一条瞬时通知；错误类停留更久。 */
export function pushToast(kind: Toast["kind"], message: string) {
  if (!message) return;
  const id = ++toastIdSeq;
  app.toasts = [...app.toasts.slice(-4), { id, kind, message }];
  setTimeout(() => dismissToast(id), kind === "error" ? 6000 : 3000);
}

export function dismissToast(id: number) {
  app.toasts = app.toasts.filter((toast) => toast.id !== id);
}

function loadLanguage(): Locale {
  const storage = getStorage();
  return storage?.getItem("sshnat.language") === "en" ? "en" : "zh";
}

function applyLanguageDom(language: Locale) {
  if (typeof document !== "undefined") document.documentElement.lang = language === "en" ? "en" : "zh-CN";
}

export function setLanguage(language: Locale) {
  const previous = app.language;
  const previousStorage = getStorage()?.getItem("sshnat.language") ?? null;
  // 先乐观切换，保证交互即时；后端（托盘语言来源）写入失败则回滚并提示。
  app.language = language;
  applyLanguageDom(language);
  return SettingsService.SetLanguage(language)
    .then(() => {
      getStorage()?.setItem("sshnat.language", language);
    })
    .catch((error: unknown) => {
      app.language = previous;
      applyLanguageDom(previous);
      if (previousStorage) getStorage()?.setItem("sshnat.language", previousStorage);
      pushToast("error", `${t(previous, "settings.language")}: ${errorMessage(error)}`);
    });
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

  // 能力位：server 构建没有原生文件对话框，隐藏相关入口而不是点了才报错。
  try {
    const info = await SettingsService.Get();
    app.serverMode = !!info?.serverMode;
  } catch { /* 纯前端模式：保持默认 */ }

  // 后端持久化的语言优先于本地缓存（托盘语言同源）；未设置时把本地选择推给后端。
  try {
    const persisted = await SettingsService.GetLanguage();
    if ((persisted === "en" || persisted === "zh") && persisted !== app.language) {
      app.language = persisted;
      if (typeof document !== "undefined") document.documentElement.lang = persisted === "en" ? "en" : "zh-CN";
      getStorage()?.setItem("sshnat.language", persisted);
    } else if (!persisted) {
      SettingsService.SetLanguage(app.language).catch(() => {});
    }
  } catch { /* 纯前端模式：忽略 */ }

  const offStatus = Events.On(EV.tunnelStatus, (ev: any) => {
    const d: StatusEvent = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
    const t = app.tunnels.find((x) => x.id === d.tunnelId);
    if (!t) return;
    statusEventRevisions.set(d.tunnelId, (statusEventRevisions.get(d.tunnelId) ?? 0) + 1);
    t.status = d.status;
    t.running = d.status !== "stopped" && d.status !== "error";
    t.error = d.error ?? "";
    if (d.status === "reconnecting" && d.nextInMs) {
      app.nextRetryAt[d.tunnelId] = Date.now() + d.nextInMs;
    } else {
      delete app.nextRetryAt[d.tunnelId];
    }
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
    delete app.nextRetryAt[id];
    recalculateTotalRates();
  });
}

export function deleteTunnel(id: string): Promise<void> {
  return runTunnelOperation(id, async () => {
    await TunnelService.Delete(id);
    app.tunnels = app.tunnels.filter((entry) => entry.id !== id);
    delete app.txHistory[id];
    delete app.rxHistory[id];
    delete app.nextRetryAt[id];
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

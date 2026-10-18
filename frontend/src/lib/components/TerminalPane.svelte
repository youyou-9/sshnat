<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import "@xterm/xterm/css/xterm.css";
  import { LoaderCircle, RefreshCw } from "@lucide/svelte";
  import { Events } from "@wailsio/runtime";
  import { EV, TerminalService, type Host } from "@/lib/api";
  import { app, zoomTerminalFont } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  /** 字体族追加等宽回退，避免所选字体未安装时落到比例字体。 */
  function fontStack(family: string): string {
    const fallback = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";
    return family === "monospace" ? fallback : `"${family}", ${fallback}`;
  }

  interface Props {
    host: Host;
    active: boolean;
  }
  let { host, active }: Props = $props();

  type Phase = "connecting" | "connected" | "exited" | "error";
  let phase = $state<Phase>("connecting");
  let errorMessage = $state("");

  let container = $state<HTMLDivElement | null>(null);
  let term: Terminal | null = null;
  let fitAddon: FitAddon | null = null;
  let sessionId = "";
  let offData: (() => void) | null = null;
  let offExit: (() => void) | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let disposed = false;
  let connecting = false;
  let onWheelRef: (event: WheelEvent) => void = () => {};
  // 退出事件可能早于 Open 响应到达：先记录（含错误文案），解析后再对账，
  // 避免出现“显示已连接、实际 shell 已退出”的空壳。
  const exitedSessions = new Map<string, string>();

  function newSessionId(): string {
    const rand = Math.random().toString(36).slice(2, 10);
    return `term_${Date.now().toString(36)}_${rand}`;
  }

  async function connect() {
    if (connecting) return; // 连点重连时避免并发 Open 造成会话泄漏
    connecting = true;
    phase = "connecting";
    errorMessage = "";
    if (!term || !fitAddon) {
      connecting = false;
      return;
    }
    term.reset();
    // 先生成并写入 sessionId，保证后端最早的一批输出事件不会因 id 未知被丢弃。
    sessionId = newSessionId();
    const requestedId = sessionId;
    try {
      fitAddon.fit();
      const dims = fitAddon.proposeDimensions();
      const session = await TerminalService.Open({
        hostId: host.id,
        cols: dims?.cols ?? 80,
        rows: dims?.rows ?? 24,
        sessionId,
      });
      if (!session) throw new Error("no session returned");
      // 组件已销毁（如关闭标签）时立刻回收后端会话，避免 SSH/PTY 泄漏。
      if (disposed) {
        TerminalService.Close(session.sessionId).catch(() => {});
        sessionId = "";
        return;
      }
      const resolvedId = session.sessionId || requestedId;
      sessionId = resolvedId;
      // 对账：连接期间到达的退出事件说明 shell 已结束，不能报“已连接”。
      if (exitedSessions.has(resolvedId)) {
        errorMessage = exitedSessions.get(resolvedId) ?? "";
        exitedSessions.delete(resolvedId);
        sessionId = "";
        phase = "exited";
        return;
      }
      phase = "connected";
      refit();
    } catch (e) {
      if (!disposed) {
        errorMessage = String(e instanceof Error ? e.message : e);
        phase = "error";
      }
      sessionId = "";
    } finally {
      connecting = false;
    }
  }

  function refit() {
    if (!fitAddon || !term || !container) return;
    // 隐藏或折叠时容器尺寸为 0，fit() 会算出非法行列数，直接跳过。
    if (container.clientWidth < 20 || container.clientHeight < 20) return;
    fitAddon.fit();
    if (sessionId && term.cols > 0 && term.rows > 0) {
      TerminalService.Resize(sessionId, term.cols, term.rows).catch(() => {});
    }
  }

  onMount(() => {
    if (!container) return;
    term = new Terminal({
      cursorBlink: true,
      fontSize: app.terminalFontSize,
      fontFamily: fontStack(app.terminalFontFamily),
      theme: {
        background: "#0d1117",
        foreground: "#e6edf3",
        cursor: "#22d3ee",
        selectionBackground: "#264f78",
      },
    });
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(container);
    term.onData((data) => {
      if (sessionId) TerminalService.Write(sessionId, data).catch(() => {});
    });

    // Ctrl+滚轮缩放字号（终端内最顺手的操作；阻止页面级缩放）。
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      zoomTerminalFont(event.deltaY < 0 ? 1 : -1);
    };
    onWheelRef = onWheel;
    container.addEventListener("wheel", onWheel, { passive: false });

    // Ctrl+加号/减号/0：缩放与重置，不把组合键发给远端 shell。
    term.attachCustomKeyEventHandler((event) => {
      if (event.type !== "keydown" || !(event.ctrlKey || event.metaKey)) return true;
      const key = event.key;
      if (key === "+" || key === "=") {
        zoomTerminalFont(1);
        return false;
      }
      if (key === "-" || key === "_") {
        zoomTerminalFont(-1);
        return false;
      }
      if (key === "0") {
        zoomTerminalFont(13 - app.terminalFontSize);
        return false;
      }
      return true;
    });

    offData = Events.On(EV.terminalData, (ev: any) => {
      const d = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
      if (d?.sessionId === sessionId && typeof d?.data === "string") term?.write(d.data);
    });
    offExit = Events.On(EV.terminalExit, (ev: any) => {
      const d = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev);
      if (!d?.sessionId) return;
      const exitedId = String(d.sessionId);
      const exitError = d?.error ? String(d.error) : "";
      if (exitedId !== sessionId) return; // 其他标签的会话
      // 连接期间的退出必须先记下，等 Open 返回后再对账，
      // 否则这里清空 sessionId 会让 Open 结果覆盖成“已连接”假象。
      if (phase === "connecting") {
        exitedSessions.set(exitedId, exitError);
        return;
      }
      sessionId = "";
      errorMessage = exitError;
      phase = "exited";
    });

    resizeObserver = new ResizeObserver(() => refit());
    resizeObserver.observe(container);
    connect();
  });

  // 标签切换到本 pane 时重新适配尺寸（隐藏期间容器尺寸为 0）。
  $effect(() => {
    if (active) {
      setTimeout(refit, 30);
      term?.focus();
    }
  });

  // 设置页/快捷键改字号或字体后，立即应用到已打开的终端并重新适配行列。
  $effect(() => {
    const size = app.terminalFontSize;
    const family = app.terminalFontFamily;
    if (!term) return;
    term.options.fontSize = size;
    term.options.fontFamily = fontStack(family);
    refit();
  });

  onDestroy(() => {
    disposed = true;
    if (container) container.removeEventListener("wheel", onWheelRef);
    resizeObserver?.disconnect();
    offData?.();
    offExit?.();
    if (sessionId) TerminalService.Close(sessionId).catch(() => {});
    sessionId = "";
    term?.dispose();
    term = null;
  });
</script>

<div class="relative h-full w-full bg-[#0d1117]">
  <div bind:this={container} class="h-full w-full pl-2 pt-1"></div>
  {#if phase === "connecting"}
    <div class="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 text-xs text-dim">
      <LoaderCircle size={14} class="animate-spin" />{t(app.language, "terminal.connecting")}
    </div>
  {:else if phase === "exited" || phase === "error"}
    <div class="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/70">
      <p class="max-w-lg break-words px-4 text-center text-sm {phase === 'error' ? 'text-bad' : 'text-dim'}">
        {phase === "error" ? `${t(app.language, "terminal.error")}: ${errorMessage}` : t(app.language, "terminal.exited")}
        {#if phase === "exited" && errorMessage}<span class="text-bad"> · {errorMessage}</span>{/if}
      </p>
      <button class="flex items-center gap-1 rounded-chip border border-edge bg-panel px-3 py-1.5 text-xs text-ink hover:border-accent" onclick={connect}>
        <RefreshCw size={13} />{t(app.language, "terminal.reconnect")}
      </button>
    </div>
  {/if}
</div>

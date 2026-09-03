<script lang="ts">
  import {
    Plus,
    Inbox,
    Play,
    Square,
    Activity,
    ArrowUp,
    ArrowDown,
    Server,
    Radio,
    ScrollText,
    ArrowRight,
  } from "@lucide/svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import TunnelCard from "@/lib/components/TunnelCard.svelte";
  import NewTunnelModal from "@/lib/components/NewTunnelModal.svelte";
  import { app, startTunnel, stopTunnel } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  let modalOpen = $state(false);

  const runningTunnels = $derived(app.tunnels.filter((t) => t.running));
  const connectedCount = $derived(app.tunnels.filter((t) => t.status === "connected").length);
  const totalActiveConns = $derived(
    app.tunnels.reduce((acc, t) => acc + (t.stats?.activeConn ?? 0), 0)
  );

  function fmtRate(n: number): string {
    if (n <= 0) return "0 B/s";
    const units = ["B/s", "KB/s", "MB/s", "GB/s"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
  }

  function fmtBytes(n: number): string {
    if (!n) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
  }

  const totalTxBytes = $derived(
    app.tunnels.reduce((acc, t) => acc + (t.stats?.tx ?? 0), 0)
  );
  const totalRxBytes = $derived(
    app.tunnels.reduce((acc, t) => acc + (t.stats?.rx ?? 0), 0)
  );

  let batchOperating = $state(false);

  async function startAll() {
    batchOperating = true;
    try {
      await Promise.all(app.tunnels.map((t) => (!t.running ? startTunnel(t.id) : Promise.resolve())));
    } finally {
      batchOperating = false;
    }
  }

  async function stopAll() {
    batchOperating = true;
    try {
      await Promise.all(app.tunnels.map((t) => (t.running ? stopTunnel(t.id) : Promise.resolve())));
    } finally {
      batchOperating = false;
    }
  }
</script>

<div class="flex h-full flex-col">
  <!-- 头部 -->
  <header class="flex h-14 shrink-0 items-center justify-between border-b border-edge px-5">
    <div>
      <div class="flex items-center gap-2">
        <h1 class="text-[15px] font-semibold text-ink">{t(app.language, "dashboard.title")}</h1>
        <span class="rounded-full bg-ok/10 border border-ok/30 px-2 py-0.5 text-[10px] text-ok font-mono flex items-center gap-1">
          <span class="size-1.5 rounded-full bg-ok animate-pulse"></span>
          {t(app.language, "dashboard.systemRunning")}
        </span>
      </div>
      <p class="text-xs text-dim">
        {app.tunnels.length} {t(app.language, "dashboard.tunnels")} · {connectedCount} {t(app.language, "dashboard.connected")} · {app.hosts.length} {t(app.language, "hosts.count")}
      </p>
    </div>
    <div class="flex items-center gap-2">
      <Button variant="ghost" size="sm" disabled={batchOperating || app.tunnels.length === 0} onclick={startAll}>
        <Play size={12} class="mr-1 text-ok" /> {t(app.language, "common.startAll")}
      </Button>
      <Button variant="ghost" size="sm" disabled={batchOperating || runningTunnels.length === 0} onclick={stopAll}>
        <Square size={12} class="mr-1 text-bad" /> {t(app.language, "common.stopAll")}
      </Button>
      <Button variant="primary" size="sm" onclick={() => (modalOpen = true)}>
        <Plus size={14} class="mr-1" />
        {t(app.language, "dashboard.new")}
      </Button>
    </div>
  </header>

  <!-- 仪表盘主体 -->
  <div class="flex-1 space-y-5 overflow-y-auto p-5">
    <!-- Top KPI 指标栅格 -->
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-5">
      <!-- KPI 1: 隧道运行态 -->
      <div class="rounded-card border border-edge bg-panel p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-dim text-xs">
          <span>{t(app.language, "dashboard.tunnelStatus")}</span>
          <Radio size={14} class={connectedCount > 0 ? "text-ok" : "text-dim"} />
        </div>
        <div class="mt-2 flex items-baseline gap-1.5">
          <span class="font-mono text-xl font-bold text-ink">{connectedCount}</span>
          <span class="text-xs text-dim">/ {app.tunnels.length} {t(app.language, "dashboard.running")}</span>
        </div>
      </div>

      <!-- KPI 2: 实时上传速率 -->
      <div class="rounded-card border border-edge bg-panel p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-dim text-xs">
          <span>{t(app.language, "dashboard.realtimeTx")}</span>
          <ArrowUp size={14} class="text-accent" />
        </div>
        <div class="mt-2">
          <div class="font-mono text-xl font-bold text-accent">{fmtRate(app.totalTx)}</div>
          <div class="mt-0.5 text-[11px] text-dim/70">{t(app.language, "dashboard.cumulative")}: {fmtBytes(totalTxBytes)}</div>
        </div>
      </div>

      <!-- KPI 3: 实时下载速率 -->
      <div class="rounded-card border border-edge bg-panel p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-dim text-xs">
          <span>{t(app.language, "dashboard.realtimeRx")}</span>
          <ArrowDown size={14} class="text-ok" />
        </div>
        <div class="mt-2">
          <div class="font-mono text-xl font-bold text-ok">{fmtRate(app.totalRx)}</div>
          <div class="mt-0.5 text-[11px] text-dim/70">{t(app.language, "dashboard.cumulative")}: {fmtBytes(totalRxBytes)}</div>
        </div>
      </div>

      <!-- KPI 4: 活跃连接 -->
      <div class="rounded-card border border-edge bg-panel p-3.5 flex flex-col justify-between">
        <div class="flex items-center justify-between text-dim text-xs">
          <span>{t(app.language, "dashboard.activeConns")}</span>
          <Activity size={14} class="text-accent" />
        </div>
        <div class="mt-2 flex items-baseline gap-1.5">
          <span class="font-mono text-xl font-bold text-ink">{totalActiveConns}</span>
          <span class="text-xs text-dim">{t(app.language, "dashboard.tcpSessions")}</span>
        </div>
      </div>

      <!-- KPI 5: 托管主机数 -->
      <div class="rounded-card border border-edge bg-panel p-3.5 flex flex-col justify-between col-span-2 sm:col-span-4 lg:col-span-1">
        <div class="flex items-center justify-between text-dim text-xs">
          <span>{t(app.language, "dashboard.configuredHosts")}</span>
          <Server size={14} class="text-dim" />
        </div>
        <div class="mt-2 flex items-baseline gap-1.5">
          <span class="font-mono text-xl font-bold text-ink">{app.hosts.length}</span>
          <span class="text-xs text-dim">{t(app.language, "dashboard.remoteHosts")}</span>
        </div>
      </div>
    </div>

    <!-- 运行中隧道监控区 -->
    <div>
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <h2 class="text-sm font-semibold text-ink">{t(app.language, "dashboard.activeTunnels")}</h2>
          <Badge tone={connectedCount > 0 ? "ok" : "dim"} mono>{connectedCount}</Badge>
        </div>
        <button
          type="button"
          class="flex items-center gap-1 text-xs text-accent hover:underline"
          onclick={() => (app.page = "tunnels")}
        >
          <span>{t(app.language, "dashboard.manageAll")}</span>
          <ArrowRight size={12} />
        </button>
      </div>

      {#if app.tunnels.length === 0}
        <div class="flex h-40 flex-col items-center justify-center gap-2.5 rounded-card border border-edge bg-panel p-5 text-dim">
          <Inbox size={28} class="opacity-40" />
          <p class="text-xs">{t(app.language, "dashboard.emptyTunnels")}</p>
          <Button variant="primary" size="sm" onclick={() => (modalOpen = true)}>
            <Plus size={12} class="mr-1" /> {t(app.language, "dashboard.createFirst")}
          </Button>
        </div>
      {:else if runningTunnels.length === 0}
        <div class="flex h-36 flex-col items-center justify-center gap-2.5 rounded-card border border-dashed border-edge bg-panel/50 p-5 text-dim">
          <Radio size={24} class="opacity-40" />
          <p class="text-xs">{t(app.language, "dashboard.noRunningTunnels")} ({app.tunnels.length} {t(app.language, "dashboard.tunnels")})</p>
          <Button variant="ghost" size="sm" onclick={startAll}>
            <Play size={12} class="mr-1 text-ok" /> {t(app.language, "dashboard.startAllTunnels")}
          </Button>
        </div>
      {:else}
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-2 2xl:grid-cols-3">
          {#each runningTunnels as t (t.id)}
            <TunnelCard tunnel={t} />
          {/each}
        </div>
      {/if}
    </div>

    <!-- 最近动态事件流 (Live Activity) -->
    <div class="rounded-card border border-edge bg-panel p-4">
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <ScrollText size={15} class="text-accent" />
          <h3 class="text-sm font-semibold text-ink">{t(app.language, "dashboard.recentActivity")}</h3>
          <span class="rounded bg-edge/40 px-1.5 py-0.5 text-[10px] text-dim font-mono">{app.logs.length}</span>
        </div>
        <button
          type="button"
          class="flex items-center gap-1 text-xs text-dim hover:text-accent transition-colors"
          onclick={() => (app.page = "logs")}
        >
          <span>{t(app.language, "dashboard.openAllLogs")}</span>
          <ArrowRight size={12} />
        </button>
      </div>

      {#if app.logs.length === 0}
        <p class="py-4 text-center text-xs text-dim">{t(app.language, "dashboard.noActivity")}</p>
      {:else}
        <ul class="space-y-1.5 font-mono text-xs max-h-48 overflow-y-auto">
          {#each app.logs.slice(0, 6) as log (log.id)}
            <li class="flex items-baseline gap-2.5 rounded px-2 py-1 bg-base/60 hover:bg-hover transition-colors">
              <span class="shrink-0 text-[11px] text-dim/60 font-mono">
                {log.time.slice(11, 19) || log.time}
              </span>
              {#if log.tunnelId}
                <span class="shrink-0 rounded bg-edge/50 px-1.5 py-0.2 text-[10px] text-accent font-mono">
                  {log.tunnelId}
                </span>
              {/if}
              <span class="text-ink truncate">{log.message}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
</div>

<NewTunnelModal bind:open={modalOpen} />

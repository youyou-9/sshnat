<script lang="ts">
  import {
    Plus,
    RefreshCw,
    Play,
    Square,
    Search,
    LayoutGrid,
    List,
    ScrollText,
    Pencil,
    Trash2,
    Copy,
    Check,
    ArrowLeftRight,
    Activity,
    Radio,
    ArrowUp,
    ArrowDown,
  } from "@lucide/svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import Switch from "@/lib/components/ui/Switch.svelte";
  import Input from "@/lib/components/ui/Input.svelte";
  import ConfirmDialog from "@/lib/components/ui/ConfirmDialog.svelte";
  import TunnelCard from "@/lib/components/TunnelCard.svelte";
  import NewTunnelModal from "@/lib/components/NewTunnelModal.svelte";
  import TunnelLogsModal from "@/lib/components/TunnelLogsModal.svelte";
  import { buildTunnelCliCommand, copyText, formatTunnelRoute } from "@/lib/tunnel-command";
  import {
    app,
    refreshTunnels,
    startTunnel,
    stopTunnel,
    deleteTunnel,
    setAllTunnelsRunning,
    errorMessage,
    pushToast,
  } from "@/lib/state.svelte";
  import type { Tunnel } from "@/lib/api";
  import { t, tn } from "@/lib/i18n";

  // 新建隧道弹窗由全局状态驱动，命令面板（Ctrl+K）也能打开它。
  let editModalOpen = $state(false);
  let logsModalOpen = $state(false);
  let activeTunnel = $state<Tunnel | null>(null);

  let searchQuery = $state("");
  let typeFilter = $state<"ALL" | "L" | "R" | "D">("ALL");
  let statusFilter = $state<"ALL" | "RUNNING" | "STOPPED">("ALL");
  let viewMode = $state<"grid" | "table">("grid");

  let batchOperating = $state(false);
  let copiedId = $state<string | null>(null);
  let actionError = $state("");
  let deletingTunnel = $state<Tunnel | null>(null);
  let deleteConfirmOpen = $state(false);
  const operating = $derived(batchOperating || Object.keys(app.tunnelBusy).length > 0);

  const runningCount = $derived(app.tunnels.filter((t) => t.running).length);
  const connectedCount = $derived(app.tunnels.filter((t) => t.status === "connected").length);
  const totalActiveConns = $derived(app.tunnels.reduce((acc, t) => acc + (t.stats?.activeConn ?? 0), 0));
  const totalTxBytes = $derived(app.tunnels.reduce((acc, t) => acc + (t.stats?.tx ?? 0), 0));
  const totalRxBytes = $derived(app.tunnels.reduce((acc, t) => acc + (t.stats?.rx ?? 0), 0));

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

  const filteredTunnels = $derived.by(() => {
    let list = app.tunnels;
    if (typeFilter !== "ALL") {
      list = list.filter((t) => t.type === typeFilter);
    }
    if (statusFilter === "RUNNING") {
      list = list.filter((t) => t.running);
    } else if (statusFilter === "STOPPED") {
      list = list.filter((t) => !t.running);
    }
    const q = searchQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((t) => {
        const host = app.hosts.find((h) => h.id === t.hostId);
        return (
          t.name.toLowerCase().includes(q) ||
          String(t.localPort).includes(q) ||
          String(t.targetPort).includes(q) ||
          String(t.remotePort).includes(q) ||
          String(t.socksPort).includes(q) ||
          (t.localBindHost?.toLowerCase().includes(q) ?? false) ||
          (t.localSocket?.toLowerCase().includes(q) ?? false) ||
          (t.targetHost?.toLowerCase().includes(q) ?? false) ||
          (t.targetSocket?.toLowerCase().includes(q) ?? false) ||
          (t.remoteBindHost?.toLowerCase().includes(q) ?? false) ||
          (t.remoteSocket?.toLowerCase().includes(q) ?? false) ||
          (host?.name.toLowerCase().includes(q) ?? false) ||
          (host?.host.toLowerCase().includes(q) ?? false)
        );
      });
    }
    return list;
  });

  function formatRoute(t: Tunnel): string {
    return formatTunnelRoute(t);
  }

  function fmtBytes(n?: number): string {
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

  function getHostName(hostId: string): string {
    return app.hosts.find((h) => h.id === hostId)?.name ?? hostId;
  }

  async function copyCli(tunnel: Tunnel) {
    const host = app.hosts.find((h) => h.id === tunnel.hostId);
    try {
      if (await copyText(buildTunnelCliCommand(tunnel, host, { hosts: app.hosts, useAppDefaults: true }))) {
        copiedId = tunnel.id;
        pushToast("success", `${tunnel.name}: ${t(app.language, "common.copied")}`);
        setTimeout(() => (copiedId = null), 1500);
      } else actionError = t(app.language, "tunnel.copyError");
    } catch (error) {
      actionError = errorMessage(error);
    }
  }

  async function handleToggle(tunnel: Tunnel, checked: boolean) {
    if (app.tunnelBusy[tunnel.id]) return;
    actionError = "";
    try {
      if (checked) await startTunnel(tunnel.id);
      else await stopTunnel(tunnel.id);
      pushToast("success", `${tunnel.name}: ${t(app.language, checked ? "toast.tunnelStarted" : "toast.tunnelStopped")}`);
    } catch (error) {
      actionError = `${tunnel.name}: ${errorMessage(error)}`;
      pushToast("error", actionError);
    }
  }

  function askDelete(tunnel: Tunnel) {
    if (app.tunnelBusy[tunnel.id]) return;
    deletingTunnel = tunnel;
    deleteConfirmOpen = true;
  }

  async function handleDelete() {
    const tunnel = deletingTunnel;
    if (!tunnel || app.tunnelBusy[tunnel.id]) return;
    actionError = "";
    try {
      await deleteTunnel(tunnel.id);
      pushToast("success", `${tunnel.name}: ${t(app.language, "toast.tunnelDeleted")}`);
    } catch (error) {
      actionError = errorMessage(error);
      pushToast("error", `${tunnel.name}: ${actionError}`);
    }
  }

  async function handleRefresh() {
    if (app.loading) return;
    actionError = "";
    try {
      await refreshTunnels();
    } catch (error) {
      actionError = errorMessage(error);
    }
  }

  function openEdit(t: Tunnel) {
    activeTunnel = t;
    editModalOpen = true;
  }

  function openLogs(t: Tunnel) {
    activeTunnel = t;
    logsModalOpen = true;
  }

  async function startAll() {
    if (operating) return;
    batchOperating = true;
    actionError = "";
    try {
      const failures = await setAllTunnelsRunning(true);
      actionError = failures.map((failure) => `${failure.name}: ${failure.error}`).join("\n");
    } finally {
      batchOperating = false;
    }
  }

  async function stopAll() {
    if (operating) return;
    batchOperating = true;
    actionError = "";
    try {
      const failures = await setAllTunnelsRunning(false);
      actionError = failures.map((failure) => `${failure.name}: ${failure.error}`).join("\n");
    } finally {
      batchOperating = false;
    }
  }
</script>

<div class="flex h-full flex-col">
  <!-- 头部控制栏 -->
  <header class="flex h-14 shrink-0 items-center justify-between border-b border-edge px-5">
    <div>
      <div class="flex items-center gap-2">
        <h1 class="text-title font-semibold tracking-[-0.01em] text-ink">{t(app.language, "workbench.title")}</h1>
        {#if connectedCount > 0}
          <span class="flex items-center gap-1 rounded-full border border-ok/30 bg-ok/10 px-2 py-0.5 font-mono text-[10px] text-ok">
            <span class="size-1.5 animate-pulse rounded-full bg-ok"></span>
            {t(app.language, "dashboard.systemRunning")}
          </span>
        {/if}
      </div>
      <p class="text-xs text-dim">
        {tn(app.language, app.tunnels.length, "unit.tunnelOne", "unit.tunnelMany")} · {runningCount} {t(app.language, "tunnels.running")} · {tn(app.language, app.hosts.length, "unit.hostOne", "unit.hostMany")}
      </p>
    </div>
    <div class="flex items-center gap-2">
      <Button variant="ghost" size="sm" disabled={operating || runningCount === app.tunnels.length} onclick={startAll} title={t(app.language, "common.startAll")}>
        <Play size={12} class="mr-1 text-ok" /> {t(app.language, "common.startAll")}
      </Button>
      <Button variant="ghost" size="sm" disabled={operating || runningCount === 0} onclick={stopAll} title={t(app.language, "common.stopAll")}>
        <Square size={12} class="mr-1 text-bad" /> {t(app.language, "common.stopAll")}
      </Button>
      <button
        class="flex items-center gap-1 rounded border border-edge bg-panel px-2.5 py-1 text-xs text-dim transition-colors hover:text-ink hover:bg-hover"
        onclick={handleRefresh}
        disabled={app.loading || operating}
        title={t(app.language, "tunnels.refresh")}
      >
        <RefreshCw size={12} />
        <span>{t(app.language, "common.refresh")}</span>
      </button>
      <Button variant="primary" size="sm" onclick={() => (app.newTunnelOpen = true)}>
        <Plus size={14} class="mr-1" />
        {t(app.language, "dashboard.new")}
      </Button>
    </div>
  </header>
  {#if actionError}
    <p role="alert" class="m-3 whitespace-pre-wrap rounded-chip border border-bad/40 bg-bad/10 px-3 py-2 text-xs text-bad">{actionError}</p>
  {/if}

  <!-- KPI 概览条：一眼看清运行态与流量 -->
  <div class="grid shrink-0 grid-cols-2 gap-3 px-5 pt-4 lg:grid-cols-4">
    <div class="group rounded-card border border-edge bg-panel px-3.5 py-3 transition-colors hover:border-accent/25">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-medium uppercase tracking-wider text-dim">{t(app.language, "dashboard.activeTunnels")}</span>
        <span class="flex size-6 items-center justify-center rounded-chip bg-accent/10 text-accent"><Activity size={12} /></span>
      </div>
      <div class="mt-2 font-mono text-xl leading-none text-ink">{connectedCount}<span class="ml-1.5 font-sans text-xs text-dim">/ {app.tunnels.length}</span></div>
      <div class="mt-1.5 text-[10px] text-dim-2">{t(app.language, "dashboard.running")}</div>
    </div>
    <div class="group rounded-card border border-edge bg-panel px-3.5 py-3 transition-colors hover:border-accent/25">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-medium uppercase tracking-wider text-dim">{t(app.language, "dashboard.activeConns")}</span>
        <span class="flex size-6 items-center justify-center rounded-chip bg-ok/10 text-ok"><Radio size={12} /></span>
      </div>
      <div class="mt-2 font-mono text-xl leading-none text-ink">{totalActiveConns}</div>
      <div class="mt-1.5 text-[10px] text-dim-2">{t(app.language, "dashboard.tcpSessions")}</div>
    </div>
    <div class="group rounded-card border border-edge bg-panel px-3.5 py-3 transition-colors hover:border-accent/25">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-medium uppercase tracking-wider text-dim">{t(app.language, "dashboard.realtimeTx")}</span>
        <span class="flex size-6 items-center justify-center rounded-chip bg-accent/10 text-accent"><ArrowUp size={12} /></span>
      </div>
      <div class="mt-2 flex items-baseline gap-1.5">
        <span class="font-mono text-xl leading-none text-ink">{fmtRate(app.totalTx)}</span>
        <span class="font-mono text-[10px] text-dim">{fmtBytes(totalTxBytes)}</span>
      </div>
      <div class="mt-1.5 text-[10px] text-dim-2">{t(app.language, "dashboard.cumulative")}</div>
    </div>
    <div class="group rounded-card border border-edge bg-panel px-3.5 py-3 transition-colors hover:border-accent/25">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-medium uppercase tracking-wider text-dim">{t(app.language, "dashboard.realtimeRx")}</span>
        <span class="flex size-6 items-center justify-center rounded-chip bg-ok/10 text-ok"><ArrowDown size={12} /></span>
      </div>
      <div class="mt-2 flex items-baseline gap-1.5">
        <span class="font-mono text-xl leading-none text-ink">{fmtRate(app.totalRx)}</span>
        <span class="font-mono text-[10px] text-dim">{fmtBytes(totalRxBytes)}</span>
      </div>
      <div class="mt-1.5 text-[10px] text-dim-2">{t(app.language, "dashboard.cumulative")}</div>
    </div>
  </div>

  <!-- 工具栏：搜索、分类、视图切换 -->
  <div class="mt-4 flex flex-wrap items-center justify-between gap-3 border-y border-edge/60 bg-base/50 px-5 py-2.5">
    <!-- 搜索与类型过滤 -->
    <div class="flex flex-wrap items-center gap-2">
      <div class="relative w-56">
        <Search size={13} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-dim" />
        <input
          type="text"
          placeholder={t(app.language, "tunnels.searchPlaceholder")}
          bind:value={searchQuery}
          class="h-7 w-full rounded-chip border border-edge bg-base pl-8 pr-2.5 text-xs text-ink placeholder:text-dim-2 focus:border-accent focus:outline-none"
        />
      </div>

      <!-- 类型过滤：统一分段控件样式 -->
      <div class="inline-flex items-center gap-0.5 rounded-chip bg-base p-0.5 text-xs" role="group" aria-label={t(app.language, "tunnels.colType")}>
        <button
          class={`rounded-[3px] px-2 py-1 font-mono text-2xs transition-colors ${typeFilter === 'ALL' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (typeFilter = 'ALL')}
        >
          {t(app.language, "common.all")}
        </button>
        <button
          class={`rounded-[3px] px-2 py-1 font-mono text-2xs transition-colors ${typeFilter === 'L' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (typeFilter = 'L')}
        >
          -L {t(app.language, "tunnels.local")}
        </button>
        <button
          class={`rounded-[3px] px-2 py-1 font-mono text-2xs transition-colors ${typeFilter === 'R' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (typeFilter = 'R')}
        >
          -R {t(app.language, "tunnels.remote")}
        </button>
        <button
          class={`rounded-[3px] px-2 py-1 font-mono text-2xs transition-colors ${typeFilter === 'D' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (typeFilter = 'D')}
        >
          -D SOCKS
        </button>
      </div>

      <!-- 状态过滤 -->
      <div class="inline-flex items-center gap-0.5 rounded-chip bg-base p-0.5 text-xs" role="group" aria-label={t(app.language, "tunnels.colStatus")}>
        <button
          class={`rounded-[3px] px-2 py-1 text-2xs transition-colors ${statusFilter === 'ALL' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (statusFilter = 'ALL')}
        >
          {t(app.language, "tunnels.allStatus")}
        </button>
        <button
          class={`rounded-[3px] px-2 py-1 text-2xs transition-colors ${statusFilter === 'RUNNING' ? 'bg-ok/15 text-ok shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (statusFilter = 'RUNNING')}
        >
          {t(app.language, "tunnels.onlyRunning")}
        </button>
        <button
          class={`rounded-[3px] px-2 py-1 text-2xs transition-colors ${statusFilter === 'STOPPED' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
          onclick={() => (statusFilter = 'STOPPED')}
        >
          {t(app.language, "tunnels.onlyStopped")}
        </button>
      </div>
    </div>

    <!-- 视图切换 -->
    <div class="inline-flex items-center gap-0.5 rounded-chip bg-base p-0.5">
      <button
        class={`rounded-[3px] p-1.5 transition-colors ${viewMode === 'grid' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
        title={t(app.language, "tunnels.cardView")}
        aria-label={t(app.language, "tunnels.cardView")}
        aria-pressed={viewMode === 'grid'}
        onclick={() => (viewMode = 'grid')}
      >
        <LayoutGrid size={13} />
      </button>
      <button
        class={`rounded-[3px] p-1.5 transition-colors ${viewMode === 'table' ? 'bg-panel text-ink shadow-sm' : 'text-dim hover:text-ink'}`}
        title={t(app.language, "tunnels.listView")}
        aria-label={t(app.language, "tunnels.listView")}
        aria-pressed={viewMode === 'table'}
        onclick={() => (viewMode = 'table')}
      >
        <List size={13} />
      </button>
    </div>
  </div>

  <!-- 隧道列表区 -->
  <div class="flex-1 overflow-y-auto p-5">
    {#if app.tunnels.length === 0 && app.loading}
      <!-- 首屏骨架：避免“闪一下空状态” -->
      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 2xl:grid-cols-3">
        {#each [1, 2] as i (i)}
          <div class="rounded-card border border-edge bg-panel p-4">
            <div class="h-3 w-24 animate-pulse rounded bg-hover"></div>
            <div class="mt-3 h-2.5 w-40 animate-pulse rounded bg-hover"></div>
            <div class="mt-2 h-2.5 w-28 animate-pulse rounded bg-hover"></div>
            <div class="mt-4 flex gap-3">
              <div class="h-16 flex-1 animate-pulse rounded-chip bg-hover"></div>
              <div class="h-16 flex-1 animate-pulse rounded-chip bg-hover"></div>
            </div>
          </div>
        {/each}
      </div>
    {:else if app.tunnels.length === 0}
      <div class="flex h-full flex-col items-center justify-center gap-3 text-center text-dim">
        <div class="flex size-12 items-center justify-center rounded-card bg-panel ring-1 ring-edge">
          <ArrowLeftRight size={22} strokeWidth={1.5} class="text-dim-2" />
        </div>
        <p class="max-w-sm text-meta text-dim">{t(app.language, "workbench.empty")}</p>
        <Button variant="primary" size="sm" onclick={() => (app.newTunnelOpen = true)}>
          <Plus size={14} class="mr-1" />
          {t(app.language, "dashboard.new")}
        </Button>
      </div>
    {:else if filteredTunnels.length === 0}
      <div class="flex h-48 flex-col items-center justify-center gap-2 text-dim">
        <Search size={20} strokeWidth={1.5} class="text-dim-2" />
        <p class="text-meta">{t(app.language, "tunnels.noResults")}</p>
      </div>
    {:else if viewMode === "grid"}
      <!-- 卡片栅格视图 -->
      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 2xl:grid-cols-3">
        {#each filteredTunnels as t (t.id)}
          <TunnelCard tunnel={t} />
        {/each}
      </div>
    {:else}
      <!-- 紧凑表格/行列表视图 -->
      <div class="rounded-card border border-edge bg-panel overflow-x-auto">
        <table class="w-full min-w-[850px] text-left text-xs">
          <thead class="border-b border-edge bg-base/70 text-2xs uppercase tracking-wider text-dim">
            <tr>
              <th class="px-3 py-2 font-medium">{t(app.language, "tunnels.colStatus")}</th>
              <th class="px-3 py-2 font-medium">{t(app.language, "tunnels.colType")}</th>
              <th class="px-3 py-2 font-medium">{t(app.language, "tunnels.colName")}</th>
              <th class="px-3 py-2 font-medium">{t(app.language, "tunnels.colRoute")}</th>
              <th class="px-3 py-2 font-medium">{t(app.language, "tunnels.colHost")}</th>
              <th class="px-3 py-2 font-medium">{t(app.language, "tunnels.colTraffic")}</th>
              <th class="sticky right-0 z-10 border-l border-edge bg-base px-3 py-2 font-medium text-right">{t(app.language, "tunnels.colActions")}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-edge/60 font-mono">
            {#each filteredTunnels as tunnel (tunnel.id)}
              <tr class="group transition-colors hover:bg-hover">
                <td class="px-3 py-2">
                  <div class="flex items-center gap-2">
                    <Switch
                      checked={tunnel.running}
                      disabled={!!app.tunnelBusy[tunnel.id]}
                      aria-label={`${t(app.language, "tunnel.toggle")}: ${tunnel.name}`}
                      onCheckedChange={(checked) => handleToggle(tunnel, checked)}
                    />
                    <span class={`size-2 rounded-full ${tunnel.status === 'connected' ? 'bg-ok' : tunnel.status === 'reconnecting' ? 'bg-warn animate-pulse' : 'bg-dim/40'}`}></span>
                  </div>
                </td>
                <td class="px-3 py-2.5">
                  <Badge tone={tunnel.type === 'L' ? 'accent' : tunnel.type === 'R' ? 'warn' : 'ok'} mono>
                    -{tunnel.type}
                  </Badge>
                </td>
                <td class="min-w-36 max-w-56 break-words px-3 py-2 font-sans font-medium text-ink">
                  {tunnel.name}
                  {#if app.tunnelOperationErrors[tunnel.id] || tunnel.error}
                    <div class="mt-1 max-w-[280px] whitespace-normal break-words text-[11px] text-bad">{app.tunnelOperationErrors[tunnel.id] || tunnel.error}</div>
                  {/if}
                </td>
                <td class="min-w-56 max-w-80 break-all px-3 py-2 text-dim">
                  {formatRoute(tunnel)}
                </td>
                <td class="min-w-24 max-w-44 break-words px-3 py-2 text-dim-2 font-sans">
                  {getHostName(tunnel.hostId)}
                </td>
                <td class="whitespace-nowrap px-3 py-2 text-dim">
                  ↑{fmtBytes(tunnel.stats?.tx)} · ↓{fmtBytes(tunnel.stats?.rx)}
                </td>
                <td class="sticky right-0 z-10 border-l border-edge bg-panel px-3 py-2 text-right font-sans group-hover:bg-hover">
                  <div class="inline-flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      class="size-7"
                      aria-label={t(app.language, "tunnel.viewLogs")}
                      title={t(app.language, "tunnel.viewLogs")}
                      onclick={() => openLogs(tunnel)}
                    >
                      <ScrollText size={13} class="text-accent" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      class="size-7"
                      aria-label={t(app.language, "tunnel.copyCli")}
                      title={t(app.language, "tunnel.copyCli")}
                      onclick={() => copyCli(tunnel)}
                    >
                      {#if copiedId === tunnel.id}
                        <Check size={13} class="text-ok" />
                      {:else}
                        <Copy size={13} />
                      {/if}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      class="size-7"
                      aria-label={t(app.language, "tunnel.edit")}
                      title={t(app.language, "tunnel.edit")}
                      disabled={!!app.tunnelBusy[tunnel.id]}
                      onclick={() => openEdit(tunnel)}
                    >
                      <Pencil size={13} />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      class="size-7 text-bad/70 hover:text-bad"
                      aria-label={t(app.language, "hosts.delete")}
                      title={t(app.language, "hosts.delete")}
                      disabled={!!app.tunnelBusy[tunnel.id]}
                      onclick={() => askDelete(tunnel)}
                    >
                      <Trash2 size={13} />
                    </Button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>

<NewTunnelModal bind:open={app.newTunnelOpen} />

{#if activeTunnel}
  <NewTunnelModal bind:open={editModalOpen} tunnelToEdit={activeTunnel} />
  <TunnelLogsModal bind:open={logsModalOpen} tunnel={activeTunnel} />
{/if}

{#if deletingTunnel}
  <ConfirmDialog
    bind:open={deleteConfirmOpen}
    title={t(app.language, "tunnel.delete")}
    message={`${deletingTunnel.name}: ${t(app.language, "tunnel.deleteConfirm")}`}
    confirmLabel={t(app.language, "tunnel.delete")}
    onConfirm={handleDelete}
  />
{/if}

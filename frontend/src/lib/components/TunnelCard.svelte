<script lang="ts">
  import { Trash2, Pencil, Copy, Check, AlertTriangle, ScrollText } from "@lucide/svelte";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Switch from "@/lib/components/ui/Switch.svelte";
  import Sparkline from "@/lib/components/Sparkline.svelte";
  import NewTunnelModal from "@/lib/components/NewTunnelModal.svelte";
  import TunnelLogsModal from "@/lib/components/TunnelLogsModal.svelte";
  import { buildTunnelCliCommand, copyText, formatTunnelRoute } from "@/lib/tunnel-command";
  import {
    app,
    startTunnel,
    stopTunnel,
    deleteTunnel,
    errorMessage,
  } from "@/lib/state.svelte";
  import type { Tunnel } from "@/lib/api";
  import { t } from "@/lib/i18n";

  let { tunnel }: { tunnel: Tunnel } = $props();
  let editModalOpen = $state(false);
  let logsModalOpen = $state(false);

  const typeBadge: Record<string, { label: string; tone: "accent" | "warn" | "ok" }> = {
    L: { label: "-L", tone: "accent" },
    R: { label: "-R", tone: "warn" },
    D: { label: "-D", tone: "ok" },
  };

  const route = $derived.by(() => {
    return formatTunnelRoute(tunnel);
  });

  const connected = $derived(tunnel.status === "connected");
  const hostObj = $derived(app.hosts.find((h) => h.id === tunnel.hostId));
  const hostName = $derived(hostObj?.name ?? tunnel.hostId);
  const isTargetSameAsHost = $derived(
    tunnel.type === "L" && !tunnel.targetSocket && !!hostObj?.host && !/^(localhost|127\..*|\[?::1\]?)$/i.test(hostObj.host.trim()) && (tunnel.targetHost?.trim() === hostObj.host.trim())
  );
  let copied = $state(false);
  let actionError = $state("");
  const operationError = $derived(actionError || app.tunnelOperationErrors[tunnel.id] || tunnel.error);

  async function copyCliCommand() {
    actionError = "";
    try {
      const cmd = buildTunnelCliCommand(tunnel, hostObj, { hosts: app.hosts, useAppDefaults: true });
      if (await copyText(cmd)) {
        copied = true;
        setTimeout(() => (copied = false), 1500);
      } else actionError = t(app.language, "tunnel.copyError");
    } catch (error) {
      actionError = errorMessage(error);
    }
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

  function fmtRate(n?: number): string {
    if (!n) return "0 B/s";
    return `${fmtBytes(n)}/s`;
  }

  async function toggle(checked: boolean) {
    if (app.tunnelBusy[tunnel.id]) return;
    actionError = "";
    try {
      if (checked) await startTunnel(tunnel.id);
      else await stopTunnel(tunnel.id);
    } catch (error) {
      actionError = errorMessage(error);
    }
  }

  async function handleDelete() {
    if (app.tunnelBusy[tunnel.id]) return;
    if (typeof window !== "undefined" && !window.confirm(t(app.language, "tunnel.deleteConfirm"))) {
      return;
    }
    actionError = "";
    try {
      await deleteTunnel(tunnel.id);
    } catch (error) {
      actionError = errorMessage(error);
    }
  }
</script>

<div
  class="flex flex-col rounded-card border border-edge bg-panel p-4 transition-colors hover:border-dim/40"
  data-testid="tunnel-card"
>
  <div class="mb-3 flex items-start justify-between gap-3">
    <div class="min-w-0">
      <div class="flex items-center gap-2">
        <span class="truncate text-sm font-medium text-ink">{tunnel.name}</span>
        <Badge tone={typeBadge[tunnel.type]?.tone ?? "dim"} mono>
          {typeBadge[tunnel.type]?.label ?? tunnel.type}
        </Badge>
      </div>
      <div class="mt-1 truncate font-mono text-xs text-dim">{route}</div>
      <div class="mt-0.5 truncate text-[11px] text-dim/70">{t(app.language, "common.via")} {hostName}</div>
    </div>

    <!-- 启停开关（只用 switch，无叠加对勾） -->
    <Switch checked={tunnel.running} disabled={!!app.tunnelBusy[tunnel.id]} aria-label={`${t(app.language, "tunnel.toggle")}: ${tunnel.name}`} onCheckedChange={toggle} />
  </div>

  <!-- 状态行 -->
  <div class="mb-3 flex items-center gap-1.5 text-xs">
    <span
      class={`size-1.5 rounded-full ${
        connected ? "bg-ok" : tunnel.status === "reconnecting" ? "bg-warn animate-pulse" : "bg-dim/50"
      }`}
    ></span>
    <span class={connected ? "text-ok" : tunnel.status === "reconnecting" ? "text-warn" : "text-dim"}>
      {#if connected}
        {t(app.language, "status.connected")}
      {:else if tunnel.status === "reconnecting"}
        {t(app.language, "status.reconnecting")}
      {:else if tunnel.status === "starting"}
        {t(app.language, "status.connecting")}
      {:else if tunnel.status === "error"}
        {t(app.language, "status.error")}
      {:else}
        {t(app.language, "status.stopped")}
      {/if}
    </span>
    {#if operationError}
      <span role="alert" class="truncate font-mono text-[11px] text-bad/80" title={operationError}>{operationError}</span>
    {/if}
  </div>

  {#if isTargetSameAsHost}
    <div class="mb-3 flex items-center justify-between rounded bg-warn/10 border border-warn/30 px-2.5 py-1.5 text-[11px] text-warn">
      <span class="flex items-center gap-1">
        <AlertTriangle size={12} class="shrink-0" />
        {t(app.language, "tunnel.targetHostWarning")}
      </span>
      <button
        type="button"
        class="ml-1 underline hover:text-ink font-medium shrink-0"
        onclick={() => (editModalOpen = true)}
      >
        {t(app.language, "tunnel.fixLoopback")}
      </button>
    </div>
  {/if}

  <!-- Tx/Rx + sparkline -->
  <div class="grid grid-cols-2 gap-3">
    <div class="rounded-chip bg-base px-2.5 py-2">
      <div class="flex items-baseline justify-between">
        <span class="text-[11px] uppercase tracking-wide text-dim">Tx</span>
        <span class="font-mono text-[11px] text-accent">{fmtRate(app.txHistory[tunnel.id]?.at(-1))}</span>
      </div>
      <div class="font-mono text-sm text-ink">{fmtBytes(tunnel.stats?.tx ?? 0)}</div>
      <div class="mt-1">
        <Sparkline data={app.txHistory[tunnel.id] ?? []} color="var(--c-accent)" />
      </div>
    </div>
    <div class="rounded-chip bg-base px-2.5 py-2">
      <div class="flex items-baseline justify-between">
        <span class="text-[11px] uppercase tracking-wide text-dim">Rx</span>
        <span class="font-mono text-[11px] text-ok">{fmtRate(app.rxHistory[tunnel.id]?.at(-1))}</span>
      </div>
      <div class="font-mono text-sm text-ink">{fmtBytes(tunnel.stats?.rx ?? 0)}</div>
      <div class="mt-1">
        <Sparkline data={app.rxHistory[tunnel.id] ?? []} color="var(--c-ok)" />
      </div>
    </div>
  </div>

  <div class="mt-3 flex items-center justify-between border-t border-edge pt-2.5">
    <span class="font-mono text-[11px] text-dim/70">
        {tunnel.stats?.activeConn ?? 0} {t(app.language, "common.active")} · {tunnel.stats?.totalConn ?? 0} {t(app.language, "common.total")}
    </span>
    <div class="flex items-center gap-1">
      <Button variant="ghost" size="icon" aria-label={t(app.language, "tunnel.viewLogs")} title={t(app.language, "tunnel.viewLogs")} onclick={() => (logsModalOpen = true)}>
        <ScrollText size={14} class="text-accent" />
      </Button>
      <Button variant="ghost" size="icon" aria-label={t(app.language, "tunnel.copyCli")} title={t(app.language, "tunnel.copyCli")} onclick={copyCliCommand}>
        {#if copied}
          <Check size={14} class="text-ok" />
        {:else}
          <Copy size={14} />
        {/if}
      </Button>
      <Button variant="ghost" size="icon" disabled={!!app.tunnelBusy[tunnel.id]} aria-label={t(app.language, "tunnel.edit")} title={t(app.language, "tunnel.edit")} onclick={() => (editModalOpen = true)}>
        <Pencil size={14} />
      </Button>
      <Button variant="ghost" size="icon" disabled={!!app.tunnelBusy[tunnel.id]} aria-label={t(app.language, "tunnel.delete")} title={t(app.language, "tunnel.delete")} onclick={handleDelete}>
        <Trash2 size={14} />
      </Button>
    </div>
  </div>
</div>

<NewTunnelModal bind:open={editModalOpen} tunnelToEdit={tunnel} />
<TunnelLogsModal bind:open={logsModalOpen} {tunnel} />

<script lang="ts">
  import { ScrollText, Copy, Check, ArrowDown, ExternalLink } from "@lucide/svelte";
  import Dialog from "@/lib/components/ui/Dialog.svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import { app } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";
  import type { Tunnel } from "@/lib/api";

  let {
    open = $bindable(false),
    tunnel,
  }: {
    open?: boolean;
    tunnel: Tunnel;
  } = $props();

  let follow = $state(true);
  let copied = $state(false);
  let logContainer = $state<HTMLElement | null>(null);

  const tunnelLogs = $derived(
    app.logs.filter((l) => l.tunnelId === tunnel.id)
  );

  function formatTime(iso: string): string {
    try {
      const d = new Date(iso);
      if (isNaN(d.getTime())) return iso;
      return d.toTimeString().slice(0, 8);
    } catch {
      return iso;
    }
  }

  function getLogLevel(message: string): "error" | "ok" | "accent" | "dim" {
    if (/failed|error|refused|lost|abnormally/i.test(message)) return "error";
    if (/connected successfully|connection active/i.test(message)) return "ok";
    if (/starting|inbound connection|socks5/i.test(message)) return "accent";
    return "dim";
  }

  function copyLogs() {
    if (tunnelLogs.length === 0) return;
    const text = tunnelLogs
      .map((l) => `[${l.time}] [${tunnel.name}] ${l.message}`)
      .join("\n");
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      navigator.clipboard.writeText(text);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    }
  }

  function goToAllLogs() {
    open = false;
    app.page = "logs";
  }

  $effect(() => {
    if (follow && tunnelLogs.length && logContainer) {
      logContainer.scrollTop = 0;
    }
  });
</script>

<Dialog
  bind:open
  title={`${t(app.language, "logs.tunnelLogTitle")} · ${tunnel.name}`}
  description={`ID: ${tunnel.id} · ${t(app.language, "host.port")}: ${tunnel.type === 'D' ? tunnel.socksPort : tunnel.localPort || tunnel.remotePort}`}
>
  <div class="space-y-3">
    <!-- 头部小工具栏 -->
    <div class="flex items-center justify-between text-xs border-b border-edge/60 pb-2">
      <div class="flex items-center gap-2">
        <Badge tone={tunnel.status === 'connected' ? 'ok' : tunnel.status === 'reconnecting' ? 'warn' : 'dim'}>
          {tunnel.status}
        </Badge>
        <span class="text-dim">{tunnelLogs.length} {t(app.language, "logs.records")}</span>
      </div>

      <div class="flex items-center gap-1.5">
        <button
          class={`flex items-center gap-1 rounded px-2 py-0.5 text-[11px] transition-colors border ${
            follow ? "border-accent/40 bg-accent/10 text-accent" : "border-edge text-dim hover:text-ink"
          }`}
          onclick={() => (follow = !follow)}
        >
          <ArrowDown size={11} />
          <span>{t(app.language, "logs.follow")}</span>
        </button>

        <button
          class="flex items-center gap-1 rounded border border-edge bg-base px-2 py-0.5 text-[11px] text-dim hover:text-ink transition-colors"
          onclick={copyLogs}
          disabled={tunnelLogs.length === 0}
        >
          {#if copied}
            <Check size={11} class="text-ok" />
            <span class="text-ok">{t(app.language, "common.copied")}</span>
          {:else}
            <Copy size={11} />
            <span>{t(app.language, "logs.copyAll")}</span>
          {/if}
        </button>

        <button
          class="flex items-center gap-1 rounded border border-edge bg-base px-2 py-0.5 text-[11px] text-dim hover:text-accent transition-colors"
          onclick={goToAllLogs}
          title={t(app.language, "logs.fullscreen")}
        >
          <ExternalLink size={11} />
          <span>{t(app.language, "logs.fullscreen")}</span>
        </button>
      </div>
    </div>

    <!-- 日志流区域 -->
    <div
      class="max-h-[380px] min-h-[160px] overflow-y-auto rounded-chip border border-edge bg-base/90 p-3"
      bind:this={logContainer}
    >
      {#if tunnelLogs.length === 0}
        <div class="flex h-36 flex-col items-center justify-center gap-2 text-dim">
          <ScrollText size={24} class="opacity-40" />
          <p class="text-xs">{t(app.language, "logs.tunnelEmpty")}</p>
        </div>
      {:else}
        <ul class="space-y-1.5 font-mono text-xs">
          {#each tunnelLogs as log (log.id)}
            {@const lvl = getLogLevel(log.message)}
            <li class="flex items-baseline gap-2.5 rounded px-1.5 py-0.5 hover:bg-hover transition-colors">
              <span class="shrink-0 text-[11px] text-dim/60 font-mono" title={log.time}>
                {formatTime(log.time)}
              </span>
              <span
                class={`break-all whitespace-pre-wrap ${
                  lvl === "error"
                    ? "text-bad font-medium"
                    : lvl === "ok"
                    ? "text-ok"
                    : lvl === "accent"
                    ? "text-accent"
                    : "text-ink"
                }`}
              >
                {log.message}
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)}>{t(app.language, "common.close")}</Button>
  {/snippet}
</Dialog>

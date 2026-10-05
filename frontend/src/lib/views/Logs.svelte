<script lang="ts">
  import { ScrollText, Trash2, ArrowDown, Search, Copy, Check } from "@lucide/svelte";
  import { app, clearLogs } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";
  import Button from "@/lib/components/ui/Button.svelte";
  import Select from "@/lib/components/ui/Select.svelte";
  import { copyText } from "@/lib/tunnel-command";

  let follow = $state(true);
  let copied = $state(false);
  let logContainer = $state<HTMLElement | null>(null);
  let selectedTunnelId = $state("ALL");
  let searchKeyword = $state("");

  const tunnelOptions = $derived([
    { value: "ALL", label: t(app.language, "logs.allTunnels") },
    ...app.tunnels.map((t) => ({ value: t.id, label: `${t.name} (${t.id})` })),
  ]);

  const filteredLogs = $derived.by(() => {
    let list = app.logs;
    if (selectedTunnelId !== "ALL") {
      list = list.filter((l) => l.tunnelId === selectedTunnelId);
    }
    const q = searchKeyword.trim().toLowerCase();
    if (q) {
      list = list.filter((l) =>
        l.message.toLowerCase().includes(q) ||
        (l.tunnelId?.toLowerCase().includes(q) ?? false) ||
        l.time.includes(q)
      );
    }
    return list;
  });

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
    if (/failed|error|refused|lost|abnormally|timeout/i.test(message)) return "error";
    if (/connected successfully|connection active/i.test(message)) return "ok";
    if (/starting|inbound connection|socks5/i.test(message)) return "accent";
    return "dim";
  }

  async function copyFiltered() {
    if (filteredLogs.length === 0) return;
    const text = filteredLogs
      .map((l) => {
        const tunName = app.tunnels.find((t) => t.id === l.tunnelId)?.name ?? l.tunnelId ?? "system";
        return `[${l.time}] [${tunName}] ${l.message}`;
      })
      .join("\n");
    try {
      if (await copyText(text)) {
        copied = true;
        setTimeout(() => (copied = false), 1500);
      }
    } catch (error) {
      console.error("Copy logs failed:", error);
    }
  }

  $effect(() => {
    if (follow && filteredLogs.length && logContainer) {
      logContainer.scrollTop = 0;
    }
  });
</script>

<div class="flex h-full flex-col">
  <header class="flex h-14 shrink-0 items-center justify-between border-b border-edge px-5">
    <div class="flex items-center gap-3">
      <h1 class="text-[15px] font-semibold text-ink">{t(app.language, "logs.title")}</h1>
      <span class="rounded-chip bg-panel border border-edge px-2 py-0.5 font-mono text-[11px] text-dim">
        {filteredLogs.length} / {app.logs.length}
      </span>
    </div>
    <div class="flex items-center gap-2">
      <button
        class={`flex items-center gap-1.5 rounded-chip px-2.5 py-1 text-xs transition-colors border ${
          follow ? "border-accent/40 bg-accent/10 text-accent" : "border-edge text-dim hover:text-ink"
        }`}
        onclick={() => (follow = !follow)}
      >
        <ArrowDown size={12} />
        {t(app.language, "logs.follow")}
      </button>

      <button
        class="flex items-center gap-1 rounded border border-edge bg-panel px-2.5 py-1 text-xs text-dim hover:text-ink hover:bg-hover transition-colors"
        onclick={copyFiltered}
        disabled={filteredLogs.length === 0}
      >
        {#if copied}
          <Check size={12} class="text-ok" />
          <span class="text-ok">{t(app.language, "common.copied")}</span>
        {:else}
          <Copy size={12} />
          <span>{t(app.language, "logs.copyAll")}</span>
        {/if}
      </button>

      {#if app.logs.length > 0}
        <Button variant="ghost" size="sm" onclick={clearLogs}>
          <Trash2 size={13} class="mr-1" />
          {t(app.language, "logs.clear")}
        </Button>
      {/if}
    </div>
  </header>

  <!-- 日志过滤栏 -->
  <div class="flex items-center gap-3 border-b border-edge/60 bg-base/60 px-5 py-2">
    <div class="w-64">
      <Select bind:value={selectedTunnelId} options={tunnelOptions} />
    </div>

    <div class="relative flex-1 max-w-sm">
      <Search size={13} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-dim" />
      <input
        type="text"
        placeholder={t(app.language, "logs.searchPlaceholder")}
        bind:value={searchKeyword}
        class="h-8 w-full rounded-chip border border-edge bg-base pl-8 pr-2.5 text-xs text-ink placeholder:text-dim/60 focus:border-accent focus:outline-none"
      />
    </div>
  </div>

  <!-- 日志流列表 -->
  <div class="flex-1 overflow-y-auto p-5" bind:this={logContainer}>
    {#if filteredLogs.length === 0}
      <div class="flex h-full flex-col items-center justify-center gap-3 text-dim">
        <ScrollText size={32} class="opacity-40" />
        <p class="text-sm">{t(app.language, "logs.noMatchingLogs")}</p>
      </div>
    {:else}
      <ul class="space-y-1 font-mono text-xs">
        {#each filteredLogs as log (log.id)}
          {@const lvl = getLogLevel(log.message)}
          {@const tunName = app.tunnels.find((t) => t.id === log.tunnelId)?.name}
          <li class="flex items-baseline gap-2.5 rounded-chip px-2.5 py-1.5 hover:bg-hover transition-colors">
            <span class="shrink-0 font-mono text-[11px] text-dim/60" title={log.time}>
              {formatTime(log.time)}
            </span>
            {#if log.tunnelId}
              <span class="shrink-0 rounded bg-edge/40 px-1.5 py-0.5 text-[10px] text-accent">
                {tunName ? `${tunName}` : log.tunnelId}
              </span>
            {/if}
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

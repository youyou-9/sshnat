<script lang="ts">
  import {
    LayoutDashboard,
    Server,
    ArrowLeftRight,
    ScrollText,
    Settings,
    ArrowUp,
    ArrowDown,
  } from "@lucide/svelte";
  import { app, type Page } from "@/lib/state.svelte";
  import { SettingsService, type AppInfo } from "@/lib/api";
  import { t } from "@/lib/i18n";

  let { class: className = "" }: { class?: string } = $props();
  let appInfo = $state<AppInfo | null>(null);

  $effect(() => {
    SettingsService.Get()
      .then((info) => {
        if (info) appInfo = info;
      })
      .catch(() => {});
  });

  const items: { id: Page; key: string; icon: typeof LayoutDashboard }[] = [
    { id: "dashboard", key: "nav.dashboard", icon: LayoutDashboard },
    { id: "hosts", key: "nav.hosts", icon: Server },
    { id: "tunnels", key: "nav.tunnels", icon: ArrowLeftRight },
    { id: "logs", key: "nav.logs", icon: ScrollText },
    { id: "settings", key: "nav.settings", icon: Settings },
  ];

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
</script>

<aside
  class={`flex w-[68px] shrink-0 flex-col border-r border-edge bg-base sm:w-[240px] ${className}`}
  data-testid="sidebar"
>
  <div class="flex h-14 items-center justify-center gap-2.5 border-b border-edge px-2 sm:justify-start sm:px-4">
    <div class="flex size-7 items-center justify-center rounded-chip bg-accent/15">
      <ArrowLeftRight size={16} class="text-accent" />
    </div>
    <div class="hidden items-baseline gap-2 sm:flex">
      <span class="text-[15px] font-semibold tracking-tight text-ink">SSHNat</span>
      {#if appInfo?.version}
        <span class="rounded bg-accent/15 px-1.5 py-0.5 font-mono text-[10px] font-bold text-accent">v{appInfo.version}</span>
      {/if}
    </div>
  </div>

  <nav class="flex-1 space-y-0.5 overflow-y-auto p-2">
    {#each items as item (item.id)}
      <button
        class={`flex w-full items-center justify-center gap-2.5 rounded-chip px-2.5 py-2 text-sm transition-colors sm:justify-start
                ${
                  app.page === item.id
                    ? "bg-hover text-accent"
                    : "text-dim hover:bg-hover hover:text-ink"
                }`}
        title={t(app.language, item.key)}
        onclick={() => (app.page = item.id)}
      >
        <item.icon size={16} />
        <span class="hidden sm:inline">{t(app.language, item.key)}</span>
      </button>
    {/each}
  </nav>

  <!-- 底部总上传/下载速率 -->
  <div class="hidden border-t border-edge p-3 sm:block">
    <div class="mb-2 text-[11px] font-medium uppercase tracking-wider text-dim">{t(app.language, "nav.throughput")}</div>
    <div class="grid grid-cols-2 gap-2">
      <div class="rounded-chip border border-edge bg-panel px-2 py-1.5">
        <div class="flex items-center gap-1 text-[11px] text-dim">
          <ArrowUp size={11} class="text-accent" /> {t(app.language, "nav.tx")}
        </div>
        <div class="font-mono text-xs text-ink">{fmtRate(app.totalTx)}</div>
      </div>
      <div class="rounded-chip border border-edge bg-panel px-2 py-1.5">
        <div class="flex items-center gap-1 text-[11px] text-dim">
          <ArrowDown size={11} class="text-ok" /> {t(app.language, "nav.rx")}
        </div>
        <div class="font-mono text-xs text-ink">{fmtRate(app.totalRx)}</div>
      </div>
    </div>
  </div>
</aside>

<script lang="ts">
  import { ChevronDown, ChevronUp, SquareTerminal, X } from "@lucide/svelte";
  import TerminalPane from "@/lib/components/TerminalPane.svelte";
  import { app, closeTerminalTab } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  let collapsed = $state(false);
  let maximized = $state(false);

  const activeTab = $derived(app.terminals.find((tab) => tab.id === app.activeTerminalId) ?? app.terminals.at(-1));
  const hostOf = (hostId: string) => app.hosts.find((h) => h.id === hostId);
  // 折叠优先级最高：最大化状态下折叠不能保留 70vh 死区。
  const dockHeight = $derived(collapsed ? "36px" : maximized ? "70vh" : "320px");
</script>

{#if app.terminals.length > 0}
  <section
    class="flex shrink-0 flex-col border-t border-edge bg-panel"
    style="height: {dockHeight}"
    data-testid="terminal-dock"
    aria-label={t(app.language, "terminal.title")}
  >
    <!-- 标签栏 -->
    <div class="flex h-9 shrink-0 items-center justify-between border-b border-edge bg-base/60 px-2">
      <div class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
        {#each app.terminals as tab (tab.id)}
          <div
            class="group flex max-w-56 items-center gap-1.5 rounded-chip px-2 py-1 text-xs transition-colors
              {tab.id === activeTab?.id ? 'bg-panel text-ink shadow-[inset_0_-2px_0_0_var(--c-accent)]' : 'text-dim hover:bg-hover hover:text-ink'}"
          >
            <button
              class="flex min-w-0 items-center gap-1.5"
              title={hostOf(tab.hostId)?.name ?? tab.subtitle}
              onclick={() => { app.activeTerminalId = tab.id; collapsed = false; }}
            >
              <SquareTerminal size={12} class="shrink-0" />
              <!-- 标题实时取主机名，主机改名后标签同步 -->
              <span class="truncate">{hostOf(tab.hostId)?.name ?? tab.title}</span>
            </button>
            <button
              class="shrink-0 rounded p-0.5 opacity-50 transition-opacity hover:opacity-100"
              aria-label={`${t(app.language, "terminal.close")}: ${tab.title}`}
              title={t(app.language, "terminal.close")}
              onclick={(e) => { e.stopPropagation(); closeTerminalTab(tab.id); }}
            >
              <X size={11} />
            </button>
          </div>
        {/each}
      </div>
      <div class="flex shrink-0 items-center gap-0.5">
        <button
          class="rounded-chip p-1 text-dim transition-colors hover:bg-hover hover:text-ink"
          title={t(app.language, "terminal.toggleSize")}
          aria-label={t(app.language, "terminal.toggleSize")}
          onclick={() => { maximized = !maximized; if (collapsed) collapsed = false; }}
        >
          {#if maximized}<ChevronDown size={14} />{:else}<ChevronUp size={14} />{/if}
        </button>
        <button
          class="rounded-chip p-1 text-dim transition-colors hover:bg-hover hover:text-ink"
          title={t(app.language, "terminal.collapse")}
          aria-label={t(app.language, "terminal.collapse")}
          onclick={() => (collapsed = !collapsed)}
        >
          {#if collapsed}<ChevronUp size={14} />{:else}<ChevronDown size={14} />{/if}
        </button>
      </div>
    </div>

    <!-- 会话面板：始终挂载（折叠也只是隐藏），避免杀会话/丢 scrollback -->
    <div class="relative min-h-0 flex-1 overflow-hidden" class:hidden={collapsed}>
      {#each app.terminals as tab (tab.id)}
        {@const host = hostOf(tab.hostId)}
        <div class="absolute inset-0" class:hidden={tab.id !== activeTab?.id || collapsed}>
          {#if host}
            <TerminalPane {host} active={tab.id === activeTab?.id && !collapsed} />
          {:else}
            <div class="flex h-full items-center justify-center text-xs text-dim">{t(app.language, "terminal.hostMissing")}</div>
          {/if}
        </div>
      {/each}
    </div>
  </section>
{/if}

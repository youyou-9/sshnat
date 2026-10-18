<script lang="ts">
  import {
    Plus,
    Play,
    Square,
    LayoutDashboard,
    Server,
    ScrollText,
    Settings as SettingsIcon,
    SquareTerminal,
    CornerDownLeft,
    Search,
  } from "@lucide/svelte";
  import { app, gotoPage, openTerminal, pushToast, setAllTunnelsRunning, startTunnel, stopTunnel } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  type Action = {
    id: string;
    label: string;
    hint?: string;
    icon: any;
    run: () => void | Promise<void>;
  };

  // 导航项随语言切换实时更新，因此每次派生都重新构建。
  function navActions(): Action[] {
    return [
      { id: "nav-workbench", label: t(app.language, "nav.workbench"), hint: "g w", icon: LayoutDashboard, run: () => { gotoPage("workbench"); } },
      { id: "nav-hosts", label: t(app.language, "nav.hosts"), hint: "g h", icon: Server, run: () => { gotoPage("hosts"); } },
      { id: "nav-logs", label: t(app.language, "nav.logs"), hint: "g l", icon: ScrollText, run: () => { gotoPage("logs"); } },
      { id: "nav-settings", label: t(app.language, "nav.settings"), hint: "g s", icon: SettingsIcon, run: () => { gotoPage("settings"); } },
    ];
  }

  const actions = $derived<Action[]>([
    { id: "new-tunnel", label: t(app.language, "palette.newTunnel"), icon: Plus, run: () => { gotoPage("workbench"); app.newTunnelOpen = true; } },
    { id: "start-all", label: t(app.language, "common.startAll"), icon: Play, run: () => batch(true) },
    { id: "stop-all", label: t(app.language, "common.stopAll"), icon: Square, run: () => batch(false) },
    ...navActions(),
    ...app.hosts.map<Action>((host) => ({
      id: `term-${host.id}`,
      label: `${t(app.language, "terminal.open")} · ${host.name}`,
      hint: `${host.user}@${host.host}`,
      icon: SquareTerminal,
      run: () => openTerminal(host),
    })),
    ...app.tunnels.flatMap<Action>((tunnel) => [
      {
        id: `toggle-${tunnel.id}`,
        label: `${tunnel.running ? t(app.language, "tunnel.stop") : t(app.language, "tunnel.start")} · ${tunnel.name}`,
        hint: tunnel.type === "L" ? `-L :${tunnel.localPort}` : tunnel.type === "R" ? `-R :${tunnel.remotePort}` : `-D :${tunnel.socksPort}`,
        icon: tunnel.running ? Square : Play,
        run: async () => {
          try {
            if (tunnel.running) await stopTunnel(tunnel.id);
            else await startTunnel(tunnel.id);
          } catch (e) {
            pushToast("error", String(e instanceof Error ? e.message : e));
          }
        },
      },
    ]),
  ]);

  async function batch(running: boolean) {
    const failures = await setAllTunnelsRunning(running);
    if (failures.length) pushToast("error", failures.map((f) => `${f.name}: ${f.error}`).join("\n"));
  }

  let query = $state("");
  let selected = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);
  let lastFocused: HTMLElement | null = null;

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return actions.slice(0, 12);
    return actions
      .filter((a) => a.label.toLowerCase().includes(q) || (a.hint ?? "").toLowerCase().includes(q))
      .slice(0, 12);
  });

  $effect(() => {
    if (app.paletteOpen) {
      lastFocused = typeof document !== "undefined" ? (document.activeElement as HTMLElement | null) : null;
      query = "";
      selected = 0;
      setTimeout(() => inputEl?.focus(), 20);
    } else {
      // 关闭后把焦点还给打开前的元素，键盘用户不会掉到页面开头。
      lastFocused?.focus?.();
      lastFocused = null;
    }
  });

  $effect(() => {
    if (selected >= filtered.length) selected = Math.max(0, filtered.length - 1);
  });

  function close() {
    app.paletteOpen = false;
  }

  async function runAction(action: Action | undefined) {
    if (!action) return;
    close();
    await action.run();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      selected = Math.min(selected + 1, Math.max(0, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      selected = Math.max(selected - 1, 0);
    } else if (e.key === "Enter") {
      e.preventDefault();
      runAction(filtered[selected]);
    } else if (e.key === "Escape") {
      e.preventDefault();
      close();
    }
  }
</script>

{#if app.paletteOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-[70] flex items-start justify-center bg-black/50 pt-[12vh] backdrop-blur-sm" role="presentation" onclick={close}>
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div
      class="w-[min(560px,92vw)] overflow-hidden rounded-card border border-edge bg-panel shadow-[var(--shadow-pop)]"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      aria-label={t(app.language, "palette.title")}
      onkeydown={onKeydown}
      onclick={(e) => e.stopPropagation()}
    >
      <div class="flex items-center gap-2 border-b border-edge px-3 py-2.5">
        <Search size={14} class="text-dim" />
        <!-- keydown 只在弹层根节点处理一次，输入框冒泡即可，避免方向键/回车被执行两次 -->
        <input
          bind:this={inputEl}
          bind:value={query}
          placeholder={t(app.language, "palette.placeholder")}
          class="min-w-0 flex-1 bg-transparent text-sm text-ink placeholder:text-dim-2 focus:outline-none"
          spellcheck="false"
        />
        <kbd class="rounded border border-edge px-1.5 py-0.5 font-mono text-[10px] text-dim">Esc</kbd>
      </div>
      <ul class="max-h-[45vh] overflow-y-auto p-1.5">
        {#each filtered as action, index (action.id)}
          <li>
            <button
              class="flex w-full items-center gap-2.5 rounded-chip px-2.5 py-2 text-left text-sm transition-colors
                {index === selected ? 'bg-hover text-ink' : 'text-dim hover:bg-hover/60 hover:text-ink'}"
              onclick={() => runAction(action)}
              onmouseenter={() => (selected = index)}
            >
              <action.icon size={14} class={index === selected ? "text-accent" : "text-dim"} />
              <span class="min-w-0 flex-1 truncate">{action.label}</span>
              {#if action.hint}<span class="shrink-0 font-mono text-[10px] text-dim-2">{action.hint}</span>{/if}
              {#if index === selected}<CornerDownLeft size={12} class="shrink-0 text-dim" />{/if}
            </button>
          </li>
        {:else}
          <li class="px-3 py-6 text-center text-xs text-dim">{t(app.language, "palette.empty")}</li>
        {/each}
      </ul>
    </div>
  </div>
{/if}

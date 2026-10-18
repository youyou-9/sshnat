<script lang="ts">
  import { onMount } from "svelte";
  import Sidebar from "@/lib/components/Sidebar.svelte";
  import Toasts from "@/lib/components/Toasts.svelte";
  import TerminalDock from "@/lib/components/TerminalDock.svelte";
  import CommandPalette from "@/lib/components/CommandPalette.svelte";
  import Workbench from "@/lib/views/Workbench.svelte";
  import Hosts from "@/lib/views/Hosts.svelte";
  import Logs from "@/lib/views/Logs.svelte";
  import Settings from "@/lib/views/Settings.svelte";
  import { app, initApp, anyDialogOpen, gotoPage, type Page } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  let ready = $state(false);

  onMount(() => {
    let cleanup = () => {};
    initApp()
      .then((c) => {
        cleanup = c;
        ready = true;
      })
      .catch((err) => {
        console.error("SSHNat init error:", err);
        ready = true;
      });
    return () => cleanup();
  });

  // 页面路由（无路由库，runes 驱动）。
  function pageComponent(p: Page) {
    switch (p) {
      case "workbench":
        return Workbench;
      case "hosts":
        return Hosts;
      case "logs":
        return Logs;
      case "settings":
        return Settings;
    }
  }

  const CurrentPage = $derived(pageComponent(app.page)!);

  // 键盘：Ctrl/Cmd+K 命令面板；g+w/h/l/s 页面跳转（输入框内不触发）。
  let pendingG = false;

  function isTypingTarget(target: EventTarget | null): boolean {
    const el = target as HTMLElement | null;
    if (!el) return false;
    const tag = el.tagName?.toLowerCase();
    return tag === "input" || tag === "textarea" || tag === "select" || el.isContentEditable;
  }

  function onKeydown(e: KeyboardEvent) {
    // Ctrl/Cmd+K 打开命令面板；输入框/终端内不抢键，AltGr 与输入法组字也跳过。
    if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.isComposing && e.key.toLowerCase() === "k") {
      if (isTypingTarget(e.target) && !app.paletteOpen) return;
      // 已有模态框时不再叠一层命令面板（避免面板导航卸载弹层丢失未保存输入）。
      if (anyDialogOpen() && !app.paletteOpen) return;
      e.preventDefault();
      app.paletteOpen = !app.paletteOpen;
      return;
    }
    if (app.paletteOpen) return;
    // 任何模态框打开时不响应页面跳转序列，避免 {#key app.page} 卸载弹层。
    if (anyDialogOpen()) return;
    if (isTypingTarget(e.target)) return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    if (pendingG) {
      pendingG = false;
      const map: Record<string, Page> = { w: "workbench", h: "hosts", l: "logs", s: "settings" };
      const page = map[e.key.toLowerCase()];
      if (page) {
        e.preventDefault();
        gotoPage(page);
      }
      return;
    }
    if (e.key.toLowerCase() === "g") {
      pendingG = true;
      setTimeout(() => (pendingG = false), 900);
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if ready}
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-base">
    <div class="flex min-h-0 flex-1">
      <Sidebar />
      <main class="min-w-0 flex-1">
        <!-- 页面切换轻淡入，低成本提升质感 -->
        {#key app.page}
          <div class="page-fade h-full">
            <CurrentPage />
          </div>
        {/key}
      </main>
    </div>
    <!-- 底部多终端 Dock：跨页面常驻 -->
    <TerminalDock />
    <Toasts />
    <CommandPalette />
  </div>
{:else}
  <div class="flex h-screen w-screen items-center justify-center bg-base text-dim">
    <span class="font-mono text-sm">{t(app.language, "loading.text")}</span>
  </div>
{/if}

<style>
  .page-fade {
    animation: page-in 120ms ease-out;
  }
  @keyframes page-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
</style>

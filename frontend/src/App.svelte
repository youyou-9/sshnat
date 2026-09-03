<script lang="ts">
  import { onMount } from "svelte";
  import Sidebar from "@/lib/components/Sidebar.svelte";
  import Dashboard from "@/lib/views/Dashboard.svelte";
  import Hosts from "@/lib/views/Hosts.svelte";
  import Tunnels from "@/lib/views/Tunnels.svelte";
  import Logs from "@/lib/views/Logs.svelte";
  import Settings from "@/lib/views/Settings.svelte";
  import { app, initApp, type Page } from "@/lib/state.svelte";
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
      case "dashboard":
        return Dashboard;
      case "hosts":
        return Hosts;
      case "tunnels":
        return Tunnels;
      case "logs":
        return Logs;
      case "settings":
        return Settings;
    }
  }

  const CurrentPage = $derived(pageComponent(app.page)!);
</script>

{#if ready}
  <div class="flex h-screen w-screen overflow-hidden bg-base">
    <Sidebar />
    <main class="min-w-0 flex-1">
      <CurrentPage />
    </main>
  </div>
{:else}
  <div class="flex h-screen w-screen items-center justify-center bg-base text-dim">
    <span class="font-mono text-sm">{t(app.language, "loading.text")}</span>
  </div>
{/if}

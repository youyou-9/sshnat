<script lang="ts">
  import { Settings as SettingsIcon, Languages, MonitorCog, SunMoon } from "@lucide/svelte";
  import { SettingsService, type AppInfo } from "@/lib/api";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import Select from "@/lib/components/ui/Select.svelte";
  import { app, setTheme } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";
  import { applyTheme, type ThemeMode } from "@/lib/theme";

  let info = $state<AppInfo | null>(null);
  const languageOptions = $derived([
    { value: "zh", label: t(app.language, "settings.zh") },
    { value: "en", label: t(app.language, "settings.en") },
  ]);
  const themeOptions = $derived([
    { value: "system", label: t(app.language, "settings.themeSystem") },
    { value: "dark", label: t(app.language, "settings.themeDark") },
    { value: "light", label: t(app.language, "settings.themeLight") },
  ]);
  $effect(() => { SettingsService.Get().then((v: any) => (info = v)).catch(() => {}); });
  $effect(() => { if (typeof localStorage !== "undefined") localStorage.setItem("sshnat.language", app.language); });
  $effect(() => { applyTheme(app.theme); });
</script>

<div class="flex h-full flex-col">
  <header class="flex h-14 shrink-0 items-center border-b border-edge px-5"><h1 class="text-[15px] font-semibold text-ink">{t(app.language, "settings.title")}</h1></header>
  <div class="flex-1 overflow-y-auto p-5">
    <div class="max-w-xl space-y-4">
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><SunMoon size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.theme")}</span></div>
        <div class="flex items-center justify-between gap-4">
          <div><p class="text-sm text-ink">{t(app.language, "settings.theme")}</p><p class="text-xs text-dim">{t(app.language, "settings.themeHint")}</p></div>
          <div class="w-36"><Select bind:value={app.theme} options={themeOptions} /></div>
        </div>
      </div>
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><Languages size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.language")}</span></div>
        <div class="flex items-center justify-between gap-4"><div><p class="text-sm text-ink">{t(app.language, "settings.language")}</p><p class="text-xs text-dim">{t(app.language, "settings.languageHint")}</p></div><div class="w-36"><Select bind:value={app.language} options={languageOptions} /></div></div>
      </div>
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><MonitorCog size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.tray")}</span></div>
        <div class="flex items-start gap-3"><div class="mt-0.5 rounded-chip bg-ok/10 p-1.5 text-ok"><MonitorCog size={14} /></div><div><p class="text-sm text-ink">{t(app.language, "settings.trayEnabled")}</p><p class="mt-1 text-xs leading-5 text-dim">{t(app.language, "settings.trayHint")}</p></div></div>
        <p class="mt-3 border-t border-edge pt-3 text-[11px] text-dim/70">{t(app.language, "settings.trayNote")}</p>
      </div>
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><SettingsIcon size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.about")}</span></div>
        <dl class="space-y-2 text-xs"><div class="flex justify-between"><dt class="text-dim">{t(app.language, "settings.appName")}</dt><dd>{info?.name ?? "—"}</dd></div><div class="flex justify-between"><dt class="text-dim">{t(app.language, "settings.version")}</dt><dd><Badge tone="dim" mono>{info?.version ?? "—"}</Badge></dd></div><div class="flex items-center justify-between gap-4"><dt class="shrink-0 text-dim">{t(app.language, "settings.config")}</dt><dd class="truncate font-mono text-[11px] text-dim">{info?.configPath ?? "—"}</dd></div></dl>
      </div>
    </div>
  </div>
</div>

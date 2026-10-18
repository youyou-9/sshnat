<script lang="ts">
  import { Settings as SettingsIcon, Languages, MonitorCog, SunMoon, Power, RefreshCw, ExternalLink, Check, Info, LoaderCircle, SquareTerminal, Minus, Plus } from "@lucide/svelte";
  import { Browser } from "@wailsio/runtime";
  import { SettingsService, type AppInfo, type UpdateInfo } from "@/lib/api";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Select from "@/lib/components/ui/Select.svelte";
  import Switch from "@/lib/components/ui/Switch.svelte";
  import {
    app,
    setTheme,
    setLanguage,
    setTerminalFont,
    setTerminalFontSize,
    TERMINAL_FONTS,
    TERMINAL_FONT_MIN,
    TERMINAL_FONT_MAX,
  } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";
  import { type ThemeMode } from "@/lib/theme";
  import { type Locale } from "@/lib/i18n";
  import ConfigTransfer from "@/lib/components/ConfigTransfer.svelte";

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
  const terminalFontOptions = TERMINAL_FONTS.map((family) => ({ value: family, label: family }));
  // 预览直接用所选字体栈，方便对比不同字体的实际宽度。
  const previewFont = $derived(
    app.terminalFontFamily === "monospace"
      ? "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
      : `"${app.terminalFontFamily}", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`,
  );

  // 开机自启动
  let autoStart = $state(false);
  let autoStartBusy = $state(false);
  let autoStartError = $state("");

  async function toggleAutoStart(enabled: boolean) {
    if (autoStartBusy) return;
    autoStartBusy = true;
    autoStartError = "";
    const previous = autoStart;
    autoStart = enabled; // 乐观更新
    try {
      await SettingsService.SetAutoStart(enabled);
    } catch (e) {
      autoStart = previous;
      autoStartError = String(e instanceof Error ? e.message : e);
    } finally {
      autoStartBusy = false;
    }
  }

  // 检查更新
  let updateBusy = $state(false);
  let updateInfo = $state<UpdateInfo | null>(null);
  let updateError = $state("");

  async function checkUpdate() {
    if (updateBusy) return;
    updateBusy = true;
    updateInfo = null;
    updateError = "";
    try {
      updateInfo = await SettingsService.CheckUpdate();
    } catch (e) {
      updateError = String(e instanceof Error ? e.message : e);
    } finally {
      updateBusy = false;
    }
  }

  $effect(() => {
    SettingsService.Get().then((v: any) => (info = v)).catch(() => {});
    SettingsService.GetAutoStart().then((v) => (autoStart = v)).catch(() => {});
  });
</script>

<div class="flex h-full flex-col">
  <header class="flex h-14 shrink-0 items-center border-b border-edge px-5"><h1 class="text-title font-semibold tracking-[-0.01em] text-ink">{t(app.language, "settings.title")}</h1></header>
  <div class="flex-1 overflow-y-auto p-5">
    <div class="mx-auto w-full max-w-2xl space-y-4">
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><SunMoon size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.theme")}</span></div>
        <div class="flex items-center justify-between gap-4">
          <p class="text-meta text-dim">{t(app.language, "settings.themeHint")}</p>
          <div class="w-36"><Select value={app.theme} options={themeOptions} onValueChange={(v) => setTheme(v as ThemeMode)} /></div>
        </div>
      </div>
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><Languages size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.language")}</span></div>
        <div class="flex items-center justify-between gap-4"><p class="text-meta text-dim">{t(app.language, "settings.languageHint")}</p><div class="w-36"><Select value={app.language} options={languageOptions} onValueChange={(v) => setLanguage(v as Locale)} /></div></div>
      </div>
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><Power size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.autostart")}</span></div>
        <div class="flex items-center justify-between gap-4">
          <p class="text-meta text-dim">{t(app.language, "settings.autostartHint")}</p>
          <Switch id="autostart" checked={autoStart} onCheckedChange={toggleAutoStart} disabled={autoStartBusy} />
        </div>
        {#if autoStartError}<p role="alert" class="mt-2 rounded-chip border border-bad/40 bg-bad/10 p-2 text-xs text-bad">{t(app.language, "settings.autostartError")}: {autoStartError}</p>{/if}
      </div>
      <!-- 终端外观：字体族 + 字号，终端内也支持 Ctrl+滚轮/加减号实时缩放 -->
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><SquareTerminal size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.terminal")}</span></div>
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="text-sm text-ink">{t(app.language, "settings.terminalFont")}</p>
            <p class="text-meta text-dim">{t(app.language, "settings.terminalFontHint")}</p>
          </div>
          <div class="w-44">
            <Select value={app.terminalFontFamily} options={terminalFontOptions} onValueChange={setTerminalFont} />
          </div>
        </div>
        <div class="mt-3 flex items-center justify-between gap-4 border-t border-edge pt-3">
          <div>
            <p class="text-sm text-ink">{t(app.language, "settings.terminalFontSize")}</p>
            <p class="text-meta text-dim">{t(app.language, "settings.terminalFontSizeHint")}</p>
          </div>
          <div class="flex items-center gap-1.5">
            <Button size="icon" variant="ghost" disabled={app.terminalFontSize <= TERMINAL_FONT_MIN} aria-label={t(app.language, "settings.terminalFontSmaller")} title={t(app.language, "settings.terminalFontSmaller")} onclick={() => setTerminalFontSize(app.terminalFontSize - 1)}>
              <Minus size={13} />
            </Button>
            <span class="w-9 text-center font-mono text-sm text-ink" aria-live="polite">{app.terminalFontSize}</span>
            <Button size="icon" variant="ghost" disabled={app.terminalFontSize >= TERMINAL_FONT_MAX} aria-label={t(app.language, "settings.terminalFontLarger")} title={t(app.language, "settings.terminalFontLarger")} onclick={() => setTerminalFontSize(app.terminalFontSize + 1)}>
              <Plus size={13} />
            </Button>
            <Button size="sm" variant="ghost" onclick={() => setTerminalFontSize(13)}>{t(app.language, "settings.terminalFontReset")}</Button>
          </div>
        </div>
        <pre class="mt-3 overflow-x-auto rounded-chip bg-base px-3 py-2 text-ink" style={`font-family: ${previewFont}; font-size: ${app.terminalFontSize}px; line-height: 1.45`}>user@bastion:~$ ssh -L 8080:db:3306 nat
Welcome to SSHNat remote terminal<span class="animate-pulse">▍</span></pre>
      </div>
      <ConfigTransfer />
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><MonitorCog size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.tray")}</span></div>
        <div class="flex items-start gap-3"><div class="mt-0.5 rounded-chip bg-ok/10 p-1.5 text-ok"><MonitorCog size={14} /></div><div><p class="text-sm text-ink">{t(app.language, "settings.trayEnabled")}</p><p class="mt-1 text-xs leading-5 text-dim">{t(app.language, "settings.trayHint")}</p></div></div>
        <p class="mt-3 border-t border-edge pt-3 text-[11px] text-dim-2">{t(app.language, "settings.trayNote")}</p>
      </div>
      <div class="rounded-card border border-edge bg-panel p-4">
        <div class="mb-3 flex items-center gap-2"><SettingsIcon size={15} class="text-dim" /><span class="text-sm font-medium text-ink">{t(app.language, "settings.about")}</span></div>
        <dl class="space-y-2 text-xs">
          <div class="flex justify-between"><dt class="text-dim">{t(app.language, "settings.appName")}</dt><dd>{info?.name ?? "—"}</dd></div>
          <div class="flex justify-between"><dt class="text-dim">{t(app.language, "settings.version")}</dt><dd><Badge tone="dim" mono>{info?.version ?? "—"}</Badge></dd></div>
          <div class="flex items-center justify-between gap-4"><dt class="shrink-0 text-dim">{t(app.language, "settings.config")}</dt><dd class="truncate font-mono text-[11px] text-dim">{info?.configPath ?? "—"}</dd></div>
        </dl>
        <div class="mt-3 border-t border-edge pt-3">
          <div class="flex items-center justify-between gap-4">
            <p class="text-sm text-ink">{t(app.language, "settings.updates")}</p>
            <Button size="sm" variant="ghost" disabled={updateBusy} onclick={checkUpdate}>
              {#if updateBusy}<LoaderCircle size={13} class="mr-1 animate-spin" />{t(app.language, "settings.updateChecking")}
              {:else}<RefreshCw size={13} class="mr-1" />{t(app.language, "settings.updateCheck")}{/if}
            </Button>
          </div>
          {#if updateError}
            <p role="alert" class="mt-2 break-words rounded-chip border border-bad/40 bg-bad/10 p-2 text-xs text-bad">{t(app.language, "settings.updateFailed")}: {updateError}</p>
          {:else if updateInfo}
            {@const info2 = updateInfo}
            <!-- 按后端 reason 分支本地化，不依赖英文文案做判据 -->
            {@const inconclusive = info2.reason === "no-release" || info2.reason === "dev-build" || info2.reason === "unparsable"}
            <div class="mt-2 flex items-center gap-2 rounded-chip border px-2.5 py-2 text-xs {info2.hasUpdate ? "border-accent/40 bg-accent/10 text-accent" : inconclusive ? "border-warn/40 bg-warn/10 text-warn" : "border-ok/40 bg-ok/10 text-ok"}" role="status">
              {#if info2.hasUpdate}
                <span class="min-w-0 flex-1">{t(app.language, "settings.updateAvailable")}: v{info2.latest} <span class="text-dim">({t(app.language, "settings.updateCurrent")}: v{info2.current})</span></span>
                {#if info2.url}
                  {@const releaseUrl = info2.url}
                  <Button size="sm" variant="ghost" onclick={() => Browser.OpenURL(releaseUrl)}><ExternalLink size={13} class="mr-1" />{t(app.language, "settings.updateDownload")}</Button>
                {/if}
              {:else if inconclusive}
                <Info size={13} />
                <span class="min-w-0 flex-1">
                  {info2.reason === "no-release" ? t(app.language, "settings.updateNoRelease") : info2.reason === "dev-build" ? t(app.language, "settings.updateDevBuild") : t(app.language, "settings.updateUnparsable")}
                  {info2.latest ? ` · v${info2.latest}` : ""}
                </span>
              {:else}
                <Check size={13} />
                <span>{info2.latest ? `${t(app.language, "settings.updateLatest")} (v${info2.latest})` : t(app.language, "settings.updateLatest")}</span>
              {/if}
            </div>
          {/if}
        </div>
      </div>
    </div>
  </div>
</div>

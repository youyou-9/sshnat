<script lang="ts">
  import { Copy, Download, FileUp } from "@lucide/svelte";
  import { SettingsService } from "@/lib/api";
  import { app, refreshHosts, refreshTunnels, pushToast } from "@/lib/state.svelte";
  import { pruneTerminalState } from "@/lib/state.svelte";
  import { copyText } from "@/lib/tunnel-command";
  import { t } from "@/lib/i18n";
  import Button from "@/lib/components/ui/Button.svelte";
  import ConfirmDialog from "@/lib/components/ui/ConfirmDialog.svelte";
  import Label from "@/lib/components/ui/Label.svelte";
  import Select from "@/lib/components/ui/Select.svelte";
  import Switch from "@/lib/components/ui/Switch.svelte";

  let documentText = $state("");
  let mode = $state("merge");
  let includeSecrets = $state(false);
  let busy = $state(false);
  let error = $state("");
  let message = $state("");
  let replaceConfirmOpen = $state(false);
  let fileInput = $state<HTMLInputElement | null>(null);
  let pickedFileName = $state("");
  const label = (zh: string, en: string) => app.language === "zh" ? zh : en;
  const modes = $derived([
    { value: "merge", label: label("合并：保留现有条目", "Merge: keep existing entries") },
    { value: "replace", label: label("替换：先备份现有配置", "Replace: back up existing config first") },
  ]);

  async function exportConfig(toFile: boolean) {
    if (busy) return;
    busy = true; error = ""; message = "";
    try {
      if (toFile) {
        const path = await SettingsService.ExportFile(includeSecrets);
        if (path) {
          message = `${label("已导出至", "Exported to")}: ${path}`;
          pushToast("success", t(app.language, "toast.exported"));
        }
      } else {
        const text = await SettingsService.Export(includeSecrets);
        if (!await copyText(text)) throw new Error(label("无法访问剪贴板，请使用保存文件", "Clipboard unavailable; use Save File"));
        message = label("配置 JSON 已复制", "Configuration JSON copied");
        pushToast("success", t(app.language, "toast.exported"));
      }
    } catch (e) { error = String(e instanceof Error ? e.message : e); }
    finally { busy = false; }
  }

  async function readFile(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    pickedFileName = file.name;
    if (busy) { input.value = ""; return; }
    busy = true;
    documentText = "";
    error = ""; message = "";
    try {
      if (file.size > 10 * 1024 * 1024) throw new Error(label("配置文件不能大于 10 MiB", "Configuration must be at most 10 MiB"));
      documentText = await file.text();
    } catch (e) { error = String(e instanceof Error ? e.message : e); }
    finally { input.value = ""; busy = false; }
  }

  function requestImport() {
    if (busy || !documentText.trim()) return;
    if (mode === "replace") {
      replaceConfirmOpen = true; // 破坏性操作走确认对话框
      return;
    }
    importConfig();
  }

  async function importConfig() {
    if (busy || !documentText.trim()) return;
    busy = true; error = ""; message = "";
    try {
      const result = await SettingsService.Import(documentText, mode);
      if (!result) throw new Error(label("导入未返回结果", "Import returned no result"));
      if (mode === "replace") { app.txHistory = {}; app.rxHistory = {}; }
      documentText = "";
      message = `${label("导入完成", "Imported")}: ${result.hostsAdded} ${label("台主机", "hosts")}, ${result.tunnelsAdded} ${label("条隧道", "tunnels")}`;
      if (result.backupPath) message += ` · ${label("备份", "Backup")}: ${result.backupPath}`;
      pushToast("success", t(app.language, "toast.imported"));
      try {
        await Promise.all([refreshHosts(), refreshTunnels()]);
        // 替换/合并后主机集合可能变化：清掉僵尸终端标签与失效的重试时间戳。
        pruneTerminalState();
      } catch (e) {
        error = `${label("配置已保存，但列表刷新失败，请重新打开应用", "Configuration saved, but list refresh failed; reopen the app")}: ${String(e instanceof Error ? e.message : e)}`;
      }
    } catch (e) { error = String(e instanceof Error ? e.message : e); }
    finally { busy = false; }
  }
</script>

<section class="space-y-3 rounded-card border border-edge bg-panel p-4" data-testid="config-transfer">
  <h2 class="flex items-center gap-2 text-sm font-medium text-ink"><FileUp size={15} class="text-dim" />{label("配置迁移与备份", "Configuration & backups")}</h2>
  <p class="text-xs leading-5 text-dim">{label("导出主机与隧道规则，或导入 JSON 文件以迁移、恢复配置。私钥文件与 known_hosts 需另行复制。导入的隧道保持停止状态。", "Export hosts and tunnel rules, or import JSON to migrate or restore. Copy private key files and known_hosts separately. Imported tunnels remain stopped.")}</p>
  <div class="flex items-center justify-between gap-3">
    <Label for="include-secrets">{label("完整备份（包含密码与私钥口令）", "Full backup (include passwords and key passphrases)")}</Label>
    <Switch id="include-secrets" checked={includeSecrets} onCheckedChange={(v) => includeSecrets = v} disabled={busy} />
  </div>
  {#if includeSecrets}<p class="text-xs text-warn">{label("备份文件包含认证凭据，请保存在私有位置。", "The backup contains credentials; keep it in a private location.")}</p>{/if}
  <div class="flex flex-wrap gap-2">
    <!-- server 构建没有原生保存对话框：隐藏保存按钮并给出说明 -->
    {#if !app.serverMode}
      <Button size="sm" disabled={busy} onclick={() => exportConfig(true)}><Download size={13} class="mr-1" />{label("保存配置文件", "Save Configuration File")}</Button>
    {:else}
      <p class="w-full text-2xs text-dim-2">{label("服务器模式没有原生保存对话框，请使用“复制配置 JSON”。", "Server mode has no native save dialog; use “Copy Configuration JSON”.")}</p>
    {/if}
    <Button variant="ghost" size="sm" disabled={busy} onclick={() => exportConfig(false)}><Copy size={13} class="mr-1" />{label("复制配置 JSON", "Copy Configuration JSON")}</Button>
  </div>
  <div class="space-y-2 border-t border-edge pt-3">
    <Label for="import-file">{label("读取 JSON 文件", "Read JSON File")}</Label>
    <!-- 原生 file 按钮文案跟随系统语言（英文界面会残留中文），隐藏后自绘 -->
    <input bind:this={fileInput} id="import-file" type="file" accept=".json,application/json" disabled={busy} onchange={readFile} class="sr-only" tabindex="-1" aria-hidden="true" />
    <div class="flex items-center gap-2">
      <Button size="sm" variant="ghost" disabled={busy} onclick={() => fileInput?.click()}>{label("选择文件", "Choose File")}</Button>
      <span class="truncate text-xs text-dim">{pickedFileName || label("未选择文件", "No file chosen")}</span>
    </div>
    <Label for="import-json">{label("或粘贴配置 JSON", "Or Paste Configuration JSON")}</Label>
    <textarea id="import-json" bind:value={documentText} disabled={busy} rows={4} spellcheck="false" placeholder={JSON.stringify({version: 1, hosts: [], tunnels: []})} class="w-full resize-y rounded-chip border border-edge bg-base px-2.5 py-2 font-mono text-xs text-ink focus:border-accent focus:outline-none"></textarea>
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="min-w-60 flex-1"><Label for="import-mode" class="sr-only">{label("导入方式", "Import Mode")}</Label><Select id="import-mode" bind:value={mode} options={modes} disabled={busy} /></div>
      <Button variant="primary" size="sm" disabled={busy || !documentText.trim()} onclick={requestImport}>{busy ? "…" : label("导入配置", "Import Configuration")}</Button>
    </div>
    {#if mode === "replace"}<p class="text-xs text-warn">{label("替换前须停止所有隧道。旧配置的完整备份可通过此处再次导入恢复。", "Stop all tunnels before replacing. Restore the full backup by importing it here.")}</p>{/if}
  </div>
  {#if error}<p role="alert" class="break-words rounded-chip border border-bad/40 bg-bad/10 p-2 text-xs text-bad">{error}</p>{/if}
  {#if message}<p role="status" class="break-words rounded-chip border border-ok/40 bg-ok/10 p-2 text-xs text-ok">{message}</p>{/if}
</section>

<ConfirmDialog
  bind:open={replaceConfirmOpen}
  title={label("替换配置", "Replace Configuration")}
  message={label("替换全部主机和隧道？旧配置将备份到配置目录。请先停止所有隧道。", "Replace all hosts and tunnels? The old configuration will be backed up. Stop all tunnels first.")}
  confirmLabel={label("导入配置", "Import Configuration")}
  onConfirm={importConfig}
/>

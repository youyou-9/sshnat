<script lang="ts">
  import { FolderOpen, LoaderCircle } from "@lucide/svelte";
  import { SettingsService } from "@/lib/api";
  import Input from "@/lib/components/ui/Input.svelte";
  import { app, pushToast } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  // 文件路径输入框 + 原生文件选择器按钮。
  let {
    value = $bindable(""),
    placeholder = "",
    id,
    disabled = false,
  }: {
    value?: string;
    placeholder?: string;
    id?: string;
    disabled?: boolean;
  } = $props();

  let picking = $state(false);

  async function browse() {
    if (picking || disabled) return;
    picking = true;
    try {
      const path = await SettingsService.PickFile();
      if (path) value = path;
    } catch (e) {
      pushToast("error", String(e instanceof Error ? e.message : e));
    } finally {
      picking = false;
    }
  }
</script>

<div class="flex items-center gap-1.5">
  <div class="min-w-0 flex-1">
    <Input {id} {placeholder} bind:value {disabled} class="font-mono" />
  </div>
  <!-- server 构建没有原生对话框，直接隐藏浏览按钮，避免点了才报错 -->
  {#if !app.serverMode}
    <button
      type="button"
      class="flex h-8 shrink-0 items-center gap-1 rounded-chip border border-edge bg-base px-2 text-xs text-dim transition-colors hover:border-dim/50 hover:text-ink disabled:opacity-50"
      {disabled}
      aria-label={t(app.language, "common.browse")}
      title={t(app.language, "common.browse")}
      onclick={browse}
    >
      {#if picking}<LoaderCircle size={13} class="animate-spin" />{:else}<FolderOpen size={13} />{/if}
      {t(app.language, "common.browse")}
    </button>
  {/if}
</div>

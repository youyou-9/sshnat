<script lang="ts">
  import Dialog from "@/lib/components/ui/Dialog.svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import { app } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  // 破坏性操作的统一确认对话框，替代 window.confirm。
  let {
    open = $bindable(false),
    title,
    message,
    confirmLabel = "",
    onConfirm,
  }: {
    open?: boolean;
    title: string;
    message: string;
    confirmLabel?: string;
    onConfirm: () => void;
  } = $props();

  function confirm() {
    open = false;
    onConfirm();
  }
</script>

<Dialog bind:open {title} description={message}>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)}>{t(app.language, "tunnel.cancel")}</Button>
    <Button variant="danger" onclick={confirm}>{confirmLabel || t(app.language, "common.confirm")}</Button>
  {/snippet}
</Dialog>

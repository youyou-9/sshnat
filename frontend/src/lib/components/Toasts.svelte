<script lang="ts">
  import { CheckCircle2, AlertCircle, Info, X } from "@lucide/svelte";
  import { app, dismissToast } from "@/lib/state.svelte";
</script>

<!-- 放在右上角（页头下方），避免遮住底部终端 Dock -->
<div class="pointer-events-none fixed right-4 top-16 z-[60] flex w-80 flex-col gap-2" role="region" aria-live="polite">
  {#each app.toasts as toast (toast.id)}
    <div
      class="toast-item pointer-events-auto flex items-start gap-2 rounded-card border border-edge bg-panel/95 p-3 text-xs text-ink shadow-[var(--shadow-menu)] backdrop-blur"
      role={toast.kind === "error" ? "alert" : "status"}
    >
      {#if toast.kind === "success"}<CheckCircle2 size={14} class="mt-0.5 shrink-0 text-ok" />
      {:else if toast.kind === "error"}<AlertCircle size={14} class="mt-0.5 shrink-0 text-bad" />
      {:else}<Info size={14} class="mt-0.5 shrink-0 text-accent" />{/if}
      <span class="min-w-0 flex-1 break-words leading-5">{toast.message}</span>
      <button class="shrink-0 rounded p-0.5 text-dim hover:text-ink" aria-label="dismiss" onclick={() => dismissToast(toast.id)}><X size={12} /></button>
    </div>
  {/each}
</div>

<style>
  .toast-item {
    animation: toast-in 160ms ease-out;
  }
  @keyframes toast-in {
    from { opacity: 0; transform: translateY(-6px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>

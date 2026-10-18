<script lang="ts">
  import { Dialog as DialogPrimitive } from "bits-ui";
  import { X } from "@lucide/svelte";
  import type { Snippet } from "svelte";
  import { app, dialogOpened, dialogClosed } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";
  // shadcn 风格对话框：头/身体/底三段固定，只有身体滚动，
  // 长表单的按钮永远不会被滚出视野。
  let {
    open = $bindable(false),
    title,
    description = "",
    children,
    footer,
  }: {
    open?: boolean;
    title: string;
    description?: string;
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  // 登记模态框开合，供全局快捷键判断是否要挂起页面跳转。
  // 计数是普通模块变量（非响应式），避免 effect 自触发死循环。
  $effect(() => {
    if (!open) return;
    dialogOpened();
    return () => dialogClosed();
  });
</script>

<DialogPrimitive.Root bind:open>
  <DialogPrimitive.Portal>
    <DialogPrimitive.Overlay class="fixed inset-0 z-40 bg-scrim backdrop-blur-sm" />
    <DialogPrimitive.Content
      class="fixed left-1/2 top-1/2 z-50 grid max-h-[90vh] w-[min(600px,94vw)] -translate-x-1/2 -translate-y-1/2
             grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden rounded-card border border-edge bg-panel p-5
             shadow-[var(--shadow-pop)] focus:outline-none"
    >
      <div class="mb-4 flex items-start justify-between gap-4">
        <div>
          <DialogPrimitive.Title class="text-title font-semibold tracking-[-0.01em] text-ink">{title}</DialogPrimitive.Title>
          {#if description}
            <DialogPrimitive.Description class="mt-1 text-meta leading-5 text-dim">
              {description}
            </DialogPrimitive.Description>
          {/if}
        </div>
        <DialogPrimitive.Close
          class="rounded-chip p-1 text-dim transition-colors hover:bg-hover hover:text-ink"
          aria-label={t(app.language, "common.close")}
        >
          <X size={16} />
        </DialogPrimitive.Close>
      </div>

      <div class="min-h-0 overflow-y-auto pr-1">
        {@render children?.()}
      </div>

      {#if footer}
        <div class="mt-4 flex items-center justify-end gap-2 border-t border-edge pt-3">
          {@render footer()}
        </div>
      {/if}
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>
</DialogPrimitive.Root>

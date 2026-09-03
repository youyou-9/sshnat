<script lang="ts">
  import { Dialog as DialogPrimitive } from "bits-ui";
  import { X } from "@lucide/svelte";
  import type { Snippet } from "svelte";
  // shadcn 风格对话框：扁平、1px 边框、深底。
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
</script>

<DialogPrimitive.Root bind:open>
  <DialogPrimitive.Portal>
    <DialogPrimitive.Overlay
      class="fixed inset-0 z-40 bg-black/60 backdrop-blur-[1px]"
    />
    <DialogPrimitive.Content
      class="fixed left-1/2 top-1/2 z-50 w-[min(580px,94vw)] max-h-[90vh] overflow-y-auto overflow-x-hidden -translate-x-1/2 -translate-y-1/2
             rounded-card border border-edge bg-panel p-5 focus:outline-none"
    >
      <div class="mb-4 flex items-start justify-between gap-4">
        <div>
          <DialogPrimitive.Title class="text-base font-semibold text-ink">{title}</DialogPrimitive.Title>
          {#if description}
            <DialogPrimitive.Description class="mt-1 text-xs text-dim">
              {description}
            </DialogPrimitive.Description>
          {/if}
        </div>
        <DialogPrimitive.Close
          class="rounded-chip p-1 text-dim transition-colors hover:bg-hover hover:text-ink"
          aria-label="Close"
        >
          <X size={16} />
        </DialogPrimitive.Close>
      </div>

      {@render children?.()}

      {#if footer}
        <div class="mt-5 flex items-center justify-end gap-2">
          {@render footer()}
        </div>
      {/if}
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>
</DialogPrimitive.Root>

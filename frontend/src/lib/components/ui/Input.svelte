<script lang="ts">
  // 深底 + 边框输入框（修正 Stitch 初稿白底缺陷）。
  import type { HTMLInputAttributes } from "svelte/elements";

  let {
    class: className = "",
    value = $bindable(""),
    ...rest
  }: { class?: string; value?: string } & HTMLInputAttributes = $props();
</script>

<input
  bind:value
  class={`h-8 w-full rounded-chip border border-edge bg-base px-2.5 text-sm text-ink
          placeholder:text-dim/60 transition-colors
          hover:border-dim/50 focus:outline-none focus:border-accent
          disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
  {...rest}
/>

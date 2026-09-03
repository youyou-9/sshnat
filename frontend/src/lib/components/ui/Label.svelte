<script lang="ts">
  import type { HTMLLabelAttributes } from "svelte/elements";

  let { class: className = "", children, ...rest }: { class?: string } & HTMLLabelAttributes =
    $props();
</script>

<label class={`text-xs font-medium text-dim ${className}`} {...rest}>
  {@render children?.()}
</label>

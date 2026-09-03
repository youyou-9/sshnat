<script lang="ts">
  // shadcn-svelte 风格按钮（源码内联），变体对齐 SSHNat 设计系统。
  import type { Snippet } from "svelte";
  import type { HTMLButtonAttributes } from "svelte/elements";

  type Variant = "primary" | "secondary" | "ghost" | "danger";
  type Size = "sm" | "md" | "icon";

  let {
    variant = "secondary" as Variant,
    size = "md" as Size,
    class: className = "",
    children,
    ...rest
  }: { variant?: Variant; size?: Size; class?: string; children?: Snippet } & HTMLButtonAttributes =
    $props();

  const variants: Record<Variant, string> = {
    primary:
      "bg-accent text-[var(--c-base)] font-medium hover:brightness-110 focus-visible:outline-accent",
    secondary: "bg-panel border border-edge text-ink hover:bg-hover focus-visible:outline-edge",
    ghost: "bg-transparent text-dim hover:text-ink hover:bg-hover focus-visible:outline-edge",
    danger: "bg-transparent border border-bad/50 text-bad hover:bg-bad/10 focus-visible:outline-bad",
  };

  const sizes: Record<Size, string> = {
    sm: "h-7 px-2.5 text-xs rounded-chip gap-1",
    md: "h-8 px-3 text-sm rounded-chip gap-1.5",
    icon: "h-8 w-8 rounded-chip",
  };
</script>

<button
  class={`inline-flex items-center justify-center whitespace-nowrap transition-colors
          focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-2
          disabled:pointer-events-none disabled:opacity-50
          ${variants[variant]} ${sizes[size]} ${className}`}
  {...rest}
>
  {@render children?.()}
</button>

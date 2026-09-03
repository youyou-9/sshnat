<script lang="ts">
  import { Select as SelectPrimitive } from "bits-ui";
  import { ChevronDown } from "@lucide/svelte";

  // shadcn 风格下拉（bits-ui Select）：避免浏览器原生双箭头与默认蓝。
  export type Option = { value: string; label: string };

  let {
    value = $bindable(""),
    options = [] as Option[],
    placeholder = "Select…",
    disabled = false,
    class: className = "",
    onValueChange,
  }: {
    value?: string;
    options?: Option[];
    placeholder?: string;
    disabled?: boolean;
    class?: string;
    onValueChange?: (val: string) => void;
  } = $props();
  const selectedLabel = $derived(options.find((o) => o.value === value)?.label ?? placeholder);
</script>

<SelectPrimitive.Root
  type="single"
  bind:value
  {disabled}
  onValueChange={(v) => {
    onValueChange?.(v);
  }}
>
  <SelectPrimitive.Trigger
    class={`flex h-8 w-full items-center justify-between rounded-chip border border-edge bg-base
            px-2.5 text-sm transition-colors hover:border-dim/50
            focus:outline-none focus:border-accent disabled:opacity-50 ${className}`}
  >
    <span class={value ? "text-ink" : "text-dim/60"}>{selectedLabel}</span>
    <ChevronDown size={14} class="text-dim" />
  </SelectPrimitive.Trigger>

  <SelectPrimitive.Portal>
    <SelectPrimitive.Content
      class="z-50 max-h-64 min-w-[var(--bits-select-trigger-width)] overflow-hidden rounded-chip
             border border-edge bg-panel text-sm shadow-none"
    >
      <SelectPrimitive.Viewport class="p-1">
        {#each options as opt (opt.value)}
          <SelectPrimitive.Item
            value={opt.value}
            label={opt.label}
            class="flex cursor-default select-none items-center rounded-chip px-2 py-1.5 text-ink
                   outline-none data-highlighted:bg-hover data-[state=checked]:text-accent"
          >
            {opt.label}
          </SelectPrimitive.Item>
        {/each}
      </SelectPrimitive.Viewport>
    </SelectPrimitive.Content>
  </SelectPrimitive.Portal>
</SelectPrimitive.Root>

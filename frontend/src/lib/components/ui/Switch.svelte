<script lang="ts">
  import { Switch as SwitchPrimitive } from "bits-ui";

  // shadcn 风格开关：只用 switch，不叠加对勾（修正初稿缺陷）。
  let {
    checked = $bindable(false),
    disabled = false,
    id,
    "aria-label": ariaLabel,
    onCheckedChange,
    class: className = "",
  }: {
    checked?: boolean;
    disabled?: boolean;
    id?: string;
    "aria-label"?: string;
    onCheckedChange?: (checked: boolean) => void;
    class?: string;
  } = $props();

  const track = $derived(
    `inline-flex h-[18px] w-8 shrink-0 items-center rounded-full border transition-colors
     focus-visible:outline focus-visible:outline-1 focus-visible:outline-accent
     disabled:cursor-not-allowed disabled:opacity-50 ${
       checked
         ? "border-accent/60 bg-accent/25"
         : "border-edge bg-hover"
     } ${className}`
  );
  const thumb = $derived(
    `block size-3.5 rounded-full transition-transform ${
      checked ? "translate-x-[14px] bg-accent" : "translate-x-[2px] bg-dim"
    }`
  );
</script>

<SwitchPrimitive.Root bind:checked {disabled} {id} aria-label={ariaLabel} {onCheckedChange} class={track}>
  <SwitchPrimitive.Thumb class={thumb} />
</SwitchPrimitive.Root>

<script lang="ts">
  import type { Snippet } from "svelte";

  // 设计系统徽章：L/R/D 类型与状态。
  type Tone = "accent" | "ok" | "warn" | "bad" | "dim";

  let {
    tone = "dim",
    children,
    mono = false,
  }: { tone?: Tone; children?: Snippet; mono?: boolean } = $props();

  const tones: Record<Tone, string> = {
    accent: "border-accent/40 text-accent bg-accent/10",
    ok: "border-ok/40 text-ok bg-ok/10",
    warn: "border-warn/40 text-warn bg-warn/10",
    bad: "border-bad/40 text-bad bg-bad/10",
    dim: "border-edge text-dim bg-hover",
  };
</script>

<span
  class={`inline-flex items-center rounded-chip border px-1.5 py-0.5 text-[11px] leading-none
          ${tones[tone]} ${mono ? "font-mono" : ""}`}
>
  {@render children?.()}
</span>

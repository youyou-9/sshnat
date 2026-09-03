<script lang="ts">
  // 轻量 SVG sparkline：无图表库依赖，数据为速率增量序列。
  let {
    data = [] as number[],
    color = "var(--c-accent)",
    width = 120,
    height = 32,
  }: { data?: number[]; color?: string; width?: number; height?: number } = $props();

  const points = $derived.by(() => {
    if (data.length < 2) return "";
    const max = Math.max(...data, 1);
    const step = width / (data.length - 1);
    return data
      .map((v, i) => {
        const x = (i * step).toFixed(1);
        const y = (height - 2 - (v / max) * (height - 4)).toFixed(1);
        return `${x},${y}`;
      })
      .join(" ");
  });

  const flat = $derived(data.length < 2);
</script>

<svg {width} {height} viewBox={`0 0 ${width} ${height}`} class="overflow-visible">
  {#if flat}
    <line x1="0" y1={height - 1} x2={width} y2={height - 1} stroke="{color}" stroke-opacity="0.35" stroke-width="1" />
  {:else}
    <polyline
      {points}
      fill="none"
      stroke={color}
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  {/if}
</svg>

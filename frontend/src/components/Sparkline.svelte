<!--
  Tiny inline-SVG sparkline used by SensorCard. No axes, no tooltips —
  just a smooth line + soft area fill scaled to the box.
-->
<script lang="ts">
    interface Props {
        values: number[];
        width?: number;
        height?: number;
        stroke?: string;
    }
    let { values, width = 120, height = 36, stroke = "currentColor" }: Props = $props();

    const path = $derived.by(() => {
        if (values.length < 2) return { line: "", area: "" };
        const min = Math.min(...values);
        const max = Math.max(...values);
        const span = max - min || 1;
        const stepX = width / (values.length - 1);
        const pts = values.map((v, i) => {
            const x = i * stepX;
            // 4px padding top/bottom so the stroke isn't clipped.
            const y = height - 4 - ((v - min) / span) * (height - 8);
            return [x, y] as const;
        });
        const line = pts.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`).join(" ");
        const area = `${line} L${pts[pts.length - 1][0].toFixed(2)},${height} L0,${height} Z`;
        return { line, area };
    });
</script>

<svg viewBox="0 0 {width} {height}" width={width} height={height} aria-hidden="true" preserveAspectRatio="none">
    {#if path.area}
        <path d={path.area} fill={stroke} opacity="0.12" />
        <path d={path.line} fill="none" stroke={stroke} stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
    {/if}
</svg>

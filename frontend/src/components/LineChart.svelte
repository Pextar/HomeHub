<!--
  Self-contained inline-SVG line chart. Mobile-first: scales to its
  container width, touch-friendly tap-to-inspect.

  Inputs: array of {time, value} readings. Renders an area+line chart
  with y-axis labels (min/max/mid) and a couple of x-axis time ticks.
-->
<script lang="ts">
    import type { SensorReading } from "../lib/types";

    interface Props {
        readings: SensorReading[];
        unit?: string;
        height?: number;
        stroke?: string;
    }
    let { readings, unit = "", height = 220, stroke = "var(--primary)" }: Props = $props();

    let containerWidth = $state(640);

    // Track our own width so the SVG scales with the page.
    let container: HTMLDivElement | undefined = $state();
    $effect(() => {
        if (!container) return;
        const ro = new ResizeObserver((entries) => {
            for (const e of entries) containerWidth = Math.max(280, Math.floor(e.contentRect.width));
        });
        ro.observe(container);
        return () => ro.disconnect();
    });

    const PAD_LEFT = 44;
    const PAD_RIGHT = 12;
    const PAD_TOP = 12;
    const PAD_BOTTOM = 26;

    const chart = $derived.by(() => {
        if (readings.length === 0) {
            return { line: "", area: "", points: [] as { x: number; y: number; r: SensorReading }[],
                min: 0, max: 0, plotW: 0, plotH: 0, xTicks: [] as { x: number; label: string }[], yTicks: [] as { y: number; label: string }[] };
        }
        const w = containerWidth;
        const plotW = w - PAD_LEFT - PAD_RIGHT;
        const plotH = height - PAD_TOP - PAD_BOTTOM;
        const vals = readings.map(r => r.value);
        let min = Math.min(...vals);
        let max = Math.max(...vals);
        if (min === max) { min -= 1; max += 1; }
        const span = max - min;

        const times = readings.map(r => new Date(r.time).getTime());
        const tMin = times[0];
        const tMax = times[times.length - 1];
        const tSpan = (tMax - tMin) || 1;

        const points = readings.map((r, i) => {
            const x = PAD_LEFT + ((times[i] - tMin) / tSpan) * plotW;
            const y = PAD_TOP + (1 - (r.value - min) / span) * plotH;
            return { x, y, r };
        });

        const line = points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(" ");
        const last = points[points.length - 1];
        const first = points[0];
        const area = `${line} L${last.x.toFixed(2)},${PAD_TOP + plotH} L${first.x.toFixed(2)},${PAD_TOP + plotH} Z`;

        const yTicks = [
            { y: PAD_TOP, label: formatValue(max) },
            { y: PAD_TOP + plotH / 2, label: formatValue((max + min) / 2) },
            { y: PAD_TOP + plotH, label: formatValue(min) },
        ];

        // Show three x ticks: first, middle, last.
        const tickCount = Math.min(3, readings.length);
        const xTicks = Array.from({ length: tickCount }, (_, i) => {
            const idx = Math.floor((readings.length - 1) * (i / Math.max(1, tickCount - 1)));
            return { x: points[idx].x, label: formatTime(times[idx]) };
        });

        return { line, area, points, min, max, plotW, plotH, xTicks, yTicks };
    });

    // Spoken summary for screen readers — the visual chart is meaningless to
    // them, so expose current / min / max instead of a generic label.
    const ariaSummary = $derived.by(() => {
        if (readings.length < 2) return "Sensor reading chart, not enough data yet";
        const u = unit ? ` ${unit}` : "";
        const current = readings[readings.length - 1].value;
        return `Sensor readings. Current ${formatValue(current)}${u}, ` +
            `range ${formatValue(chart.min)} to ${formatValue(chart.max)}${u}, ` +
            `${readings.length} points`;
    });

    function formatValue(v: number): string {
        if (Math.abs(v) >= 100) return v.toFixed(0);
        if (Math.abs(v) >= 10) return v.toFixed(1);
        return v.toFixed(2);
    }

    function formatTime(ts: number): string {
        const d = new Date(ts);
        return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    }

    // Tap-to-inspect: highlight the nearest point.
    let hoverIndex = $state<number | null>(null);
    function pickFromEvent(e: PointerEvent) {
        if (chart.points.length === 0) return;
        const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
        const x = e.clientX - rect.left;
        let best = 0;
        let bestDist = Infinity;
        for (let i = 0; i < chart.points.length; i++) {
            const d = Math.abs(chart.points[i].x - x);
            if (d < bestDist) { best = i; bestDist = d; }
        }
        hoverIndex = best;
    }
</script>

<div class="chart" bind:this={container}>
    {#if readings.length < 2}
        <div class="empty">Not enough data yet — waiting for readings.</div>
    {:else}
        <svg viewBox="0 0 {containerWidth} {height}" width={containerWidth} height={height}
            role="img" aria-label={ariaSummary}
            onpointermove={pickFromEvent}
            onpointerdown={pickFromEvent}
            onpointerleave={() => hoverIndex = null}>
            <!-- Y gridlines + labels -->
            {#each chart.yTicks as t (t.label + t.y)}
                <line x1={PAD_LEFT} x2={containerWidth - PAD_RIGHT} y1={t.y} y2={t.y}
                    stroke="var(--border)" stroke-dasharray="2 4" />
                <text x={PAD_LEFT - 8} y={t.y + 4} text-anchor="end" class="axis">{t.label}</text>
            {/each}

            <!-- X labels — first anchors left, last anchors right so they
                 don't overlap the y-axis labels or clip at the chart edge. -->
            {#each chart.xTicks as t, i (i + ":" + t.label)}
                <text x={t.x} y={height - 6} class="axis"
                    text-anchor={i === 0 ? "start" : i === chart.xTicks.length - 1 ? "end" : "middle"}>{t.label}</text>
            {/each}

            <!-- Area + line -->
            <path d={chart.area} fill={stroke} opacity="0.14" />
            <path d={chart.line} fill="none" stroke={stroke} stroke-width="2"
                stroke-linecap="round" stroke-linejoin="round" />

            <!-- Hover marker -->
            {#if hoverIndex !== null}
                {@const p = chart.points[hoverIndex]}
                <line x1={p.x} x2={p.x} y1={PAD_TOP} y2={PAD_TOP + chart.plotH}
                    stroke="var(--text-muted)" stroke-dasharray="2 3" />
                <circle cx={p.x} cy={p.y} r="4" fill={stroke} stroke="var(--bg-elevated)" stroke-width="2" />
            {/if}
        </svg>

        {#if hoverIndex !== null}
            {@const p = chart.points[hoverIndex]}
            <div class="readout">
                <span class="rv">{formatValue(p.r.value)}{unit ? ` ${unit}` : ""}</span>
                <span class="rt">{new Date(p.r.time).toLocaleString([], { dateStyle: "short", timeStyle: "short" })}</span>
            </div>
        {/if}
    {/if}
</div>

<style>
    .chart {
        position: relative;
        width: 100%;
        touch-action: pan-y;
    }
    svg { display: block; max-width: 100%; }
    .axis {
        fill: var(--text-muted);
        font-size: 10px;
        font-family: var(--font-mono, monospace);
    }
    .empty {
        height: 120px;
        display: grid;
        place-items: center;
        color: var(--text-muted);
        font-size: 13px;
        border: 1px dashed var(--border);
        border-radius: var(--radius-md);
    }
    .readout {
        position: absolute;
        top: 8px;
        right: 8px;
        display: flex;
        flex-direction: column;
        align-items: flex-end;
        gap: 2px;
        padding: 6px 10px;
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        pointer-events: none;
    }
    .rv {
        font-weight: 700;
        font-family: var(--font-mono, monospace);
    }
    .rt {
        font-size: 11px;
        color: var(--text-muted);
    }
</style>

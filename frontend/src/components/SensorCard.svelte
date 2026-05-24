<script lang="ts">
    import Icon from "./Icon.svelte";
    import Sparkline from "./Sparkline.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import type { Sensor, SensorReading } from "../lib/types";

    interface Props {
        sensor: Sensor;
        compact?: boolean;
    }
    let { sensor, compact = false }: Props = $props();

    let readings = $state<SensorReading[]>([]);
    let loading = $state(true);

    async function load() {
        try {
            readings = await api.sensorReadings(sensor.id, { limit: 60, since_minutes: 24 * 60 });
        } catch (e) {
            toasts.error("Sensor history failed", (e as Error).message);
        } finally {
            loading = false;
        }
    }
    $effect(() => {
        sensor.last_reading_at;
        load();
    });

    const sparkValues = $derived(readings.map(r => r.value));
    const iconName = $derived(iconForKind(sensor.kind));
    const kindLabel = $derived(sensor.kind === "custom" ? "Sensor" : sensor.kind);

    const alert = $derived.by(() => {
        const v = sensor.last_value;
        if (v === undefined || v === null) return null;
        if (sensor.alert_min !== undefined && v < sensor.alert_min) return `Below ${sensor.alert_min}${sensor.unit}`;
        if (sensor.alert_max !== undefined && v > sensor.alert_max) return `Above ${sensor.alert_max}${sensor.unit}`;
        return null;
    });

    const lastFormatted = $derived(formatValue(sensor.last_value));
    const lastAgo = $derived(sensor.last_reading_at ? agoString(sensor.last_reading_at) : "no data yet");

    function iconForKind(kind: string) {
        switch (kind) {
            case "temperature": return "temperature";
            case "humidity":    return "humidity";
            case "motion":      return "motion";
            case "light":       return "light";
            case "power":       return "power";
            default:            return "sensor";
        }
    }
    function formatValue(v?: number): string {
        if (v === undefined || v === null) return "—";
        if (Math.abs(v) >= 100) return v.toFixed(0);
        if (Math.abs(v) >= 10) return v.toFixed(1);
        return v.toFixed(2);
    }
    function agoString(iso: string): string {
        const diffSec = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
        if (diffSec < 60) return `${diffSec}s ago`;
        const m = Math.round(diffSec / 60);
        if (m < 60) return `${m} min ago`;
        const h = Math.round(m / 60);
        if (h < 24) return `${h}h ago`;
        return `${Math.round(h / 24)}d ago`;
    }
</script>

<div class="sensor" class:compact class:alerting={alert}>
    <div class="top">
        <span class="ico"><Icon name={iconName} size={16} /></span>
        {#if alert}<span class="alert-badge mono" title={alert}>ALERT</span>{/if}
    </div>

    <div class="label">
        <div class="name" title={sensor.name}>{sensor.name}</div>
        <div class="kind">{sensor.room || kindLabel}</div>
    </div>

    <div class="value">
        <span class="num num-display">{lastFormatted}</span>
        {#if sensor.unit}<span class="unit">{sensor.unit}</span>{/if}
    </div>

    <div class="spark">
        {#if loading}
            <div class="spark-ph"></div>
        {:else if sparkValues.length >= 2}
            <Sparkline values={sparkValues} width={120} height={22} />
        {:else}
            <div class="last">Last: <span class="mono">{lastAgo}</span></div>
        {/if}
    </div>
</div>

<style>
    .sensor {
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: 14px;
        display: flex;
        flex-direction: column;
        gap: 8px;
        min-width: 0;
        transition: border-color var(--t-fast), transform var(--t-fast), box-shadow var(--t-fast);
    }
    @media (hover: hover) {
        .sensor:hover { border-color: var(--border-strong); transform: translateY(-2px); box-shadow: var(--shadow-md); }
    }
    .sensor.alerting { border-color: var(--bad); }

    .top { display: flex; align-items: center; justify-content: space-between; min-height: 18px; }
    .ico { color: var(--cool); display: inline-flex; }
    .sensor.alerting .ico { color: var(--bad); }
    .alert-badge { font-size: 10px; font-weight: 500; letter-spacing: 0.04em; color: var(--bad); }

    .label { min-width: 0; }
    .name { font-weight: 600; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .kind { color: var(--text-mute); font-size: 11px; margin-top: 1px; text-transform: capitalize;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

    .value { display: flex; align-items: baseline; gap: 4px; margin-top: 2px; }
    .num { font-size: 26px; }
    .sensor.alerting .num { color: var(--bad); }
    .unit { color: var(--text-mute); font-size: 12px; }

    .spark { height: 22px; color: var(--cool); margin-top: 2px; }
    .sensor.alerting .spark { color: var(--bad); }
    .spark-ph {
        height: 22px;
        border-radius: 6px;
        background: var(--card-2);
    }
    .last { color: var(--text-dim); font-size: 11px; }

    .sensor.compact { padding: 12px; gap: 6px; }
    .sensor.compact .num { font-size: 22px; }
</style>

<script lang="ts">
    import Icon from "./Icon.svelte";
    import Sparkline from "./Sparkline.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import SensorModal from "../modals/SensorModal.svelte";
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
            const r = await api.sensorReadings(sensor.id, { limit: 60, since_minutes: 24 * 60 });
            readings = r;
        } catch (e) {
            toasts.error("Sensor history failed", (e as Error).message);
        } finally {
            loading = false;
        }
    }
    // Re-fetch when the sensor gets a new reading (data.refresh polls
    // every 30s, so this keeps the sparkline live).
    $effect(() => {
        sensor.last_reading_at;
        load();
    });

    const tone = $derived(toneForKind(sensor.kind));
    const iconName = $derived(iconForKind(sensor.kind));
    const sparkValues = $derived(readings.map(r => r.value));

    const lastFormatted = $derived(formatValue(sensor.last_value));
    const lastAgo = $derived(sensor.last_reading_at ? agoString(sensor.last_reading_at) : "no data yet");

    // An alert fires when the latest reading crosses a configured threshold.
    const alert = $derived.by(() => {
        const v = sensor.last_value;
        if (v === undefined || v === null) return null;
        if (sensor.alert_min !== undefined && v < sensor.alert_min) return `Below ${sensor.alert_min}${sensor.unit}`;
        if (sensor.alert_max !== undefined && v > sensor.alert_max) return `Above ${sensor.alert_max}${sensor.unit}`;
        return null;
    });

    function toneForKind(kind: string): "warn" | "info" | "success" | "danger" | "primary" {
        switch (kind) {
            case "temperature": return "warn";
            case "humidity":    return "info";
            case "motion":      return "primary";
            case "light":       return "success";
            case "power":       return "danger";
            default:            return "info";
        }
    }
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
        const t = new Date(iso).getTime();
        const diffSec = Math.max(0, Math.round((Date.now() - t) / 1000));
        if (diffSec < 60) return `${diffSec}s ago`;
        const m = Math.round(diffSec / 60);
        if (m < 60) return `${m} min ago`;
        const h = Math.round(m / 60);
        if (h < 24) return `${h}h ago`;
        const d = Math.round(h / 24);
        return `${d}d ago`;
    }
</script>

<div class="card sensor" class:compact class:alerting={alert} data-tone={tone}>
    <div class="header">
        <div class="ico"><Icon name={iconName} size={18} /></div>
        <div class="title-wrap">
            <div class="title">{sensor.name}</div>
            <div class="sub">{sensor.room || sensor.kind}</div>
        </div>
        {#if alert}
            <span class="alert-badge" title={alert}>
                <Icon name="bolt" size={12} /> Alert
            </span>
        {/if}
        {#if !compact}
            <button class="icon-btn" aria-label="Edit sensor"
                onclick={() => openModal(SensorModal, { existing: sensor })}>
                <Icon name="edit" size={16} />
            </button>
        {/if}
    </div>

    <div class="value">
        <span class="num">{lastFormatted}</span>
        {#if sensor.unit}<span class="unit">{sensor.unit}</span>{/if}
    </div>

    <div class="spark">
        {#if loading}
            <div class="spark-placeholder">Loading…</div>
        {:else if sparkValues.length >= 2}
            <Sparkline values={sparkValues} width={220} height={36} />
        {:else}
            <div class="spark-placeholder">Waiting for readings…</div>
        {/if}
    </div>

    <div class="footer">{lastAgo}</div>
</div>

<style>
    .sensor {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        min-width: 0;
    }
    .header {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .ico {
        width: 36px; height: 36px;
        border-radius: var(--radius-md);
        display: grid; place-items: center;
        flex-shrink: 0;
    }
    .sensor[data-tone="warn"]    .ico { background: var(--warn-soft);    color: var(--warn);    }
    .sensor[data-tone="warn"]    .spark { color: var(--warn);    }
    .sensor[data-tone="info"]    .ico { background: var(--info-soft);    color: var(--info);    }
    .sensor[data-tone="info"]    .spark { color: var(--info);    }
    .sensor[data-tone="success"] .ico { background: var(--success-soft); color: var(--success); }
    .sensor[data-tone="success"] .spark { color: var(--success); }
    .sensor[data-tone="danger"]  .ico { background: var(--danger-soft);  color: var(--danger);  }
    .sensor[data-tone="danger"]  .spark { color: var(--danger);  }
    .sensor[data-tone="primary"] .ico { background: var(--info-soft);    color: var(--info);    }
    .sensor[data-tone="primary"] .spark { color: var(--info);    }
    .title-wrap { min-width: 0; flex: 1; }
    .title { font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .sub {
        color: var(--text-muted);
        font-size: 12px;
        text-transform: capitalize;
    }
    .icon-btn {
        background: transparent;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        width: 32px; height: 32px;
        display: grid; place-items: center;
        cursor: pointer;
        color: var(--text-muted);
    }
    .icon-btn:hover { color: var(--text); background: var(--surface-hover); }

    .value {
        display: flex;
        align-items: baseline;
        gap: 4px;
    }
    .num {
        font-size: 2rem;
        font-weight: 700;
        font-variant-numeric: tabular-nums;
        line-height: 1;
    }
    .unit { color: var(--text-muted); font-weight: 500; }

    .spark { color: var(--info); height: 36px; }
    .spark-placeholder {
        color: var(--text-faint);
        font-size: 12px;
        display: grid; place-items: center;
        height: 36px;
    }
    .footer {
        font-size: 12px;
        color: var(--text-muted);
    }

    .sensor.compact { padding: var(--space-3); gap: var(--space-2); }
    .sensor.compact .num { font-size: 1.5rem; }

    .sensor.alerting {
        border-color: var(--danger);
        box-shadow: inset 0 0 0 1px var(--danger-soft);
    }
    .sensor.alerting .num { color: var(--danger); }
    .alert-badge {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        font-size: 11px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--danger);
        background: var(--danger-soft);
        padding: 3px 8px;
        border-radius: 999px;
        flex-shrink: 0;
    }
</style>

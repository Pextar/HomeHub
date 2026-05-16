<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import SensorCard from "../components/SensorCard.svelte";
    import LineChart from "../components/LineChart.svelte";
    import Icon from "../components/Icon.svelte";
    import Segmented from "../components/Segmented.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import SensorModal from "../modals/SensorModal.svelte";
    import { scale, fly } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";
    import type { Sensor, SensorReading } from "../lib/types";

    const v = $derived(data.value);

    let selectedId = $state<string | null>(null);
    let range = $state<"1h" | "24h" | "7d">("24h");
    let detailReadings = $state<SensorReading[]>([]);
    let detailLoading = $state(false);

    // Pick the first sensor by default once data has loaded.
    $effect(() => {
        if (selectedId === null && v.sensors.length > 0) {
            selectedId = v.sensors[0].id;
        }
        if (selectedId && !v.sensors.find(s => s.id === selectedId)) {
            selectedId = v.sensors[0]?.id ?? null;
        }
    });

    const selected = $derived<Sensor | undefined>(v.sensors.find(s => s.id === selectedId));

    $effect(() => {
        if (!selected) { detailReadings = []; return; }
        loadDetail(selected.id, range);
    });

    async function loadDetail(id: string, r: typeof range) {
        const minutes = r === "1h" ? 60 : r === "24h" ? 24 * 60 : 7 * 24 * 60;
        detailLoading = true;
        try {
            detailReadings = await api.sensorReadings(id, { since_minutes: minutes, limit: 500 });
        } catch (e) {
            toasts.error("Couldn't load readings", (e as Error).message);
            detailReadings = [];
        } finally {
            detailLoading = false;
        }
    }

    function formatBig(v: number): string {
        if (Math.abs(v) >= 100) return v.toFixed(0);
        if (Math.abs(v) >= 10) return v.toFixed(1);
        return v.toFixed(2);
    }
</script>

<Topbar title="Sensors" subtitle="{v.sensors.length} configured">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(SensorModal, {})}>Add sensor</button>
    {/snippet}
</Topbar>

{#if v.sensors.length === 0}
    <EmptyState icon="sensor" title="No sensors yet"
        message="Add a 433MHz sensor to start charting its readings.">
        <button class="btn btn-primary" onclick={() => openModal(SensorModal, {})}>Add sensor</button>
    </EmptyState>
{:else}
    {#if selected}
        <section class="card detail" in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}>
            <header class="detail-head">
                <div class="title-block">
                    <h2>{selected.name}</h2>
                    <div class="sub">
                        <span class="kind">{selected.kind}</span>
                        {#if selected.room}· {selected.room}{/if}
                        {#if selected.code}· <code>{selected.code}</code>{/if}
                    </div>
                </div>
                <Segmented
                    name="sensor-range"
                    bind:value={range}
                    options={[
                        { value: "1h",  label: "1h" },
                        { value: "24h", label: "24h" },
                        { value: "7d",  label: "7d" },
                    ]}
                />
            </header>

            <div class="big-value">
                {#if selected.last_value !== undefined && selected.last_value !== null}
                    <span class="bv">{formatBig(selected.last_value)}</span>
                    {#if selected.unit}<span class="bu">{selected.unit}</span>{/if}
                {:else}
                    <span class="bv muted">—</span>
                {/if}
                <button class="icon-btn" aria-label="Edit sensor"
                    onclick={() => openModal(SensorModal, { existing: selected })}>
                    <Icon name="edit" size={16} />
                </button>
            </div>

            <LineChart readings={detailReadings} unit={selected.unit} />
            {#if detailLoading}<div class="loading">Loading…</div>{/if}
        </section>
    {/if}

    <section class="card">
        <div class="card-header"><h2>All sensors</h2></div>
        <div class="grid">
            {#each v.sensors as s, i (s.id)}
                <button class="grid-item" type="button" onclick={() => selectedId = s.id}
                    class:active={selectedId === s.id}
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.96, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <SensorCard sensor={s} />
                </button>
            {/each}
        </div>
    </section>
{/if}

<style>
    .detail { display: flex; flex-direction: column; gap: var(--space-4); }
    .detail-head {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-4);
        flex-wrap: wrap;
    }
    .title-block h2 { margin: 0; }
    .sub {
        color: var(--text-muted);
        font-size: 13px;
        margin-top: 2px;
        display: flex; gap: 6px; flex-wrap: wrap;
    }
    .sub .kind { text-transform: capitalize; }
    .sub code {
        background: var(--surface);
        border-radius: 4px;
        padding: 1px 6px;
        font-size: 12px;
    }
    .big-value {
        display: flex;
        align-items: baseline;
        gap: 6px;
    }
    .bv {
        font-size: 3rem;
        font-weight: 700;
        line-height: 1;
        font-variant-numeric: tabular-nums;
    }
    .bv.muted { color: var(--text-faint); }
    .bu { color: var(--text-muted); font-weight: 500; font-size: 1.25rem; }
    .icon-btn {
        margin-left: auto;
        background: transparent;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        width: 32px; height: 32px;
        display: grid; place-items: center;
        cursor: pointer;
        color: var(--text-muted);
    }
    .icon-btn:hover { color: var(--text); background: var(--surface-hover); }
    .loading { color: var(--text-muted); font-size: 12px; }

    .grid {
        display: grid;
        grid-template-columns: 1fr;
        gap: var(--space-3);
    }
    @media (min-width: 600px) {
        .grid { grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); }
    }
    .grid-item {
        text-align: left;
        background: transparent;
        border: 0;
        padding: 0;
        cursor: pointer;
        border-radius: var(--radius-lg);
        transition: transform var(--t-fast);
    }
    .grid-item:hover { transform: translateY(-1px); }
    .grid-item.active :global(.card.sensor) {
        outline: 2px solid var(--primary);
        outline-offset: 2px;
    }
</style>

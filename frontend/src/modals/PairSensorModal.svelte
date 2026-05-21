<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal, openModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import SensorModal from "./SensorModal.svelte";
    import { onMount, onDestroy } from "svelte";
    import { fly } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";
    import type { DiscoveryCandidate } from "../lib/types";

    let candidates = $state<DiscoveryCandidate[]>([]);
    let until = $state<number>(0);
    let now = $state<number>(Date.now());
    let starting = $state(true);
    let error = $state<string | null>(null);
    let pollTimer: ReturnType<typeof setInterval> | null = null;
    let clockTimer: ReturnType<typeof setInterval> | null = null;

    const remaining = $derived(Math.max(0, Math.ceil((until - now) / 1000)));
    const active = $derived(remaining > 0);

    // Existing codes — used to filter out anything the user has already
    // adopted, in case the backend buffer overlaps a sensor we just saved.
    const knownCodes = $derived(new Set(data.value.sensors.map(s => s.code)));
    const visible = $derived(candidates.filter(c => !knownCodes.has(c.code)));

    onMount(async () => {
        try {
            const res = await api.startSensorPair(60);
            until = new Date(res.until).getTime();
            starting = false;
            await poll();
            pollTimer = setInterval(poll, 1500);
            clockTimer = setInterval(() => { now = Date.now(); }, 250);
        } catch (e) {
            starting = false;
            error = (e as Error).message;
            toasts.error("Couldn't start pairing", error);
        }
    });

    onDestroy(() => {
        if (pollTimer) clearInterval(pollTimer);
        if (clockTimer) clearInterval(clockTimer);
    });

    async function poll() {
        try {
            const res = await api.discoverSensors();
            candidates = res.candidates;
            until = new Date(res.until).getTime();
        } catch {
            // Transient errors during polling are silent; the user can retry.
        }
    }

    async function restart() {
        try {
            const res = await api.startSensorPair(60);
            until = new Date(res.until).getTime();
            candidates = [];
            error = null;
            if (!pollTimer) pollTimer = setInterval(poll, 1500);
            if (!clockTimer) clockTimer = setInterval(() => { now = Date.now(); }, 250);
        } catch (e) {
            error = (e as Error).message;
            toasts.error("Couldn't restart pairing", error);
        }
    }

    function adopt(c: DiscoveryCandidate) {
        const field = pickField(c);
        closeModal();
        openModal(SensorModal, {
            prefill: { code: c.code, protocol: c.protocol || "rtl_433", field },
        });
    }

    // pickField guesses which JSON field the user is most likely to want
    // charted: temperature/humidity/lux/power if present, otherwise the
    // first numeric field the packet carried.
    function pickField(c: DiscoveryCandidate): string {
        const keys = Object.keys(c.fields);
        const prefer = ["temperature_C", "temperature_F", "temperature", "humidity", "lux", "power_W", "battery_ok"];
        for (const p of prefer) {
            if (keys.includes(p)) return p;
        }
        return keys[0] ?? "";
    }

    function fieldSummary(c: DiscoveryCandidate): string {
        const entries = Object.entries(c.fields);
        if (entries.length === 0) return "no numeric fields";
        return entries
            .slice(0, 4)
            .map(([k, v]) => `${k}=${formatVal(v)}`)
            .join(", ");
    }

    function formatVal(v: number): string {
        if (Number.isInteger(v)) return String(v);
        return v.toFixed(2);
    }
</script>

<Modal
    title="Pair sensor"
    subtitle={active
        ? `Listening for unknown 433MHz emitters — ${remaining}s left.`
        : starting
            ? "Starting…"
            : "Pair window closed."}
>
    {#snippet body()}
        {#if starting}
            <p class="muted">Opening pair window…</p>
        {:else if error}
            <p class="err">{error}</p>
        {:else}
            <div class="hint">
                Trigger your sensor (press its button, walk in front of motion
                sensor, wait for it to report). Any unknown packet that
                arrives in the window will appear below.
            </div>

            {#if visible.length === 0}
                <div class="empty">
                    {#if active}
                        <div class="pulse" aria-hidden="true"></div>
                        Waiting for packets…
                    {:else}
                        Nothing heard. Click <strong>Listen again</strong> below.
                    {/if}
                </div>
            {:else}
                <ul class="list" role="list">
                    {#each visible as c (c.code)}
                        <li animate:flip={{ duration: dur(220), easing: cubicOut }}
                            in:fly={{ y: 6, duration: dur(180), easing: cubicOut }}>
                            <button class="row" type="button" onclick={() => adopt(c)}>
                                <div class="row-main">
                                    <div class="code">{c.code}</div>
                                    <div class="fields">{fieldSummary(c)}</div>
                                </div>
                                <div class="row-meta">
                                    <span class="badge">{c.count}×</span>
                                    <span class="adopt">Adopt →</span>
                                </div>
                            </button>
                        </li>
                    {/each}
                </ul>
            {/if}
        {/if}
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Close</button>
        {#if !active && !starting}
            <button class="btn btn-primary" onclick={restart}>Listen again</button>
        {/if}
    {/snippet}
</Modal>

<style>
    .muted { color: var(--text-muted); }
    .err { color: var(--danger); }
    .hint {
        font-size: 13px;
        color: var(--text-muted);
        background: var(--surface);
        border-radius: var(--radius-sm);
        padding: var(--space-3);
    }

    .empty {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        color: var(--text-muted);
        font-size: 14px;
        padding: var(--space-4) 0;
        justify-content: center;
    }
    .pulse {
        width: 10px; height: 10px;
        border-radius: 50%;
        background: var(--primary);
        animation: pulse 1.2s ease-in-out infinite;
    }
    @keyframes pulse {
        0%, 100% { opacity: 0.35; transform: scale(0.9); }
        50%      { opacity: 1;    transform: scale(1.15); }
    }

    .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
    .row {
        width: 100%;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-3);
        cursor: pointer;
        text-align: left;
        color: inherit;
    }
    .row:hover { background: var(--surface-hover); border-color: var(--primary); }
    .row:active { transform: scale(0.99); transition: transform 60ms ease; }
    .row-main { min-width: 0; flex: 1; }
    .code { font-family: var(--font-mono, monospace); font-weight: 600; word-break: break-all; }
    .fields { font-size: 12px; color: var(--text-muted); margin-top: 2px; word-break: break-all; }
    .row-meta { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; }
    .badge {
        font-size: 11px;
        color: var(--text-muted);
        background: var(--bg-elevated);
        border-radius: 999px;
        padding: 2px 8px;
        font-variant-numeric: tabular-nums;
    }
    .adopt { font-size: 12px; color: var(--primary); font-weight: 600; }
</style>

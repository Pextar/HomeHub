<script lang="ts">
    import { untrack } from "svelte";
    import { fly, fade } from "svelte/transition";
    import { backOut } from "svelte/easing";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { DAY_SHORT, DAY_NAMES } from "../lib/utils";
    import type { Socket, Schedule } from "../lib/types";

    interface Props {
        onClose: () => void;
        existing?: Schedule | null;
    }
    let { onClose, existing = null }: Props = $props();

    const lamps = $derived(data.value.sockets);
    const isEdit = $derived(existing !== null);

    let selectedId = $state<string>(
        untrack(() => existing?.target_id || existing?.socket_id || "")
    );
    // Default to the first lamp when nothing is pre-selected.
    $effect(() => {
        if (!selectedId && lamps.length > 0) selectedId = lamps[0].id;
    });

    let action = $state<"on" | "off">(untrack(() => existing?.action === "off" ? "off" : "on"));
    let time = $state(untrack(() => existing?.time || "20:00"));
    let days = $state<number[]>(untrack(() => [...(existing?.days ?? [])]));
    let saving = $state(false);

    function lampEmoji(lamp: Socket): string {
        return lamp.emoji?.trim() ? lamp.emoji : "💡";
    }

    function toggleDay(idx: number) {
        if (days.includes(idx)) {
            days = days.filter(d => d !== idx);
        } else {
            days = [...days, idx].sort((a, b) => a - b);
        }
    }

    async function save() {
        if (saving || !selectedId) return;
        saving = true;
        try {
            const payload = {
                target_type: "socket",
                target_id: selectedId,
                action,
                time_mode: "fixed",
                time,
                solar_offset_minutes: 0,
                days,
                enabled: true,
                random_offset_minutes: 0,
            };
            if (existing) {
                await api.updateSchedule(existing.id, payload);
                toasts.success("Schedule saved!");
            } else {
                await api.createSchedule(payload);
                toasts.success("Schedule added!");
            }
            await data.refresh();
            onClose();
        } catch (e) {
            toasts.error("Oops!", (e as Error).message);
        } finally {
            saving = false;
        }
    }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="ks-overlay" transition:fade={{ duration: 200 }}
     onclick={onClose} role="presentation">
    <div class="ks-panel"
         onclick={(e) => e.stopPropagation()}
         onkeydown={(e) => e.stopPropagation()}
         in:fly={{ y: 60, duration: 350, easing: backOut }}
         role="dialog" aria-label="Schedule a lamp" aria-modal="true" tabindex="-1">

        <header class="ks-head">
            <h2>⏰ {isEdit ? "Edit schedule" : "New schedule"}</h2>
            <button class="ks-close" onclick={onClose} aria-label="Close">✕</button>
        </header>

        <!-- Which lamp? -->
        <div class="ks-section">
            <p class="ks-label">Which lamp?</p>
            <div class="ks-lamp-grid">
                {#each lamps as lamp (lamp.id)}
                    <button class="ks-lamp"
                        class:sel={selectedId === lamp.id}
                        onclick={() => (selectedId = lamp.id)}
                        aria-pressed={selectedId === lamp.id}>
                        <span class="ks-lamp-emoji">{lampEmoji(lamp)}</span>
                        <span class="ks-lamp-name">{lamp.name}</span>
                    </button>
                {/each}
            </div>
        </div>

        <!-- What? -->
        <div class="ks-section">
            <p class="ks-label">What should happen?</p>
            <div class="ks-action-row">
                <button class="ks-act ks-on"
                    class:sel={action === "on"}
                    onclick={() => (action = "on")}
                    aria-pressed={action === "on"}>
                    💡 Turn ON
                </button>
                <button class="ks-act ks-off"
                    class:sel={action === "off"}
                    onclick={() => (action = "off")}
                    aria-pressed={action === "off"}>
                    🌙 Turn OFF
                </button>
            </div>
        </div>

        <!-- When? -->
        <div class="ks-section">
            <p class="ks-label">What time?</p>
            <input class="ks-time" type="time" bind:value={time} aria-label="Schedule time" />
        </div>

        <!-- Which days? -->
        <div class="ks-section">
            <p class="ks-label">
                Which days?
                {#if days.length === 0}<span class="ks-hint">every day</span>{/if}
            </p>
            <div class="ks-days" role="group" aria-label="Days of the week">
                {#each DAY_SHORT as d, i}
                    <button class="ks-day"
                        class:sel={days.includes(i)}
                        onclick={() => toggleDay(i)}
                        aria-label={DAY_NAMES[i]}
                        aria-pressed={days.includes(i)}>
                        {d}
                    </button>
                {/each}
            </div>
        </div>

        <div class="ks-footer">
            <button class="ks-btn ks-cancel" onclick={onClose}>Cancel</button>
            <button class="ks-btn ks-save" onclick={save}
                disabled={saving || !selectedId}>
                {saving ? "Saving…" : isEdit ? "Save" : "Add it! ✓"}
            </button>
        </div>
    </div>
</div>

<style>
    .ks-overlay {
        position: fixed;
        inset: 0;
        z-index: 150;
        background: rgba(10, 12, 24, 0.6);
        backdrop-filter: blur(6px);
        display: flex;
        align-items: flex-end;
        justify-content: center;
    }
    @media (min-width: 700px) {
        .ks-overlay { align-items: center; }
    }

    .ks-panel {
        width: 100%;
        max-width: 560px;
        max-height: 92vh;
        overflow-y: auto;
        background: var(--bg-elevated);
        border-top-left-radius: var(--radius-xl);
        border-top-right-radius: var(--radius-xl);
        padding: var(--space-5);
        padding-bottom: calc(var(--space-6) + env(safe-area-inset-bottom));
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
    }
    @media (min-width: 700px) {
        .ks-panel { border-radius: var(--radius-xl); }
    }

    /* ── Header ── */
    .ks-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .ks-head h2 {
        font-size: clamp(1.3rem, 5vw, 1.75rem);
        font-weight: 800;
        letter-spacing: -0.02em;
    }
    .ks-close {
        width: 48px;
        height: 48px;
        border-radius: 50%;
        border: none;
        background: var(--surface-hover);
        color: var(--text);
        font-size: 1.4rem;
        font-weight: 700;
        cursor: pointer;
        flex-shrink: 0;
        -webkit-tap-highlight-color: transparent;
    }
    .ks-close:active { transform: scale(0.92); }

    /* ── Sections ── */
    .ks-section { display: flex; flex-direction: column; gap: var(--space-3); }

    .ks-label {
        font-size: 0.78rem;
        font-weight: 700;
        letter-spacing: 0.1em;
        text-transform: uppercase;
        color: var(--text-muted);
    }
    .ks-hint {
        font-weight: 500;
        text-transform: none;
        letter-spacing: 0;
        margin-left: 4px;
        opacity: 0.75;
    }

    /* ── Lamp grid ── */
    .ks-lamp-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(80px, 1fr));
        gap: var(--space-2);
    }
    .ks-lamp {
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: 5px;
        padding: 10px 6px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg);
        cursor: pointer;
        transition: border-color 0.15s ease, background 0.15s ease, transform 0.1s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .ks-lamp:active { transform: scale(0.93); }
    .ks-lamp.sel {
        border-color: #ffd23f;
        background: linear-gradient(160deg, #2b2011, #1e1a0e);
    }
    .ks-lamp-emoji {
        font-size: 1.9rem;
        line-height: 1;
    }
    .ks-lamp-name {
        font-size: 0.72rem;
        font-weight: 700;
        color: var(--text);
        text-align: center;
        line-height: 1.2;
    }

    /* ── Action buttons ── */
    .ks-action-row { display: flex; gap: var(--space-3); }
    .ks-act {
        flex: 1;
        padding: 16px 12px;
        font-size: 1.1rem;
        font-weight: 800;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg);
        color: var(--text-muted);
        cursor: pointer;
        min-height: 56px;
        transition: all 0.18s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .ks-act:active { transform: scale(0.95); }
    .ks-on.sel {
        background: linear-gradient(160deg, #fff3c4, #ffd23f);
        border-color: #ffd23f;
        color: #5e4500;
    }
    .ks-off.sel {
        background: #1a2140;
        border-color: #4460a8;
        color: #a0bcf0;
    }

    /* ── Time input ── */
    .ks-time {
        width: 100%;
        text-align: center;
        font-size: 2.2rem;
        font-weight: 800;
        font-family: var(--font-mono);
        padding: 10px 16px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg);
        color: var(--text);
        outline: none;
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
    }
    @media (pointer: coarse), (max-width: 600px) {
        .ks-time { font-size: 1.6rem; min-height: 58px; }
    }

    /* ── Day chips ── */
    .ks-days {
        display: flex;
        gap: var(--space-2);
        justify-content: space-between;
    }
    .ks-day {
        flex: 1;
        aspect-ratio: 1;
        min-width: 36px;
        max-width: 54px;
        border-radius: 50%;
        border: 2px solid var(--border);
        background: var(--bg);
        color: var(--text-muted);
        font-size: 0.85rem;
        font-weight: 800;
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: center;
        transition: all 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .ks-day:active { transform: scale(0.85); }
    .ks-day.sel {
        background: #ffd23f;
        border-color: #ffd23f;
        color: #5e4500;
    }
    @media (pointer: coarse) {
        .ks-day { min-width: 40px; min-height: 40px; }
    }

    /* ── Footer ── */
    .ks-footer { display: flex; gap: var(--space-3); }
    .ks-btn {
        flex: 1;
        padding: 16px;
        font-size: 1.05rem;
        font-weight: 800;
        border: none;
        border-radius: var(--radius-lg);
        cursor: pointer;
        transition: transform 0.15s ease, opacity 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .ks-btn:active { transform: scale(0.95); }
    .ks-btn:disabled { opacity: 0.55; cursor: not-allowed; transform: none; }
    .ks-cancel {
        background: var(--surface-hover);
        color: var(--text-muted);
    }
    .ks-save {
        flex: 2;
        background: linear-gradient(160deg, #fff3c4, #ffd23f);
        color: #5e4500;
    }

    @media (prefers-reduced-motion: reduce) {
        .ks-lamp, .ks-act, .ks-day, .ks-btn, .ks-close { transition: none; }
    }
</style>

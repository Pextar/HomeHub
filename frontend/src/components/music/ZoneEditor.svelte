<script lang="ts">
    /**
     * Naming a room the user built, and choosing what is in it.
     *
     * Dragging one room card onto another is how most of these come into
     * existence, and it needs no form at all. This is for the rest: renaming
     * one, adding a fourth speaker to it, dropping one out, deleting it.
     *
     * It is a form, so §11 gives it the sheet shape and the sticky footer, and
     * it reaches that shape by *swapping* with whatever raised it rather than
     * opening over it — a sheet must never open another sheet
     * (`lib/sheet-run.ts`).
     *
     * It claims nothing about how the room will play. The route is the
     * backend's decision per playback and it reports one on every read;
     * predicting it here would mean a second copy of the route engine in the
     * UI, drifting. The player says what the room will actually do.
     */
    import Icon from "../Icon.svelte";
    import MusicSheet from "./MusicSheet.svelte";
    import QuietCard from "./QuietCard.svelte";
    import { untrack } from "svelte";
    import type { MediaEndpoint, MediaVendor, MediaZone } from "../../lib/types";
    import type { ZonesBridge } from "../../lib/music/zones.svelte";

    let {
        /** Null when creating. */
        zone = null,
        zones,
        onCancel,
        /** Saved: the caller decides where to go next. */
        onSaved,
        onDelete,
        onOpenSpeakers,
        scrollEl = $bindable<HTMLElement | null>(null),
        dismissing = $bindable(false),
    }: {
        zone?: MediaZone | null;
        zones: ZonesBridge;
        onCancel: () => void;
        onSaved: (z: MediaZone) => void;
        onDelete: (z: MediaZone) => void;
        onOpenSpeakers: () => void;
        scrollEl?: HTMLElement | null;
        dismissing?: boolean;
    } = $props();

    /** The makes, in the order the picker lists them. */
    const VENDORS: { id: MediaVendor; label: string }[] = [
        { id: "sonos", label: "Sonos" },
        { id: "kef", label: "KEF" },
    ];

    let name = $state(untrack(() => zone?.name ?? ""));
    let nameError = $state("");
    let saving = $state(false);
    // Ticked members, in the order they were ticked — order is meaningful:
    // the first member that can lead becomes the coordinator for the routes
    // that need one, so the set alone would lose something the backend uses.
    let picked = $state<string[]>(untrack(() => [...(zone?.members ?? [])]));
    const pickedSet = $derived(new Set(picked));

    const byVendor = $derived.by(() => {
        // Rebuilt by the derivation, never mutated after — a reactive map
        // would have nothing to observe.
        // eslint-disable-next-line svelte/prefer-svelte-reactivity
        const out = new Map<MediaVendor, MediaEndpoint[]>();
        for (const v of VENDORS) {
            const list = zones.endpoints.filter((e) => e.vendor === v.id);
            if (list.length) out.set(v.id, list);
        }
        return out;
    });

    function toggle(member: string) {
        picked = pickedSet.has(member)
            ? picked.filter((m) => m !== member)
            : [...picked, member];
    }

    /** Other rooms a speaker already belongs to — worth knowing before a
     *  speaker ends up in two that both try to play to it. */
    function alsoIn(member: string): string {
        return zones
            .zonesWith(member)
            .filter((z) => z.id !== zone?.id)
            .map((z) => z.name)
            .join(", ");
    }

    async function save() {
        if (saving) return;
        const trimmed = name.trim();
        if (!trimmed) {
            nameError = "Give the room a name.";
            return;
        }
        saving = true;
        try {
            const saved = zone
                ? // The room rides along unchanged: the update replaces it
                  // wholesale, so not sending it would quietly clear a field
                  // this form doesn't offer.
                  await zones.update(zone.id, { name: trimmed, members: picked, room: zone.room })
                : await zones.create({ name: trimmed, members: picked });
            if (saved) onSaved(saved);
        } finally {
            saving = false;
        }
    }
</script>

<MusicSheet
    label={zone ? "Edit room" : "New room"}
    title={zone ? zone.name : "New room"}
    sub="Speakers that play together — any mix of makes"
    backIcon="chevronLeft"
    backLabel="Back"
    onBack={onCancel}
    onDismiss={onCancel}
    bind:scrollEl
    bind:dismissing
>
    <form
        class="z-form"
        onsubmit={(e) => {
            e.preventDefault();
            void save();
        }}
    >
        <div class="field">
            <label for="zone-name">Name</label>
            <input
                id="zone-name"
                type="text"
                bind:value={name}
                placeholder="e.g. Downstairs"
                autocomplete="off"
                aria-invalid={nameError ? "true" : undefined}
                aria-describedby={nameError ? "zone-name-err" : undefined}
                oninput={() => (nameError = "")}
            />
            {#if nameError}<div id="zone-name-err" class="field-error">{nameError}</div>{/if}
        </div>

        <div class="field">
            <span class="field-label">Speakers</span>
            {#if zones.endpoints.length === 0}
                <!-- Nothing until the first read answers: claiming "no
                     speakers" on a slow network is the same false claim
                     Zones itself avoids for "no zones yet". -->
                {#if zones.endpointsLoaded}
                    <QuietCard
                        title="No speakers to add"
                        action={{ label: "Speakers", onClick: onOpenSpeakers }}
                    >
                        Register a Sonos or KEF speaker and it can join a room.
                    </QuietCard>
                {/if}
            {:else}
                {#each VENDORS as v (v.id)}
                    {@const list = byVendor.get(v.id) ?? []}
                    {#if list.length}
                        <div class="eyrow z-make">{v.label}</div>
                        <div class="z-picker">
                            {#each list as e (e.member)}
                                {@const on = pickedSet.has(e.member)}
                                {@const other = alsoIn(e.member)}
                                <label class="z-row" class:on>
                                    <input
                                        type="checkbox"
                                        checked={on}
                                        onchange={() => toggle(e.member)}
                                    />
                                    <span class="z-row-meta">
                                        <span class="z-row-name">{e.name}</span>
                                        <span class="field-help">
                                            {e.model || v.label}{#if e.room} · {e.room}{/if}{#if other}
                                                · also in {other}{/if}
                                        </span>
                                    </span>
                                    {#if on}
                                        <span class="z-order mono"
                                            >{picked.indexOf(e.member) + 1}</span
                                        >
                                    {/if}
                                </label>
                            {/each}
                        </div>
                    {/if}
                {/each}
                <div class="field-help">
                    Tick everything that should play together. The order you tick them in is the
                    order the room keeps — the first that can lead does.
                </div>
            {/if}
        </div>

        {#if zone}
            <button type="button" class="z-delete" onclick={() => onDelete(zone)}>
                <Icon name="trash" size={15} /> Delete this room
            </button>
        {/if}

        <!-- §11's form footer: amber primary, card secondary, and it sticks so
             a long picker never scrolls Save off the screen. -->
        <div class="z-foot">
            <button type="button" class="btn" onclick={onCancel}>Cancel</button>
            <button type="submit" class="btn btn-primary" disabled={saving}>
                {saving ? "Saving…" : zone ? "Save room" : "Create room"}
            </button>
        </div>
    </form>
</MusicSheet>

<style>
    .z-form { display: flex; flex-direction: column; gap: var(--space-5); }
    .z-make { margin-top: var(--space-2); }

    .z-picker { display: flex; flex-direction: column; gap: 4px; }
    .z-row {
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 52px; padding: 8px var(--space-3);
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        cursor: pointer;
        transition: border-color var(--t-fast), background var(--t-fast);
    }
    .z-row.on { background: var(--on-soft); border-color: var(--tile-on-border); }
    @media (hover: hover) { .z-row:hover { border-color: var(--border-strong); } }
    .z-row input { width: auto; padding: 0; flex-shrink: 0; }
    .z-row-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .z-row-name {
        font-size: 13.5px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* Position in the room, because it decides which speaker can lead. */
    .z-order { font-size: 11px; color: var(--on); flex-shrink: 0; }
    @media (pointer: coarse) {
        .z-row { min-height: 56px; }
    }

    .z-delete {
        align-self: flex-start;
        display: flex; align-items: center; gap: 6px;
        min-height: 44px; padding: 0 var(--space-2);
        background: none; border: 0;
        color: var(--bad); font: inherit; font-size: 13px; cursor: pointer;
    }
    .z-delete:hover { text-decoration: underline; }

    .z-foot {
        position: sticky; bottom: 0; z-index: 2;
        display: grid; grid-template-columns: 1fr 2fr; gap: var(--space-3);
        padding: var(--space-3) 0 var(--space-2);
        background: var(--bg-bar);
        backdrop-filter: blur(18px) saturate(1.3);
        -webkit-backdrop-filter: blur(18px) saturate(1.3);
    }

    @media (prefers-reduced-motion: reduce) {
        .z-row { transition-duration: 0.001ms; }
    }
</style>

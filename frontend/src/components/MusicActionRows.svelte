<script lang="ts">
    import Icon from "./Icon.svelte";
    import type { MusicAction, MusicActionKind } from "../lib/types";
    import type { MediaRoomOption } from "../lib/media-rooms";

    /**
     * The music half of a scene step or an automation rule: one row per room,
     * a verb, and a level where the verb needs one.
     *
     * One component for both editors, for the reason the catalog has one
     * track row (DESIGN.md §14): a second copy is the one that drifts, and
     * these rows are the whole vocabulary the house has for "and do this to
     * the sound".
     *
     * Three verbs, and no fourth without a picker to go with it. A scene is a
     * *moment* — "we're watching a film", "everyone's out" — and starting a
     * particular record needs something named to start, which is what the
     * music timers are (Music › a room › Wake up). Offering "play…" here
     * without that would be a control that can't finish its own sentence.
     *
     * One row per room is enforced by the store, so it is enforced here too:
     * a room already spoken for is absent from the next row's picker rather
     * than offered and then refused on save (§15.1).
     */
    let {
        music = $bindable(),
        rooms,
        /** Loading and outage, kept apart: a house with no speakers gets one
         *  sentence, a failed read gets a different one, and neither is an
         *  empty picker somebody taps at. */
        loading = false,
        failed = false,
        idPrefix = "music",
    }: {
        music: MusicAction[];
        rooms: MediaRoomOption[];
        loading?: boolean;
        failed?: boolean;
        idPrefix?: string;
    } = $props();

    const VERBS: { key: MusicActionKind; label: string }[] = [
        { key: "pause", label: "Pause" },
        { key: "resume", label: "Resume" },
        { key: "volume", label: "Set volume" },
    ];

    /** Rooms still free, plus whatever this row already holds. */
    function available(current: string): MediaRoomOption[] {
        // Plain Set: built whole on every call from `music`, read once, and
        // never mutated after — there is nothing here to be reactive about.
        // eslint-disable-next-line svelte/prefer-svelte-reactivity
        const taken = new Set(music.map((m) => m.room));
        taken.delete(current);
        return rooms.filter((r) => !taken.has(r.key));
    }

    const allTaken = $derived(rooms.length > 0 && music.length >= rooms.length);

    function add() {
        const free = available("")[0];
        if (!free) return;
        music = [...music, { room: free.key, action: "pause" }];
    }

    function remove(i: number) {
        music = music.filter((_, idx) => idx !== i);
    }

    function setVerb(i: number, action: MusicActionKind) {
        const next = [...music];
        // A level belongs to "volume" alone. Carried across a verb change it
        // would be sent with a pause and silently dropped by the store —
        // which is worse than losing it here, where the row shows it going.
        next[i] = {
            ...next[i],
            action,
            volume: action === "volume" ? (next[i].volume ?? 30) : undefined,
        };
        music = next;
    }
</script>

<div class="mus">
    {#if loading}
        <div class="mus-note">
            <span class="skeleton sk-line"></span>
        </div>
    {:else if failed}
        <div class="mus-note">Couldn't read the speakers in this house.</div>
    {:else if rooms.length === 0}
        <div class="mus-note">No speakers or zones in this house yet.</div>
    {:else}
        {#each music as row, i (i)}
            <div class="mus-row">
                <select
                    class="mus-room"
                    id="{idPrefix}-room-{i}"
                    value={row.room}
                    onchange={(e) => (row.room = (e.target as HTMLSelectElement).value)}
                    aria-label="Room for music action {i + 1}"
                >
                    {#each available(row.room) as r (r.key)}
                        <option value={r.key}>
                            {r.name}{r.kind === "zone" ? " (zone)" : ""}
                        </option>
                    {/each}
                </select>

                <div class="mus-verbs" role="group" aria-label="What to do in this room">
                    {#each VERBS as vb (vb.key)}
                        <button
                            type="button"
                            class="verb"
                            class:active={row.action === vb.key}
                            onclick={() => setVerb(i, vb.key)}
                        >
                            {vb.label}
                        </button>
                    {/each}
                </div>

                {#if row.action === "volume"}
                    <label class="mus-vol">
                        <input
                            type="range"
                            min="0"
                            max="100"
                            step="1"
                            value={row.volume ?? 30}
                            oninput={(e) => (row.volume = +(e.target as HTMLInputElement).value)}
                            aria-label="Volume for {row.room}"
                        />
                        <span class="mono">{row.volume ?? 30}%</span>
                    </label>
                {/if}

                <button
                    type="button"
                    class="mus-remove"
                    onclick={() => remove(i)}
                    aria-label="Remove this music action"
                >
                    <Icon name="close" size={14} />
                </button>
            </div>
        {/each}

        {#if !allTaken}
            <button type="button" class="add-dashed-btn" onclick={add}>
                <Icon name="plus" size={15} /> Add music
            </button>
        {/if}
    {/if}
</div>

<style>
    .mus {
        display: flex;
        flex-direction: column;
        gap: 8px;
    }
    .mus-note {
        font-size: 12.5px;
        color: var(--text-muted);
    }
    .sk-line {
        display: block;
        height: 14px;
        border-radius: var(--r-sm);
        max-width: 180px;
    }

    /* One row: room, verb, optional level, and the way out. It wraps rather
       than scrolls — a sheet is narrow and a row that ran off the edge would
       hide the control that removes it. */
    .mus-row {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 8px;
        padding: 8px 10px;
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        background: var(--card-2);
    }
    .mus-room {
        flex: 1 1 140px;
        min-width: 0;
        font-size: 13px;
    }
    .mus-verbs {
        display: inline-flex;
        gap: 4px;
    }
    .verb {
        padding: 6px 10px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: transparent;
        color: var(--text-muted);
        font-family: inherit;
        font-size: 12.5px;
        cursor: pointer;
        min-height: 32px;
        touch-action: manipulation;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast);
    }
    .verb.active {
        background: var(--on-soft);
        border-color: transparent;
        color: var(--on);
        font-weight: 600;
    }
    /* The level always takes its own line, so the remove target keeps the
       end of the first one whichever verb is chosen — a control that moves
       between rows depending on their contents is one you have to look for. */
    .mus-vol {
        display: flex;
        align-items: center;
        gap: 10px;
        order: 1;
        flex: 1 1 100%;
    }
    .mus-vol input {
        flex: 1;
        min-width: 80px;
        /* Amber, like every other fill in the app. Without this the browser
           draws its own accent, which on most of them is blue (§12). */
        accent-color: var(--on);
    }
    .mus-vol span {
        font-size: 12px;
        color: var(--text-muted);
        min-width: 38px;
        text-align: right;
    }
    .mus-remove {
        margin-left: auto;
        order: 0;
        min-width: 32px;
        min-height: 32px;
        display: grid;
        place-items: center;
        border: none;
        background: transparent;
        color: var(--text-dim);
        border-radius: var(--r-sm);
        cursor: pointer;
        touch-action: manipulation;
    }
    .mus-remove:hover {
        color: var(--bad);
    }

    .add-dashed-btn {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        width: 100%;
        min-height: 44px;
        padding: 10px 14px;
        border: 1px dashed var(--border-strong);
        border-radius: var(--r-md);
        background: transparent;
        color: var(--text-muted);
        font-family: inherit;
        font-size: 13px;
        cursor: pointer;
        touch-action: manipulation;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast);
    }
    .add-dashed-btn:hover {
        background: var(--surface-hover);
        color: var(--text);
        border-color: var(--text-muted);
    }

    /* Touch: the pill row and the remove target both reach 44px, and the
       select keeps 16px so iOS doesn't zoom the sheet on focus. */
    @media (pointer: coarse), (max-width: 600px) {
        .mus-room {
            font-size: 16px;
        }
        .verb {
            min-height: 40px;
        }
        .mus-remove {
            min-width: 44px;
            min-height: 44px;
        }
    }
    @media (prefers-reduced-motion: reduce) {
        .verb,
        .add-dashed-btn {
            transition-duration: 0.001ms;
        }
    }
</style>

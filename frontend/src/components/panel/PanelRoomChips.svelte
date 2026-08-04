<script lang="ts">
    import Waveform from "../music/Waveform.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // Where the sound goes. This used to sit inside the player card, in a
    // column narrow enough that three rooms wrapped to two rows and six to
    // three — 150px of a 720px panel spent on the control you touch least,
    // taken from the cover you look at most. On the surface's own header it
    // is one line, and the card's height goes to the cover instead (§16).
    let { music }: { music: PanelMusicStore } = $props();

    const featured = $derived(music.featured);

    /** How a chip names its make, for the ones that aren't obvious. */
    function chipTitle(kind: string): string {
        return kind === "zone" ? "HomeHub room" : kind === "kef" ? "KEF speaker" : "Sonos room";
    }
</script>

{#if music.sources.length > 1}
    <div class="p-sources" role="group" aria-label="Room">
        {#each music.sources as s (s.key)}
            <button
                class="p-chip"
                class:active={featured?.key === s.key}
                aria-pressed={featured?.key === s.key}
                title={chipTitle(s.kind)}
                onclick={() => (music.selected = s.key)}
            >
                <!-- Which room is playing, readable from across the room —
                     without having to select each one to find out. -->
                {#if s.playing}<span class="p-chipwave"><Waveform /></span>{/if}
                {s.title}
            </button>
        {/each}
    </div>
{/if}

<style>
    .p-sources {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        min-width: 0;
    }
    .p-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 8px 14px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .p-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .p-chip.active {
        background: var(--text);
        color: var(--bg);
        border-color: var(--text);
    }
    .p-chipwave {
        display: inline-flex;
        margin-right: 1px;
    }

    /* Distance-scaled targets: this is a wall, so every chip on it clears
       the §2 floor rather than inheriting a phone's sizing. */
    @media (pointer: coarse) {
        .p-chip {
            min-height: 44px;
            padding-inline: 16px;
        }
    }
</style>

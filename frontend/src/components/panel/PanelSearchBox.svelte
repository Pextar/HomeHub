<script lang="ts">
    /**
     * The panel's search box, the line under it that names where a tap will
     * land, and the kind chips that filter what came back.
     *
     * The destination line is here rather than left to the results because
     * the results are otherwise the one place on the wall that never names
     * their own room — and a wall is the surface most likely to be used by
     * whoever walked past it last.
     *
     * With the keyboard up the chips and that line stand down. The
     * arithmetic they are answering: the reference wall is 768px tall and
     * its docked landscape keyboard takes ~350 of them, leaving the header,
     * the box and the dock to share what remains. Every row bought back
     * there is a row of results.
     *
     * The wrapper is `display: contents` so these stay direct children of
     * the search column's flex box — a real box here would swallow the gap
     * the column sets between them.
     */
    import { fade } from "svelte/transition";
    import Icon from "../Icon.svelte";
    import { dur } from "../../lib/motion";
    import { SEARCH_KINDS as KINDS } from "../../lib/music/catalog";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { PanelSource } from "../../lib/panel-music.svelte";

    let {
        spotify,
        featured,
        fullBleed = false,
        kbOpen = false,
        liveMessage,
        searchEl = $bindable(null),
        onTyping,
        onQueryKey,
        onClear,
        onDone,
    }: {
        spotify: SpotifyStore;
        featured: PanelSource | undefined;
        /** The results have the whole width, so the way back needs a target. */
        fullBleed?: boolean;
        /** The software keyboard is up: the chips and the room line go. */
        kbOpen?: boolean;
        /** What a screen reader is told about the results, politely. */
        liveMessage: string;
        /** The input itself: the depth focuses and blurs it from a dozen
         *  places (a result tapped, Escape, arriving on the pane). */
        searchEl?: HTMLInputElement | null;
        /** Typing claims the screen — the surest signal there is that this
         *  is a search and not a glance at the player. */
        onTyping: () => void;
        onQueryKey: (e: KeyboardEvent) => void;
        onClear: () => void;
        /** Give the player its column back. */
        onDone: () => void;
    } = $props();

    let boxFocused = $state(false);
</script>

<div class="s-head" class:kb-open={kbOpen}>
        <div class="s-boxrow">
            <div class="s-box">
                <Icon name="search" size={20} />
                <input
                    bind:this={searchEl}
                    value={spotify.query}
                    placeholder={boxFocused
                        ? "Search songs, artists, albums"
                        : "Tap to search"}
                    aria-label="Search music"
                    autocapitalize="off"
                    autocomplete="off"
                    autocorrect="off"
                    spellcheck="false"
                    enterkeyhint="search"
                    oninput={(e) => {
                        onTyping();
                        spotify.query = e.currentTarget.value;
                        spotify.onQueryInput();
                    }}
                    onpointerdown={onTyping}
                    onfocus={() => (boxFocused = true)}
                    onblur={() => (boxFocused = false)}
                    onkeydown={onQueryKey}
                />
                {#if spotify.query}
                    <button class="s-clear" onclick={onClear} aria-label="Clear search">
                        <Icon name="close" size={16} />
                    </button>
                {/if}
            </div>
            <!-- The named way out of the full-bleed search, so
                 giving the player its column back is a target and
                 not a keystroke nobody standing at a wall has.
                 The dock's own cover does the same thing. -->
            {#if fullBleed}
                <button
                    class="k-chip s-done"
                    in:fade={{ duration: dur(120) }}
                    onclick={onDone}>Done</button
                >
            {/if}
        </div>

        <!-- Where a tap lands, said under the box the tapping
             starts in: the results are otherwise the one place on
             the wall that never names their own destination, and
             a wall is the surface most likely to be used by
             whoever walked past it last. It rides on a line of
             its own now that the pane switcher has left the
             column for the header — one thin band replacing a
             whole row, which the results keep. -->
        <p class="s-dest">
            <Icon name="speaker" size={14} />
            <span
                >{featured
                    ? `Plays on ${featured.title}`
                    : "No speaker is answering"}</span
            >
        </p>

        {#if spotify.results}
            {@const r = spotify.results}
            <div class="s-kinds">
                <button
                    class="k-chip"
                    class:active={spotify.kindFilter === "all"}
                    onclick={() => (spotify.kindFilter = "all")}>All</button
                >
                {#each KINDS as k (k.id)}
                    {#if r[k.id].length > 0}
                        <button
                            class="k-chip"
                            class:active={spotify.kindFilter === k.id}
                            onclick={() => (spotify.kindFilter = k.id)}
                            >{k.label}
                            <span class="mono">{r[k.id].length}</span></button
                        >
                    {/if}
                {/each}
            </div>
        {/if}

        <p class="sr-only" role="status" aria-live="polite">{liveMessage}</p>
</div>

<style>
    /* Layout-neutral: the box, the line and the chips stay direct children
       of the search column's flex box. */
    .s-head {
        display: contents;
    }
    /* The catalog stack owns the column's scroll the same way — including
       the one-axis rule: an artist's page is rows of text too, and the
       pixel rounding invents pans it sideways exactly like the queue
       (§12). */
    /* The box was 56px when it shared the column with the pane switcher and
       was the thing that had to be found. The switcher is on the header now
       and the results below are what this column is for, so the box gives
       back the height it was only using to be large. Its text stays at 17px
       — that floor is iOS's, not a preference (§16). */
    /* The box and its way out, on one line. The chip only exists while the
       search has the screen, so the row is the box alone the rest of the
       time and nothing moves when it appears. */
    .s-boxrow {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        flex-shrink: 0;
    }
    .s-boxrow .s-box {
        flex: 1;
        min-width: 0;
    }
    .s-done {
        flex-shrink: 0;
        min-height: 48px;
    }
    .s-box {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        height: 48px;
        padding: 0 var(--space-2) 0 var(--space-4);
        border-radius: var(--r-md);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text-dim);
        flex-shrink: 0;
    }
    .s-box:focus-within {
        border-color: var(--border);
        box-shadow: var(--focus-ring);
    }
    input {
        flex: 1;
        min-width: 0;
        border: 0;
        background: none;
        color: var(--text);
        font-family: inherit;
        font-size: 17px; /* ≥16px keeps iOS from auto-zooming on focus */
        outline: none;
        /* The kiosk root disables selection (user-select: none) so the wall
           panel can't be text-selected by a stray touch. Safari treats that
           as inherited focus suppression too: without opting this input back
           in, a tap never raises the software keyboard. */
        user-select: text;
        -webkit-user-select: text;
        /* The kiosk root also disables the touch callout (-webkit-touch-callout:
           none) to suppress the copy/paste bubble on a stray touch. That's a
           second, independent suppressor from user-select: with it inherited
           here, taps focus the input (caret shows) but Safari still withholds
           the software keyboard. Opt back in to actually raise it. */
        -webkit-touch-callout: default;
    }
    input::placeholder {
        color: var(--text-dim);
    }
    .s-clear {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: 0;
        background: none;
        color: var(--text-mute);
        cursor: pointer;
        border-radius: var(--r-sm);
    }

    .s-kinds {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        flex-shrink: 0;
    }

    /* Where a tap lands. Quiet — it is a caption, not a control — but
       always there, because the room chips live in the other column. */
    .s-dest {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: 0;
        font-size: 12.5px;
        color: var(--text-mute);
        flex-shrink: 0;
    }
    /* A note under a shelf that stands in for an empty state. */
    /* Type mode: the chips and the destination line stand down, because
       filtering is what the choosing phase is for. Scoped to this block
       now that it is a component — the depth's `.kb-open .s-dest` would no
       longer reach in here. */
    .s-head.kb-open .s-kinds,
    .s-head.kb-open .s-dest {
        display: none;
    }
</style>

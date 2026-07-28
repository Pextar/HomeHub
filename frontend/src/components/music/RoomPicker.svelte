<script lang="ts">
    /**
     * "Playing on — Kitchen": where the thing you are about to start will come
     * out, on every surface that can start something.
     *
     * This replaces a row of radio chips. That row listed every room in the
     * house, grouped by make, on top of every browsing screen — so the busiest
     * part of the screen was the part you almost never change, and in a house
     * with six rooms it wrapped to three lines before you saw a single search
     * result. One button, the room's name on it, and a menu behind it for the
     * times you do want to change it.
     *
     * A house with one room gets a plain line: a menu with one item in it is
     * a control that can't do anything.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { RoomsModel } from "../../lib/music/rooms.svelte";

    let {
        destination,
        rooms,
    }: {
        destination: Destination;
        rooms: RoomsModel;
    } = $props();

    let open = $state(false);

    $effect(() => {
        if (!open) return;
        const close = () => (open = false);
        // The opening click calls stopPropagation, so it never reaches here.
        document.addEventListener("click", close);
        return () => document.removeEventListener("click", close);
    });

    /** The menu answers the arrow keys, like every other menu in the app. */
    function menuNav(node: HTMLElement) {
        const items = () =>
            Array.from(node.querySelectorAll<HTMLButtonElement>("[role='menuitemradio']"));
        (items().find((b) => b.getAttribute("aria-checked") === "true") ?? items()[0])?.focus();
        function onKey(e: KeyboardEvent) {
            if (e.key === "Escape") {
                e.stopPropagation();
                open = false;
                return;
            }
            if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
            e.preventDefault();
            const list = items();
            const i = list.indexOf(document.activeElement as HTMLButtonElement);
            const next = e.key === "ArrowDown" ? i + 1 : i - 1;
            list[(next + list.length) % list.length]?.focus();
        }
        node.addEventListener("keydown", onKey);
        return { destroy: () => node.removeEventListener("keydown", onKey) };
    }
</script>

<div class="pick">
    <span class="pick-label">Playing on</span>
    {#if destination.list.length <= 1}
        <span class="pick-one">{destination.label || "no room"}</span>
    {:else}
        <button
            class="pick-btn"
            aria-haspopup="menu"
            aria-expanded={open}
            onclick={(e) => {
                e.stopPropagation();
                open = !open;
            }}
        >
            <span>{destination.label || "Pick a room"}</span>
            <Icon name="chevronDown" size={14} />
        </button>
        {#if open}
            <div class="pick-menu" role="menu" use:menuNav>
                {#each destination.list as r (r.key)}
                    <button
                        class="pick-item"
                        role="menuitemradio"
                        aria-checked={destination.is(r)}
                        onclick={() => {
                            destination.focus(r);
                            open = false;
                        }}
                    >
                        <span class="pick-mark">
                            {#if rooms.isPlaying(r)}
                                <Waveform />
                            {:else if destination.is(r)}
                                <Icon name="check" size={13} />
                            {/if}
                        </span>
                        <span class="pick-name">{r.name}</span>
                        {#if r.grouped}
                            <span class="pick-count mono">{r.members.length}</span>
                        {/if}
                    </button>
                {/each}
            </div>
        {/if}
    {/if}
</div>

<style>
    .pick { position: relative; display: flex; align-items: center; gap: var(--space-2); }
    .pick-label {
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.12em; text-transform: uppercase;
        color: var(--text-dim); flex-shrink: 0;
    }
    .pick-one {
        font-size: 13px; font-weight: 500; color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .pick-btn {
        display: flex; align-items: center; gap: 4px;
        min-width: 0; min-height: 32px; padding: 4px 8px 4px 10px;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text); font: inherit; font-size: 13px; font-weight: 500;
        cursor: pointer;
        transition: border-color var(--t-fast);
    }
    .pick-btn span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    @media (hover: hover) { .pick-btn:hover { border-color: var(--border-strong); } }
    @media (pointer: coarse) { .pick-btn { min-height: 44px; } }

    .pick-menu {
        position: absolute; top: calc(100% + 6px); left: 0; z-index: var(--z-menu);
        min-width: 190px; max-height: 320px; overflow-y: auto;
        display: flex; flex-direction: column; gap: 1px;
        padding: 5px;
        background: var(--card); border: 1px solid var(--border-strong);
        border-radius: var(--r-md); box-shadow: var(--shadow-md);
    }
    .pick-item {
        display: flex; align-items: center; gap: var(--space-2);
        min-height: 38px; padding: 6px var(--space-2);
        background: none; border: 0; border-radius: var(--r-sm);
        color: var(--text); font: inherit; font-size: 13px;
        text-align: left; cursor: pointer;
    }
    @media (hover: hover) { .pick-item:hover { background: var(--card-2); } }
    @media (pointer: coarse) { .pick-item { min-height: 44px; } }
    .pick-item[aria-checked="true"] { color: var(--on); }
    .pick-mark {
        width: 16px; flex-shrink: 0;
        display: flex; align-items: center; justify-content: center;
    }
    .pick-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .pick-count { font-size: 10.5px; color: var(--text-dim); flex-shrink: 0; }
</style>

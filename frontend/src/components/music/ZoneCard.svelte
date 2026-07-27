<script lang="ts">
    /**
     * One zone in the Zones sheet: what is in it, what it is playing, what a
     * play would do, and the two ways on — open its player, or edit its
     * membership.
     *
     * It is a card rather than a §11 row because it carries three things a row
     * can't: a transport, a progress hairline, and the route note that says
     * whether this zone is natively grouped or streamed by HomeHub. Playing, it
     * takes the sanctioned `.tile.on` surface like every other playing surface
     * in the module — no separate music gradient exists (DESIGN.md §15).
     */
    import type { Snippet } from "svelte";
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import ProgressLine from "./ProgressLine.svelte";
    import ZoneRoute from "./ZoneRoute.svelte";
    import type { MediaZone } from "../../lib/types";
    import type { ZonesBridge } from "../../lib/music/zones.svelte";

    let {
        zone: z,
        zones,
        onOpen,
        onEdit,
        /** Play/pause for the zone. Absent while there is nothing to resume.
         *  Takes the zone, so one snippet serves every card in the list. */
        transport = undefined,
    }: {
        zone: MediaZone;
        zones: ZonesBridge;
        onOpen: () => void;
        onEdit: () => void;
        transport?: Snippet<[MediaZone]>;
    } = $props();

    const playing = $derived(zones.isPlaying(z));
    const empty = $derived(zones.speakersOf(z).length === 0);
    const interrupts = $derived(zones.wouldInterrupt(z)?.name ?? "");
</script>

<div class="zone-card" class:playing>
    <div class="zone-top">
        <button class="zone-open" onclick={onOpen} disabled={empty}>
            <span class="zone-ico" class:on={playing}>
                {#if playing}
                    <Waveform />
                {:else}
                    <Icon name="groups" size={17} />
                {/if}
            </span>
            <span class="zone-meta">
                <span class="zone-name" title={z.name}>{z.name}</span>
                <span class="zone-sub">{zones.memberLine(z)}</span>
                {#if !empty}
                    <span class="zone-now">{zones.nowLine(z)}</span>
                {/if}
            </span>
        </button>
        {#if transport}{@render transport(z)}{/if}
        <button class="zone-edit" onclick={onEdit}>Edit</button>
    </div>

    <!-- The zone's own words for what it is about to do. Empty zones get the
         one thing that is true of them: they store, but they don't play. -->
    {#if empty}
        <p class="zone-note">
            <Icon name="info" size={13} />
            <span>No speakers in it yet — add some and it can play.</span>
        </p>
    {:else}
        <ZoneRoute
            route={z.route}
            sync={z.sync}
            reason={z.reason}
            problem={z.problem}
            {interrupts}
        />
    {/if}

    <ProgressLine value={zones.progress(z)} />
</div>

<style>
    .zone-card {
        position: relative; overflow: hidden;
        display: flex; flex-direction: column; gap: var(--space-2);
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        transition: border-color var(--t-fast);
    }
    .zone-card.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
    @media (hover: hover) { .zone-card:hover { border-color: var(--border-strong); } }

    .zone-top { display: flex; align-items: center; gap: var(--space-3); }
    .zone-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        background: none; border: 0; padding: 0;
        color: var(--text); text-align: left; cursor: pointer;
        transition: transform var(--t-fast);
    }
    .zone-open:active { transform: scale(0.99); }
    .zone-open:disabled { cursor: default; opacity: 0.7; }
    .zone-ico {
        width: 44px; height: 44px; border-radius: var(--r-md);
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--card-3); color: var(--text-mute);
    }
    .zone-ico.on { background: var(--on-soft); color: var(--on); }
    .zone-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .zone-name {
        font-size: 14px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .zone-sub, .zone-now {
        font-size: 12.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .zone-now { color: var(--text-dim); font-size: 12px; }

    /* Same weight as Zones' own "Ungroup" — a secondary way on, not a
       competing target. */
    .zone-edit {
        flex-shrink: 0;
        background: none; border: 0; padding: 2px 4px;
        color: var(--text-mute); font: inherit; font-size: 11px;
        cursor: pointer;
    }
    .zone-edit:hover { color: var(--text); }
    @media (pointer: coarse) {
        .zone-edit { min-width: 44px; min-height: 44px; }
    }

    .zone-note {
        display: flex; align-items: flex-start; gap: 6px;
        margin: 0; font-size: 12px; line-height: 1.4; color: var(--text-dim);
    }
    .zone-note :global(svg) { flex-shrink: 0; margin-top: 1px; }

    @media (prefers-reduced-motion: reduce) {
        .zone-card { transition-duration: 0.001ms; }
    }
</style>

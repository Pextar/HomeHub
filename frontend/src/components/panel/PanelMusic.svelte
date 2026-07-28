<script lang="ts">
    import PanelPlayerCard from "./PanelPlayerCard.svelte";
    import { route } from "../../lib/stores.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The panel dashboard's right column: what's playing, plus the transport
    // and volume that answer from across the room. Music's second satellite
    // outside its own view (after Home's "Playing now" card, DESIGN.md §6.8).
    // The art taps through to the panel's own music depth — search and the
    // library one level in, without leaving the kiosk (§16). The speaker
    // state itself lives in the shared store the parent panel owns.
    let { music }: { music: PanelMusicStore } = $props();
</script>

{#if music.hasSpeakers}
    <section class="music" aria-label="Now playing">
        <header class="m-head">
            <h2>Music</h2>
        </header>
        <PanelPlayerCard {music} onOpen={() => route.go("panel", { music: "1" })} />
    </section>
{/if}

<style>
    .music {
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
    }
    .m-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: var(--space-3);
        flex-shrink: 0;
    }
    h2 {
        margin: 0;
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
</style>

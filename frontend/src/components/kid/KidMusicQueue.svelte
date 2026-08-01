<script lang="ts">
    /**
     * The kid queue (DESIGN.md §17): the featured room's Up-next list in
     * big friendly rows. A tap plays that song right away; the ✕ takes it
     * out — two taps, the kid module's way of saying "really?" without a
     * modal (same pattern as the schedules on KidHome). Clearing the whole
     * queue also stops the music, so that one is two taps as well and says
     * what it does: "Stop & clear?"
     */
    import { onDestroy } from "svelte";
    import { haptic } from "../../lib/utils";
    import { trimClock } from "../../lib/music/time";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    let { music, onFindMusic }: { music: PanelMusicStore; onFindMusic: () => void } = $props();

    const featured = $derived(music.featured);

    // Two-tap confirm, one arm at a time: "rm:3" for a row, "all" for the
    // clear button. Disarms itself after 3s, like KidHome's schedule delete.
    let armed = $state<string | null>(null);
    let armTimer: ReturnType<typeof setTimeout> | undefined;
    onDestroy(() => clearTimeout(armTimer));

    function arm(key: string, action: () => void) {
        haptic();
        if (armed === key) {
            armed = null;
            clearTimeout(armTimer);
            action();
        } else {
            armed = key;
            clearTimeout(armTimer);
            armTimer = setTimeout(() => (armed = null), 3000);
        }
    }
</script>

{#if music.queueLoading && music.queue.length === 0}
    <div class="kmq-sklist" aria-hidden="true">
        {#each Array(4) as _, i (i)}
            <div class="kmq-skel"></div>
        {/each}
    </div>
{:else if music.queue.length === 0}
    <div class="kmq-empty">
        <div class="kmq-empty-emoji">🎶</div>
        <p>Nothing's coming up!</p>
        <button class="kmq-find" onclick={onFindMusic}>🔎 Find a song</button>
    </div>
{:else}
    <div class="kmq-head">
        <h3>🎶 Up next <span class="mono">{music.queue.length}</span></h3>
        <button
            class="kmq-clear"
            class:armed={armed === "all"}
            disabled={!!music.busy["qclear:" + (featured?.id ?? "")]}
            onclick={() => arm("all", () => music.clearQueue())}
        >
            {armed === "all" ? "Stop & clear?" : "Clear all"}
        </button>
    </div>

    <div class="kmq-list">
        {#each music.queue as q (q.track)}
            {@const current = q.track === featured?.queueTrack}
            <div class="kmq-row" class:current>
                <button
                    class="kmq-main"
                    disabled={!!music.busy["jump:" + q.track]}
                    onclick={() => music.jumpTo(q.track)}
                    aria-label={current ? `Playing: ${q.title ?? "song"}` : `Play ${q.title ?? "this song"} now`}
                >
                    <span class="kmq-num mono">{current ? "🎵" : q.track}</span>
                    {#if q.art_uri}
                        <img class="kmq-art" src={q.art_uri} alt="" loading="lazy" />
                    {:else}
                        <span class="kmq-art kmq-art-none" aria-hidden="true">🎵</span>
                    {/if}
                    <span class="kmq-names">
                        <span class="kmq-name">{q.title ?? "Unknown song"}</span>
                        {#if q.artist}
                            <span class="kmq-sub">{current ? "Playing now! · " : ""}{q.artist}</span>
                        {:else if current}
                            <span class="kmq-sub">Playing now!</span>
                        {/if}
                    </span>
                    {#if q.duration}
                        <span class="kmq-dur mono">{trimClock(q.duration)}</span>
                    {/if}
                </button>
                <button
                    class="kmq-x"
                    class:armed={armed === "rm:" + q.track}
                    disabled={!!music.busy["qrm:" + q.track]}
                    aria-label={armed === "rm:" + q.track ? `Really take ${q.title ?? "this song"} out?` : `Take ${q.title ?? "this song"} out of the queue`}
                    onclick={() => arm("rm:" + q.track, () => music.removeQueued(q.track))}
                >
                    {armed === "rm:" + q.track ? "?" : "✕"}
                </button>
            </div>
        {/each}
    </div>
{/if}

<style>
    .kmq-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        margin-bottom: var(--space-3);
    }
    .kmq-head h3 {
        font-size: 1.05rem;
        font-weight: 800;
        letter-spacing: -0.01em;
    }
    .kmq-clear {
        font-size: 0.9rem;
        font-weight: 800;
        padding: 10px 16px;
        min-height: 44px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
        white-space: nowrap;
        transition: transform 0.12s ease, background 0.18s ease, border-color 0.18s ease, color 0.18s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmq-clear:active { transform: scale(0.94); }
    .kmq-clear.armed {
        background: var(--kid-pink);
        border-color: var(--kid-pink);
        color: var(--kid-fg);
    }

    .kmq-list { display: flex; flex-direction: column; gap: var(--space-2); }
    .kmq-row {
        display: flex;
        align-items: stretch;
        gap: var(--space-2);
    }
    .kmq-main {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2) var(--space-3);
        min-height: 64px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        cursor: pointer;
        text-align: left;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmq-main:active { transform: scale(0.98); border-color: var(--kid-accent); }
    .kmq-main:disabled { opacity: 0.6; }
    .kmq-row.current .kmq-main {
        border-color: var(--kid-accent);
        background: var(--kid-accent-soft);
    }
    .kmq-num {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.95rem;
        font-weight: 700;
        color: var(--text-faint);
        width: 2.2ch;
        text-align: center;
        flex-shrink: 0;
    }
    .kmq-art {
        width: 46px;
        height: 46px;
        border-radius: var(--radius-md);
        object-fit: cover;
        flex-shrink: 0;
    }
    .kmq-art-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 1.3rem;
    }
    .kmq-names {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .kmq-name {
        font-size: 1rem;
        font-weight: 800;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kmq-sub {
        font-size: 0.82rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kmq-row.current .kmq-sub { color: var(--kid-accent); font-weight: 800; }
    .kmq-dur {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.85rem;
        font-weight: 700;
        color: var(--text-muted);
        flex-shrink: 0;
    }
    .kmq-x {
        width: 52px;
        min-height: 52px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        color: var(--text-muted);
        font-size: 1rem;
        font-weight: 800;
        cursor: pointer;
        flex-shrink: 0;
        align-self: center;
        transition: transform 0.12s ease, background 0.18s ease, border-color 0.18s ease, color 0.18s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmq-x:active { transform: scale(0.9); }
    .kmq-x.armed {
        background: var(--kid-pink);
        border-color: var(--kid-pink);
        color: var(--kid-fg);
        animation: kmq-shake 0.35s ease;
    }
    @keyframes kmq-shake {
        0%, 100% { transform: translateX(0); }
        25% { transform: translateX(-4px); }
        75% { transform: translateX(4px); }
    }

    .kmq-sklist { display: flex; flex-direction: column; gap: var(--space-3); }
    .kmq-skel {
        height: 64px;
        border-radius: var(--radius-lg);
        background: linear-gradient(90deg, var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: kmq-shimmer 1.5s linear infinite;
    }
    @keyframes kmq-shimmer {
        0% { background-position: 200% 0; }
        100% { background-position: -200% 0; }
    }
    .kmq-empty {
        text-align: center;
        color: var(--text-muted);
        padding: var(--space-6) var(--space-4);
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-3);
    }
    .kmq-empty-emoji { font-size: 3rem; }
    .kmq-empty p { font-size: 1.1rem; font-weight: 700; }
    .kmq-find {
        font-size: 1.05rem;
        font-weight: 800;
        padding: 14px 28px;
        min-height: 56px;
        border-radius: 999px;
        border: none;
        background: var(--kid-accent-grad);
        color: var(--kid-on-text);
        box-shadow: 0 0 0 4px var(--kid-ring);
        cursor: pointer;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmq-find:active { transform: scale(0.94); }

    @media (prefers-reduced-motion: reduce) {
        .kmq-x.armed { animation: none; }
        .kmq-skel { animation: none; }
    }
</style>

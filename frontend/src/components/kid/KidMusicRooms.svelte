<script lang="ts">
    /**
     * The kid rooms pane (DESIGN.md §17): play-together grouping, Sonos-
     * native and tap-based like the wall's. Every room is a big card; the
     * one the music plays from wears a ⭐, and every other card gets one
     * obvious button — "🤝 Join {star}" — that moves the whole room into
     * the star's group. The star's own card names its speakers and offers
     * the way back apart: a ✕ on one speaker, or "Split up" for all of
     * them. Both are two taps — un-grouping mid-song is the one surprise
     * here worth a "really?" — using the kid module's arm pattern.
     *
     * Tapping a room's name moves the ⭐: the featured room is where
     * search plays, so picking the star belongs on the card itself.
     * Cross-vendor HomeHub rooms are never created here, same as the wall
     * — and with Sonos the only make a kid can reach, there's nothing to
     * explain.
     */
    import { onDestroy } from "svelte";
    import { haptic } from "../../lib/utils";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";

    let { music }: { music: PanelMusicStore } = $props();

    const featured = $derived(music.featured);

    // Two-tap confirm for the apart-going gestures: "split" for the whole
    // group, "leave:{id}" for one speaker stepping out.
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

    function star(s: PanelSource) {
        haptic();
        music.selected = s.key;
    }

    const memberWord = (n: number) => (n === 1 ? "1 speaker" : `${n} speakers`);
</script>

<p class="kmr-hint">Rooms can play the same song together! 🤝<br />The ⭐ room is where your music plays.</p>

<div class="kmr-list">
    {#each music.sources as s (s.key)}
        {@const isStar = featured?.key === s.key}
        {@const members = s.members ?? []}
        <div class="kmr-card" class:star={isStar}>
            <div class="kmr-row">
                <button
                    class="kmr-main"
                    onclick={() => star(s)}
                    aria-label={isStar ? `${s.title} — the music plays here` : `Make ${s.title} the star room`}
                    aria-pressed={isStar}
                >
                    <span class="kmr-star-emoji" aria-hidden="true">{isStar ? "⭐" : "🔊"}</span>
                    <span class="kmr-names">
                        <span class="kmr-name">{s.title}</span>
                        <span class="kmr-sub">
                            {memberWord(members.length)}{#if s.playing} · playing 🎵{/if}
                        </span>
                    </span>
                </button>
                {#if !isStar && featured}
                    <button
                        class="kmr-join"
                        disabled={!!music.busy["join:" + s.id]}
                        onclick={() => music.joinSource(s)}
                    >
                        🤝 Join {featured.title}
                    </button>
                {/if}
            </div>

            {#if isStar && members.length > 1}
                <div class="kmr-members">
                    {#each members as m (m.id)}
                        <span class="kmr-mchip">
                            {m.name}{#if m.coordinator}
                                <span class="kmr-lead" aria-label="the leader">⭐</span>
                            {:else}
                                <button
                                    class="kmr-x"
                                    class:armed={armed === "leave:" + m.id}
                                    disabled={!!music.busy["leave:" + m.id]}
                                    aria-label={armed === "leave:" + m.id
                                        ? `Really take ${m.name} out?`
                                        : `Take ${m.name} out of the group`}
                                    onclick={() => arm("leave:" + m.id, () => music.leaveMember(m.id))}
                                >
                                    {armed === "leave:" + m.id ? "?" : "✕"}
                                </button>
                            {/if}
                        </span>
                    {/each}
                </div>
                <button
                    class="kmr-split"
                    class:armed={armed === "split"}
                    disabled={!!music.busy["ungroup:" + s.id]}
                    onclick={() => arm("split", () => music.ungroupFeatured())}
                >
                    {armed === "split" ? "Split up?" : "🧩 Split up"}
                </button>
            {/if}
        </div>
    {/each}
</div>

<style>
    .kmr-hint {
        font-size: 0.95rem;
        font-weight: 600;
        color: var(--text-muted);
        text-align: center;
        line-height: 1.5;
        margin-bottom: var(--space-4);
    }

    .kmr-list { display: flex; flex-direction: column; gap: var(--space-3); }
    .kmr-card {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding: var(--space-4);
        border-radius: var(--radius-xl);
        border: 3px solid var(--border);
        background: var(--bg-elevated);
        transition: border-color 0.2s ease, box-shadow 0.2s ease;
    }
    .kmr-card.star {
        border-color: var(--kid-accent);
        box-shadow: 0 0 0 4px var(--kid-ring);
    }

    .kmr-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .kmr-main {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2);
        min-height: 56px;
        border: none;
        border-radius: var(--radius-lg);
        background: transparent;
        cursor: pointer;
        text-align: left;
        -webkit-tap-highlight-color: transparent;
    }
    .kmr-main:active { transform: scale(0.98); }
    .kmr-star-emoji { font-size: 2rem; line-height: 1; flex-shrink: 0; }
    .kmr-names {
        display: flex;
        flex-direction: column;
        gap: 2px;
        min-width: 0;
    }
    .kmr-name {
        font-size: 1.15rem;
        font-weight: 800;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kmr-sub {
        font-size: 0.85rem;
        font-weight: 600;
        color: var(--text-muted);
    }

    .kmr-join {
        flex-shrink: 0;
        font-size: 1rem;
        font-weight: 800;
        padding: 12px 18px;
        min-height: 56px;
        border-radius: 999px;
        border: none;
        background: var(--kid-accent-grad);
        color: var(--kid-on-text);
        box-shadow: 0 0 0 3px var(--kid-ring);
        cursor: pointer;
        max-width: 55%;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmr-join:active { transform: scale(0.94); }
    .kmr-join:disabled { opacity: 0.6; }

    .kmr-members {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    .kmr-mchip {
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
        padding: 8px 8px 8px 14px;
        min-height: 48px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--surface);
        font-size: 0.95rem;
        font-weight: 800;
        color: var(--text);
    }
    .kmr-lead { font-size: 0.9rem; }
    .kmr-x {
        width: 44px;
        height: 44px;
        border-radius: 50%;
        border: none;
        background: var(--surface-hover);
        color: var(--text-muted);
        font-size: 0.85rem;
        font-weight: 800;
        cursor: pointer;
        transition: background 0.18s ease, color 0.18s ease, transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmr-x:active { transform: scale(0.88); }
    .kmr-x.armed {
        background: var(--kid-pink);
        color: var(--kid-fg);
        animation: kmr-shake 0.35s ease;
    }
    @keyframes kmr-shake {
        0%, 100% { transform: translateX(0); }
        25% { transform: translateX(-3px); }
        75% { transform: translateX(3px); }
    }

    .kmr-split {
        font-size: 1rem;
        font-weight: 800;
        padding: 14px 20px;
        min-height: 56px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--surface);
        color: var(--text-muted);
        cursor: pointer;
        transition: transform 0.12s ease, background 0.18s ease, border-color 0.18s ease, color 0.18s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kmr-split:active { transform: scale(0.97); }
    .kmr-split.armed {
        background: var(--kid-pink);
        border-color: var(--kid-pink);
        color: var(--kid-fg);
    }

    @media (prefers-reduced-motion: reduce) {
        .kmr-x.armed { animation: none; }
    }
</style>

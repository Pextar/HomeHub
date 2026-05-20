<script lang="ts">
    import { onMount } from "svelte";
    import { fade, scale } from "svelte/transition";
    import { backOut } from "svelte/easing";
    import { api } from "../lib/api";
    import { data, toasts, session } from "../lib/stores.svelte";
    import type { Socket } from "../lib/types";
    import KidLampPanel from "./KidLampPanel.svelte";

    // Matter/Tasmota bulbs get the colour + brightness playground; plain RF
    // sockets just flip on/off on tap.
    const isSmart = (lamp: Socket) => lamp.protocol === "matter" || lamp.protocol === "tasmota";
    let active = $state<Socket | null>(null);
    let confirmExit = $state(false);

    function onTap(lamp: Socket) {
        if (isSmart(lamp)) {
            bump(lamp.id);
            active = lamp;
        } else {
            toggle(lamp);
        }
    }

    const name = $derived(session.user?.username ?? "");
    // Kid profiles only ever receive their assigned lamps from the API, so
    // the whole socket list is theirs to show.
    const lamps = $derived(data.value.sockets);

    // ── Welcome splash + confetti ───────────────────────────────────────
    const GREETED_KEY = "kid-greeted";
    let showWelcome = $state(!sessionStorage.getItem(GREETED_KEY));
    const COLORS = ["#ff5d8f", "#ffd23f", "#3ddc97", "#4d9bff", "#b15dff", "#ff8c42"];
    const confetti = Array.from({ length: 70 }, (_, i) => ({
        id: i,
        left: Math.random() * 100,
        delay: Math.random() * 0.5,
        duration: 1.8 + Math.random() * 1.4,
        color: COLORS[i % COLORS.length],
        size: 8 + Math.random() * 8,
        rotate: Math.random() * 360,
    }));

    onMount(() => {
        if (!showWelcome) return;
        sessionStorage.setItem(GREETED_KEY, "1");
        const t = setTimeout(() => (showWelcome = false), 2600);
        return () => clearTimeout(t);
    });

    // ── Toggle with optimistic flip + a little bounce ───────────────────
    let popping = $state<Set<string>>(new Set());

    function bump(id: string) {
        popping.add(id);
        popping = new Set(popping);
        setTimeout(() => {
            popping.delete(id);
            popping = new Set(popping);
        }, 450);
    }

    async function toggle(lamp: Socket) {
        bump(lamp.id);
        const prev = lamp.state;
        lamp.state = !prev; // optimistic — store is reactive
        try {
            await api.socketToggle(lamp.id);
        } catch (e) {
            lamp.state = prev;
            toasts.error("Oops!", (e as Error).message);
        }
    }

    async function signOut() {
        try { await api.logout(); } catch { /* ignore */ }
        sessionStorage.removeItem(GREETED_KEY);
        window.location.reload();
    }

    function lampEmoji(lamp: Socket): string {
        return lamp.emoji && lamp.emoji.trim() ? lamp.emoji : "💡";
    }
</script>

{#if showWelcome}
    <div class="welcome" transition:fade={{ duration: 350 }} onclick={() => (showWelcome = false)} role="presentation">
        <div class="confetti" aria-hidden="true">
            {#each confetti as c (c.id)}
                <span
                    style="left:{c.left}%; background:{c.color}; width:{c.size}px; height:{c.size}px;
                           animation-delay:{c.delay}s; animation-duration:{c.duration}s;
                           transform:rotate({c.rotate}deg);"
                ></span>
            {/each}
        </div>
        <div class="greeting" in:scale={{ duration: 600, easing: backOut, start: 0.4 }}>
            <div class="wave">👋</div>
            <h1>Hi {name}!</h1>
        </div>
    </div>
{/if}

<div class="kid">
    <header class="kid-head">
        <h2>{name}'s lamps</h2>
        <button class="signout" onclick={() => confirmExit = true} aria-label="Sign out">👋 Bye</button>
    </header>

    {#if lamps.length === 0}
        <div class="none">
            <div class="none-emoji">🔌</div>
            <p>No lamps yet! Ask a grown-up to add some.</p>
        </div>
    {:else}
        <div class="grid">
            {#each lamps as lamp (lamp.id)}
                <button
                    class="tile"
                    class:on={lamp.state}
                    class:pop={popping.has(lamp.id)}
                    onclick={() => onTap(lamp)}
                    aria-pressed={lamp.state}
                >
                    {#if isSmart(lamp)}<span class="paint" aria-hidden="true">🎨</span>{/if}
                    <span class="emoji">{lampEmoji(lamp)}</span>
                    <span class="label">{lamp.name}</span>
                    <span class="status">{lamp.state ? "ON" : "OFF"}</span>
                </button>
            {/each}
        </div>
    {/if}
</div>

{#if active}
    <KidLampPanel socket={active} onClose={() => (active = null)} />
{/if}

{#if confirmExit}
    <div class="exit-backdrop" transition:fade={{ duration: 200 }} onclick={() => confirmExit = false} role="presentation">
        <div class="exit-card" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} in:scale={{ duration: 300, easing: backOut, start: 0.7 }} role="dialog" tabindex="-1">
            <div class="exit-emoji">👋</div>
            <p class="exit-q">Time to go?</p>
            <div class="exit-btns">
                <button class="exit-btn stay" onclick={() => confirmExit = false}>Stay!</button>
                <button class="exit-btn leave" onclick={signOut}>Bye bye</button>
            </div>
        </div>
    </div>
{/if}

<style>
    .kid {
        min-height: 100vh;
        padding: var(--space-5);
        padding-bottom: calc(var(--space-5) + env(safe-area-inset-bottom));
        background:
            radial-gradient(circle at 15% 0%, rgba(77, 155, 255, 0.18), transparent 45%),
            radial-gradient(circle at 90% 10%, rgba(255, 93, 143, 0.18), transparent 40%),
            var(--bg);
    }
    .kid-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        margin-bottom: var(--space-5);
    }
    .kid-head h2 {
        font-size: clamp(1.5rem, 5vw, 2.25rem);
        font-weight: 800;
        letter-spacing: -0.02em;
    }
    .signout {
        font-size: 1rem;
        font-weight: 700;
        padding: 12px 20px;
        min-height: 44px;
        border-radius: 999px;
        border: none;
        background: var(--surface-hover);
        color: var(--text);
        cursor: pointer;
        flex-shrink: 0;
    }
    .signout:active { transform: scale(0.95); }

    .grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(min(160px, 100%), 1fr));
        gap: var(--space-4);
    }
    .tile {
        aspect-ratio: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--space-2);
        border: 3px solid var(--border);
        border-radius: var(--radius-xl);
        background: var(--bg-elevated);
        cursor: pointer;
        color: var(--text-muted);
        transition: transform 0.18s ease, box-shadow 0.25s ease, background 0.25s ease, border-color 0.25s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .tile:active { transform: scale(0.95); }
    .tile { position: relative; }
    .paint {
        position: absolute;
        top: 10px;
        right: 12px;
        font-size: 1.4rem;
        opacity: 0.85;
    }
    .tile .emoji {
        font-size: clamp(3rem, 14vw, 5rem);
        line-height: 1;
        filter: grayscale(0.6) opacity(0.6);
        transition: filter 0.25s ease, transform 0.25s ease;
    }
    .tile .label {
        font-size: clamp(1rem, 3.5vw, 1.25rem);
        font-weight: 800;
        color: var(--text);
        text-align: center;
        line-height: 1.1;
    }
    .tile .status {
        font-size: 0.85rem;
        font-weight: 800;
        letter-spacing: 0.1em;
    }
    .tile.on {
        border-color: #ffd23f;
        background: linear-gradient(160deg, #fff3c4, #ffd23f);
        color: #7a5b00;
        box-shadow: 0 0 0 4px rgba(255, 210, 63, 0.25), 0 12px 40px rgba(255, 196, 0, 0.45);
        animation: glow 2.2s ease-in-out infinite;
    }
    .tile.on .emoji { filter: none; transform: scale(1.08); }
    .tile.on .label { color: #5e4500; }
    /* Springy bounce when tapped. */
    .tile.pop { animation: pop 0.45s cubic-bezier(0.34, 1.56, 0.64, 1); }
    .tile.on.pop { animation: pop 0.45s cubic-bezier(0.34, 1.56, 0.64, 1), glow 2.2s ease-in-out infinite; }

    @keyframes pop {
        0% { transform: scale(1); }
        40% { transform: scale(1.12); }
        70% { transform: scale(0.96); }
        100% { transform: scale(1); }
    }
    @keyframes glow {
        0%, 100% { box-shadow: 0 0 0 4px rgba(255, 210, 63, 0.25), 0 12px 40px rgba(255, 196, 0, 0.45); }
        50% { box-shadow: 0 0 0 7px rgba(255, 210, 63, 0.30), 0 16px 52px rgba(255, 196, 0, 0.6); }
    }
    @media (prefers-reduced-motion: reduce) {
        .tile, .tile.on { animation: none; }
        .tile.pop, .tile.on.pop { animation: none; }
    }

    .none {
        text-align: center;
        color: var(--text-muted);
        margin-top: 18vh;
    }
    .none-emoji { font-size: 4rem; margin-bottom: var(--space-3); }
    .none p { font-size: 1.25rem; font-weight: 700; }

    /* ── Welcome overlay ── */
    .welcome {
        position: fixed;
        inset: 0;
        z-index: 200;
        display: grid;
        place-items: center;
        overflow: hidden;
        background:
            radial-gradient(circle at 50% 40%, rgba(77, 155, 255, 0.25), transparent 60%),
            var(--bg);
    }
    .greeting { text-align: center; }
    .greeting .wave {
        font-size: 5rem;
        animation: wave 1s ease-in-out infinite;
        transform-origin: 70% 70%;
    }
    .greeting h1 {
        font-size: clamp(2.5rem, 10vw, 4.5rem);
        font-weight: 900;
        letter-spacing: -0.03em;
        background: linear-gradient(120deg, #4d9bff, #b15dff, #ff5d8f);
        -webkit-background-clip: text;
        background-clip: text;
        -webkit-text-fill-color: transparent;
    }
    @keyframes wave {
        0%, 100% { transform: rotate(-8deg); }
        50% { transform: rotate(18deg); }
    }
    .confetti { position: absolute; inset: 0; pointer-events: none; }
    .confetti span {
        position: absolute;
        top: -20px;
        border-radius: 2px;
        animation-name: fall;
        animation-timing-function: linear;
        animation-iteration-count: 1;
    }
    @keyframes fall {
        0% { transform: translateY(-20px) rotate(0deg); opacity: 1; }
        100% { transform: translateY(105vh) rotate(540deg); opacity: 1; }
    }
    @media (prefers-reduced-motion: reduce) {
        .confetti span { display: none; }
        .greeting .wave { animation: none; }
    }

    /* ── Exit confirmation ── */
    .exit-backdrop {
        position: fixed;
        inset: 0;
        z-index: 300;
        background: rgba(0, 0, 0, 0.55);
        display: grid;
        place-items: center;
        padding: var(--space-5);
    }
    .exit-card {
        background: var(--bg-elevated);
        border-radius: var(--radius-xl);
        padding: var(--space-7) var(--space-6);
        text-align: center;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-4);
        max-width: 320px;
        width: 100%;
        box-shadow: 0 24px 64px rgba(0,0,0,0.35);
    }
    .exit-emoji { font-size: 4rem; line-height: 1; }
    .exit-q { font-size: 1.75rem; font-weight: 900; letter-spacing: -0.02em; }
    .exit-btns { display: flex; gap: var(--space-3); width: 100%; }
    .exit-btn {
        flex: 1;
        padding: 16px;
        font-size: 1.1rem;
        font-weight: 800;
        border: none;
        border-radius: var(--radius-lg);
        cursor: pointer;
        transition: transform 0.15s ease;
    }
    .exit-btn:active { transform: scale(0.95); }
    .exit-btn.stay { background: var(--primary); color: white; }
    .exit-btn.leave { background: var(--surface-hover); color: var(--text-muted); }
</style>

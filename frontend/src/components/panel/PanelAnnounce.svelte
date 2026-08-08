<script lang="ts">
    import Icon from "../Icon.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    /**
     * Calling the house from the wall (DESIGN.md §16).
     *
     * This is the one thing a hallway panel can do that a phone in a pocket
     * cannot: make a sound in a room you are not standing in. "Dinner's
     * ready" is shouted up a staircase, and the panel is already on the wall
     * at the bottom of it.
     *
     * It takes the shelf's place at the foot of the music band rather than
     * opening anything. A swap, not a sheet and not a screen — the kiosk has
     * neither (§16), the player above it keeps playing and stays touchable,
     * and one tap on Cancel puts the shelf back. The band's height is already
     * allocated to "what could go on next"; for the ten seconds someone is
     * calling the kids, this is that.
     *
     * Two shapes, and the server picks which:
     *
     *   With a voice   The presets, then a box for anything else. Presets
     *                  first because typing is the worst thing a wall can
     *                  ask for (§16) and because the same four sentences are
     *                  what a house actually announces.
     *   Without one    One button. Every room hears the chime and no words,
     *                  and the strip says exactly that instead of taking a
     *                  sentence nobody will hear.
     */
    let { music, onClose }: { music: PanelMusicStore; onClose: () => void } = $props();

    const status = $derived(music.announce);
    const voice = $derived(!!status?.voice);
    const busy = $derived(!!music.busy["announce"]);

    /** The things this house shouts, from household settings.
     *
     *  Not editable *here* — a preset list configurable from the wall is a
     *  settings screen on a kiosk, and configuration lives in the full app
     *  (§16) — but not hardcoded either, which is what they were. Two things
     *  make them the household's rather than the app's: typing is the worst
     *  thing a wall asks anyone to do, so the presets are most of what this
     *  control is; and they are read out by a text-to-speech voice that
     *  speaks one language, which the household picks. Piper reads whatever
     *  text it is handed in the phonetics of the voice it was started with —
     *  it does not translate — so four sentences compiled into everybody's
     *  app is four sentences most households cannot use.
     *
     *  An empty list is a household that wants the box and nothing above it,
     *  and is honoured: the row is then the box and Say it, which is exactly
     *  what "no presets" should look like. */
    const presets = $derived(status?.presets ?? []);

    let text = $state("");
    let box = $state<HTMLInputElement | null>(null);

    /** Which rooms to call. Empty means every reachable room — the default,
     *  and the common case: most calls are meant for the whole house, so
     *  picking a subset is an opt-in narrowing, not a required step. */
    let picked = $state<Set<string>>(new Set());

    function toggleRoom(id: string) {
        if (busy) return;
        const next = new Set(picked);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        picked = next;
    }

    const allRoomNames = $derived(status?.rooms.map((r) => r.name).join(" · ") ?? "");
    const pickedNames = $derived(
        status?.rooms.filter((r) => picked.has(r.id)).map((r) => r.name),
    );

    /** The confirmation, for as long as it is worth showing. A wall has
     *  nobody to dismiss a card, so this expires on its own — and the strip
     *  closes with it, because the job is done. */
    const CONFIRM_MS = 4000;
    /** Only *this* opening's announcement counts. The store keeps the last
     *  one for as long as the panel runs, so without this the strip would
     *  open on a stale confirmation from an hour ago and close itself. */
    const openedAt = Date.now();
    const sent = $derived(
        music.lastAnnounce && music.lastAnnounce.at >= openedAt ? music.lastAnnounce : null,
    );
    let closeTimer: ReturnType<typeof setTimeout> | undefined;
    $effect(() => {
        if (!sent) return;
        clearTimeout(closeTimer);
        closeTimer = setTimeout(onClose, CONFIRM_MS);
        return () => clearTimeout(closeTimer);
    });

    function say(what: string) {
        if (busy) return;
        music.sendAnnouncement(what, picked.size ? [...picked] : undefined);
        text = "";
    }

    function onKey(e: KeyboardEvent) {
        if (e.key === "Enter" && text.trim()) {
            e.preventDefault();
            say(text.trim());
            box?.blur(); // the keyboard's job is done; give the band back
        }
    }
</script>

<section class="ann" aria-label="Announce">
    <header class="a-head">
        <h3 class="s-label">
            {#if sent}Announced{:else if voice}Announce{:else}Chime the house{/if}
        </h3>
        <!-- Where it lands, said before the tap. The wall must never be
             vague about which rooms are about to be interrupted. -->
        {#if sent}
            <p class="a-where">
                {sent.rooms.join(" · ")}{#if !sent.spoken}<span class="a-warn">
                        &nbsp;— chime only</span
                    >{/if}
            </p>
        {:else if status?.rooms.length}
            <p class="a-where">
                Heard in {pickedNames?.length ? pickedNames.join(" · ") : allRoomNames}
            </p>
        {/if}
        <button class="a-x" onclick={onClose} aria-label="Close announce">
            <Icon name="close" size={16} />
        </button>
    </header>

    {#snippet roomPicker()}
        {#if status && status.rooms.length > 1}
            <div class="a-row a-rooms" role="group" aria-label="Rooms to announce to">
                {#each status.rooms as r (r.id)}
                    <button
                        type="button"
                        class="a-room"
                        class:picked={picked.has(r.id)}
                        disabled={busy}
                        aria-pressed={picked.has(r.id)}
                        onclick={() => toggleRoom(r.id)}
                    >
                        {r.name}
                    </button>
                {/each}
            </div>
        {/if}
    {/snippet}

    {#if sent}
        <p class="a-said">“{sent.text || "Chime"}”</p>
    {:else if !status?.available}
        <p class="a-note">No speaker is answering, so there is nowhere to announce.</p>
    {:else if !voice}
        {@render roomPicker()}
        <div class="a-row">
            <button class="a-preset wide" disabled={busy} onclick={() => say("")}>
                <Icon name="megaphone" size={18} /><span
                    >{picked.size ? "Chime selected" : "Chime every room"}</span
                >
            </button>
        </div>
        <p class="a-note">
            No voice service is configured, so rooms hear a chime and no words. Set one up in the
            full app.
        </p>
    {:else}
        {@render roomPicker()}
        <div class="a-row">
            {#each presets as p (p)}
                <button class="a-preset" disabled={busy} onclick={() => say(p)}>{p}</button>
            {/each}
            <label class="a-box">
                <span class="sr-only">Something else to announce</span>
                <input
                    bind:this={box}
                    bind:value={text}
                    onkeydown={onKey}
                    type="text"
                    placeholder="Something else…"
                    maxlength={status?.max_text ?? 200}
                    disabled={busy}
                    enterkeyhint="send"
                />
            </label>
            <button class="a-send" disabled={busy || !text.trim()} onclick={() => say(text.trim())}>
                <Icon name="megaphone" size={18} /><span>Say it</span>
            </button>
        </div>
    {/if}
</section>

<style>
    /* The shelf's own footprint: this replaces it, so it inherits its
       height rather than pushing the player around when it opens. */
    .ann {
        flex: none;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .a-head {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .s-label {
        margin: 0;
        flex-shrink: 0;
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .a-where {
        margin: 0;
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        white-space: nowrap;
        text-overflow: ellipsis;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    .a-warn {
        color: var(--text-dim);
    }
    .a-x {
        flex-shrink: 0;
        display: grid;
        place-items: center;
        width: 44px;
        height: 44px;
        margin: -8px -8px -8px 0;
        border: 0;
        border-radius: var(--r-sm);
        background: none;
        color: var(--text-dim);
        cursor: pointer;
    }
    .a-x:active {
        color: var(--text);
    }

    .a-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-width: 0;
        overflow-x: auto;
        scrollbar-width: none;
        padding-bottom: 2px;
    }
    .a-row::-webkit-scrollbar {
        display: none;
    }

    /* Sized for a wall rather than for a phone: these are aimed at from a
       step away, mid-stride, usually one-handed. */
    .a-preset {
        flex: none;
        min-height: 52px;
        padding: 0 var(--space-5);
        border: 1px solid var(--border);
        border-radius: var(--r-pill);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 15px;
        font-weight: 500;
        cursor: pointer;
        transition: transform var(--t-fast);
    }

    /* Room filters, one step smaller than the preset row below them — a
       narrowing before the sentence, not the action itself. Unpicked means
       "every room" (the default), so nothing here starts selected. */
    .a-rooms {
        padding-bottom: 0;
    }
    .a-room {
        flex: none;
        min-height: 44px;
        padding: 0 var(--space-4);
        border: 1px solid var(--border);
        border-radius: var(--r-pill);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13.5px;
        font-weight: 500;
        cursor: pointer;
        transition: transform var(--t-fast), background var(--t-fast), color var(--t-fast),
            border-color var(--t-fast);
    }
    .a-room.picked {
        background: var(--on-soft);
        color: var(--on);
        border-color: transparent;
    }
    .a-room:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .a-room:disabled {
        opacity: 0.55;
    }
    .a-room:focus-visible {
        box-shadow: var(--focus-ring);
        outline: none;
    }
    .a-preset.wide {
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
    }
    .a-preset:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .a-preset:disabled,
    .a-send:disabled {
        opacity: 0.55;
    }
    .a-preset:focus-visible,
    .a-send:focus-visible,
    .a-box input:focus-visible {
        box-shadow: var(--focus-ring);
        outline: none;
    }

    .a-box {
        flex: 1 1 220px;
        min-width: 160px;
    }
    .a-box input {
        width: 100%;
        height: 52px;
        padding: 0 var(--space-4);
        border: 1px solid var(--border);
        border-radius: var(--r-pill);
        background: var(--card);
        color: var(--text);
        font-family: inherit;
        /* 17px, like the depth's search box: the floor is iOS's, not a
           preference — anything smaller zooms the whole kiosk on focus. */
        font-size: 17px;
    }
    .a-box input::placeholder {
        color: var(--text-dim);
    }

    .a-send {
        flex: none;
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
        min-height: 52px;
        padding: 0 var(--space-5);
        border: 1px solid var(--tile-on-border);
        border-radius: var(--r-pill);
        background: var(--tile-on-gradient);
        color: var(--on);
        font-family: inherit;
        font-size: 15px;
        font-weight: 600;
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .a-send:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }

    .a-said {
        margin: 0;
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.01em;
        overflow: hidden;
        white-space: nowrap;
        text-overflow: ellipsis;
    }
    .a-note {
        margin: 0;
        font-size: 12.5px;
        color: var(--text-dim);
    }

    @media (orientation: portrait), (max-width: 900px) {
        .a-preset,
        .a-send,
        .a-box input {
            min-height: 48px;
            height: auto;
            font-size: 15px;
        }
        .a-box input {
            height: 48px;
            font-size: 16px;
        }
    }
</style>

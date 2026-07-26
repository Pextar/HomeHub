<script lang="ts">
    /**
     * Search — Spotify's catalog, and the one-time account setup in front of
     * it. A sheet over Home, behind the plain search icon in its header.
     *
     * The box behaves like a search box: typing debounces, Enter runs the
     * query immediately, a clear X appears once there is something to clear
     * (Escape does the same from inside the field), and arriving here puts the
     * caret in the box — on `(pointer: fine)` only, since auto-focus on a
     * phone throws the software keyboard over the results.
     *
     * Tapping a result plays it now; "Play next" and "Add to queue" live
     * behind the row's overflow, and only for a Sonos destination — the queue
     * is a Sonos group's, so a control that would be refused is not rendered
     * at all.
     */
    import type { Snippet } from "svelte";
    import { tick as flushDOM } from "svelte";
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import MusicSheet from "./MusicSheet.svelte";
    import { dur } from "../../lib/motion";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { SearchHistory } from "../../lib/music/history.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        spotify,
        recents,
        destination,
        busy,
        /** True when the sheet was opened to type in, rather than to read. */
        autofocus = false,
        docked = false,
        onDismiss,
        onDisconnect,
        onPlayItem,
        onEnqueue,
        targetRow,
        scrollEl = $bindable<HTMLElement | null>(null),
    }: {
        spotify: SpotifyStore;
        recents: SearchHistory;
        destination: Destination;
        busy: Busy;
        autofocus?: boolean;
        docked?: boolean;
        onDismiss: () => void;
        onDisconnect: () => void;
        onPlayItem: (item: SpotifyItem) => void;
        onEnqueue: (item: SpotifyItem, next: boolean) => void;
        targetRow: Snippet;
        scrollEl?: HTMLElement | null;
    } = $props();

    let searchEl = $state<HTMLInputElement | null>(null);

    // Only where a keyboard is already there. On a phone an auto-focus throws
    // up the software keyboard over the results the user came to look at.
    $effect(() => {
        if (!autofocus || !spotify.connected) return;
        if (!window.matchMedia("(pointer: fine)").matches) return;
        void flushDOM().then(() => searchEl?.focus());
    });

    // Enter runs the search now instead of waiting out the debounce; Escape
    // clears the box rather than closing something behind it.
    function onQueryKey(e: KeyboardEvent) {
        if (e.key === "Enter") {
            e.preventDefault();
            spotify.runNow();
        } else if (e.key === "Escape" && spotify.query) {
            e.stopPropagation();
            spotify.clearQuery();
            searchEl?.focus();
        }
    }
    function runHistoryQuery(q: string) {
        spotify.runQuery(q);
        searchEl?.focus();
    }
    function clearQuery() {
        spotify.clearQuery();
        searchEl?.focus();
    }

    // ── Row overflow menus ───────────────────────────────────────────────
    // Keyed by item URI: at most one menu is open at a time.
    let menuFor = $state<string | null>(null);
    $effect(() => {
        if (!menuFor) return;
        const close = () => (menuFor = null);
        // The opening click calls stopPropagation, so it never reaches here.
        document.addEventListener("click", close);
        return () => document.removeEventListener("click", close);
    });
    function toggleMenu(e: MouseEvent, uri: string) {
        e.stopPropagation();
        menuFor = menuFor === uri ? null : uri;
    }
    /** An open menu takes focus and answers the arrow keys, so queueing a
     *  result never means tabbing back through the whole results list. */
    function menuNav(node: HTMLElement) {
        const items = () =>
            Array.from(node.querySelectorAll<HTMLButtonElement>("[role='menuitem']"));
        items()[0]?.focus();
        function onKey(e: KeyboardEvent) {
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

    /**
     * Escape closes an open row menu before it closes the sheet, so the shell
     * asks here first. Answers whether it consumed the key.
     */
    export function closeMenu(): boolean {
        if (!menuFor) return false;
        menuFor = null;
        return true;
    }
</script>

<MusicSheet
    label="Search"
    title="Search"
    sub={spotify.connected ? "Spotify" : ""}
    backLabel="Close Search"
    onBack={onDismiss}
    {onDismiss}
    {docked}
    bind:scrollEl
>
    <!-- ── Spotify search ──────────────────────────────────────────── -->
    {#if spotify.status}
        <section class="card">
            {#if !spotify.status?.configured || spotify.setupOpen}
                <div class="card-header"><h2>Spotify search</h2></div>
                <p class="sp-help">
                    Search Spotify's catalog and play straight to your speakers.
                    One-time setup — playback itself uses the Spotify account
                    already linked to your Sonos.
                </p>
                <ol class="sp-steps">
                    <li>
                        <a class="sp-link" href="https://developer.spotify.com/dashboard"
                            target="_blank" rel="noopener noreferrer">Open the Spotify dashboard</a>
                        and create an app (any name, "Web API" is enough).
                    </li>
                    <li>
                        Give the app this Redirect URI:
                        <span class="sp-redirect">
                            <code class="mono">{spotify.status?.redirect_uri}</code>
                            <button type="button" class="chip" onclick={() => spotify.copyRedirect()}>
                                <Icon name={spotify.copied ? "check" : "copy"} size={13} />
                                {spotify.copied ? "Copied" : "Copy"}
                            </button>
                        </span>
                    </li>
                    <li>Paste the app's Client ID here:</li>
                </ol>
                <form class="sp-config" onsubmit={(e) => { e.preventDefault(); void spotify.saveClientId(); }}>
                    <input type="text" class="mono" placeholder="Client ID"
                        aria-label="Spotify client ID" bind:value={spotify.clientId} />
                    <button type="submit" class="btn btn-primary" disabled={spotify.saving || !spotify.clientId.trim()}>
                        {spotify.saving ? "Saving…" : "Save"}
                    </button>
                    {#if spotify.setupOpen}
                        <button type="button" class="btn btn-ghost" onclick={() => (spotify.setupOpen = false)}>Cancel</button>
                    {/if}
                </form>
            {:else if !spotify.connected}
                <div class="card-header"><h2>Spotify search</h2></div>
                <p class="sp-help">
                    Client ID saved — now connect your Spotify account. You'll
                    approve access once on Spotify's page{spotify.status?.manual
                        ? "; it opens in a new tab and ends on an unreachable 127.0.0.1 address — that's expected."
                        : ", then land back here."}
                </p>
                <div class="sp-actions">
                    <button class="btn btn-primary" onclick={() => spotify.connect()}>Connect Spotify</button>
                    <button class="btn btn-ghost" onclick={() => { spotify.clientId = ""; spotify.setupOpen = true; }}>
                        Change client ID
                    </button>
                </div>
                {#if spotify.status?.manual}
                    <div class="field sp-paste">
                        <label for="sp-paste-input">
                            After approving, copy the full address from that tab and paste it here to finish:
                        </label>
                        <div class="sp-config">
                            <input id="sp-paste-input" type="text" class="mono"
                                placeholder="http://127.0.0.1:…/api/spotify/callback?code=…"
                                bind:value={spotify.pasteUrl} />
                            <button type="button" class="btn btn-primary"
                                disabled={spotify.finishing || !spotify.pasteUrl.trim()} onclick={() => spotify.finishConnect()}>
                                {spotify.finishing ? "Finishing…" : "Finish"}
                            </button>
                        </div>
                    </div>
                {/if}
            {:else}
                <!-- No <h2> here: the sheet's own head already says
                     "Search". This row only answers "as whom". -->
                <div class="card-header sp-head">
                    <div class="sp-account">
                        <span class="sp-conn" title="Connected to Spotify">
                            <span class="sp-dot" aria-hidden="true"></span>
                            <span class="sp-conn-label">Connected</span>
                            <span class="sp-user mono">{spotify.status?.display_name || "Spotify"}</span>
                        </span>
                        <button class="chip" onclick={onDisconnect}
                            aria-label="Disconnect Spotify">Disconnect</button>
                    </div>
                </div>
                <div class="sp-search">
                    <Icon name="search" size={16} />
                    <input
                        type="text"
                        class="sp-input"
                        placeholder="Songs, albums, playlists…"
                        aria-label="Search Spotify"
                        autocomplete="off"
                        enterkeyhint="search"
                        bind:this={searchEl}
                        bind:value={spotify.query}
                        oninput={() => spotify.onQueryInput()}
                        onkeydown={onQueryKey}
                    />
                    {#if spotify.query}
                        <button class="icon-btn sp-clear" aria-label="Clear search" onclick={spotify.clearQuery}>
                            <Icon name="close" size={14} />
                        </button>
                    {/if}
                </div>
                {#if !spotify.query && !spotify.results && recents.list.length > 0}
                    <div class="sp-history">
                        <div class="sp-history-head">
                            <span class="sp-browse-label">
                                Recent searches{#if destination.list.length > 1 && destination.label} · {destination.label}{/if}
                            </span>
                            <button type="button" class="chip sp-hist-clear" onclick={() => recents.clear()}>Clear</button>
                        </div>
                        <div class="sp-history-list">
                            {#each recents.list as h (h)}
                                <div class="sp-hist-chip">
                                    <button type="button" class="sp-hist-run" onclick={() => runHistoryQuery(h)}>
                                        <Icon name="search" size={12} />
                                        <span>{h}</span>
                                    </button>
                                    <button type="button" class="icon-btn sp-hist-x"
                                        aria-label={`Remove "${h}" from recent searches`}
                                        onclick={() => recents.remove(h)}>
                                        <Icon name="close" size={10} />
                                    </button>
                                </div>
                            {/each}
                        </div>
                    </div>
                {/if}
                <div class="sp-filters">
                    {#if spotify.results}
                        <button class="chip" class:active={spotify.kindFilter === "tracks"} onclick={() => (spotify.kindFilter = "tracks")}>Songs</button>
                        <button class="chip" class:active={spotify.kindFilter === "albums"} onclick={() => (spotify.kindFilter = "albums")}>Albums</button>
                        <button class="chip" class:active={spotify.kindFilter === "playlists"} onclick={() => (spotify.kindFilter = "playlists")}>Playlists</button>
                    {:else if spotify.myPlaylists.length > 0}
                        <span class="sp-browse-label">Your playlists</span>
                    {/if}
                    <div class="sp-targets" class:pushed={!!spotify.results}>{@render targetRow()}</div>
                </div>
                <!-- Playing on a KEF speaker goes out through Spotify Connect,
                     which needs a permission this login may predate. Saying so
                     before the tap beats a 409 after it, and reconnecting is
                     the only thing that fixes it. -->
                {#if destination.kefSpeaker && spotify.status && !spotify.status.playback}
                    <div class="sp-note">
                        <Icon name="info" size={14} />
                        <span>
                            Reconnect Spotify to start music on {destination.kefSpeaker.name} —
                            this login was made before HomeHub could ask for that.
                        </span>
                        <button class="chip" onclick={() => spotify.connect()}>Reconnect</button>
                    </div>
                {/if}
                {#if spotify.searching}
                    <div class="skeleton sp-skeleton"></div>
                {:else if spotify.results && spotify.shownItems.length === 0}
                    <div class="sp-none">No {spotify.kindFilter} matched "{spotify.query.trim()}".</div>
                {:else if !spotify.results && spotify.shownItems.length === 0}
                    <!-- No query and no playlists to browse — say what this
                         box does rather than leaving a blank panel. -->
                    <div class="sp-none">
                        Search Spotify for a song, album or playlist. Tapping a result
                        plays it on the room shown above{#if !destination.kefSpeaker}; the row's
                        overflow menu queues it without interrupting{/if}.
                    </div>
                {:else}
                    <div class="sp-results">
                        {#each spotify.shownItems as item (item.uri)}
                            <div class="sp-row">
                                <button class="sp-open" disabled={busy.is("item:" + item.uri) || !destination.current}
                                    onclick={() => onPlayItem(item)}>
                                    {#if item.art_url}
                                        <img class="sp-art" src={item.art_url} alt="" loading="lazy" />
                                    {:else}
                                        <div class="sp-art placeholder">[ art ]</div>
                                    {/if}
                                    <span class="sp-meta">
                                        <span class="sp-name">{item.name}</span>
                                        {#if item.sub}<span class="sp-sub">{item.sub}</span>{/if}
                                    </span>
                                    <span class="sp-play"><Icon name="play" size={16} /></span>
                                </button>
                                <!-- Tapping the row plays now; queueing without
                                     interrupting lives behind the overflow —
                                     and only for a Sonos destination, since
                                     the sonos.queue is a Sonos group's. A KEF
                                     speaker has none, so the control that
                                     would be refused isn't there at all. -->
                                {#if destination.sonosTarget}
                                    <button class="icon-btn sp-more" aria-label="More for {item.name}"
                                        aria-haspopup="menu" aria-expanded={menuFor === item.uri}
                                        disabled={busy.is("q:" + item.uri)}
                                        onclick={(e) => toggleMenu(e, item.uri)}>
                                        <Icon name="more" size={16} />
                                    </button>
                                {/if}
                                {#if menuFor === item.uri}
                                    <div class="overflow-menu" role="menu" use:menuNav
                                        in:scale={{ start: 0.95, duration: dur(140), easing: cubicOut, opacity: 0 }}
                                        out:scale={{ start: 0.95, duration: dur(100), easing: cubicOut, opacity: 0 }}>
                                        <button class="overflow-item" role="menuitem"
                                            onclick={() => onEnqueue(item, true)}>
                                            <Icon name="skipNext" size={16} /><span>Play next</span>
                                        </button>
                                        <button class="overflow-item" role="menuitem"
                                            onclick={() => onEnqueue(item, false)}>
                                            <Icon name="queue" size={16} /><span>Add to queue</span>
                                        </button>
                                    </div>
                                {/if}
                            </div>
                        {/each}
                    </div>
                {/if}
            {/if}
        </section>
    {/if}
</MusicSheet>

<style>
    /* ── Spotify search ── */
    .sp-help { font-size: 12.5px; color: var(--text-mute); line-height: 1.5; }
    .sp-steps {
        margin: 0; padding-left: 20px;
        display: flex; flex-direction: column; gap: var(--space-2);
        font-size: 12.5px; color: var(--text-mute); line-height: 1.5;
    }
    .sp-steps li::marker { font-family: var(--font-mono); color: var(--text-dim); }
    .sp-link { color: var(--on); text-decoration: underline; text-underline-offset: 2px; }
    .sp-redirect {
        display: flex; align-items: center; gap: var(--space-2);
        flex-wrap: wrap; margin-top: 4px;
    }
    .sp-redirect code {
        font-family: var(--font-mono); font-size: 12px; color: var(--text);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-sm); padding: 4px 8px;
        word-break: break-all; user-select: all;
    }
    .sp-paste label { font-size: 12.5px; color: var(--text-mute); }
    .sp-config { display: flex; gap: var(--space-2); align-items: center; }
    .sp-config input { flex: 1; min-width: 0; }
    .sp-actions { display: flex; gap: var(--space-2); }

    .sp-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    .sp-account { display: flex; align-items: center; gap: var(--space-3); }
    /* Positive "you're connected" signal, so the neighbouring Disconnect
       button reads as an action and not as the account's status. */
    .sp-conn { display: flex; align-items: center; gap: 6px; min-width: 0; }
    .sp-dot {
        width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0;
        background: var(--on); box-shadow: 0 0 0 4px var(--on-soft);
    }
    .sp-conn-label {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--on);
    }
    .sp-user {
        font-size: 11px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    .sp-search {
        display: flex; align-items: center; gap: var(--space-2);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-md); padding: 10px var(--space-3);
        color: var(--text-mute);
    }
    .sp-input {
        flex: 1; min-width: 0; background: none; border: 0; outline: none;
        color: var(--text); font-size: 14px;
    }
    .sp-clear { width: 30px; height: 30px; flex-shrink: 0; color: var(--text-mute); }
    /* The box already frames the field, so the ring goes on the container —
       a second rounded shape drawn inside it read as a box in a box. */
    .sp-search:focus-within { border-color: var(--border-strong); box-shadow: var(--focus-ring); }
    .sp-input:focus, .sp-input:focus-visible { box-shadow: none; }
    .sp-filters { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
    .sp-browse-label {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-dim);
    }
    /* Sits opposite the kind filters when there are results to filter, and
       leads the row when there aren't. */
    .sp-targets.pushed { margin-left: auto; }
    .sp-skeleton { height: 120px; border-radius: var(--r-md); }
    .sp-none { font-size: 12.5px; color: var(--text-mute); }
    /* One-line explanation above the results, for a destination that needs
       something before it can play. Quiet: it isn't a fault, it's a step. */
    .sp-note {
        display: flex; align-items: center; gap: var(--space-2);
        padding: var(--space-2) var(--space-3);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        font-size: 12.5px; color: var(--text-mute);
    }
    .sp-note :global(svg) { flex: none; color: var(--text-dim); }
    .sp-note span { flex: 1; min-width: 0; }
    .sp-note .chip { flex: none; }

    .sp-history { display: flex; flex-direction: column; gap: var(--space-2); }
    .sp-history-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); }
    .sp-hist-clear { padding: 3px 10px; font-size: 11px; }
    .sp-history-list { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .sp-hist-chip {
        display: inline-flex; align-items: center;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
    }
    .sp-hist-run {
        display: inline-flex; align-items: center; gap: 6px;
        padding: 7px 4px 7px 12px;
        background: transparent; border: 0; border-radius: var(--r-pill) 0 0 var(--r-pill);
        font: inherit; font-size: 12.5px; color: var(--text-mute); cursor: pointer;
    }
    @media (hover: hover) { .sp-hist-run:hover { color: var(--text); } }
    .sp-hist-chip .sp-hist-x { width: 26px; height: 26px; margin-right: 3px; color: var(--text-dim); }

    .sp-results { display: flex; flex-direction: column; gap: 2px; }
    /* The row is a container, not a control: tapping the body plays now,
       the trailing overflow queues without interrupting. */
    .sp-row {
        position: relative;
        display: flex; align-items: center; gap: var(--space-1);
        border-radius: var(--r-md);
        transition: background 150ms ease;
    }
    @media (hover: hover) { .sp-row:hover { background: var(--card-2); } }
    .sp-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 52px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text); cursor: pointer; text-align: left; font: inherit;
    }
    .sp-open:active:not(:disabled) { background: var(--card-3); }
    .sp-open:disabled { opacity: 0.5; cursor: default; }
    .sp-more { width: 36px; height: 36px; flex-shrink: 0; margin-right: 4px; }
    .sp-more:disabled { opacity: 0.4; }

    .overflow-menu {
        position: absolute; right: 8px; top: 46px; z-index: 12;
        min-width: 180px;
        display: flex; flex-direction: column;
        background: var(--card-2);
        border: 1px solid var(--border-strong);
        border-radius: var(--r-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .overflow-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: 0;
        border-bottom: 1px solid var(--hairline);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left;
    }
    .overflow-item:last-child { border-bottom: 0; }
    @media (hover: hover) { .overflow-item:hover { background: var(--card-3); } }
    .sp-art {
        width: 40px; height: 40px; border-radius: var(--r-sm);
        object-fit: cover; background: var(--card-2);
        border: 1px solid var(--hairline); flex-shrink: 0;
    }
    div.sp-art { display: grid; place-items: center; font-size: 8px; color: var(--text-dim); }
    .sp-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .sp-name {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-play {
        width: 36px; height: 36px; display: grid; place-items: center;
        border-radius: 50%; color: var(--text-mute); flex-shrink: 0;
        transition: color 150ms ease, background 150ms ease;
    }
    .sp-row:hover .sp-play { background: var(--on-soft); color: var(--on); }



    /* ── Touch: hit areas grow to the 44px floor ── */
    @media (pointer: coarse) {
        .sp-play, .sp-more, .sp-clear { width: 44px; height: 44px; }
        .sp-input, .sp-config input { font-size: 16px; } /* prevents iOS auto-zoom */
    }
    @media (prefers-reduced-motion: reduce) {
        .sp-row { transition-duration: 0.001ms; }
    }
</style>

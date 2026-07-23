<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Icon from "../components/Icon.svelte";
    import SonosSpeakerModal from "../modals/SonosSpeakerModal.svelte";
    import { api } from "../lib/api";
    import { toasts, route } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import { copyText } from "../lib/clipboard";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";
    import type {
        SonosStatus, SonosSpeakerView, SonosGroupView, SonosFavorite,
        SpotifyStatus, SpotifyItem, SpotifyResults,
    } from "../lib/types";

    let status = $state<SonosStatus | null>(null);
    let loaded = $state(false);
    let favorites = $state<SonosFavorite[]>([]);
    let favsLoaded = $state(false);
    // Which group's coordinator receives a tapped favorite. Defaults to the
    // first group; shown as chips when there is more than one group.
    let favTarget = $state<string | null>(null);

    // Volume the user just set, keyed by speaker id. The 5s poll must not
    // yank the slider back to a stale value while the command is still
    // propagating, so recent local sets win over polled state briefly.
    let volOverride: Record<string, { v: number; at: number }> = {};
    let localVol = $state<Record<string, number>>({});
    let groupVol = $state<Record<string, number>>({});

    // Actions in flight (play/pause/join/…) keyed by "<action>:<id>".
    let busy = $state<Record<string, boolean>>({});

    const speakerById = $derived(new Map((status?.speakers ?? []).map((s) => [s.id, s])));
    const groups = $derived(status?.groups ?? []);
    // Registered speakers the live topology doesn't mention — offline or on
    // another network. Shown separately so they stay visible and editable.
    const offline = $derived(
        (status?.speakers ?? []).filter((s) => !groups.some((g) => g.member_ids.includes(s.id))),
    );
    const playingCount = $derived(
        groups.filter((g) => coordinatorOf(g)?.state?.playing).length,
    );

    function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
        return speakerById.get(g.coordinator_id) ?? speakerById.get(g.member_ids[0]);
    }

    function groupTitle(g: SonosGroupView): string {
        const names = g.member_ids
            .map((id) => speakerById.get(id)?.name)
            .filter((n): n is string => !!n);
        if (names.length <= 2) return names.join(" + ");
        return `${names[0]} + ${names.length - 1} more`;
    }

    // ── Data loading ─────────────────────────────────────────────────────
    let pollTimer: ReturnType<typeof setInterval> | undefined;
    let statusSeq = 0;

    async function refresh() {
        const seq = ++statusSeq;
        try {
            const st = await api.sonosStatus();
            if (seq !== statusSeq) return;
            status = st;
            const now = Date.now();
            for (const sp of st.speakers) {
                const ov = volOverride[sp.id];
                if (ov && now - ov.at < 3000) continue; // user just moved it
                if (sp.state) localVol[sp.id] = sp.state.volume;
            }
            for (const g of st.groups) {
                // Group volume isn't reported by the status poll; seed the
                // slider with the members' average unless recently set.
                const key = "g:" + g.coordinator_id;
                const ov = volOverride[key];
                if (ov && now - ov.at < 3000) continue;
                const vols = g.member_ids
                    .map((id) => st.speakers.find((s) => s.id === id)?.state?.volume)
                    .filter((v): v is number => v !== undefined);
                if (vols.length) {
                    groupVol[g.coordinator_id] = Math.round(vols.reduce((a, b) => a + b, 0) / vols.length);
                }
            }
            if (!favTarget || !st.groups.some((g) => g.coordinator_id === favTarget)) {
                favTarget = st.groups[0]?.coordinator_id ?? null;
            }
            if (!favsLoaded && st.speakers.some((s) => s.reachable)) {
                void loadFavorites(st.speakers.find((s) => s.reachable)!.id);
            }
        } catch (e) {
            if (seq !== statusSeq) return;
            if (!loaded) toasts.error("Couldn't reach Sonos", (e as Error).message);
        } finally {
            if (seq === statusSeq) loaded = true;
        }
    }

    async function loadFavorites(speakerId: string) {
        favsLoaded = true;
        try {
            favorites = await api.sonosFavorites(speakerId);
        } catch {
            favsLoaded = false; // retry on a later poll
        }
    }

    onMount(() => {
        void refresh();
        pollTimer = setInterval(refresh, 5000);
    });
    onDestroy(() => clearInterval(pollTimer));

    // ── Actions ──────────────────────────────────────────────────────────
    async function run(key: string, fn: () => Promise<unknown>, errTitle: string) {
        if (busy[key]) return;
        busy[key] = true;
        try {
            await fn();
            await refresh();
        } catch (e) {
            toasts.error(errTitle, (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    function togglePlay(g: SonosGroupView) {
        const c = coordinatorOf(g);
        if (!c) return;
        const playing = c.state?.playing;
        void run(
            "play:" + c.id,
            () => (playing ? api.sonosPause(c.id) : api.sonosPlay(c.id)),
            playing ? "Pause failed" : "Play failed",
        );
    }

    function skip(g: SonosGroupView, dir: "next" | "previous") {
        const c = coordinatorOf(g);
        if (!c) return;
        void run(dir + ":" + c.id, () => (dir === "next" ? api.sonosNext(c.id) : api.sonosPrevious(c.id)), "Skip failed");
    }

    // Sliders update the local value live (oninput) and send on release
    // (onchange), so dragging doesn't flood the speaker with SOAP calls.
    function setVolume(id: string, v: number) {
        localVol[id] = v;
        volOverride[id] = { v, at: Date.now() };
        api.sonosSetVolume(id, v).catch((e) => toasts.error("Volume failed", (e as Error).message));
    }

    function setGroupVolume(coordinatorId: string, v: number) {
        groupVol[coordinatorId] = v;
        volOverride["g:" + coordinatorId] = { v, at: Date.now() };
        api.sonosSetVolume(coordinatorId, v, true).catch((e) => toasts.error("Volume failed", (e as Error).message));
    }

    function toggleMute(sp: SonosSpeakerView) {
        void run("mute:" + sp.id, () => api.sonosSetMute(sp.id, !sp.state?.muted), "Mute failed");
    }

    function join(speakerId: string, g: SonosGroupView) {
        void run("join:" + speakerId, () => api.sonosJoin(speakerId, g.coordinator_id), "Grouping failed");
    }

    function leave(speakerId: string) {
        void run("leave:" + speakerId, () => api.sonosLeave(speakerId), "Ungrouping failed");
    }

    function playFavorite(f: SonosFavorite) {
        if (!favTarget) return;
        void run("fav:" + f.id, () => api.sonosPlayFavorite(favTarget!, f), "Couldn't play favorite");
    }

    // ── Spotify search ───────────────────────────────────────────────────
    let spotify = $state<SpotifyStatus | null>(null);
    let spotifySetup = $state(false); // client-ID form expanded
    let clientId = $state("");
    let spotifySaving = $state(false);
    let query = $state("");
    let searching = $state(false);
    let results = $state<SpotifyResults | null>(null);
    let kindFilter = $state<"tracks" | "albums" | "playlists">("tracks");
    let myPlaylists = $state<SpotifyItem[]>([]);
    let myPlaylistsLoaded = false;

    async function loadSpotify() {
        try {
            spotify = await api.spotifyStatus();
            if (spotify.connected && !myPlaylistsLoaded) {
                myPlaylistsLoaded = true;
                myPlaylists = await api.spotifyMyPlaylists().catch(() => []);
            }
        } catch {
            spotify = null; // integration unavailable — hide the card
        }
    }

    // The OAuth callback bounces back to /#/music?spotify=… — surface the
    // outcome once, then clean the query off the URL.
    onMount(() => {
        const q = route.query;
        if (q.spotify === "connected") {
            toasts.success("Spotify connected");
            route.go("music");
        } else if (q.spotify_error) {
            toasts.error("Spotify login failed", q.spotify_error);
            route.go("music");
        }
        void loadSpotify();
    });

    async function saveClientId() {
        if (spotifySaving || !clientId.trim()) return;
        spotifySaving = true;
        try {
            await api.spotifySetConfig(clientId.trim());
            spotifySetup = false;
            await loadSpotify();
            toasts.success("Client ID saved", "Now connect your Spotify account.");
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            spotifySaving = false;
        }
    }

    let pasteUrl = $state("");
    let finishing = $state(false);
    let copied = $state(false);

    async function copyRedirect() {
        if (!spotify) return;
        if (await copyText(spotify.redirect_uri)) {
            copied = true;
            setTimeout(() => (copied = false), 1800);
        }
    }

    async function connectSpotify() {
        // Manual flow: keep this page open — the consent tab is opened
        // synchronously (before the await) so popup blockers allow it,
        // then pointed at the authorize URL once it arrives.
        const tab = spotify?.manual ? window.open("about:blank", "_blank") : null;
        try {
            const { url } = await api.spotifyLoginURL();
            if (spotify?.manual) {
                if (tab) tab.location.href = url;
                else window.location.href = url; // popup blocked — same tab still works
            } else {
                window.location.href = url; // bounces back here automatically
            }
        } catch (e) {
            tab?.close();
            toasts.error("Couldn't start Spotify login", (e as Error).message);
        }
    }

    async function finishConnect() {
        if (finishing || !pasteUrl.trim()) return;
        finishing = true;
        try {
            await api.spotifyExchange(pasteUrl);
            pasteUrl = "";
            toasts.success("Spotify connected");
            await loadSpotify();
        } catch (e) {
            toasts.error("Couldn't finish the login", (e as Error).message);
        } finally {
            finishing = false;
        }
    }

    async function disconnectSpotify() {
        try {
            await api.spotifyDisconnect();
            results = null;
            query = "";
            myPlaylists = [];
            myPlaylistsLoaded = false;
            await loadSpotify();
        } catch (e) {
            toasts.error("Disconnect failed", (e as Error).message);
        }
    }

    let searchTimer: ReturnType<typeof setTimeout> | undefined;
    let searchSeq = 0;
    function onQueryInput() {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(doSearch, 400);
    }
    async function doSearch() {
        const q = query.trim();
        const seq = ++searchSeq;
        if (!q) { results = null; searching = false; return; }
        searching = true;
        try {
            const r = await api.spotifySearch(q, 8);
            if (seq !== searchSeq) return;
            results = r;
        } catch (e) {
            if (seq !== searchSeq) return;
            toasts.error("Search failed", (e as Error).message);
        } finally {
            if (seq === searchSeq) searching = false;
        }
    }

    const shownItems = $derived<SpotifyItem[]>(
        results ? results[kindFilter] : myPlaylists,
    );

    function playItem(item: SpotifyItem) {
        if (!favTarget) return;
        void run(
            "item:" + item.uri,
            () => api.sonosPlayItem(favTarget!, { service: "Spotify", uri: item.uri, title: item.name }),
            "Couldn't play",
        );
    }

    async function openSpeakerModal(sp?: SonosSpeakerView) {
        const changed = await openModal<boolean>(SonosSpeakerModal, sp ? { existing: sp } : {});
        if (changed) void refresh();
    }

    // Speakers outside this group that could join it.
    function joinables(g: SonosGroupView): SonosSpeakerView[] {
        return (status?.speakers ?? []).filter(
            (s) => !g.member_ids.includes(s.id) && s.reachable,
        );
    }

    // "0:03:12" → "3:12" (Sonos always sends leading hours)
    function clock(t?: string): string {
        if (!t) return "";
        return t.replace(/^0:0?/, "");
    }
</script>

<Topbar
    title="Music"
    subtitle={status
        ? `${status.speakers.length} speaker${status.speakers.length === 1 ? "" : "s"} · ${playingCount} playing`
        : "Sonos"}
>
    {#snippet actions()}
        <button class="chip" onclick={() => openSpeakerModal()}>
            <Icon name="plus" size={14} /> Add speaker
        </button>
    {/snippet}
</Topbar>

{#if !loaded}
    <section class="card"><div class="skeleton sk"></div></section>
{:else if (status?.speakers.length ?? 0) === 0}
    <EmptyState
        icon="speaker"
        title="No speakers yet"
        message="Add your Sonos speakers to control playback, volume and grouping right here — no Sonos app needed."
    >
        <button class="btn btn-primary" onclick={() => openSpeakerModal()}>Add speaker</button>
    </EmptyState>
{:else}
    <div class="groups">
        {#each groups as g (g.coordinator_id)}
            {@const c = coordinatorOf(g)}
            {@const st = c?.state}
            {@const grouped = g.member_ids.length > 1}
            <section class="card group" in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}>
                <header class="group-head">
                    <span class="dot" class:on={st?.playing}></span>
                    <h2>{groupTitle(g)}</h2>
                    {#if st?.playing}<span class="live mono">PLAYING</span>{/if}
                </header>

                <div class="now">
                    {#if st?.track?.art_uri}
                        <img class="art" src={st.track.art_uri} alt="" loading="lazy" />
                    {:else}
                        <div class="art placeholder">[ art ]</div>
                    {/if}
                    <div class="now-meta">
                        {#if st?.track?.title}
                            <div class="track" title={st.track.title}>{st.track.title}</div>
                            <div class="artist">
                                {[st.track.artist, st.track.album].filter(Boolean).join(" · ")}
                            </div>
                            {#if st.position}
                                <div class="pos mono">
                                    {clock(st.position)}{st.duration ? ` / ${clock(st.duration)}` : ""}
                                </div>
                            {/if}
                        {:else}
                            <div class="track idle">Nothing playing</div>
                            <div class="artist">Pick a favorite below, or start from any Sonos-aware app.</div>
                        {/if}
                    </div>
                    <div class="transport">
                        <button class="icon-btn t-btn" aria-label="Previous track"
                            disabled={!c || busy["previous:" + c.id]}
                            onclick={() => skip(g, "previous")}>
                            <Icon name="skipPrev" size={18} />
                        </button>
                        <button class="icon-btn t-btn t-main" class:playing={st?.playing}
                            aria-label={st?.playing ? "Pause" : "Play"}
                            disabled={!c || busy["play:" + c?.id]}
                            onclick={() => togglePlay(g)}>
                            <Icon name={st?.playing ? "pause" : "play"} size={20} />
                        </button>
                        <button class="icon-btn t-btn" aria-label="Next track"
                            disabled={!c || busy["next:" + c.id]}
                            onclick={() => skip(g, "next")}>
                            <Icon name="skipNext" size={18} />
                        </button>
                    </div>
                </div>

                {#if grouped}
                    <div class="vol-row group-vol">
                        <Icon name="volume" size={16} />
                        <span class="vol-label">Group</span>
                        <input type="range" min="0" max="100" step="1"
                            aria-label="Group volume"
                            value={groupVol[g.coordinator_id] ?? 0}
                            oninput={(e) => (groupVol[g.coordinator_id] = e.currentTarget.valueAsNumber)}
                            onchange={(e) => setGroupVolume(g.coordinator_id, e.currentTarget.valueAsNumber)} />
                        <span class="vol-num mono">{groupVol[g.coordinator_id] ?? 0}</span>
                    </div>
                {/if}

                <div class="members">
                    {#each g.member_ids as id (id)}
                        {@const sp = speakerById.get(id)}
                        {#if sp}
                            <div class="member">
                                <button class="icon-btn m-mute" aria-label={sp.state?.muted ? `Unmute ${sp.name}` : `Mute ${sp.name}`}
                                    disabled={busy["mute:" + sp.id]}
                                    onclick={() => toggleMute(sp)}>
                                    <Icon name={sp.state?.muted ? "volumeOff" : "volume"} size={16} />
                                </button>
                                <span class="m-name" class:muted={sp.state?.muted}>{sp.name}</span>
                                <input type="range" min="0" max="100" step="1"
                                    aria-label="{sp.name} volume"
                                    value={localVol[sp.id] ?? sp.state?.volume ?? 0}
                                    oninput={(e) => (localVol[sp.id] = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => setVolume(sp.id, e.currentTarget.valueAsNumber)} />
                                <span class="vol-num mono">{localVol[sp.id] ?? sp.state?.volume ?? 0}</span>
                                {#if grouped}
                                    <button class="icon-btn m-act" aria-label="Remove {sp.name} from group"
                                        disabled={busy["leave:" + sp.id]}
                                        onclick={() => leave(sp.id)}>
                                        <Icon name="close" size={14} />
                                    </button>
                                {:else}
                                    <button class="icon-btn m-act" aria-label="Edit {sp.name}"
                                        onclick={() => openSpeakerModal(sp)}>
                                        <Icon name="edit" size={14} />
                                    </button>
                                {/if}
                            </div>
                        {/if}
                    {/each}
                </div>

                {#if joinables(g).length > 0}
                    <div class="joiners">
                        {#each joinables(g) as sp (sp.id)}
                            <button class="chip" disabled={busy["join:" + sp.id]} onclick={() => join(sp.id, g)}>
                                <Icon name="plus" size={13} /> {sp.name}
                            </button>
                        {/each}
                    </div>
                {/if}

                {#if g.unregistered?.length}
                    <div class="unreg mono">
                        also in this group: {g.unregistered.join(", ")} — add them to control here
                    </div>
                {/if}
            </section>
        {/each}
    </div>

    {#if spotify}
        <section class="card">
            {#if !spotify.configured || spotifySetup}
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
                            <code class="mono">{spotify.redirect_uri}</code>
                            <button type="button" class="chip" onclick={copyRedirect}>
                                <Icon name={copied ? "check" : "copy"} size={13} />
                                {copied ? "Copied" : "Copy"}
                            </button>
                        </span>
                    </li>
                    <li>Paste the app's Client ID here:</li>
                </ol>
                <form class="sp-config" onsubmit={(e) => { e.preventDefault(); saveClientId(); }}>
                    <input type="text" class="mono" placeholder="Client ID"
                        aria-label="Spotify client ID" bind:value={clientId} />
                    <button type="submit" class="btn btn-primary" disabled={spotifySaving || !clientId.trim()}>
                        {spotifySaving ? "Saving…" : "Save"}
                    </button>
                    {#if spotifySetup}
                        <button type="button" class="btn btn-ghost" onclick={() => (spotifySetup = false)}>Cancel</button>
                    {/if}
                </form>
            {:else if !spotify.connected}
                <div class="card-header"><h2>Spotify search</h2></div>
                <p class="sp-help">
                    Client ID saved — now connect your Spotify account. You'll
                    approve access once on Spotify's page{spotify.manual
                        ? "; it opens in a new tab and ends on an unreachable 127.0.0.1 address — that's expected."
                        : ", then land back here."}
                </p>
                <div class="sp-actions">
                    <button class="btn btn-primary" onclick={connectSpotify}>Connect Spotify</button>
                    <button class="btn btn-ghost" onclick={() => { clientId = ""; spotifySetup = true; }}>
                        Change client ID
                    </button>
                </div>
                {#if spotify.manual}
                    <div class="field sp-paste">
                        <label for="sp-paste-input">
                            After approving, copy the full address from that tab and paste it here to finish:
                        </label>
                        <div class="sp-config">
                            <input id="sp-paste-input" type="text" class="mono"
                                placeholder="http://127.0.0.1:…/api/spotify/callback?code=…"
                                bind:value={pasteUrl} />
                            <button type="button" class="btn btn-primary"
                                disabled={finishing || !pasteUrl.trim()} onclick={finishConnect}>
                                {finishing ? "Finishing…" : "Finish"}
                            </button>
                        </div>
                    </div>
                {/if}
            {:else}
                <div class="card-header sp-head">
                    <h2>Search</h2>
                    <div class="sp-account">
                        <span class="sp-user mono">{spotify.display_name || "Spotify"}</span>
                        <button class="chip" onclick={disconnectSpotify}>Disconnect</button>
                    </div>
                </div>
                <input
                    type="search"
                    class="sp-input"
                    placeholder="Search songs, albums, playlists…"
                    aria-label="Search Spotify"
                    bind:value={query}
                    oninput={onQueryInput}
                />
                <div class="sp-filters">
                    {#if results}
                        <button class="chip" class:active={kindFilter === "tracks"} onclick={() => (kindFilter = "tracks")}>Songs</button>
                        <button class="chip" class:active={kindFilter === "albums"} onclick={() => (kindFilter = "albums")}>Albums</button>
                        <button class="chip" class:active={kindFilter === "playlists"} onclick={() => (kindFilter = "playlists")}>Playlists</button>
                    {:else if myPlaylists.length > 0}
                        <span class="sp-browse-label">Your playlists</span>
                    {/if}
                    {#if groups.length > 1}
                        <div class="fav-targets sp-targets" role="radiogroup" aria-label="Play on">
                            {#each groups as g (g.coordinator_id)}
                                <button class="chip" class:on={favTarget === g.coordinator_id}
                                    onclick={() => (favTarget = g.coordinator_id)}>
                                    {groupTitle(g)}
                                </button>
                            {/each}
                        </div>
                    {/if}
                </div>
                {#if searching}
                    <div class="skeleton sp-skeleton"></div>
                {:else if results && shownItems.length === 0}
                    <div class="sp-none">No {kindFilter} matched "{query.trim()}".</div>
                {:else}
                    <div class="sp-results">
                        {#each shownItems as item (item.uri)}
                            <button class="sp-row" disabled={busy["item:" + item.uri] || !favTarget}
                                onclick={() => playItem(item)}>
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
                        {/each}
                    </div>
                {/if}
            {/if}
        </section>
    {/if}

    {#if offline.length > 0}
        <section class="card">
            <div class="card-header"><h2>Unreachable</h2></div>
            <div class="members">
                {#each offline as sp (sp.id)}
                    <div class="member off">
                        <Icon name="speaker" size={16} />
                        <span class="m-name">{sp.name}</span>
                        <span class="off-ip mono">{sp.ip}</span>
                        <button class="icon-btn m-act" aria-label="Edit {sp.name}"
                            onclick={() => openSpeakerModal(sp)}>
                            <Icon name="edit" size={14} />
                        </button>
                    </div>
                {/each}
            </div>
        </section>
    {/if}

    {#if favorites.length > 0}
        <section class="card">
            <div class="card-header favs-head">
                <h2>Favorites</h2>
                {#if groups.length > 1}
                    <div class="fav-targets" role="radiogroup" aria-label="Play favorites on">
                        {#each groups as g (g.coordinator_id)}
                            <button class="chip" class:on={favTarget === g.coordinator_id}
                                onclick={() => (favTarget = g.coordinator_id)}>
                                {groupTitle(g)}
                            </button>
                        {/each}
                    </div>
                {/if}
            </div>
            <div class="favs h-scroll">
                {#each favorites as f (f.id)}
                    <button class="fav" disabled={busy["fav:" + f.id] || !favTarget}
                        onclick={() => playFavorite(f)}>
                        {#if f.art_uri}
                            <img class="fav-art" src={f.art_uri} alt="" loading="lazy" />
                        {:else}
                            <div class="fav-art placeholder">[ art ]</div>
                        {/if}
                        <span class="fav-title">{f.title}</span>
                        {#if f.service}<span class="fav-sub mono">{f.service}</span>{/if}
                    </button>
                {/each}
            </div>
        </section>
    {/if}
{/if}

<style>
    .sk { height: 180px; border-radius: var(--r-md); }

    .groups { display: flex; flex-direction: column; gap: var(--space-4); }
    .group { display: flex; flex-direction: column; gap: var(--space-4); }

    .group-head { display: flex; align-items: center; gap: var(--space-2); }
    .group-head h2 {
        font-size: 17px; font-weight: 600; letter-spacing: -0.02em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .live {
        margin-left: auto;
        font-size: 10px; letter-spacing: 0.08em;
        color: var(--on);
    }

    /* ── Now playing ── */
    .now { display: flex; align-items: center; gap: var(--space-4); }
    .art {
        width: 72px; height: 72px;
        border-radius: var(--r-md);
        object-fit: cover;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    div.art { font-size: 10px; }
    .now-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
    .track {
        font-size: 15px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .track.idle { color: var(--text-mute); font-weight: 500; }
    .artist {
        font-size: 12.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .pos { font-size: 11px; color: var(--text-dim); font-feature-settings: "tnum" 1; }

    .transport { display: flex; align-items: center; gap: var(--space-1); flex-shrink: 0; }
    .t-btn { width: 40px; height: 40px; }
    .t-main {
        width: 48px; height: 48px;
        border-radius: 50%;
        background: var(--card-3);
        color: var(--text);
    }
    .t-main.playing {
        background: var(--on);
        color: var(--bg);
        box-shadow: 0 0 14px 2px var(--on-glow);
    }

    /* ── Volume rows ── */
    .vol-row, .member {
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 44px;
    }
    .vol-row { color: var(--text-mute); }
    .group-vol {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: 4px var(--space-3);
    }
    .vol-label { font-size: 12.5px; font-weight: 500; }
    .vol-num {
        font-size: 12px;
        font-feature-settings: "tnum" 1;
        color: var(--text-mute);
        width: 3ch; text-align: right; flex-shrink: 0;
    }

    .members { display: flex; flex-direction: column; gap: 2px; }
    .member .m-name {
        font-size: 13.5px; font-weight: 500;
        width: 110px; flex-shrink: 0;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .member .m-name.muted { color: var(--text-dim); }
    .m-mute, .m-act { width: 36px; height: 36px; flex-shrink: 0; }
    .member.off { color: var(--text-mute); }
    .member.off .m-name { width: auto; }
    .off-ip { margin-left: auto; font-size: 11px; color: var(--text-dim); }

    input[type="range"] {
        flex: 1;
        min-width: 60px;
        appearance: none;
        height: 6px;
        border-radius: 3px;
        outline: none;
        background: var(--card-3);
        accent-color: var(--on);
    }
    input[type="range"]::-webkit-slider-thumb {
        appearance: none;
        width: 18px; height: 18px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer;
        box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]::-moz-range-thumb {
        width: 18px; height: 18px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer;
        box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]:focus-visible {
        box-shadow: 0 0 0 2px var(--on-soft);
    }

    .joiners { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .unreg { font-size: 11px; color: var(--text-dim); }

    /* ── Favorites ── */
    .favs-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3); flex-wrap: wrap;
    }
    .fav-targets { display: flex; gap: var(--space-2); flex-wrap: wrap; }
    .favs {
        display: flex; gap: var(--space-3);
        padding-bottom: var(--space-1);
    }
    .fav {
        display: flex; flex-direction: column; gap: 6px;
        width: 112px;
        background: transparent; border: 0; padding: 0;
        cursor: pointer; text-align: left;
        color: var(--text);
        font: inherit;
    }
    .fav:disabled { opacity: 0.5; cursor: default; }
    .fav-art {
        width: 112px; height: 112px;
        border-radius: var(--r-md);
        object-fit: cover;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        transition: transform 120ms ease;
    }
    div.fav-art { font-size: 10px; }
    @media (hover: hover) {
        .fav:hover .fav-art { transform: translateY(-1px); }
    }
    .fav:active .fav-art { transform: scale(0.97); }
    .fav-title {
        font-size: 12.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .fav-sub { font-size: 10px; color: var(--text-dim); letter-spacing: 0.04em; }

    /* ── Spotify search ── */
    .sp-help { font-size: 12.5px; color: var(--text-mute); line-height: 1.5; }
    .sp-steps {
        margin: 0; padding-left: 20px;
        display: flex; flex-direction: column; gap: var(--space-2);
        font-size: 12.5px; color: var(--text-mute); line-height: 1.5;
    }
    .sp-steps li::marker {
        font-family: var(--font-mono);
        color: var(--text-dim);
    }
    .sp-link { color: var(--on); text-decoration: underline; text-underline-offset: 2px; }
    .sp-redirect {
        display: flex; align-items: center; gap: var(--space-2);
        flex-wrap: wrap;
        margin-top: 4px;
    }
    .sp-redirect code {
        font-family: var(--font-mono);
        font-size: 12px; color: var(--text);
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-sm);
        padding: 4px 8px;
        word-break: break-all; user-select: all;
    }
    .sp-paste label { font-size: 12.5px; color: var(--text-mute); }
    .sp-config { display: flex; gap: var(--space-2); align-items: center; }
    .sp-config input { flex: 1; min-width: 0; }
    .sp-actions { display: flex; gap: var(--space-2); }

    .sp-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    .sp-account { display: flex; align-items: center; gap: var(--space-2); }
    .sp-user { font-size: 11px; color: var(--text-mute); }

    .sp-filters {
        display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap;
    }
    .sp-browse-label {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .sp-targets { margin-left: auto; }
    .sp-skeleton { height: 120px; border-radius: var(--r-md); }
    .sp-none { font-size: 12.5px; color: var(--text-mute); }

    .sp-results { display: flex; flex-direction: column; gap: 2px; }
    .sp-row {
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 52px;
        padding: 6px var(--space-2);
        background: transparent;
        border: 0; border-radius: var(--r-md);
        color: var(--text);
        cursor: pointer; text-align: left; font: inherit;
        transition: background 150ms ease;
    }
    .sp-row:hover:not(:disabled) { background: var(--card-2); }
    .sp-row:active:not(:disabled) { background: var(--card-3); }
    .sp-row:disabled { opacity: 0.5; cursor: default; }
    .sp-art {
        width: 40px; height: 40px;
        border-radius: var(--r-sm);
        object-fit: cover;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    div.sp-art { font-size: 8px; }
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
        width: 36px; height: 36px;
        display: grid; place-items: center;
        border-radius: 50%;
        color: var(--text-mute);
        flex-shrink: 0;
        transition: color 150ms ease, background 150ms ease;
    }
    .sp-row:hover .sp-play { background: var(--on-soft); color: var(--on); }

    /* Touch: sliders and hit areas grow to the 44px floor. */
    @media (pointer: coarse) {
        .t-btn { width: 44px; height: 44px; }
        .t-main { width: 52px; height: 52px; }
        .m-mute, .m-act { width: 44px; height: 44px; }
        input[type="range"] { height: 10px; border-radius: 5px; }
        input[type="range"]::-webkit-slider-thumb { width: 26px; height: 26px; }
        input[type="range"]::-moz-range-thumb { width: 26px; height: 26px; }
        .member .m-name { width: 90px; }
        .sp-play { width: 44px; height: 44px; }
        .sp-input { font-size: 16px; } /* prevents iOS auto-zoom */
    }

    @media (max-width: 600px) {
        .now { flex-wrap: wrap; }
        .transport { margin-left: auto; }
    }

    @media (prefers-reduced-motion: reduce) {
        .fav-art, .t-main { transition-duration: 0.001ms; }
    }
</style>

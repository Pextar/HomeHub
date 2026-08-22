<script lang="ts">
    import Icon from "../components/Icon.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import NowPlaying from "../components/NowPlaying.svelte";
    import HomeHero from "../components/home/HomeHero.svelte";
    import HomeFavorites from "../components/home/HomeFavorites.svelte";
    import HomeGroups from "../components/home/HomeGroups.svelte";
    import HomeRooms from "../components/home/HomeRooms.svelte";
    import HomeSensors from "../components/home/HomeSensors.svelte";
    import HomeTimers from "../components/home/HomeTimers.svelte";
    import HomeDevices from "../components/home/HomeDevices.svelte";
    import HomeEditor from "../components/home/HomeEditor.svelte";
    import AddDeviceModal from "../modals/AddDeviceModal.svelte";
    import { route, data, session } from "../lib/stores.svelte";
    import { homeLayout } from "../lib/home-layout.svelte";
    import { sectionsFor } from "../lib/home-layout";
    import { haptic } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";

    /**
     * Home.
     *
     * The screen used to be a fixed run of sections in source order. It is now
     * a *list* — `lib/home-layout.ts` says which sections exist and the user's
     * stored layout says which of them appear and in what order — and this
     * file is the shell around it: the greeting, the clock, and the switch
     * into the arranging mode. Each section is its own component under
     * `components/home/`, which is what lets them be reordered at all: a
     * section has to be one thing you can move, and its CSS has to travel with
     * it (see frontend/CLAUDE.md on splitting a big file).
     */

    const v = $derived(data.value);

    // ── Live clock ───────────────────────────────────────────────────────────
    let now = $state(new Date());
    $effect(() => {
        const id = setInterval(() => {
            now = new Date();
        }, 1000);
        return () => clearInterval(id);
    });
    const greeting = $derived(
        now.getHours() < 12 ? "Good morning"
        : now.getHours() < 18 ? "Good afternoon"
        : "Good evening",
    );
    const dateLabel = $derived(
        now.toLocaleDateString([], { weekday: "long" }) +
            ", " +
            now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
    );
    const name = $derived(session.user?.username || "there");
    // Derived from the minute, not the 1-second tick — the hero re-sorts the
    // whole schedule list off this, which it must not do every second.
    const nowMinute = $derived(now.getHours() * 60 + now.getMinutes());

    // ── The layout ───────────────────────────────────────────────────────────
    let editing = $state(false);
    const sections = $derived(
        sectionsFor(homeLayout.layout, session.isAdmin).filter((s) => !homeLayout.isHidden(s.id)),
    );

    function customise() {
        haptic();
        editing = true;
    }
</script>

{#if editing}
    <HomeEditor onDone={() => (editing = false)} />
{:else}
    <!-- ── Greeting header ────────────────────────────────────────────── -->
    <header class="greeting">
        <div class="greet-text">
            <div class="greet-date mono">{dateLabel}</div>
            <h1 class="greet-title">{greeting},<br /><span class="greet-name">{name}</span></h1>
        </div>
        <div class="greet-actions">
            {#if session.isAdmin}
                <button
                    class="chip search-chip"
                    onclick={() => route.go("sockets", { focus: "search" })}
                    aria-label="Search devices"
                >
                    <Icon name="search" size={14} />
                    Search devices…
                </button>
                <button class="chip add-device" onclick={() => openModal(AddDeviceModal, {})}>
                    <Icon name="plus" size={14} /> Add device
                </button>
            {/if}
            <button class="chip icon-chip" aria-label="Customise home screen" onclick={customise}>
                <Icon name="sliders" size={16} />
            </button>
            {#if session.isAdmin}
                <button class="chip icon-chip" aria-label="Activity" onclick={() => route.go("activity")}>
                    <Icon name="activity" size={16} />
                </button>
            {/if}
        </div>
    </header>

    <!-- ── The sections, in the order this device keeps them ──────────── -->
    {#each sections as s (s.id)}
        {#if s.id === "hero"}
            <HomeHero {nowMinute} />
        {:else if s.id === "nowplaying"}
            <!-- Owns its own Sonos poll and hides itself when there are no
                 speakers, so it costs nothing on a home without them. -->
            {#if v.loaded}<NowPlaying />{/if}
        {:else if s.id === "favorites"}
            <HomeFavorites />
        {:else if s.id === "groups"}
            <HomeGroups />
        {:else if s.id === "rooms"}
            {#if v.loaded}<HomeRooms />{/if}
        {:else if s.id === "sensors"}
            <HomeSensors />
        {:else if s.id === "timers"}
            <HomeTimers />
        {:else if s.id === "devices"}
            <HomeDevices />
        {/if}
    {/each}

    {#if sections.length === 0}
        <EmptyState
            fill
            icon="sliders"
            title="Nothing on your home screen"
            message="Every section is switched off. Bring some back and put them in the order you want them."
        >
            <button class="btn btn-primary" onclick={customise}>Customise home</button>
        </EmptyState>
    {/if}
{/if}

<style>
    .greeting {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .greet-date { color: var(--text-mute); font-size: 13px; font-weight: 500; }
    .greet-title {
        font-size: 30px;
        font-weight: 600;
        letter-spacing: -0.03em;
        margin-top: 4px;
        line-height: 1.1;
    }
    .greet-name { color: var(--text-mute); }
    .greet-actions { display: flex; gap: var(--space-2); flex-shrink: 0; align-items: center; }
    .icon-chip {
        width: 38px; height: 38px;
        padding: 0;
        justify-content: center;
    }
    /* Search chip and "Add device" are desktop-only labels */
    .search-chip { display: none; }
    .add-device { display: none; }
    @media (min-width: 700px) { .add-device { display: inline-flex; } }
    @media (min-width: 1024px) {
        .search-chip {
            display: inline-flex;
            gap: 8px;
            padding: 9px 14px;
            color: var(--text-mute);
        }
    }
</style>

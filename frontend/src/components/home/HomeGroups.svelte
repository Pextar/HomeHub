<script lang="ts">
    import HomeSectionHead from "./HomeSectionHead.svelte";
    import Icon from "../Icon.svelte";
    import Switch from "../Switch.svelte";
    import { data, route } from "../../lib/stores.svelte";
    import { api } from "../../lib/api";
    import { runAction } from "../../lib/utils";
    import { scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../../lib/motion";

    const v = $derived(data.value);

    // Groups with a live on-count, so the tile tracks optimistic toggles
    // rather than waiting for the next server refresh.
    const groups = $derived(
        v.groups.map((g) => ({
            ...g,
            on: g.socket_ids.filter((id) => v.sockets.find((s) => s.id === id)?.state).length,
        })),
    );

    // A group's switch means "everything in it", matching the Groups view.
    function toggleGroup(g: { id: string; name: string }, on: boolean) {
        runAction(() => api.groupAction(g.id, on ? "on" : "off"));
    }
</script>

{#if groups.length > 0}
    <section class="home-section">
        <HomeSectionHead icon="groups" title="Groups">
            {#snippet actions()}
                <button class="chip" onclick={() => route.go("groups")}>All</button>
            {/snippet}
        </HomeSectionHead>
        <div class="home-grid">
            {#each groups as g, i (g.id)}
                {@const anyOn = g.on > 0}
                <div
                    class="tile group-tile"
                    class:on={anyOn}
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}
                >
                    <div class="gt-top">
                        <span class="gt-ico" class:on={anyOn}><Icon name="groups" size={17} /></span>
                        <Switch
                            checked={anyOn}
                            onChange={(c) => toggleGroup(g, c)}
                            ariaLabel="Toggle {g.name}"
                        />
                    </div>
                    <button class="gt-body" onclick={() => route.go("groups")} aria-label="Open {g.name}">
                        <span class="gt-name">{g.name}</span>
                        <span class="gt-meta">
                            <span class="count" class:lit={anyOn}>{g.on}</span><span class="slash">
                                / {g.socket_ids.length}</span
                            > on
                        </span>
                    </button>
                </div>
            {/each}
        </div>
    </section>
{/if}

<style>
    /* Tiles, not flat rows: a group is an on/off surface like everything else
       on this page, so it gets the sanctioned .tile.on treatment and the same
       switch the Groups view uses. */
    .group-tile { min-width: 0; }
    .gt-top { display: flex; justify-content: space-between; align-items: flex-start; }
    .gt-ico {
        width: 36px; height: 36px;
        border-radius: 10px;
        background: var(--card-3);
        color: var(--text-mute);
        display: grid; place-items: center;
        flex-shrink: 0;
        transition: background var(--t-med), color var(--t-med);
    }
    .gt-ico.on { background: var(--on); color: var(--primary-fg); }
    .gt-body {
        all: unset;
        cursor: pointer;
        touch-action: manipulation;
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
    }
    .gt-body:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-sm); }
    .gt-name {
        font-weight: 600; font-size: 15px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .gt-meta {
        color: var(--text-mute); font-size: 12.5px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .count { font-family: var(--font-mono); font-feature-settings: "tnum" 1; color: var(--text-mute); }
    .count.lit { color: var(--on); }
    .slash { color: var(--text-dim); }
</style>

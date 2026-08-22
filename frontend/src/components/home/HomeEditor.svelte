<script lang="ts">
    import Icon from "../Icon.svelte";
    import HomeEditorRow from "./HomeEditorRow.svelte";
    import ConfirmModal from "../ConfirmModal.svelte";
    import HomeSensorsModal from "../../modals/HomeSensorsModal.svelte";
    import { data, session } from "../../lib/stores.svelte";
    import { homeLayout } from "../../lib/home-layout.svelte";
    import { homeSensors, sectionsFor, type HomeSectionId } from "../../lib/home-layout";
    import { createListReorder } from "../../lib/list-reorder.svelte";
    import { openModal, modalStack } from "../../lib/modal.svelte";
    import { plural } from "../../lib/utils";
    import { fly } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../../lib/motion";

    /**
     * Arranging the home screen.
     *
     * It is a **mode on the page**, not a settings screen somewhere else: the
     * thing being arranged is this route, and the list is in the order the
     * page is. Sections collapse to one row each while it is up, because a
     * 400px-tall Rooms grid is not something you can drag around a phone —
     * what you are moving here is the *order*, and a row is the honest size
     * for that.
     *
     * Everything applies the moment it is done. There is no Save: a switch
     * that has to be committed is a switch that can be lost, and the screen
     * behind this one is the confirmation (DESIGN.md §10).
     */
    interface Props {
        onDone: () => void;
    }
    let { onDone }: Props = $props();

    const v = $derived(data.value);

    /** The sections this profile can arrange, in their current order. */
    const sections = $derived(sectionsFor(homeLayout.layout, session.isAdmin));
    const ids = $derived(sections.map((s) => s.id));

    let listEl = $state<HTMLElement>();
    let liveMsg = $state("");

    /**
     * A move is expressed against the list on screen, which for a non-admin is
     * a subset of the stored order — so the target index is translated back
     * through the sections they can't see, which keep their places.
     */
    function moveVisible(id: string, to: number) {
        const full = homeLayout.order.filter((x) => x !== id);
        const rest = ids.filter((x) => x !== id);
        const at =
            to <= 0 ?
                rest.length ? full.indexOf(rest[0])
                :   full.length
            :   full.indexOf(rest[Math.min(to, rest.length) - 1]) + 1;
        homeLayout.move(id as HomeSectionId, at);
    }

    const reorder = createListReorder({
        ids: () => ids,
        move: moveVisible,
        rowOf: (id) => listEl?.querySelector<HTMLElement>(`[data-row="${id}"]`) ?? null,
        label: (id) => sections.find((s) => s.id === id)?.title ?? id,
        announce: (m) => (liveMsg = m),
    });

    /** What each section is holding right now — the reason to keep it or not. */
    function summary(id: HomeSectionId): string {
        switch (id) {
            case "hero": {
                const on = v.sockets.filter((s) => s.state).length;
                return `${on} of ${v.sockets.length} devices on`;
            }
            case "favorites": {
                const n = v.sockets.filter((s) => s.favorite).length;
                return n === 0 ? "Nothing starred yet" : plural(n, "starred device");
            }
            case "groups":
                return v.groups.length === 0 ? "No groups yet" : plural(v.groups.length, "group");
            case "rooms":
                return v.rooms.length === 0 ? "No rooms yet" : plural(v.rooms.length, "room");
            case "sensors": {
                if (v.sensors.length === 0) return "No sensors yet";
                const shown = homeSensors(v.sensors, homeLayout.layout).length;
                return homeLayout.sensors === null ?
                        `First ${shown} of ${v.sensors.length}, automatically`
                    :   `${shown} of ${v.sensors.length} sensors chosen`;
            }
            case "timers":
                return v.timers.length === 0 ?
                        "Nothing counting down"
                    :   plural(v.timers.length, "timer") + " running";
            case "devices":
                return plural(v.sockets.length, "device") + ", filterable by room";
            case "nowplaying":
                return "Whatever a speaker is playing";
        }
    }

    async function reset() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Reset the home screen?",
            message: "Every section comes back, in the order it shipped in. Your sensor choices are cleared too.",
            confirmLabel: "Reset",
        });
        if (!ok) return;
        homeLayout.reset();
        liveMsg = "Home screen reset to its default layout.";
    }

    // Escape leaves the mode — but only when it is the frontmost thing; a
    // sheet opened from a row answers it first.
    function onKey(e: KeyboardEvent) {
        if (e.key !== "Escape" || modalStack().length > 0 || reorder.lifted) return;
        onDone();
    }

    // A section that stops existing mid-drag (a profile change, a reset)
    // shouldn't leave the gesture holding a row that isn't there.
    $effect(() => {
        reorder.prune(new Set(ids));
    });
</script>

<svelte:window
    onpointermove={reorder.onPointerMove}
    onpointerup={reorder.onPointerUp}
    onpointercancel={reorder.end}
    onkeydown={onKey}
/>

<header class="edit-head" in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}>
    <div class="lead">
        <div class="eyebrow mono">Home screen</div>
        <h1>Customise</h1>
    </div>
    <button class="chip on done" onclick={onDone}>
        <Icon name="check" size={14} /> Done
    </button>
</header>

<p class="lede">
    Drag a section by its handle to move it, or switch it off to take it off the
    home screen. Sensors chooses its own readings.
</p>

<ul class="rows" bind:this={listEl}>
    {#each sections as s (s.id)}
        <!-- The slot animates when the order changes (a keyboard move); the row
             inside it carries the drag's own transform. Two elements because
             one would have FLIP and the gesture writing the same property. -->
        <li class="slot" animate:flip={{ duration: dur(220), easing: cubicOut }}>
            <HomeEditorRow
                section={s}
                summary={summary(s.id)}
                shown={!homeLayout.isHidden(s.id)}
                offset={reorder.offset(s.id)}
                lifted={reorder.lifted === s.id}
                settling={reorder.settling}
                onShow={(on) => homeLayout.setHidden(s.id, !on)}
                onOptions={s.options ? () => openModal(HomeSensorsModal, {}) : undefined}
                onPointerDown={(e) => reorder.onPointerDown(e, s.id)}
                onKeyDown={(e) => reorder.onKeyDown(e, s.id)}
                onClickCapture={reorder.onClickCapture}
            />
        </li>
    {/each}
</ul>

<!-- Stated once under the list, not as chrome on every row (DESIGN.md §15.4). -->
<p class="footnote">
    With a handle focused, <kbd>↑</kbd> and <kbd>↓</kbd> move its section a place at a time.
</p>

<div class="edit-foot">
    <button class="btn btn-ghost" onclick={reset}>Reset to default</button>
</div>

<div class="sr-only" role="status" aria-live="polite">{liveMsg}</div>

<style>
    .edit-head {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .eyebrow { color: var(--on); font-size: 11px; letter-spacing: 0.1em; text-transform: uppercase; }
    .edit-head h1 {
        font-size: 30px;
        font-weight: 600;
        letter-spacing: -0.03em;
        margin-top: 4px;
        line-height: 1.1;
    }
    .done { min-height: 38px; }
    .lede { color: var(--text-mute); font-size: 13px; line-height: 1.5; max-width: 46ch; }

    /* A 60px row stretched across a 1600px window puts its switch a screen
       away from its name. The list is a column of rows, so it gets a column's
       width and stops. */
    .rows,
    .footnote,
    .edit-foot { max-width: 760px; }

    .rows {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .slot { display: block; }

    .footnote { color: var(--text-dim); font-size: 12px; }
    .footnote kbd {
        font-family: var(--font-mono);
        font-size: 11px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: 5px;
        padding: 1px 5px;
    }

    .edit-foot { display: flex; }
</style>

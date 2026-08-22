<script lang="ts">
    import Icon from "../Icon.svelte";
    import Switch from "../Switch.svelte";
    import type { HomeSection } from "../../lib/home-layout";

    /**
     * One section, as a row you can pick up.
     *
     * Three targets, left to right, each its own ≥44px control: the grip
     * moves the row, the body opens whatever settings the section has, and
     * the switch decides whether it is on the home screen at all. A section
     * with nothing to configure gets no body button rather than a button that
     * does nothing when you press it.
     */
    interface Props {
        section: HomeSection;
        /** What this section holds right now, in one line. */
        summary: string;
        shown: boolean;
        /** Where the drag has this row, in px. */
        offset: number;
        lifted: boolean;
        /** One frame after a drop, while the list re-orders under the finger. */
        settling: boolean;
        onShow: (on: boolean) => void;
        onOptions?: () => void;
        onPointerDown: (e: PointerEvent) => void;
        onKeyDown: (e: KeyboardEvent) => void;
        onClickCapture: (e: MouseEvent) => void;
    }
    let {
        section,
        summary,
        shown,
        offset,
        lifted,
        settling,
        onShow,
        onOptions,
        onPointerDown,
        onKeyDown,
        onClickCapture,
    }: Props = $props();

    const only = $derived(
        section.only === "phone" ? "Phone" : section.only === "desktop" ? "Desktop" : "",
    );
</script>

<div
    class="row"
    class:off={!shown}
    class:lifted
    class:settling
    data-row={section.id}
    style="transform: translateY({offset}px)"
>
    <button
        class="grip"
        type="button"
        aria-label="Reorder {section.title}"
        onpointerdown={onPointerDown}
        onkeydown={onKeyDown}
        onclickcapture={onClickCapture}
    >
        <Icon name="grip" size={18} />
    </button>

    {#if onOptions}
        <!-- Named for what it opens: the visible line is the section's title
             and its summary, which is not what a settings button promises. -->
        <button class="body" type="button" aria-label="{section.title} settings" onclick={onOptions}>
            <span class="ico"><Icon name={section.icon} size={17} /></span>
            <span class="text">
                <span class="name">
                    {section.title}
                    {#if only}<span class="only mono">{only}</span>{/if}
                </span>
                <span class="sub">{summary}</span>
            </span>
            <span class="chev"><Icon name="chevronRight" size={16} /></span>
        </button>
    {:else}
        <div class="body plain">
            <span class="ico"><Icon name={section.icon} size={17} /></span>
            <span class="text">
                <span class="name">
                    {section.title}
                    {#if only}<span class="only mono">{only}</span>{/if}
                </span>
                <span class="sub">{summary}</span>
            </span>
        </div>
    {/if}

    <Switch checked={shown} onChange={onShow} ariaLabel="Show {section.title} on the home screen" />
</div>

<style>
    .row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: 8px 12px 8px 4px;
        min-height: 60px;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        /* The shove: rows step out of a lifted row's way. Off while a drop
           settles, so nothing animates back from where it already is. */
        transition: transform 180ms var(--spring), opacity var(--t-med), box-shadow var(--t-fast);
        position: relative;
        touch-action: pan-y;
    }
    .row.settling { transition: none; }
    /* The lifted row is under the finger: it must not lag behind it. */
    .row.lifted {
        transition: opacity var(--t-med), box-shadow var(--t-fast);
        z-index: 2;
        border-color: var(--on);
        box-shadow: var(--shadow-lg);
        background: var(--card-2);
    }
    /* A switched-off section keeps its place in the list — it just stops
       claiming to be part of the screen. */
    .row.off { opacity: 0.55; }
    .row.off .ico { background: var(--card-2); color: var(--text-dim); }

    .grip {
        all: unset;
        flex-shrink: 0;
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        color: var(--text-dim);
        cursor: grab;
        border-radius: var(--r-sm);
        /* The gesture owns this control outright — without this the browser
           takes the first vertical millimetre for a page scroll and the row
           never lifts on a phone. */
        touch-action: none;
    }
    .grip:active { cursor: grabbing; color: var(--on); }
    .grip:focus-visible { box-shadow: var(--focus-ring); }
    @media (hover: hover) {
        .grip:hover { color: var(--text-mute); }
    }

    .body {
        all: unset;
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 44px;
        cursor: pointer;
        touch-action: manipulation;
    }
    .body.plain { cursor: default; }
    .body:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-sm); }

    .ico {
        width: 36px; height: 36px;
        flex-shrink: 0;
        border-radius: 10px;
        background: var(--on-soft);
        color: var(--on);
        display: grid; place-items: center;
        transition: background var(--t-med), color var(--t-med);
    }
    .text { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
    .name {
        font-size: 14.5px; font-weight: 600;
        display: flex; align-items: center; gap: 6px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* Two sections only render at one end of the breakpoint (DESIGN.md §5).
       Saying so is better than a switch that promises more than it can do. */
    .only {
        font-size: 9.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        padding: 1px 6px;
        flex-shrink: 0;
    }
    .sub {
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .chev { color: var(--text-dim); flex-shrink: 0; display: inline-flex; }
</style>

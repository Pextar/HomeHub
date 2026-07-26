<script lang="ts">
    /**
     * Album art — the largest element on the player, carrying the bulb glow
     * underneath (the same light a lit device gives off).
     *
     * On a phone it is also the transport: it is the biggest target on the
     * screen, so it **swipes sideways to change track**, following the finger
     * at half speed and firing past ~60px of travel. Vertical always loses to
     * the sheet's own drag and scroll, so the two gestures never fight.
     *
     * `onSkip` absent means the source has no skips to give and the art is
     * just art.
     */
    let {
        artUri = undefined,
        /** Set while the sheet is riding out a committed drag-down: no new
         *  gesture should start during those 220ms. */
        sheetDismissing = false,
        onSkip = undefined,
    }: {
        artUri?: string;
        sheetDismissing?: boolean;
        onSkip?: (dir: "next" | "previous") => void;
    } = $props();

    let dx = $state(0);
    let swiping = $state(false);
    let start: { x: number; y: number } | null = null;

    function onPointerDown(e: PointerEvent) {
        if (e.pointerType === "mouse" || sheetDismissing || !onSkip) return;
        start = { x: e.clientX, y: e.clientY };
        dx = 0;
    }
    function onPointerMove(e: PointerEvent) {
        if (!start) return;
        const moveX = e.clientX - start.x;
        const moveY = e.clientY - start.y;
        if (!swiping) {
            // Vertical wins early: that gesture belongs to the sheet.
            if (Math.abs(moveY) > 10 && Math.abs(moveY) >= Math.abs(moveX)) {
                start = null;
                return;
            }
            if (Math.abs(moveX) < 12) return;
            swiping = true;
            try {
                (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
            } catch {
                /* not capturable */
            }
        }
        // Half speed: the art nudges along with the finger rather than being
        // thrown off the screen.
        dx = moveX * 0.5;
        e.preventDefault();
    }
    function onPointerUp() {
        if (!start) return;
        const moved = dx;
        start = null;
        swiping = false;
        dx = 0;
        if (Math.abs(moved) < 30) return; // ~60px of travel
        onSkip?.(moved < 0 ? "next" : "previous");
    }
    function onPointerCancel() {
        start = null;
        swiping = false;
        dx = 0;
    }
</script>

<div
    class="p-art"
    class:swiping
    role="none"
    onpointerdown={onPointerDown}
    onpointermove={onPointerMove}
    onpointerup={onPointerUp}
    onpointercancel={onPointerCancel}
    style:transform={dx ? `translateX(${dx}px)` : ""}
    style:opacity={swiping ? Math.max(0.55, 1 - Math.abs(dx) / 200) : undefined}
>
    {#if artUri}
        <img src={artUri} alt="" draggable="false" />
    {:else}
        <div class="p-art-ph">[ album art ]</div>
    {/if}
</div>

<style>
    .p-art {
        display: flex; justify-content: center; padding: var(--space-2) 0 0;
        /* Horizontal is the swipe-to-skip gesture's; vertical stays the
           sheet's (scroll, drag-to-dismiss). */
        touch-action: pan-y;
        transition: transform 260ms var(--spring), opacity var(--t-fast);
        will-change: transform;
    }
    .p-art.swiping { transition: none; }
    .p-art img { user-select: none; -webkit-user-drag: none; }
    .p-art img, .p-art-ph {
        width: min(340px, 78vw); aspect-ratio: 1;
        border-radius: var(--r-lg); object-fit: cover;
    }
    .p-art img {
        background: var(--card-3); border: 1px solid var(--tile-on-border);
        box-shadow: 0 18px 40px -18px var(--on-glow);
    }
    .p-art-ph {
        display: grid; place-items: center;
        background: var(--tile-on-gradient); border: 1px solid var(--tile-on-border);
        color: var(--text-dim); font-family: var(--font-mono); font-size: 11px;
    }
    @media (prefers-reduced-motion: reduce) {
        .p-art { transition-duration: 0.001ms; }
    }
</style>

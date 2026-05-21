<!--
  HSV color wheel: hue around the circumference, saturation from center to
  edge. Brightness is controlled separately (a slider in the modal), so the
  wheel itself stays at V=1.0 — that's standard for smart-light pickers and
  keeps colors readable when picking.

  Implementation notes:
    - The wheel is painted once to a canvas on first render and on resize.
      Recomputing per-pixel HSV→RGB on every pointer move would be wasteful
      and Safari struggles with it.
    - Pointer events use setPointerCapture so a drag that starts inside the
      circle keeps tracking even when the finger leaves the wheel.
    - We clamp the puck to the inner disc (sat ≤ 1.0) but allow the pointer
      to wander; this matches Hue/Home and avoids the puck snapping away
      from the finger.
-->
<script lang="ts">
    interface Props {
        // Current color as #RRGGBB (uppercase or lowercase).
        color: string;
        // Called continuously as the user drags. Receives #RRGGBB uppercase.
        onChange: (color: string) => void;
        disabled?: boolean;
        size?: number;
    }
    let { color, onChange, disabled = false, size = 260 }: Props = $props();

    let canvas: HTMLCanvasElement | undefined = $state();
    let puckEl: HTMLDivElement | undefined = $state();
    let wrapEl: HTMLDivElement | undefined = $state();
    let dragging = $state(false);

    // Local mirror of incoming color, normalized to {h: 0..360, s: 0..1}.
    // Derived from the prop except while the user is actively dragging,
    // when local input wins so a slow network round-trip can't fight the UI.
    let h = $state(0);
    let s = $state(0);

    $effect(() => {
        if (dragging) return;
        const hs = hexToHsv(color);
        h = hs.h;
        s = hs.s;
    });

    // Repaint whenever the canvas mounts or the size changes — a single
    // onMount paint would leave a blank/stale wheel after a responsive resize.
    $effect(() => {
        // Touch `size` and `canvas` so the effect re-runs when either changes.
        size;
        if (canvas) paint();
    });

    // Keyboard control: ←/→ adjust hue, ↑/↓ adjust saturation. Shift = fine.
    function onKeyDown(e: KeyboardEvent) {
        if (disabled) return;
        const hueStep = e.shiftKey ? 2 : 8;
        const satStep = e.shiftKey ? 0.02 : 0.06;
        let handled = true;
        switch (e.key) {
            case "ArrowLeft":  h = (h - hueStep + 360) % 360; break;
            case "ArrowRight": h = (h + hueStep) % 360; break;
            case "ArrowUp":    s = Math.min(1, s + satStep); break;
            case "ArrowDown":  s = Math.max(0, s - satStep); break;
            default: handled = false;
        }
        if (handled) {
            e.preventDefault();
            onChange(rgbToHex(hsvToRgb(h, s, 1)));
        }
    }

    function paint() {
        if (!canvas) return;
        const dpr = window.devicePixelRatio || 1;
        const px = size * dpr;
        canvas.width = px;
        canvas.height = px;
        canvas.style.width = size + "px";
        canvas.style.height = size + "px";
        const ctx = canvas.getContext("2d");
        if (!ctx) return;
        const r = px / 2;
        const img = ctx.createImageData(px, px);
        const data = img.data;
        // Walk every pixel, compute polar coordinates, convert HSV→RGB.
        for (let y = 0; y < px; y++) {
            for (let x = 0; x < px; x++) {
                const dx = x - r;
                const dy = y - r;
                const dist = Math.sqrt(dx * dx + dy * dy);
                if (dist > r) continue;
                const sat = Math.min(1, dist / r);
                let hue = (Math.atan2(dy, dx) * 180) / Math.PI;
                if (hue < 0) hue += 360;
                const [rr, gg, bb] = hsvToRgb(hue, sat, 1);
                const idx = (y * px + x) * 4;
                data[idx]     = rr;
                data[idx + 1] = gg;
                data[idx + 2] = bb;
                data[idx + 3] = 255;
            }
        }
        ctx.putImageData(img, 0, 0);
    }

    function updateFromEvent(e: PointerEvent) {
        if (!wrapEl) return;
        const rect = wrapEl.getBoundingClientRect();
        const cx = rect.width / 2;
        const cy = rect.height / 2;
        const dx = e.clientX - rect.left - cx;
        const dy = e.clientY - rect.top - cy;
        const r = rect.width / 2;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const sat = Math.min(1, dist / r);
        let hue = (Math.atan2(dy, dx) * 180) / Math.PI;
        if (hue < 0) hue += 360;
        h = hue;
        s = sat;
        const hex = rgbToHex(hsvToRgb(h, s, 1));
        onChange(hex);
    }

    function onPointerDown(e: PointerEvent) {
        if (disabled) return;
        dragging = true;
        (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
        updateFromEvent(e);
    }
    function onPointerMove(e: PointerEvent) {
        if (!dragging) return;
        updateFromEvent(e);
    }
    function onPointerUp(e: PointerEvent) {
        if (!dragging) return;
        dragging = false;
        try { (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId); } catch {}
    }

    // Puck position in pixels relative to the wrap (centered coords).
    const puckX = $derived.by(() => {
        const r = size / 2;
        return r + Math.cos((h * Math.PI) / 180) * s * r;
    });
    const puckY = $derived.by(() => {
        const r = size / 2;
        return r + Math.sin((h * Math.PI) / 180) * s * r;
    });
    const puckColor = $derived(rgbToHex(hsvToRgb(h, s, 1)));

    // --- HSV / RGB helpers ---
    function hsvToRgb(h: number, s: number, v: number): [number, number, number] {
        const c = v * s;
        const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
        const m = v - c;
        let r = 0, g = 0, b = 0;
        if      (h < 60)  { r = c; g = x; }
        else if (h < 120) { r = x; g = c; }
        else if (h < 180) { g = c; b = x; }
        else if (h < 240) { g = x; b = c; }
        else if (h < 300) { r = x; b = c; }
        else              { r = c; b = x; }
        return [
            Math.round((r + m) * 255),
            Math.round((g + m) * 255),
            Math.round((b + m) * 255),
        ];
    }
    function rgbToHex([r, g, b]: [number, number, number]): string {
        const h = (n: number) => n.toString(16).padStart(2, "0").toUpperCase();
        return "#" + h(r) + h(g) + h(b);
    }
    function hexToHsv(hex: string): { h: number; s: number; v: number } {
        const m = hex.replace(/^#/, "");
        if (m.length !== 6) return { h: 0, s: 0, v: 1 };
        const r = parseInt(m.slice(0, 2), 16) / 255;
        const g = parseInt(m.slice(2, 4), 16) / 255;
        const b = parseInt(m.slice(4, 6), 16) / 255;
        const max = Math.max(r, g, b), min = Math.min(r, g, b);
        const d = max - min;
        let hue = 0;
        if (d !== 0) {
            if      (max === r) hue = ((g - b) / d) % 6;
            else if (max === g) hue = (b - r) / d + 2;
            else                hue = (r - g) / d + 4;
            hue = hue * 60;
            if (hue < 0) hue += 360;
        }
        const sat = max === 0 ? 0 : d / max;
        return { h: hue, s: sat, v: max };
    }
</script>

<div bind:this={wrapEl}
    class="wheel-wrap"
    class:disabled
    style:width="{size}px"
    style:height="{size}px"
    onpointerdown={onPointerDown}
    onpointermove={onPointerMove}
    onpointerup={onPointerUp}
    onpointercancel={onPointerUp}
    onkeydown={onKeyDown}
    role="slider"
    tabindex={disabled ? -1 : 0}
    aria-label="Color"
    aria-valuemin={0}
    aria-valuemax={360}
    aria-valuenow={Math.round(h)}
    aria-valuetext={puckColor}
    aria-disabled={disabled}>
    <canvas bind:this={canvas}></canvas>
    <div bind:this={puckEl}
        class="puck"
        style:left="{puckX}px"
        style:top="{puckY}px"
        style:background={puckColor}>
    </div>
</div>

<style>
    .wheel-wrap {
        position: relative;
        border-radius: 50%;
        touch-action: none;
        user-select: none;
        cursor: crosshair;
        box-shadow: 0 8px 24px rgba(0,0,0,0.25), inset 0 0 0 2px rgba(255,255,255,0.08);
    }
    .wheel-wrap.disabled {
        opacity: 0.4;
        cursor: not-allowed;
        filter: grayscale(0.7);
    }
    .wheel-wrap:focus-visible {
        outline: none;
        box-shadow: 0 8px 24px rgba(0,0,0,0.25), var(--focus-ring);
    }
    canvas {
        display: block;
        border-radius: 50%;
        pointer-events: none;
    }
    .puck {
        position: absolute;
        width: 22px;
        height: 22px;
        border-radius: 50%;
        border: 3px solid #fff;
        transform: translate(-50%, -50%);
        box-shadow: 0 2px 6px rgba(0,0,0,0.45), 0 0 0 1px rgba(0,0,0,0.25);
        pointer-events: none;
        transition: transform 0.05s linear;
    }
</style>

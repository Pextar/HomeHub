<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import { data, route } from "../lib/stores.svelte";
    import { socketAction, protocolKind } from "../lib/utils";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";
    import type { Socket } from "../lib/types";

    const v = $derived(data.value);

    // ── Blueprint canvas geometry ───────────────────────────────────
    // A fixed viewBox the SVG scales into. Light positions live in this
    // coordinate space so a saved layout survives any screen size.
    const VIEW_W = 360;
    const VIEW_H = 480;
    const NODE_R = 9; // outer ring radius

    // ── Saved layout — per-socket x/y, client-only (localStorage). ───
    // The data model has no geometry, so the plan is something the user
    // arranges themselves. Defaults cluster each room together; dragging
    // in edit mode overrides a node and persists it here.
    const POS_KEY = "floorplan.positions.v1";
    type Pos = { x: number; y: number };
    function loadPositions(): Record<string, Pos> {
        try {
            const raw = localStorage.getItem(POS_KEY);
            return raw ? JSON.parse(raw) : {};
        } catch { return {}; }
    }
    let positions = $state<Record<string, Pos>>(loadPositions());
    $effect(() => {
        try { localStorage.setItem(POS_KEY, JSON.stringify(positions)); } catch { /* private mode */ }
    });

    const protoKey = protocolKind;
    const PROTO_COLOR: Record<string, string> = {
        rf: "var(--p-rf)", wifi: "var(--p-wifi)", matter: "var(--p-matter)", mqtt: "var(--p-mqtt)",
    };
    const PROTO_LABEL: Record<string, string> = { rf: "RF", wifi: "WIFI", matter: "MTR", mqtt: "MQTT" };

    function roomOf(s: Socket): string {
        return s.room?.trim() || "Unassigned";
    }

    type Node = { socket: Socket; room: string; on: boolean; proto: string; x: number; y: number };
    type Zone = { name: string; x: number; y: number; w: number; h: number; on: number; total: number };

    // Deterministic fallback layout: split the canvas into a grid of room
    // regions (busiest rooms first), then lay each room's nodes out in a
    // sub-grid inside its region with headroom at the top for the label.
    function defaultLayout(sockets: Socket[]): Record<string, Pos> {
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const groups = new Map<string, Socket[]>();
        for (const s of sockets) {
            const r = roomOf(s);
            if (!groups.has(r)) groups.set(r, []);
            groups.get(r)!.push(s);
        }
        const rooms = [...groups.entries()].sort((a, b) => b[1].length - a[1].length);
        const n = rooms.length || 1;
        const cols = n <= 1 ? 1 : n <= 4 ? 2 : 3;
        const rowsN = Math.ceil(n / cols);
        const regionW = VIEW_W / cols;
        const regionH = VIEW_H / rowsN;
        const pad = 30;
        const out: Record<string, Pos> = {};

        rooms.forEach(([, list], i) => {
            const col = i % cols, row = Math.floor(i / cols);
            const ox = col * regionW, oy = row * regionH;
            const innerW = regionW - pad * 2;
            const innerH = regionH - pad * 2 - 12;
            const m = list.length;
            const sc = Math.max(1, Math.ceil(Math.sqrt(m)));
            const sr = Math.max(1, Math.ceil(m / sc));
            list.forEach((s, j) => {
                const cx = j % sc, cy = Math.floor(j / sc);
                const x = ox + pad + (sc > 1 ? (cx * innerW) / (sc - 1) : innerW / 2);
                const y = oy + pad + 14 + (sr > 1 ? (cy * innerH) / (sr - 1) : innerH / 2);
                out[s.id] = { x, y };
            });
        });
        return out;
    }

    function clampX(x: number) { return Math.max(NODE_R + 2, Math.min(VIEW_W - NODE_R - 2, x)); }
    function clampY(y: number) { return Math.max(NODE_R + 14, Math.min(VIEW_H - NODE_R - 2, y)); }

    // Resolve each socket to a coordinate (saved overrides the default) and
    // bundle nodes with the room zones that frame them.
    const scene = $derived.by(() => {
        const sockets = v.sockets;
        const defaults = defaultLayout(sockets);
        const nodes: Node[] = sockets.map((socket) => {
            const p = positions[socket.id] ?? defaults[socket.id] ?? { x: VIEW_W / 2, y: VIEW_H / 2 };
            return {
                socket,
                room: roomOf(socket),
                on: socket.state,
                proto: protoKey(socket.protocol),
                x: clampX(p.x),
                y: clampY(p.y),
            };
        });

        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const byRoom = new Map<string, Node[]>();
        for (const node of nodes) {
            if (!byRoom.has(node.room)) byRoom.set(node.room, []);
            byRoom.get(node.room)!.push(node);
        }

        const zones: Zone[] = [...byRoom.entries()].map(([name, ns]) => {
            const xs = ns.map((n) => n.x), ys = ns.map((n) => n.y);
            const padZ = 24;
            const minX = Math.max(2, Math.min(...xs) - padZ);
            const maxX = Math.min(VIEW_W - 2, Math.max(...xs) + padZ);
            const minY = Math.max(2, Math.min(...ys) - padZ - 6); // extra top room for label
            const maxY = Math.min(VIEW_H - 2, Math.max(...ys) + padZ);
            return {
                name,
                x: minX, y: minY, w: maxX - minX, h: maxY - minY,
                on: ns.filter((n) => n.on).length,
                total: ns.length,
            };
        });

        return { nodes, zones };
    });

    const totalOn = $derived(v.sockets.filter((s) => s.state).length);
    const totalSockets = $derived(v.sockets.length);
    const roomCount = $derived(scene.zones.length);
    const activeNodes = $derived(scene.nodes.filter((n) => n.on));

    // ── Mode ─────────────────────────────────────────────────────────
    let editing = $state(false);
    function toggleEdit() { editing = !editing; }

    function resetLayout() {
        positions = {};
    }

    // ── Toggle (view mode) ───────────────────────────────────────────
    async function toggleNode(node: Node) {
        await socketAction(node.socket, "toggle");
    }

    // ── Drag (edit mode) ─────────────────────────────────────────────
    let svgEl = $state<SVGSVGElement>();
    let dragId = $state<string | null>(null);

    function clientToView(clientX: number, clientY: number): Pos {
        const rect = svgEl!.getBoundingClientRect();
        const x = ((clientX - rect.left) / rect.width) * VIEW_W;
        const y = ((clientY - rect.top) / rect.height) * VIEW_H;
        return { x: clampX(x), y: clampY(y) };
    }

    function onNodePointerDown(e: PointerEvent, node: Node) {
        if (!editing) return;
        e.preventDefault();
        e.stopPropagation();
        dragId = node.socket.id;
        (e.currentTarget as Element).setPointerCapture(e.pointerId);
    }
    function onNodePointerMove(e: PointerEvent, node: Node) {
        if (!editing || dragId !== node.socket.id || !svgEl) return;
        positions = { ...positions, [node.socket.id]: clientToView(e.clientX, e.clientY) };
    }
    function onNodePointerUp() { dragId = null; }

    function shortName(name: string): string {
        return name.length > 12 ? name.slice(0, 11) + "…" : name;
    }

</script>

<Topbar title="Floor plan" subtitle={`${totalOn} on · ${roomCount} room${roomCount === 1 ? "" : "s"}`}>
    {#snippet actions()}
        {#if editing}
            <button class="btn btn-ghost" onclick={resetLayout}>Reset</button>
        {/if}
        <button class="btn" class:btn-primary={editing} class:btn-ghost={!editing} onclick={toggleEdit}>
            {editing ? "Done" : "Edit"}
        </button>
    {/snippet}
</Topbar>

<div class="plan" class:editing>
    <!-- technical strip -->
    <div class="strip">
        <span class="mono tag">PLAN ⌁ N→</span>
        <span class="mono dims">{totalOn}/{totalSockets} ON</span>
    </div>

    {#if !v.loaded && totalSockets === 0}
        <div class="canvas skeleton" aria-hidden="true"></div>
    {:else if totalSockets === 0}
        <div class="empty">
            <p>No devices to map yet</p>
            <span>Add devices in <strong>Devices</strong> and they'll appear on the plan, grouped by room.</span>
        </div>
    {:else}
        <div class="canvas">
            <svg
                bind:this={svgEl}
                viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
                width="100%"
                role="group"
                aria-label="Floor plan of your devices"
            >
                <defs>
                    <radialGradient id="fp-glow" cx="50%" cy="50%" r="50%">
                        <stop offset="0%" stop-color="var(--on)" stop-opacity="0.85" />
                        <stop offset="50%" stop-color="var(--on)" stop-opacity="0.22" />
                        <stop offset="100%" stop-color="var(--on)" stop-opacity="0" />
                    </radialGradient>
                    <pattern id="fp-grid" width="20" height="20" patternUnits="userSpaceOnUse">
                        <path d="M 20 0 L 0 0 0 20" fill="none" stroke="var(--hairline)" stroke-width="0.5" />
                    </pattern>
                </defs>

                <rect x="0" y="0" width={VIEW_W} height={VIEW_H} fill="url(#fp-grid)" />

                <!-- glows behind everything -->
                {#each scene.nodes as node (node.socket.id + "-glow")}
                    {#if node.on}
                        <circle cx={node.x} cy={node.y} r="30" fill="url(#fp-glow)" />
                    {/if}
                {/each}

                <!-- room zones (data-driven; replaces decorative walls) -->
                {#each scene.zones as zone (zone.name)}
                    <g class="zone" class:lit={zone.on > 0}>
                        <rect
                            x={zone.x} y={zone.y} width={zone.w} height={zone.h}
                            rx="10"
                            fill="none"
                            stroke={zone.on > 0 ? "var(--on)" : "var(--text-dim)"}
                            stroke-opacity={zone.on > 0 ? 0.35 : 0.25}
                            stroke-width="1"
                            stroke-dasharray="3 4"
                        />
                        <text
                            x={zone.x + 8} y={zone.y + 13}
                            class="zone-label mono"
                            fill={zone.on > 0 ? "var(--on)" : "var(--text-dim)"}
                        >{zone.name.toUpperCase()} · {zone.on}/{zone.total}</text>
                    </g>
                {/each}

                <!-- light nodes -->
                {#each scene.nodes as node (node.socket.id)}
                    <g
                        class="node"
                        class:on={node.on}
                        class:dragging={dragId === node.socket.id}
                        role="button"
                        tabindex="0"
                        aria-label={`${node.socket.name}, ${node.room}, ${node.on ? "on" : "off"}`}
                        style:cursor={editing ? "grab" : "pointer"}
                        onclick={() => { if (!editing) toggleNode(node); }}
                        onkeydown={(e) => { if (!editing && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); toggleNode(node); } }}
                        onpointerdown={(e) => onNodePointerDown(e, node)}
                        onpointermove={(e) => onNodePointerMove(e, node)}
                        onpointerup={onNodePointerUp}
                        onpointercancel={onNodePointerUp}
                    >
                        <circle
                            cx={node.x} cy={node.y} r={NODE_R}
                            fill="none"
                            stroke={node.on ? "var(--on)" : "var(--text-dim)"}
                            stroke-opacity={node.on ? 1 : 0.5}
                            stroke-width={node.on ? 1.5 : 1}
                        />
                        <circle
                            cx={node.x} cy={node.y} r={node.on ? 4 : 2}
                            fill={node.on ? "var(--on)" : "var(--text-dim)"}
                        />
                        {#if node.on || editing}
                            <text
                                x={node.x + NODE_R + 4} y={node.y + 3}
                                class="node-label mono"
                                fill={node.on ? "var(--on)" : "var(--text-dim)"}
                            >{shortName(node.socket.name)}</text>
                        {/if}
                    </g>
                {/each}

                <!-- north compass -->
                <g transform={`translate(${VIEW_W - 26} ${VIEW_H - 28})`} aria-hidden="true">
                    <circle cx="0" cy="0" r="11" fill="none" stroke="var(--on)" stroke-opacity="0.3" />
                    <path d="M 0 -7 L 3 0 L 0 7 L -3 0 Z" fill="var(--on)" fill-opacity="0.6" />
                    <text x="0" y="-13" text-anchor="middle" class="compass mono" fill="var(--on)">N</text>
                </g>
            </svg>

            {#if editing}
                <p class="hint mono">Drag any light to reposition it · changes save automatically</p>
            {/if}
        </div>

        <!-- active strip -->
        {#if activeNodes.length > 0}
            <div class="active">
                <div class="active-head mono">ACTIVE ⌁ {String(activeNodes.length).padStart(2, "0")}</div>
                <div class="active-row h-scroll">
                    {#each activeNodes as node (node.socket.id)}
                        <button
                            class="active-card"
                            onclick={() => toggleNode(node)}
                            in:fly={{ y: 8, duration: dur(200), easing: cubicOut }}
                        >
                            <span class="ac-proto mono" style:color={PROTO_COLOR[node.proto]}>{PROTO_LABEL[node.proto]}</span>
                            <span class="ac-name">{node.socket.name}</span>
                            <span class="ac-room mono">{node.room}</span>
                        </button>
                    {/each}
                </div>
            </div>
        {/if}
    {/if}

    <!-- Room management lives on its own first-class screen. -->
    <button class="rooms-link" onclick={() => route.go("rooms")}>
        <span class="rl-ico"><Icon name="couch" size={18} /></span>
        <span class="rl-text">
            <span class="rl-title">Rooms</span>
            <span class="rl-sub mono">
                {v.rooms.length === 0
                    ? "Set up rooms to organise your devices"
                    : `${v.rooms.length} room${v.rooms.length === 1 ? "" : "s"} · add, rename or remove`}
            </span>
        </span>
        <span class="rl-chev"><Icon name="chevronDown" size={18} /></span>
    </button>
</div>

<style>
    .plan {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        max-width: 560px;
        width: 100%;
    }

    .strip {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 0 2px;
    }
    .tag { color: var(--on); font-size: 10px; letter-spacing: 0.18em; }
    .dims { color: var(--text-dim); font-size: 10px; letter-spacing: 0.08em; }

    .canvas {
        background: var(--bg);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: 10px;
        overflow: hidden;
    }

    .canvas.skeleton {
        height: 360px;
        background: linear-gradient(90deg, var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: shimmer 1.5s linear infinite;
    }
    @media (prefers-reduced-motion: reduce) { .canvas.skeleton { animation: none; } }

    svg { display: block; touch-action: manipulation; }
    /* Only capture touch gestures while dragging nodes; otherwise let the
       page scroll normally when the plan is touched. */
    .editing svg { touch-action: none; }

    .zone-label { font-size: 7px; letter-spacing: 0.18em; }
    .node-label { font-size: 7px; letter-spacing: 0.04em; }
    .compass { font-size: 7px; }

    .node:focus-visible { outline: none; }
    .node:focus-visible circle:first-of-type {
        stroke: var(--on);
        stroke-opacity: 1;
    }
    .node.dragging { cursor: grabbing !important; }
    .editing .node circle { transition: none; }

    .hint {
        margin: 8px 2px 2px;
        color: var(--text-dim);
        font-size: 10px;
        letter-spacing: 0.04em;
        text-align: center;
    }

    .empty {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 6px;
        text-align: center;
        padding: var(--space-10) var(--space-4);
        background: var(--bg-elevated);
        border: 1px dashed var(--border-strong);
        border-radius: var(--r-lg);
        color: var(--text-faint);
    }
    .empty p { font-weight: 600; color: var(--text-mute); font-size: 15px; }
    .empty span { font-size: 13px; max-width: 320px; }

    /* active strip */
    .active { padding: 0 2px; }
    .active-head {
        color: var(--text-mute);
        font-size: 9px;
        letter-spacing: 0.18em;
        margin-bottom: 8px;
    }
    .active-row { padding-bottom: 4px; }
    .active-card {
        display: flex;
        flex-direction: column;
        gap: 2px;
        align-items: flex-start;
        min-width: 104px;
        padding: 8px 10px;
        border: 1px solid var(--on-soft);
        border-radius: var(--r-md);
        background: var(--card);
        cursor: pointer;
        text-align: left;
        touch-action: manipulation;
        transition: border-color var(--t-fast), background var(--t-fast), transform var(--t-fast);
    }
    .active-card:hover { border-color: var(--on); }
    .active-card:active { transform: scale(0.97); }
    .ac-proto { font-size: 9px; letter-spacing: 0.1em; }
    .ac-name {
        font-size: 12px; font-weight: 500; color: var(--text);
        white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
        max-width: 120px;
    }
    .ac-room { font-size: 9.5px; color: var(--text-mute); }

    @media (max-width: 600px) {
        .active-card { min-height: 44px; }
    }

    /* Link out to the first-class Rooms screen */
    .rooms-link {
        margin-top: var(--space-2);
        display: flex;
        align-items: center;
        gap: 12px;
        width: 100%;
        padding: 14px;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        cursor: pointer;
        text-align: left;
        touch-action: manipulation;
        transition: border-color var(--t-fast), background var(--t-fast), transform var(--t-fast);
    }
    @media (hover: hover) {
        .rooms-link:hover { border-color: var(--border-strong); background: var(--card-2); }
    }
    .rooms-link:active { transform: scale(0.99); }
    .rooms-link:focus-visible { box-shadow: var(--focus-ring); }

    .rl-ico {
        width: 40px; height: 40px;
        border-radius: 12px;
        background: var(--on-soft);
        color: var(--on);
        display: grid; place-items: center;
        flex-shrink: 0;
    }
    .rl-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
    .rl-title { font-size: 15px; font-weight: 600; color: var(--text); }
    .rl-sub {
        font-size: 11px;
        color: var(--text-mute);
        letter-spacing: 0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .rl-chev {
        flex-shrink: 0;
        color: var(--text-dim);
        display: grid;
        place-items: center;
        transform: rotate(-90deg);
    }

    @media (max-width: 600px) {
        .rooms-link { min-height: 64px; }
    }
</style>

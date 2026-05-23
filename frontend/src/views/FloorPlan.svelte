<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import Switch from "../components/Switch.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { openModal } from "../lib/modal.svelte";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";
    import { data, toasts } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { socketAction } from "../lib/utils";
    import { fly, fade, scale } from "svelte/transition";
    import { cubicOut, backOut } from "svelte/easing";
    import { dur, stagger, sheet } from "../lib/motion";
    import type { Socket } from "../lib/types";

    const v = $derived(data.value);

    type RoomCell = {
        name: string;
        sockets: Socket[];
        on: number;
        total: number;
        warmth: number;       // 0..1 drives the warm radial glow
        size: "small" | "wide" | "big";
        isDraft: boolean;     // draft = client-only, no sockets yet
    };

    function pickSize(total: number): RoomCell["size"] {
        if (total >= 5) return "big";
        if (total >= 3) return "wide";
        return "small";
    }

    // ── Draft rooms — created in the UI but not yet persisted server-side.
    // Stored in localStorage so they survive reloads. A draft becomes "real"
    // the moment a socket is assigned to it.
    const DRAFT_KEY = "floorplan.draftRooms";
    function loadDrafts(): string[] {
        try {
            const raw = localStorage.getItem(DRAFT_KEY);
            return raw ? JSON.parse(raw) : [];
        } catch { return []; }
    }
    let draftRooms = $state<string[]>(loadDrafts());
    $effect(() => {
        try { localStorage.setItem(DRAFT_KEY, JSON.stringify(draftRooms)); } catch {}
    });

    // ── Per-room emoji "identity" — cosmetic, client-only, keyed by name.
    // Gives every room a face without touching the backend (rooms are just
    // a string on each socket). Falls back to a smart guess from the name.
    const EMOJI_KEY = "floorplan.roomEmoji";
    function loadEmoji(): Record<string, string> {
        try {
            const raw = localStorage.getItem(EMOJI_KEY);
            return raw ? JSON.parse(raw) : {};
        } catch { return {}; }
    }
    let roomEmoji = $state<Record<string, string>>(loadEmoji());
    $effect(() => {
        try { localStorage.setItem(EMOJI_KEY, JSON.stringify(roomEmoji)); } catch {}
    });

    function guessEmoji(name: string): string {
        const n = name.toLowerCase();
        if (/living|lounge|family/.test(n)) return "🛋️";
        if (/kitchen|cook/.test(n)) return "🍳";
        if (/bed|master/.test(n)) return "🛏️";
        if (/bath|shower|toilet|wc|washroom/.test(n)) return "🛁";
        if (/office|study|work|desk/.test(n)) return "💻";
        if (/kid|child|nursery|play/.test(n)) return "🧸";
        if (/dining|dinner/.test(n)) return "🍽️";
        if (/garage|car/.test(n)) return "🚗";
        if (/garden|yard|patio|outdoor|balcon/.test(n)) return "🌿";
        if (/hall|entry|foyer|corridor|stair/.test(n)) return "🚪";
        if (/tv|media|cinema|theat/.test(n)) return "📺";
        if (/laundry|utility/.test(n)) return "🧺";
        if (/gym|fitness/.test(n)) return "🏋️";
        return "💡";
    }
    function emojiFor(name: string): string {
        return roomEmoji[name] || guessEmoji(name);
    }

    const SUGGESTIONS: { name: string; emoji: string }[] = [
        { name: "Living Room", emoji: "🛋️" },
        { name: "Kitchen", emoji: "🍳" },
        { name: "Bedroom", emoji: "🛏️" },
        { name: "Bathroom", emoji: "🛁" },
        { name: "Office", emoji: "💻" },
        { name: "Kids' Room", emoji: "🧸" },
        { name: "Dining", emoji: "🍽️" },
        { name: "Garage", emoji: "🚗" },
        { name: "Garden", emoji: "🌿" },
        { name: "Hallway", emoji: "🚪" },
    ];
    const EMOJI_CHOICES = [
        "💡","🛋️","🍳","🛏️","🛁","💻","🧸","🍽️","🚗",
        "🌿","🚪","📺","🧺","🪴","🎮","📚","🎵","🏋️",
    ];

    const realCells = $derived.by(() => {
        const map = new Map<string, Socket[]>();
        for (const s of v.sockets) {
            const room = s.room?.trim() || "";
            if (!room) continue;
            if (!map.has(room)) map.set(room, []);
            map.get(room)!.push(s);
        }
        const cells: RoomCell[] = [...map.entries()].map(([name, sockets]) => {
            const on = sockets.filter(s => s.state).length;
            return {
                name,
                sockets,
                on,
                total: sockets.length,
                warmth: sockets.length === 0 ? 0 : on / sockets.length,
                size: pickSize(sockets.length),
                isDraft: false,
            };
        });
        return cells.sort((a, b) => b.total - a.total);
    });

    // Merge in any drafts that don't yet collide with a real room name.
    const cells = $derived.by<RoomCell[]>(() => {
        const realNames = new Set(realCells.map(r => r.name));
        const drafts: RoomCell[] = draftRooms
            .filter(n => !realNames.has(n))
            .map(name => ({
                name, sockets: [], on: 0, total: 0, warmth: 0,
                size: "small", isDraft: true,
            }));
        return [...realCells, ...drafts];
    });

    const unassigned = $derived(v.sockets.filter(s => !(s.room?.trim())));

    const totalOn = $derived(v.sockets.filter(s => s.state).length);
    const totalSockets = $derived(v.sockets.length);

    // ── Selection / mode ─────────────────────────────────────────────
    let selectedRoom = $state<string | null>(null);
    let editing      = $state(false);
    const selectedCell = $derived(cells.find(c => c.name === selectedRoom) ?? null);

    // Snapshot kept alive while the room panel animates out.
    // selectedCell becomes null the instant selectedRoom is cleared, which
    // would blank every {selectedCell.*} binding mid-flight. _panelCellMemo
    // never resets to null (only forwards on non-null updates), so panelCell
    // still carries real content for the full 180 ms fly-out.
    let _panelCellMemo = $state<typeof selectedCell>(null);
    $effect(() => { if (selectedCell !== null) _panelCellMemo = selectedCell; });
    const panelCell = $derived(_panelCellMemo ?? selectedCell);

    function pickRoom(name: string) {
        // On desktop the grid stays clickable beside the docked panel, so a
        // room tap also bows out of the create flow.
        creating = false;
        selectedRoom = selectedRoom === name ? null : name;
    }

    // Toggle edit mode. Closing edit mode also clears any selection so the
    // user lands on a clean view.
    function toggleEdit() {
        editing = !editing;
        selectedRoom = null;
        creating = false;
    }

    // ── Control actions (normal mode) ────────────────────────────────
    // Optimistic flip + rollback, matching SocketCard — the room tile lights
    // up instantly instead of waiting for a full refetch.
    async function toggleSocket(s: Socket) {
        await socketAction(s, "toggle");
    }

    // Fire all the changing sockets in parallel with optimistic state, then
    // reconcile against the server response. A serial await-loop made a
    // multi-device room feel sluggish on mobile.
    async function roomAllOn(cell: RoomCell) {
        const targets = cell.sockets.filter(s => !s.state);
        await Promise.all(targets.map(s => socketAction(s, "on")));
    }
    async function roomAllOff(cell: RoomCell) {
        const targets = cell.sockets.filter(s => s.state);
        await Promise.all(targets.map(s => socketAction(s, "off")));
    }

    // ── Create-room flow ─────────────────────────────────────────────
    // A dedicated step: name it, pick a vibe, then create. Replaces the old
    // "auto-named draft + disguised rename field" that left people unsure
    // what to do.
    let creating = $state(false);
    let newRoomName = $state("");
    let newRoomEmoji = $state("");
    let createInput = $state<HTMLInputElement>();

    // Focus the name field when the create sheet opens — but not on touch,
    // where summoning the keyboard immediately would cover the sheet's
    // quick-pick options. Touch users tap the field when ready.
    $effect(() => {
        if (!creating || !createInput) return;
        if (window.matchMedia("(pointer: coarse)").matches) return;
        createInput.focus();
    });

    function startCreate() {
        if (!editing) editing = true;
        selectedRoom = null;
        newRoomName = "";
        newRoomEmoji = "";
        creating = true;
    }
    function cancelCreate() { creating = false; }
    function applySuggestion(s: { name: string; emoji: string }) {
        newRoomName = s.name;
        newRoomEmoji = s.emoji;
    }
    const createEmoji = $derived(newRoomEmoji || (newRoomName.trim() ? guessEmoji(newRoomName) : "💡"));

    function confirmCreate() {
        const name = newRoomName.trim();
        if (!name) {
            toasts.warn("Name your room", "Type a name or tap a quick pick.");
            createInput?.focus();
            return;
        }
        if (cells.some(c => c.name.toLowerCase() === name.toLowerCase())) {
            toasts.error("Name in use", `A room called "${name}" already exists.`);
            return;
        }
        draftRooms = [...draftRooms, name];
        roomEmoji = { ...roomEmoji, [name]: createEmoji };
        creating = false;
        selectedRoom = name;
    }

    // ── Edit actions ─────────────────────────────────────────────────
    let emojiPickerOpen = $state(false);
    // Reset the inline emoji picker whenever the selected room changes.
    $effect(() => { selectedRoom; emojiPickerOpen = false; });
    function setRoomEmoji(name: string, emoji: string) {
        roomEmoji = { ...roomEmoji, [name]: emoji };
        emojiPickerOpen = false;
    }

    // Which sheet (if any) is showing. The room panel and the create sheet
    // are mutually exclusive; on desktop one of them docks beside the grid.
    const roomPanelOpen = $derived(
        !!selectedCell && !creating && (editing || !selectedCell.isDraft),
    );
    const panelOpen = $derived(roomPanelOpen || creating);

    // Rename a room. For real rooms, bulk-update every socket that lives
    // there; for drafts, just update the draft entry.
    async function renameRoom(oldName: string, newNameRaw: string) {
        const newName = newNameRaw.trim();
        if (!newName || newName === oldName) return;

        // Reject if the new name collides with another room
        if (cells.some(c => c.name !== oldName && c.name.toLowerCase() === newName.toLowerCase())) {
            toasts.error("Name in use", `A room called "${newName}" already exists.`);
            return;
        }

        const cell = cells.find(c => c.name === oldName);
        if (!cell) return;

        // Carry the room's emoji over to its new name.
        if (roomEmoji[oldName]) {
            const { [oldName]: moved, ...rest } = roomEmoji;
            roomEmoji = { ...rest, [newName]: moved };
        }

        if (cell.isDraft) {
            draftRooms = draftRooms.map(n => n === oldName ? newName : n);
            selectedRoom = newName;
            return;
        }

        try {
            await Promise.all(cell.sockets.map(s =>
                api.updateSocket(s.id, { name: s.name, code: s.code, protocol: s.protocol, room: newName })
            ));
            selectedRoom = newName;
            await data.refresh();
            toasts.success(`Renamed to "${newName}"`);
        } catch (e) {
            toasts.error("Rename failed", (e as Error).message);
        }
    }

    async function deleteRoom(cell: RoomCell) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: `Delete "${cell.name}"?`,
            message: cell.isDraft
                ? "This empty room will be removed."
                : `${cell.total} device${cell.total === 1 ? "" : "s"} will be moved to Unassigned. The devices themselves stay.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;

        // Forget the room's emoji either way.
        if (roomEmoji[cell.name]) {
            const { [cell.name]: _gone, ...rest } = roomEmoji;
            roomEmoji = rest;
        }

        if (cell.isDraft) {
            draftRooms = draftRooms.filter(n => n !== cell.name);
            selectedRoom = null;
            return;
        }

        try {
            await Promise.all(cell.sockets.map(s =>
                api.updateSocket(s.id, { name: s.name, code: s.code, protocol: s.protocol, room: "" })
            ));
            selectedRoom = null;
            await data.refresh();
            toasts.success(`Removed "${cell.name}"`);
        } catch (e) {
            toasts.error("Delete failed", (e as Error).message);
        }
    }

    async function moveSocketToRoom(socket: Socket, targetRoom: string) {
        try {
            await api.updateSocket(socket.id, {
                name: socket.name, code: socket.code, protocol: socket.protocol, room: targetRoom,
            });
            // If this was a draft and now has its first socket, prune it
            // (the derivation will hide it automatically, but tidy storage).
            draftRooms = draftRooms.filter(n => n !== targetRoom);
            await data.refresh();
        } catch (e) {
            toasts.error("Move failed", (e as Error).message);
        }
    }

    async function unassignSocket(socket: Socket) {
        try {
            await api.updateSocket(socket.id, {
                name: socket.name, code: socket.code, protocol: socket.protocol, room: "",
            });
            await data.refresh();
        } catch (e) {
            toasts.error("Update failed", (e as Error).message);
        }
    }

    // For the "Add device" picker in edit mode: every socket not already
    // in the selected room. Show their current room as a hint.
    // Uses panelCell so the list stays populated while the close animation plays.
    const addable = $derived.by(() => {
        if (!panelCell) return [] as Socket[];
        return v.sockets.filter(s => (s.room?.trim() || "") !== panelCell.name);
    });

    let addPick = $state("");

    async function performAdd() {
        if (!panelCell || !addPick) return;
        const sock = v.sockets.find(s => s.id === addPick);
        if (!sock) return;
        await moveSocketToRoom(sock, panelCell.name);
        addPick = "";
    }

    // Quick "Move to…" from an unassigned row — picks a room and assigns
    // the socket to it, then resets the select.
    async function onOrphanMove(sock: Socket, e: Event) {
        const sel = e.currentTarget as HTMLSelectElement;
        const target = sel.value;
        if (!target) return;
        sel.value = "";
        await moveSocketToRoom(sock, target);
    }

    // Lock the page behind the bottom sheet on mobile so the floor plan
    // doesn't scroll under it. On desktop the panel docks inline beside the
    // grid, so the page stays scrollable.
    $effect(() => {
        if (!selectedRoom && !creating) return;
        if (typeof window === "undefined") return;
        if (!window.matchMedia("(max-width: 900px)").matches) return;
        // iOS Safari ignores overflow:hidden on body — use the position:fixed
        // ref-counted lock that actually stops background scroll.
        lockBodyScroll();
        return () => unlockBodyScroll();
    });

    // ── Drag-to-dismiss (mobile bottom sheet handle) ─────────────────
    let dragY = $state(0);
    let dragging = $state(false);
    let panelDismissing = $state(false);
    let dragStartY = 0;

    function onHandlePointerDown(e: PointerEvent) {
        if (panelDismissing) return;
        dragging = true;
        dragStartY = e.clientY;
        dragY = 0;
        (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
        e.preventDefault();
    }

    function onHandlePointerMove(e: PointerEvent) {
        if (!dragging) return;
        dragY = Math.max(0, e.clientY - dragStartY);
    }

    function onHandlePointerUp() {
        if (!dragging) return;
        dragging = false;
        if (dragY > 80) {
            panelDismissing = true;
            dragY = 600;
            setTimeout(() => {
                // The room panel and the create sheet are never open at the
                // same time, so one shared drag controller dismisses whichever
                // is showing.
                if (creating) creating = false;
                else selectedRoom = null;
                dragY = 0;
                panelDismissing = false;
            }, 220);
        } else {
            // Let Svelte flush the transition property change before resetting
            // dragY so the spring-back CSS transition actually fires.
            requestAnimationFrame(() => { dragY = 0; });
        }
    }

    function onHandlePointerCancel() {
        if (!dragging) return;
        dragging = false;
        requestAnimationFrame(() => { dragY = 0; });
    }
</script>

<svelte:window onkeydown={(e) => {
    if (e.key !== "Escape") return;
    if (creating) cancelCreate();
    else if (selectedRoom) selectedRoom = null;
}} />

<Topbar title="Floor plan" subtitle="Your home at a glance">
    {#snippet actions()}
        <button class="btn" class:btn-primary={editing} class:btn-ghost={!editing}
            onclick={toggleEdit}>
            {editing ? "Done" : "Edit"}
        </button>
    {/snippet}
</Topbar>

<!-- ── House pulse ──────────────────────────────────────────────── -->
<div class="pulse" data-active={totalOn > 0}>
    <div class="pulse-num">
        <span class="big">{totalOn}</span>
        <span class="of">/ {totalSockets}</span>
    </div>
    <div class="pulse-text">
        <div class="pulse-title">
            {totalOn === 0
                ? "House is asleep"
                : totalOn === 1 ? "1 device on"
                : `${totalOn} devices on`}
        </div>
        <div class="pulse-sub">
            {cells.length} room{cells.length === 1 ? "" : "s"}
            {#if unassigned.length > 0} · {unassigned.length} unassigned{/if}
        </div>
    </div>
</div>

<!-- ── Floor plan stage: grid on the left, docked panel on the right ─ -->
<div class="stage" class:has-panel={panelOpen}>
<div class="stage-grid">
{#if !v.loaded && cells.length === 0}
    <div class="house">
        <div class="rooms">
            {#each Array.from({ length: 4 }) as _, i (i)}
                <div class="room skeleton-room" aria-hidden="true"></div>
            {/each}
        </div>
    </div>
{:else if cells.length === 0 && !editing}
    <div class="empty">
        <div class="empty-emoji" aria-hidden="true">🏠</div>
        <p>Let's map your home</p>
        <span>Create a room for each space, then drop your devices in. Give each one a name and a vibe.</span>
        <button class="btn btn-primary" onclick={startCreate}>Create your first room</button>
    </div>
{:else}
    <div class="house" class:editing>
        <div class="rooms">
            {#each cells as cell, i (cell.name)}
                <button
                    class="room"
                    data-size={cell.size}
                    class:selected={selectedRoom === cell.name}
                    class:lit={cell.on > 0}
                    class:draft={cell.isDraft}
                    style="--warmth: {cell.warmth}"
                    onclick={() => pickRoom(cell.name)}
                    aria-expanded={selectedRoom === cell.name}
                    aria-label={`${cell.name}, ${cell.on} of ${cell.total} on`}
                    in:fly={{ y: 12, duration: dur(260), delay: stagger(i, 40), easing: cubicOut }}
                >
                    <span class="room-watermark" aria-hidden="true">{emojiFor(cell.name)}</span>
                    <div class="room-head">
                        <span class="room-title">
                            <span class="room-emoji" aria-hidden="true">{emojiFor(cell.name)}</span>
                            <span class="room-name">{cell.name}</span>
                        </span>
                        {#if editing}
                            <span class="edit-badge" aria-hidden="true">
                                <Icon name="edit" size={11} />
                            </span>
                        {:else if cell.isDraft}
                            <span class="room-count empty-tag">empty</span>
                        {:else}
                            <span class="room-count" data-on={cell.on > 0}>
                                {cell.on}<span class="slash">/</span>{cell.total}
                            </span>
                        {/if}
                    </div>
                    <div class="dots" aria-hidden="true">
                        {#each cell.sockets as s (s.id)}
                            <span class="dot" data-on={s.state}></span>
                        {/each}
                    </div>
                </button>
            {/each}

            {#if editing}
                <button class="room add-tile" onclick={startCreate}
                    aria-label="Create a new room"
                    in:fly={{ y: 12, duration: dur(220), easing: cubicOut }}>
                    <Icon name="plus" size={20} />
                    <span>New room</span>
                </button>
            {/if}
        </div>
    </div>
{/if}
</div><!-- /.stage-grid -->

<!-- ── Selected room panel (docked beside grid on desktop, sheet on mobile) ─ -->
{#if roomPanelOpen}
    <div class="sheet-root"
        role="presentation"
        onclick={(e) => { if (e.target === e.currentTarget) selectedRoom = null; }}
        in:fade={{ duration: dur(140) }}
        out:fade={{ duration: dur(120) }}>
        <div class="panel" class:edit={editing}
            role="dialog"
            aria-label={panelCell?.name}
            aria-modal="true"
            style:transform={dragY > 0 ? `translateY(${dragY}px)` : ''}
            style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
            style:transition={dragging ? 'none' : panelDismissing ? 'transform 0.22s ease-in, opacity 0.22s ease-in' : 'transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)'}
            in:sheet={{ breakpoint: 900, duration: 320 }}
            out:sheet={{ instant: panelDismissing, breakpoint: 900, duration: 240 }}>

            <div class="sheet-handle" aria-hidden="true"
                onpointerdown={onHandlePointerDown}
                onpointermove={onHandlePointerMove}
                onpointerup={onHandlePointerUp}
                onpointercancel={onHandlePointerCancel}></div>

            <div class="panel-head">
                {#if editing}
                    <div class="ph-left">
                        <div class="name-row">
                            <button class="room-emoji-btn"
                                class:open={emojiPickerOpen}
                                onclick={() => emojiPickerOpen = !emojiPickerOpen}
                                aria-label="Change room icon"
                                aria-expanded={emojiPickerOpen}>{emojiFor(panelCell?.name ?? "")}</button>
                            <input class="rename-input"
                                type="text"
                                value={panelCell?.name ?? ""}
                                aria-label="Room name"
                                onblur={(e) => panelCell && renameRoom(panelCell.name, (e.target as HTMLInputElement).value)}
                                onkeydown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }}
                            />
                        </div>
                        {#if emojiPickerOpen}
                            <div class="emoji-grid inline" transition:fly={{ y: -6, duration: dur(140), easing: cubicOut }}>
                                {#each EMOJI_CHOICES as e (e)}
                                    <button type="button" class="emoji-cell"
                                        class:active={emojiFor(panelCell?.name ?? "") === e}
                                        onclick={() => panelCell && setRoomEmoji(panelCell.name, e)}
                                        aria-label={`Use ${e}`}>{e}</button>
                                {/each}
                            </div>
                        {/if}
                        <span class="ph-sub">
                            {panelCell?.isDraft
                                ? "Empty room — add a device to save it"
                                : `${panelCell?.total ?? 0} device${(panelCell?.total ?? 0) === 1 ? "" : "s"}`}
                        </span>
                    </div>
                {:else}
                    <div class="ph-left">
                        <span class="ph-name">
                            <span class="ph-emoji" aria-hidden="true">{emojiFor(panelCell?.name ?? "")}</span>
                            {panelCell?.name}
                        </span>
                        <span class="ph-sub">{panelCell?.on ?? 0} of {panelCell?.total ?? 0} on</span>
                    </div>
                {/if}
                <button class="icon-btn" onclick={() => selectedRoom = null} aria-label="Close">
                    <Icon name="close" size={16} />
                </button>
            </div>

            <div class="panel-body">
                {#if editing}
                    {#if (panelCell?.sockets.length ?? 0) > 0}
                        <ul class="socket-list">
                            {#each panelCell?.sockets ?? [] as s (s.id)}
                                <li class="socket-row edit">
                                    <span class="socket-name">{s.name}</span>
                                    <button class="link-btn danger"
                                        onclick={() => unassignSocket(s)}
                                        aria-label={`Remove ${s.name} from ${panelCell?.name ?? ""}`}>
                                        Remove
                                    </button>
                                </li>
                            {/each}
                        </ul>
                    {/if}

                    {#if addable.length > 0}
                        <div class="add-row">
                            <select bind:value={addPick} aria-label="Add device">
                                <option value="">Add device…</option>
                                {#each addable as s (s.id)}
                                    <option value={s.id}>
                                        {s.name}{s.room ? ` (${s.room})` : " (unassigned)"}
                                    </option>
                                {/each}
                            </select>
                            <button class="btn btn-primary btn-xs" disabled={!addPick} onclick={performAdd}>
                                Add
                            </button>
                        </div>
                    {:else if v.sockets.length === 0}
                        <div class="hint">No devices yet — add some in <strong>Devices</strong>.</div>
                    {/if}
                {:else}
                    <ul class="socket-list">
                        {#each panelCell?.sockets ?? [] as s (s.id)}
                            <li class="socket-row">
                                <span class="socket-name">{s.name}</span>
                                <Switch
                                    checked={s.state}
                                    ariaLabel={`Toggle ${s.name}`}
                                    onChange={() => toggleSocket(s)}
                                />
                            </li>
                        {/each}
                    </ul>
                {/if}
            </div>

            <div class="panel-foot">
                {#if editing}
                    <button class="btn btn-danger btn-xs" onclick={() => panelCell && deleteRoom(panelCell)}>
                        Delete room
                    </button>
                {:else}
                    <button class="btn btn-success btn-xs"
                        disabled={panelCell?.on === panelCell?.total}
                        onclick={() => { const c = panelCell; selectedRoom = null; c && roomAllOn(c); }}>All on</button>
                    <button class="btn btn-danger btn-xs"
                        disabled={panelCell?.on === 0}
                        onclick={() => { const c = panelCell; selectedRoom = null; c && roomAllOff(c); }}>All off</button>
                {/if}
            </div>
        </div>
    </div>
{/if}

<!-- ── Create-room sheet ────────────────────────────────────────── -->
{#if creating}
    <div class="sheet-root"
        role="presentation"
        onclick={(e) => { if (e.target === e.currentTarget) cancelCreate(); }}
        in:fade={{ duration: dur(140) }}
        out:fade={{ duration: dur(120) }}>
        <div class="panel create-panel"
            role="dialog"
            aria-label="New room"
            aria-modal="true"
            style:transform={dragY > 0 ? `translateY(${dragY}px)` : ''}
            style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
            style:transition={dragging ? 'none' : panelDismissing ? 'transform 0.22s ease-in, opacity 0.22s ease-in' : 'transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)'}
            in:sheet={{ breakpoint: 900, duration: 320 }}
            out:sheet={{ instant: panelDismissing, breakpoint: 900, duration: 240 }}>

            <div class="sheet-handle" aria-hidden="true"
                onpointerdown={onHandlePointerDown}
                onpointermove={onHandlePointerMove}
                onpointerup={onHandlePointerUp}
                onpointercancel={onHandlePointerCancel}></div>

            <div class="panel-head">
                <div class="ph-left">
                    <span class="ph-name">New room</span>
                    <span class="ph-sub">Name it and give it a vibe</span>
                </div>
                <button class="icon-btn" onclick={cancelCreate} aria-label="Close">
                    <Icon name="close" size={16} />
                </button>
            </div>

            <div class="panel-body create-body">
                <div class="create-preview">
                    {#key createEmoji}
                        <span class="create-emoji" aria-hidden="true"
                            in:scale={{ duration: dur(260), start: 0.5, easing: backOut }}>{createEmoji}</span>
                    {/key}
                    <input class="create-name"
                        type="text"
                        bind:this={createInput}
                        bind:value={newRoomName}
                        placeholder="Room name"
                        aria-label="Room name"
                        onkeydown={(e) => { if (e.key === "Enter") confirmCreate(); }} />
                </div>

                <div class="create-label">Quick picks</div>
                <div class="suggest-row">
                    {#each SUGGESTIONS as s (s.name)}
                        <button type="button" class="suggest-chip"
                            class:active={newRoomName.trim().toLowerCase() === s.name.toLowerCase()}
                            onclick={() => applySuggestion(s)}>
                            <span aria-hidden="true">{s.emoji}</span>
                            {s.name}
                        </button>
                    {/each}
                </div>

                <div class="create-label">Icon</div>
                <div class="emoji-grid">
                    {#each EMOJI_CHOICES as e (e)}
                        <button type="button" class="emoji-cell"
                            class:active={createEmoji === e}
                            onclick={() => newRoomEmoji = e}
                            aria-label={`Use ${e}`}>{e}</button>
                    {/each}
                </div>
            </div>

            <div class="panel-foot">
                <button class="btn btn-ghost btn-xs" onclick={cancelCreate}>Cancel</button>
                <button class="btn btn-primary btn-xs create-go" onclick={confirmCreate}>
                    Create room
                </button>
            </div>
        </div>
    </div>
{/if}
</div><!-- /.stage -->

<!-- ── Unassigned sockets ───────────────────────────────────────── -->
{#if unassigned.length > 0}
    <section class="orphans">
        <div class="orphans-head">
            <span class="orphans-title">Unassigned</span>
            <span class="orphans-sub">
                {unassigned.length} device{unassigned.length === 1 ? "" : "s"} not on the plan
            </span>
        </div>
        <ul class="socket-list">
            {#each unassigned as s (s.id)}
                <li class="socket-row orphan">
                    <span class="socket-name">{s.name}</span>
                    <div class="orphan-actions">
                        {#if cells.length > 0}
                            <select class="move-select"
                                value=""
                                onchange={(e) => onOrphanMove(s, e)}
                                aria-label={`Move ${s.name} to a room`}>
                                <option value="">Move to…</option>
                                {#each cells as c (c.name)}
                                    <option value={c.name}>{c.name}</option>
                                {/each}
                            </select>
                        {/if}
                        <Switch
                            checked={s.state}
                            ariaLabel={`Toggle ${s.name}`}
                            onChange={() => toggleSocket(s)}
                        />
                    </div>
                </li>
            {/each}
        </ul>
    </section>
{/if}

<style>
    /* ── House pulse ──────────────────────────────────── */
    .pulse {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        padding: var(--space-4) var(--space-5);
        border-radius: var(--radius-lg);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        transition: box-shadow var(--t-med), border-color var(--t-med);
    }
    .pulse[data-active="true"] {
        border-color: var(--on);
        box-shadow: 0 0 0 1px var(--on-soft),
                    0 12px 32px -16px var(--on-glow);
    }
    .pulse-num {
        display: flex; align-items: baseline; gap: 4px;
        font-family: var(--font-mono);
        font-variant-numeric: tabular-nums;
    }
    .pulse-num .big {
        font-size: 2.5rem; font-weight: 700; line-height: 1;
        letter-spacing: -0.03em; color: var(--text);
    }
    .pulse[data-active="true"] .pulse-num .big { color: var(--on); }
    .pulse-num .of { font-size: 1rem; color: var(--text-faint); font-weight: 500; }
    .pulse-title { font-weight: 700; font-size: 1rem; letter-spacing: -0.01em; }
    .pulse-sub { color: var(--text-muted); font-size: 12px; margin-top: 2px; }

    /* ── House ────────────────────────────────────────── */
    .house {
        padding: 6px;
        border-radius: var(--radius-lg);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        box-shadow: inset 0 0 0 1px var(--surface);
        transition: border-color var(--t-fast), box-shadow var(--t-med);
    }
    .house.editing {
        border-color: var(--primary);
        box-shadow: inset 0 0 0 1px var(--surface),
                    0 0 0 2px var(--primary-glow);
    }
    .rooms {
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        grid-auto-rows: 92px;
        grid-auto-flow: dense;
        gap: 6px;
    }
    @media (max-width: 600px) {
        .rooms { grid-template-columns: repeat(4, 1fr); grid-auto-rows: 80px; gap: 4px; }
    }

    .room {
        all: unset;
        cursor: pointer;
        position: relative;
        display: flex;
        flex-direction: column;
        padding: 10px;
        border-radius: 8px;
        background:
            radial-gradient(120% 80% at 50% 60%,
                rgba(245, 189, 110, calc(var(--warmth, 0) * 0.36)) 0%,
                rgba(245, 189, 110, calc(var(--warmth, 0) * 0.08)) 50%,
                transparent 75%),
            var(--surface);
        border: 1px solid var(--border);
        transition: transform var(--t-fast), border-color var(--t-fast),
                    box-shadow var(--t-med), background var(--t-med);
        overflow: hidden;
        grid-column: span 2;
    }
    .room[data-size="small"] { grid-column: span 2; grid-row: span 1; }
    .room[data-size="wide"]  { grid-column: span 2; grid-row: span 2; }
    .room[data-size="big"]   { grid-column: span 4; grid-row: span 2; }

    /* Cold-load placeholder tiles while the first fetch resolves. */
    .skeleton-room {
        min-height: 96px;
        cursor: default;
        background: linear-gradient(90deg,
            var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: fp-shimmer 1.5s linear infinite;
    }
    @keyframes fp-shimmer {
        0% { background-position: 200% 0; }
        100% { background-position: -200% 0; }
    }
    @media (prefers-reduced-motion: reduce) {
        .skeleton-room { animation: none; }
    }
    @media (min-width: 600px) {
        .room[data-size="small"] { grid-column: span 1; grid-row: span 1; }
        .room[data-size="wide"]  { grid-column: span 2; grid-row: span 1; }
        .room[data-size="big"]   { grid-column: span 2; grid-row: span 2; }
    }
    .room.lit {
        border-color: rgba(245, 189, 110, calc(var(--warmth, 0) * 0.5 + 0.2));
    }
    .room.draft {
        border-style: dashed;
        background:
            repeating-linear-gradient(45deg,
                transparent 0 6px,
                rgba(255, 255, 255, 0.02) 6px 12px),
            var(--surface);
    }
    @media (hover: hover) {
        .room:hover { transform: translateY(-1px); border-color: var(--border-strong); }
    }
    .room:focus-visible { box-shadow: var(--focus-ring); }
    .room.selected {
        border-color: var(--primary);
        box-shadow: 0 0 0 3px var(--primary-glow);
    }

    .room-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-2); min-width: 0;
    }
    .room-name {
        font-weight: 600; font-size: 13px; letter-spacing: -0.01em;
        white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
        color: var(--text);
    }
    .room-count {
        font-size: 11px;
        font-family: var(--font-mono);
        font-variant-numeric: tabular-nums;
        color: var(--text-faint); flex-shrink: 0; font-weight: 500;
    }
    .room-count[data-on="true"] { color: var(--on); }
    .room-count .slash { opacity: 0.5; margin: 0 1px; }
    .empty-tag {
        text-transform: uppercase; letter-spacing: 0.05em;
        font-size: 9px; padding: 2px 6px;
        border-radius: 999px; background: var(--surface-hover);
        color: var(--text-faint);
    }

    .dots {
        margin-top: auto;
        display: flex; flex-wrap: wrap; gap: 5px;
        align-items: flex-end;
        padding-top: var(--space-2);
    }
    .dot {
        width: 9px; height: 9px; border-radius: 50%;
        background: transparent;
        border: 1.5px solid var(--border-strong);
        transition: background var(--t-fast), border-color var(--t-fast),
                    box-shadow var(--t-fast);
    }
    .dot[data-on="true"] {
        background: var(--on); border-color: var(--on);
        box-shadow: 0 0 8px var(--on-glow),
                    0 0 0 2px var(--on-soft);
    }

    /* Pencil badge — replaces the on/off count in edit mode */
    .edit-badge {
        width: 18px; height: 18px;
        border-radius: 4px;
        background: var(--primary-soft);
        color: var(--primary);
        display: grid; place-items: center;
        flex-shrink: 0;
        pointer-events: none;
    }
    .room.selected .edit-badge { background: var(--primary); color: #fff; }

    /* + Add room tile */
    .add-tile {
        grid-column: span 2;
        grid-row: span 1;
        display: flex !important;
        align-items: center; justify-content: center;
        gap: 6px;
        background: transparent;
        border: 1.5px dashed var(--border-strong);
        color: var(--text-muted);
        font-size: 13px; font-weight: 600;
    }
    @media (min-width: 600px) {
        .add-tile { grid-column: span 1; }
    }
    .add-tile:hover { color: var(--primary); border-color: var(--primary); }

    /* ── Empty state ──────────────────────────────────── */
    .empty {
        display: flex; flex-direction: column; align-items: center;
        gap: var(--space-2);
        padding: var(--space-10) var(--space-4);
        border-radius: var(--radius-lg);
        background: var(--bg-elevated);
        border: 1px dashed var(--border);
        color: var(--text-faint); text-align: center;
    }
    .empty p { margin: 0; font-weight: 600; color: var(--text-muted); font-size: 15px; }
    .empty span { font-size: 13px; max-width: 280px; }

    /* ── Stage: grid + docked panel ───────────────────── */
    /* Desktop ≥901px: two columns — grid flexes, panel docks on the right.
       Mobile ≤900px: single column; the panel escapes as a bottom sheet. */
    .stage { display: flex; min-width: 0; }
    .stage-grid { flex: 1; min-width: 0; }
    @media (min-width: 901px) {
        .stage { gap: var(--space-4); align-items: flex-start; }
    }

    /* ── Sheet wrapper ────────────────────────────────── */
    /* Desktop: a transparent pass-through so the panel flows into the stage
       as a column. Mobile: full-viewport backdrop, panel docked to bottom. */
    .sheet-root { display: contents; }

    @media (max-width: 900px) {
        .sheet-root {
            display: flex;
            position: fixed;
            inset: 0;
            align-items: flex-end;
            justify-content: center;
            background: rgba(8, 11, 22, 0.45);
            backdrop-filter: blur(3px);
            -webkit-backdrop-filter: blur(3px);
            z-index: 120;
            overscroll-behavior: contain;
        }
        :global([data-theme="light"]) .sheet-root {
            background: rgba(20, 24, 38, 0.30);
        }
    }

    /* Visual drag handle — sheet only */
    .sheet-handle { display: none; }
    @media (max-width: 900px) {
        .sheet-handle {
            display: block;
            width: 36px;
            height: 5px;
            border-radius: 999px;
            background: var(--border-strong);
            margin: 8px auto 4px;
            flex-shrink: 0;
            touch-action: none;
            cursor: grab;
            /* Larger touch target without changing visual size */
            padding: 12px 32px;
            margin-inline: auto;
            box-sizing: content-box;
        }
        .sheet-handle:active { cursor: grabbing; }
    }

    /* ── Panel (control + edit) ───────────────────────── */
    .panel {
        /* Desktop: a docked side column that sticks as you scroll the grid. */
        width: 360px;
        flex-shrink: 0;
        position: sticky;
        top: var(--space-6);
        max-height: calc(100vh - var(--space-6) * 2);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        overflow: hidden;
        box-shadow: var(--shadow-md), inset 4px 0 0 var(--primary);
        display: flex;
        flex-direction: column;
        min-height: 0;
    }
    .panel.edit { box-shadow: var(--shadow-md), inset 4px 0 0 var(--warn); }

    /* Mobile: dock as a sheet, rounded top corners, frosted look */
    @media (max-width: 900px) {
        .panel {
            position: static;
            width: 100%;
            max-width: none;
            max-height: 85vh;
            border-radius: var(--radius-xl) var(--radius-xl) 0 0;
            background: var(--bg-bar);
            backdrop-filter: saturate(180%) blur(24px);
            -webkit-backdrop-filter: saturate(180%) blur(24px);
            border: none;
            border-top: 0.5px solid var(--separator);
            box-shadow: 0 -2px 24px rgba(0, 0, 0, 0.25);
            /* Push content above the bottom tab bar's safe area */
            padding-bottom: env(safe-area-inset-bottom);
        }
        .panel.edit { box-shadow: 0 -2px 24px rgba(0, 0, 0, 0.25); }
    }

    .panel-head {
        display: flex; align-items: center; justify-content: space-between;
        padding: var(--space-4) var(--space-5) var(--space-3);
        border-bottom: 1px solid var(--border);
        flex-shrink: 0;
    }
    .panel-body {
        flex: 1 1 auto;
        min-height: 0;
        overflow-y: auto;
        -webkit-overflow-scrolling: touch;
        display: flex;
        flex-direction: column;
    }
    .ph-left { display: flex; flex-direction: column; min-width: 0; flex: 1; }
    .ph-name { font-weight: 700; letter-spacing: -0.01em; }
    .ph-sub { color: var(--text-faint); font-size: 12px; margin-top: 2px; }

    .rename-input {
        all: unset;
        font-weight: 700; letter-spacing: -0.01em;
        font-size: 1rem;
        padding: 4px 8px;
        margin-left: -8px;
        border-radius: var(--radius-sm);
        background: transparent;
        border: 1px solid transparent;
        width: 100%;
        box-sizing: border-box;
    }
    .rename-input:hover { background: var(--surface); }
    .rename-input:focus { background: var(--surface); border-color: var(--primary); }

    .socket-list {
        list-style: none; margin: 0;
        padding: var(--space-2) var(--space-3);
        display: flex; flex-direction: column; gap: 2px;
    }
    /* When the list lives directly inside the orphans card it scrolls itself.
       Inside the panel the parent `.panel-body` handles scroll, so don't
       constrain the list here. */
    .orphans .socket-list {
        max-height: 50vh; overflow-y: auto;
        -webkit-overflow-scrolling: touch;
    }
    .socket-row {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
        padding: 10px 8px;
        border-radius: var(--radius-sm);
        font-size: 14px;
    }
    .socket-row:hover { background: var(--surface); }
    .socket-row.edit { background: var(--surface); }
    .socket-name {
        flex: 1; min-width: 0; font-weight: 500;
        white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    }
    .panel-foot {
        display: flex; gap: var(--space-2);
        padding: var(--space-3) var(--space-4);
        border-top: 1px solid var(--border);
        flex-shrink: 0;
    }

    .add-row {
        display: flex; gap: var(--space-2);
        padding: 0 var(--space-3) var(--space-3);
        flex-wrap: wrap;
    }
    .add-row select {
        flex: 1 1 160px; min-width: 0;
        padding: 10px 32px 10px 12px;
        border-radius: var(--radius-sm);
        background-color: var(--surface);
        border: 1px solid var(--border);
        color: var(--text);
        font: inherit;
        font-size: 14px;
        min-height: 40px;
    }

    .hint {
        padding: var(--space-2) var(--space-5);
        color: var(--text-faint); font-size: 13px;
    }

    .link-btn {
        background: none; border: none;
        padding: 8px 12px;
        font: inherit; font-size: 13px; font-weight: 600;
        cursor: pointer; color: var(--text-muted);
        border-radius: var(--radius-sm);
        min-height: 36px;
        flex-shrink: 0;
    }
    .link-btn:hover { background: var(--surface-hover); color: var(--text); }
    .link-btn.danger:hover { color: var(--danger); }

    /* ── Unassigned ───────────────────────────────────── */
    .orphans {
        background: var(--bg-elevated);
        border: 1px dashed var(--border);
        border-radius: var(--radius-lg);
        overflow: hidden;
    }
    .orphans-head {
        padding: var(--space-3) var(--space-5);
        border-bottom: 1px solid var(--border);
        display: flex; flex-direction: column;
    }
    .orphans-title {
        font-weight: 600; font-size: 13px;
        text-transform: uppercase; letter-spacing: 0.06em;
        color: var(--text-muted);
    }
    .orphans-sub { color: var(--text-faint); font-size: 12px; margin-top: 2px; }

    /* Unassigned row: name on the left, [Move-to] + [Switch] on the right. */
    .socket-row.orphan { align-items: center; }
    .orphan-actions {
        display: flex; align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .move-select {
        max-width: 140px;
        padding: 6px 32px 6px 10px;
        border-radius: var(--radius-md);
        background-color: var(--surface);
        border: 1px solid var(--border);
        color: var(--text);
        font: inherit;
        font-size: 13px;
        min-height: 36px;
    }
    /* On very narrow screens, drop the actions onto their own line so the
       name doesn't get crushed. */
    @media (max-width: 380px) {
        .socket-row.orphan {
            flex-wrap: wrap;
        }
        .orphan-actions { width: 100%; justify-content: flex-end; }
    }

    :global(.btn-xs) { padding: 6px 12px; font-size: 12px; min-height: 36px; }

    /* Bump the close button to a 44 px tap target on touch screens —
       the global 32 px icon-btn is below Apple HIG / Material guidance. */
    @media (max-width: 600px) {
        .panel-head :global(.icon-btn) { width: 44px; height: 44px; }
        /* 16px prevents iOS zoom-on-focus; 44px meets the touch minimum. */
        .add-row select,
        .move-select { font-size: 16px; min-height: 44px; }
        .rename-input { font-size: 16px; }
        .move-select { max-width: 160px; }
        :global(.btn-xs) { min-height: 44px; padding: 8px 14px; }
        .link-btn { min-height: 44px; }
    }

    /* Socket rows inside the panel — comfortable minimum height on mobile */
    .socket-row { min-height: 44px; }

    /* ── Room emoji identity ──────────────────────────── */
    .room-title {
        display: flex; align-items: center; gap: 5px;
        min-width: 0; flex: 1;
    }
    .room-emoji { font-size: 15px; line-height: 1; flex-shrink: 0; }
    .ph-emoji { margin-right: 4px; }

    /* ── Empty state CTA ──────────────────────────────── */
    .empty-emoji { font-size: 44px; line-height: 1; }
    .empty .btn-primary { margin-top: var(--space-3); }

    /* ── Create-room sheet ────────────────────────────── */
    .create-body { gap: var(--space-4); padding: var(--space-4) var(--space-5) var(--space-5); }

    /* Big live preview: emoji + the name you're typing */
    .create-preview {
        display: flex; align-items: center; gap: var(--space-3);
        padding: var(--space-3);
        border-radius: var(--radius-md);
        background: var(--surface);
        border: 1px solid var(--border);
    }
    .create-emoji {
        font-size: 34px; line-height: 1; flex-shrink: 0;
        width: 56px; height: 56px;
        display: grid; place-items: center;
        border-radius: var(--radius-md);
        background: var(--bg-elevated);
        box-shadow: var(--shadow-sm);
    }
    .create-name {
        all: unset;
        flex: 1; min-width: 0;
        font-size: 1.25rem; font-weight: 700; letter-spacing: -0.01em;
        padding: 6px 4px;
        border-bottom: 2px solid transparent;
    }
    .create-name::placeholder { color: var(--text-faint); font-weight: 600; }
    .create-name:focus { border-bottom-color: var(--primary); }

    .create-label {
        font-size: 11px; font-weight: 700;
        text-transform: uppercase; letter-spacing: 0.06em;
        color: var(--text-muted);
        margin-bottom: calc(var(--space-2) * -1);
    }

    .suggest-row { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .suggest-chip {
        display: inline-flex; align-items: center; gap: 6px;
        padding: 8px 12px;
        border-radius: 999px;
        border: 1px solid var(--border);
        background: var(--surface);
        color: var(--text);
        font: inherit; font-size: 13px; font-weight: 600;
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast),
                    transform var(--t-fast);
    }
    .suggest-chip:hover { background: var(--surface-hover); border-color: var(--border-strong); }
    .suggest-chip:active { transform: scale(0.96); }
    .suggest-chip.active {
        background: var(--primary-soft);
        border-color: var(--primary);
        color: var(--primary);
    }

    .emoji-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(44px, 1fr));
        gap: var(--space-2);
    }
    .emoji-grid.inline {
        margin-top: var(--space-2);
        grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
    }
    .emoji-cell {
        aspect-ratio: 1;
        display: grid; place-items: center;
        font-size: 22px; line-height: 1;
        border-radius: var(--radius-md);
        border: 1px solid var(--border);
        background: var(--surface);
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast),
                    transform var(--t-fast);
    }
    .emoji-cell:hover { background: var(--surface-hover); }
    .emoji-cell:active { transform: scale(0.92); }
    .emoji-cell.active {
        border-color: var(--primary);
        box-shadow: 0 0 0 2px var(--primary-glow);
        background: var(--primary-soft);
    }

    .create-go { flex: 1; }

    /* Editable emoji button beside the rename field */
    .name-row { display: flex; align-items: center; gap: var(--space-2); }
    .room-emoji-btn {
        flex-shrink: 0;
        width: 40px; height: 40px;
        display: grid; place-items: center;
        font-size: 22px; line-height: 1;
        border-radius: var(--radius-md);
        border: 1px solid var(--border);
        background: var(--surface);
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast);
    }
    .room-emoji-btn:hover { background: var(--surface-hover); }
    .room-emoji-btn.open { border-color: var(--primary); background: var(--primary-soft); }

    @media (max-width: 600px) {
        .create-name { font-size: 1.25rem; }
        .suggest-chip { padding: 10px 14px; font-size: 14px; }
        .room-emoji-btn { width: 44px; height: 44px; }
    }

    /* ── Out-of-the-box flair ─────────────────────────── */

    /* Giant faint emoji watermark behind each tile's content. */
    .room-watermark {
        position: absolute;
        right: -8px;
        bottom: -12px;
        font-size: 58px;
        line-height: 1;
        opacity: 0.1;
        transform: rotate(-8deg);
        pointer-events: none;
        z-index: 0;
        transition: opacity var(--t-med), transform var(--t-med);
    }
    .room.lit .room-watermark {
        opacity: calc(0.12 + var(--warmth, 0) * 0.24);
        transform: rotate(-8deg) scale(1.06);
    }
    .room.selected .room-watermark { opacity: 0.22; }
    /* Keep the real content above the watermark. */
    .room-head, .dots { position: relative; z-index: 1; }

    /* Lit rooms get a warm halo that scales with how much is on. */
    .room.lit {
        box-shadow:
            0 0 0 1px rgba(245, 189, 110, calc(var(--warmth, 0) * 0.28)),
            0 10px 26px -14px rgba(245, 189, 110, calc(var(--warmth, 0) * 0.7));
    }
    /* Selection cue always wins over the lit halo. */
    .room.selected {
        box-shadow: 0 0 0 3px var(--primary-glow);
    }

    /* House-pulse hero: a breathing amber aura when anything is on. */
    .pulse { position: relative; overflow: hidden; }
    .pulse::before {
        content: "";
        position: absolute;
        inset: 0;
        z-index: 0;
        background: radial-gradient(120% 160% at 10% 50%,
            rgba(245, 189, 110, 0.18), transparent 55%);
        opacity: 0;
        transition: opacity var(--t-med);
        pointer-events: none;
    }
    .pulse[data-active="true"]::before {
        opacity: 1;
        animation: aura-breathe 4.5s ease-in-out infinite;
    }
    @keyframes aura-breathe {
        0%, 100% { opacity: 0.6; transform: scale(1); }
        50%      { opacity: 1;   transform: scale(1.04); }
    }
    .pulse-num, .pulse-text { position: relative; z-index: 1; }
    .pulse[data-active="true"] .pulse-num .big {
        text-shadow: 0 0 18px var(--on-glow);
    }

    /* Honour reduced-motion: drop the ambient animation. */
    @media (prefers-reduced-motion: reduce) {
        .pulse[data-active="true"]::before { animation: none; }
    }
</style>

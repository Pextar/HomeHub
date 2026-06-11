<script lang="ts">
    import { onMount, tick } from "svelte";
    import Icon from "../components/Icon.svelte";
    import { data, toasts, route } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { socketAction, protocolKind, isSmartProtocol } from "../lib/utils";
    import type { Socket, Group } from "../lib/types";

    const v = $derived(data.value);

    const totalOn = $derived(v.sockets.filter((s) => s.state).length);
    const totalSockets = $derived(v.sockets.length);
    const roomCount = $derived(new Set(v.sockets.map((s) => s.room?.trim() || "Unassigned")).size);
    const hubUp = $derived(v.health === "ok");

    const protoKey = protocolKind;
    const PROTO_COLOR: Record<string, string> = {
        rf: "var(--p-rf)", wifi: "var(--p-wifi)", matter: "var(--p-matter)", mqtt: "var(--p-mqtt)",
    };

    // Devices sorted room → name so the table reads like a host list.
    const devices = $derived(
        [...v.sockets].sort((a, b) => {
            const ar = (a.room || "").toLowerCase(), br = (b.room || "").toLowerCase();
            if (ar !== br) return ar.localeCompare(br);
            return a.name.localeCompare(b.name);
        }),
    );

    function hostOf(s: Socket): string {
        const ns = (s.room?.trim() || "unassigned").toLowerCase().replace(/\s+/g, "-");
        const name = s.name.toLowerCase().replace(/\s+/g, "-");
        return `${ns}/${name}`;
    }

    // ── Live brightness ───────────────────────────────────────────────
    // Smart lights (Tasmota/Matter) carry a 0-100 level the base Socket type
    // doesn't, so fetch it per-device and key it by socket id. Best-effort:
    // unreachable lights simply drop out and fall back to a binary bar.
    let levels = $state<Record<string, number>>({});
    let levelsBusy = false;
    async function refreshLevels() {
        if (levelsBusy) return;
        levelsBusy = true;
        try {
            const smart = v.sockets.filter((s) => s.protocol === "tasmota" || s.protocol.startsWith("matter"));
            const results = await Promise.allSettled(smart.map(async (s) => {
                if (s.protocol === "tasmota") {
                    const st = await api.tasmotaGetState(s.id);
                    return [s.id, st.dimmer ?? (st.on ? 100 : 0)] as const;
                }
                const st = await api.matterGetState(s.id);
                return [s.id, st.level ?? (st.on ? 100 : 0)] as const;
            }));
            const next: Record<string, number> = {};
            for (const r of results) if (r.status === "fulfilled" && r.value[1] != null) next[r.value[0]] = r.value[1];
            levels = next;
        } finally {
            levelsBusy = false;
        }
    }
    // Refetch on mount and whenever the smart-light set or on/off states
    // change. Dimming keeps a light "on", so runLine also refetches directly.
    // The signature comparison matters: every 30s poll/SSE refresh replaces
    // the sockets array (new identity), which re-runs this effect — without
    // the early return each refresh would re-query every bridge device.
    let lastLevelsSig: string | null = null;
    $effect(() => {
        const sig = v.sockets.filter((s) => s.protocol === "tasmota" || s.protocol.startsWith("matter"))
            .map((s) => s.id + (s.state ? "1" : "0")).join(",");
        if (sig === lastLevelsSig) return;
        lastLevelsSig = sig;
        refreshLevels();
    });
    onMount(() => {
        const id = setInterval(refreshLevels, 30_000);
        return () => clearInterval(id);
    });

    // ── Interactive brightness bar (smart lights) ─────────────────────
    // Drag or click along a light's LEVEL bar to set it; arrow keys nudge
    // by 10%. Snapped to 10% so the value matches the 10-cell block display.
    const isDimmable = (s: Socket) => isSmartProtocol(s.protocol);
    let drag = $state<{ id: string; pct: number } | null>(null);

    function pctFromX(clientX: number, el: HTMLElement): number {
        const rect = el.getBoundingClientRect();
        const ratio = rect.width ? (clientX - rect.left) / rect.width : 0;
        return Math.max(0, Math.min(100, Math.round(ratio * 10) * 10));
    }

    // Optimistically reflect the new level (and on/off), push it to the
    // device, then reconcile from the server.
    async function commitLevel(s: Socket, pct: number) {
        levels = { ...levels, [s.id]: pct };
        if (pct > 0 && !s.state) data.applySocket({ ...s, state: true });
        if (pct <= 0 && s.state) data.applySocket({ ...s, state: false });
        try {
            await setLevel(s, pct);
        } catch (e) {
            toasts.error("Set level failed", (e as Error).message);
        }
        await data.refresh();
        void refreshLevels();
    }

    function onBarDown(e: PointerEvent, s: Socket) {
        e.preventDefault();
        const el = e.currentTarget as HTMLElement;
        el.setPointerCapture(e.pointerId);
        drag = { id: s.id, pct: pctFromX(e.clientX, el) };
    }
    function onBarMove(e: PointerEvent, s: Socket) {
        if (drag?.id !== s.id) return;
        drag = { id: s.id, pct: pctFromX(e.clientX, e.currentTarget as HTMLElement) };
    }
    async function onBarUp(e: PointerEvent, s: Socket) {
        if (drag?.id !== s.id) return;
        const pct = drag.pct;
        drag = null;
        await commitLevel(s, pct);
    }
    function onBarKey(e: KeyboardEvent, s: Socket, cur: number) {
        let next: number | null = null;
        if (e.key === "ArrowRight" || e.key === "ArrowUp") next = cur + 10;
        else if (e.key === "ArrowLeft" || e.key === "ArrowDown") next = cur - 10;
        else if (e.key === "Home") next = 0;
        else if (e.key === "End") next = 100;
        else return;
        e.preventDefault();
        void commitLevel(s, Math.max(0, Math.min(100, next)));
    }

    // ── Live event tail ───────────────────────────────────────────────
    // Server activity (real) plus a local echo of commands typed here, so
    // feedback is immediate without waiting for the next refresh.
    type LogLine = { t: string; k: "ok" | "set" | "in" | "err"; m: string };
    const EV_COLOR: Record<LogLine["k"], string> = {
        ok: "var(--good)", set: "var(--on)", in: "var(--cool)", err: "var(--bad)",
    };

    function hms(iso: string): string {
        const d = new Date(iso);
        if (isNaN(d.getTime())) return "--:--:--";
        return d.toTimeString().slice(0, 8);
    }
    function classify(action: string, status: string): LogLine["k"] {
        if (status === "error") return "err";
        if (/^(on|off|toggle)$/.test(action)) return "set";
        if (action === "activate") return "ok";
        return "in";
    }

    let localLog = $state<LogLine[]>([]);
    function echo(k: LogLine["k"], m: string) {
        localLog = [{ t: new Date().toTimeString().slice(0, 8), k, m }, ...localLog].slice(0, 40);
    }

    const serverLog = $derived<LogLine[]>(
        v.activity.map((e) => ({
            t: hms(e.time),
            k: classify(e.action, e.status),
            m: e.error ? `${e.label}: ${e.error}` : `${e.label}${e.action ? ` → ${e.action}` : ""}`,
        })),
    );
    // Most recent 60 events, oldest → newest so the freshest sits at the
    // bottom, next to the prompt — like a real terminal.
    const tail = $derived([...localLog, ...serverLog].slice(0, 60).reverse());

    // Auto-scroll the log to the newest line, but only when the user is
    // already parked near the bottom (so scrolling up to read isn't yanked).
    let scrollEl = $state<HTMLElement>();
    let stick = $state(false);
    function onScroll() {
        if (!scrollEl) return;
        stick = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight < 48;
    }
    async function scrollToBottom() {
        await tick();
        if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    }
    $effect(() => {
        void tail;
        if (stick) scrollToBottom();
    });

    // ── Command bar ───────────────────────────────────────────────────
    let cmd = $state("");
    let busy = $state(false);
    let focused = $state(false);
    let inputEl = $state<HTMLInputElement>();

    type Action = "on" | "off" | "toggle";

    // A command target can be a device, a group, or a room. on/off/toggle all
    // resolve against the same name pool so you don't have to remember which
    // kind a name is.
    type Target =
        | { kind: "device"; socket: Socket }
        | { kind: "group"; group: Group }
        | { kind: "room"; name: string };

    function roomNames(): string[] {
        return [...new Set(v.sockets.map((s) => s.room?.trim()).filter(Boolean) as string[])];
    }

    // Resolve a free-text query to a target. Exact matches (device → group →
    // room) win; otherwise fall back to a substring match in the same order.
    function resolveTarget(raw: string): Target | undefined {
        const q = raw.trim().toLowerCase().replace(/^(the|my)\s+/, "");
        if (!q) return undefined;
        const rooms = roomNames();

        const sEx = v.sockets.find((s) => s.name.toLowerCase() === q) ?? v.sockets.find((s) => hostOf(s) === q);
        if (sEx) return { kind: "device", socket: sEx };
        const gEx = v.groups.find((g) => g.name.toLowerCase() === q);
        if (gEx) return { kind: "group", group: gEx };
        const rEx = rooms.find((r) => r.toLowerCase() === q);
        if (rEx) return { kind: "room", name: rEx };

        const sIn = v.sockets.find((s) => s.name.toLowerCase().includes(q));
        if (sIn) return { kind: "device", socket: sIn };
        const gIn = v.groups.find((g) => g.name.toLowerCase().includes(q));
        if (gIn) return { kind: "group", group: gIn };
        const rIn = rooms.find((r) => r.toLowerCase().includes(q));
        if (rIn) return { kind: "room", name: rIn };

        return undefined;
    }

    // Apply an action to a resolved target; returns a label for the log.
    async function applyAction(t: Target, action: Action): Promise<string> {
        if (t.kind === "device") {
            await socketAction(t.socket, action);
            return `${hostOf(t.socket)} → ${action}`;
        }
        if (t.kind === "group") {
            const r = await api.groupAction(t.group.id, action);
            await data.refresh();
            return `group:${t.group.name} → ${action} (${r.updated})`;
        }
        // room — backend has no toggle, only on/off
        if (action === "toggle") throw new Error("rooms can only be turned on or off");
        const r = action === "on" ? await api.roomOn(t.name) : await api.roomOff(t.name);
        await data.refresh();
        return `room:${t.name} → ${action} (${r.updated})`;
    }

    // Match a room name leniently in both directions ("living room" ↔ "Living").
    function resolveRoom(raw: string): string | undefined {
        const q = raw.trim().toLowerCase();
        if (!q) return undefined;
        const rooms = roomNames();
        return rooms.find((r) => r.toLowerCase() === q)
            ?? rooms.find((r) => r.toLowerCase().includes(q) || q.includes(r.toLowerCase()));
    }

    function resolveGroup(raw: string): Group | undefined {
        const q = raw.trim().toLowerCase();
        if (!q) return undefined;
        return v.groups.find((g) => g.name.toLowerCase() === q) ?? v.groups.find((g) => g.name.toLowerCase().includes(q));
    }

    // Find a device by name within a specific room.
    function deviceInRoom(subject: string, room: string): Socket | undefined {
        const n = subject.trim().toLowerCase();
        if (!n) return undefined;
        const inRoom = v.sockets.filter((s) => (s.room?.trim().toLowerCase() ?? "") === room.toLowerCase());
        return inRoom.find((s) => s.name.toLowerCase() === n) ?? inRoom.find((s) => s.name.toLowerCase().includes(n));
    }

    // Words meaning "the whole scope" — drives "all"/whole-room commands.
    const WHOLE = new Set(["everything", "all", "all lights", "lights", "light", "them", "all of them", "everything else"]);

    // Strip filler words so sentences parse naturally ("turn off the lamp"
    // → "turn off lamp"). Keeps meaningful words like "in" and "all".
    function norm(s: string): string {
        return s.toLowerCase()
            .replace(/[.!?,]+$/g, "")
            .replace(/\b(the|a|an|please|just|to|of)\b/g, " ")
            .replace(/\s+/g, " ")
            .trim();
    }

    // Pull the on/off/toggle action out of a token list, whether it leads
    // ("turn off X", "off X") or trails ("X off", "turn X off").
    function extractAction(tokens: string[]): { action: Action; rest: string[] } | null {
        const lead = tokens[0];
        const last = tokens[tokens.length - 1];
        if (lead === "turn" || lead === "switch") {
            if (tokens[1] === "on" || tokens[1] === "off") return { action: tokens[1] as Action, rest: tokens.slice(2) };
            if (last === "on" || last === "off") return { action: last as Action, rest: tokens.slice(1, -1) };
            return null;
        }
        if (lead === "on" || lead === "off" || lead === "toggle") return { action: lead as Action, rest: tokens.slice(1) };
        if (last === "on" || last === "off" || last === "toggle") return { action: last as Action, rest: tokens.slice(0, -1) };
        return null;
    }

    async function doAction(action: Action, targetStr: string) {
        if (!targetStr.trim()) { echo("err", `usage: ${action} <device | group | room>`); return; }
        const t = resolveTarget(targetStr);
        if (!t) { echo("err", `nothing matching "${targetStr.trim()}"`); return; }
        echo("set", await applyAction(t, action));
    }

    async function allOnOff(action: Action) {
        if (action === "toggle") { echo("err", `say "all on" or "all off"`); return; }
        const r = action === "on" ? await api.allOn() : await api.allOff();
        echo("ok", `all ${action} (${r.updated})`);
        await data.refresh();
    }

    // ── Brightness & colour (smart lights only) ───────────────────────
    const COLORS: Record<string, string> = {
        red: "ff3b30", orange: "ff9500", amber: "ffb84d", warm: "ffd9a0",
        yellow: "ffd60a", lime: "a8e060", green: "34c759", teal: "40c8c0",
        cyan: "32ade6", blue: "0a84ff", indigo: "5e5ce6", violet: "bf5af2",
        purple: "bf5af2", magenta: "ff2d9b", pink: "ff375f", white: "ffffff",
    };
    const isSmart = (s: Socket) => isSmartProtocol(s.protocol);

    // Expand a target to the concrete sockets it covers.
    function socketsForTarget(t: Target): Socket[] {
        if (t.kind === "device") return [t.socket];
        if (t.kind === "group") return t.group.socket_ids.map((id) => v.sockets.find((s) => s.id === id)).filter(Boolean) as Socket[];
        return v.sockets.filter((s) => (s.room?.trim() ?? "") === t.name);
    }
    function labelOf(t: Target): string {
        return t.kind === "device" ? hostOf(t.socket) : t.kind === "group" ? `group:${t.group.name}` : `room:${t.name}`;
    }
    async function setLevel(s: Socket, pct: number) {
        if (pct <= 0) { await socketAction(s, "off"); return; }
        if (s.protocol === "tasmota") await api.tasmotaSetState(s.id, { dimmer: pct, on: true });
        else await api.matterSetState(s.id, { level: pct, on: true });
    }
    async function setColor(s: Socket, hex: string) {
        if (s.protocol === "tasmota") await api.tasmotaSetState(s.id, { color: hex, on: true });
        else await api.matterSetState(s.id, { color: hex, on: true });
    }
    // Apply a per-light mutation across a target, skipping non-smart members.
    async function applySmart(targetStr: string, verb: string, fn: (s: Socket) => Promise<void>) {
        const t = resolveTarget(targetStr);
        if (!t) { echo("err", `nothing matching "${targetStr.trim()}"`); return; }
        const socks = socketsForTarget(t);
        let done = 0, skipped = 0;
        for (const s of socks) {
            if (!isSmart(s)) { skipped++; continue; }
            try { await fn(s); done++; } catch { skipped++; }
        }
        await data.refresh();
        if (done === 0) echo("err", `no dimmable lights in "${targetStr.trim()}"`);
        else echo("set", `${labelOf(t)} → ${verb}${skipped ? ` (${skipped} skipped)` : ""}`);
    }
    const applyLevel = (target: string, pct: number) =>
        applySmart(target, `${Math.max(0, Math.min(100, pct))}%`, (s) => setLevel(s, Math.max(0, Math.min(100, pct))));
    const applyColor = (target: string, name: string) =>
        applySmart(target, name, (s) => setColor(s, COLORS[name]));

    // ── Query / info commands (print to the tail, change nothing) ─────
    function runQuery(first: string): boolean {
        if (first === "status") {
            echo("in", `${totalOn}/${totalSockets} on · ${roomCount} rooms · hub:${hubUp ? "up" : "down"}`);
        } else if (first === "ls" || first === "list" || first === "devices") {
            const items = devices.slice(0, 16).map((d) => `${d.name}=${d.state ? "on" : "off"}`);
            echo("in", `devices: ${items.join(" · ")}${devices.length > 16 ? ` · +${devices.length - 16}` : ""}` || "devices: none");
        } else if (first === "rooms") {
            const rs = roomNames().map((r) => {
                const ss = v.sockets.filter((s) => (s.room?.trim() ?? "") === r);
                return `${r} ${ss.filter((s) => s.state).length}/${ss.length}`;
            });
            echo("in", rs.length ? `rooms: ${rs.join(" · ")}` : "rooms: none");
        } else if (first === "groups") {
            echo("in", v.groups.length ? `groups: ${v.groups.map((g) => `${g.name}(${g.socket_ids.length})`).join(" · ")}` : "groups: none");
        } else if (first === "scenes") {
            echo("in", v.scenes.length ? `scenes: ${v.scenes.map((s) => s.name).join(" · ")}` : "scenes: none");
        } else if (first === "on?" || first === "lit") {
            const on = v.sockets.filter((s) => s.state).map((s) => s.name);
            echo("in", on.length ? `on: ${on.join(" · ")}` : "on: nothing");
        } else if (first === "clear" || first === "cls") {
            localLog = [];
        } else if (first === "help" || first === "?") {
            echo("in", "on|off|toggle <device|group|room> · turn off <name> · <name> off · X in <room> · set <name> 60% · set <name> warm · scene <name> · all off · and/then chains · ls·rooms·groups·scenes·status·clear · ↑↓ history · Tab complete · drag a LEVEL bar to dim");
        } else return false;
        return true;
    }

    // Execute one command segment. Returns the action it applied (so the next
    // bare segment in an "and" chain can inherit it), or undefined.
    async function execSegment(raw: string, inherited?: Action): Promise<Action | undefined> {
        const line = raw.trim();
        if (!line) return undefined;
        const lower = line.toLowerCase().replace(/[.!?,]+$/g, "");
        const first = lower.split(/\s+/)[0];

        if (runQuery(first)) return undefined;

        // scene activation
        if (first === "scene" || first === "activate") {
            const q = line.slice(first.length).trim();
            const sc = v.scenes.find((x) => x.name.toLowerCase() === q.toLowerCase())
                ?? v.scenes.find((x) => x.name.toLowerCase().includes(q.toLowerCase()));
            if (!sc) echo("err", `no scene matching "${q}"`);
            else { await api.activateScene(sc.id); echo("ok", `scene.${sc.name} activated`); await data.refresh(); }
            return undefined;
        }

        // explicit "room <name> on|off"
        if (first === "room") {
            const m = line.slice(4).trim().match(/^(.+?)\s+(on|off)$/i);
            const room = m && resolveRoom(m[1]);
            if (!m) echo("err", "usage: room <name> on|off");
            else if (!room) echo("err", `no room matching "${m[1].trim()}"`);
            else { echo("set", await applyAction({ kind: "room", name: room }, m[2].toLowerCase() as Action)); return m[2].toLowerCase() as Action; }
            return undefined;
        }
        // explicit "group <name> on|off|toggle"
        if (first === "group") {
            const m = line.slice(5).trim().match(/^(.+?)\s+(on|off|toggle)$/i);
            const g = m && resolveGroup(m[1]);
            if (!m) echo("err", "usage: group <name> on|off|toggle");
            else if (!g) echo("err", `no group matching "${m[1].trim()}"`);
            else { echo("set", await applyAction({ kind: "group", group: g }, m[2].toLowerCase() as Action)); return m[2].toLowerCase() as Action; }
            return undefined;
        }

        // "set <target> <value>" / "dim|brighten <target> [to] <pct>"
        if (first === "set" || first === "dim" || first === "brighten") {
            const body = line.slice(first.length).trim().replace(/\bto\b/gi, " ").replace(/\s+/g, " ").trim();
            const parts = body.split(" ");
            const value = parts[parts.length - 1]?.toLowerCase() ?? "";
            const target = parts.slice(0, -1).join(" ");
            if (first === "dim" && !/^\d/.test(value)) await applyLevel(body, 25);
            else if (first === "brighten" && !/^\d/.test(value)) await applyLevel(body, 100);
            else if (!target) echo("err", `usage: ${first} <name> <0-100 | colour | on|off>`);
            else if (value === "on" || value === "off" || value === "toggle") await doAction(value as Action, target);
            else if (/^\d{1,3}%?$/.test(value)) await applyLevel(target, parseInt(value, 10));
            else if (COLORS[value]) await applyColor(target, value);
            else echo("err", `don't know "${value}" — use 0-100, a colour, or on/off`);
            return undefined;
        }

        // trailing "<target> 60%" or "<target> to <colour>"
        const pctM = line.match(/^(.+?)\s+(?:to\s+)?(\d{1,3})\s*%$/i) ?? line.match(/^(.+?)\s+to\s+(\d{1,3})$/i);
        if (pctM) { await applyLevel(pctM[1], parseInt(pctM[2], 10)); return undefined; }
        const colM = line.match(/^(.+?)\s+to\s+([a-z]+)$/i);
        if (colM && COLORS[colM[2].toLowerCase()]) { await applyColor(colM[1], colM[2].toLowerCase()); return undefined; }

        // natural on/off/toggle phrasing
        const tokens = norm(line).split(" ").filter(Boolean);
        let ext = extractAction(tokens);
        if (!ext && inherited) ext = { action: inherited, rest: tokens }; // inherit verb in a chain
        if (!ext) { echo("err", `didn't understand "${line}" (try "help")`); return undefined; }

        const { action, rest } = ext;
        const inIdx = rest.indexOf("in");
        if (inIdx >= 0) {
            const subject = rest.slice(0, inIdx).join(" ").trim();
            const roomPhrase = rest.slice(inIdx + 1).join(" ");
            const room = resolveRoom(roomPhrase);
            if (!room) echo("err", `no room matching "${roomPhrase.trim()}"`);
            else if (!subject || WHOLE.has(subject)) echo("set", await applyAction({ kind: "room", name: room }, action));
            else {
                const dev = deviceInRoom(subject, room);
                if (!dev) echo("err", `no device "${subject}" in ${room}`);
                else echo("set", await applyAction({ kind: "device", socket: dev }, action));
            }
        } else {
            const phrase = rest.join(" ").trim();
            if (!phrase || WHOLE.has(phrase)) await allOnOff(action);
            else await doAction(action, phrase);
        }
        return action;
    }

    // ── Command history ───────────────────────────────────────────────
    let history = $state<string[]>([]);
    let hi = $state(-1); // -1 = current (unsubmitted) line

    // Split a line into segments on connectors so several commands can run
    // at once: "turn off the kitchen and the hall", "lamp on, tv off".
    const CONNECTORS = /\s*(?:,|;|&|\band\b|\bthen\b|\bplus\b)\s*/i;

    async function runLine(raw: string) {
        const whole = raw.trim();
        if (!whole || busy) return;
        busy = true;
        history = [whole, ...history.filter((h) => h !== whole)].slice(0, 50);
        hi = -1;
        echo("in", `› ${whole}`);
        try {
            const segs = whole.split(CONNECTORS).map((s) => s.trim()).filter(Boolean);
            let last: Action | undefined;
            for (const seg of segs) {
                const used = await execSegment(seg, last);
                if (used) last = used;
            }
        } catch (e) {
            echo("err", (e as Error).message);
            toasts.error("Command failed", (e as Error).message);
        } finally {
            busy = false;
            cmd = "";
            acReset();
            stick = true;
            void scrollToBottom();
            void refreshLevels();
        }
    }

    // Quick chips: always-useful commands plus the first scenes.
    const quick = $derived([
        "all off",
        "all on",
        "ls",
        ...v.scenes.slice(0, 3).map((s) => `scene ${s.name}`),
    ]);

    // ── Tab completion ────────────────────────────────────────────────
    const VERBS = [
        "turn on ", "turn off ", "toggle ", "on ", "off ", "set ", "scene ",
        "all off", "all on", "status", "list", "rooms", "groups", "scenes", "help", "clear",
    ];
    const vocab = $derived([
        ...v.sockets.map((s) => s.name),
        ...v.groups.map((g) => g.name),
        ...roomNames(),
        ...v.scenes.map((s) => s.name),
    ]);
    let acMatches: string[] = [];
    let acIdx = 0;
    let acHead = "";
    let acLast = "";
    function acReset() { acMatches = []; acIdx = 0; acLast = ""; }
    function tabComplete() {
        if (acMatches.length && cmd === acLast) {
            acIdx = (acIdx + 1) % acMatches.length;
        } else {
            const m = cmd.match(/^(\s*(?:(?:turn|switch)\s+(?:on|off)|on|off|toggle|set|dim|brighten|scene|activate|room|group)\s+)(.*)$/i);
            acHead = m ? m[1] : "";
            const frag = (m ? m[2] : cmd).replace(/^\s+/, "").toLowerCase();
            const pool = m ? vocab : [...VERBS, ...vocab];
            acMatches = pool.filter((x) => x.toLowerCase().startsWith(frag) && x.toLowerCase() !== frag);
            acIdx = 0;
        }
        if (!acMatches.length) return;
        cmd = acHead + acMatches[acIdx];
        acLast = cmd;
    }

    function onKey(e: KeyboardEvent) {
        if (e.key === "Enter") { e.preventDefault(); runLine(cmd); return; }
        if (e.key === "Tab") { e.preventDefault(); tabComplete(); return; }
        if (e.key === "ArrowUp") {
            e.preventDefault();
            if (history.length) { hi = Math.min(hi + 1, history.length - 1); cmd = history[hi]; }
            return;
        }
        if (e.key === "ArrowDown") {
            e.preventDefault();
            if (hi >= 0) { hi -= 1; cmd = hi < 0 ? "" : history[hi]; }
            return;
        }
        acReset();
    }
</script>

<div class="console">
    <header class="head">
        <button class="chip back" onclick={() => route.go("settings")} aria-label="Back to settings">
            <Icon name="chevronLeft" size={16} />
        </button>
        <div class="title">
            <div class="h">Console</div>
            <div class="conn mono" data-up={hubUp}>
                ● homehub@pi · {hubUp ? "live" : "offline"}
            </div>
        </div>
        <span class="head-spacer" aria-hidden="true"></span>
    </header>

    <!-- status box -->
    <div class="status">
        <div class="prompt mono">$ status --watch</div>
        <div class="status-box mono">
            <span class="hl">{String(totalOn).padStart(2, "0")}</span>/{String(totalSockets).padStart(2, "0")} on
            · <span class="hl">{roomCount}</span> room{roomCount === 1 ? "" : "s"}
            · <span class={hubUp ? "good" : "bad"}>hub:{hubUp ? "up" : "down"}</span>
            · <span class="cool">net:ok</span>
        </div>
    </div>

    <div class="scroll" bind:this={scrollEl} onscroll={onScroll}>
        <!-- devices -->
        <div class="sec-head mono"># DEVICES</div>
        {#if devices.length === 0}
            <div class="muted mono">no devices configured</div>
        {:else}
            <div class="row head-row mono">
                <span></span><span>HOST</span><span class="r">VAL</span><span>LEVEL</span>
            </div>
            {#each devices as d (d.id)}
                {@const dimmable = isDimmable(d)}
                {@const dragPct = drag && drag.id === d.id ? drag.pct : null}
                {@const dragging = dragPct != null}
                {@const lvl = dragPct ?? levels[d.id]}
                {@const pct = dragPct ?? (d.state ? (levels[d.id] ?? 100) : 0)}
                {@const filled = Math.round(pct / 10)}
                <div class="row mono" class:dim={!d.state}>
                    <span class="led" data-on={d.state}>{d.state ? "●" : "○"}</span>
                    <span class="host">
                        <span class="proto" style:color={PROTO_COLOR[protoKey(d.protocol)]}>{protoKey(d.protocol).padEnd(6)}</span><span class="hostname">{hostOf(d)}</span>
                    </span>
                    <span class="r state" data-on={d.state}>{dragging ? `${pct}%` : !d.state ? "--" : lvl != null ? `${lvl}%` : "ON"}</span>
                    {#if dimmable}
                        <span
                            class="bar interactive"
                            class:dragging
                            data-on={d.state || pct > 0}
                            role="slider"
                            tabindex="0"
                            aria-label={`${d.name} brightness`}
                            aria-valuemin="0"
                            aria-valuemax="100"
                            aria-valuenow={pct}
                            onpointerdown={(e) => onBarDown(e, d)}
                            onpointermove={(e) => onBarMove(e, d)}
                            onpointerup={(e) => onBarUp(e, d)}
                            onpointercancel={() => (drag = null)}
                            onkeydown={(e) => onBarKey(e, d, pct)}
                        >{"█".repeat(filled)}{"░".repeat(10 - filled)}</span>
                    {:else}
                        <span class="bar" data-on={d.state}>{"█".repeat(filled)}{"░".repeat(10 - filled)}</span>
                    {/if}
                </div>
            {/each}
        {/if}

        <!-- live tail -->
        <div class="sec-head mono"># TAIL · LIVE</div>
        {#if tail.length === 0}
            <div class="muted mono">no recent activity</div>
        {:else}
            {#each tail as e, i (i)}
                <div class="tail mono">
                    <span class="ts">{e.t}</span>
                    <span class="kind" style:color={EV_COLOR[e.k]}>{e.k.padEnd(3)}</span>
                    <span class="msg">{e.m}</span>
                </div>
            {/each}
        {/if}
    </div>

    <!-- command bar -->
    <div class="cmdbar">
        <div class="quick h-scroll">
            {#each quick as q (q)}
                <button class="qchip mono" onclick={() => runLine(q)} disabled={busy}>{q}</button>
            {/each}
        </div>
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <div class="cmdline" onclick={() => inputEl?.focus()}>
            <span class="caret mono">›</span>
            {#if !focused && !cmd}<span class="blink" aria-hidden="true"></span>{/if}
            <input
                bind:this={inputEl}
                class="cmdinput mono"
                type="text"
                bind:value={cmd}
                onkeydown={onKey}
                onfocus={() => (focused = true)}
                onblur={() => (focused = false)}
                placeholder="set lamp 60% · kitchen off and hall off · ↑ history · Tab · help"
                aria-label="Console command"
                autocomplete="off"
                autocapitalize="off"
                spellcheck="false"
            />
            {#if busy}<span class="run mono">…</span>{/if}
        </div>
    </div>
</div>

<style>
    /* Console is the one screen allowed the deepest surface (DESIGN.md §2).
       It is always a dark terminal, so the warm-dark palette is pinned here
       as scoped variables — child styles (and the .chip primitive) then
       render correctly even when the app is in light theme. */
    .console {
        --con-line: rgba(245, 189, 110, 0.2);
        --text: #eceae4;
        --text-mute: #9c988e;
        --text-dim: #66635c;
        --on: #f5bd6e;
        --on-soft: rgba(245, 189, 110, 0.14);
        --good: #9cc28a;
        --bad: #e08a7a;
        --cool: #84acc4;
        --card: #1f1d17;
        --card-2: #26231c;
        --card-3: #2e2a22;
        --hairline: #2a2720;
        --surface: rgba(245, 240, 228, 0.04);
        --surface-hover: rgba(245, 240, 228, 0.08);
        --p-rf: #f5a06e;
        --p-wifi: #9cc28a;
        --p-matter: #c4a4e0;
        --p-mqtt: #e0c47a;
        display: flex;
        flex-direction: column;
        min-height: calc(100vh - var(--space-7) * 2);
        margin: calc(var(--space-7) * -1) -36px;
        background: #0a0907;
        font-family: var(--font-mono);
        color: var(--text);
    }

    @media (max-width: 900px) {
        /* Break out of the main padding on the top and sides, but keep the
           bottom in normal flow so the command bar stays clear of the fixed
           bottom nav (main already reserves space for it). */
        .console {
            margin: calc(var(--space-4) * -1) calc(var(--space-4) * -1) 0;
            min-height: calc(100vh - 60px - var(--space-4) * 2 - env(safe-area-inset-bottom));
        }
    }

    .head {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 14px 16px 10px;
        border-bottom: 1px solid var(--con-line);
    }
    .back { width: 36px; height: 36px; padding: 0; justify-content: center; flex-shrink: 0; }
    .title { flex: 1; text-align: center; font-family: var(--font-sans); }
    .head-spacer { width: 36px; flex-shrink: 0; }
    .h { font-size: 15px; font-weight: 600; color: var(--text); }
    .conn { font-size: 10px; letter-spacing: 0.1em; margin-top: 1px; color: var(--good); }
    .conn[data-up="false"] { color: var(--bad); }

    .status { padding: 12px 16px 6px; }
    .prompt { color: var(--on); font-size: 11px; letter-spacing: 0.04em; }
    .status-box {
        margin-top: 8px;
        padding: 9px 12px;
        border: 1px solid var(--con-line);
        border-radius: var(--r-sm);
        font-size: 11px;
        color: var(--text-mute);
        line-height: 1.5;
    }
    .status-box .hl { color: var(--on); }
    .good { color: var(--good); }
    .bad { color: var(--bad); }
    .cool { color: var(--cool); }

    .scroll {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        -webkit-overflow-scrolling: touch;
        padding: 4px 16px 16px;
    }

    .sec-head {
        color: var(--text-dim);
        font-size: 9px;
        letter-spacing: 0.18em;
        margin: 16px 0 8px;
    }
    .sec-head:first-child { margin-top: 4px; }
    .muted { color: var(--text-dim); font-size: 11px; padding: 4px 0; }

    .row {
        display: grid;
        grid-template-columns: 14px 1fr 44px auto;
        gap: 8px;
        align-items: center;
        padding: 5px 0;
        font-size: 11px;
        border-bottom: 1px dotted rgba(245, 189, 110, 0.1);
    }
    .head-row {
        color: var(--text-dim);
        font-size: 9px;
        letter-spacing: 0.1em;
        border-bottom: none;
        padding-bottom: 2px;
    }
    .row.dim { color: var(--text-mute); }
    .led { color: var(--on); }
    .led[data-on="false"] { color: var(--text-dim); }
    .host { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
    .proto { font-size: 9px; letter-spacing: 0.1em; white-space: pre; }
    .hostname { margin-left: 6px; }
    .r { text-align: right; }
    .state { color: var(--on); }
    .state[data-on="false"] { color: var(--text-dim); }
    .bar { letter-spacing: -0.5px; color: var(--on); white-space: nowrap; }
    .bar[data-on="false"] { color: var(--text-dim); }
    /* Draggable brightness bar for smart lights. */
    .bar.interactive {
        cursor: ew-resize;
        touch-action: none;
        user-select: none;
        -webkit-user-select: none;
        padding: 4px 2px;
        margin: -4px 0;
        border-radius: 4px;
    }
    .bar.interactive:hover { background: var(--surface); }
    .bar.interactive.dragging { background: var(--surface-hover); }
    .bar.interactive:focus-visible { outline: 1px solid var(--on); outline-offset: 1px; box-shadow: none; }
    /* Bigger touch target on coarse pointers without changing the row much. */
    @media (pointer: coarse) {
        .bar.interactive { padding: 10px 4px; margin: -10px 0; }
    }

    .tail { font-size: 10.5px; line-height: 1.6; color: var(--text-mute); }
    .tail .ts { color: var(--text-dim); }
    .tail .kind { margin: 0 8px; white-space: pre; }
    .tail .msg { color: var(--text); }

    .cmdbar {
        border-top: 1px solid rgba(245, 189, 110, 0.25);
        background: #0a0907;
        padding: 10px 16px calc(14px + env(safe-area-inset-bottom));
    }
    .quick { margin-bottom: 10px; }
    .qchip {
        flex-shrink: 0;
        border: 1px solid rgba(245, 189, 110, 0.3);
        background: transparent;
        padding: 4px 9px;
        font-size: 10px;
        letter-spacing: 0.04em;
        color: var(--on);
        border-radius: var(--r-sm);
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast);
    }
    .qchip:hover { background: var(--on-soft); }
    .qchip:disabled { opacity: 0.5; cursor: not-allowed; }
    .cmdline { display: flex; align-items: center; gap: 8px; cursor: text; }
    .caret { color: var(--on); font-size: 14px; }
    /* Idle terminal cursor — shown only when unfocused and empty. While the
       field is focused the native (amber) caret takes over. */
    .blink {
        width: 8px;
        height: 15px;
        background: var(--on);
        flex-shrink: 0;
        animation: blink 1.1s steps(2) infinite;
    }
    @keyframes blink { 50% { opacity: 0; } }
    @media (prefers-reduced-motion: reduce) { .blink { animation: none; } }
    .cmdinput {
        flex: 1;
        background: transparent;
        border: none;
        padding: 6px 0;
        color: var(--text);
        caret-color: var(--on);
        font-size: 13px;
    }
    .cmdinput::placeholder { color: var(--text-dim); }
    .cmdinput:focus-visible { outline: none; box-shadow: none; }
    .run { color: var(--on); }

    @media (max-width: 600px) {
        .cmdinput { font-size: 16px; } /* prevents iOS zoom-on-focus */
        .qchip { min-height: 32px; }
    }
</style>

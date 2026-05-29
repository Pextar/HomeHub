<script lang="ts">
    import Icon from "../components/Icon.svelte";
    import { data, toasts, route } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { socketAction } from "../lib/utils";
    import type { Socket, Group } from "../lib/types";

    const v = $derived(data.value);

    const totalOn = $derived(v.sockets.filter((s) => s.state).length);
    const totalSockets = $derived(v.sockets.length);
    const roomCount = $derived(new Set(v.sockets.map((s) => s.room?.trim() || "Unassigned")).size);
    const hubUp = $derived(v.health === "ok");

    function protoKey(p: string): "rf" | "wifi" | "matter" | "mqtt" {
        if (p === "tasmota" || p === "wifi") return "wifi";
        if (p.startsWith("matter")) return "matter";
        if (p === "mqtt") return "mqtt";
        return "rf";
    }
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
    const tail = $derived([...localLog, ...serverLog].slice(0, 60));

    // ── Command bar ───────────────────────────────────────────────────
    let cmd = $state("");
    let busy = $state(false);

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
            await api.groupAction(t.group.id, action);
            await data.refresh();
            return `group:${t.group.name} → ${action}`;
        }
        // room — backend has no toggle, only on/off
        if (action === "toggle") throw new Error("rooms can only be turned on or off");
        if (action === "on") await api.roomOn(t.name); else await api.roomOff(t.name);
        await data.refresh();
        return `room:${t.name} → ${action}`;
    }

    async function doAction(action: Action, targetStr: string) {
        if (!targetStr.trim()) { echo("err", `usage: ${action} <device | group | room>`); return; }
        const t = resolveTarget(targetStr);
        if (!t) { echo("err", `nothing matching "${targetStr.trim()}"`); return; }
        echo("set", await applyAction(t, action));
    }

    async function run(raw: string) {
        const line = raw.trim();
        if (!line || busy) return;
        busy = true;
        echo("in", `› ${line}`);
        try {
            const parts = line.split(/\s+/);
            const verb = parts[0].toLowerCase();
            const rest = parts.slice(1).join(" ");

            // everything on/off
            if (verb === "all" && (parts[1]?.toLowerCase() === "off" || parts[1]?.toLowerCase() === "on")) {
                const on = parts[1].toLowerCase() === "on";
                if (on) await api.allOn(); else await api.allOff();
                echo("ok", `all ${on ? "on" : "off"}`);
                await data.refresh();

            // bare action verb: on/off/toggle <target>
            } else if (verb === "on" || verb === "off" || verb === "toggle") {
                await doAction(verb, rest);

            // natural phrasing: "turn off …" / "switch on …"
            } else if (verb === "turn" || verb === "switch") {
                const st = parts[1]?.toLowerCase();
                if (st === "on" || st === "off") await doAction(st, parts.slice(2).join(" "));
                else echo("err", `usage: ${verb} on|off <device | group | room>`);

            // scene activation
            } else if (verb === "scene" || verb === "activate") {
                const sc = v.scenes.find((x) => x.name.toLowerCase() === rest.toLowerCase())
                    ?? v.scenes.find((x) => x.name.toLowerCase().includes(rest.toLowerCase()));
                if (!sc) { echo("err", `no scene matching "${rest}"`); }
                else { await api.activateScene(sc.id); echo("ok", `scene.${sc.name} activated`); await data.refresh(); }

            // explicit room verb (kept for clarity): "room <name> on|off"
            } else if (verb === "room") {
                const m = rest.match(/^(.*)\s+(on|off)$/i);
                if (!m) { echo("err", "usage: room <name> on|off"); }
                else { echo("set", await applyAction({ kind: "room", name: m[1].trim() }, m[2].toLowerCase() as Action)); }

            } else if (verb === "help" || verb === "?") {
                echo("in", "turn on|off <name> · on|off|toggle <device|group|room> · scene <name> · all on|off");

            } else {
                echo("err", `unknown command: ${verb} (try "help")`);
            }
        } catch (e) {
            echo("err", (e as Error).message);
            toasts.error("Command failed", (e as Error).message);
        } finally {
            busy = false;
            cmd = "";
        }
    }

    // Quick chips: a couple of always-useful commands plus the first scenes.
    const quick = $derived([
        "all off",
        "all on",
        ...v.scenes.slice(0, 3).map((s) => `scene ${s.name}`),
    ]);

    function onKey(e: KeyboardEvent) {
        if (e.key === "Enter") { e.preventDefault(); run(cmd); }
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

    <div class="scroll">
        <!-- devices -->
        <div class="sec-head mono"># DEVICES</div>
        {#if devices.length === 0}
            <div class="muted mono">no devices configured</div>
        {:else}
            <div class="row head-row mono">
                <span></span><span>HOST</span><span class="r">STATE</span><span>LEVEL</span>
            </div>
            {#each devices as d (d.id)}
                <div class="row mono" class:dim={!d.state}>
                    <span class="led" data-on={d.state}>{d.state ? "●" : "○"}</span>
                    <span class="host">
                        <span class="proto" style:color={PROTO_COLOR[protoKey(d.protocol)]}>{protoKey(d.protocol).padEnd(6)}</span><span class="hostname">{hostOf(d)}</span>
                    </span>
                    <span class="r state" data-on={d.state}>{d.state ? "ON" : "--"}</span>
                    <span class="bar" data-on={d.state}>{d.state ? "██████████" : "░░░░░░░░░░"}</span>
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
                <button class="qchip mono" onclick={() => run(q)} disabled={busy}>{q}</button>
            {/each}
        </div>
        <div class="cmdline">
            <span class="caret mono">›</span>
            <input
                class="cmdinput mono"
                type="text"
                bind:value={cmd}
                onkeydown={onKey}
                placeholder="turn off lamp · on kitchen · scene evening · all off · help"
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
    .bar { letter-spacing: -0.5px; color: var(--on); }
    .bar[data-on="false"] { color: var(--text-dim); }

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
    .cmdline { display: flex; align-items: center; gap: 8px; }
    .caret { color: var(--on); font-size: 14px; }
    .cmdinput {
        flex: 1;
        background: transparent;
        border: none;
        padding: 6px 0;
        color: var(--text);
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

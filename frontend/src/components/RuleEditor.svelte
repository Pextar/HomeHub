<script lang="ts" module>
    import { data } from "../lib/stores.svelte";
    import { isSmartProtocol } from "../lib/utils";
    import type { RuleActionDraft, RuleDraft, TargetType, AutomationAction, Socket } from "../lib/types";

    /** Member sockets of a group/room action target. Rooms are matched by the
     *  socket's room name (sockets reference rooms by name, targets by id). */
    export function membersOf(a: RuleActionDraft): Socket[] {
        const v = data.value;
        if (a.target_type === "group") {
            const g = v.groups.find(x => x.id === a.target_id);
            return (g?.socket_ids ?? [])
                .map(id => v.sockets.find(s => s.id === id))
                .filter((s): s is Socket => !!s);
        }
        if (a.target_type === "room") {
            const rn = v.rooms.find(r => r.id === a.target_id)?.name;
            return v.sockets.filter(s => s.room === rn);
        }
        return [];
    }

    /** Expand one THEN draft action into the API actions it implies. A per-lamp
     *  group/room action becomes one socket action per configured member; every
     *  other action maps 1:1, mirroring the legacy inline builders. */
    export function compileAction(a: RuleActionDraft): AutomationAction[] {
        if ((a.target_type === "group" || a.target_type === "room") && a.action === "on" && a.perLamp) {
            const out: AutomationAction[] = [];
            for (const m of membersOf(a)) {
                const cfg = a.perLamp[m.id] ?? { state: "on", level: a.level ?? 100, color: a.color ?? "" };
                if (cfg.state === "ignore") continue;
                const act: AutomationAction = { target_type: "socket", target_id: m.id, action: cfg.state };
                if (cfg.state === "on" && isSmartProtocol(m.protocol)) {
                    act.level = cfg.level ?? 100;
                    if (cfg.color) act.color = cfg.color;
                }
                out.push(act);
            }
            return out;
        }
        const base: AutomationAction = {
            target_type: a.target_type,
            target_id: a.target_id,
            action: (a.target_type === "scene" ? "activate" : a.action) as AutomationAction["action"],
        };
        if (a.action === "on") {
            if (a.target_type === "socket") {
                base.level = a.level ?? 100;
                if (a.color) base.color = a.color;
            } else if (a.target_type === "group" || a.target_type === "room") {
                // Uniform: only attach lighting info when the user moved it off the default.
                if (a.color || a.level !== 100) {
                    base.level = a.level ?? 100;
                    if (a.color) base.color = a.color;
                }
            }
        }
        return [base];
    }

    /** Colour presets for smart socket targets. Also used by the
     *  SceneModal snapshot section — keep the two in sync by importing. */
    export const COLOURS: { hex: string; name: string }[] = [
        { hex: "", name: "Auto" },
        { hex: "f5bd6e", name: "Warm" },
        { hex: "ffe9c4", name: "Soft" },
        { hex: "ffffff", name: "Bright" },
        { hex: "c4a4e0", name: "Lilac" },
        { hex: "7aa4d9", name: "Cool" },
    ];

    /** Selectable targets for a THEN action's target type. */
    export function targetsFor(type: string): { id: string; label: string }[] {
        const v = data.value;
        if (type === "socket") return v.sockets.map(s => ({ id: s.id, label: s.name }));
        if (type === "group")  return v.groups.map(g => ({ id: g.id, label: g.name }));
        if (type === "room")   return [...v.rooms].sort((a, b) => a.name.localeCompare(b.name)).map(r => ({ id: r.id, label: r.name }));
        return v.scenes.map(s => ({ id: s.id, label: s.name }));
    }

    /** First target type that has at least one entity to point at. */
    export function firstTargetType(): TargetType {
        const v = data.value;
        return v.sockets.length ? "socket" : v.groups.length ? "group" : v.rooms.length ? "room" : "scene";
    }

    /** A fresh THEN row, seeded with the first valid target so a newly
     *  added row never needs to be "fixed up" later. */
    export function blankRuleAction(): RuleActionDraft {
        const target_type = firstTargetType();
        return {
            target_type,
            target_id: targetsFor(target_type)[0]?.id ?? "",
            action: target_type === "scene" ? "activate" : "on",
            level: 100,
            color: "",
        };
    }
</script>

<script lang="ts" generics="T extends RuleDraft">
    import Segmented from "./Segmented.svelte";
    import DayPicker from "./DayPicker.svelte";
    import Icon from "./Icon.svelte";
    // isSmartProtocol is imported in the module script above (shared scope).

    interface Props {
        /** The rule being edited; the editor mutates it in place. */
        draft: T;
        /** Unique per instance — prefixes input ids and radio-group names. */
        idPrefix: string;
    }
    let { draft = $bindable(), idPrefix }: Props = $props();

    const v = data.value;
    const isSmart = isSmartProtocol;

    // Matter lamp presets — same palette as MatterLightModal. White presets
    // carry a CT-approximated hex for display and to nudge the RGB mode toward
    // the right temperature on lamps that don't expose CT via this bridge path.
    type MatterPreset = { label: string; level: number; color: string; cssColor: string };
    const MATTER_PRESETS: MatterPreset[] = [
        { label: "Reading",     level: 100, color: "dcdbd6", cssColor: "#dcdbd6" },
        { label: "Concentrate", level: 100, color: "d2e5f4", cssColor: "#d2e5f4" },
        { label: "Daylight",    level: 100, color: "d5e2eb", cssColor: "#d5e2eb" },
        { label: "Warm",        level: 80,  color: "edcaa2", cssColor: "#edcaa2" },
        { label: "Relax",       level: 40,  color: "f1c696", cssColor: "#f1c696" },
        { label: "Night",       level: 12,  color: "f9bf7f", cssColor: "#f9bf7f" },
        { label: "Sunset",      level: 70,  color: "ff6a3d", cssColor: "#ff6a3d" },
        { label: "Forest",      level: 60,  color: "3dbf6a", cssColor: "#3dbf6a" },
        { label: "Ocean",       level: 70,  color: "3dafff", cssColor: "#3dafff" },
        { label: "Lavender",    level: 60,  color: "b47cff", cssColor: "#b47cff" },
        { label: "Rose",        level: 60,  color: "ff6fa3", cssColor: "#ff6fa3" },
    ];

    const hasLocation = $derived(v.settings.latitude !== 0 || v.settings.longitude !== 0);
    const sensorUnit = $derived(v.sensors.find(s => s.id === draft.trigSensorId)?.unit ?? "");

    function solarSummary(mode: string, offset: number): string {
        const event = mode === "sunrise" ? "sunrise" : "sunset";
        if (offset === 0) return `At ${event}`;
        const abs = Math.abs(offset);
        const h = Math.floor(abs / 60);
        const mins = abs % 60;
        const parts = [h && `${h}h`, mins && `${mins}m`].filter(Boolean).join(" ");
        return `${parts} ${offset < 0 ? "before" : "after"} ${event}`;
    }

    // Selections are seeded when a row is created (blankRuleAction) and
    // re-seeded only when the user switches the row's target type — never
    // from an effect. If the referenced entity has since been deleted, a
    // disabled "(removed)" option keeps the stale selection visible instead
    // of silently rewriting it mid-edit.
    function retarget(a: RuleActionDraft) {
        a.target_id = targetsFor(a.target_type)[0]?.id ?? "";
        if (a.target_type === "scene") a.action = "activate";
        else if (a.action === "activate") a.action = "on";
        a.perLamp = undefined; // membership changed — drop any per-lamp overrides
    }

    // ── Per-lamp authoring (group/room "on" actions) ──────────────────
    // Switching to per-lamp seeds every member from the uniform base the user
    // already set, so the matrix starts as "all the same" and they tweak the
    // outliers. Compilation to socket actions lives in compileAction (module).
    function lampCfg(a: RuleActionDraft, id: string) {
        return a.perLamp?.[id] ?? { state: "on" as const, level: a.level ?? 100, color: a.color ?? "" };
    }
    function setLamp(a: RuleActionDraft, id: string,
        patch: Partial<{ state: "on" | "off" | "ignore"; level: number; color: string }>) {
        if (!a.perLamp) return;
        a.perLamp[id] = { ...lampCfg(a, id), ...patch };
    }
    function enablePerLamp(a: RuleActionDraft) {
        if (a.perLamp) return;
        const pl: Record<string, { state: "on" | "off" | "ignore"; level: number; color: string }> = {};
        for (const m of membersOf(a)) pl[m.id] = { state: "on", level: a.level ?? 100, color: a.color ?? "" };
        a.perLamp = pl;
    }
    function setAllLamps(a: RuleActionDraft, state: "on" | "off" | "ignore") {
        for (const m of membersOf(a)) setLamp(a, m.id, { state });
    }
    function setAllLampLevel(a: RuleActionDraft, level: number) {
        if (isNaN(level)) return;
        for (const m of membersOf(a)) if (isSmart(m.protocol)) setLamp(a, m.id, { level, state: "on" });
    }
    function setAllLampColor(a: RuleActionDraft, color: string) {
        for (const m of membersOf(a)) if (isSmart(m.protocol)) setLamp(a, m.id, { color, state: "on" });
    }
    // Representative bulk values — the first smart member. After a bulk apply
    // every smart member matches, so this reflects the shared setting.
    function bulkLampLevel(a: RuleActionDraft): number {
        const f = membersOf(a).find(m => isSmart(m.protocol));
        return f ? lampCfg(a, f.id).level : 100;
    }
    function bulkLampColor(a: RuleActionDraft): string {
        const f = membersOf(a).find(m => isSmart(m.protocol));
        return f ? lampCfg(a, f.id).color : "";
    }
    function targetMissing(a: RuleActionDraft): boolean {
        return !targetsFor(a.target_type).some(t => t.id === a.target_id);
    }

    function addAction() {
        draft.actions = [...draft.actions, blankRuleAction()];
    }
    function removeAction(i: number) {
        draft.actions = draft.actions.filter((_, idx) => idx !== i);
    }
    function addCondition() {
        draft.conditions = [...draft.conditions,
            { type: "device", socket_id: v.sockets[0]?.id ?? "", state: "on" }];
    }
    function removeCondition(i: number) {
        draft.conditions = draft.conditions.filter((_, idx) => idx !== i);
    }
</script>

<!-- WHEN -->
<div class="block when">
    <div class="block-head"><span class="tag cool">When</span></div>
    <Segmented name="{idPrefix}-trigtype" bind:value={draft.trigType}
        options={[
            { value: "time",   label: "Time" },
            { value: "sensor", label: "Sensor", disabled: v.sensors.length === 0 },
            { value: "device", label: "Device", disabled: v.sockets.length === 0 },
        ]} />

    {#if draft.trigType === "time"}
        <div class="field mt">
            <Segmented name="{idPrefix}-timemode" bind:value={draft.trigTimeMode}
                options={[
                    { value: "fixed",   label: "Fixed" },
                    { value: "sunrise", label: "Sunrise" },
                    { value: "sunset",  label: "Sunset" },
                ]} />
        </div>
        {#if draft.trigTimeMode === "fixed"}
            <div class="field mt">
                <label for="{idPrefix}-time">Time</label>
                <input id="{idPrefix}-time" type="time" bind:value={draft.trigTime} />
            </div>
        {:else}
            <div class="field mt">
                <label for="{idPrefix}-solar">Offset</label>
                <input id="{idPrefix}-solar" type="range" min="-120" max="120" step="5"
                    bind:value={draft.trigSolarOffset} />
                <div class="solar-summary">{solarSummary(draft.trigTimeMode, draft.trigSolarOffset)}</div>
                {#if !hasLocation}
                    <div class="field-help warn">Set a location in Settings for solar triggers to fire.</div>
                {/if}
            </div>
        {/if}
        <div class="field mt">
            <span class="field-label">On days</span>
            <DayPicker bind:days={draft.trigDays} />
            <div class="field-help">Leave empty for every day.</div>
        </div>

    {:else if draft.trigType === "sensor"}
        <div class="field-row mt">
            <div class="field">
                <label for="{idPrefix}-sensor">Sensor</label>
                <select id="{idPrefix}-sensor" bind:value={draft.trigSensorId}>
                    {#each v.sensors as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                </select>
            </div>
            <div class="field">
                <label for="{idPrefix}-op">Crosses</label>
                <select id="{idPrefix}-op" bind:value={draft.trigOp}>
                    <option value="above">Above</option>
                    <option value="below">Below</option>
                </select>
            </div>
        </div>
        <div class="field mt">
            <label for="{idPrefix}-val">Threshold{sensorUnit ? ` (${sensorUnit})` : ""}</label>
            <input id="{idPrefix}-val" type="number" step="0.1" bind:value={draft.trigValue} />
        </div>

    {:else}
        <div class="field-row mt">
            <div class="field">
                <label for="{idPrefix}-dev">Device</label>
                <select id="{idPrefix}-dev" bind:value={draft.trigSocketId}>
                    {#each v.sockets as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                </select>
            </div>
            <div class="field">
                <label for="{idPrefix}-state">Turns</label>
                <select id="{idPrefix}-state" bind:value={draft.trigToState}>
                    <option value="on">On</option>
                    <option value="off">Off</option>
                </select>
            </div>
        </div>
    {/if}
</div>

<!-- ONLY IF (conditions) -->
<div class="block iff">
    <div class="block-head">
        <span class="tag">Only if</span>
        <button type="button" class="chip-sm" onclick={addCondition}>
            <Icon name="plus" size={12} /> Condition
        </button>
    </div>
    {#if draft.conditions.length === 0}
        <div class="field-help">Optional — without conditions the rule runs every time it triggers.</div>
    {/if}
    {#each draft.conditions as c, ci (c)}
        <div class="rowcard">
            <div class="field-row">
                <div class="field">
                    <select bind:value={c.type}>
                        <option value="device">Device is</option>
                        <option value="time_before">Time is before</option>
                        <option value="time_after">Time is after</option>
                        <option value="time_range">Time between</option>
                    </select>
                </div>
                {#if c.type === "device"}
                    <div class="field">
                        <select bind:value={c.socket_id}>
                            {#each v.sockets as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                        </select>
                    </div>
                {/if}
            </div>
            {#if c.type === "device"}
                <div class="field mt-sm">
                    <select bind:value={c.state}>
                        <option value="on">On</option>
                        <option value="off">Off</option>
                    </select>
                </div>
            {:else if c.type === "time_before"}
                <div class="field mt-sm">
                    <input type="time" bind:value={c.before} aria-label="Before time" />
                </div>
            {:else if c.type === "time_after"}
                <div class="field mt-sm">
                    <input type="time" bind:value={c.after} aria-label="After time" />
                </div>
            {:else}
                <div class="field-row mt-sm">
                    <div class="field"><input type="time" bind:value={c.after} aria-label="After" /></div>
                    <div class="field"><input type="time" bind:value={c.before} aria-label="Before" /></div>
                </div>
            {/if}
            <button type="button" class="row-remove"
                onclick={() => removeCondition(ci)}
                aria-label="Remove condition">
                <Icon name="trash" size={14} /> Remove
            </button>
        </div>
    {/each}
</div>

<!-- THEN -->
<div class="block then">
    <div class="block-head">
        <span class="tag on">Then</span>
        <button type="button" class="chip-sm" onclick={addAction}>
            <Icon name="plus" size={12} /> Action
        </button>
    </div>
    {#each draft.actions as a, ai (a)}
        <div class="rowcard">
            <div class="field-row">
                <div class="field">
                    <select bind:value={a.target_type} onchange={() => retarget(a)}>
                        <option value="socket" disabled={v.sockets.length === 0}>Device</option>
                        <option value="group"  disabled={v.groups.length === 0}>Group</option>
                        <option value="room"   disabled={v.rooms.length === 0}>Room</option>
                        <option value="scene"  disabled={v.scenes.length === 0}>Scene</option>
                    </select>
                </div>
                <div class="field">
                    <select bind:value={a.target_id}>
                        {#if a.target_id && targetMissing(a)}
                            <option value={a.target_id} disabled>(removed)</option>
                        {/if}
                        {#each targetsFor(a.target_type) as t (t.id)}<option value={t.id}>{t.label}</option>{/each}
                    </select>
                </div>
            </div>
            <div class="field mt-sm" style:opacity={a.target_type === "scene" ? 0.6 : 1}>
                <select bind:value={a.action} disabled={a.target_type === "scene"}>
                    {#if a.target_type === "scene"}
                        <option value="activate">Activate</option>
                    {:else}
                        <option value="on">Turn on</option>
                        <option value="off">Turn off</option>
                        <option value="toggle">Toggle</option>
                    {/if}
                </select>
            </div>
            {#if a.action === "on"}
                {#if a.target_type === "socket" && isSmart(v.sockets.find(s => s.id === a.target_id)?.protocol ?? "")}
                    <div class="action-light-row">
                        <div class="bright">
                            <span class="bright-ico"><Icon name="sun" size={14} /></span>
                            <input type="range" min="1" max="100" step="1" bind:value={a.level} aria-label="Brightness" />
                            <span class="bright-val mono">{a.level ?? 100}%</span>
                        </div>
                        <div class="swatches">
                            {#each COLOURS as c (c.name)}
                                <button type="button" class="swatch"
                                    class:active={(a.color ?? "") === c.hex}
                                    class:auto={c.hex === ""}
                                    style={c.hex ? `background:#${c.hex}` : ""}
                                    title={c.name}
                                    aria-label="{c.name} color"
                                    onclick={() => a.color = c.hex}>
                                    {#if c.hex === ""}<Icon name="close" size={12} />{/if}
                                </button>
                            {/each}
                        </div>
                    </div>
                {:else if a.target_type === "group" || a.target_type === "room"}
                    <div class="lamp-mode" role="group" aria-label="Lighting detail">
                        <button type="button" class="mode-btn" class:active={!a.perLamp}
                            aria-pressed={!a.perLamp}
                            onclick={() => a.perLamp = undefined}>All the same</button>
                        <button type="button" class="mode-btn" class:active={!!a.perLamp}
                            aria-pressed={!!a.perLamp}
                            onclick={() => enablePerLamp(a)}>Per lamp</button>
                    </div>
                    {#if !a.perLamp}
                        <div class="action-light-row">
                            <div class="bright">
                                <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                <input type="range" min="1" max="100" step="1" bind:value={a.level} aria-label="Brightness" />
                                <span class="bright-val mono">{a.level ?? 100}%</span>
                            </div>
                            <div class="preset-chips" role="group" aria-label="Lighting preset">
                                <button type="button" class="preset-chip auto"
                                    class:active={!a.color}
                                    title="No preset — turn on at previous brightness"
                                    aria-label="No lighting preset" aria-pressed={!a.color}
                                    onclick={() => { a.color = ""; a.level = 100; }}>
                                    —
                                </button>
                                {#each MATTER_PRESETS as p (p.label)}
                                    <button type="button" class="preset-chip"
                                        class:active={a.color === p.color}
                                        style="--pc: {p.cssColor}"
                                        title="{p.label} · {p.level}%"
                                        aria-label="{p.label} preset" aria-pressed={a.color === p.color}
                                        onclick={() => { a.level = p.level; a.color = p.color; }}>
                                        <span class="preset-dot" style="background:{p.cssColor}"></span>
                                        {p.label}
                                    </button>
                                {/each}
                            </div>
                        </div>
                    {:else}
                        {@const members = membersOf(a)}
                        {#if members.length === 0}
                            <div class="lamp-empty mono">No devices in this {a.target_type}</div>
                        {:else}
                            <div class="lamp-matrix">
                                <div class="bulk-bar">
                                    <span class="bulk-lbl">Set all<span class="bulk-n mono">{members.length}</span></span>
                                    <div class="state-group" role="group" aria-label="Set all lamps">
                                        <button type="button" class="state-btn"
                                            onclick={() => setAllLamps(a, 'ignore')}
                                            aria-label="Leave all lamps unchanged">—</button>
                                        <button type="button" class="state-btn s-on"
                                            onclick={() => setAllLamps(a, 'on')}
                                            aria-label="Turn all lamps on">On</button>
                                        <button type="button" class="state-btn s-off"
                                            onclick={() => setAllLamps(a, 'off')}
                                            aria-label="Turn all lamps off">Off</button>
                                    </div>
                                    {#if members.some(m => isSmart(m.protocol))}
                                        <div class="bulk-light">
                                            <div class="bright">
                                                <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                                <input type="range" min="1" max="100" step="1"
                                                    value={bulkLampLevel(a)}
                                                    oninput={(e) => setAllLampLevel(a, parseInt((e.target as HTMLInputElement).value, 10))}
                                                    aria-label="Brightness for all lamps" />
                                                <span class="bright-val mono">{bulkLampLevel(a)}%</span>
                                            </div>
                                            <div class="swatches">
                                                {#each COLOURS as c (c.name)}
                                                    <button type="button" class="swatch"
                                                        class:active={bulkLampColor(a) === c.hex}
                                                        class:auto={c.hex === ""}
                                                        style={c.hex ? `background:#${c.hex}` : ""}
                                                        title="{c.name} for all lamps"
                                                        aria-label="{c.name} for all lamps"
                                                        onclick={() => setAllLampColor(a, c.hex)}>
                                                        {#if c.hex === ""}<Icon name="close" size={12} />{/if}
                                                    </button>
                                                {/each}
                                            </div>
                                        </div>
                                    {/if}
                                </div>
                                <div class="lamp-rows">
                                    {#each members as m, mi (m.id)}
                                        {@const cfg = lampCfg(a, m.id)}
                                        {#if mi > 0}<div class="row-sep" aria-hidden="true"></div>{/if}
                                        <div class="lamp-row" class:row-on={cfg.state === 'on'}>
                                            <div class="lamp-main">
                                                <div class="row-bulb"
                                                    class:bulb-on={cfg.state === 'on'}
                                                    class:bulb-off={cfg.state === 'off'}
                                                    aria-hidden="true">
                                                    <Icon name="light" size={14} />
                                                </div>
                                                <div class="row-info">
                                                    <span class="row-name">{m.name}</span>
                                                    <span class="row-room">{m.room || "Unassigned"}</span>
                                                </div>
                                                <div class="state-group" role="group" aria-label="Action for {m.name}">
                                                    <button type="button" class="state-btn"
                                                        class:s-active={cfg.state === 'ignore'}
                                                        onclick={() => setLamp(a, m.id, { state: 'ignore' })}
                                                        aria-pressed={cfg.state === 'ignore'}
                                                        aria-label="Leave {m.name} unchanged">—</button>
                                                    <button type="button" class="state-btn s-on"
                                                        class:s-active={cfg.state === 'on'}
                                                        onclick={() => setLamp(a, m.id, { state: 'on' })}
                                                        aria-pressed={cfg.state === 'on'}
                                                        aria-label="Turn {m.name} on">On</button>
                                                    <button type="button" class="state-btn s-off"
                                                        class:s-active={cfg.state === 'off'}
                                                        onclick={() => setLamp(a, m.id, { state: 'off' })}
                                                        aria-pressed={cfg.state === 'off'}
                                                        aria-label="Turn {m.name} off">Off</button>
                                                </div>
                                            </div>
                                            {#if cfg.state === 'on' && isSmart(m.protocol)}
                                                <div class="light-row">
                                                    <div class="bright">
                                                        <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                                        <input type="range" min="1" max="100" step="1"
                                                            value={cfg.level}
                                                            oninput={(e) => setLamp(a, m.id, { level: parseInt((e.target as HTMLInputElement).value, 10) })}
                                                            aria-label="Brightness for {m.name}" />
                                                        <span class="bright-val mono">{cfg.level}%</span>
                                                    </div>
                                                    <div class="swatches">
                                                        {#each COLOURS as c (c.name)}
                                                            <button type="button" class="swatch"
                                                                class:active={cfg.color === c.hex}
                                                                class:auto={c.hex === ""}
                                                                style={c.hex ? `background:#${c.hex}` : ""}
                                                                title={c.name}
                                                                aria-label="{c.name} for {m.name}"
                                                                onclick={() => setLamp(a, m.id, { color: c.hex })}>
                                                                {#if c.hex === ""}<Icon name="close" size={12} />{/if}
                                                            </button>
                                                        {/each}
                                                    </div>
                                                </div>
                                            {/if}
                                        </div>
                                    {/each}
                                </div>
                            </div>
                        {/if}
                    {/if}
                {/if}
            {/if}
            {#if draft.actions.length > 1}
                <button type="button" class="row-remove"
                    onclick={() => removeAction(ai)}
                    aria-label="Remove action">
                    <Icon name="trash" size={14} /> Remove
                </button>
            {/if}
        </div>
    {/each}
</div>

<style>
    /* ── WHEN / ONLY-IF / THEN blocks ─────────────────────────────── */
    .block {
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .block.when { border-left: 3px solid var(--cool); }
    .block.iff  { border-left: 3px solid var(--border-strong); }
    .block.then { border-left: 3px solid var(--on); }
    .block-head { display: flex; align-items: center; justify-content: space-between; }
    .tag {
        font-family: var(--font-mono);
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-mute);
    }
    .tag.cool { color: var(--cool); }
    .tag.on   { color: var(--on); }
    .mt    { margin-top: var(--space-3); }
    .mt-sm { margin-top: var(--space-2); }

    .chip-sm {
        padding: 4px 10px;
        font-size: 12px;
        cursor: pointer;
        display: inline-flex;
        align-items: center;
        gap: 4px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-mute);
        transition: background var(--t-fast), color var(--t-fast);
    }
    .chip-sm:hover { background: var(--card-3); color: var(--text); }

    .rowcard {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-sm);
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .row-remove {
        align-self: flex-end;
        display: inline-flex; align-items: center; gap: 4px;
        background: none; border: 0; cursor: pointer;
        color: var(--text-mute); font-size: 12px; padding: 2px 4px;
        border-radius: var(--r-sm);
        transition: color var(--t-fast);
    }
    .row-remove:hover { color: var(--bad); }

    .action-light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding-top: var(--space-2);
    }

    /* ── Per-lamp matrix (group/room "on" actions) ───────────────────
       Mirrors the SceneModal snapshot picker so the two read identically. */
    .lamp-mode {
        display: inline-flex; gap: 1px; padding: 2px; margin-top: var(--space-2);
        background: var(--bg-elevated); border: 1px solid var(--border);
        border-radius: var(--r-pill); align-self: flex-start;
    }
    .mode-btn {
        padding: 5px 12px; border: none; background: transparent;
        font-size: 12px; font-weight: 500; color: var(--text-mute);
        border-radius: var(--r-pill); cursor: pointer; touch-action: manipulation;
        white-space: nowrap; line-height: 1;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .mode-btn:hover:not(.active) { color: var(--text); }
    .mode-btn.active { background: var(--card-3); color: var(--text); box-shadow: var(--shadow-sm); }

    .lamp-empty { padding: 14px 12px; text-align: center; font-size: 12px; color: var(--text-dim); }
    .lamp-matrix {
        display: flex; flex-direction: column; gap: 8px;
        margin-top: var(--space-2);
        border: 1px solid var(--hairline); border-radius: var(--r-sm);
        background: var(--card-2); padding: 6px;
    }
    .bulk-bar {
        display: flex; align-items: center; flex-wrap: wrap; gap: 10px;
        padding: 6px 8px; border-radius: var(--r-sm); background: var(--card-3);
    }
    .bulk-lbl {
        display: inline-flex; align-items: center; gap: 6px;
        font-family: var(--font-mono); font-size: 10.5px;
        text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-mute);
        flex-shrink: 0;
    }
    .bulk-n { font-size: 11px; color: var(--text-dim); background: var(--card-2); border-radius: var(--r-pill); padding: 0 6px; }
    .bulk-light { display: flex; align-items: center; gap: 12px; flex: 1; flex-wrap: wrap; min-width: 200px; }
    .bulk-light .bright { flex: 1; min-width: 140px; }

    .lamp-rows { display: flex; flex-direction: column; }
    .row-sep { height: 1px; background: var(--separator); margin: 0 10px 0 52px; }
    .lamp-row {
        display: flex; flex-direction: column;
        border-radius: var(--r-sm); overflow: hidden;
        transition: background var(--t-fast);
    }
    .lamp-row.row-on { background: var(--on-soft); }
    .lamp-main { display: flex; align-items: center; gap: 12px; padding: 8px 10px; min-height: 46px; }
    .row-bulb {
        width: 28px; height: 28px; border-radius: 50%;
        background: var(--card-3); display: grid; place-items: center;
        color: var(--text-dim); flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .row-bulb.bulb-on {
        background: var(--on); color: #1d180f;
        box-shadow: 0 0 0 1px var(--on), 0 0 14px 2px var(--on-glow);
    }
    .row-bulb.bulb-off { background: var(--bg-elevated); color: var(--text-dim); }
    .row-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .row-name { font-size: 13px; font-weight: 500; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .row-room { font-size: 11px; color: var(--text-mute); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

    .state-group {
        display: flex; background: var(--bg-elevated); border: 1px solid var(--border);
        border-radius: var(--r-pill); padding: 2px; gap: 1px; flex-shrink: 0;
    }
    .state-btn {
        padding: 5px 10px; border-radius: var(--r-pill); border: none;
        background: transparent; font-size: 12px; font-weight: 500;
        color: var(--text-mute); cursor: pointer; touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
        white-space: nowrap; line-height: 1;
    }
    .state-btn:hover:not(.s-active) { color: var(--text); }
    .state-btn.s-active { background: var(--card-3); color: var(--text); box-shadow: var(--shadow-sm); }
    .state-btn.s-on.s-active { background: var(--on-soft); color: var(--on); box-shadow: none; }

    .light-row { display: flex; flex-direction: column; gap: 8px; padding: 0 10px 10px 50px; }

    @media (prefers-reduced-motion: reduce) {
        .mode-btn, .state-btn, .row-bulb, .lamp-row { transition-duration: 0.001ms; }
    }
    @media (pointer: coarse) {
        .mode-btn, .state-btn { min-height: 34px; }
    }

    /* ── Matter preset chips (group/room actions) ─────────────────── */
    .preset-chips {
        display: flex;
        flex-wrap: wrap;
        gap: 5px;
    }
    .preset-chip {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 3px 9px;
        font-size: 12px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-mute);
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
        white-space: nowrap;
    }
    .preset-chip:hover { background: var(--card-3); color: var(--text); }
    .preset-chip.active {
        background: var(--card-3);
        color: var(--text);
        box-shadow: 0 0 0 1px var(--border-strong) inset;
    }
    .preset-chip.auto { color: var(--text-dim); font-size: 13px; }
    .preset-dot {
        width: 10px;
        height: 10px;
        border-radius: 50%;
        flex-shrink: 0;
        border: 1px solid rgba(255,255,255,0.15);
    }
    @media (pointer: coarse) {
        .preset-chip { padding: 6px 12px; font-size: 13px; min-height: 36px; }
        .preset-dot { width: 12px; height: 12px; }
    }
    .solar-summary {
        margin-top: 5px;
        font-weight: 600;
        font-size: 0.9rem;
        color: var(--text);
    }
    .field-help.warn { color: var(--warn, var(--danger)); }

    /* ── Smart-light controls ────────────────────────────────────── */
    .bright { display: flex; align-items: center; gap: 8px; }
    .bright-ico { color: var(--on); display: inline-flex; flex-shrink: 0; }
    .bright input[type="range"] { flex: 1; }
    .bright-val { font-size: 12px; color: var(--text-muted); min-width: 38px; text-align: right; }
    .swatches { display: flex; gap: 6px; flex-wrap: wrap; }
    .swatch {
        width: 24px; height: 24px; border-radius: 50%;
        border: 1px solid var(--hairline); cursor: pointer;
        display: grid; place-items: center; padding: 0;
        color: var(--text-muted); touch-action: manipulation;
        transition: box-shadow var(--t-fast);
    }
    .swatch.auto { background: var(--card-3); }
    .swatch.active { box-shadow: 0 0 0 2px var(--on), 0 0 0 4px var(--bg-elevated); }

    /* ── Mobile ──────────────────────────────────────────────────── */
    @media (max-width: 600px) {
        .swatch { width: 28px; height: 28px; }
        .bright input[type="range"] { height: 28px; }
    }
    @media (pointer: coarse) {
        input[type="range"] { height: 28px; }
        .swatch { width: 30px; height: 30px; }
    }
</style>

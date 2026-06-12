<script lang="ts" module>
    import { data } from "../lib/stores.svelte";
    import type { RuleActionDraft, RuleDraft, TargetType } from "../lib/types";

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
    import { isSmartProtocol } from "../lib/utils";

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
                    {@const activeIdx = MATTER_PRESETS.findIndex(p => p.color === a.color && p.level === a.level)}
                    <div class="action-light-row">
                        <div class="bright">
                            <span class="bright-ico"><Icon name="sun" size={14} /></span>
                            <input type="range" min="1" max="100" step="1" bind:value={a.level} aria-label="Brightness" />
                            <span class="bright-val mono">{a.level ?? 100}%</span>
                        </div>
                        <div class="preset-chips" role="group" aria-label="Lighting preset">
                            <button type="button" class="preset-chip auto"
                                class:active={activeIdx === -1}
                                title="No preset — turn on at previous brightness"
                                aria-label="No lighting preset" aria-pressed={activeIdx === -1}
                                onclick={() => { a.color = ""; a.level = 100; }}>
                                —
                            </button>
                            {#each MATTER_PRESETS as p, pi (p.label)}
                                <button type="button" class="preset-chip"
                                    class:active={activeIdx === pi}
                                    style="--pc: {p.cssColor}"
                                    title="{p.label} · {p.level}%"
                                    aria-label="{p.label} preset" aria-pressed={activeIdx === pi}
                                    onclick={() => { a.level = p.level; a.color = p.color; }}>
                                    <span class="preset-dot" style="background:{p.cssColor}"></span>
                                    {p.label}
                                </button>
                            {/each}
                        </div>
                    </div>
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

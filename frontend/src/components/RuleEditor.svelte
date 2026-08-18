<script lang="ts" generics="T extends RuleDraft">
    import Segmented from "./Segmented.svelte";
    import DayPicker from "./DayPicker.svelte";
    import Icon from "./Icon.svelte";
    import MusicActionRows from "./MusicActionRows.svelte";
    import LightRow from "./LightRow.svelte";
    import RuleLampMatrix from "./RuleLampMatrix.svelte";
    import { data } from "../lib/stores.svelte";
    import { isSmartProtocol } from "../lib/utils";
    import {
        membersOf, targetsFor, blankRuleAction,
    } from "../lib/rules";
    import type {
        RuleActionDraft, RuleDraft, AutomationCondition,
    } from "../lib/types";
    import { loadMediaRooms, type MediaRoomOption } from "../lib/media-rooms";
    import { onMount } from "svelte";

    interface Props {
        /** The rule being edited; the editor mutates it in place. */
        draft: T;
        /** Unique per instance — prefixes input ids and radio-group names. */
        idPrefix: string;
    }
    let { draft = $bindable(), idPrefix }: Props = $props();

    // The speakers and zones this rule can aim music at. Read once per
    // editor: the list changes when somebody adds a speaker, which doesn't
    // happen while a rule is being written.
    let rooms = $state<MediaRoomOption[]>([]);
    let roomsLoading = $state(true);
    let roomsFailed = $state(false);
    onMount(() => {
        void loadMediaRooms()
            .then((r) => {
                rooms = r;
                // A draft built before the list arrived has no room to point
                // at. Seeded here, once, rather than from an effect: a rule
                // being edited must not have its room rewritten under the
                // cursor if the list is re-read (same rule as retarget()).
                if (!draft.trigRoom) draft.trigRoom = r[0]?.key ?? "";
            })
            .catch(() => (roomsFailed = true))
            .finally(() => (roomsLoading = false));
    });

    const v = data.value;
    const isSmart = isSmartProtocol;

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
        normalizeAction(a);
    }

    // "Set colour / brightness" is only meaningful for a target that has smart
    // lights; a plain socket or an all-RF group/room can't accept it.
    function canSetLight(a: RuleActionDraft): boolean {
        if (a.target_type === "socket") {
            return isSmart(v.sockets.find(s => s.id === a.target_id)?.protocol ?? "");
        }
        if (a.target_type === "group" || a.target_type === "room") {
            return membersOf(a).some(m => isSmart(m.protocol));
        }
        return false;
    }
    // Keep the action valid after the target changes: a "set" that the new
    // target can't accept falls back to a plain "on".
    function normalizeAction(a: RuleActionDraft) {
        if (a.action === "set" && !canSetLight(a)) a.action = "on";
    }

    // Per-lamp authoring is RuleLampMatrix's; switching into it seeds every
    // member from the uniform setting already chosen, so the matrix opens as
    // "all the same" rather than as an empty form.
    function enablePerLamp(a: RuleActionDraft) {
        if (a.perLamp) return;
        const pl: Record<string, { state: "on" | "off" | "ignore"; level: number; color: string }> = {};
        for (const m of membersOf(a)) pl[m.id] = { state: "on", level: a.level ?? 100, color: a.color ?? "" };
        a.perLamp = pl;
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
    // Switching a condition's kind re-seeds the half that kind reads, so a
    // row never carries a room from the picker it was last on.
    function retype(c: AutomationCondition) {
        if (c.type === "device") {
            c.socket_id ||= v.sockets[0]?.id ?? "";
            if (c.state !== "on" && c.state !== "off") c.state = "on";
        } else if (c.type === "music") {
            c.room ||= rooms[0]?.key ?? "";
            if (c.state !== "playing" && c.state !== "stopped") c.state = "stopped";
        }
    }
    function removeCondition(i: number) {
        draft.conditions = draft.conditions.filter((_, idx) => idx !== i);
    }
</script>


<!-- WHEN -->
<div class="block when">
    <div class="block-head"><span class="tag cool">When</span></div>
    <!-- full: with a fourth option this control is ~330px of inline pill,
         which is wider than a sheet on a 360px phone. Sharing the width is
         what the variant is for (see Segmented's own note on Music's
         four-way subnav) and it costs the three-option case nothing. -->
    <Segmented name="{idPrefix}-trigtype" full bind:value={draft.trigType}
        options={[
            { value: "time",   label: "Time" },
            { value: "sensor", label: "Sensor", disabled: v.sensors.length === 0 },
            { value: "device", label: "Device", disabled: v.sockets.length === 0 },
            { value: "music",  label: "Music",  disabled: rooms.length === 0 },
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

    {:else if draft.trigType === "music"}
        <!-- The other half of what a room can be asked. A rule already drives
             the music (Then, below); this is where it can watch it, so "when
             the film ends in the living room" is a trigger rather than
             something you have to remember to press. -->
        {#if roomsLoading}
            <div class="field mt"><span class="skeleton room-sk"></span></div>
        {:else if roomsFailed}
            <div class="field-help mt">Couldn't read the speakers in this house.</div>
        {:else if rooms.length === 0}
            <div class="field-help mt">No speakers or zones in this house yet.</div>
        {:else}
            <div class="field-row mt">
                <div class="field">
                    <label for="{idPrefix}-room">Room</label>
                    <select id="{idPrefix}-room" bind:value={draft.trigRoom}>
                        {#if draft.trigRoom && !rooms.some(r => r.key === draft.trigRoom)}
                            <option value={draft.trigRoom} disabled>(removed)</option>
                        {/if}
                        {#each rooms as r (r.key)}
                            <option value={r.key}>{r.name}{r.kind === "zone" ? " (zone)" : ""}</option>
                        {/each}
                    </select>
                </div>
                <div class="field">
                    <label for="{idPrefix}-musicstate">Starts / stops</label>
                    <select id="{idPrefix}-musicstate" bind:value={draft.trigMusicState}>
                        <option value="stopped">Stops playing</option>
                        <option value="playing">Starts playing</option>
                    </select>
                </div>
            </div>
            <div class="field-help">
                Music or TV — whatever the room is making a sound with.
            </div>
        {/if}

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
                    <select bind:value={c.type} onchange={() => retype(c)}>
                        <option value="device">Device is</option>
                        <option value="music" disabled={rooms.length === 0}>Music is</option>
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
                {:else if c.type === "music"}
                    <div class="field">
                        <select bind:value={c.room} aria-label="Room">
                            {#if c.room && !rooms.some(r => r.key === c.room)}
                                <option value={c.room} disabled>(removed)</option>
                            {/if}
                            {#each rooms as r (r.key)}
                                <option value={r.key}>{r.name}{r.kind === "zone" ? " (zone)" : ""}</option>
                            {/each}
                        </select>
                    </div>
                {:else if c.type === "time_before"}
                    <div class="field">
                        <input type="time" bind:value={c.before} class="cond-time" aria-label="Before time" />
                    </div>
                {:else if c.type === "time_after"}
                    <div class="field">
                        <input type="time" bind:value={c.after} class="cond-time" aria-label="After time" />
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
            {:else if c.type === "music"}
                <div class="field mt-sm">
                    <select bind:value={c.state} aria-label="What the room is doing">
                        <option value="stopped">Not playing</option>
                        <option value="playing">Playing</option>
                    </select>
                </div>
            {:else if c.type === "time_range"}
                <div class="field-row mt-sm">
                    <div class="field">
                        <label for="{idPrefix}-cond-{ci}-after">From</label>
                        <input type="time" id="{idPrefix}-cond-{ci}-after" bind:value={c.after} class="cond-time" />
                    </div>
                    <div class="field">
                        <label for="{idPrefix}-cond-{ci}-before">To</label>
                        <input type="time" id="{idPrefix}-cond-{ci}-before" bind:value={c.before} class="cond-time" />
                    </div>
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
                    <select bind:value={a.target_id} onchange={() => normalizeAction(a)}>
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
                        {#if canSetLight(a)}
                            <option value="set">Set colour / brightness</option>
                        {/if}
                    {/if}
                </select>
            </div>
            {#if a.action === "on" || a.action === "set"}
                {#if a.target_type === "socket" && isSmart(v.sockets.find(s => s.id === a.target_id)?.protocol ?? "")}
                    <div class="action-light-row">
                        <LightRow
                            level={a.level ?? 100}
                            color={a.color ?? ""}
                            onLevel={(lvl) => (a.level = lvl)}
                            onPreset={(lvl, col) => { a.level = lvl; a.color = col; }}
                        />
                    </div>
                {:else if a.target_type === "group" || a.target_type === "room"}
                    {#if a.action === "set"}
                        <!-- Brightness/colour only — applied uniformly to every
                             smart member, with no per-lamp on/off matrix. -->
                        {@const members = membersOf(a)}
                        {#if members.some(m => isSmart(m.protocol))}
                            <div class="action-light-row">
                                <LightRow
                                    level={a.level ?? 100}
                                    color={a.color ?? ""}
                                    onLevel={(lvl) => (a.level = lvl)}
                                    onPreset={(lvl, col) => { a.level = lvl; a.color = col; }}
                                />
                            </div>
                        {:else}
                            <div class="lamp-empty mono">No smart lights in this {a.target_type}</div>
                        {/if}
                    {:else}
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
                                <LightRow
                                    level={a.level ?? 100}
                                    color={a.color ?? ""}
                                    onLevel={(lvl) => (a.level = lvl)}
                                    onPreset={(lvl, col) => { a.level = lvl; a.color = col; }}
                                />
                            </div>
                        {:else}
                            <RuleLampMatrix bind:action={draft.actions[ai]} />
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

    <!-- The other half of the house, under the same Then. A rule that turns
         everything off and leaves a speaker playing to an empty room was the
         gap this closes; the rows are the same ones a scene step gets. -->
    <div class="then-music">
        <span class="music-lbl">Music</span>
        <MusicActionRows
            bind:music={draft.music}
            {rooms}
            loading={roomsLoading}
            failed={roomsFailed}
            idPrefix="{idPrefix}-music"
        />
    </div>
</div>

<style>
    /* Music under the socket actions, behind its own rule: part of the same
       Then, not a fourth block that would need its own trigger. */
    .then-music {
        display: flex;
        flex-direction: column;
        gap: 8px;
        margin-top: 10px;
        padding-top: 10px;
        border-top: 1px solid var(--hairline);
    }
    /* The speaker list is a network read, so the room picker gets the
       skeleton primitive rather than an empty select somebody taps at. */
    .room-sk {
        display: block;
        height: 14px;
        max-width: 180px;
        border-radius: var(--r-sm);
    }

    .music-lbl {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

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
    @media (prefers-reduced-motion: reduce) {
        .mode-btn { transition-duration: 0.001ms; }
    }
    @media (pointer: coarse) {
        .mode-btn { min-height: 34px; }
    }

    /* Time inputs inside condition cards — mono face so digits align */
    .cond-time {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        letter-spacing: 0.02em;
        text-align: center;
    }

    .solar-summary {
        margin-top: 5px;
        font-weight: 600;
        font-size: 0.9rem;
        color: var(--text);
    }
    .field-help.warn { color: var(--warn, var(--danger)); }

    /* ── Mobile ──────────────────────────────────────────────────── */
    @media (pointer: coarse) {
        input[type="range"] { height: 28px; }
    }
</style>

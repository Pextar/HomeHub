<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Segmented from "../components/Segmented.svelte";
    import DayPicker from "../components/DayPicker.svelte";
    import Switch from "../components/Switch.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { isSmartProtocol } from "../lib/utils";
    import { untrack } from "svelte";
    import type {
        Automation, AutomationTrigger, AutomationCondition, AutomationAction,
        AutomationTriggerType, TargetType,
    } from "../lib/types";

    const COLOURS: { hex: string; name: string }[] = [
        { hex: "", name: "Auto" },
        { hex: "f5bd6e", name: "Warm" },
        { hex: "ffe9c4", name: "Soft" },
        { hex: "ffffff", name: "Bright" },
        { hex: "c4a4e0", name: "Lilac" },
        { hex: "7aa4d9", name: "Cool" },
    ];
    const isSmart = isSmartProtocol;

    interface Props { existing?: Automation | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const v = data.value;

    const firstTargetType = (): TargetType =>
        v.sockets.length ? "socket" : v.groups.length ? "group" : "scene";

    // ── Trigger state ────────────────────────────────────────
    let triggerType = $state<AutomationTriggerType>(untrack(() => existing?.trigger.type ?? "time"));
    let timeMode = $state(untrack(() => existing?.trigger.time_mode ?? "fixed"));
    let time = $state(untrack(() => existing?.trigger.time || "07:00"));
    let solarOffset = $state<number>(untrack(() => existing?.trigger.solar_offset_minutes ?? 0));
    let days = $state<number[]>(untrack(() => [...(existing?.trigger.days ?? [])]));
    let sensorId = $state(untrack(() => existing?.trigger.sensor_id ?? v.sensors[0]?.id ?? ""));
    let op = $state(untrack(() => existing?.trigger.op ?? "below"));
    let value = $state<number>(untrack(() => existing?.trigger.value ?? 20));
    let triggerSocketId = $state(untrack(() => existing?.trigger.socket_id ?? v.sockets[0]?.id ?? ""));
    let toState = $state(untrack(() => existing?.trigger.to_state ?? "on"));

    // ── Conditions / actions / meta ──────────────────────────
    let conditions = $state<AutomationCondition[]>(untrack(() =>
        (existing?.conditions ?? []).map(c => ({ ...c })),
    ));
    // Named thenActions (not "actions") to avoid colliding with the Modal's
    // {#snippet actions()} footer, which would shadow it in template scope.
    let thenActions = $state<AutomationAction[]>(untrack(() => {
        const ex = existing?.actions;
        return ex && ex.length
            ? ex.map((a): AutomationAction => ({ ...a, level: a.level ?? 100, color: a.color ?? "" }))
            : [{ target_type: firstTargetType(), target_id: "", action: "on", level: 100, color: "" } as AutomationAction];
    }));
    let name = $state(untrack(() => existing?.name ?? ""));
    let enabled = $state(untrack(() => existing ? existing.enabled : true));
    let saving = $state(false);
    let error = $state("");

    const hasLocation = $derived(v.settings.latitude !== 0 || v.settings.longitude !== 0);
    const selectedSensor = $derived(v.sensors.find(s => s.id === sensorId));

    function targetsFor(type: string) {
        if (type === "socket") return v.sockets.map(s => ({ id: s.id, label: s.name }));
        if (type === "group") return v.groups.map(g => ({ id: g.id, label: g.name }));
        return v.scenes.map(s => ({ id: s.id, label: s.name }));
    }
    // Seed any action whose target_id is empty/stale with the first valid id.
    $effect(() => {
        for (const a of thenActions) {
            const opts = targetsFor(a.target_type);
            if (!opts.find(o => o.id === a.target_id)) a.target_id = opts[0]?.id ?? "";
            if (a.target_type === "scene") a.action = "activate";
            else if (a.action === "activate") a.action = "on";
        }
    });

    function addAction() {
        thenActions = [...thenActions, { target_type: firstTargetType(), target_id: "", action: "on", level: 100, color: "" }];
    }
    function removeAction(i: number) {
        thenActions = thenActions.filter((_, idx) => idx !== i);
    }
    function addCondition() {
        conditions = [...conditions, { type: "device", socket_id: v.sockets[0]?.id ?? "", state: "on" }];
    }
    function removeCondition(i: number) {
        conditions = conditions.filter((_, idx) => idx !== i);
    }

    function buildPayload(): Partial<Automation> {
        let trigger: AutomationTrigger;
        if (triggerType === "time") {
            trigger = {
                type: "time",
                time_mode: timeMode as AutomationTrigger["time_mode"],
                time: timeMode === "fixed" ? time : "",
                solar_offset_minutes: timeMode === "fixed" ? 0 : solarOffset,
                days,
            };
        } else if (triggerType === "sensor") {
            trigger = { type: "sensor", sensor_id: sensorId, op: op as "above" | "below", value: Number(value) };
        } else {
            trigger = { type: "device", socket_id: triggerSocketId, to_state: toState as "on" | "off" };
        }
        return {
            name,
            enabled,
            trigger,
            conditions: conditions.map(c =>
                c.type === "device"
                    ? { type: "device", socket_id: c.socket_id, state: c.state }
                    : { type: "time_range", after: c.after, before: c.before },
            ),
            actions: thenActions.map(a => {
                const base = {
                    target_type: a.target_type,
                    target_id: a.target_id,
                    action: a.target_type === "scene" ? "activate" : a.action,
                };
                if (a.target_type === "socket" && a.action === "on") {
                    return { ...base, level: a.level ?? 100, color: a.color ?? "" };
                }
                return base;
            }),
        };
    }

    async function save() {
        if (saving) return;
        if (!name.trim()) { error = "Give the automation a name."; return; }
        error = "";
        saving = true;
        try {
            if (existing) {
                await api.updateAutomation(existing.id, buildPayload());
                toasts.success("Automation updated");
            } else {
                await api.createAutomation(buildPayload());
                toasts.success("Automation created");
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            error = (e as Error).message;
            toasts.error("Save failed", error);
        } finally {
            saving = false;
        }
    }

    let running = $state(false);
    async function runNow() {
        if (!existing || running) return; // double-click must not fire twice
        running = true;
        try {
            await api.runAutomation(existing.id);
            toasts.success("Automation ran", existing.name);
            await data.refresh();
        } catch (e) {
            toasts.error("Run failed", (e as Error).message);
        } finally {
            running = false;
        }
    }
</script>

<Modal title={isEdit ? "Edit automation" : "New automation"}
    subtitle="When something happens, optionally check conditions, then run actions.">
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="auto-name">Name</label>
                <input id="auto-name" type="text" bind:value={name} placeholder="Sunset porch" maxlength="60"
                    aria-invalid={error && !name.trim() ? "true" : undefined} />
            </div>

            <!-- ── WHEN ──────────────────────────────────── -->
            <div class="block when">
                <div class="block-head"><span class="tag cool">When</span></div>
                <Segmented name="auto-trigger" bind:value={triggerType}
                    options={[
                        { value: "time",   label: "Time" },
                        { value: "sensor", label: "Sensor", disabled: v.sensors.length === 0 },
                        { value: "device", label: "Device", disabled: v.sockets.length === 0 },
                    ]} />

                {#if triggerType === "time"}
                    <div class="field mt">
                        <Segmented name="auto-timemode" bind:value={timeMode}
                            options={[
                                { value: "fixed",   label: "Fixed" },
                                { value: "sunrise", label: "Sunrise" },
                                { value: "sunset",  label: "Sunset" },
                            ]} />
                    </div>
                    {#if timeMode === "fixed"}
                        <div class="field mt">
                            <label for="auto-time">Time</label>
                            <input id="auto-time" type="time" bind:value={time} required />
                        </div>
                    {:else}
                        <div class="field mt">
                            <label for="auto-solar">Offset (minutes)</label>
                            <input id="auto-solar" type="range" min="-120" max="120" step="5" bind:value={solarOffset} />
                            <div class="field-help">{solarOffset === 0 ? `At ${timeMode}` : `${Math.abs(solarOffset)} min ${solarOffset < 0 ? "before" : "after"} ${timeMode}`}</div>
                            {#if !hasLocation}<div class="field-help warn">Set a location in Settings for solar triggers to fire.</div>{/if}
                        </div>
                    {/if}
                    <div class="field mt">
                        <span class="field-label">On days</span>
                        <DayPicker bind:days={days} />
                        <div class="field-help">Leave empty for every day.</div>
                    </div>
                {:else if triggerType === "sensor"}
                    <div class="field-row mt">
                        <div class="field">
                            <label for="auto-sensor">Sensor</label>
                            <select id="auto-sensor" bind:value={sensorId}>
                                {#each v.sensors as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                            </select>
                        </div>
                        <div class="field">
                            <label for="auto-op">Crosses</label>
                            <select id="auto-op" bind:value={op}>
                                <option value="above">Above</option>
                                <option value="below">Below</option>
                            </select>
                        </div>
                    </div>
                    <div class="field mt">
                        <label for="auto-value">Threshold{selectedSensor?.unit ? ` (${selectedSensor.unit})` : ""}</label>
                        <input id="auto-value" type="number" step="0.1" bind:value={value} />
                    </div>
                {:else}
                    <div class="field-row mt">
                        <div class="field">
                            <label for="auto-dev">Device</label>
                            <select id="auto-dev" bind:value={triggerSocketId}>
                                {#each v.sockets as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                            </select>
                        </div>
                        <div class="field">
                            <label for="auto-tostate">Turns</label>
                            <select id="auto-tostate" bind:value={toState}>
                                <option value="on">On</option>
                                <option value="off">Off</option>
                            </select>
                        </div>
                    </div>
                {/if}
            </div>

            <!-- ── IF (conditions) ───────────────────────── -->
            <div class="block iff">
                <div class="block-head">
                    <span class="tag">If</span>
                    <button type="button" class="chip sm" onclick={addCondition}><Icon name="plus" size={12} /> Condition</button>
                </div>
                {#if conditions.length === 0}
                    <p class="field-help">No conditions — actions run every time the trigger fires.</p>
                {/if}
                {#each conditions as c, i (c)}
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
                        <button type="button" class="remove" onclick={() => removeCondition(i)} aria-label="Remove condition">
                            <Icon name="trash" size={14} /> Remove
                        </button>
                    </div>
                {/each}
            </div>

            <!-- ── THEN (actions) ────────────────────────── -->
            <div class="block then">
                <div class="block-head">
                    <span class="tag on">Then</span>
                    <button type="button" class="chip sm" onclick={addAction}><Icon name="plus" size={12} /> Action</button>
                </div>
                {#each thenActions as a, i (a)}
                    <div class="rowcard">
                        <div class="field-row">
                            <div class="field">
                                <select bind:value={a.target_type}>
                                    <option value="socket" disabled={v.sockets.length === 0}>Device</option>
                                    <option value="group" disabled={v.groups.length === 0}>Group</option>
                                    <option value="scene" disabled={v.scenes.length === 0}>Scene</option>
                                </select>
                            </div>
                            <div class="field">
                                <select bind:value={a.target_id}>
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
                        {#if a.target_type === "socket" && a.action === "on" && isSmart(v.sockets.find(s => s.id === a.target_id)?.protocol ?? "")}
                            <div class="light-row">
                                <div class="bright">
                                    <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                    <input type="range" min="1" max="100" step="1"
                                        bind:value={a.level}
                                        aria-label="Brightness" />
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
                        {/if}
                        {#if thenActions.length > 1}
                            <button type="button" class="remove" onclick={() => removeAction(i)} aria-label="Remove action">
                                <Icon name="trash" size={14} /> Remove
                            </button>
                        {/if}
                    </div>
                {/each}
            </div>

            <label class="field enabled-row">
                <Switch bind:checked={enabled} ariaLabel="Enabled" />
                <span>Enabled</span>
            </label>

            {#if error}<div class="field-error">{error}</div>{/if}
        </form>
    {/snippet}
    {#snippet actions()}
        {#if isEdit}
            <button class="btn btn-ghost" onclick={runNow} disabled={running}>{running ? "Running…" : "Run now"}</button>
        {/if}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Create"}
        </button>
    {/snippet}
</Modal>

<style>
    form { display: flex; flex-direction: column; gap: var(--space-4); }
    .mt { margin-top: var(--space-3); }
    .mt-sm { margin-top: var(--space-2); }

    .block {
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .block.when { border-left: 3px solid var(--cool); }
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
    .tag.on { color: var(--on); }

    .chip.sm { padding: 4px 10px; font-size: 12px; cursor: pointer; }

    .rowcard {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-sm);
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .remove {
        align-self: flex-end;
        display: inline-flex; align-items: center; gap: 4px;
        background: none; border: 0; cursor: pointer;
        color: var(--text-mute); font-size: 12px; padding: 2px 4px;
    }
    .remove:hover { color: var(--bad); }

    .enabled-row { flex-direction: row; align-items: center; gap: 12px; }
    .field-help.warn { color: var(--warn, var(--danger)); }

    /* ── Smart-light controls ──────────────────────────────── */
    .light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding: var(--space-2) 0 0;
    }
    .bright { display: flex; align-items: center; gap: 8px; }
    .bright-ico { color: var(--on); display: inline-flex; flex-shrink: 0; }
    .bright input[type="range"] { flex: 1; }
    .bright-val { font-size: 12px; color: var(--text-mute); min-width: 38px; text-align: right; }
    .swatches { display: flex; gap: 6px; flex-wrap: wrap; }
    .swatch {
        width: 24px; height: 24px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        cursor: pointer;
        display: grid; place-items: center;
        padding: 0;
        color: var(--text-mute);
        touch-action: manipulation;
        transition: box-shadow var(--t-fast);
    }
    .swatch.auto { background: var(--card-3); }
    .swatch.active { box-shadow: 0 0 0 2px var(--on), 0 0 0 4px var(--bg); }

    @media (pointer: coarse) {
        input[type="range"] { height: 28px; }
        .swatch { width: 30px; height: 30px; }
    }
</style>

<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { COLOURS } from "../components/RuleEditor.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets, isSmartProtocol } from "../lib/utils";
    import { untrack } from "svelte";
    import type { Scene, SceneStep, SceneAccent } from "../lib/types";

    interface Props { existing?: Scene | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));
    const v = data.value;

    const isSmart = isSmartProtocol;

    // Scene tile identity. Icons come from the shared icon set; accent keys map
    // to design tokens so the tile stays theme-aware.
    const SCENE_ICONS = [
        "scenes", "light", "sun", "moon", "sunrise", "sunset",
        "bed", "couch", "utensils", "home", "power", "bolt", "clock", "star",
    ] as const;
    const SCENE_ACCENTS: { key: SceneAccent; token: string }[] = [
        { key: "amber",  token: "var(--on)" },
        { key: "cool",   token: "var(--cool)" },
        { key: "violet", token: "var(--p-matter)" },
        { key: "orange", token: "var(--p-rf)" },
        { key: "green",  token: "var(--good)" },
        { key: "gold",   token: "var(--p-mqtt)" },
    ];

    // ── Snapshot (manual activation) state ────────────────────────────
    type StepState = {
        delay_minutes: number;
        perSocket: Record<string, "ignore" | "on" | "off">;
        levels: Record<string, number>;
        colors: Record<string, string>;
    };

    function blankStepState(delay = 0): StepState {
        return {
            delay_minutes: delay,
            perSocket: Object.fromEntries(sockets.map(s => [s.id, "ignore" as const])),
            levels:    Object.fromEntries(sockets.map(s => [s.id, 100])),
            colors:    Object.fromEntries(sockets.map(s => [s.id, ""])),
        };
    }

    function stepFromSceneStep(step: SceneStep): StepState {
        const perSocket: Record<string, "ignore" | "on" | "off"> =
            Object.fromEntries(sockets.map(s => [s.id, "ignore" as const]));
        const levels: Record<string, number> =
            Object.fromEntries(sockets.map(s => [s.id, 100]));
        const colors: Record<string, string> =
            Object.fromEntries(sockets.map(s => [s.id, ""]));
        for (const a of step.actions) {
            perSocket[a.socket_id] = a.action;
            if (a.level != null) levels[a.socket_id] = a.level;
            if (a.color)         colors[a.socket_id] = a.color;
        }
        return { delay_minutes: step.delay_minutes, perSocket, levels, colors };
    }

    let steps = $state<StepState[]>(untrack(() => {
        if (existing?.steps?.length) return existing.steps.map(stepFromSceneStep);
        if (existing?.actions?.length) {
            const step = blankStepState(0);
            for (const a of existing.actions) {
                step.perSocket[a.socket_id] = a.action;
                if (a.level != null) step.levels[a.socket_id] = a.level;
                if (a.color)         step.colors[a.socket_id] = a.color;
            }
            return [step];
        }
        return [blankStepState(0)];
    }));

    // Auto-expand snapshot section if the existing scene has configured devices.
    const existingHasSnapshot = untrack(() =>
        !!(existing?.steps?.some(s => s.actions.length > 0) || existing?.actions?.length)
    );
    let snapshotOpen = $state(true);

    const snapshotDeviceCount = $derived(
        steps.flatMap(step => Object.values(step.perSocket).filter(v => v !== "ignore")).length
    );

    // ── Snapshot scope + bulk fill ────────────────────────────────────
    // The snapshot stores per-device values, but real edits usually start
    // from "set this whole room/group, then tweak a couple of lamps". The
    // scope chip narrows the visible device rows; the bulk controls write
    // every shown row in one tap. Scope is a view filter only — rows hidden
    // by it keep whatever they were already configured to.
    // Start a fresh snapshot scoped to the scene's room (the common "set up
    // this room's lamps" flow); when editing an existing snapshot, show All
    // so no already-configured device is hidden behind the filter on open.
    let scope = $state<string>(untrack(() =>
        (!existingHasSnapshot && existing?.room) ? `room:${existing.room}` : ""));

    const scopeOptions = $derived([
        { key: "", label: "All" },
        ...[...v.rooms].sort((a, b) => a.name.localeCompare(b.name))
            .map(r => ({ key: `room:${r.name}`, label: r.name })),
        ...v.groups.map(g => ({ key: `group:${g.id}`, label: g.name })),
    ]);

    const shownSockets = $derived.by(() => {
        if (scope.startsWith("room:")) {
            const rn = scope.slice(5);
            return sockets.filter(s => (s.room || "") === rn);
        }
        if (scope.startsWith("group:")) {
            const g = v.groups.find(x => x.id === scope.slice(6));
            const ids = new Set(g?.socket_ids ?? []);
            return sockets.filter(s => ids.has(s.id));
        }
        return sockets;
    });
    const shownHasSmart = $derived(shownSockets.some(s => isSmart(s.protocol)));

    // Representative bulk values — taken from the first shown smart light.
    // After a bulk apply every shown light matches, so this reflects the
    // shared setting back into the slider/swatch UI.
    function bulkLevel(step: StepState): number {
        const f = shownSockets.find(s => isSmart(s.protocol));
        return f ? step.levels[f.id] : 100;
    }
    function bulkColor(step: StepState): string {
        const f = shownSockets.find(s => isSmart(s.protocol));
        return f ? step.colors[f.id] : "";
    }
    function setAll(step: StepState, state: "ignore" | "on" | "off") {
        for (const s of shownSockets) step.perSocket[s.id] = state;
    }
    function setAllLevel(step: StepState, level: number) {
        if (isNaN(level)) return;
        for (const s of shownSockets) if (isSmart(s.protocol)) {
            step.levels[s.id] = level;
            step.perSocket[s.id] = "on";
        }
    }
    function setAllColor(step: StepState, hex: string) {
        for (const s of shownSockets) if (isSmart(s.protocol)) {
            step.colors[s.id] = hex;
            step.perSocket[s.id] = "on";
        }
    }
    // Count of devices configured in a step but hidden by the current scope,
    // so a narrowed view never looks like settings silently vanished.
    function hiddenConfigured(step: StepState): number {
        const shown = new Set(shownSockets.map(s => s.id));
        let n = 0;
        for (const [id, st] of Object.entries(step.perSocket))
            if (st !== "ignore" && !shown.has(id)) n++;
        return n;
    }

    function addStep() {
        const last = steps[steps.length - 1]?.delay_minutes ?? 0;
        steps = [...steps, blankStepState(last + 60)];
    }

    function removeStep(i: number) {
        steps = steps.filter((_, idx) => idx !== i);
    }

    // Fill a step from the live on/off state of every device, so a scene can be
    // built from "how the room looks right now" in one tap. Brightness/colour
    // aren't part of the base socket model, so existing per-device values are
    // left untouched.
    function captureCurrentState(stepIndex: number) {
        const step = steps[stepIndex];
        if (!step) return;
        for (const s of shownSockets) {
            step.perSocket[s.id] = s.state ? "on" : "off";
        }
        const n = shownSockets.length;
        toasts.info("Captured current state",
            `${n} device${n === 1 ? "" : "s"} set to their current on/off state`);
    }

    function buildSteps() {
        return steps.map(step => ({
            delay_minutes: step.delay_minutes,
            actions: Object.entries(step.perSocket)
                .filter(([, v]) => v !== "ignore")
                .map(([socket_id, action]) => {
                    const a: { socket_id: string; action: "on" | "off"; level?: number; color?: string } =
                        { socket_id, action: action as "on" | "off" };
                    const sock = sockets.find(s => s.id === socket_id);
                    if (action === "on" && sock && isSmart(sock.protocol)) {
                        a.level = step.levels[socket_id];
                        if (step.colors[socket_id]) a.color = step.colors[socket_id];
                    }
                    return a;
                }),
        })).filter(s => s.actions.length > 0);
    }

    // ── Form state ─────────────────────────────────────────────────────
    let name = $state(untrack(() => existing?.name ?? ""));
    let room = $state(untrack(() => existing?.room ?? ""));
    let icon = $state<string>(untrack(() => existing?.icon ?? ""));
    let color = $state<SceneAccent | "">(untrack(() => existing?.color ?? ""));
    let saving = $state(false);
    let testing = $state(false);
    let nameError = $state("");

    const modalTitle = $derived(isEdit ? "Edit scene" : "New scene");
    const modalSubtitle = "A saved lighting look you activate by tapping. For things that happen on a trigger, use Automations.";

    // ── Test ────────────────────────────────────────────────────────────
    // Runs the saved scene immediately so you can preview it. Only available
    // when editing — a brand-new scene must be saved first.
    async function testRun() {
        if (!existing || testing) return;
        testing = true;
        try {
            const res = await api.activateScene(existing.id);
            toasts.success("Scene activated",
                `${res.updated} device${res.updated === 1 ? "" : "s"} updated`);
            await data.refresh();
        } catch (e) {
            toasts.error("Test failed", (e as Error).message);
        } finally {
            testing = false;
        }
    }

    // ── Save ────────────────────────────────────────────────────────────
    async function save() {
        if (saving) return;
        nameError = name.trim() ? "" : "Give the scene a name.";
        if (nameError) return;

        saving = true;
        try {
            const sceneName = name.trim();
            // Snapshot steps are optional — send whatever is configured (may be []).
            const payload = { name: sceneName, room: room || undefined, icon: icon || undefined, color: color || undefined, steps: buildSteps() };

            if (isEdit) {
                await api.updateScene(existing!.id, payload);
            } else {
                await api.createScene(payload);
            }

            toasts.success(isEdit ? "Scene updated" : "Scene created", sceneName);
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }
</script>

<Modal title={modalTitle} subtitle={modalSubtitle} size="wide">
    {#snippet body()}
        <div class="scene-form">

            <!-- ── Name + Room ──────────────────────────────────── -->
            <div class="field-row">
                <div class="field" style="flex:2">
                    <label for="scn-name">Name</label>
                    <input id="scn-name" type="text" bind:value={name}
                        placeholder="e.g. Evening lighting" autocomplete="off"
                        aria-invalid={nameError ? "true" : undefined}
                        aria-describedby={nameError ? "scn-name-err" : undefined}
                        oninput={() => nameError = ""} />
                    {#if nameError}<div id="scn-name-err" class="field-error">{nameError}</div>{/if}
                </div>
                <div class="field" style="flex:1">
                    <label for="scn-room">Room <span class="opt">(optional)</span></label>
                    <select id="scn-room" bind:value={room}>
                        <option value="">No room</option>
                        {#each [...v.rooms].sort((a, b) => a.name.localeCompare(b.name)) as r (r.id)}
                            <option value={r.name}>{r.name}</option>
                        {/each}
                    </select>
                </div>
            </div>

            <!-- ── Icon + accent ────────────────────────────────── -->
            <div class="field-row identity-row">
                <div class="field" style="flex:2">
                    <span class="field-label">Icon <span class="opt">(optional)</span></span>
                    <div class="icon-picker" role="group" aria-label="Scene icon">
                        <button type="button" class="icon-opt"
                            class:active={icon === ""}
                            title="No icon"
                            aria-label="No icon" aria-pressed={icon === ""}
                            onclick={() => icon = ""}>
                            <Icon name="close" size={15} />
                        </button>
                        {#each SCENE_ICONS as ic (ic)}
                            <button type="button" class="icon-opt"
                                class:active={icon === ic}
                                title={ic}
                                aria-label="{ic} icon" aria-pressed={icon === ic}
                                onclick={() => icon = ic}>
                                <Icon name={ic} size={16} />
                            </button>
                        {/each}
                    </div>
                </div>
                <div class="field" style="flex:1">
                    <span class="field-label">Accent <span class="opt">(optional)</span></span>
                    <div class="accent-picker" role="group" aria-label="Scene accent">
                        <button type="button" class="accent-opt auto"
                            class:active={color === ""}
                            title="Auto"
                            aria-label="Auto accent" aria-pressed={color === ""}
                            onclick={() => color = ""}>
                            <Icon name="close" size={12} />
                        </button>
                        {#each SCENE_ACCENTS as a (a.key)}
                            <button type="button" class="accent-opt"
                                class:active={color === a.key}
                                style="background:{a.token}"
                                title={a.key}
                                aria-label="{a.key} accent" aria-pressed={color === a.key}
                                onclick={() => color = a.key}></button>
                        {/each}
                    </div>
                </div>
            </div>

            <!-- ── Snapshot (manual activation) ─────────────────── -->
            <div class="form-section">
                <button type="button" class="snapshot-toggle"
                    onclick={() => snapshotOpen = !snapshotOpen}
                    aria-expanded={snapshotOpen}>
                    <span class="snapshot-chevron" class:open={snapshotOpen}>
                        <Icon name="chevronDown" size={14} />
                    </span>
                    <span class="form-sec-label">Lights &amp; devices</span>
                    {#if !snapshotOpen && snapshotDeviceCount > 0}
                        <span class="snapshot-count mono">{snapshotDeviceCount} device{snapshotDeviceCount > 1 ? "s" : ""}</span>
                    {/if}
                </button>

                {#if snapshotOpen}
                    <div class="snapshot-body">
                        <div class="field-help snapshot-hint">
                            What this scene sets when you tap it. Pick a room, set them all, then tweak individual lamps. Add steps to ramp over time.
                        </div>
                        {#if scopeOptions.length > 1}
                            <div class="scope-chips" role="group" aria-label="Filter devices by room or group">
                                {#each scopeOptions as opt (opt.key)}
                                    <button type="button" class="scope-chip"
                                        class:active={scope === opt.key}
                                        aria-pressed={scope === opt.key}
                                        onclick={() => scope = opt.key}>{opt.label}</button>
                                {/each}
                            </div>
                        {/if}
                        <div class="steps-wrap">
                            {#each steps as step, i (step)}
                                <div class="step-card">
                                    <div class="step-header">
                                        <span class="step-badge">Step {i + 1}</span>
                                        {#if i === 0}
                                            <span class="step-when">Runs immediately</span>
                                        {:else}
                                            <div class="step-timing">
                                                <span class="timing-lbl">After</span>
                                                <input type="number" class="delay-input mono"
                                                    min="1" max="1440" step="1"
                                                    value={step.delay_minutes}
                                                    oninput={(e) => {
                                                        const v = parseInt((e.target as HTMLInputElement).value, 10);
                                                        if (!isNaN(v) && v >= 1) step.delay_minutes = v;
                                                    }}
                                                    aria-label="Delay in minutes for step {i + 1}" />
                                                <span class="timing-lbl">min</span>
                                            </div>
                                        {/if}
                                        <button type="button" class="step-capture"
                                            onclick={() => captureCurrentState(i)}
                                            title="Set every device to its current on/off state">
                                            <Icon name="bolt" size={13} /> Capture now
                                        </button>
                                        {#if i !== 0}
                                            <button type="button" class="remove-step"
                                                onclick={() => removeStep(i)}
                                                aria-label="Remove step {i + 1}">
                                                <Icon name="close" size={14} />
                                            </button>
                                        {/if}
                                    </div>

                                    {#if shownSockets.length > 0}
                                        <div class="bulk-bar">
                                            <span class="bulk-lbl">Set all<span class="bulk-n mono">{shownSockets.length}</span></span>
                                            <div class="state-group" role="group"
                                                aria-label="Set all shown devices in step {i + 1}">
                                                <button type="button" class="state-btn"
                                                    onclick={() => setAll(step, 'ignore')}
                                                    aria-label="Ignore all shown devices">—</button>
                                                <button type="button" class="state-btn s-on"
                                                    onclick={() => setAll(step, 'on')}
                                                    aria-label="Turn all shown devices on">On</button>
                                                <button type="button" class="state-btn s-off"
                                                    onclick={() => setAll(step, 'off')}
                                                    aria-label="Turn all shown devices off">Off</button>
                                            </div>
                                            {#if shownHasSmart}
                                                <div class="bulk-light">
                                                    <div class="bright">
                                                        <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                                        <input type="range" min="1" max="100" step="1"
                                                            value={bulkLevel(step)}
                                                            oninput={(e) => setAllLevel(step, parseInt((e.target as HTMLInputElement).value, 10))}
                                                            aria-label="Brightness for all shown lights" />
                                                        <span class="bright-val mono">{bulkLevel(step)}%</span>
                                                    </div>
                                                    <div class="swatches">
                                                        {#each COLOURS as c (c.name)}
                                                            <button type="button" class="swatch"
                                                                class:active={bulkColor(step) === c.hex}
                                                                class:auto={c.hex === ""}
                                                                style={c.hex ? `background:#${c.hex}` : ""}
                                                                title="{c.name} for all shown"
                                                                aria-label="{c.name} for all shown lights"
                                                                onclick={() => setAllColor(step, c.hex)}>
                                                                {#if c.hex === ""}<Icon name="close" size={12} />{/if}
                                                            </button>
                                                        {/each}
                                                    </div>
                                                </div>
                                            {/if}
                                        </div>
                                    {/if}

                                    <div class="picker">
                                        {#if shownSockets.length === 0}
                                            <div class="picker-empty mono">No devices in this scope</div>
                                        {/if}
                                        {#each shownSockets as s, si (s.id)}
                                            {@const state = step.perSocket[s.id]}
                                            {#if si > 0}<div class="row-sep" aria-hidden="true"></div>{/if}
                                            <div class="picker-row"
                                                class:row-on={state === 'on'}
                                                class:row-off={state === 'off'}>
                                                <div class="row-main">
                                                    <div class="row-bulb"
                                                        class:bulb-on={state === 'on'}
                                                        class:bulb-off={state === 'off'}
                                                        aria-hidden="true">
                                                        <Icon name="light" size={14} />
                                                    </div>
                                                    <div class="row-info">
                                                        <span class="row-name">{s.name}</span>
                                                        <span class="row-room">{s.room || "Unassigned"}</span>
                                                    </div>
                                                    <div class="state-group" role="group"
                                                        aria-label="Action for {s.name} in step {i + 1}">
                                                        <button type="button" class="state-btn"
                                                            class:s-active={state === 'ignore'}
                                                            onclick={() => step.perSocket[s.id] = 'ignore'}
                                                            aria-pressed={state === 'ignore'}
                                                            aria-label="Ignore {s.name}">—</button>
                                                        <button type="button" class="state-btn s-on"
                                                            class:s-active={state === 'on'}
                                                            onclick={() => step.perSocket[s.id] = 'on'}
                                                            aria-pressed={state === 'on'}
                                                            aria-label="Turn {s.name} on">On</button>
                                                        <button type="button" class="state-btn s-off"
                                                            class:s-active={state === 'off'}
                                                            onclick={() => step.perSocket[s.id] = 'off'}
                                                            aria-pressed={state === 'off'}
                                                            aria-label="Turn {s.name} off">Off</button>
                                                    </div>
                                                </div>
                                                {#if state === 'on' && isSmart(s.protocol)}
                                                    <div class="light-row">
                                                        <div class="bright">
                                                            <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                                            <input type="range" min="1" max="100" step="1"
                                                                bind:value={step.levels[s.id]}
                                                                aria-label="Brightness for {s.name}" />
                                                            <span class="bright-val mono">{step.levels[s.id]}%</span>
                                                        </div>
                                                        <div class="swatches">
                                                            {#each COLOURS as c (c.name)}
                                                                <button type="button" class="swatch"
                                                                    class:active={step.colors[s.id] === c.hex}
                                                                    class:auto={c.hex === ""}
                                                                    style={c.hex ? `background:#${c.hex}` : ""}
                                                                    title={c.name}
                                                                    aria-label="{c.name} for {s.name}"
                                                                    onclick={() => step.colors[s.id] = c.hex}>
                                                                    {#if c.hex === ""}<Icon name="close" size={12} />{/if}
                                                                </button>
                                                            {/each}
                                                        </div>
                                                    </div>
                                                {/if}
                                            </div>
                                        {/each}
                                        {#if hiddenConfigured(step) > 0}
                                            <div class="hidden-hint">
                                                <span class="mono">{hiddenConfigured(step)}</span> configured device{hiddenConfigured(step) === 1 ? "" : "s"} hidden by this filter
                                            </div>
                                        {/if}
                                    </div>
                                </div>
                            {/each}

                            <button type="button" class="add-dashed-btn" onclick={addStep}>
                                <Icon name="plus" size={15} /> Add another step
                            </button>

                            <div class="field-help" style="margin-top:6px">
                                Each step runs after its delay. Add steps to ramp brightness over time.
                            </div>
                        </div>
                    </div>
                {/if}
            </div>

        </div>
    {/snippet}

    {#snippet actions()}
        {#if isEdit}
            <button class="btn btn-ghost" onclick={testRun} disabled={testing || saving}
                title="Run the saved scene now to preview it">
                {testing ? "Testing…" : "Test"}
            </button>
        {/if}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? (isEdit ? "Saving…" : "Creating…") : (isEdit ? "Save" : "Create scene")}
        </button>
    {/snippet}
</Modal>

<style>
    .scene-form {
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
    }

    /* ── Section heads ────────────────────────────────────────────── */
    .form-section {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }
    .form-sec-label {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--text-mute);
    }

    /* ── Icon + accent pickers ───────────────────────────────────── */
    .identity-row { align-items: flex-start; }
    .icon-picker { display: flex; flex-wrap: wrap; gap: 6px; }
    .icon-opt {
        width: 34px; height: 34px;
        display: grid; place-items: center;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-sm);
        color: var(--text-mute);
        cursor: pointer; touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .icon-opt:hover { background: var(--card-3); color: var(--text); }
    .icon-opt.active {
        color: var(--on);
        border-color: rgba(245,189,110,0.35);
        box-shadow: 0 0 0 1px var(--on) inset;
    }
    .accent-picker { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
    .accent-opt {
        width: 26px; height: 26px; border-radius: 50%;
        border: 1px solid var(--hairline); padding: 0;
        cursor: pointer; touch-action: manipulation;
        display: grid; place-items: center; color: var(--text-mute);
        transition: box-shadow var(--t-fast);
    }
    .accent-opt.auto { background: var(--card-3); }
    .accent-opt.active { box-shadow: 0 0 0 2px var(--on), 0 0 0 4px var(--bg-elevated); }

    /* ── Capture-current-state ───────────────────────────────────── */
    .step-capture {
        display: inline-flex; align-items: center; gap: 4px;
        padding: 4px 10px; font-size: 12px;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill); color: var(--text-mute);
        cursor: pointer; touch-action: manipulation; white-space: nowrap;
        margin-left: auto;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .step-capture:hover { background: var(--card-3); color: var(--text); }
    .step-capture :global(svg) { color: var(--on); }

    /* ── Snapshot toggle ─────────────────────────────────────────── */
    .snapshot-toggle {
        display: flex;
        align-items: center;
        gap: 7px;
        background: none;
        border: none;
        cursor: pointer;
        padding: 6px 0;
        color: var(--text-mute);
        text-align: left;
        width: 100%;
    }
    .snapshot-toggle:hover .form-sec-label { color: var(--text); }
    .snapshot-chevron {
        display: inline-flex;
        color: var(--text-dim);
        transition: transform var(--t-fast);
        flex-shrink: 0;
    }
    .snapshot-chevron.open { transform: rotate(0deg); }
    .snapshot-chevron:not(.open) { transform: rotate(-90deg); }
    .snapshot-count {
        font-size: 11px;
        color: var(--text-mute);
        margin-left: auto;
    }

    .snapshot-body {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }
    .snapshot-hint { margin-bottom: 2px; }

    /* ── Scope filter + bulk fill ─────────────────────────────────── */
    .scope-chips { display: flex; flex-wrap: wrap; gap: 6px; }
    .scope-chip {
        padding: 4px 11px; font-size: 12px;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill); color: var(--text-mute);
        cursor: pointer; touch-action: manipulation; white-space: nowrap;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .scope-chip:hover { background: var(--card-3); color: var(--text); }
    .scope-chip.active {
        background: var(--card-3); color: var(--text);
        box-shadow: 0 0 0 1px var(--border-strong) inset;
    }
    .bulk-bar {
        display: flex; align-items: center; flex-wrap: wrap; gap: 10px;
        padding: 8px 12px; margin: 4px 4px 0;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-sm);
    }
    .bulk-lbl {
        display: inline-flex; align-items: center; gap: 6px;
        font-family: var(--font-mono); font-size: 10.5px;
        text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-mute);
        flex-shrink: 0;
    }
    .bulk-n {
        font-size: 11px; color: var(--text-dim);
        background: var(--card-3); border-radius: var(--r-pill); padding: 0 6px;
    }
    .bulk-light {
        display: flex; align-items: center; gap: 12px;
        flex: 1; flex-wrap: wrap; min-width: 220px;
    }
    .bulk-light .bright { flex: 1; min-width: 150px; }
    .picker-empty { padding: 16px 12px; text-align: center; font-size: 12px; color: var(--text-dim); }
    .hidden-hint { padding: 8px 12px 4px 58px; font-size: 11.5px; color: var(--text-dim); }
    @media (prefers-reduced-motion: reduce) {
        .scope-chip { transition-duration: 0.001ms; }
    }
    @media (pointer: coarse) {
        .scope-chip { min-height: 34px; padding: 7px 13px; }
    }

    /* ── Steps (inside snapshot) ────────────────────────────────── */
    .steps-wrap { display: flex; flex-direction: column; gap: 10px; }
    .step-card {
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        overflow: hidden;
        background: var(--surface);
    }
    .step-header {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 12px;
        background: var(--card-3);
        border-bottom: 1px solid var(--border);
        flex-wrap: wrap;
    }
    .step-badge {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--text-muted);
        flex-shrink: 0;
    }
    .step-when { font-size: 12px; color: var(--text-muted); flex: 1; }
    .step-timing { display: flex; align-items: center; gap: 5px; flex: 1; }
    .timing-lbl { font-size: 12px; color: var(--text-muted); white-space: nowrap; }
    .delay-input { width: 56px; padding: 3px 7px; text-align: right; border-radius: var(--radius-sm); font-size: 13px; }
    .remove-step {
        background: transparent; border: 0; padding: 4px; cursor: pointer;
        color: var(--text-muted); display: grid; place-items: center;
        border-radius: var(--radius-sm); margin-left: auto; flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .remove-step:hover { background: var(--surface-hover); color: var(--bad); }

    /* ── Device picker (inside steps) ────────────────────────────── */
    .picker { display: flex; flex-direction: column; padding: 4px; }
    .row-sep { height: 1px; background: var(--separator); margin: 0 12px 0 58px; }
    .picker-row {
        display: flex; flex-direction: column;
        border-radius: var(--radius-sm); overflow: hidden;
        transition: background var(--t-fast);
    }
    .picker-row.row-on { background: var(--primary-soft); }
    .row-main { display: flex; align-items: center; gap: 12px; padding: 10px 12px; min-height: 48px; }
    .row-bulb {
        width: 30px; height: 30px; border-radius: 50%;
        background: var(--card-3); display: grid; place-items: center;
        color: var(--text-faint); flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .row-bulb.bulb-on {
        background: var(--primary); color: var(--primary-fg);
        box-shadow: 0 0 0 1px var(--primary), 0 0 14px 2px var(--primary-glow);
    }
    .row-bulb.bulb-off { background: var(--surface-hover); color: var(--text-faint); }
    .row-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .row-name { font-size: 13.5px; font-weight: 500; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .row-room { font-size: 11.5px; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

    .state-group {
        display: flex; background: var(--bg-elevated); border: 1px solid var(--border);
        border-radius: var(--r-pill); padding: 2px; gap: 1px; flex-shrink: 0;
    }
    .state-btn {
        padding: 5px 10px; border-radius: var(--r-pill); border: none;
        background: transparent; font-size: 12px; font-weight: 500;
        color: var(--text-muted); cursor: pointer; touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
        white-space: nowrap; line-height: 1;
    }
    .state-btn:hover:not(.s-active) { background: var(--surface-hover); color: var(--text); }
    .state-btn.s-active { background: var(--card-3); color: var(--text); box-shadow: var(--shadow-sm); }
    .state-btn.s-on.s-active { background: var(--primary-soft); color: var(--primary); box-shadow: none; }

    /* ── Smart-light controls ────────────────────────────────────── */
    .light-row { display: flex; flex-direction: column; gap: 8px; padding: 0 12px 12px 54px; }
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

    /* ── Add (dashed) button — used for both rules and steps ─────── */
    .add-dashed-btn {
        display: inline-flex; align-items: center; gap: 6px;
        padding: 10px 14px; border: 1px dashed var(--border-strong);
        border-radius: var(--r-md); background: transparent;
        color: var(--text-muted); font-size: 13px; cursor: pointer;
        touch-action: manipulation; width: 100%; justify-content: center;
        transition: background var(--t-fast), color var(--t-fast), border-color var(--t-fast);
        min-height: 44px;
    }
    .add-dashed-btn:hover { background: var(--surface-hover); color: var(--text); border-color: var(--text-muted); }

    /* ── Reduced motion ───────────────────────────────────────────── */
    @media (prefers-reduced-motion: reduce) {
        .snapshot-chevron, .row-bulb, .picker-row, .state-btn,
        .add-dashed-btn { transition-duration: 0.001ms; }
    }

    .opt { color: var(--text-muted); font-weight: 400; font-size: 12px; }

    /* ── Mobile ──────────────────────────────────────────────────── */
    @media (max-width: 600px) {
        .row-main { min-height: 52px; padding: 10px; }
        .state-btn { padding: 7px 10px; font-size: 12px; min-height: 36px; }
        .delay-input { font-size: 16px; padding: 5px 8px; }
        .remove-step { min-width: 44px; min-height: 44px; }
        .add-dashed-btn { min-height: 48px; }
        .swatch { width: 28px; height: 28px; }
        .bright input[type="range"] { height: 28px; }
        .light-row { padding: 0 10px 12px 52px; }
    }
    @media (pointer: coarse) {
        input[type="range"] { height: 28px; }
        .swatch { width: 30px; height: 30px; }
        .state-btn { min-height: 34px; }
    }
</style>

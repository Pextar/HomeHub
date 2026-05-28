<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import Segmented from "../components/Segmented.svelte";
    import DayPicker from "../components/DayPicker.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets } from "../lib/utils";
    import { untrack } from "svelte";
    import type {
        Scene, SceneStep, AutomationTriggerType, AutomationTrigger,
        AutomationAction, TargetType, Automation,
    } from "../lib/types";

    interface Props { existing?: Scene | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));
    const v = data.value;

    const isSmart = (protocol: string) =>
        protocol === "tasmota" || protocol === "matter" || protocol === "matter-thread";

    const COLOURS: { hex: string; name: string }[] = [
        { hex: "", name: "Auto" },
        { hex: "f5bd6e", name: "Warm" },
        { hex: "ffe9c4", name: "Soft" },
        { hex: "ffffff", name: "Bright" },
        { hex: "c4a4e0", name: "Lilac" },
        { hex: "7aa4d9", name: "Cool" },
    ];

    // ── Scene step state ───────────────────────────────────────────────
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
        if (existing?.steps?.length) {
            return existing.steps.map(stepFromSceneStep);
        }
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

    let name = $state(untrack(() => existing?.name ?? ""));
    let saving = $state(false);
    let nameError = $state("");
    let stepsError = $state("");

    // ── Wizard state ───────────────────────────────────────────────────
    let wizardStep = $state(1); // 1 = scene content, 2 = rules

    // ── Rule draft types ───────────────────────────────────────────────
    type RuleActionDraft = {
        target_type: TargetType;
        target_id: string;
        action: string;
        level: number;
        color: string;
    };

    type RuleDraft = {
        _key: string;
        automationId: string; // "" = new; non-empty = existing automation ID to update
        trigType: AutomationTriggerType;
        trigTimeMode: string;
        trigTime: string;
        trigSolarOffset: number;
        trigDays: number[];
        trigSensorId: string;
        trigOp: "above" | "below";
        trigValue: number;
        trigSocketId: string;
        trigToState: "on" | "off";
        actions: RuleActionDraft[];
    };

    const hasLocation = $derived(v.settings.latitude !== 0 || v.settings.longitude !== 0);

    function firstTargetType(): TargetType {
        return v.sockets.length ? "socket" : v.groups.length ? "group" : "scene";
    }

    function targetsFor(type: string) {
        if (type === "socket") return v.sockets.map(s => ({ id: s.id, label: s.name }));
        if (type === "group") return v.groups.map(g => ({ id: g.id, label: g.name }));
        return v.scenes.map(s => ({ id: s.id, label: s.name }));
    }

    function solarSummary(mode: string, offset: number): string {
        const event = mode === "sunrise" ? "sunrise" : "sunset";
        if (offset === 0) return `At ${event}`;
        const abs = Math.abs(offset);
        const h = Math.floor(abs / 60);
        const mins = abs % 60;
        const parts = [h && `${h}h`, mins && `${mins}m`].filter(Boolean).join(" ");
        return `${parts} ${offset < 0 ? "before" : "after"} ${event}`;
    }

    function blankRule(): RuleDraft {
        return {
            _key: Math.random().toString(36).slice(2),
            automationId: "",
            trigType: "time",
            trigTimeMode: "fixed",
            trigTime: "07:00",
            trigSolarOffset: 0,
            trigDays: [],
            trigSensorId: v.sensors[0]?.id ?? "",
            trigOp: "below",
            trigValue: 20,
            trigSocketId: v.sockets[0]?.id ?? "",
            trigToState: "on",
            actions: [{ target_type: firstTargetType(), target_id: "", action: "on", level: 100, color: "" }],
        };
    }

    function ruleFromAutomation(a: Automation): RuleDraft {
        const t = a.trigger;
        return {
            _key: Math.random().toString(36).slice(2),
            automationId: a.id,
            trigType: t.type as AutomationTriggerType,
            trigTimeMode: t.time_mode ?? "fixed",
            trigTime: t.time || "07:00",
            trigSolarOffset: t.solar_offset_minutes ?? 0,
            trigDays: [...(t.days ?? [])],
            trigSensorId: t.sensor_id ?? v.sensors[0]?.id ?? "",
            trigOp: (t.op ?? "below") as "above" | "below",
            trigValue: t.value ?? 20,
            trigSocketId: t.socket_id ?? v.sockets[0]?.id ?? "",
            trigToState: (t.to_state ?? "on") as "on" | "off",
            actions: (a.actions ?? []).map(act => ({
                target_type: act.target_type as TargetType,
                target_id: act.target_id,
                action: act.action as string,
                level: act.level ?? 100,
                color: act.color ?? "",
            })),
        };
    }

    let rules = $state<RuleDraft[]>(untrack(() => {
        if (existing) {
            const owned = data.value.automations.filter(a => a.scene_id === existing!.id);
            if (owned.length) return owned.map(ruleFromAutomation);
        }
        return [];
    }));

    // Seed stale/missing target IDs whenever rules change.
    $effect(() => {
        for (const rule of rules) {
            for (const a of rule.actions) {
                const opts = targetsFor(a.target_type);
                if (!opts.find(o => o.id === a.target_id)) a.target_id = opts[0]?.id ?? "";
                if (a.target_type === "scene") a.action = "activate";
                else if (a.action === "activate") a.action = "on";
            }
        }
    });

    function addRule() {
        rules = [...rules, blankRule()];
    }

    function removeRule(i: number) {
        rules = rules.filter((_, idx) => idx !== i);
    }

    function addRuleAction(ri: number) {
        rules[ri].actions = [
            ...rules[ri].actions,
            { target_type: firstTargetType(), target_id: "", action: "on", level: 100, color: "" },
        ];
    }

    function removeRuleAction(ri: number, ai: number) {
        rules[ri].actions = rules[ri].actions.filter((_, idx) => idx !== ai);
    }

    // Dynamic modal title / subtitle per wizard step
    const modalTitle = $derived(
        wizardStep === 1
            ? (isEdit ? "Edit scene" : "New scene")
            : "Automated rules"
    );
    const modalSubtitle = $derived(
        wizardStep === 1
            ? (isEdit
                ? "Adjust device settings and timing for this scene."
                : "A scene can drive devices through multiple timed steps — even the same lamp at different dim levels.")
            : "Add rules that fire automatically. Each rule has its own trigger and device actions — they run independently."
    );

    // ── Step management ─────────────────────────────────────────────────
    function addStep() {
        const last = steps[steps.length - 1]?.delay_minutes ?? 0;
        steps = [...steps, blankStepState(last + 60)];
        stepsError = "";
    }

    function removeStep(i: number) {
        steps = steps.filter((_, idx) => idx !== i);
    }

    // ── Build steps payload ─────────────────────────────────────────────
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

    // ── Build rule payload ──────────────────────────────────────────────
    function buildRulePayload(rule: RuleDraft, sceneId: string, sceneName: string, idx: number): Partial<Automation> {
        let trigger: AutomationTrigger;
        if (rule.trigType === "time") {
            trigger = {
                type: "time",
                time_mode: rule.trigTimeMode as AutomationTrigger["time_mode"],
                time: rule.trigTimeMode === "fixed" ? rule.trigTime : "",
                solar_offset_minutes: rule.trigTimeMode === "fixed" ? 0 : rule.trigSolarOffset,
                days: rule.trigDays,
            };
        } else if (rule.trigType === "sensor") {
            trigger = {
                type: "sensor",
                sensor_id: rule.trigSensorId,
                op: rule.trigOp,
                value: Number(rule.trigValue),
            };
        } else {
            trigger = {
                type: "device",
                socket_id: rule.trigSocketId,
                to_state: rule.trigToState,
            };
        }
        const actions: AutomationAction[] = rule.actions.map(a => {
            const base: AutomationAction = {
                target_type: a.target_type,
                target_id: a.target_id,
                action: (a.target_type === "scene" ? "activate" : a.action) as AutomationAction["action"],
            };
            if (a.target_type === "socket" && a.action === "on") {
                base.level = a.level ?? 100;
                if (a.color) base.color = a.color;
            }
            return base;
        });
        return {
            name: `${sceneName} – rule ${idx + 1}`,
            enabled: true,
            trigger,
            conditions: [],
            actions,
            scene_id: sceneId,
        };
    }

    // ── Wizard navigation ───────────────────────────────────────────────
    function advanceToRules() {
        nameError = name.trim() ? "" : "Give the scene a name.";
        const builtSteps = buildSteps();
        stepsError = builtSteps.length === 0
            ? "Set at least one device to On or Off in any step."
            : "";
        if (nameError || stepsError) return;
        wizardStep = 2;
    }

    // ── Save ────────────────────────────────────────────────────────────
    async function save() {
        if (saving) return;

        nameError = name.trim() ? "" : "Give the scene a name.";
        const builtSteps = buildSteps();
        stepsError = builtSteps.length === 0
            ? "Set at least one device to On or Off in any step."
            : "";
        if (nameError || stepsError) {
            wizardStep = 1;
            return;
        }

        saving = true;
        try {
            const sceneName = name.trim();
            const payload = { name: sceneName, steps: builtSteps };

            let sceneId: string;
            if (isEdit) {
                await api.updateScene(existing!.id, payload);
                sceneId = existing!.id;
            } else {
                const created = await api.createScene(payload);
                sceneId = created.id;
            }

            // Delete automations owned by this scene that were removed
            const survivingIds = new Set(rules.map(r => r.automationId).filter(Boolean));
            for (const a of data.value.automations) {
                if (a.scene_id === sceneId && !survivingIds.has(a.id)) {
                    try { await api.deleteAutomation(a.id); } catch (_) { /* best-effort */ }
                }
            }

            // Create / update each rule
            let ruleSaveErrors = 0;
            for (let i = 0; i < rules.length; i++) {
                const rule = rules[i];
                const rp = buildRulePayload(rule, sceneId, sceneName, i);
                try {
                    if (rule.automationId) {
                        await api.updateAutomation(rule.automationId, rp);
                    } else {
                        await api.createAutomation(rp);
                    }
                } catch (_) {
                    ruleSaveErrors++;
                }
            }

            const ruleCount = rules.length;
            if (ruleSaveErrors > 0) {
                toasts.warn(
                    isEdit ? "Scene updated" : "Scene created",
                    `${ruleCount - ruleSaveErrors} of ${ruleCount} rules saved — check Automations for details.`
                );
            } else {
                toasts.success(
                    isEdit ? "Scene updated" : "Scene created",
                    ruleCount > 0
                        ? `${sceneName} · ${ruleCount} rule${ruleCount > 1 ? "s" : ""}`
                        : sceneName
                );
            }

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
        <!-- ── Wizard step indicator ─────────────────────────────── -->
        <div class="wizard-track" aria-label="Step {wizardStep} of 2">
            <div class="wiz-step" class:wiz-active={wizardStep === 1} class:wiz-done={wizardStep > 1}>
                <div class="wiz-dot">
                    {#if wizardStep > 1}
                        <Icon name="check" size={12} />
                    {:else}
                        <span class="mono">1</span>
                    {/if}
                </div>
                <span class="wiz-label">Scene</span>
            </div>
            <div class="wiz-line" class:wiz-filled={wizardStep > 1}></div>
            <div class="wiz-step" class:wiz-active={wizardStep === 2}>
                <div class="wiz-dot"><span class="mono">2</span></div>
                <span class="wiz-label">Rules</span>
            </div>
        </div>

        <!-- ── Step 1: Scene content ──────────────────────────────── -->
        {#if wizardStep === 1}
            <form onsubmit={(e) => { e.preventDefault(); advanceToRules(); }}>
                <div class="field">
                    <label for="scn-name">Name</label>
                    <input id="scn-name" type="text" bind:value={name}
                        placeholder="e.g. Evening lighting" autocomplete="off" required
                        aria-invalid={nameError ? "true" : undefined}
                        aria-describedby={nameError ? "scn-name-err" : undefined}
                        oninput={() => nameError = ""} />
                    {#if nameError}<div id="scn-name-err" class="field-error">{nameError}</div>{/if}
                </div>

                <div class="steps-wrap" style="margin-top:var(--space-4)">
                    {#each steps as step, i (i)}
                        <div class="step-card">
                            <div class="step-header">
                                <span class="step-badge">Step {i + 1}</span>
                                {#if i === 0}
                                    <span class="step-when">Runs immediately</span>
                                {:else}
                                    <div class="step-timing">
                                        <span class="timing-lbl">After</span>
                                        <input
                                            type="number"
                                            class="delay-input mono"
                                            min="1"
                                            max="1440"
                                            step="1"
                                            value={step.delay_minutes}
                                            oninput={(e) => {
                                                const v = parseInt((e.target as HTMLInputElement).value, 10);
                                                if (!isNaN(v) && v >= 0) step.delay_minutes = v;
                                            }}
                                            aria-label="Delay in minutes for step {i + 1}"
                                        />
                                        <span class="timing-lbl">min</span>
                                    </div>
                                    <button type="button" class="remove-step"
                                        onclick={() => removeStep(i)}
                                        aria-label="Remove step {i + 1}">
                                        <Icon name="close" size={14} />
                                    </button>
                                {/if}
                            </div>

                            <div class="picker">
                                {#each sockets as s, si (s.id)}
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
                                                <button
                                                    type="button"
                                                    class="state-btn"
                                                    class:s-active={state === 'ignore'}
                                                    onclick={() => { step.perSocket[s.id] = 'ignore'; stepsError = ''; }}
                                                    aria-pressed={state === 'ignore'}
                                                    aria-label="Ignore {s.name} in step {i + 1}"
                                                >—</button>
                                                <button
                                                    type="button"
                                                    class="state-btn s-on"
                                                    class:s-active={state === 'on'}
                                                    onclick={() => { step.perSocket[s.id] = 'on'; stepsError = ''; }}
                                                    aria-pressed={state === 'on'}
                                                    aria-label="Turn {s.name} on in step {i + 1}"
                                                >On</button>
                                                <button
                                                    type="button"
                                                    class="state-btn s-off"
                                                    class:s-active={state === 'off'}
                                                    onclick={() => { step.perSocket[s.id] = 'off'; stepsError = ''; }}
                                                    aria-pressed={state === 'off'}
                                                    aria-label="Turn {s.name} off in step {i + 1}"
                                                >Off</button>
                                            </div>
                                        </div>
                                        {#if state === 'on' && isSmart(s.protocol)}
                                            <div class="light-row">
                                                <div class="bright">
                                                    <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                                    <input type="range" min="1" max="100" step="1"
                                                        bind:value={step.levels[s.id]}
                                                        aria-label="Brightness for {s.name} in step {i + 1}" />
                                                    <span class="bright-val mono">{step.levels[s.id]}%</span>
                                                </div>
                                                <div class="swatches">
                                                    {#each COLOURS as c (c.name)}
                                                        <button type="button" class="swatch"
                                                            class:active={step.colors[s.id] === c.hex}
                                                            class:auto={c.hex === ""}
                                                            style={c.hex ? `background:#${c.hex}` : ""}
                                                            title={c.name}
                                                            aria-label="{c.name} for {s.name} in step {i + 1}"
                                                            onclick={() => step.colors[s.id] = c.hex}>
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
                    {/each}

                    {#if stepsError}<div class="field-error steps-err">{stepsError}</div>{/if}

                    <button type="button" class="add-step-btn" onclick={addStep}>
                        <Icon name="plus" size={15} />
                        Add another step
                    </button>

                    <div class="field-help steps-hint">
                        Each step runs after its delay from when the scene is activated.
                        Add multiple steps to ramp a lamp's brightness over time.
                    </div>
                </div>
            </form>
        {/if}

        <!-- ── Step 2: Automated rules ────────────────────────────── -->
        {#if wizardStep === 2}
            <div class="rules-section">

                {#if rules.length === 0}
                    <div class="rules-empty">
                        <div class="rules-empty-icon"><Icon name="automation" size={28} /></div>
                        <div class="rules-empty-title">No rules yet</div>
                        <div class="rules-empty-sub">Rules fire automatically — each with its own trigger and device actions. The scene can still be activated manually from the Scenes screen.</div>
                    </div>
                {/if}

                {#each rules as rule, ri (rule._key)}
                    <div class="rule-card">
                        <div class="rule-header">
                            <span class="rule-badge">Rule {ri + 1}</span>
                            <button type="button" class="rule-remove"
                                onclick={() => removeRule(ri)}
                                aria-label="Remove rule {ri + 1}">
                                <Icon name="trash" size={14} />
                            </button>
                        </div>

                        <div class="rule-inner">
                            <!-- WHEN block -->
                            <div class="block when">
                                <div class="block-head"><span class="tag cool">When</span></div>
                                <Segmented name="rule-{ri}-trigtype" bind:value={rule.trigType}
                                    options={[
                                        { value: "time",   label: "Time" },
                                        { value: "sensor", label: "Sensor", disabled: v.sensors.length === 0 },
                                        { value: "device", label: "Device", disabled: v.sockets.length === 0 },
                                    ]} />

                                {#if rule.trigType === "time"}
                                    <div class="field mt">
                                        <Segmented name="rule-{ri}-timemode" bind:value={rule.trigTimeMode}
                                            options={[
                                                { value: "fixed",   label: "Fixed" },
                                                { value: "sunrise", label: "Sunrise" },
                                                { value: "sunset",  label: "Sunset" },
                                            ]} />
                                    </div>
                                    {#if rule.trigTimeMode === "fixed"}
                                        <div class="field mt">
                                            <label for="rule-{ri}-time">Time</label>
                                            <input id="rule-{ri}-time" type="time" bind:value={rule.trigTime} required />
                                        </div>
                                    {:else}
                                        <div class="field mt">
                                            <label for="rule-{ri}-solar">Offset</label>
                                            <input id="rule-{ri}-solar" type="range" min="-120" max="120" step="5"
                                                bind:value={rule.trigSolarOffset} />
                                            <div class="solar-summary">{solarSummary(rule.trigTimeMode, rule.trigSolarOffset)}</div>
                                            {#if !hasLocation}
                                                <div class="field-help warn">Set a location in Settings for solar triggers to fire.</div>
                                            {/if}
                                        </div>
                                    {/if}
                                    <div class="field mt">
                                        <span class="field-label">On days</span>
                                        <DayPicker bind:days={rule.trigDays} />
                                        <div class="field-help">Leave empty for every day.</div>
                                    </div>

                                {:else if rule.trigType === "sensor"}
                                    <div class="field-row mt">
                                        <div class="field">
                                            <label for="rule-{ri}-sensor">Sensor</label>
                                            <select id="rule-{ri}-sensor" bind:value={rule.trigSensorId}>
                                                {#each v.sensors as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                                            </select>
                                        </div>
                                        <div class="field">
                                            <label for="rule-{ri}-op">Crosses</label>
                                            <select id="rule-{ri}-op" bind:value={rule.trigOp}>
                                                <option value="above">Above</option>
                                                <option value="below">Below</option>
                                            </select>
                                        </div>
                                    </div>
                                    <div class="field mt">
                                        <label for="rule-{ri}-val">Threshold{v.sensors.find(s => s.id === rule.trigSensorId)?.unit ? ` (${v.sensors.find(s => s.id === rule.trigSensorId)?.unit})` : ""}</label>
                                        <input id="rule-{ri}-val" type="number" step="0.1" bind:value={rule.trigValue} />
                                    </div>

                                {:else}
                                    <div class="field-row mt">
                                        <div class="field">
                                            <label for="rule-{ri}-dev">Device</label>
                                            <select id="rule-{ri}-dev" bind:value={rule.trigSocketId}>
                                                {#each v.sockets as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
                                            </select>
                                        </div>
                                        <div class="field">
                                            <label for="rule-{ri}-state">Turns</label>
                                            <select id="rule-{ri}-state" bind:value={rule.trigToState}>
                                                <option value="on">On</option>
                                                <option value="off">Off</option>
                                            </select>
                                        </div>
                                    </div>
                                {/if}
                            </div>

                            <!-- THEN block -->
                            <div class="block then">
                                <div class="block-head">
                                    <span class="tag on">Then</span>
                                    <button type="button" class="chip sm"
                                        onclick={() => addRuleAction(ri)}>
                                        <Icon name="plus" size={12} /> Action
                                    </button>
                                </div>
                                {#each rule.actions as a, ai (ai)}
                                    <div class="rowcard">
                                        <div class="field-row">
                                            <div class="field">
                                                <select bind:value={a.target_type}>
                                                    <option value="socket" disabled={v.sockets.length === 0}>Device</option>
                                                    <option value="group"  disabled={v.groups.length === 0}>Group</option>
                                                    <option value="scene"  disabled={v.scenes.length === 0}>Scene</option>
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
                                            <div class="action-light-row">
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
                                        {#if rule.actions.length > 1}
                                            <button type="button" class="row-remove"
                                                onclick={() => removeRuleAction(ri, ai)}
                                                aria-label="Remove action">
                                                <Icon name="trash" size={14} /> Remove
                                            </button>
                                        {/if}
                                    </div>
                                {/each}
                            </div>
                        </div>
                    </div>
                {/each}

                <button type="button" class="add-rule-btn" onclick={addRule}>
                    <Icon name="plus" size={15} />
                    Add rule
                </button>
            </div>
        {/if}
    {/snippet}

    {#snippet actions()}
        {#if wizardStep === 1}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
            <button class="btn btn-primary" onclick={advanceToRules}>
                Next: Rules
                <span class="next-arrow" aria-hidden="true"><Icon name="chevronDown" size={15} /></span>
            </button>
        {:else}
            <button class="btn btn-ghost" onclick={() => wizardStep = 1}>← Back</button>
            <button class="btn btn-primary" onclick={save} disabled={saving}>
                {saving ? (isEdit ? "Saving…" : "Creating…") : (isEdit ? "Save" : "Create scene")}
            </button>
        {/if}
    {/snippet}
</Modal>

<style>
    /* ── Wizard step indicator ───────────────────────────────────── */
    .wizard-track {
        display: flex;
        align-items: center;
        gap: 0;
        margin-bottom: var(--space-5);
    }
    .wiz-step {
        display: flex;
        align-items: center;
        gap: 7px;
        flex-shrink: 0;
    }
    .wiz-dot {
        width: 26px;
        height: 26px;
        border-radius: 50%;
        border: 2px solid var(--border-strong);
        display: grid;
        place-items: center;
        color: var(--text-muted);
        transition: background var(--t-fast), border-color var(--t-fast), color var(--t-fast);
        flex-shrink: 0;
    }
    .wiz-dot .mono { font-size: 11px; font-weight: 700; }
    .wiz-active .wiz-dot {
        background: var(--primary);
        border-color: var(--primary);
        color: var(--primary-fg);
    }
    .wiz-done .wiz-dot {
        background: var(--primary-soft);
        border-color: var(--primary);
        color: var(--primary);
    }
    .wiz-label {
        font-size: 12px;
        color: var(--text-muted);
        font-weight: 500;
        transition: color var(--t-fast);
    }
    .wiz-active .wiz-label { color: var(--text); font-weight: 600; }
    .wiz-done .wiz-label { color: var(--primary); }
    .wiz-line {
        flex: 1;
        height: 2px;
        background: var(--border-strong);
        margin: 0 10px;
        border-radius: 1px;
        transition: background var(--t-med);
    }
    .wiz-line.wiz-filled { background: var(--primary); }

    /* ── Step cards ──────────────────────────────────────────────── */
    .steps-wrap {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }
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
    .step-when {
        font-size: 12px;
        color: var(--text-muted);
        flex: 1;
        min-width: 0;
    }
    .step-timing {
        display: flex;
        align-items: center;
        gap: 5px;
        flex: 1;
        min-width: 0;
    }
    .timing-lbl {
        font-size: 12px;
        color: var(--text-muted);
        white-space: nowrap;
    }
    .delay-input {
        width: 56px;
        padding: 3px 7px;
        text-align: right;
        border-radius: var(--radius-sm);
        font-size: 13px;
    }
    .remove-step {
        position: relative;
        background: transparent;
        border: 0;
        padding: 4px;
        cursor: pointer;
        color: var(--text-muted);
        display: grid;
        place-items: center;
        border-radius: var(--radius-sm);
        margin-left: auto;
        flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .remove-step:hover { background: var(--surface-hover); color: var(--bad); }

    /* ── Device picker ────────────────────────────────────────────── */
    .picker {
        display: flex;
        flex-direction: column;
        padding: 4px;
    }
    .row-sep {
        height: 1px;
        background: var(--separator);
        margin: 0 12px 0 58px;
    }
    .picker-row {
        display: flex;
        flex-direction: column;
        border-radius: var(--radius-sm);
        overflow: hidden;
        transition: background var(--t-fast);
    }
    .picker-row.row-on { background: var(--primary-soft); }
    .row-main {
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 10px 12px;
        min-height: 48px;
    }
    .row-bulb {
        width: 30px;
        height: 30px;
        border-radius: 50%;
        background: var(--card-3);
        display: grid;
        place-items: center;
        color: var(--text-faint);
        flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .row-bulb.bulb-on {
        background: var(--primary);
        color: var(--primary-fg);
        box-shadow: 0 0 0 1px var(--primary), 0 0 14px 2px var(--primary-glow);
    }
    .row-bulb.bulb-off {
        background: var(--surface-hover);
        color: var(--text-faint);
    }
    .row-info {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
    }
    .row-name {
        font-size: 13.5px;
        font-weight: 500;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .row-room {
        font-size: 11.5px;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* ── 3-state action control ──────────────────────────────────── */
    .state-group {
        display: flex;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--r-pill);
        padding: 2px;
        gap: 1px;
        flex-shrink: 0;
    }
    .state-btn {
        padding: 5px 10px;
        border-radius: var(--r-pill);
        border: none;
        background: transparent;
        font-size: 12px;
        font-weight: 500;
        color: var(--text-muted);
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
        white-space: nowrap;
        line-height: 1;
    }
    .state-btn:hover:not(.s-active) {
        background: var(--surface-hover);
        color: var(--text);
    }
    .state-btn.s-active {
        background: var(--card-3);
        color: var(--text);
        box-shadow: var(--shadow-sm);
    }
    .state-btn.s-on.s-active {
        background: var(--primary-soft);
        color: var(--primary);
        box-shadow: none;
    }

    /* ── Smart-light controls (scene steps) ──────────────────────── */
    .light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding: 0 12px 12px 54px;
    }
    .bright { display: flex; align-items: center; gap: 8px; }
    .bright-ico { color: var(--on); display: inline-flex; flex-shrink: 0; }
    .bright input[type="range"] { flex: 1; }
    .bright-val { font-size: 12px; color: var(--text-muted); min-width: 38px; text-align: right; }
    .swatches { display: flex; gap: 6px; flex-wrap: wrap; }
    .swatch {
        width: 24px; height: 24px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        cursor: pointer;
        display: grid; place-items: center;
        padding: 0;
        color: var(--text-muted);
        touch-action: manipulation;
        transition: box-shadow var(--t-fast);
    }
    .swatch.auto { background: var(--card-3); }
    .swatch.active { box-shadow: 0 0 0 2px var(--on), 0 0 0 4px var(--bg-elevated); }

    /* ── Add step button ──────────────────────────────────────────── */
    .add-step-btn {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 7px 14px;
        border: 1px dashed var(--border-strong);
        border-radius: var(--radius-md);
        background: transparent;
        color: var(--text-muted);
        font-size: 13px;
        cursor: pointer;
        touch-action: manipulation;
        margin-top: 4px;
        width: 100%;
        justify-content: center;
        transition: background var(--t-fast), color var(--t-fast), border-color var(--t-fast);
    }
    .add-step-btn:hover {
        background: var(--surface-hover);
        color: var(--text);
        border-color: var(--text-muted);
    }
    .steps-err  { margin-top: 6px; }
    .steps-hint { margin-top: 6px; }

    /* ── Rules section (step 2) ───────────────────────────────────── */
    .rules-section {
        display: flex;
        flex-direction: column;
        gap: 12px;
    }

    .rules-empty {
        display: flex;
        flex-direction: column;
        align-items: center;
        text-align: center;
        gap: 8px;
        padding: 32px 24px;
        border: 1px dashed var(--border);
        border-radius: var(--r-md);
    }
    .rules-empty-icon { color: var(--text-dim); }
    .rules-empty-title { font-size: 14px; font-weight: 500; color: var(--text-mute); }
    .rules-empty-sub {
        font-size: 12.5px;
        color: var(--text-dim);
        line-height: 1.5;
        max-width: 340px;
    }

    .rule-card {
        border: 1px solid var(--border);
        border-radius: var(--r-md);
        overflow: hidden;
    }
    .rule-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 8px 12px;
        background: var(--card-3);
        border-bottom: 1px solid var(--border);
    }
    .rule-badge {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--text-mute);
    }
    .rule-remove {
        background: transparent;
        border: 0;
        padding: 4px 6px;
        cursor: pointer;
        color: var(--text-dim);
        display: inline-flex;
        align-items: center;
        gap: 4px;
        border-radius: var(--r-sm);
        font-size: 12px;
        transition: background var(--t-fast), color var(--t-fast);
        min-width: 32px;
        min-height: 32px;
        justify-content: center;
    }
    .rule-remove:hover { background: var(--surface-hover); color: var(--bad); }

    .rule-inner {
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }

    /* ── Shared block styles (WHEN / THEN) ───────────────────────── */
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
    .tag.on   { color: var(--on); }
    .mt    { margin-top: var(--space-3); }
    .mt-sm { margin-top: var(--space-2); }

    .chip.sm {
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
    .chip.sm:hover { background: var(--card-3); color: var(--text); }

    /* ── Action row cards ────────────────────────────────────────── */
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

    /* ── Smart-light controls (rule actions) ─────────────────────── */
    .action-light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding-top: var(--space-2);
    }

    .solar-summary {
        margin-top: 5px;
        font-weight: 600;
        font-size: 0.9rem;
        color: var(--text);
    }
    .field-help.warn { color: var(--warn, var(--danger)); }

    /* ── Add rule button ──────────────────────────────────────────── */
    .add-rule-btn {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 10px 14px;
        border: 1px dashed var(--border-strong);
        border-radius: var(--r-md);
        background: transparent;
        color: var(--text-muted);
        font-size: 13px;
        cursor: pointer;
        touch-action: manipulation;
        width: 100%;
        justify-content: center;
        transition: background var(--t-fast), color var(--t-fast), border-color var(--t-fast);
        min-height: 44px;
    }
    .add-rule-btn:hover {
        background: var(--surface-hover);
        color: var(--text);
        border-color: var(--text-muted);
    }

    /* Rotated chevron on "Next" button */
    .next-arrow {
        display: inline-flex;
        transform: rotate(-90deg);
        margin-left: 4px;
    }

    /* ── Reduced motion ───────────────────────────────────────────── */
    @media (prefers-reduced-motion: reduce) {
        .wiz-dot, .wiz-line { transition-duration: 0.001ms; }
        .row-bulb, .picker-row, .state-btn { transition-duration: 0.001ms; }
        .rule-card, .add-rule-btn, .add-step-btn { transition-duration: 0.001ms; }
    }

    /* ── Mobile layout ───────────────────────────────────────────── */
    @media (max-width: 600px) {
        .row-main { min-height: 52px; padding: 10px; }
        .state-btn { padding: 7px 10px; font-size: 12px; min-height: 36px; }
        .delay-input { font-size: 16px; padding: 5px 8px; }
        .remove-step { min-width: 44px; min-height: 44px; }
        .add-step-btn { min-height: 44px; }
        .swatch { width: 28px; height: 28px; }
        .bright input[type="range"] { height: 28px; }
        .light-row { padding: 0 10px 12px 52px; }
        .rule-remove { min-width: 44px; min-height: 44px; }
    }
    @media (pointer: coarse) {
        input[type="range"] { height: 28px; }
        .swatch { width: 30px; height: 30px; }
        .state-btn { min-height: 34px; }
        .rule-remove { min-width: 44px; min-height: 44px; }
    }
</style>

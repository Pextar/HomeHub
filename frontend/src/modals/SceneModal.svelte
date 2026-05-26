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
        Scene, SceneStep, ScheduleTimeMode, AutomationTriggerType,
    } from "../lib/types";

    interface Props { existing?: Scene | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));
    const v = data.value;

    const isSmart = (protocol: string) =>
        protocol === "tasmota" || protocol === "matter" || protocol === "matter-thread";

    // Colour presets mirror the light-detail mockup; "" means leave colour as-is.
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
        // Legacy scene with flat actions
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

    // ── Wizard state (new scenes only) ─────────────────────────────────
    let wizardStep = $state(1); // 1 = scene content, 2 = activation method

    type ActivationMode = "manual" | "schedule" | "trigger";
    let activationMode = $state<ActivationMode>("manual");

    // ── Schedule activation fields ──────────────────────────────────────
    let schedTimeMode = $state<ScheduleTimeMode>("fixed");
    let schedTime = $state("08:00");
    let schedSolarOffset = $state(0);
    let schedDays = $state<number[]>([]);
    let schedRandomOffset = $state(0);

    // ── Trigger/automation activation fields ────────────────────────────
    let autoName = $state("");
    let trigType = $state<AutomationTriggerType>("time");
    let trigTimeMode = $state("fixed");
    let trigTime = $state("07:00");
    let trigSolarOffset = $state(0);
    let trigDays = $state<number[]>([]);
    let trigSensorId = $state(untrack(() => v.sensors[0]?.id ?? ""));
    let trigOp = $state<"above" | "below">("below");
    let trigValue = $state<number>(20);
    let trigSocketId = $state(untrack(() => v.sockets[0]?.id ?? ""));
    let trigToState = $state<"on" | "off">("on");

    const hasLocation = $derived(v.settings.latitude !== 0 || v.settings.longitude !== 0);

    const schedSolarLabel = $derived.by(() => {
        const m = schedSolarOffset;
        const event = schedTimeMode === "sunrise" ? "sunrise" : "sunset";
        if (m === 0) return `At ${event}`;
        const abs = Math.abs(m);
        const h = Math.floor(abs / 60);
        const mins = abs % 60;
        const parts = [h && `${h}h`, mins && `${mins}m`].filter(Boolean).join(" ");
        return `${parts} ${m < 0 ? "before" : "after"} ${event}`;
    });

    const trigSolarLabel = $derived.by(() => {
        const m = trigSolarOffset;
        const event = trigTimeMode === "sunrise" ? "sunrise" : "sunset";
        if (m === 0) return `At ${event}`;
        const abs = Math.abs(m);
        const h = Math.floor(abs / 60);
        const mins = abs % 60;
        const parts = [h && `${h}h`, mins && `${mins}m`].filter(Boolean).join(" ");
        return `${parts} ${m < 0 ? "before" : "after"} ${event}`;
    });

    // Dynamic modal title / subtitle per wizard step
    const modalTitle = $derived(
        isEdit ? "Edit scene" :
        wizardStep === 1 ? "New scene" :
        "When should it run?"
    );
    const modalSubtitle = $derived(
        isEdit
            ? "Adjust device settings and timing for this scene."
            : wizardStep === 1
            ? "A scene can drive devices through multiple timed steps — even the same lamp at different dim levels."
            : "Choose how this scene gets triggered. You can always add more in Schedules or Automations later."
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

    function stepLabel(i: number, delay: number): string {
        if (i === 0) return "Runs immediately";
        if (delay === 0) return "Also immediately";
        if (delay % 60 === 0) {
            const h = delay / 60;
            return `After ${h} ${h === 1 ? "hour" : "hours"}`;
        }
        if (delay < 60) return `After ${delay} min`;
        const h = Math.floor(delay / 60);
        const m = delay % 60;
        return `After ${h}h ${m}m`;
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

    // ── Wizard navigation ───────────────────────────────────────────────
    function advanceToActivation() {
        nameError = name.trim() ? "" : "Give the scene a name.";
        const builtSteps = buildSteps();
        stepsError = builtSteps.length === 0
            ? "Set at least one device to On or Off in any step."
            : "";
        if (nameError || stepsError) return;
        // Pre-fill automation name from scene name
        if (!autoName) autoName = `${name.trim()} trigger`;
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
            if (!isEdit) wizardStep = 1;
            return;
        }

        saving = true;
        try {
            const payload = { name: name.trim(), steps: builtSteps };

            if (isEdit) {
                await api.updateScene(existing!.id, payload);
                toasts.success("Scene updated", payload.name);
                closeModal();
                await data.refresh();
                return;
            }

            // New scene — create it first, then optionally create activation
            const created = await api.createScene(payload);
            const sceneId = created.id;

            try {
                if (activationMode === "schedule") {
                    await api.createSchedule({
                        target_type: "scene",
                        target_id: sceneId,
                        action: "activate",
                        time_mode: schedTimeMode,
                        time: schedTimeMode === "fixed" ? schedTime : "",
                        solar_offset_minutes: schedTimeMode === "fixed" ? 0 : schedSolarOffset,
                        days: schedDays,
                        enabled: true,
                        random_offset_minutes: schedRandomOffset,
                    });
                    toasts.success("Scene created", `${payload.name} · schedule added`);
                } else if (activationMode === "trigger") {
                    // Build trigger payload
                    let trigger: Record<string, unknown>;
                    if (trigType === "time") {
                        trigger = {
                            type: "time",
                            time_mode: trigTimeMode,
                            time: trigTimeMode === "fixed" ? trigTime : "",
                            solar_offset_minutes: trigTimeMode === "fixed" ? 0 : trigSolarOffset,
                            days: trigDays,
                        };
                    } else if (trigType === "sensor") {
                        trigger = {
                            type: "sensor",
                            sensor_id: trigSensorId,
                            op: trigOp,
                            value: Number(trigValue),
                        };
                    } else {
                        trigger = {
                            type: "device",
                            socket_id: trigSocketId,
                            to_state: trigToState,
                        };
                    }
                    // eslint-disable-next-line @typescript-eslint/no-explicit-any
                    await api.createAutomation({
                        name: autoName.trim() || `${payload.name} automation`,
                        enabled: true,
                        trigger: trigger as any,
                        conditions: [],
                        actions: [{ target_type: "scene", target_id: sceneId, action: "activate" }],
                    });
                    toasts.success("Scene created", `${payload.name} · automation added`);
                } else {
                    toasts.success("Scene created", payload.name);
                }
            } catch (_activationErr) {
                // Scene was created; activation setup failed. Surface a warning
                // so the user knows to add it separately.
                toasts.warn("Scene created", "Activation setup failed — add it later in Schedules or Automations.");
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
        <!-- ── Wizard step indicator (new scenes only) ─────────────── -->
        {#if !isEdit}
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
                    <span class="wiz-label">Activation</span>
                </div>
            </div>
        {/if}

        <!-- ── Step 1 / Edit mode: Scene content ──────────────────── -->
        {#if isEdit || wizardStep === 1}
            <form onsubmit={(e) => { e.preventDefault(); isEdit ? save() : advanceToActivation(); }}>
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

        <!-- ── Step 2: Activation method ──────────────────────────── -->
        {#if !isEdit && wizardStep === 2}
            <div class="act-section">

                <!-- Method cards -->
                <div class="act-cards" role="radiogroup" aria-label="Activation method">
                    <button
                        type="button"
                        class="act-card"
                        class:act-selected={activationMode === "manual"}
                        aria-pressed={activationMode === "manual"}
                        onclick={() => activationMode = "manual"}
                    >
                        <div class="act-icon act-icon-manual">
                            <Icon name="scenes" size={20} />
                        </div>
                        <div class="act-card-body">
                            <div class="act-title">Manually</div>
                            <div class="act-desc">Activate from the Scenes screen whenever you want</div>
                        </div>
                    </button>

                    <button
                        type="button"
                        class="act-card"
                        class:act-selected={activationMode === "schedule"}
                        aria-pressed={activationMode === "schedule"}
                        onclick={() => activationMode = "schedule"}
                    >
                        <div class="act-icon act-icon-schedule">
                            <Icon name="clock" size={20} />
                        </div>
                        <div class="act-card-body">
                            <div class="act-title">On a schedule</div>
                            <div class="act-desc">Runs automatically at set times or around sunrise/sunset</div>
                        </div>
                    </button>

                    <button
                        type="button"
                        class="act-card"
                        class:act-selected={activationMode === "trigger"}
                        aria-pressed={activationMode === "trigger"}
                        onclick={() => activationMode = "trigger"}
                    >
                        <div class="act-icon act-icon-trigger">
                            <Icon name="automation" size={20} />
                        </div>
                        <div class="act-card-body">
                            <div class="act-title">When triggered</div>
                            <div class="act-desc">Fires based on a sensor reading, device state, or a time event</div>
                        </div>
                    </button>
                </div>

                <!-- ── Schedule config ──────────────────────────── -->
                {#if activationMode === "schedule"}
                    <div class="act-config">
                        <div class="act-config-head">
                            <Icon name="clock" size={15} />
                            Schedule configuration
                        </div>

                        <div class="field">
                            <span class="field-label">When</span>
                            <Segmented name="scn-sched-mode" bind:value={schedTimeMode}
                                options={[
                                    { value: "fixed",   label: "Fixed time" },
                                    { value: "sunrise", label: "Sunrise" },
                                    { value: "sunset",  label: "Sunset" },
                                ]} />
                        </div>

                        {#if schedTimeMode === "fixed"}
                            <div class="field">
                                <label for="scn-sched-time">Time</label>
                                <input id="scn-sched-time" type="time" bind:value={schedTime} required />
                                <div class="field-help">24-hour HH:MM in the server's local time.</div>
                            </div>
                        {:else}
                            <div class="field">
                                <label for="scn-sched-solar">Offset</label>
                                <input id="scn-sched-solar" type="range" min="-120" max="120" step="5"
                                    bind:value={schedSolarOffset}
                                    aria-valuetext={schedSolarLabel} />
                                <div class="solar-summary">{schedSolarLabel}</div>
                                {#if !hasLocation}
                                    <div class="field-help warn">Set the controller's latitude/longitude in Settings — without a location, this schedule cannot fire.</div>
                                {:else}
                                    <div class="field-help">Drag to pick how far before (−) or after (+) the event to fire.</div>
                                {/if}
                            </div>
                        {/if}

                        <div class="field">
                            <span class="field-label">Days</span>
                            <DayPicker bind:days={schedDays} />
                            <div class="field-help">Leave empty to fire every day.</div>
                        </div>

                        <div class="field">
                            <label for="scn-sched-rand">Random interval</label>
                            <select id="scn-sched-rand" bind:value={schedRandomOffset}>
                                <option value={0}>None – fire at exact time</option>
                                <option value={5}>Up to 5 min after</option>
                                <option value={10}>Up to 10 min after</option>
                                <option value={15}>Up to 15 min after</option>
                                <option value={30}>Up to 30 min after</option>
                                <option value={60}>Up to 60 min after</option>
                            </select>
                            <div class="field-help">Fires at a random time within the chosen window.</div>
                        </div>
                    </div>
                {/if}

                <!-- ── Trigger / automation config ─────────────── -->
                {#if activationMode === "trigger"}
                    <div class="act-config">
                        <div class="act-config-head">
                            <Icon name="automation" size={15} />
                            Automation configuration
                        </div>

                        <div class="field">
                            <label for="scn-auto-name">Automation name</label>
                            <input id="scn-auto-name" type="text" bind:value={autoName}
                                placeholder="{name.trim() || 'My scene'} trigger"
                                maxlength="60" autocomplete="off" />
                        </div>

                        <div class="block when">
                            <div class="block-head"><span class="tag cool">When</span></div>
                            <Segmented name="scn-trig-type" bind:value={trigType}
                                options={[
                                    { value: "time",   label: "Time" },
                                    { value: "sensor", label: "Sensor", disabled: v.sensors.length === 0 },
                                    { value: "device", label: "Device", disabled: v.sockets.length === 0 },
                                ]} />

                            {#if trigType === "time"}
                                <div class="field mt">
                                    <Segmented name="scn-trig-timemode" bind:value={trigTimeMode}
                                        options={[
                                            { value: "fixed",   label: "Fixed" },
                                            { value: "sunrise", label: "Sunrise" },
                                            { value: "sunset",  label: "Sunset" },
                                        ]} />
                                </div>
                                {#if trigTimeMode === "fixed"}
                                    <div class="field mt">
                                        <label for="scn-trig-time">Time</label>
                                        <input id="scn-trig-time" type="time" bind:value={trigTime} required />
                                    </div>
                                {:else}
                                    <div class="field mt">
                                        <label for="scn-trig-solar">Offset (minutes)</label>
                                        <input id="scn-trig-solar" type="range"
                                            min="-120" max="120" step="5"
                                            bind:value={trigSolarOffset} />
                                        <div class="solar-summary">
                                            {trigSolarLabel}
                                        </div>
                                        {#if !hasLocation}
                                            <div class="field-help warn">Set a location in Settings for solar triggers to fire.</div>
                                        {/if}
                                    </div>
                                {/if}
                                <div class="field mt">
                                    <span class="field-label">On days</span>
                                    <DayPicker bind:days={trigDays} />
                                    <div class="field-help">Leave empty for every day.</div>
                                </div>

                            {:else if trigType === "sensor"}
                                <div class="field-row mt">
                                    <div class="field">
                                        <label for="scn-trig-sensor">Sensor</label>
                                        <select id="scn-trig-sensor" bind:value={trigSensorId}>
                                            {#each v.sensors as s (s.id)}
                                                <option value={s.id}>{s.name}</option>
                                            {/each}
                                        </select>
                                    </div>
                                    <div class="field">
                                        <label for="scn-trig-op">Crosses</label>
                                        <select id="scn-trig-op" bind:value={trigOp}>
                                            <option value="above">Above</option>
                                            <option value="below">Below</option>
                                        </select>
                                    </div>
                                </div>
                                <div class="field mt">
                                    <label for="scn-trig-val">Threshold{v.sensors.find(s => s.id === trigSensorId)?.unit ? ` (${v.sensors.find(s => s.id === trigSensorId)?.unit})` : ""}</label>
                                    <input id="scn-trig-val" type="number" step="0.1" bind:value={trigValue} />
                                </div>

                            {:else}
                                <!-- device trigger -->
                                <div class="field-row mt">
                                    <div class="field">
                                        <label for="scn-trig-dev">Device</label>
                                        <select id="scn-trig-dev" bind:value={trigSocketId}>
                                            {#each v.sockets as s (s.id)}
                                                <option value={s.id}>{s.name}</option>
                                            {/each}
                                        </select>
                                    </div>
                                    <div class="field">
                                        <label for="scn-trig-state">Turns</label>
                                        <select id="scn-trig-state" bind:value={trigToState}>
                                            <option value="on">On</option>
                                            <option value="off">Off</option>
                                        </select>
                                    </div>
                                </div>
                            {/if}
                        </div>
                    </div>
                {/if}
            </div>
        {/if}
    {/snippet}

    {#snippet actions()}
        {#if !isEdit && wizardStep === 1}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
            <button class="btn btn-primary" onclick={advanceToActivation}>
                Next: Activation
                <span class="next-arrow" aria-hidden="true"><Icon name="chevronDown" size={15} /></span>
            </button>
        {:else if !isEdit && wizardStep === 2}
            <button class="btn btn-ghost" onclick={() => wizardStep = 1}>← Back</button>
            <button class="btn btn-primary" onclick={save} disabled={saving}>
                {saving ? "Creating…" : "Create scene"}
            </button>
        {:else}
            <!-- Edit mode -->
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
            <button class="btn btn-primary" onclick={save} disabled={saving}>
                {saving ? "Saving…" : "Save"}
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
    /* Inline timing row for steps 2+ */
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

    /* Hairline separator between rows, indented past the bulb icon */
    .row-sep {
        height: 1px;
        background: var(--separator);
        margin: 0 12px 0 58px;
    }

    /* Each device is a flex column: main row + optional light controls */
    .picker-row {
        display: flex;
        flex-direction: column;
        border-radius: var(--radius-sm);
        overflow: hidden;
        transition: background var(--t-fast);
    }
    .picker-row.row-on { background: var(--primary-soft); }

    /* Inner flex row: bulb · info · state buttons */
    .row-main {
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 10px 12px;
        min-height: 48px;
    }

    /* Circular bulb state indicator */
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
    /* Default active state (ignore) */
    .state-btn.s-active {
        background: var(--card-3);
        color: var(--text);
        box-shadow: var(--shadow-sm);
    }
    /* On active — amber tint */
    .state-btn.s-on.s-active {
        background: var(--primary-soft);
        color: var(--primary);
        box-shadow: none;
    }
    /* Off active — neutral (uses .s-active base) */

    /* ── Smart-light controls (lives inside .picker-row) ─────────── */
    .light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        /* Indent to align with device name: 12px pad + 30px bulb + 12px gap */
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

    /* ── Activation section (step 2) ─────────────────────────────── */
    .act-section {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }

    .act-cards {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 10px;
    }
    .act-card {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        gap: 10px;
        padding: 14px;
        border: 2px solid var(--border);
        border-radius: var(--radius-md);
        background: var(--surface);
        cursor: pointer;
        text-align: left;
        transition: border-color var(--t-fast), background var(--t-fast), box-shadow var(--t-fast);
    }
    .act-card:hover {
        background: var(--surface-hover);
        border-color: var(--border-strong);
    }
    .act-card.act-selected {
        border-color: var(--primary);
        background: var(--primary-soft);
        box-shadow: 0 0 0 1px var(--primary);
    }

    .act-icon {
        display: inline-flex;
        padding: 8px;
        border-radius: var(--radius-sm);
        background: var(--card-3);
        color: var(--text-muted);
        flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .act-card.act-selected .act-icon { background: var(--primary-soft); color: var(--primary); }

    /* Subtle tint per method so the icons are recognisable at a glance */
    .act-icon-schedule { background: var(--info-soft); color: var(--info); }
    .act-card.act-selected .act-icon-schedule { background: var(--info-soft); color: var(--info); }
    .act-icon-trigger  { background: var(--warn-soft); color: var(--warn); }
    .act-card.act-selected .act-icon-trigger { background: var(--warn-soft); color: var(--warn); }

    .act-card-body { display: flex; flex-direction: column; gap: 3px; }
    .act-title {
        font-size: 13px;
        font-weight: 600;
        color: var(--text);
    }
    .act-desc {
        font-size: 12px;
        color: var(--text-muted);
        line-height: 1.45;
    }

    /* ── Activation config block ─────────────────────────────────── */
    .act-config {
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        animation: slideDown 0.15s ease both;
    }
    .act-config-head {
        display: flex;
        align-items: center;
        gap: 6px;
        font-size: 12px;
        font-weight: 600;
        color: var(--text-muted);
        text-transform: uppercase;
        letter-spacing: 0.06em;
    }

    .solar-summary {
        margin-top: 5px;
        font-weight: 600;
        font-size: 0.9rem;
        color: var(--text);
    }
    .field-help.warn { color: var(--warn, var(--danger)); }

    /* Automation "when" block (matches AutomationModal style) */
    .block {
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .block.when { border-left: 3px solid var(--cool); }
    .block-head { display: flex; align-items: center; justify-content: space-between; }
    .tag {
        font-family: var(--font-mono);
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-mute);
    }
    .tag.cool { color: var(--cool); }
    .mt { margin-top: var(--space-3); }

    /* Rotated chevron used on the "Next" button */
    .next-arrow {
        display: inline-flex;
        transform: rotate(-90deg);
        margin-left: 4px;
    }

    @keyframes slideDown {
        from { opacity: 0; transform: translateY(-5px); }
        to   { opacity: 1; transform: translateY(0);    }
    }
    @media (prefers-reduced-motion: reduce) {
        .act-config { animation-duration: 0.001ms; }
        .wiz-dot, .wiz-line, .act-card { transition-duration: 0.001ms; }
        .row-bulb, .picker-row, .state-btn { transition-duration: 0.001ms; }
    }

    /* ── Mobile layout ───────────────────────────────────────────── */
    @media (max-width: 600px) {
        .act-cards { grid-template-columns: 1fr; gap: 8px; }
        .act-card {
            flex-direction: row;
            align-items: center;
            gap: 12px;
            padding: 12px;
        }
        .act-card-body { gap: 2px; }
        /* Picker rows: ensure 44px minimum touch target */
        .row-main { min-height: 52px; padding: 10px; }
        /* 3-state buttons: bigger tap area on touch */
        .state-btn { padding: 7px 10px; font-size: 12px; min-height: 36px; }
        /* Delay input: 16px prevents iOS zoom */
        .delay-input { font-size: 16px; padding: 5px 8px; }
        .remove-step { min-width: 44px; min-height: 44px; }
        .add-step-btn { min-height: 44px; }
        .swatch { width: 28px; height: 28px; }
        .bright input[type="range"] { height: 28px; }
        /* Light controls: tighter indent on narrow screens */
        .light-row { padding: 0 10px 12px 52px; }
    }
    @media (pointer: coarse) {
        .act-card { min-height: 44px; }
        input[type="range"] { height: 28px; }
        .swatch { width: 30px; height: 30px; }
        .state-btn { min-height: 34px; }
    }
</style>

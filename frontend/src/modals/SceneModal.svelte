<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import Segmented from "../components/Segmented.svelte";
    import Switch from "../components/Switch.svelte";
    import DayPicker from "../components/DayPicker.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets, formatAgo } from "../lib/utils";
    import { untrack } from "svelte";
    import type {
        Scene, SceneStep, AutomationTriggerType, AutomationTrigger,
        AutomationAction, AutomationCondition, TargetType, Automation, SceneAccent,
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
    let snapshotOpen = $state(existingHasSnapshot);

    const snapshotDeviceCount = $derived(
        steps.flatMap(step => Object.values(step.perSocket).filter(v => v !== "ignore")).length
    );

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
        for (const s of sockets) {
            step.perSocket[s.id] = s.state ? "on" : "off";
        }
        toasts.info("Captured current state",
            `${sockets.length} device${sockets.length === 1 ? "" : "s"} set to their current on/off state`);
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

    // ── Rule state ─────────────────────────────────────────────────────
    type RuleActionDraft = {
        target_type: TargetType;
        target_id: string;
        action: string;
        level: number;
        color: string;
    };

    type RuleDraft = {
        _key: string;
        automationId: string;
        enabled: boolean;
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
        conditions: AutomationCondition[];
        actions: RuleActionDraft[];
    };

    const hasLocation = $derived(v.settings.latitude !== 0 || v.settings.longitude !== 0);

    function firstTargetType(): TargetType {
        return v.sockets.length ? "socket" : v.groups.length ? "group" : v.rooms.length ? "room" : "scene";
    }

    function targetsFor(type: string) {
        if (type === "socket") return v.sockets.map(s => ({ id: s.id, label: s.name }));
        if (type === "group")  return v.groups.map(g => ({ id: g.id, label: g.name }));
        if (type === "room")   return [...v.rooms].sort((a, b) => a.name.localeCompare(b.name)).map(r => ({ id: r.id, label: r.name }));
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
            enabled: true,
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
            conditions: [],
            actions: [{ target_type: firstTargetType(), target_id: "", action: "on", level: 100, color: "" }],
        };
    }

    function ruleFromAutomation(a: Automation): RuleDraft {
        const t = a.trigger;
        return {
            _key: Math.random().toString(36).slice(2),
            automationId: a.id,
            enabled: a.enabled,
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
            conditions: (a.conditions ?? []).map(c => ({ ...c })),
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

    function addRule() { rules = [...rules, blankRule()]; }
    function removeRule(i: number) { rules = rules.filter((_, idx) => idx !== i); }
    function addRuleAction(ri: number) {
        rules[ri].actions = [...rules[ri].actions,
            { target_type: firstTargetType(), target_id: "", action: "on", level: 100, color: "" }];
    }
    function removeRuleAction(ri: number, ai: number) {
        rules[ri].actions = rules[ri].actions.filter((_, idx) => idx !== ai);
    }
    function addCondition(ri: number) {
        rules[ri].conditions = [...rules[ri].conditions,
            { type: "device", socket_id: v.sockets[0]?.id ?? "", state: "on" }];
    }
    function removeCondition(ri: number, ci: number) {
        rules[ri].conditions = rules[ri].conditions.filter((_, idx) => idx !== ci);
    }

    // Owned automation backing a rule, used to surface last-fired / run-count.
    function ruleStats(automationId: string): { count: number; ago: string } | null {
        if (!automationId) return null;
        const a = data.value.automations.find(x => x.id === automationId);
        if (!a) return null;
        return { count: a.run_count ?? 0, ago: formatAgo(a.last_fired_at) };
    }

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
            trigger = { type: "sensor", sensor_id: rule.trigSensorId, op: rule.trigOp, value: Number(rule.trigValue) };
        } else {
            trigger = { type: "device", socket_id: rule.trigSocketId, to_state: rule.trigToState };
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
        const conditions: AutomationCondition[] = rule.conditions.map(c =>
            c.type === "device"
                ? { type: "device", socket_id: c.socket_id, state: c.state }
                : { type: "time_range", after: c.after, before: c.before },
        );
        return { name: `${sceneName} – rule ${idx + 1}`, enabled: rule.enabled, trigger, conditions, actions, scene_id: sceneId };
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
    const modalSubtitle = "Add automated rules and optionally set a manual activation snapshot.";

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

            let sceneId: string;
            if (isEdit) {
                await api.updateScene(existing!.id, payload);
                sceneId = existing!.id;
            } else {
                const created = await api.createScene(payload);
                sceneId = created.id;
            }

            // Delete removed rules, create / update surviving ones.
            const survivingIds = new Set(rules.map(r => r.automationId).filter(Boolean));
            for (const a of data.value.automations) {
                if (a.scene_id === sceneId && !survivingIds.has(a.id)) {
                    try { await api.deleteAutomation(a.id); } catch (_) { /* best-effort */ }
                }
            }
            let ruleSaveErrors = 0;
            for (let i = 0; i < rules.length; i++) {
                try {
                    const rp = buildRulePayload(rules[i], sceneId, sceneName, i);
                    if (rules[i].automationId) await api.updateAutomation(rules[i].automationId, rp);
                    else await api.createAutomation(rp);
                } catch (_) { ruleSaveErrors++; }
            }

            const rc = rules.length;
            if (ruleSaveErrors > 0) {
                toasts.warn(isEdit ? "Scene updated" : "Scene created",
                    `${rc - ruleSaveErrors} of ${rc} rules saved.`);
            } else {
                toasts.success(isEdit ? "Scene updated" : "Scene created",
                    rc > 0 ? `${sceneName} · ${rc} rule${rc > 1 ? "s" : ""}` : sceneName);
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

            <!-- ── Rules ─────────────────────────────────────────── -->
            <div class="form-section">
                <div class="form-sec-head">
                    <span class="form-sec-label">Rules</span>
                    <span class="form-sec-hint">Each rule fires independently on its own trigger</span>
                </div>

                {#if rules.length === 0}
                    <div class="rules-empty">
                        <div class="rules-empty-icon"><Icon name="automation" size={26} /></div>
                        <div class="rules-empty-title">No rules yet</div>
                        <div class="rules-empty-sub">Rules control devices automatically — at sunset, at a fixed time, when a sensor crosses a threshold, or when a device changes state.</div>
                    </div>
                {/if}

                {#each rules as rule, ri (rule._key)}
                    <div class="rule-card">
                        <div class="rule-header">
                            <span class="rule-badge">Rule {ri + 1}</span>
                            {#if !rule.enabled}<span class="rule-muted">Off</span>{/if}
                            {#if ruleStats(rule.automationId)}
                                {@const st = ruleStats(rule.automationId)}
                                <span class="rule-stats mono" title="How often this rule has fired">
                                    Ran {st!.count}×{st!.ago ? ` · ${st!.ago}` : ""}
                                </span>
                            {/if}
                            <div class="rule-head-actions">
                                <Switch bind:checked={rule.enabled}
                                    ariaLabel="Enable rule {ri + 1}" />
                                <button type="button" class="rule-remove"
                                    onclick={() => removeRule(ri)}
                                    aria-label="Remove rule {ri + 1}">
                                    <Icon name="trash" size={14} />
                                </button>
                            </div>
                        </div>
                        <div class="rule-inner">

                            <!-- WHEN -->
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
                                            <input id="rule-{ri}-time" type="time" bind:value={rule.trigTime} />
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

                            <!-- ONLY IF (conditions) -->
                            <div class="block iff">
                                <div class="block-head">
                                    <span class="tag">Only if</span>
                                    <button type="button" class="chip-sm" onclick={() => addCondition(ri)}>
                                        <Icon name="plus" size={12} /> Condition
                                    </button>
                                </div>
                                {#if rule.conditions.length === 0}
                                    <div class="field-help">Optional — without conditions the rule runs every time it triggers.</div>
                                {/if}
                                {#each rule.conditions as c, ci (ci)}
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
                                            onclick={() => removeCondition(ri, ci)}
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
                                    <button type="button" class="chip-sm" onclick={() => addRuleAction(ri)}>
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
                                                    <option value="room"   disabled={v.rooms.length === 0}>Room</option>
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

                <button type="button" class="add-dashed-btn" onclick={addRule}>
                    <Icon name="plus" size={15} /> Add rule
                </button>
            </div>

            <!-- ── Snapshot (manual activation) ─────────────────── -->
            <div class="form-section">
                <button type="button" class="snapshot-toggle"
                    onclick={() => snapshotOpen = !snapshotOpen}
                    aria-expanded={snapshotOpen}>
                    <span class="snapshot-chevron" class:open={snapshotOpen}>
                        <Icon name="chevronDown" size={14} />
                    </span>
                    <span class="form-sec-label">Manual activation snapshot</span>
                    <span class="opt-pill">optional</span>
                    {#if !snapshotOpen && snapshotDeviceCount > 0}
                        <span class="snapshot-count mono">{snapshotDeviceCount} device{snapshotDeviceCount > 1 ? "s" : ""}</span>
                    {/if}
                </button>

                {#if snapshotOpen}
                    <div class="snapshot-body">
                        <div class="field-help snapshot-hint">
                            Set what happens when you tap this scene manually. Supports multi-step sequences with delays.
                        </div>
                        <div class="steps-wrap">
                            {#each steps as step, i (i)}
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
                                                        if (!isNaN(v) && v >= 0) step.delay_minutes = v;
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
    .form-sec-head {
        display: flex;
        align-items: baseline;
        gap: 8px;
    }
    .form-sec-label {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--text-mute);
    }
    .form-sec-hint {
        font-size: 12px;
        color: var(--text-dim);
    }

    /* ── Rules empty state ───────────────────────────────────────── */
    .rules-empty {
        display: flex;
        flex-direction: column;
        align-items: center;
        text-align: center;
        gap: 8px;
        padding: 28px 24px;
        border: 1px dashed var(--border);
        border-radius: var(--r-md);
    }
    .rules-empty-icon { color: var(--text-dim); }
    .rules-empty-title { font-size: 13.5px; font-weight: 500; color: var(--text-mute); }
    .rules-empty-sub { font-size: 12.5px; color: var(--text-dim); line-height: 1.5; max-width: 340px; }

    /* ── Rule cards ───────────────────────────────────────────────── */
    .rule-card {
        border: 1px solid var(--border);
        border-radius: var(--r-md);
        overflow: hidden;
    }
    .rule-header {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 12px;
        background: var(--card-3);
        border-bottom: 1px solid var(--border);
    }
    .rule-head-actions {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-left: auto;
    }
    .rule-muted {
        font-family: var(--font-mono);
        font-size: 10px;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--text-dim);
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        padding: 1px 7px;
    }
    .rule-stats {
        font-size: 11px;
        color: var(--text-dim);
    }
    .block.iff { border-left: 3px solid var(--border-strong); }
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
        border-radius: var(--r-sm);
        min-width: 32px;
        min-height: 32px;
        justify-content: center;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .rule-remove:hover { background: var(--surface-hover); color: var(--bad); }
    .rule-inner {
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }

    /* ── WHEN / THEN blocks ───────────────────────────────────────── */
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
    .solar-summary {
        margin-top: 5px;
        font-weight: 600;
        font-size: 0.9rem;
        color: var(--text);
    }
    .field-help.warn { color: var(--warn, var(--danger)); }

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
    .opt-pill {
        font-size: 10.5px;
        font-family: var(--font-mono);
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--text-dim);
        background: var(--card-3);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        padding: 1px 7px;
    }
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
        .rule-card, .add-dashed-btn { transition-duration: 0.001ms; }
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
        .rule-remove { min-width: 44px; min-height: 44px; }
    }
    @media (pointer: coarse) {
        input[type="range"] { height: 28px; }
        .swatch { width: 30px; height: 30px; }
        .state-btn { min-height: 34px; }
        .rule-remove { min-width: 44px; min-height: 44px; }
    }
</style>

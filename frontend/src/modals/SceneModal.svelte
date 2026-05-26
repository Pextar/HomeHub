<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets } from "../lib/utils";
    import { untrack } from "svelte";
    import type { Scene, SceneStep } from "../lib/types";

    interface Props { existing?: Scene | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));

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

    // ── Step state ──────────────────────────────────────────────────────
    // Each step holds per-socket action, level, and colour maps.
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

    function addStep() {
        // Suggest a delay 60 minutes after the last step's delay.
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

    // ── Save ────────────────────────────────────────────────────────────
    async function save() {
        if (saving) return;

        nameError = name.trim() ? "" : "Give the scene a name.";

        // Build steps payload, skip fully-ignored steps.
        const builtSteps = steps.map(step => ({
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

        stepsError = builtSteps.length === 0
            ? "Set at least one device to On or Off in any step."
            : "";

        if (nameError || stepsError) return;
        saving = true;
        try {
            const payload = { name: name.trim(), steps: builtSteps };
            if (existing) {
                await api.updateScene(existing.id, payload);
                toasts.success("Scene updated", payload.name);
            } else {
                await api.createScene(payload);
                toasts.success("Scene created", payload.name);
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

<Modal
    title={isEdit ? "Edit scene" : "New scene"}
    subtitle="A scene can drive devices through multiple timed steps — even the same lamp at different dim levels."
    size="wide"
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
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
                            <span class="step-when">{stepLabel(i, step.delay_minutes)}</span>
                            {#if i > 0}
                                <div class="step-delay-wrap">
                                    <span class="delay-label">After</span>
                                    <input
                                        type="number"
                                        class="delay-input"
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
                                    <span class="delay-label">min</span>
                                </div>
                                <button type="button" class="remove-step" onclick={() => removeStep(i)}
                                    aria-label="Remove step {i + 1}">
                                    <Icon name="close" size={14} />
                                </button>
                            {/if}
                        </div>

                        <div class="picker">
                            {#each sockets as s (s.id)}
                                <div class="picker-row">
                                    <div class="info">
                                        <div>{s.name}</div>
                                        <div class="field-help">{s.room || "Unassigned"}</div>
                                    </div>
                                    <select bind:value={step.perSocket[s.id]}
                                        aria-label="Action for {s.name} in step {i + 1}"
                                        onchange={() => stepsError = ""}>
                                        <option value="ignore">Ignore</option>
                                        <option value="on">Turn on</option>
                                        <option value="off">Turn off</option>
                                    </select>
                                </div>
                                {#if step.perSocket[s.id] === "on" && isSmart(s.protocol)}
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
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Create scene"}
        </button>
    {/snippet}
</Modal>

<style>
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
        padding: 8px 10px;
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
        color: var(--text-mute);
        flex-shrink: 0;
    }
    .step-when {
        font-size: 12px;
        color: var(--text);
        flex: 1;
        min-width: 0;
    }
    .step-delay-wrap {
        display: flex;
        align-items: center;
        gap: 4px;
        flex-shrink: 0;
    }
    .delay-label {
        font-size: 12px;
        color: var(--text-mute);
    }
    .delay-input {
        /* Global input styles (padding, border, bg, color, radius) apply;
           only override what's needed for the compact inline display. */
        width: 60px;
        padding: 3px 6px;
        text-align: right;
        border-radius: var(--radius-sm);
    }
    .remove-step {
        position: relative;
        background: transparent;
        border: 0;
        padding: 4px;
        cursor: pointer;
        color: var(--text-mute);
        display: grid;
        place-items: center;
        border-radius: var(--radius-sm);
        margin-left: auto;
        flex-shrink: 0;
    }
    .remove-step:hover { background: var(--surface-hover); color: var(--bad); }

    /* ── Device picker (same as before, now inside each step card) ─ */
    .picker {
        display: flex;
        flex-direction: column;
        gap: 4px;
        max-height: 280px;
        overflow-y: auto;
        padding: 4px;
    }
    .picker-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 8px 10px;
        border-radius: var(--radius-sm);
    }
    .info { flex: 1; min-width: 0; }
    .picker-row select {
        width: auto;
        padding: 4px 10px;
        font-size: 13px;
    }

    /* Smart-light brightness + colour */
    .light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding: 2px 10px 10px 10px;
        margin: -2px 0 4px;
    }
    .bright { display: flex; align-items: center; gap: 8px; }
    .bright-ico { color: var(--on); display: inline-flex; flex-shrink: 0; }
    .bright input[type="range"] { flex: 1; }
    .bright-val { font-size: 12px; color: var(--text-mute); min-width: 38px; text-align: right; }
    .swatches { display: flex; gap: 8px; }
    .swatch {
        width: 24px; height: 24px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        cursor: pointer;
        display: grid; place-items: center;
        padding: 0;
        color: var(--text-mute);
    }
    .swatch.auto { background: var(--card-3); }
    .swatch.active { box-shadow: 0 0 0 2px var(--on), 0 0 0 4px var(--bg-elevated); }

    /* ── Add step button ─────────────────────────────────────────── */
    .add-step-btn {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 7px 14px;
        border: 1px dashed var(--border-strong);
        border-radius: var(--radius-md);
        background: transparent;
        color: var(--text-mute);
        font-size: 13px;
        cursor: pointer;
        margin-top: 4px;
        width: 100%;
        justify-content: center;
    }
    .add-step-btn:hover {
        background: var(--surface-hover);
        color: var(--text);
        border-color: var(--text-mute);
    }

    /* ── Spacing helpers (avoids inline style="") ────────────────── */
    .steps-err  { margin-top: 6px; }
    .steps-hint { margin-top: 6px; }

    @media (pointer: coarse) {
        .picker-row { min-height: 44px; padding: 10px; }
        /* 16px prevents iOS auto-zoom on focus */
        .picker-row select { font-size: 16px; padding: 8px 12px; min-height: 44px; }
        .delay-input { font-size: 16px; padding: 6px 8px; }
        .remove-step { min-width: 44px; min-height: 44px; }
        .add-step-btn { min-height: 44px; }
        .swatch { width: 30px; height: 30px; }
        .bright input[type="range"] { height: 28px; }
    }
</style>

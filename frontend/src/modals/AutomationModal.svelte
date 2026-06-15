<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Switch from "../components/Switch.svelte";
    import Icon from "../components/Icon.svelte";
    import RuleEditor, { blankRuleAction, compileAction } from "../components/RuleEditor.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import type {
        Automation, AutomationRule, AutomationTrigger, AutomationCondition,
        AutomationAction, AutomationTriggerType, RuleDraft,
    } from "../lib/types";

    interface Props { existing?: Automation | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const v = data.value;

    // Each rule is edited as a RuleDraft (WHEN / ONLY-IF / THEN) by RuleEditor,
    // plus a stable key for the keyed #each.
    type RuleDraftKeyed = RuleDraft & { _key: string };
    const newKey = () => Math.random().toString(36).slice(2);

    function blankDraft(): RuleDraftKeyed {
        return {
            _key: newKey(),
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
            actions: [blankRuleAction()],
        };
    }

    function draftFromRule(r: AutomationRule): RuleDraftKeyed {
        const t = r.trigger;
        const actions = (r.actions ?? []).map(act => ({
            target_type: act.target_type,
            target_id: act.target_id,
            action: act.action as string,
            level: act.level ?? 100,
            color: act.color ?? "",
        }));
        return {
            _key: newKey(),
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
            conditions: (r.conditions ?? []).map(c => ({ ...c })),
            actions: actions.length ? actions : [blankRuleAction()],
        };
    }

    let rules = $state<RuleDraftKeyed[]>(untrack(() =>
        existing?.rules?.length ? existing.rules.map(draftFromRule) : [blankDraft()]));

    // Per-rule Run targets the saved rule at that index, so it's only offered for
    // rules that already exist on the persisted automation.
    const savedRuleCount = untrack(() => existing?.rules?.length ?? 0);

    let name = $state(untrack(() => existing?.name ?? ""));
    let enabled = $state(untrack(() => existing ? existing.enabled : true));
    let saving = $state(false);
    let runningIdx = $state<number | null>(null);
    let error = $state("");

    function addRule() { rules = [...rules, blankDraft()]; }
    function removeRule(i: number) { rules = rules.filter((_, idx) => idx !== i); }

    function buildTrigger(d: RuleDraft): AutomationTrigger {
        if (d.trigType === "time") {
            return {
                type: "time",
                time_mode: d.trigTimeMode as AutomationTrigger["time_mode"],
                time: d.trigTimeMode === "fixed" ? d.trigTime : "",
                solar_offset_minutes: d.trigTimeMode === "fixed" ? 0 : d.trigSolarOffset,
                days: d.trigDays,
            };
        }
        if (d.trigType === "sensor") {
            return { type: "sensor", sensor_id: d.trigSensorId, op: d.trigOp, value: Number(d.trigValue) };
        }
        return { type: "device", socket_id: d.trigSocketId, to_state: d.trigToState };
    }

    function buildRule(d: RuleDraft): AutomationRule {
        const conditions: AutomationCondition[] = d.conditions.map(c => {
            if (c.type === "device")      return { type: "device",      socket_id: c.socket_id, state: c.state };
            if (c.type === "time_before") return { type: "time_before", before: c.before };
            if (c.type === "time_after")  return { type: "time_after",  after: c.after };
            return { type: "time_range", after: c.after, before: c.before };
        });
        // compileAction expands per-lamp group/room actions into one socket
        // action per member; every other action maps 1:1.
        const actions: AutomationAction[] = d.actions.flatMap(compileAction);
        return { trigger: buildTrigger(d), conditions, actions };
    }

    function buildPayload(): Partial<Automation> {
        return { name, enabled, rules: rules.map(buildRule) };
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

    async function runRule(i: number) {
        if (!existing || runningIdx !== null) return;
        runningIdx = i;
        try {
            await api.runAutomationRule(existing.id, i);
            toasts.success("Rule ran", `${existing.name} · rule ${i + 1}`);
            await data.refresh();
        } catch (e) {
            toasts.error("Run failed", (e as Error).message);
        } finally {
            runningIdx = null;
        }
    }
</script>

<Modal title={isEdit ? "Edit automation" : "New automation"}
    subtitle="One or more rules — each runs its actions when its trigger fires."
    size="wide">
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="auto-name">Name</label>
                <input id="auto-name" type="text" bind:value={name} placeholder="Sunset porch" maxlength="60"
                    aria-invalid={error && !name.trim() ? "true" : undefined} />
            </div>

            {#each rules as rule, ri (rule._key)}
                <div class="rule-card">
                    <div class="rule-header">
                        <span class="rule-badge">Rule {ri + 1}</span>
                        <div class="rule-actions">
                            {#if isEdit && ri < savedRuleCount}
                                <button type="button" class="rule-run" onclick={() => runRule(ri)}
                                    disabled={runningIdx !== null}
                                    title="Run this rule's actions now">
                                    {runningIdx === ri ? "Running…" : "Run"}
                                </button>
                            {/if}
                            {#if rules.length > 1}
                                <button type="button" class="rule-remove" onclick={() => removeRule(ri)}
                                    aria-label="Remove rule {ri + 1}">
                                    <Icon name="trash" size={14} />
                                </button>
                            {/if}
                        </div>
                    </div>
                    <div class="rule-body">
                        <RuleEditor bind:draft={rules[ri]} idPrefix="auto-{ri}" />
                    </div>
                </div>
            {/each}

            <button type="button" class="add-rule-btn" onclick={addRule}>
                <Icon name="plus" size={15} /> Add rule
            </button>

            <label class="field enabled-row">
                <Switch bind:checked={enabled} ariaLabel="Enabled" />
                <span>Enabled</span>
            </label>

            {#if error}<div class="field-error">{error}</div>{/if}
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Create"}
        </button>
    {/snippet}
</Modal>

<style>
    form { display: flex; flex-direction: column; gap: var(--space-4); }
    .enabled-row { flex-direction: row; align-items: center; gap: 12px; }

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
    .rule-badge {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--text-mute);
    }
    .rule-actions { display: flex; align-items: center; gap: 6px; margin-left: auto; }
    .rule-run {
        padding: 4px 12px;
        font-size: 12px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-mute);
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .rule-run:hover:not(:disabled) { background: var(--card-3); color: var(--text); }
    .rule-run:disabled { opacity: 0.5; cursor: default; }
    .rule-remove {
        background: transparent;
        border: 0;
        padding: 4px 6px;
        cursor: pointer;
        color: var(--text-dim);
        display: inline-flex;
        align-items: center;
        justify-content: center;
        border-radius: var(--r-sm);
        min-width: 32px;
        min-height: 32px;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .rule-remove:hover { background: var(--surface-hover); color: var(--bad); }
    .rule-body {
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }

    .add-rule-btn {
        display: inline-flex; align-items: center; gap: 6px;
        padding: 10px 14px; border: 1px dashed var(--border-strong);
        border-radius: var(--r-md); background: transparent;
        color: var(--text-mute); font-size: 13px; cursor: pointer;
        touch-action: manipulation; width: 100%; justify-content: center;
        transition: background var(--t-fast), color var(--t-fast), border-color var(--t-fast);
        min-height: 44px;
    }
    .add-rule-btn:hover { background: var(--surface-hover); color: var(--text); border-color: var(--text-mute); }

    @media (pointer: coarse) {
        .rule-remove { min-width: 44px; min-height: 44px; }
        .rule-run { min-height: 36px; }
    }
</style>

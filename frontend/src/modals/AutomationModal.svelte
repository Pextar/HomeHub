<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Switch from "../components/Switch.svelte";
    import RuleEditor, { blankRuleAction } from "../components/RuleEditor.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import type {
        Automation, AutomationTrigger, AutomationCondition, AutomationAction,
        AutomationTriggerType, RuleDraft,
    } from "../lib/types";

    interface Props { existing?: Automation | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const v = data.value;

    // ── Rule draft (WHEN / ONLY-IF / THEN), edited by RuleEditor ──────
    function blankDraft(): RuleDraft {
        return {
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

    function draftFromAutomation(a: Automation): RuleDraft {
        const t = a.trigger;
        const actions = (a.actions ?? []).map(act => ({
            target_type: act.target_type,
            target_id: act.target_id,
            action: act.action as string,
            level: act.level ?? 100,
            color: act.color ?? "",
        }));
        return {
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
            actions: actions.length ? actions : [blankRuleAction()],
        };
    }

    let draft = $state<RuleDraft>(untrack(() =>
        existing ? draftFromAutomation(existing) : blankDraft()));

    let name = $state(untrack(() => existing?.name ?? ""));
    let enabled = $state(untrack(() => existing ? existing.enabled : true));
    let saving = $state(false);
    let error = $state("");

    function buildPayload(): Partial<Automation> {
        let trigger: AutomationTrigger;
        if (draft.trigType === "time") {
            trigger = {
                type: "time",
                time_mode: draft.trigTimeMode as AutomationTrigger["time_mode"],
                time: draft.trigTimeMode === "fixed" ? draft.trigTime : "",
                solar_offset_minutes: draft.trigTimeMode === "fixed" ? 0 : draft.trigSolarOffset,
                days: draft.trigDays,
            };
        } else if (draft.trigType === "sensor") {
            trigger = { type: "sensor", sensor_id: draft.trigSensorId, op: draft.trigOp, value: Number(draft.trigValue) };
        } else {
            trigger = { type: "device", socket_id: draft.trigSocketId, to_state: draft.trigToState };
        }
        const actions: AutomationAction[] = draft.actions.map(a => {
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
                    // Only include lighting info when the user explicitly chose a preset
                    // (any color set, or brightness moved from the default 100%).
                    if (a.color || a.level !== 100) {
                        base.level = a.level ?? 100;
                        if (a.color) base.color = a.color;
                    }
                }
            }
            return base;
        });
        const conditions: AutomationCondition[] = draft.conditions.map(c =>
            c.type === "device"
                ? { type: "device", socket_id: c.socket_id, state: c.state }
                : { type: "time_range", after: c.after, before: c.before },
        );
        return { name, enabled, trigger, conditions, actions };
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

            <RuleEditor bind:draft idPrefix="auto" />

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
    .enabled-row { flex-direction: row; align-items: center; gap: 12px; }
</style>

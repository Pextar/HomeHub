<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Segmented from "../components/Segmented.svelte";
    import DayPicker from "../components/DayPicker.svelte";
    import Switch from "../components/Switch.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores";
    import type { Schedule, TargetType } from "../lib/types";

    interface Props { existing?: Schedule | null; }
    let { existing = null }: Props = $props();
    const isEdit = !!existing;

    const v = data.value;

    const initialType: TargetType =
        (existing?.target_type as TargetType | undefined)
        ?? (existing?.socket_id ? "socket" : null)
        ?? (v.sockets.length ? "socket" : v.groups.length ? "group" : "scene");

    let targetType = $state<string>(initialType);
    let targetId = $state<string>(existing?.target_id || existing?.socket_id || "");
    let action = $state<string>(existing?.action ?? "on");
    let time = $state(existing?.time || "08:00");
    let days = $state<number[]>([...(existing?.days ?? [])]);
    let enabled = $state(existing ? existing.enabled : true);

    // Available targets for the current type, plus reset of selection /
    // action when switching type.
    const targets = $derived.by(() => {
        if (targetType === "socket") return v.sockets.map(s => ({ id: s.id, label: `${s.name}${s.room ? ` · ${s.room}` : ""}` }));
        if (targetType === "group")  return v.groups.map(g => ({ id: g.id, label: `${g.name} · ${g.socket_ids.length} sockets` }));
        return v.scenes.map(s => ({ id: s.id, label: `${s.name} · ${s.actions.length} actions` }));
    });

    $effect(() => {
        if (!targets.find(t => t.id === targetId)) {
            targetId = targets[0]?.id ?? "";
        }
        if (targetType === "scene") action = "activate";
        else if (action === "activate") action = "on";
    });

    async function save() {
        if (!targetId) { toasts.warn("Missing target", "Pick something to schedule."); return; }
        const payload: Partial<Schedule> = {
            target_type: targetType as TargetType,
            target_id: targetId,
            action: action as Schedule["action"],
            time,
            days,
            enabled,
        };
        try {
            if (existing) {
                await api.updateSchedule(existing.id, payload);
                toasts.success("Schedule updated");
            } else {
                await api.createSchedule(payload);
                toasts.success("Schedule added");
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        }
    }
</script>

<Modal
    title={isEdit ? "Edit schedule" : "Add schedule"}
    subtitle={isEdit ? "Update when this schedule fires." : "Run a socket, group or scene at a chosen time."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label>Target type</label>
                <Segmented
                    name="sched-target-type"
                    bind:value={targetType}
                    options={[
                        { value: "socket", label: "Socket", disabled: v.sockets.length === 0 },
                        { value: "group",  label: "Group",  disabled: v.groups.length === 0 },
                        { value: "scene",  label: "Scene",  disabled: v.scenes.length === 0 },
                    ]}
                />
            </div>
            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="sched-target">Target</label>
                    <select id="sched-target" bind:value={targetId} required>
                        {#each targets as t (t.id)}
                            <option value={t.id}>{t.label}</option>
                        {/each}
                    </select>
                </div>
                <div class="field" style:opacity={targetType === "scene" ? 0.6 : 1}>
                    <label for="sched-action">Action</label>
                    <select id="sched-action" bind:value={action} disabled={targetType === "scene"}>
                        {#if targetType === "scene"}
                            <option value="activate">Activate</option>
                        {:else}
                            <option value="on">Turn ON</option>
                            <option value="off">Turn OFF</option>
                            <option value="toggle">Toggle</option>
                        {/if}
                    </select>
                </div>
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label for="sched-time">Time</label>
                <input id="sched-time" type="time" bind:value={time} required />
                <div class="field-help">24-hour HH:MM in the server's local time.</div>
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label>Days</label>
                <DayPicker bind:days={days} />
                <div class="field-help">Leave empty to fire every day.</div>
            </div>
            <label class="field" style="flex-direction:row; align-items:center; gap:12px; margin-top:var(--space-4)">
                <Switch bind:checked={enabled} ariaLabel="Enabled" />
                <span>Enabled</span>
            </label>
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save}>
            {isEdit ? "Save" : "Add schedule"}
        </button>
    {/snippet}
</Modal>

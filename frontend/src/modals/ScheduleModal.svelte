<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Segmented from "../components/Segmented.svelte";
    import DayPicker from "../components/DayPicker.svelte";
    import Switch from "../components/Switch.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import type { Schedule, ScheduleTimeMode, TargetType } from "../lib/types";

    interface Props { existing?: Schedule | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const v = data.value;

    const initialType: TargetType = untrack(() =>
        (existing?.target_type as TargetType | undefined)
        ?? (existing?.socket_id ? "socket" : null)
        ?? (v.sockets.length ? "socket" : v.groups.length ? "group" : "scene")
    );

    let targetType = $state<string>(initialType);
    let targetId = $state<string>(untrack(() => existing?.target_id || existing?.socket_id || ""));
    let action = $state<string>(untrack(() => existing?.action ?? "on"));
    let timeMode = $state<ScheduleTimeMode>(untrack(() => (existing?.time_mode as ScheduleTimeMode | undefined) ?? "fixed"));
    let time = $state(untrack(() => existing?.time || "08:00"));
    let solarOffsetMinutes = $state<number>(untrack(() => existing?.solar_offset_minutes ?? 0));
    let days = $state<number[]>(untrack(() => [...(existing?.days ?? [])]));
    let enabled = $state(untrack(() => existing ? existing.enabled : true));
    let randomOffsetMinutes = $state<number>(untrack(() => existing?.random_offset_minutes ?? 0));

    const hasLocation = $derived(v.settings.latitude !== 0 || v.settings.longitude !== 0);
    // Pretty offset label: "at sunrise", "30 min before sunset", "1h 30m after sunrise".
    const solarOffsetLabel = $derived.by(() => {
        const m = solarOffsetMinutes;
        const event = timeMode === "sunrise" ? "sunrise" : "sunset";
        if (m === 0) return `At ${event}`;
        const abs = Math.abs(m);
        const h = Math.floor(abs / 60);
        const mins = abs % 60;
        const parts = [h && `${h}h`, mins && `${mins}m`].filter(Boolean).join(" ");
        return `${parts} ${m < 0 ? "before" : "after"} ${event}`;
    });

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
            time_mode: timeMode,
            // Send the time field unconditionally — the backend ignores it
            // for solar modes but the payload's TimeMode reflects the user's choice.
            time: timeMode === "fixed" ? time : "",
            solar_offset_minutes: timeMode === "fixed" ? 0 : solarOffsetMinutes,
            days,
            enabled,
            random_offset_minutes: randomOffsetMinutes,
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
                <span class="field-label">Target type</span>
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
                <span class="field-label">When</span>
                <Segmented
                    name="sched-time-mode"
                    bind:value={timeMode}
                    options={[
                        { value: "fixed",   label: "Fixed time" },
                        { value: "sunrise", label: "Sunrise" },
                        { value: "sunset",  label: "Sunset" },
                    ]}
                />
            </div>
            {#if timeMode === "fixed"}
                <div class="field" style="margin-top:var(--space-4)">
                    <label for="sched-time">Time</label>
                    <input id="sched-time" type="time" bind:value={time} required />
                    <div class="field-help">24-hour HH:MM in the server's local time.</div>
                </div>
            {:else}
                <div class="field" style="margin-top:var(--space-4)">
                    <label for="sched-solar-offset">Offset</label>
                    <input
                        id="sched-solar-offset"
                        type="range"
                        min="-120" max="120" step="5"
                        bind:value={solarOffsetMinutes}
                    />
                    <div class="solar-summary">{solarOffsetLabel}</div>
                    {#if !hasLocation}
                        <div class="field-help warn">
                            Set the controller's latitude/longitude in Settings — without a location,
                            this schedule cannot fire.
                        </div>
                    {:else}
                        <div class="field-help">Drag to pick how far before (−) or after (+) the event to fire.</div>
                    {/if}
                </div>
            {/if}
            <div class="field" style="margin-top:var(--space-4)">
                <span class="field-label">Days</span>
                <DayPicker bind:days={days} />
                <div class="field-help">Leave empty to fire every day.</div>
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label for="sched-offset">Random interval</label>
                <select id="sched-offset" bind:value={randomOffsetMinutes}>
                    <option value={0}>None – fire at exact time</option>
                    <option value={5}>Up to 5 min after</option>
                    <option value={10}>Up to 10 min after</option>
                    <option value={15}>Up to 15 min after</option>
                    <option value={30}>Up to 30 min after</option>
                    <option value={60}>Up to 60 min after</option>
                    <option value={120}>Up to 2 hours after</option>
                </select>
                <div class="field-help">Fires at a random time within the chosen window.</div>
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

<style>
    .solar-summary {
        margin-top: 6px;
        font-weight: 600;
        font-size: 0.95rem;
    }
    .field-help.warn {
        color: var(--warn, var(--danger));
    }
</style>

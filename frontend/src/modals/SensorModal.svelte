<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal, openModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import type { Sensor, SensorKind } from "../lib/types";

    interface Prefill {
        code?: string;
        protocol?: string;
        field?: string;
    }
    interface Props { existing?: Sensor | null; prefill?: Prefill | null; }
    let { existing = null, prefill = null }: Props = $props();
    const isEdit = $derived(!!existing);

    let name = $state(untrack(() => existing?.name ?? ""));
    let kind = $state<SensorKind>(untrack(() => existing?.kind ?? "temperature"));
    let unit = $state(untrack(() => existing?.unit ?? defaultUnit(existing?.kind ?? "temperature")));
    let code = $state(untrack(() => existing?.code ?? prefill?.code ?? ""));
    let protocol = $state(untrack(() => existing?.protocol ?? prefill?.protocol ?? "rtl_433"));
    let field = $state(untrack(() => existing?.field ?? prefill?.field ?? defaultField(existing?.kind ?? "temperature")));
    let room = $state(untrack(() => existing?.room ?? ""));
    // Empty string = no threshold. Stored as numbers (or omitted) on save.
    let alertMin = $state(untrack(() => existing?.alert_min ?? ""));
    let alertMax = $state(untrack(() => existing?.alert_max ?? ""));
    let saving = $state(false);

    // When the user changes kind, reset unit + field to their defaults
    // for that kind. They can still edit afterwards.
    let initialKind = untrack(() => kind);
    $effect(() => {
        if (kind === initialKind) return;
        initialKind = kind;
        unit = defaultUnit(kind);
        field = defaultField(kind);
    });

    function defaultUnit(k: SensorKind): string {
        switch (k) {
            case "temperature": return "°C";
            case "humidity":    return "%";
            case "light":       return "lux";
            case "power":       return "W";
            case "motion":      return "";
            default:            return "";
        }
    }
    function defaultField(k: SensorKind): string {
        switch (k) {
            case "temperature": return "temperature_C";
            case "humidity":    return "humidity";
            case "light":       return "lux";
            case "power":       return "power_W";
            case "motion":      return "motion";
            default:            return "";
        }
    }

    async function save() {
        if (saving) return;
        if (!name.trim()) { toasts.warn("Missing name", "Give the sensor a name."); return; }
        if (!code.trim()) { toasts.warn("Missing code", "Sensors need a 433MHz code to listen for."); return; }
        const payload: Partial<Sensor> = {
            name, kind, unit, code, protocol, field, room,
            alert_min: alertMin === "" ? undefined : Number(alertMin),
            alert_max: alertMax === "" ? undefined : Number(alertMax),
        };
        saving = true;
        try {
            if (existing) {
                await api.updateSensor(existing.id, payload);
                toasts.success("Sensor updated");
            } else {
                await api.createSensor(payload);
                toasts.success("Sensor added");
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }

    async function remove() {
        if (!existing) return;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete sensor?",
            message: `Remove "${existing.name}" and discard its reading history.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteSensor(existing.id);
            toasts.success("Sensor deleted");
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Delete failed", (e as Error).message);
        }
    }
</script>

<Modal
    title={isEdit ? "Edit sensor" : "Add sensor"}
    subtitle={isEdit ? "Update how this 433MHz sensor is matched and displayed." : "Configure a 433MHz sensor to start collecting readings."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="sensor-name">Name</label>
                <input id="sensor-name" type="text" bind:value={name} required placeholder="Living room" />
            </div>

            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="sensor-kind">Kind</label>
                    <select id="sensor-kind" bind:value={kind}>
                        <option value="temperature">Temperature</option>
                        <option value="humidity">Humidity</option>
                        <option value="motion">Motion</option>
                        <option value="light">Light</option>
                        <option value="power">Power</option>
                        <option value="custom">Custom</option>
                    </select>
                </div>
                <div class="field">
                    <label for="sensor-unit">Unit</label>
                    <input id="sensor-unit" type="text" bind:value={unit} placeholder="°C" />
                </div>
            </div>

            <div class="field" style="margin-top:var(--space-4)">
                <label for="sensor-code">Code</label>
                <input id="sensor-code" type="text" bind:value={code} required placeholder="Acurite-Tower:1234" />
                <div class="field-help">
                    For 433MHz this is the device identifier (with rtl_433, usually <code>model:id</code>).
                    For an MQTT sensor, set protocol to <code>mqtt</code> and use the topic to subscribe
                    to (wildcards <code>+</code>/<code>#</code> allowed).
                </div>
            </div>

            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="sensor-protocol">Protocol</label>
                    <input id="sensor-protocol" type="text" bind:value={protocol} placeholder="rtl_433" />
                </div>
                <div class="field">
                    <label for="sensor-field">JSON field</label>
                    <input id="sensor-field" type="text" bind:value={field} placeholder="temperature_C" />
                </div>
            </div>
            <div class="field-help" style="margin-top:calc(var(--space-2) * -1)">
                Which key in the decoder's JSON output holds the numeric value. Leave blank to auto-pick the first number.
            </div>

            <div class="field" style="margin-top:var(--space-4)">
                <label for="sensor-room">Room</label>
                <input id="sensor-room" type="text" bind:value={room} placeholder="Kitchen" />
            </div>

            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="sensor-min">Alert below {#if unit}<span class="unit-hint">({unit})</span>{/if}</label>
                    <input id="sensor-min" type="number" step="any" bind:value={alertMin} placeholder="—" />
                </div>
                <div class="field">
                    <label for="sensor-max">Alert above {#if unit}<span class="unit-hint">({unit})</span>{/if}</label>
                    <input id="sensor-max" type="number" step="any" bind:value={alertMax} placeholder="—" />
                </div>
            </div>
            <div class="field-help" style="margin-top:calc(var(--space-2) * -1)">
                The sensor is flagged when its latest reading crosses either limit. Leave blank to disable.
            </div>
        </form>
    {/snippet}
    {#snippet actions()}
        {#if isEdit}
            <button class="btn btn-ghost danger" onclick={remove}>Delete</button>
        {/if}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Add sensor"}
        </button>
    {/snippet}
</Modal>

<style>
    .danger { color: var(--danger); }
    .unit-hint { color: var(--text-faint); font-weight: 400; }
</style>

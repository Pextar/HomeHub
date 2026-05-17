<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import { PROTOCOLS } from "../lib/utils";
    import { untrack } from "svelte";
    import type { Socket } from "../lib/types";

    interface Props { existing?: Socket | null; }
    let { existing = null }: Props = $props();

    let name = $state(untrack(() => existing?.name ?? ""));
    let room = $state(untrack(() => existing?.room ?? ""));
    let code = $state(untrack(() => existing?.code ?? ""));
    let protocol = $state(untrack(() => existing?.protocol || "nexa"));

    const isEdit = $derived(!!existing);
    const isHue = $derived(protocol === "hue");

    let pairing = $state(false);
    let loadingLights = $state(false);
    let huePickerLights = $state<{ id: string; name: string }[]>([]);
    let hueLightPicker = $state(false);

    async function pair() {
        if (pairing) return;
        pairing = true;
        try {
            const r = await api.learnSocket({ protocol });
            code = r.code;
            toasts.success("Signal sent", "Did your socket click on? If not, long-press its button again and tap Pair.");
        } catch (e) {
            toasts.error("Pairing failed", (e as Error).message);
        } finally {
            pairing = false;
        }
    }

    async function pickHueLight() {
        if (loadingLights) return;
        loadingLights = true;
        try {
            const lights = await api.hueListLights();
            huePickerLights = Object.entries(lights)
                .map(([id, l]) => ({ id, name: l.name }))
                .sort((a, b) => Number(a.id) - Number(b.id));
            if (huePickerLights.length === 0) {
                toasts.warn("No lights found", "Make sure your Hue bridge is reachable.");
            } else {
                hueLightPicker = true;
            }
        } catch (e) {
            const msg = (e as Error).message;
            if (msg.includes("not configured")) {
                toasts.warn("Bridge not configured", "Add your Hue bridge in Settings first.");
            } else {
                toasts.error("Could not fetch lights", msg);
            }
        } finally {
            loadingLights = false;
        }
    }

    function selectHueLight(id: string) {
        code = id;
        hueLightPicker = false;
    }

    async function save() {
        const payload = {
            name: name.trim(),
            room: room.trim(),
            code: code.trim(),
            protocol,
        };
        if (!payload.name || !payload.code) {
            toasts.warn("Missing fields", isHue ? "Name and Hue light ID are required." : "Name and RF code are required.");
            return;
        }
        try {
            if (existing) {
                await api.updateSocket(existing.id, payload);
                toasts.success("Socket updated", payload.name);
            } else {
                await api.createSocket(payload);
                toasts.success("Socket added", payload.name);
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        }
    }
</script>

<Modal
    title={isEdit ? "Edit socket" : "Add socket"}
    subtitle={isEdit
        ? "Update this socket's details."
        : isHue
            ? "Configure a Philips Hue Wi-Fi lamp."
            : "Configure a new 433MHz controllable socket."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="sock-name">Socket name</label>
                <input id="sock-name" type="text" bind:value={name}
                    placeholder="e.g. Living room lamp" autocomplete="off" required />
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label for="sock-room">Room</label>
                <input id="sock-room" type="text" bind:value={room}
                    placeholder="e.g. Living room" autocomplete="off" />
                <div class="field-help">Optional. Used to group sockets and for room-wide on/off.</div>
            </div>
            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="sock-proto">Protocol</label>
                    <select id="sock-proto" bind:value={protocol}>
                        {#each PROTOCOLS as p}
                            <option value={p.value}>{p.label}</option>
                        {/each}
                    </select>
                </div>
                <div class="field">
                    <label for="sock-code">{isHue ? "Hue light ID" : "RF code"}</label>
                    <input id="sock-code" type="text" bind:value={code}
                        placeholder={isHue ? "e.g. 1" : "e.g. 12345"} autocomplete="off" required />
                </div>
            </div>

            {#if isHue}
                <div class="field" style="margin-top:var(--space-3)">
                    <button type="button" class="btn btn-secondary" onclick={pickHueLight} disabled={loadingLights}>
                        {loadingLights ? "Loading lights…" : "Pick a light"}
                    </button>
                    <div class="field-help">
                        Fetches the list of lights from your Hue bridge. Requires the bridge to be configured in Settings.
                    </div>
                    {#if hueLightPicker && huePickerLights.length > 0}
                        <div class="hue-picker">
                            {#each huePickerLights as l}
                                <button type="button" class="hue-light-btn" class:selected={code === l.id}
                                    onclick={() => selectHueLight(l.id)}>
                                    <span class="light-id">#{l.id}</span>
                                    <span class="light-name">{l.name}</span>
                                </button>
                            {/each}
                        </div>
                    {/if}
                </div>
            {:else if !isEdit}
                <div class="field" style="margin-top:var(--space-3)">
                    <button type="button" class="btn btn-secondary" onclick={pair} disabled={pairing}>
                        {pairing ? "Sending…" : "Pair with socket"}
                    </button>
                    <div class="field-help">
                        Long-press the button on your socket until its indicator flashes, then tap Pair.
                        I'll pick a random code and broadcast it — the socket should click on.
                    </div>
                </div>
            {/if}
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save}>
            {isEdit ? "Save" : "Add socket"}
        </button>
    {/snippet}
</Modal>

<style>
    .hue-picker {
        display: flex;
        flex-direction: column;
        gap: var(--space-1);
        margin-top: var(--space-2);
        max-height: 180px;
        overflow-y: auto;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: var(--space-1);
    }
    .hue-light-btn {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        background: transparent;
        border: 1px solid transparent;
        border-radius: var(--radius-sm);
        padding: var(--space-2) var(--space-3);
        text-align: left;
        cursor: pointer;
        font-size: 13px;
        color: var(--text);
        transition: background 0.1s;
    }
    .hue-light-btn:hover { background: var(--bg-hover); }
    .hue-light-btn.selected { border-color: var(--accent); background: var(--accent-subtle); }
    .light-id { color: var(--text-muted); font-size: 11px; min-width: 24px; }
    .light-name { font-weight: 500; }
</style>

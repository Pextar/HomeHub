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

    let pairing = $state(false);

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

    async function save() {
        const payload = {
            name: name.trim(),
            room: room.trim(),
            code: code.trim(),
            protocol,
        };
        if (!payload.name || !payload.code) {
            toasts.warn("Missing fields", "Name and RF code are required.");
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
    subtitle={isEdit ? "Update this socket's details." : "Configure a new 433MHz controllable socket."}
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
                    <label for="sock-code">RF code</label>
                    <input id="sock-code" type="text" bind:value={code}
                        placeholder="e.g. 12345" autocomplete="off" required />
                </div>
                <div class="field">
                    <label for="sock-proto">Protocol</label>
                    <select id="sock-proto" bind:value={protocol}>
                        {#each PROTOCOLS as p}
                            <option value={p.value}>{p.label}</option>
                        {/each}
                    </select>
                </div>
            </div>
            {#if !isEdit}
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

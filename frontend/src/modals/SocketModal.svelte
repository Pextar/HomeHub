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

    let name     = $state(untrack(() => existing?.name     ?? ""));
    let room     = $state(untrack(() => existing?.room     ?? ""));
    let code     = $state(untrack(() => existing?.code     ?? ""));
    let protocol = $state(untrack(() => existing?.protocol || "nexa"));
    let pairingCode = $state("");

    const isEdit     = $derived(!!existing);
    const isTasmota  = $derived(protocol === "tasmota");
    const isMatter   = $derived(protocol === "matter");

    let pairing      = $state(false);
    let probing      = $state(false);
    let commissioning = $state(false);

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

    async function commission() {
        if (commissioning) return;
        const pc = pairingCode.trim();
        if (!pc) {
            toasts.warn("Enter a pairing code", "Use the 11-digit manual code or the MT: QR payload printed on the device.");
            return;
        }
        commissioning = true;
        try {
            const r = await api.matterCommission({ pairing_code: pc });
            code = r.node_id;
            toasts.success("Device commissioned", `Assigned node id ${r.node_id}. Save to add it as a socket.`);
        } catch (e) {
            toasts.error("Commissioning failed", (e as Error).message);
        } finally {
            commissioning = false;
        }
    }

    async function testConnection() {
        if (probing) return;
        const ip = code.trim();
        if (!ip) {
            toasts.warn("Enter an IP first", "Type the device IP in the field above.");
            return;
        }
        probing = true;
        try {
            await api.tasmotaProbe(ip);
            toasts.success("Device found", `Tasmota is responding at ${ip}.`);
        } catch (e) {
            toasts.error("No device found", (e as Error).message);
        } finally {
            probing = false;
        }
    }

    async function save() {
        const payload = { name: name.trim(), room: room.trim(), code: code.trim(), protocol };
        if (!payload.name || !payload.code) {
            const missing = isTasmota ? "device IP"
                          : isMatter  ? "Matter node id (commission a device first)"
                          : "RF code";
            toasts.warn("Missing fields", `Name and ${missing} are required.`);
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
        : isTasmota
            ? "Configure a Tasmota Wi-Fi device."
            : isMatter
                ? "Commission a Matter Wi-Fi device."
                : "Configure a new 433MHz controllable socket."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="sock-name">Name</label>
                <input id="sock-name" type="text" bind:value={name}
                    placeholder="e.g. Living room lamp" autocomplete="off" required />
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label for="sock-room">Room <span class="opt">(optional)</span></label>
                <input id="sock-room" type="text" bind:value={room}
                    placeholder="e.g. Living room" autocomplete="off" />
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
                    <label for="sock-code">
                        {isTasmota ? "Device IP" : isMatter ? "Matter node id" : "RF code"}
                    </label>
                    <input id="sock-code" type="text" bind:value={code}
                        placeholder={isTasmota ? "e.g. 192.168.1.50"
                                   : isMatter  ? "auto-filled after commissioning"
                                   : "e.g. 12345"}
                        autocomplete="off" required
                        readonly={isMatter && !isEdit} />
                </div>
            </div>

            {#if isTasmota}
                <div class="field" style="margin-top:var(--space-3)">
                    <button type="button" class="btn btn-secondary" onclick={testConnection} disabled={probing}>
                        {probing ? "Testing…" : "Test connection"}
                    </button>
                    <div class="field-help">
                        Pings the device to confirm Tasmota is running at that IP.
                        Find the IP in your router's DHCP list or the Tasmota web UI.
                    </div>
                </div>
            {:else if isMatter && !isEdit}
                <div class="field" style="margin-top:var(--space-3)">
                    <label for="sock-pair">Pairing code</label>
                    <input id="sock-pair" type="text" bind:value={pairingCode}
                        placeholder="e.g. 3496-112-0001 or MT:Y.K9..."
                        autocomplete="off" />
                </div>
                <div class="field" style="margin-top:var(--space-3)">
                    <button type="button" class="btn btn-secondary" onclick={commission} disabled={commissioning}>
                        {commissioning ? "Commissioning…" : "Commission device"}
                    </button>
                    <div class="field-help">
                        Enter the 11-digit manual code or the <code>MT:</code> QR payload printed on the
                        device. The bridge uses BLE to onboard it to your Wi-Fi — this can take 30–60 seconds.
                    </div>
                </div>
            {:else if !isEdit}
                <div class="field" style="margin-top:var(--space-3)">
                    <button type="button" class="btn btn-secondary" onclick={pair} disabled={pairing}>
                        {pairing ? "Sending…" : "Pair with socket"}
                    </button>
                    <div class="field-help">
                        Long-press the button on your socket until its indicator flashes,
                        then tap Pair. I'll pick a random code and broadcast it.
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
    .opt { color: var(--text-muted); font-weight: 400; font-size: 12px; }
</style>

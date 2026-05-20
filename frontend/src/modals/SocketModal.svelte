<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal, openModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import { PROTOCOLS } from "../lib/utils";
    import { untrack } from "svelte";
    import type { Socket } from "../lib/types";
    import MatterCommissionModal from "./MatterCommissionModal.svelte";

    interface Props { existing?: Socket | null; }
    let { existing = null }: Props = $props();

    let name     = $state(untrack(() => existing?.name     ?? ""));
    let room     = $state(untrack(() => existing?.room     ?? ""));
    let code     = $state(untrack(() => existing?.code     ?? ""));
    let protocol = $state(untrack(() => existing?.protocol || "nexa"));
    let emoji    = $state(untrack(() => existing?.emoji    ?? ""));

    // Quick-pick set shown in kid mode. Tapping the active one clears it.
    const EMOJI_CHOICES = ["💡", "🛏️", "🌟", "🚀", "🦕", "🐙", "🌈", "🎮", "📺", "🎄", "🔦", "🛋️"];

    const isEdit     = $derived(!!existing);
    const isTasmota  = $derived(protocol === "tasmota");
    const isMatter   = $derived(protocol === "matter");

    let pairing      = $state(false);
    let probing      = $state(false);

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

    function startMatterSetup() {
        // Hand off to the dedicated wizard. It owns the whole flow
        // (scan/paste → commission → name/room → save), so we close
        // this generic Add Socket modal first to avoid stacking.
        closeModal();
        openModal(MatterCommissionModal, {});
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
        const payload = { name: name.trim(), room: room.trim(), code: code.trim(), protocol, emoji };
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
        {#if isMatter && !isEdit}
            <!-- Matter onboarding lives in its own wizard. Show only a
                 protocol picker + a clear hand-off so users aren't asked
                 for fields the wizard will collect itself. -->
            <div class="field">
                <label for="sock-proto">Protocol</label>
                <select id="sock-proto" bind:value={protocol}>
                    {#each PROTOCOLS as p}
                        <option value={p.value}>{p.label}</option>
                    {/each}
                </select>
            </div>
            <div class="matter-lead">
                <h3>Matter Wi-Fi device</h3>
                <p>
                    Matter devices use a one-time onboarding flow over Bluetooth.
                    We'll scan the QR code (or accept the manual pairing code), commission
                    the device onto your Wi-Fi, then add it here — all in one step.
                </p>
                <button type="button" class="btn btn-primary" onclick={startMatterSetup}>
                    Start Matter setup
                </button>
                <p class="hint">Takes about 30–60 seconds once you start.</p>
            </div>
        {:else}
            <form onsubmit={(e) => { e.preventDefault(); save(); }}>
                <div class="field">
                    <label for="sock-name">Name</label>
                    <input id="sock-name" type="text" bind:value={name}
                        placeholder="e.g. Living room lamp" autocomplete="off" required />
                </div>
                <div class="field" style="margin-top:var(--space-4)">
                    <label for="sock-room">Room <span class="opt">(optional)</span></label>
                    <input id="sock-room" type="text" bind:value={room}
                        placeholder="e.g. Living room" autocomplete="off"
                        list="sock-room-list" />
                    <datalist id="sock-room-list">
                        {#each data.value.rooms as r (r.name)}
                            <option value={r.name}></option>
                        {/each}
                    </datalist>
                </div>
                <div class="field" style="margin-top:var(--space-4)">
                    <span class="field-label">Icon <span class="opt">(for kid mode)</span></span>
                    <div class="emoji-grid" role="group" aria-label="Pick an icon">
                        {#each EMOJI_CHOICES as e}
                            <button type="button" class="emoji-btn" class:active={emoji === e}
                                aria-pressed={emoji === e}
                                onclick={() => emoji = emoji === e ? "" : e}>{e}</button>
                        {/each}
                    </div>
                    <div class="field-help">Shown big on this lamp's tile for kid profiles. Tap again to clear.</div>
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
                                       : isMatter  ? "node id from commissioning"
                                       : "e.g. 12345"}
                            autocomplete="off" required />
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
        {/if}
    {/snippet}
    {#snippet actions()}
        {#if isMatter && !isEdit}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        {:else}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
            <button class="btn btn-primary" onclick={save}>
                {isEdit ? "Save" : "Add socket"}
            </button>
        {/if}
    {/snippet}
</Modal>

<style>
    .opt { color: var(--text-muted); font-weight: 400; font-size: 12px; }
    .emoji-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(44px, 1fr));
        gap: 6px;
    }
    .emoji-btn {
        font-size: 22px;
        line-height: 1;
        aspect-ratio: 1;
        display: grid;
        place-items: center;
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        background: var(--surface);
        cursor: pointer;
        transition: transform var(--t-fast), border-color var(--t-fast), background var(--t-fast);
    }
    .emoji-btn:hover { background: var(--surface-hover); transform: translateY(-1px); }
    .emoji-btn.active {
        border-color: var(--primary);
        background: var(--primary-soft);
        box-shadow: 0 0 0 2px var(--primary-glow);
    }

    .matter-lead {
        margin-top: var(--space-4);
        padding: var(--space-4);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        align-items: flex-start;
    }
    .matter-lead h3 {
        font-size: 14px;
        font-weight: 600;
    }
    .matter-lead p {
        font-size: 13px;
        color: var(--text-muted);
        margin: 0;
    }
    .matter-lead .hint {
        font-size: 12px;
        color: var(--text-faint);
    }
</style>

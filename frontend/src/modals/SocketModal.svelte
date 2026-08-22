<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import { PROTOCOLS } from "../lib/utils";
    import { untrack } from "svelte";
    import type { Socket } from "../lib/types";

    // Edit only. Adding a device goes through AddDeviceModal, which
    // discovers the device first and decides from its capabilities whether
    // it becomes a socket, some sensors, or both.
    interface Props { existing: Socket; }
    let { existing }: Props = $props();

    let name     = $state(untrack(() => existing.name     ?? ""));
    let room     = $state(untrack(() => existing.room     ?? ""));
    let code     = $state(untrack(() => existing.code     ?? ""));
    let protocol = $state(untrack(() => existing.protocol || "nexa"));
    let emoji    = $state(untrack(() => existing.emoji    ?? ""));
    let readOnly = $state(untrack(() => existing.readonly ?? false));

    // Quick-pick set shown in kid mode. Tapping the active one clears it.
    const EMOJI_CHOICES = ["💡", "🛏️", "🌟", "🚀", "🦕", "🐙", "🌈", "🎮", "📺", "🎄", "🔦", "🛋️"];

    const isTasmota  = $derived(protocol === "tasmota");
    const isMatter   = $derived(protocol === "matter" || protocol === "matter-thread");
    const isMqtt     = $derived(protocol === "mqtt");
    const isNexa     = $derived(protocol === "nexa");

    let probing      = $state(false);
    let publishing   = $state(false);
    let saving       = $state(false);
    let errors       = $state<{ name?: string; code?: string }>({});
    // What the last pair / probe / test-signal attempt said. These three
    // buttons ask the hardware a question, so their answer belongs under the
    // button that asked it and has to stay readable while the user walks over
    // to the socket — a toast that clears itself after 3.5s is the wrong
    // shape for that, and was the only reason they were toasts.
    let probeResult  = $state<{ ok: boolean; text: string } | null>(null);
    const clear = (k: "name" | "code") => { if (errors[k]) errors = { ...errors, [k]: undefined }; };

    async function testConnection() {
        if (probing) return;
        const ip = code.trim();
        if (!ip) {
            errors = { ...errors, code: "Type the device IP first." };
            return;
        }
        probing = true;
        probeResult = null;
        try {
            await api.tasmotaProbe(ip);
            probeResult = { ok: true, text: `Device found — Tasmota is responding at ${ip}.` };
        } catch (e) {
            probeResult = { ok: false, text: `No device found: ${(e as Error).message}` };
        } finally {
            probing = false;
        }
    }

    async function sendTestSignal() {
        if (publishing) return;
        const topic = code.trim();
        if (!topic) {
            errors = { ...errors, code: "Type the command topic first." };
            return;
        }
        publishing = true;
        probeResult = null;
        try {
            await api.mqttPublish({ topic, payload: "ON" });
            probeResult = { ok: true, text: `Sent ON to ${topic}. Did the device react?` };
        } catch (e) {
            probeResult = { ok: false, text: `Publish failed: ${(e as Error).message}` };
        } finally {
            publishing = false;
        }
    }

    async function save() {
        if (saving) return;
        const payload = { name: name.trim(), room: room.trim(), code: code.trim(), protocol, emoji, ...(readOnly ? { readonly: true } : {}) };
        const codeLabel = isTasmota ? "device IP"
                        : isMatter  ? "Matter node id"
                        : isMqtt    ? "command topic"
                        : "RF code";
        const errs: typeof errors = {};
        if (!payload.name) errs.name = "Give the device a name.";
        if (!payload.code) errs.code = `Enter the ${codeLabel}.`;
        errors = errs;
        if (errs.name || errs.code) return;
        saving = true;
        try {
            if (existing) {
                await api.updateSocket(existing.id, payload);
            } else {
                await api.createSocket(payload);
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

<!-- The answer to whichever of the three hardware questions was asked. One
     snippet, because only one of those buttons is ever on screen. Declared
     out here rather than inside <Modal>, where it would read as a prop. -->
{#snippet probeLine()}
    {#if probeResult}
        <p class="probe" class:bad={!probeResult.ok} role="status">{probeResult.text}</p>
    {/if}
{/snippet}

<Modal
    title="Edit device"
    subtitle="Update this device's details."
>
    {#snippet body()}
            <form onsubmit={(e) => { e.preventDefault(); save(); }}>
                <div class="field">
                    <label for="sock-name">Name</label>
                    <input id="sock-name" type="text" bind:value={name}
                        placeholder="e.g. Living room lamp" autocomplete="off" required
                        aria-invalid={errors.name ? "true" : undefined}
                        aria-describedby={errors.name ? "sock-name-err" : undefined}
                        oninput={() => clear("name")} />
                    {#if errors.name}<div id="sock-name-err" class="field-error">{errors.name}</div>{/if}
                </div>
                <div class="field" style="margin-top:var(--space-4)">
                    <label for="sock-room">Room <span class="opt">(optional)</span></label>
                    <select id="sock-room" bind:value={room}>
                        <option value="">No room</option>
                        {#each [...data.value.rooms].sort((a, b) => a.name.localeCompare(b.name)) as r (r.id)}
                            <option value={r.name}>{r.name}</option>
                        {/each}
                        {#if room && !data.value.rooms.some(r => r.name === room)}
                            <option value={room}>{room}</option>
                        {/if}
                    </select>
                </div>
                <label class="field-checkbox" style="margin-top:var(--space-4)">
                    <input type="checkbox" bind:checked={readOnly} />
                    <span>Sensor (read-only) — disables on/off toggle</span>
                </label>
                <div class="field" style="margin-top:var(--space-4)">
                    <span class="field-label">Icon <span class="opt">(for kid mode)</span></span>
                    <div class="emoji-grid" role="group" aria-label="Pick an icon">
                        {#each EMOJI_CHOICES as e (e)}
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
                            {#each PROTOCOLS as p (p.value)}
                                <option value={p.value}>{p.label}</option>
                            {/each}
                        </select>
                    </div>
                    <div class="field">
                        <label for="sock-code">
                            {isTasmota ? "Device IP" : isMatter ? "Matter node id" : isMqtt ? "Command topic" : "RF code"}
                        </label>
                        <input id="sock-code" type="text" bind:value={code}
                            placeholder={isTasmota ? "e.g. 192.168.1.50"
                                       : isMatter  ? "node id from commissioning"
                                       : isMqtt    ? "e.g. cmnd/plug/POWER"
                                       : isNexa    ? "e.g. 12345678:0"
                                       : "e.g. 12345"}
                            autocomplete="off" required
                            aria-invalid={errors.code ? "true" : undefined}
                            aria-describedby={errors.code ? "sock-code-err" : undefined}
                            oninput={() => clear("code")} />
                        {#if errors.code}<div id="sock-code-err" class="field-error">{errors.code}</div>{/if}
                        {#if isNexa && !errors.code}
                            <div class="field-help">
                                Format: <code>houseID:unit</code> — use <strong>Pair with socket</strong> below to fill this in automatically.
                            </div>
                        {/if}
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
                        {@render probeLine()}
                    </div>
                {:else if isMqtt}
                    <div class="field" style="margin-top:var(--space-3)">
                        <button type="button" class="btn btn-secondary" onclick={sendTestSignal} disabled={publishing}>
                            {publishing ? "Sending…" : "Send test signal"}
                        </button>
                        <div class="field-help">
                            Publishes <code>ON</code> to the command topic so you can confirm the
                            device reacts. The controller sends <code>ON</code>/<code>OFF</code> to
                            this exact topic — e.g. <code>cmnd/plug/POWER</code> for Tasmota.
                        </div>
                        {@render probeLine()}
                    </div>
                {/if}
            </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : "Save"}
        </button>
    {/snippet}
</Modal>

<style>
    .opt { color: var(--text-muted); font-weight: 400; font-size: 12px; }
    /* The hardware's answer, under the button that asked. Same size as the
       help text it follows — it's the same kind of sentence — with the tone
       carried by a colour rather than a badge. */
    .probe {
        margin: 6px 0 0;
        font-size: 13px;
        line-height: 1.5;
        color: var(--good);
    }
    .probe.bad { color: var(--bad); }
    .field-checkbox {
        display: flex; align-items: center; gap: 10px;
        font-size: 14px; cursor: pointer; padding: 2px 0;
    }
    .field-checkbox input[type="checkbox"] { width: 16px; height: 16px; flex-shrink: 0; cursor: pointer; }
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
</style>

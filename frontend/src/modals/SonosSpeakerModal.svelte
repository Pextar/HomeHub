<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { closeModal, openModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import type { SonosSpeaker, SonosCandidate } from "../lib/types";

    interface Props { existing?: SonosSpeaker | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    let name = $state(untrack(() => existing?.name ?? ""));
    let ip = $state(untrack(() => existing?.ip ?? ""));
    let room = $state(untrack(() => existing?.room ?? ""));
    let saving = $state(false);
    let errors = $state<{ ip?: string }>({});

    // ── LAN discovery (add mode only) ────────────────────────────────────
    let scanning = $state(false);
    let scanned = $state(false);
    let candidates = $state<SonosCandidate[]>([]);

    async function scan() {
        if (scanning) return;
        scanning = true;
        try {
            candidates = await api.sonosDiscover();
            scanned = true;
        } catch (e) {
            toasts.error("Scan failed", (e as Error).message);
        } finally {
            scanning = false;
        }
    }

    function pick(c: SonosCandidate) {
        ip = c.ip;
        if (!name.trim()) name = c.room;
        if (!room.trim()) room = c.room;
        errors = {};
    }

    async function save() {
        if (saving) return;
        if (!ip.trim()) {
            errors = { ip: "Enter the speaker's IP address, or pick one from the scan." };
            return;
        }
        saving = true;
        try {
            if (existing) {
                await api.sonosUpdateSpeaker(existing.id, { name, ip, room });
                toasts.success("Speaker updated");
            } else {
                // Name/room may be blank — the backend fills them from the
                // speaker's own zone name.
                await api.sonosCreateSpeaker({ ip, name, room });
                toasts.success("Speaker added");
            }
            closeModal(true);
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }

    async function remove() {
        if (!existing) return;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Remove speaker?",
            message: `Remove "${existing.name}" from HomeHub. The speaker itself is untouched.`,
            confirmLabel: "Remove",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.sonosDeleteSpeaker(existing.id);
            toasts.success("Speaker removed");
            closeModal(true);
        } catch (e) {
            toasts.error("Remove failed", (e as Error).message);
        }
    }
</script>

<Modal
    title={isEdit ? "Edit speaker" : "Add Sonos speaker"}
    subtitle={isEdit
        ? "Update how this speaker appears in HomeHub."
        : "Scan the network, or enter the speaker's IP directly."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            {#if !isEdit}
                <div class="scan-row">
                    <button type="button" class="btn btn-secondary" onclick={scan} disabled={scanning}>
                        <Icon name="search" size={14} />
                        {scanning ? "Scanning…" : scanned ? "Scan again" : "Scan network"}
                    </button>
                    {#if scanning}<span class="scan-hint mono">listening for speakers…</span>{/if}
                </div>
                {#if scanning}
                    <div class="skeleton cand-skeleton"></div>
                {:else if scanned && candidates.length === 0}
                    <div class="scan-empty">
                        No speakers answered. Sonos discovery uses multicast, which some
                        Wi-Fi setups block — entering the IP below always works.
                    </div>
                {:else if candidates.length > 0}
                    <div class="cands" role="listbox" aria-label="Discovered speakers">
                        {#each candidates as c (c.uuid)}
                            <button
                                type="button"
                                class="cand"
                                class:selected={ip === c.ip}
                                disabled={c.registered}
                                onclick={() => pick(c)}
                            >
                                <Icon name="speaker" size={18} />
                                <span class="cand-info">
                                    <span class="cand-name">{c.room || c.ip}</span>
                                    <span class="cand-sub mono">{c.model}{c.model ? " · " : ""}{c.ip}</span>
                                </span>
                                {#if c.registered}
                                    <span class="cand-tag mono">ADDED</span>
                                {:else if ip === c.ip}
                                    <Icon name="check" size={16} />
                                {/if}
                            </button>
                        {/each}
                    </div>
                {/if}
            {/if}

            <div class="field" style="margin-top:var(--space-4)">
                <label for="sonos-ip">IP address</label>
                <input id="sonos-ip" type="text" bind:value={ip} required placeholder="192.168.1.50"
                    class="mono"
                    aria-invalid={errors.ip ? "true" : undefined}
                    aria-describedby={errors.ip ? "sonos-ip-err" : undefined}
                    oninput={() => (errors = {})} />
                {#if errors.ip}<div id="sonos-ip-err" class="field-error">{errors.ip}</div>{/if}
            </div>

            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="sonos-name">Name</label>
                    <input id="sonos-name" type="text" bind:value={name}
                        placeholder={isEdit ? "" : "From the speaker"} />
                </div>
                <div class="field">
                    <label for="sonos-room">Room</label>
                    <input id="sonos-room" type="text" bind:value={room}
                        placeholder={isEdit ? "" : "From the speaker"} />
                </div>
            </div>
            {#if !isEdit}
                <div class="field-help" style="margin-top:var(--space-2)">
                    Leave name and room blank to use the speaker's own zone name.
                    The speaker must be reachable when you add it.
                </div>
            {/if}
        </form>
    {/snippet}
    {#snippet actions()}
        {#if isEdit}
            <button class="btn btn-ghost danger" onclick={remove}>Remove</button>
        {/if}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Add speaker"}
        </button>
    {/snippet}
</Modal>

<style>
    .danger { color: var(--danger); }
    .scan-row { display: flex; align-items: center; gap: var(--space-3); }
    .scan-hint { font-size: 11px; color: var(--text-mute); }
    .cand-skeleton { height: 56px; border-radius: var(--r-md); margin-top: var(--space-3); }
    .scan-empty {
        margin-top: var(--space-3);
        font-size: 12.5px;
        color: var(--text-mute);
        background: var(--card-2);
        border: 1px dashed var(--border);
        border-radius: var(--r-md);
        padding: var(--space-3);
    }
    .cands {
        margin-top: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: 6px;
    }
    .cand {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 10px 12px;
        min-height: 44px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        color: var(--text);
        cursor: pointer;
        text-align: left;
        font: inherit;
        transition: background 150ms ease, border-color 150ms ease;
    }
    .cand:hover:not(:disabled) { background: var(--card-3); }
    .cand.selected { border-color: var(--on); color: var(--on); }
    .cand:disabled { opacity: 0.5; cursor: default; }
    .cand-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .cand-name {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        color: var(--text);
    }
    .cand.selected .cand-name { color: var(--on); }
    .cand-sub { font-size: 11px; color: var(--text-mute); }
    .cand-tag {
        font-size: 10px;
        letter-spacing: 0.08em;
        color: var(--text-dim);
    }
</style>

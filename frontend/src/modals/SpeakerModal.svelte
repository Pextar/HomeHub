<script lang="ts">
    // Registering a speaker — Sonos or KEF. One sheet for both, because the
    // flow is identical (scan the LAN, or type the address; name and room are
    // optional and default to what the device calls itself) and a separate
    // "Add speaker" button per brand would put the least interesting decision
    // in the user's way before they have said anything else.
    //
    // The brand picker is chip filters, per DESIGN.md §2 — the Music subnav is
    // the one sanctioned segmented control and this isn't it. It only appears
    // when adding: an existing registration belongs to the bridge that owns
    // it, and moving one across brands would mean re-adding it anyway.
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { closeModal, openModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import type { SonosSpeaker, SonosCandidate, KEFSpeaker, KEFCandidate } from "../lib/types";

    export type SpeakerBrand = "sonos" | "kef";

    interface Props {
        existing?: SonosSpeaker | KEFSpeaker | null;
        /** Which bridge owns this speaker. Locked in edit mode. */
        brand?: SpeakerBrand;
    }
    let { existing = null, brand = "sonos" }: Props = $props();
    const isEdit = $derived(!!existing);

    let kind = $state<SpeakerBrand>(untrack(() => brand));
    let name = $state(untrack(() => existing?.name ?? ""));
    let ip = $state(untrack(() => existing?.ip ?? ""));
    let room = $state(untrack(() => existing?.room ?? ""));
    let saving = $state(false);
    let errors = $state<{ ip?: string }>({});

    const BRANDS: { value: SpeakerBrand; label: string }[] = [
        { value: "sonos", label: "Sonos" },
        { value: "kef", label: "KEF" },
    ];

    // ── LAN discovery (add mode only) ────────────────────────────────────
    // A candidate is normalised to what the picker needs, so the two bridges'
    // shapes (Sonos leads with its zone name, KEF with its device name) don't
    // leak into the markup.
    type Candidate = {
        key: string;
        ip: string;
        title: string;
        model: string;
        registered: boolean;
        /** Sonos knows a room for the speaker; KEF has no equivalent. */
        room?: string;
    };
    let scanning = $state(false);
    let scanned = $state(false);
    let candidates = $state<Candidate[]>([]);

    async function scan() {
        if (scanning) return;
        scanning = true;
        candidates = [];
        try {
            if (kind === "kef") {
                const found: KEFCandidate[] = await api.kefDiscover();
                candidates = found.map((c) => ({
                    key: c.mac || c.ip,
                    ip: c.ip,
                    title: c.name || c.ip,
                    model: c.model,
                    registered: c.registered,
                }));
            } else {
                const found: SonosCandidate[] = await api.sonosDiscover();
                candidates = found.map((c) => ({
                    key: c.uuid || c.ip,
                    ip: c.ip,
                    title: c.room || c.ip,
                    model: c.model,
                    registered: c.registered,
                    room: c.room,
                }));
            }
            scanned = true;
        } catch (e) {
            toasts.error("Scan failed", (e as Error).message);
        } finally {
            scanning = false;
        }
    }

    function pickBrand(b: SpeakerBrand) {
        if (kind === b) return;
        kind = b;
        // The other bridge's results say nothing about this one.
        candidates = [];
        scanned = false;
        errors = {};
    }

    function pick(c: Candidate) {
        ip = c.ip;
        if (!name.trim()) name = c.title;
        if (!room.trim() && c.room) room = c.room;
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
                if (kind === "kef") {
                    await api.kefUpdateSpeaker(existing.id, { name, ip, room });
                } else {
                    await api.sonosUpdateSpeaker(existing.id, { name, ip, room });
                }
            } else {
                // Name/room may be blank — the backend fills them from what
                // the speaker calls itself.
                if (kind === "kef") {
                    await api.kefCreateSpeaker({ ip, name, room });
                } else {
                    await api.sonosCreateSpeaker({ ip, name, room });
                }
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
            if (kind === "kef") {
                await api.kefDeleteSpeaker(existing.id);
            } else {
                await api.sonosDeleteSpeaker(existing.id);
            }
            closeModal(true);
        } catch (e) {
            toasts.error("Remove failed", (e as Error).message);
        }
    }

    const brandName = $derived(kind === "kef" ? "KEF" : "Sonos");
</script>

<Modal
    title={isEdit ? "Edit speaker" : "Add speaker"}
    subtitle={isEdit
        ? "Update how this speaker appears in HomeHub."
        : "Scan the network, or enter the speaker's IP directly."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            {#if !isEdit}
                <div class="chips" role="radiogroup" aria-label="Speaker type">
                    {#each BRANDS as b (b.value)}
                        <button
                            type="button"
                            class="chip" class:on={kind === b.value}
                            role="radio" aria-checked={kind === b.value}
                            onclick={() => pickBrand(b.value)}
                        >
                            {b.label}
                        </button>
                    {/each}
                </div>

                <div class="scan-row" style="margin-top:var(--space-3)">
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
                        No {brandName} speakers answered. Discovery uses multicast, which some
                        Wi-Fi setups block — entering the IP below always works.
                    </div>
                {:else if candidates.length > 0}
                    <div class="cands" role="listbox" aria-label="Discovered speakers">
                        {#each candidates as c (c.key)}
                            <button
                                type="button"
                                class="cand"
                                class:selected={ip === c.ip}
                                disabled={c.registered}
                                onclick={() => pick(c)}
                            >
                                <Icon name="speaker" size={18} />
                                <span class="cand-info">
                                    <span class="cand-name">{c.title}</span>
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
                <label for="speaker-ip">IP address</label>
                <input id="speaker-ip" type="text" bind:value={ip} required placeholder="192.168.1.50"
                    class="mono"
                    aria-invalid={errors.ip ? "true" : undefined}
                    aria-describedby={errors.ip ? "speaker-ip-err" : undefined}
                    oninput={() => (errors = {})} />
                {#if errors.ip}<div id="speaker-ip-err" class="field-error">{errors.ip}</div>{/if}
            </div>

            <div class="field-row" style="margin-top:var(--space-4)">
                <div class="field">
                    <label for="speaker-name">Name</label>
                    <input id="speaker-name" type="text" bind:value={name}
                        placeholder={isEdit ? "" : "From the speaker"} />
                </div>
                <div class="field">
                    <label for="speaker-room">Room</label>
                    <input id="speaker-room" type="text" bind:value={room}
                        placeholder={isEdit ? "" : "From the speaker"} />
                </div>
            </div>
            {#if !isEdit}
                <div class="field-help" style="margin-top:var(--space-2)">
                    Leave name and room blank to use the speaker's own name.
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
    .chips { display: flex; flex-wrap: wrap; gap: var(--space-2); }
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

<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores";
    import { sortedSockets } from "../lib/utils";
    import type { Scene } from "../lib/types";

    interface Props { existing?: Scene | null; }
    let { existing = null }: Props = $props();
    const isEdit = !!existing;

    const sockets = $derived(sortedSockets(data.value.sockets));

    // Map socket_id -> "ignore" | "on" | "off"
    const initial = new Map<string, "ignore" | "on" | "off">();
    if (existing) for (const a of existing.actions) initial.set(a.socket_id, a.action);

    let name = $state(existing?.name ?? "");
    let perSocket = $state<Record<string, "ignore" | "on" | "off">>(
        Object.fromEntries(sockets.map(s => [s.id, initial.get(s.id) ?? "ignore"]))
    );

    async function save() {
        const actions = Object.entries(perSocket)
            .filter(([, v]) => v !== "ignore")
            .map(([socket_id, action]) => ({ socket_id, action: action as "on" | "off" }));
        const payload = { name: name.trim(), actions };
        if (!payload.name) { toasts.warn("Missing name", "Give the scene a name."); return; }
        if (actions.length === 0) {
            toasts.warn("No actions", "Set at least one socket to On or Off.");
            return;
        }
        try {
            if (existing) {
                await api.updateScene(existing.id, payload);
                toasts.success("Scene updated", payload.name);
            } else {
                await api.createScene(payload);
                toasts.success("Scene created", payload.name);
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        }
    }
</script>

<Modal
    title={isEdit ? "Edit scene" : "New scene"}
    subtitle="A scene drives selected sockets to specific states in one tap."
    size="wide"
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="scn-name">Name</label>
                <input id="scn-name" type="text" bind:value={name}
                    placeholder="e.g. Movie night" autocomplete="off" required />
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label>Per-socket actions</label>
                <div class="picker">
                    {#each sockets as s (s.id)}
                        <div class="picker-row">
                            <div class="info">
                                <div>{s.name}</div>
                                <div class="field-help">{s.room || "Unassigned"}</div>
                            </div>
                            <select bind:value={perSocket[s.id]} aria-label="Action for {s.name}">
                                <option value="ignore">Ignore</option>
                                <option value="on">Turn on</option>
                                <option value="off">Turn off</option>
                            </select>
                        </div>
                    {/each}
                </div>
                <div class="field-help">Ignored sockets are not touched when the scene runs.</div>
            </div>
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save}>
            {isEdit ? "Save" : "Create scene"}
        </button>
    {/snippet}
</Modal>

<style>
    .picker {
        display: flex;
        flex-direction: column;
        gap: 4px;
        max-height: 360px;
        overflow-y: auto;
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: 4px;
        background: var(--surface);
    }
    .picker-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 8px 10px;
        border-radius: var(--radius-sm);
    }
    .info { flex: 1; min-width: 0; }
    .picker-row select {
        width: auto;
        padding: 4px 10px;
        font-size: 13px;
    }
</style>

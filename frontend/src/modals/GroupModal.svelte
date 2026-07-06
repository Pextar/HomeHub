<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets } from "../lib/utils";
    import { untrack } from "svelte";
    import { SvelteSet } from "svelte/reactivity";
    import type { Group } from "../lib/types";

    interface Props { existing?: Group | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));
    let name = $state(untrack(() => existing?.name ?? ""));
    let selected = new SvelteSet(untrack(() => existing?.socket_ids ?? []));
    let saving = $state(false);
    let nameError = $state("");

    function toggle(id: string) {
        if (selected.has(id)) selected.delete(id);
        else selected.add(id);
    }

    async function save() {
        if (saving) return;
        const payload = { name: name.trim(), socket_ids: [...selected] };
        if (!payload.name) { nameError = "Give the group a name."; return; }
        saving = true;
        try {
            if (existing) {
                await api.updateGroup(existing.id, payload);
                toasts.success("Group updated", payload.name);
            } else {
                await api.createGroup(payload);
                toasts.success("Group created", payload.name);
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

<Modal
    title={isEdit ? "Edit group" : "New group"}
    subtitle="Groups let you control multiple sockets in one tap."
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="grp-name">Name</label>
                <input id="grp-name" type="text" bind:value={name}
                    placeholder="e.g. Living room lights" autocomplete="off" required
                    aria-invalid={nameError ? "true" : undefined}
                    aria-describedby={nameError ? "grp-name-err" : undefined}
                    oninput={() => nameError = ""} />
                {#if nameError}<div id="grp-name-err" class="field-error">{nameError}</div>{/if}
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <span class="field-label">Members ({sockets.length} sockets)</span>
                <div class="picker">
                    {#each sockets as s (s.id)}
                        <label class="picker-row">
                            <input type="checkbox" checked={selected.has(s.id)}
                                onchange={() => toggle(s.id)} />
                            <div>
                                <div>{s.name}</div>
                                <div class="field-help">{s.room || "Unassigned"}</div>
                            </div>
                            <span class="meta">{s.code}</span>
                        </label>
                    {/each}
                </div>
                <div class="field-help">Toggle the sockets that belong to this group.</div>
            </div>
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Create group"}
        </button>
    {/snippet}
</Modal>

<style>
    .picker {
        display: flex;
        flex-direction: column;
        gap: 4px;
        max-height: 320px;
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
        cursor: pointer;
    }
    .picker-row:hover { background: var(--surface-hover); }
    .picker-row:active { background: var(--surface); }
    .picker-row input { width: auto; padding: 0; }
    .meta { color: var(--text-muted); font-size: 12px; margin-left: auto; }
    @media (pointer: coarse) {
        .picker-row { min-height: 44px; padding: 10px; }
    }
</style>

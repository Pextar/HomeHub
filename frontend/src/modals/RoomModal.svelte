<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import { untrack } from "svelte";
    import type { Room } from "../lib/types";

    interface Props { existing?: Room | null; }
    let { existing = null }: Props = $props();

    const isEdit = $derived(!!existing);
    let name  = $state(untrack(() => existing?.name ?? ""));
    let saving = $state(false);
    let nameError = $state("");

    async function save() {
        if (saving) return;
        nameError = name.trim() ? "" : "Give the room a name.";
        if (nameError) return;
        saving = true;
        try {
            if (isEdit) {
                await api.updateRoom(existing!.id, { name: name.trim() });
                toasts.success("Room renamed", name.trim());
            } else {
                await api.createRoom({ name: name.trim() });
                toasts.success("Room added", name.trim());
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
    title={isEdit ? "Rename room" : "Add room"}
    subtitle={isEdit ? "Update the room name." : "Create a new room to organise your devices."}
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="room-name">Name</label>
                <input id="room-name" type="text" bind:value={name}
                    placeholder="e.g. Living room" autocomplete="off" required
                    aria-invalid={nameError ? "true" : undefined}
                    aria-describedby={nameError ? "room-name-err" : undefined}
                    oninput={() => nameError = ""} />
                {#if nameError}<div id="room-name-err" class="field-error">{nameError}</div>{/if}
            </div>
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Rename" : "Add room"}
        </button>
    {/snippet}
</Modal>

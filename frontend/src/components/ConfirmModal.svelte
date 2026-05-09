<script lang="ts">
    import Modal from "./Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";

    interface Props {
        title: string;
        message: string;
        confirmLabel?: string;
        danger?: boolean;
    }
    let { title, message, confirmLabel = "Confirm", danger = false }: Props = $props();
</script>

<Modal {title}>
    {#snippet body()}
        <p style="color: var(--text-muted); font-size: 13px;">{message}</p>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal(false)}>Cancel</button>
        <button
            class="btn {danger ? 'btn-danger' : 'btn-primary'}"
            onclick={() => closeModal(true)}
        >{confirmLabel}</button>
    {/snippet}
</Modal>

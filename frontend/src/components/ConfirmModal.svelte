<script lang="ts">
    import Modal from "./Modal.svelte";
    import Icon from "./Icon.svelte";
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
        <div class="confirm">
            <div class="confirm-icon" class:danger aria-hidden="true">
                <Icon name={danger ? "trash" : "check"} size={22} />
            </div>
            <p class="confirm-msg">{message}</p>
        </div>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal(false)}>Cancel</button>
        <button
            class="btn {danger ? 'btn-danger' : 'btn-primary'}"
            onclick={() => closeModal(true)}
        >{confirmLabel}</button>
    {/snippet}
</Modal>

<style>
    .confirm {
        display: flex;
        flex-direction: column;
        align-items: center;
        text-align: center;
        gap: var(--space-3);
    }
    .confirm-icon {
        width: 48px;
        height: 48px;
        border-radius: var(--radius-md);
        display: grid;
        place-items: center;
        background: var(--primary-soft);
        color: var(--primary);
    }
    .confirm-icon.danger {
        background: var(--danger-soft);
        color: var(--danger);
    }
    .confirm-msg {
        color: var(--text-muted);
        font-size: 13.5px;
        line-height: 1.45;
        max-width: 300px;
    }
</style>

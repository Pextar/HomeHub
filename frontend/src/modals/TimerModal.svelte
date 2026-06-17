<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Segmented from "../components/Segmented.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import type { Socket, SocketAction } from "../lib/types";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    let action = $state<string>("off");
    let customMins = $state<number | null>(null);
    let customError = $state("");

    const presets = [
        { label: "1 min",  seconds: 60 },
        { label: "15 min", seconds: 15 * 60 },
        { label: "30 min", seconds: 30 * 60 },
        { label: "1 hour", seconds: 60 * 60 },
        { label: "2 hours", seconds: 2 * 60 * 60 },
        { label: "4 hours", seconds: 4 * 60 * 60 },
    ];

    async function fire(seconds: number, label: string) {
        try {
            await api.socketTimer(socket.id, { action: action as SocketAction, in_seconds: seconds, note: `Quick: ${label}` });
            toasts.success("Timer set", `${socket.name}: ${action} in ${label}`);
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        }
    }

    function submitCustom(e: Event) {
        e.preventDefault();
        const mins = customMins;
        if (!mins || mins <= 0) {
            customError = "Enter a positive number of minutes.";
            return;
        }
        customError = "";
        fire(mins * 60, `${mins} min`);
    }
</script>

<Modal
    title="Set a timer · {socket.name}"
    subtitle="Schedules a one-shot action and removes itself once it fires."
>
    {#snippet body()}
        <form onsubmit={submitCustom}>
            <div class="field">
                <span class="field-label">Action</span>
                <Segmented
                    name="timer-action"
                    bind:value={action}
                    options={[
                        { value: "off", label: "Turn off" },
                        { value: "on",  label: "Turn on" },
                        { value: "toggle", label: "Toggle" },
                    ]}
                />
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <span class="field-label">Quick presets</span>
                <div class="presets">
                    {#each presets as p (p.label)}
                        <button type="button" class="btn btn-secondary"
                            onclick={() => fire(p.seconds, p.label)}>{p.label}</button>
                    {/each}
                </div>
                <div class="field-help">Click a preset to set the timer immediately.</div>
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <label for="timer-custom">Custom</label>
                <div class="custom-row">
                    <input id="timer-custom" type="number" min="1"
                        placeholder="Minutes"
                        bind:value={customMins}
                        aria-invalid={customError ? "true" : undefined}
                        aria-describedby={customError ? "timer-custom-err" : undefined}
                        oninput={() => customError = ""} />
                    <button type="submit" class="btn btn-primary">Set custom timer</button>
                </div>
                {#if customError}<div id="timer-custom-err" class="field-error">{customError}</div>{/if}
                <div class="field-help">Pick any number of minutes.</div>
            </div>
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Close</button>
    {/snippet}
</Modal>

<style>
    .presets {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    .presets :global(.btn) { flex: 0 0 auto; }

    .custom-row { display: flex; gap: 8px; align-items: center; }
    .custom-row input { max-width: 160px; }

    /* Phones: stack so the long button label never crushes the input. */
    @media (max-width: 600px) {
        .custom-row { flex-direction: column; align-items: stretch; }
        .custom-row input { max-width: none; }
        .custom-row :global(.btn) { width: 100%; }
    }
</style>

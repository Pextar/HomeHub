<script lang="ts">
    // The push-status chip: "Live" when the speakers are pushing their own
    // changes, "Polling" when HomeHub is asking them on a timer. Tapping it
    // opens the sheet that explains the difference and can fix it.
    //
    // It lives in a component rather than in each view because it appears in
    // two places — the Music topbar and Home's "Playing now" head — and the
    // two must never drift into saying the same thing differently. Both are
    // chip contexts, so the shape carries over unchanged.
    import Icon from "./Icon.svelte";
    import SonosEventsModal from "../modals/SonosEventsModal.svelte";
    import { openModal } from "../lib/modal.svelte";

    interface Props {
        /** Whether event subscriptions are feeding the state on screen. */
        live: boolean;
        /**
         * Called after the sheet closes. Retrying inside it can turn
         * subscriptions on, which changes the poll interval the host view
         * should be running — so hosts re-read their status here.
         */
        onClosed?: () => void;
    }
    let { live, onClosed }: Props = $props();

    async function open() {
        await openModal(SonosEventsModal, {});
        onClosed?.();
    }
</script>

<button
    class="chip live-chip"
    class:on={live}
    onclick={open}
    aria-label={live
        ? "Live updates on — speakers push their changes. Open details"
        : "Live updates off — speakers are being polled. Open details"}
    title={live
        ? "Speakers push their changes — updates land in under a second"
        : "Speakers are being polled — updates take a few seconds"}
>
    <Icon name={live ? "bolt" : "radio"} size={14} />
    <span>{live ? "Live" : "Polling"}</span>
</button>

<style>
    /* Quieter than the action chip it sits beside — it reports, it doesn't
       ask to be pressed — so it stays a plain chip and only picks up the
       amber .on treatment when push is actually live. */
    .live-chip :global(svg) { flex-shrink: 0; }
    .live-chip span { font-variant-numeric: tabular-nums; }

    @media (max-width: 380px) {
        /* On the narrowest phones the label yields to the action chip beside
           it. The button keeps its aria-label, and squares up to the
           icon-only chip shape (§6.3) rather than staying a pill with
           nothing in it. */
        .live-chip span { display: none; }
        .live-chip { width: 36px; padding: 0; justify-content: center; }
    }
    @media (max-width: 380px) and (pointer: coarse) {
        /* §2: an icon-only control never drops under 44×44 on touch. */
        .live-chip { width: 44px; min-height: 44px; }
    }
</style>

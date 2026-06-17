<script lang="ts">
    import Icon from "./Icon.svelte";
    import AssistantChat from "./AssistantChat.svelte";
    import { assistant, session } from "../lib/stores.svelte";
    import { onMount } from "svelte";
    import { fade } from "svelte/transition";
    import { dur, sheet } from "../lib/motion";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";

    // Global launcher for the assistant overlay. The assistant is summoned
    // (FAB on mobile, Cmd/Ctrl-K + desktop rail entry) rather than being a
    // routed page, so it floats over whatever screen you're on and the thread
    // is never lost. The desktop surface is a bottom-right popover; mobile is a
    // bottom sheet.

    const open = $derived(assistant.open);
    const status = $derived(assistant.status);
    const unreachable = $derived(status?.enabled === true && status?.reachable === false);
    // The whole feature is admin-only (matches the backend gating), so non-admin
    // profiles never see the FAB or overlay.
    const visible = $derived(session.isAdmin);

    onMount(() => {
        assistant.loadStatus();
    });

    // Lock the page only behind the MOBILE sheet (a modal surface). The
    // desktop panel is a non-modal popover — the app stays scrollable and
    // clickable beside it — so no lock there.
    $effect(() => {
        if (open && window.matchMedia("(max-width: 900px)").matches) {
            lockBodyScroll();
            return () => unlockBodyScroll();
        }
    });

    function onKey(e: KeyboardEvent) {
        // Cmd/Ctrl-K toggles the assistant from anywhere.
        if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
            if (!visible) return;
            e.preventDefault();
            assistant.toggle();
            return;
        }
        if (e.key === "Escape" && open) {
            e.preventDefault();
            assistant.hide();
        }
    }

    const subtitle = $derived(
        status?.enabled
            ? unreachable ? "Model unreachable" : `Local · ${status.model ?? "llm"}`
            : "Local AI",
    );
</script>

<svelte:window onkeydown={onKey} />

{#if visible}
    <!-- Mobile FAB: lifted clear of the bottom tab bar. Hidden on desktop,
         where the rail entry + Cmd-K are the launchers. -->
    {#if !open}
        <button class="fab" aria-label="Open assistant" onclick={() => assistant.show()}>
            <Icon name="assistant" size={24} />
        </button>
    {/if}

    {#if open}
        <!-- Mobile: a dimmed modal backdrop, click-to-dismiss. Desktop: the
             same element becomes click-through (pointer-events:none) so the
             panel floats as a non-modal popover — you keep working beside it
             and close via the X, Esc, or Cmd-K. -->
        <div
            class="backdrop"
            role="presentation"
            onclick={(e) => { if (e.target === e.currentTarget) assistant.hide(); }}
            in:fade={{ duration: dur(160) }}
            out:fade={{ duration: dur(160) }}
        >
            <div
                class="surface"
                role="dialog"
                aria-modal="true"
                aria-label="Home assistant"
                in:sheet={{ duration: 320, mode: "slide" }}
                out:sheet={{ duration: 240, mode: "slide" }}
            >
                <header class="head">
                    <span class="head-mark"><Icon name="assistant" size={18} /></span>
                    <div class="head-text">
                        <span class="head-title">Assistant</span>
                        <span class="head-sub">{subtitle}</span>
                    </div>
                    {#if assistant.messages.length > 0}
                        <button class="head-btn" aria-label="Clear conversation" onclick={() => assistant.reset()}>
                            <Icon name="trash" size={16} />
                        </button>
                    {/if}
                    <button class="head-btn" aria-label="Close assistant" onclick={() => assistant.hide()}>
                        <Icon name="close" size={18} />
                    </button>
                </header>
                <div class="body">
                    <AssistantChat />
                </div>
            </div>
        </div>
    {/if}
{/if}

<style>
    /* ── FAB (mobile) ───────────────────────────────────────── */
    .fab {
        position: fixed;
        right: 16px;
        /* Clear the fixed bottom tab bar (≈60px) + safe area + extra breathing room. */
        bottom: calc(86px + env(safe-area-inset-bottom));
        width: 56px;
        height: 56px;
        border-radius: var(--r-pill);
        border: none;
        background: var(--on);
        color: var(--bg);
        display: grid;
        place-items: center;
        cursor: pointer;
        box-shadow: 0 8px 24px var(--on-glow), var(--shadow-md);
        z-index: 90;
        transition: transform 80ms ease, opacity 150ms ease;
    }
    .fab:active { transform: scale(0.93); }
    /* Desktop uses the rail entry + Cmd-K instead of a FAB. */
    @media (min-width: 901px) {
        .fab { display: none; }
    }

    /* ── Overlay ────────────────────────────────────────────── */
    .backdrop {
        position: fixed;
        inset: 0;
        z-index: 130;
        display: flex;
        background: rgba(10, 10, 8, 0.6);
        backdrop-filter: blur(3px);
        overscroll-behavior: contain;
        /* Mobile: dock to the bottom (sheet). */
        align-items: flex-end;
        justify-content: center;
    }
    :global([data-theme="light"]) .backdrop { background: rgba(40, 34, 24, 0.35); }

    .surface {
        display: flex;
        flex-direction: column;
        /* Stays interactive even when the backdrop is click-through (desktop). */
        pointer-events: auto;
        width: 100%;
        height: 82vh;
        background: var(--bg-2);
        border-top: 1px solid var(--hairline);
        border-top-left-radius: var(--r-xl);
        border-top-right-radius: var(--r-xl);
        box-shadow: var(--shadow-lg);
        overflow: hidden;
    }

    .head {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-4) var(--space-4) var(--space-3);
        border-bottom: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    .head-mark {
        display: grid;
        place-items: center;
        width: 32px;
        height: 32px;
        border-radius: var(--r-sm);
        background: var(--on-soft);
        color: var(--on);
        flex-shrink: 0;
    }
    .head-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .head-title { font-size: 15px; font-weight: 600; letter-spacing: -0.02em; color: var(--text); }
    .head-sub { font-family: var(--font-mono); font-size: 11px; color: var(--text-mute); }
    .head-btn {
        width: 36px;
        height: 36px;
        display: grid;
        place-items: center;
        border: none;
        background: transparent;
        border-radius: var(--r-sm);
        color: var(--text-mute);
        cursor: pointer;
        flex-shrink: 0;
        transition: background 150ms ease, color 150ms ease;
    }
    .head-btn:hover { background: var(--card-3); color: var(--text); }

    .body { flex: 1; min-height: 0; padding: 0 var(--space-4) var(--space-4); display: flex; }
    .body :global(.chat), .body :global(.notice) { width: 100%; }

    /* ── Desktop: bottom-right popover panel ────────────────── */
    @media (min-width: 901px) {
        .backdrop {
            /* Non-modal: no dim, and click-through so the app behind stays
               usable. The panel itself re-enables pointer events. */
            background: transparent;
            backdrop-filter: none;
            pointer-events: none;
            align-items: flex-end;
            justify-content: flex-end;
            padding: 0 28px 28px;
        }
        :global([data-theme="light"]) .backdrop { background: transparent; }
        .surface {
            width: 420px;
            max-width: calc(100vw - 56px);
            height: min(680px, 78vh);
            border: 1px solid var(--hairline);
            border-radius: var(--r-xl);
        }
    }
</style>

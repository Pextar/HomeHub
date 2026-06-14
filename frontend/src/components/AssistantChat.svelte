<script lang="ts">
    import Icon from "./Icon.svelte";
    import { assistant } from "../lib/stores.svelte";
    import { onMount, tick } from "svelte";
    import { fly } from "svelte/transition";
    import { dur } from "../lib/motion";

    // The conversation core, shared by the mobile sheet and desktop popover.
    // It fills its container (height:100%); the surrounding surface owns the
    // header (title/status/close). No page Topbar here.

    let draft = $state("");
    let scroller = $state<HTMLDivElement | undefined>();
    let input = $state<HTMLTextAreaElement | undefined>();

    const status = $derived(assistant.status);
    const messages = $derived(assistant.messages);
    const pending = $derived(assistant.pending);
    const disabled = $derived(status?.enabled === false);
    const unreachable = $derived(status?.enabled === true && status?.reachable === false);

    const EXAMPLES = [
        "Turn off the living room",
        "Is the kitchen lamp on?",
        "Activate movie night",
        "Turn everything off",
    ];

    onMount(() => {
        // Focus the input on open, except on touch (avoids an abrupt keyboard).
        if (!window.matchMedia("(pointer: coarse)").matches) {
            requestAnimationFrame(() => input?.focus());
        }
    });

    // Pin the thread to the newest content as it streams. Reading the last
    // message's length registers token-level reactivity.
    const tail = $derived(
        messages.length + (messages.at(-1)?.content.length ?? 0) + (messages.at(-1)?.tools?.length ?? 0),
    );
    $effect(() => {
        tail;
        pending;
        if (scroller) {
            tick().then(() => scroller?.scrollTo({ top: scroller.scrollHeight, behavior: "smooth" }));
        }
    });

    async function send() {
        const text = draft.trim();
        if (!text || assistant.streaming) return;
        draft = "";
        await assistant.send(text);
    }

    function onKeydown(e: KeyboardEvent) {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
        }
    }

    function useExample(text: string) {
        if (assistant.streaming) return;
        draft = "";
        assistant.send(text);
    }
</script>

{#if disabled}
    <div class="notice">
        <Icon name="assistant" size={20} />
        <div>
            <p class="notice-title">The assistant is turned off</p>
            <p class="notice-body">
                Enable it by setting <code>LLM_ENABLED=true</code> and pointing
                <code>OLLAMA_URL</code> at a local Ollama server, then restart the controller.
            </p>
        </div>
    </div>
{:else}
    <div class="chat">
        <div class="thread" bind:this={scroller}>
            {#if unreachable}
                <div class="notice warn">
                    <Icon name="bolt" size={18} />
                    <div>
                        <p class="notice-title">Can't reach the model</p>
                        <p class="notice-body">
                            Ollama isn't responding{status?.last_error ? ` (${status.last_error})` : ""}.
                            Make sure the <code>ollama</code> service is running on the Pi.
                        </p>
                    </div>
                </div>
            {/if}

            {#if messages.length === 0}
                <div class="empty">
                    <span class="spark"><Icon name="assistant" size={26} /></span>
                    <p class="empty-title">Ask your home anything</p>
                    <p class="empty-sub">Control devices and rooms, run scenes, or check a sensor — in plain language.</p>
                    <div class="examples">
                        {#each EXAMPLES as ex}
                            <button class="example" onclick={() => useExample(ex)}>{ex}</button>
                        {/each}
                    </div>
                </div>
            {/if}

            {#each messages as m, i (i)}
                <div class="row {m.role}" in:fly={{ y: 8, duration: dur(180) }}>
                    <div class="bubble {m.role}" class:error={m.error}>
                        {#if m.role === "assistant" && m.tools && m.tools.length}
                            <div class="tools">
                                {#each m.tools as t}
                                    <span class="tool-chip">
                                        <Icon name="check" size={13} />
                                        <span class="tool-name">{t.name}</span>
                                    </span>
                                {/each}
                            </div>
                        {/if}
                        {#if m.pending && !m.content}
                            <div class="typing" aria-label="Thinking">
                                <span class="skeleton line"></span>
                                <span class="skeleton line short"></span>
                            </div>
                        {:else}
                            <p class="text">{m.content}</p>
                        {/if}
                    </div>
                </div>
            {/each}

            {#if pending}
                <div class="row assistant" in:fly={{ y: 8, duration: dur(180) }}>
                    <div class="confirm">
                        <p class="confirm-summary">{pending.summary}</p>
                        {#if pending.affected && pending.affected.length}
                            <div class="affected">
                                {#each pending.affected.slice(0, 8) as name}
                                    <span class="affected-chip">{name}</span>
                                {/each}
                                {#if pending.affected.length > 8}
                                    <span class="affected-chip more mono">+{pending.affected.length - 8}</span>
                                {/if}
                            </div>
                        {/if}
                        <div class="confirm-actions">
                            <button class="btn-ghost" onclick={() => assistant.cancel()} disabled={assistant.streaming}>
                                Cancel
                            </button>
                            <button class="btn-amber" onclick={() => assistant.confirm()} disabled={assistant.streaming}>
                                Confirm
                            </button>
                        </div>
                    </div>
                </div>
            {/if}
        </div>

        <div class="composer">
            <textarea
                bind:this={input}
                bind:value={draft}
                onkeydown={onKeydown}
                placeholder="Message your home…"
                rows="1"
                aria-label="Message"
                disabled={assistant.streaming || unreachable}
            ></textarea>
            <button
                class="send"
                aria-label="Send"
                onclick={send}
                disabled={!draft.trim() || assistant.streaming || unreachable}
            >
                <Icon name="bolt" size={18} />
            </button>
        </div>
    </div>
{/if}

<style>
    .chat {
        display: flex;
        flex-direction: column;
        height: 100%;
        min-height: 0;
    }
    .thread {
        flex: 1;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding: var(--space-3) var(--space-1) var(--space-4);
        scroll-behavior: smooth;
        min-height: 0;
    }

    /* ── Messages ───────────────────────────────────────────── */
    .row { display: flex; }
    .row.user { justify-content: flex-end; }
    .row.assistant { justify-content: flex-start; }
    .bubble {
        max-width: 86%;
        padding: 12px 15px;
        border-radius: var(--r-lg);
        font-size: 14px;
        line-height: 1.5;
        border: 1px solid var(--hairline);
    }
    .bubble.user {
        background: var(--on-soft);
        border-color: rgba(245, 189, 110, 0.22);
        border-bottom-right-radius: var(--r-sm);
        color: var(--text);
    }
    .bubble.assistant {
        background: var(--card);
        border-bottom-left-radius: var(--r-sm);
        color: var(--text);
    }
    .bubble.error { border-color: var(--bad); color: var(--bad); }
    .text { margin: 0; white-space: pre-wrap; word-break: break-word; }

    .tools { display: flex; flex-wrap: wrap; gap: var(--space-1); margin-bottom: var(--space-2); }
    .tool-chip {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 3px 9px;
        border-radius: var(--r-pill);
        background: var(--card-3);
        color: var(--text-mute);
    }
    .tool-chip :global(svg) { color: var(--good); }
    .tool-name { font-family: var(--font-mono); font-size: 11px; letter-spacing: 0.02em; }

    /* Typing skeleton (no spinners — DESIGN §2). */
    .typing { display: flex; flex-direction: column; gap: 6px; min-width: 120px; }
    .skeleton.line { height: 11px; border-radius: var(--r-pill); width: 100%; }
    .skeleton.line.short { width: 55%; }

    /* ── Empty state ────────────────────────────────────────── */
    .empty {
        margin: auto;
        display: flex;
        flex-direction: column;
        align-items: center;
        text-align: center;
        gap: var(--space-2);
        padding: var(--space-6) var(--space-4);
        max-width: 460px;
    }
    .spark {
        display: grid;
        place-items: center;
        width: 56px;
        height: 56px;
        border-radius: var(--r-lg);
        background: var(--on-soft);
        color: var(--on);
        margin-bottom: var(--space-1);
    }
    .empty-title { margin: 0; font-size: 17px; font-weight: 600; letter-spacing: -0.02em; color: var(--text); }
    .empty-sub { margin: 0; font-size: 13px; color: var(--text-mute); line-height: 1.5; }
    .examples { display: flex; flex-wrap: wrap; gap: var(--space-2); justify-content: center; margin-top: var(--space-3); }
    .example {
        padding: 8px 14px;
        border-radius: var(--r-pill);
        background: var(--card);
        border: 1px solid var(--hairline);
        color: var(--text);
        font-size: 13px;
        cursor: pointer;
        transition: background 150ms ease, border-color 150ms ease;
    }
    .example:hover { background: var(--card-2); border-color: var(--border); }

    /* ── Confirmation card ──────────────────────────────────── */
    .confirm {
        max-width: 86%;
        padding: var(--space-4);
        border-radius: var(--r-lg);
        background: var(--card);
        border: 1px solid rgba(245, 189, 110, 0.28);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }
    .confirm-summary { margin: 0; font-size: 14.5px; font-weight: 600; color: var(--text); }
    .affected { display: flex; flex-wrap: wrap; gap: 6px; }
    .affected-chip { padding: 3px 9px; border-radius: var(--r-pill); background: var(--card-3); color: var(--text-mute); font-size: 12px; }
    .affected-chip.more { color: var(--text); }
    .mono { font-family: var(--font-mono); }
    .confirm-actions { display: flex; gap: var(--space-2); justify-content: flex-end; }
    .btn-ghost, .btn-amber {
        min-height: 44px;
        padding: 0 18px;
        border-radius: var(--r-md);
        font-size: 14px;
        font-weight: 600;
        cursor: pointer;
        border: 1px solid var(--hairline);
        transition: background 150ms ease, opacity 150ms ease;
    }
    .btn-ghost { background: transparent; color: var(--text); }
    .btn-ghost:hover { background: var(--card-2); }
    .btn-amber { background: var(--on); color: var(--bg); border-color: var(--on); }
    .btn-amber:hover { opacity: 0.9; }
    .btn-ghost:disabled, .btn-amber:disabled { opacity: 0.5; cursor: default; }

    /* ── Composer ───────────────────────────────────────────── */
    .composer {
        display: flex;
        align-items: flex-end;
        gap: var(--space-2);
        padding: var(--space-3);
        border: 1px solid var(--hairline);
        border-radius: var(--r-xl);
        background: var(--card);
        margin-top: var(--space-2);
    }
    textarea {
        flex: 1;
        resize: none;
        border: none;
        background: transparent;
        color: var(--text);
        font-family: var(--font-sans);
        font-size: 15px;
        line-height: 1.5;
        max-height: 140px;
        padding: 10px 8px;
        outline: none;
    }
    textarea::placeholder { color: var(--text-dim); }
    textarea:disabled { opacity: 0.6; }
    .send {
        flex-shrink: 0;
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: none;
        border-radius: var(--r-md);
        background: var(--on);
        color: var(--bg);
        cursor: pointer;
        transition: opacity 150ms ease, transform 80ms ease;
    }
    .send:hover { opacity: 0.9; }
    .send:active { transform: scale(0.94); }
    .send:disabled { background: var(--card-3); color: var(--text-dim); cursor: default; }

    /* ── Notices ────────────────────────────────────────────── */
    .notice {
        display: flex;
        gap: var(--space-3);
        padding: var(--space-4);
        border-radius: var(--r-lg);
        background: var(--card);
        border: 1px solid var(--hairline);
        color: var(--text-mute);
    }
    .notice :global(svg) { color: var(--on); flex-shrink: 0; margin-top: 2px; }
    .notice.warn { border-color: rgba(232, 185, 107, 0.3); margin-bottom: var(--space-3); }
    .notice-title { margin: 0 0 4px; font-size: 14px; font-weight: 600; color: var(--text); }
    .notice-body { margin: 0; font-size: 13px; line-height: 1.5; }
    code {
        font-family: var(--font-mono);
        font-size: 12px;
        background: var(--card-3);
        padding: 1px 5px;
        border-radius: 5px;
        color: var(--text);
    }

    /* Prevent iOS auto-zoom on focus (DESIGN: 16px min on touch). */
    @media (pointer: coarse) {
        textarea { font-size: 16px; }
    }
</style>

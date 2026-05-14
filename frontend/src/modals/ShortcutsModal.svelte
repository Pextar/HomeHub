<script lang="ts">
    import { onMount } from "svelte";
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets } from "../lib/utils";
    import { copyText } from "../lib/clipboard";

    const origin = window.location.origin;
    const v = $derived(data.value);
    const sockets = $derived(sortedSockets(v.sockets));
    const scenes = $derived([...v.scenes].sort((a, b) => a.name.localeCompare(b.name)));
    const groups = $derived([...v.groups].sort((a, b) => a.name.localeCompare(b.name)));

    let authHeader = $state("");
    let authLoaded = $state(false);

    onMount(async () => {
        try {
            const r = await api.shortcutAuth();
            authHeader = r.header;
        } catch {
            // Non-fatal — the URLs are still useful without the header.
        }
        authLoaded = true;
    });

    function url(path: string) {
        return origin + path;
    }

    async function copy(text: string, label: string) {
        const ok = await copyText(text);
        if (ok) toasts.success("Copied", label);
        else toasts.warn("Couldn't copy", "Long-press the text to select it manually.");
    }
</script>

<Modal
    title="iOS Shortcuts"
    subtitle="Build home-screen buttons that control your sockets"
    size="wide"
>
    {#snippet body()}
        <p class="lead">
            iOS can't show a true widget from a web app, but you can build a
            <strong>Shortcut</strong> for each action below, then add them to
            your home screen via the Shortcuts widget. Every request is a
            <code>POST</code>; they only work while you're on the same WiFi as
            the Pi.
        </p>

        {#if authLoaded && authHeader}
            <div class="block">
                <div class="block-head">
                    <span class="field-label">Authorization header</span>
                    <button class="act-btn" onclick={() => copy(authHeader, "Authorization header")}>Copy</button>
                </div>
                <code class="mono">{authHeader}</code>
                <div class="field-help">
                    In each Shortcut, add a header named <code>Authorization</code> with this value.
                </div>
            </div>
        {/if}

        {#if sockets.length}
            <div class="block">
                <span class="field-label">Sockets</span>
                {#each sockets as s (s.id)}
                    <div class="row">
                        <div class="name" title={s.room || "Unassigned"}>{s.name}</div>
                        <div class="acts">
                            <button class="act-btn" onclick={() => copy(url(`/api/sockets/${encodeURIComponent(s.id)}/toggle`), `${s.name} · toggle`)}>Toggle</button>
                            <button class="act-btn" onclick={() => copy(url(`/api/sockets/${encodeURIComponent(s.id)}/on`), `${s.name} · on`)}>On</button>
                            <button class="act-btn" onclick={() => copy(url(`/api/sockets/${encodeURIComponent(s.id)}/off`), `${s.name} · off`)}>Off</button>
                        </div>
                    </div>
                {/each}
            </div>
        {/if}

        {#if scenes.length}
            <div class="block">
                <span class="field-label">Scenes</span>
                {#each scenes as sc (sc.id)}
                    <div class="row">
                        <div class="name">{sc.name}</div>
                        <div class="acts">
                            <button class="act-btn" onclick={() => copy(url(`/api/scenes/${encodeURIComponent(sc.id)}/activate`), `${sc.name} · activate`)}>Activate</button>
                        </div>
                    </div>
                {/each}
            </div>
        {/if}

        {#if groups.length}
            <div class="block">
                <span class="field-label">Groups</span>
                {#each groups as g (g.id)}
                    <div class="row">
                        <div class="name">{g.name}</div>
                        <div class="acts">
                            <button class="act-btn" onclick={() => copy(url(`/api/groups/${encodeURIComponent(g.id)}/toggle`), `${g.name} · toggle`)}>Toggle</button>
                            <button class="act-btn" onclick={() => copy(url(`/api/groups/${encodeURIComponent(g.id)}/on`), `${g.name} · on`)}>On</button>
                            <button class="act-btn" onclick={() => copy(url(`/api/groups/${encodeURIComponent(g.id)}/off`), `${g.name} · off`)}>Off</button>
                        </div>
                    </div>
                {/each}
            </div>
        {/if}

        <details class="howto">
            <summary>How to build a Shortcut</summary>
            <ol>
                <li>Open the <strong>Shortcuts</strong> app → <strong>+</strong> to create a new one.</li>
                <li>Add the action <strong>Get Contents of URL</strong>.</li>
                <li>Paste a copied URL into the URL field.</li>
                <li>Expand the action: set <strong>Method</strong> to <code>POST</code>.</li>
                <li>Under <strong>Headers</strong>, add <code>Authorization</code> and paste the header value above.</li>
                <li>Name the Shortcut, then tap <strong>Done</strong>.</li>
                <li>On your home screen, add a <strong>Shortcuts</strong> widget — your buttons appear as tappable tiles.</li>
            </ol>
        </details>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>

<style>
    .lead { color: var(--text-muted); font-size: 13px; line-height: 1.6; }
    .lead code, .howto code, .field-help code {
        font-family: var(--font-mono);
        font-size: 0.92em;
        background: var(--surface);
        padding: 1px 5px;
        border-radius: var(--radius-sm);
    }

    .block {
        display: flex;
        flex-direction: column;
        gap: 6px;
        padding: var(--space-3);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
    }
    .block-head { display: flex; align-items: center; justify-content: space-between; }
    .mono {
        font-family: var(--font-mono);
        font-size: 12px;
        word-break: break-all;
        color: var(--text);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: 8px 10px;
        user-select: all;
        -webkit-user-select: all;
    }

    .row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 6px 0;
    }
    .row + .row { border-top: 1px solid var(--border); }
    .name { flex: 1; min-width: 0; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .acts { display: flex; gap: 4px; flex-shrink: 0; }

    .act-btn {
        font: inherit;
        font-size: 12px;
        font-weight: 600;
        padding: 5px 10px;
        border-radius: var(--radius-sm);
        border: 1px solid var(--border-strong);
        background: var(--bg-elevated);
        color: var(--text);
        cursor: pointer;
        transition: background var(--t-fast);
    }
    .act-btn:hover { background: var(--surface-hover); }
    .act-btn:active { transform: translateY(1px); }

    .howto { font-size: 13px; color: var(--text-muted); }
    .howto summary { cursor: pointer; font-weight: 600; color: var(--text); }
    .howto ol { margin: var(--space-3) 0 0; padding-left: 1.3em; display: flex; flex-direction: column; gap: 6px; line-height: 1.5; }
</style>

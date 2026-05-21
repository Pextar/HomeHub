<script lang="ts">
    import { onMount, type Snippet } from "svelte";
    import { api, ApiError } from "../lib/api";

    interface Props {
        onAuthed?: () => void;
        children?: Snippet;
    }
    let { onAuthed, children }: Props = $props();

    type Phase = "checking" | "needs-login" | "authed" | "logging-in";
    let phase: Phase = $state("checking");
    // "code" is the default for everyday (limited) users; "admin" reveals the
    // username + password form.
    let mode: "code" | "admin" = $state("code");
    let code = $state("");
    let username = $state("");
    let password = $state("");
    let error = $state("");

    // Focus a field as soon as it mounts (initial render and on mode switch)
    // so the user can start typing — and on mobile the keyboard opens right
    // away, which is what people expect on a login screen.
    function autofocus(node: HTMLInputElement) {
        node.focus();
    }

    // On boot, probe a protected endpoint. 401 → show login. Anything else
    // (success or network error) → render the app and let it deal with it.
    onMount(async () => {
        try {
            await api.health();
            phase = "authed";
            onAuthed?.();
        } catch (e) {
            if (e instanceof ApiError && e.status === 401) {
                phase = "needs-login";
                return;
            }
            phase = "authed";
            onAuthed?.();
        }
    });

    async function submit(e: Event) {
        e.preventDefault();
        const body =
            mode === "code"
                ? (code.trim() ? { code: code.trim() } : null)
                : (username && password ? { username, password } : null);
        if (!body) return;
        error = "";
        phase = "logging-in";
        try {
            await api.login(body);
            password = "";
            code = "";
            phase = "authed";
            onAuthed?.();
        } catch (e) {
            error = mode === "code"
                ? "Try again! Ask a grown-up if you need help."
                : (e instanceof ApiError ? e.message : "Sign in failed");
            phase = "needs-login";
        }
    }
</script>

{#if phase === "authed"}
    {@render children?.()}
{:else if phase === "checking"}
    <div class="screen"><div class="splash">Loading…</div></div>
{:else}
    <div class="screen">
        <form class="card" onsubmit={submit}>
            <div class="head">
                <h1>HomeHub</h1>
                <p class="sub">{mode === "code" ? "Enter your login code" : "Sign in as admin"}</p>
            </div>

            {#if mode === "code"}
                <div class="field">
                    <label for="login-code">Login code</label>
                    <input id="login-code" type="text" bind:value={code}
                        inputmode="numeric" autocomplete="one-time-code"
                        autocapitalize="none" autocorrect="off"
                        placeholder="000000" class="code-input" required
                        use:autofocus />
                </div>
            {:else}
                <div class="field">
                    <label for="login-user">Username</label>
                    <input id="login-user" type="text" bind:value={username}
                        autocomplete="username" autocapitalize="none" autocorrect="off"
                        required use:autofocus />
                </div>
                <div class="field">
                    <label for="login-pass">Password</label>
                    <input id="login-pass" type="password" bind:value={password}
                        autocomplete="current-password" required />
                </div>
            {/if}

            {#if error}<div class="error" role="alert">{error}</div>{/if}
            <button class="btn btn-primary" type="submit" disabled={phase === "logging-in"}>
                {phase === "logging-in" ? "Signing in…" : "Sign in"}
            </button>

            <button type="button" class="switch-mode"
                onclick={() => { mode = mode === "code" ? "admin" : "code"; error = ""; }}>
                {mode === "code" ? "Sign in as admin" : "Use a login code instead"}
            </button>
        </form>
    </div>
{/if}

<style>
    .screen {
        min-height: 100vh;
        display: grid;
        place-items: center;
        padding: var(--space-4);
        padding-bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
    }
    .card {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-6);
        width: 100%;
        max-width: 380px;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        box-shadow: var(--shadow-md);
    }
    .head { display: flex; flex-direction: column; gap: 4px; }
    .card h1 { font-size: 1.25rem; }
    .sub { color: var(--text-muted); font-size: 13px; }
    .field { display: flex; flex-direction: column; gap: 6px; }
    .field label { font-size: 13px; color: var(--text-muted); }
    .error {
        background: var(--danger-soft);
        color: var(--danger);
        padding: 8px 12px;
        border-radius: var(--radius-md);
        font-size: 13px;
    }
    .splash { color: var(--text-muted); }
    .code-input {
        font-size: 1.5rem;
        letter-spacing: 0.4em;
        text-align: center;
        font-variant-numeric: tabular-nums;
    }
    .switch-mode {
        background: none;
        border: none;
        color: var(--text-muted);
        font-size: 13px;
        cursor: pointer;
        padding: 12px 4px; /* generous vertical padding = easy to tap */
        text-align: center;
        touch-action: manipulation;
    }
    .switch-mode:hover { color: var(--text); text-decoration: underline; }

    /* On phones, open the card to full width and anchor it toward the top
       (above the keyboard, which rises from the bottom). */
    @media (max-width: 480px) {
        .screen { align-items: flex-start; padding-top: max(env(safe-area-inset-top), var(--space-8)); }
        .card { border-radius: var(--radius-xl); padding: var(--space-6) var(--space-5); }
        .card h1 { font-size: 1.5rem; font-weight: 800; }
    }
</style>

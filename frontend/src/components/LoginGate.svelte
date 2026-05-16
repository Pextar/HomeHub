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
    let username = $state("");
    let password = $state("");
    let error = $state("");

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
        if (!username || !password) return;
        error = "";
        phase = "logging-in";
        try {
            await api.login({ username, password });
            password = "";
            phase = "authed";
            onAuthed?.();
        } catch (e) {
            error = (e instanceof ApiError ? e.message : "Sign in failed");
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
                <h1>RF Sockets</h1>
                <p class="sub">Sign in to continue</p>
            </div>
            <div class="field">
                <label for="login-user">Username</label>
                <input id="login-user" type="text" bind:value={username}
                    autocomplete="username" autocapitalize="none" autocorrect="off"
                    required />
            </div>
            <div class="field">
                <label for="login-pass">Password</label>
                <input id="login-pass" type="password" bind:value={password}
                    autocomplete="current-password" required />
            </div>
            {#if error}<div class="error" role="alert">{error}</div>{/if}
            <button class="btn btn-primary" type="submit" disabled={phase === "logging-in"}>
                {phase === "logging-in" ? "Signing in…" : "Sign in"}
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
</style>

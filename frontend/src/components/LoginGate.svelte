<script lang="ts">
    import { onMount, type Snippet } from "svelte";
    import { api, ApiError } from "../lib/api";

    interface Props {
        onAuthed?: () => void;
        children?: Snippet;
    }
    let { onAuthed, children }: Props = $props();

    type Phase = "checking" | "needs-login" | "authed" | "logging-in"
               | "invite-lookup" | "invite-set-password" | "invite-submitting" | "invite-invalid";
    let phase: Phase = $state("checking");
    // "code" is the default for everyday (limited) users; "admin" reveals the
    // username + password form.
    let mode: "code" | "admin" = $state("code");
    let code = $state("");
    let username = $state("");
    let password = $state("");
    let error = $state("");

    // Invite flow state
    let inviteToken = $state("");
    let inviteUsername = $state("");
    let newPassword = $state("");
    let confirmPassword = $state("");
    let showNewPassword = $state(false);
    let inviteError = $state("");

    // Focus a field as soon as it mounts (initial render and on mode switch)
    // so the user can start typing — and on mobile the keyboard opens right
    // away, which is what people expect on a login screen.
    function autofocus(node: HTMLInputElement) {
        node.focus();
    }

    // On boot, check whether the URL carries an invite token. If so, verify
    // it before showing the password-set form. Otherwise probe auth as usual.
    onMount(async () => {
        const params = new URLSearchParams(window.location.search);
        const token = params.get("invite");
        if (token) {
            inviteToken = token;
            phase = "invite-lookup";
            try {
                const result = await api.lookupInvite(token);
                inviteUsername = result.username;
                phase = "invite-set-password";
            } catch {
                phase = "invite-invalid";
            }
            return;
        }

        // Normal auth probe
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
            // Remove any ?invite= from the URL so a page reload doesn't
            // re-trigger the invite flow.
            clearInviteParam();
            phase = "authed";
            onAuthed?.();
        } catch (e) {
            error = mode === "code"
                ? "Try again! Ask a grown-up if you need help."
                : (e instanceof ApiError ? e.message : "Sign in failed");
            phase = "needs-login";
        }
    }

    async function submitInvite(e: Event) {
        e.preventDefault();
        inviteError = "";
        if (newPassword.length < 8) {
            inviteError = "Password must be at least 8 characters.";
            return;
        }
        if (newPassword !== confirmPassword) {
            inviteError = "Passwords don't match.";
            return;
        }
        phase = "invite-submitting";
        try {
            await api.acceptInvite(inviteToken, newPassword);
            // Server set a session cookie — clear the invite param and boot the app.
            clearInviteParam();
            phase = "authed";
            onAuthed?.();
        } catch (e) {
            inviteError = e instanceof ApiError ? e.message : "Something went wrong. Try again.";
            phase = "invite-set-password";
        }
    }

    function clearInviteParam() {
        const url = new URL(window.location.href);
        url.searchParams.delete("invite");
        window.history.replaceState({}, "", url.toString());
    }
</script>

{#if phase === "authed"}
    {@render children?.()}
{:else if phase === "checking" || phase === "invite-lookup"}
    <div class="screen"><div class="splash">Loading…</div></div>
{:else if phase === "invite-invalid"}
    <div class="screen">
        <div class="card">
            <div class="head">
                <h1>HomeHub</h1>
                <p class="sub">Invite link invalid</p>
            </div>
            <div class="error" role="alert">
                This invite link is invalid or has expired. Ask the system owner to send you a new one.
            </div>
            <button class="btn btn-primary" onclick={() => { clearInviteParam(); phase = "needs-login"; }}>
                Go to sign in
            </button>
        </div>
    </div>
{:else if phase === "invite-set-password" || phase === "invite-submitting"}
    <div class="screen">
        <form class="card" onsubmit={submitInvite}>
            <div class="head">
                <h1>HomeHub</h1>
                <p class="sub">Welcome, {inviteUsername}! Set your password to get started.</p>
            </div>

            <div class="field">
                <label for="inv-pass">Password</label>
                <div class="pass-wrap">
                    <input id="inv-pass" type={showNewPassword ? "text" : "password"}
                        bind:value={newPassword}
                        autocomplete="new-password"
                        placeholder="At least 8 characters"
                        required use:autofocus />
                    <button type="button" class="show-btn"
                        onclick={() => showNewPassword = !showNewPassword}
                        aria-label={showNewPassword ? "Hide password" : "Show password"}>
                        {showNewPassword ? "Hide" : "Show"}
                    </button>
                </div>
            </div>
            <div class="field">
                <label for="inv-confirm">Confirm password</label>
                <input id="inv-confirm" type={showNewPassword ? "text" : "password"}
                    bind:value={confirmPassword}
                    autocomplete="new-password"
                    placeholder="Repeat password"
                    required />
            </div>

            {#if inviteError}<div class="error" role="alert">{inviteError}</div>{/if}
            <button class="btn btn-primary" type="submit" disabled={phase === "invite-submitting"}>
                {phase === "invite-submitting" ? "Setting password…" : "Set password & sign in"}
            </button>
        </form>
    </div>
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
    .pass-wrap { position: relative; }
    .pass-wrap input { padding-right: 52px; }
    .show-btn {
        position: absolute;
        right: 8px;
        top: 50%;
        transform: translateY(-50%);
        background: none;
        border: none;
        padding: 4px 6px;
        cursor: pointer;
        color: var(--text-muted);
        font-size: 12px;
        border-radius: var(--radius-sm);
    }
    .show-btn:hover { color: var(--text); }

    /* On phones, open the card to full width and anchor it toward the top
       (above the keyboard, which rises from the bottom). */
    @media (max-width: 480px) {
        .screen { align-items: flex-start; padding-top: max(env(safe-area-inset-top), var(--space-8)); }
        .card { border-radius: var(--radius-xl); padding: var(--space-6) var(--space-5); }
        .card h1 { font-size: 1.5rem; font-weight: 800; }
    }
</style>

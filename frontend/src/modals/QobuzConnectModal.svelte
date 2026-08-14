<script lang="ts">
    /**
     * Qobuz — the one service HomeHub can play losslessly, and the two-step
     * setup that gets it there.
     *
     * The shape of this sheet is dictated by a fact about Qobuz rather than by
     * preference: **the two credentials come from two different parties.** The
     * app id and secret are issued to the *application* — Qobuz hands them out
     * on request — and belong to this installation. The email and password are
     * the listener's. HomeHub ships no app credentials, because embedding
     * someone else's would be both a licence breach and a secret in a public
     * repository.
     *
     * So the sheet is two numbered steps, and the second is disabled until the
     * first is done. A single form with four fields would let someone fill in
     * their Qobuz password, fail on a missing app id, and have no idea which
     * half was wrong — the backend keeps the two errors separate precisely so
     * this can.
     *
     * What it must never do is claim a quality it hasn't got. The plan line
     * shows the *entitlement's ceiling*, in Qobuz's own words, because a track
     * arrives at its own rate within that: a 16-bit album on a hi-res plan is
     * 16-bit, and a "24-bit" badge over it would be the same lie the quality
     * sheet exists to avoid.
     */
    import { onMount } from "svelte";
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import type { QobuzStatus } from "../lib/types";

    let status = $state<QobuzStatus | null>(null);
    let loaded = $state(false);

    let appId = $state("");
    let appSecret = $state("");
    let email = $state("");
    let password = $state("");

    let savingApp = $state(false);
    let signingIn = $state(false);
    let appError = $state("");
    let loginError = $state("");

    const configured = $derived(status?.configured ?? false);
    const connected = $derived(status?.connected ?? false);

    async function refresh() {
        try {
            status = await api.qobuzStatus();
        } catch (e) {
            if (!loaded) toasts.error("Couldn't read Qobuz status", (e as Error).message);
        } finally {
            loaded = true;
        }
    }

    onMount(refresh);

    async function saveApp() {
        if (savingApp || !appId.trim() || !appSecret.trim()) return;
        savingApp = true;
        appError = "";
        try {
            status = await api.qobuzSetConfig(appId.trim(), appSecret.trim());
            // Cleared on success: they are stored server-side now, and leaving
            // a secret sitting in a field is how it ends up in a screenshot.
            appId = "";
            appSecret = "";
        } catch (e) {
            appError = (e as Error).message;
        } finally {
            savingApp = false;
        }
    }

    async function signIn() {
        if (signingIn || !email.trim() || !password) return;
        signingIn = true;
        loginError = "";
        try {
            status = await api.qobuzLogin(email.trim(), password);
            // The password is never stored anywhere — not here, not on the
            // server, which keeps only the token Qobuz returned for it.
            password = "";
        } catch (e) {
            loginError = (e as Error).message;
        } finally {
            signingIn = false;
        }
    }

    async function disconnect() {
        try {
            status = await api.qobuzDisconnect();
        } catch (e) {
            toasts.error("Couldn't disconnect", (e as Error).message);
        }
    }
</script>

<Modal
    title="Qobuz"
    subtitle="The one service HomeHub plays bit-exact, end to end."
>
    {#snippet body()}
        {#if !loaded}
            <div class="skeleton q-skeleton"></div>
        {:else}
            {#if connected}
                <!-- ── Connected ────────────────────────────────────────────
                     The ceiling, not a promise about any given track. -->
                <div class="q-card">
                    <div class="q-card-head">
                        <span class="q-who">{status?.display_name || "Signed in"}</span>
                        <span class="q-tag mono lossless">Lossless</span>
                    </div>
                    {#if status?.plan}
                        <p class="q-line">
                            <span class="q-key">Plan</span>
                            <span class="q-val">{status.plan}</span>
                        </p>
                    {/if}
                    {#if status?.max_format_label}
                        <p class="q-line">
                            <span class="q-key">Tops out at</span>
                            <span class="q-val mono">{status.max_format_label}</span>
                        </p>
                    {/if}
                    <p class="q-note">
                        Each track arrives at its own rate within that. HomeHub
                        fetches the FLAC and decodes it here — the samples that
                        reach your speakers are the master, not a re-encode.
                    </p>
                </div>
                <button class="btn btn-ghost q-disconnect" onclick={disconnect}>
                    Sign out of Qobuz
                </button>
            {:else}
                <!-- ── Step 1 ───────────────────────────────────────────── -->
                <div class="eyrow">1 · App credentials</div>
                <p class="q-note">
                    Issued by Qobuz to the application, not to you — request
                    them from <span class="mono">api@qobuz.com</span>. HomeHub
                    ships none of its own.
                </p>
                {#if configured}
                    <p class="q-done">
                        <Icon name="check" size={13} />
                        <span>Stored. Enter new ones to replace them.</span>
                    </p>
                {/if}
                <div class="q-fields">
                    <label class="q-field">
                        <span>App ID</span>
                        <input
                            type="text"
                            bind:value={appId}
                            autocomplete="off"
                            aria-invalid={appError ? "true" : undefined}
                        />
                    </label>
                    <label class="q-field">
                        <span>App secret</span>
                        <input
                            type="password"
                            bind:value={appSecret}
                            autocomplete="off"
                            aria-invalid={appError ? "true" : undefined}
                        />
                    </label>
                </div>
                {#if appError}<p class="q-error">{appError}</p>{/if}
                <button
                    class="btn"
                    disabled={savingApp || !appId.trim() || !appSecret.trim()}
                    onclick={saveApp}
                >
                    {savingApp ? "Saving…" : configured ? "Replace credentials" : "Save credentials"}
                </button>

                <!-- ── Step 2 ───────────────────────────────────────────── -->
                <div class="eyrow" style="margin-top:var(--space-5)">2 · Your account</div>
                <p class="q-note">
                    Your password is sent once to get a session token and is
                    never stored — not by the browser, not by HomeHub.
                </p>
                <div class="q-fields" class:q-locked={!configured}>
                    <label class="q-field">
                        <span>Email</span>
                        <input
                            type="email"
                            bind:value={email}
                            disabled={!configured}
                            autocomplete="username"
                        />
                    </label>
                    <label class="q-field">
                        <span>Password</span>
                        <input
                            type="password"
                            bind:value={password}
                            disabled={!configured}
                            autocomplete="current-password"
                        />
                    </label>
                </div>
                {#if loginError}<p class="q-error">{loginError}</p>{/if}
                <button
                    class="btn"
                    disabled={!configured || signingIn || !email.trim() || !password}
                    onclick={signIn}
                >
                    {signingIn ? "Signing in…" : "Sign in"}
                </button>
                {#if !configured}
                    <p class="q-note">Add the app credentials above first.</p>
                {/if}
            {/if}
        {/if}
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>

<style>
    .q-skeleton {
        height: 200px;
        border-radius: var(--r-md);
    }
    .q-note {
        font-size: 12.5px;
        color: var(--text-mute);
        margin: var(--space-2) 0 0;
        line-height: 1.45;
    }
    .q-card {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: var(--space-3);
    }
    .q-card-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        margin-bottom: 6px;
    }
    .q-who {
        font-size: 13.5px;
        font-weight: 600;
        color: var(--text);
    }
    .q-tag {
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    /* The same sanctioned amber the quality sheet uses for a lossless chain —
       this is the one provider that earns it. */
    .q-tag.lossless {
        color: var(--on);
    }
    .q-line {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: var(--space-3);
        font-size: 12px;
        margin: 0;
        padding: 2px 0;
    }
    .q-key {
        color: var(--text-mute);
    }
    .q-val {
        color: var(--text);
        font-size: 11.5px;
    }
    .q-done {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: var(--space-2) 0 0;
        font-size: 12px;
        color: var(--on);
    }
    .q-error {
        margin: var(--space-2) 0 0;
        font-size: 12px;
        line-height: 1.4;
        color: var(--bad);
    }
    .q-fields {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        margin: var(--space-3) 0;
    }
    /* Step 2 before step 1 is done: dimmed rather than hidden, so the shape of
       the whole setup is visible from the start. */
    .q-fields.q-locked {
        opacity: 0.55;
    }
    .q-field {
        display: flex;
        flex-direction: column;
        gap: 4px;
    }
    .q-field span {
        font-size: 11.5px;
        color: var(--text-mute);
    }
    .q-field input {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-sm);
        color: var(--text);
        padding: 9px 10px;
        font: inherit;
        font-size: 14px;
        min-height: 40px;
    }
    .q-field input:focus {
        outline: none;
        border-color: var(--on);
    }
    .q-field input[aria-invalid="true"] {
        border-color: var(--bad);
    }
    .q-disconnect {
        margin-top: var(--space-3);
    }
    /* iOS zooms any input under 16px on focus, and touch targets get the
       sanctioned minimum. */
    @media (pointer: coarse) {
        .q-field input {
            font-size: 16px;
            min-height: 44px;
        }
        .q-disconnect {
            min-height: 44px;
        }
    }
</style>

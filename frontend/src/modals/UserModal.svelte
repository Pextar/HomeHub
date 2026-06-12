<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets } from "../lib/utils";
    import { copyText } from "../lib/clipboard";
    import { untrack } from "svelte";
    import Icon from "../components/Icon.svelte";
    import type { User } from "../lib/types";

    interface Props { existing?: User | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));
    let username = $state(untrack(() => existing?.username ?? ""));
    let password = $state("");
    let admin = $state(untrack(() => existing?.admin ?? false));
    let kid = $state(untrack(() => existing?.kid ?? false));
    let selected = $state(untrack(() => new Set(existing?.socket_ids ?? [])));
    // The code an existing limited profile signs in with. Shown so the admin
    // can re-share it; "Regenerate" issues a new one on save.
    let loginCode = $state(untrack(() => existing?.login_code ?? ""));
    let regenerate = $state(false);
    let saving = $state(false);
    let showPassword = $state(false);
    let errors = $state<{ name?: string; password?: string }>({});
    const clear = (k: "name" | "password") => { if (errors[k]) errors = { ...errors, [k]: undefined }; };

    // After creating a new admin (manager) user, we show the invite link
    // before closing. This is the only time we surface it.
    let inviteURL = $state<string | null>(null);
    let inviteUsername = $state("");
    const showInvite = $derived(inviteURL !== null);

    function toggle(id: string) {
        if (selected.has(id)) selected.delete(id);
        else selected.add(id);
        selected = new Set(selected);
    }

    async function copyCode() {
        if (await copyText(loginCode)) toasts.success("Code copied", loginCode);
        else toasts.warn("Couldn't copy", "Copy it manually: " + loginCode);
    }

    async function copyInviteURL() {
        if (!inviteURL) return;
        if (await copyText(inviteURL)) toasts.success("Invite link copied");
        else toasts.warn("Couldn't copy", "Copy it manually");
    }

    async function save() {
        if (saving) return;
        const name = username.trim();
        const errs: typeof errors = {};
        if (!name) errs.name = "Give the profile a username.";
        // Editing an existing admin: password is optional (blank = keep current).
        // Owner editing their own profile: same behaviour.
        // We never require a password on creation — admins get an invite link.
        errors = errs;
        if (errs.name || errs.password) return;

        saving = true;
        try {
            if (existing) {
                await api.updateUser(existing.id, {
                    username: name,
                    admin,
                    kid: admin ? false : kid,
                    socket_ids: [...selected],
                    ...(password.trim() ? { password } : {}),
                    ...(regenerate ? { regenerate_code: true } : {}),
                });
                toasts.success("Profile updated", name);
                closeModal(true);
            } else {
                const result = await api.createUser({
                    username: name,
                    admin,
                    kid: admin ? false : kid,
                    socket_ids: [...selected],
                });
                if (admin && result.invite_url) {
                    // Stay in the modal and show the invite link so the admin
                    // can copy it before dismissing.
                    inviteURL = result.invite_url;
                    inviteUsername = name;
                } else {
                    toasts.success("Profile created", "Share their login code with them.");
                    closeModal(true);
                }
            }
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }

    function done() {
        closeModal(true);
    }
</script>

<Modal
    title={showInvite ? "Invite link ready" : (isEdit ? "Edit profile" : "New profile")}
    subtitle={showInvite
        ? `Share this link with ${inviteUsername} so they can set their password.`
        : "Profiles sign in separately. Non-admins only see the devices you assign."}
>
    {#snippet body()}
        {#if showInvite}
            <!-- Invite link view — shown after creating a new admin user -->
            <div class="invite-box">
                <div class="invite-label">One-time invite link</div>
                <div class="invite-url">{inviteURL}</div>
                <button class="btn btn-primary invite-copy" onclick={copyInviteURL}>
                    <Icon name="copy" size={15} /> Copy link
                </button>
            </div>
            <div class="field-help" style="margin-top: var(--space-3)">
                This link expires in 7 days. Once {inviteUsername} opens it and sets their password, the link stops working.
            </div>
        {:else}
            <form onsubmit={(e) => { e.preventDefault(); save(); }}>
                <div class="field">
                    <label for="usr-name">Username</label>
                    <input id="usr-name" type="text" bind:value={username}
                        placeholder="e.g. guest" autocomplete="off" autocapitalize="none" required
                        aria-invalid={errors.name ? "true" : undefined}
                        aria-describedby={errors.name ? "usr-name-err" : undefined}
                        oninput={() => clear("name")} />
                    {#if errors.name}<div id="usr-name-err" class="field-error">{errors.name}</div>{/if}
                </div>

                {#if !existing?.owner}
                    <label class="admin-row">
                        <input type="checkbox" bind:checked={admin} />
                        <div>
                            <div>Administrator</div>
                            <div class="field-help">Full access to every device and all settings, including profiles. Signs in with username + password.</div>
                        </div>
                    </label>
                {/if}

                {#if !admin}
                    <label class="admin-row kid-row">
                        <input type="checkbox" bind:checked={kid} />
                        <div>
                            <div>Kid mode</div>
                            <div class="field-help">A big, colorful, animated layout with their lamps as oversized tap tiles.</div>
                        </div>
                    </label>
                {/if}

                {#if admin}
                    {#if isEdit}
                        <!-- Editing an existing admin: allow password change -->
                        <div class="field" style="margin-top:var(--space-4)">
                            <label for="usr-pass">Password <span class="field-help-inline">(leave blank to keep current)</span></label>
                            <div class="pass-wrap">
                                <input id="usr-pass" type={showPassword ? "text" : "password"} bind:value={password}
                                    autocomplete="new-password" placeholder="••••••••"
                                    aria-invalid={errors.password ? "true" : undefined}
                                    aria-describedby={errors.password ? "usr-pass-err" : undefined}
                                    oninput={() => clear("password")} />
                                <button type="button" class="show-btn" onclick={() => showPassword = !showPassword}
                                    aria-label={showPassword ? "Hide password" : "Show password"}>
                                    <Icon name={showPassword ? "eyeOff" : "eye"} size={18} />
                                </button>
                            </div>
                            {#if errors.password}<div id="usr-pass-err" class="field-error">{errors.password}</div>{/if}
                        </div>
                    {:else}
                        <!-- Creating a new admin: invite link will be generated -->
                        <div class="invite-notice" style="margin-top:var(--space-4)">
                            <Icon name="mail" size={15} />
                            <span>An invite link will be generated — share it so they can set their own password.</span>
                        </div>
                    {/if}
                {:else if isEdit && loginCode}
                    <div class="field" style="margin-top:var(--space-4)">
                        <span class="field-label">Login code</span>
                        <div class="code-box" class:stale={regenerate}>
                            <span class="code">{loginCode}</span>
                            <div class="code-actions">
                                <button type="button" class="btn btn-ghost btn-sm" onclick={copyCode} disabled={regenerate}>Copy</button>
                                <button type="button" class="btn btn-ghost btn-sm" onclick={() => regenerate = !regenerate}>
                                    {regenerate ? "Keep current" : "Regenerate"}
                                </button>
                            </div>
                        </div>
                        <div class="field-help">
                            {regenerate
                                ? "A new code will be generated when you save. The old one stops working."
                                : "This profile signs in with this code. Share it with them."}
                        </div>
                    </div>
                {:else if !isEdit}
                    <div class="field" style="margin-top:var(--space-4)">
                        <div class="field-help">A 6-digit login code is generated when you create this profile — you'll see it in the profile list to share.</div>
                    </div>
                {/if}

                {#if !admin}
                    <div class="field" style="margin-top:var(--space-4)">
                        <span class="field-label">Allowed devices ({selected.size} of {sockets.length})</span>
                        {#if sockets.length === 0}
                            <div class="field-help">No devices exist yet. Add devices first, then assign them here.</div>
                        {:else}
                            <div class="picker">
                                {#each sockets as s (s.id)}
                                    <label class="picker-row">
                                        <input type="checkbox" checked={selected.has(s.id)}
                                            onchange={() => toggle(s.id)} />
                                        <div>
                                            <div>{s.name}</div>
                                            <div class="field-help">{s.room || "Unassigned"}</div>
                                        </div>
                                        <span class="meta" data-proto={s.protocol === "tasmota" ? "tasmota" : s.protocol.startsWith("matter") ? "matter" : "rf"}>
                                            {s.protocol === "tasmota" ? "Wi-Fi" : s.protocol === "matter-thread" ? "Thread" : s.protocol === "matter" ? "Matter" : "RF"}
                                        </span>
                                    </label>
                                {/each}
                            </div>
                            <div class="field-help">This profile sees and controls only the checked devices.</div>
                        {/if}
                    </div>
                {/if}
            </form>
        {/if}
    {/snippet}
    {#snippet actions()}
        {#if showInvite}
            <button class="btn btn-ghost" onclick={copyInviteURL}>Copy link</button>
            <button class="btn btn-primary" onclick={done}>Done</button>
        {:else}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
            <button class="btn btn-primary" onclick={save} disabled={saving}>
                {saving ? "Saving…" : isEdit ? "Save" : "Create profile"}
            </button>
        {/if}
    {/snippet}
</Modal>

<style>
    .admin-row {
        display: flex;
        align-items: flex-start;
        gap: var(--space-3);
        padding: 10px 0 0;
        cursor: pointer;
    }
    .admin-row input { width: auto; padding: 0; margin-top: 2px; }
    .kid-row { padding-top: var(--space-2); }
    .field-help-inline { color: var(--text-muted); font-weight: 400; font-size: 12px; }
    .pass-wrap { position: relative; }
    .pass-wrap input { padding-right: 40px; }
    .show-btn {
        position: absolute;
        right: 8px;
        top: 50%;
        transform: translateY(-50%);
        background: none;
        border: none;
        padding: 4px;
        cursor: pointer;
        color: var(--text-muted);
        display: grid;
        place-items: center;
        border-radius: var(--radius-sm);
    }
    .show-btn:hover { color: var(--text); }
    .invite-notice {
        display: flex;
        align-items: flex-start;
        gap: var(--space-2);
        padding: 10px 12px;
        background: var(--primary-soft);
        border: 1px solid var(--primary);
        border-radius: var(--radius-md);
        color: var(--primary);
        font-size: 13px;
    }
    .invite-notice :global(svg) { flex-shrink: 0; margin-top: 1px; }
    .invite-box {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding: var(--space-4);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
    }
    .invite-label { font-size: 12px; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
    .invite-url {
        font-family: var(--font-mono);
        font-size: 12px;
        word-break: break-all;
        color: var(--text);
        line-height: 1.5;
    }
    .invite-copy { display: flex; align-items: center; gap: var(--space-2); align-self: flex-start; }
    .code-box {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        padding: 10px 12px;
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        background: var(--surface);
    }
    .code-box.stale .code { text-decoration: line-through; color: var(--text-muted); }
    .code {
        font-size: 1.5rem;
        font-weight: 700;
        letter-spacing: 0.25em;
        font-family: var(--font-mono);
        font-variant-numeric: tabular-nums;
    }
    .code-actions { display: flex; gap: 4px; flex-shrink: 0; }
    .btn-sm { padding: 4px 10px; font-size: 13px; }
    .picker {
        display: flex;
        flex-direction: column;
        gap: 4px;
        max-height: 320px;
        overflow-y: auto;
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: 4px;
        background: var(--surface);
    }
    .picker-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 8px 10px;
        border-radius: var(--radius-sm);
        cursor: pointer;
    }
    .picker-row:hover { background: var(--surface-hover); }
    .picker-row:active { background: var(--surface); }
    .picker-row input { width: auto; padding: 0; }
    @media (pointer: coarse) {
        .picker-row { min-height: 44px; padding: 10px; }
        .show-btn { padding: 10px; }
    }
    .meta {
        color: var(--text-muted);
        font-size: 11px;
        font-weight: 600;
        margin-left: auto;
        padding: 2px 8px;
        border-radius: 999px;
        background: var(--surface-hover);
    }
    .meta[data-proto="rf"]      { color: var(--accent-rf);     background: var(--accent-rf-soft); }
    .meta[data-proto="tasmota"] { color: var(--accent-wifi);   background: var(--accent-wifi-soft); }
    .meta[data-proto="matter"]  { color: var(--accent-matter); background: var(--accent-matter-soft); }
</style>

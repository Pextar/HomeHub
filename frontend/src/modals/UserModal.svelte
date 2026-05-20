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

    function toggle(id: string) {
        if (selected.has(id)) selected.delete(id);
        else selected.add(id);
        selected = new Set(selected);
    }

    async function copyCode() {
        if (await copyText(loginCode)) toasts.success("Code copied", loginCode);
        else toasts.warn("Couldn't copy", "Copy it manually: " + loginCode);
    }

    async function save() {
        if (saving) return;
        const name = username.trim();
        if (!name) { toasts.warn("Missing name", "Give the profile a username."); return; }
        if (admin && !isEdit && !password.trim()) {
            toasts.warn("Missing password", "Admin profiles need a password.");
            return;
        }

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
            } else {
                await api.createUser({
                    username: name,
                    admin,
                    kid: admin ? false : kid,
                    socket_ids: [...selected],
                    ...(admin ? { password } : {}),
                });
                toasts.success("Profile created", admin ? name : "Share their login code with them.");
            }
            closeModal(true);
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }
</script>

<Modal
    title={isEdit ? "Edit profile" : "New profile"}
    subtitle="Profiles sign in separately. Non-admins only see the devices you assign."
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="usr-name">Username</label>
                <input id="usr-name" type="text" bind:value={username}
                    placeholder="e.g. guest" autocomplete="off" autocapitalize="none" required />
            </div>
            <label class="admin-row">
                <input type="checkbox" bind:checked={admin} />
                <div>
                    <div>Administrator</div>
                    <div class="field-help">Full access to every device and all settings, including profiles. Signs in with username + password.</div>
                </div>
            </label>

            {#if !admin}
                <label class="admin-row kid-row">
                    <input type="checkbox" bind:checked={kid} />
                    <div>
                        <div>Kid mode 🧸</div>
                        <div class="field-help">A big, colorful, animated layout with their lamps as oversized tap tiles.</div>
                    </div>
                </label>
            {/if}

            {#if admin}
                <div class="field" style="margin-top:var(--space-4)">
                    <label for="usr-pass">Password {#if isEdit}<span class="field-help-inline">(leave blank to keep current)</span>{/if}</label>
                    <div class="pass-wrap">
                        <input id="usr-pass" type={showPassword ? "text" : "password"} bind:value={password}
                            autocomplete="new-password" placeholder={isEdit ? "••••••••" : ""} />
                        <button type="button" class="show-btn" onclick={() => showPassword = !showPassword}
                            aria-label={showPassword ? "Hide password" : "Show password"}>
                            <Icon name={showPassword ? "eyeOff" : "eye"} size={18} />
                        </button>
                    </div>
                </div>
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
                                    <span class="meta" data-proto={s.protocol === "tasmota" ? "tasmota" : s.protocol === "matter" ? "matter" : "rf"}>
                                        {s.protocol === "tasmota" ? "Wi-Fi" : s.protocol === "matter" ? "Matter" : "RF"}
                                    </span>
                                </label>
                            {/each}
                        </div>
                        <div class="field-help">This profile sees and controls only the checked devices.</div>
                    {/if}
                </div>
            {/if}
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Create profile"}
        </button>
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
    .picker-row input { width: auto; padding: 0; }
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

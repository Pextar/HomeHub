<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import UserModal from "../modals/UserModal.svelte";
    import { onMount } from "svelte";
    import { api } from "../lib/api";
    import { data, toasts, session } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import { copyText } from "../lib/clipboard";
    import type { User } from "../lib/types";

    let users = $state<User[]>([]);
    let loading = $state(true);

    async function load() {
        try {
            users = (await api.listUsers()) ?? [];
        } catch (e) {
            toasts.error("Couldn't load profiles", (e as Error).message);
        } finally {
            loading = false;
        }
    }
    onMount(load);

    const sortedUsers = $derived(
        [...users].sort((a, b) => {
            // Admins first, then alphabetical.
            if (a.admin !== b.admin) return a.admin ? -1 : 1;
            return a.username.localeCompare(b.username);
        }),
    );

    function socketName(id: string): string {
        return data.value.sockets.find((s) => s.id === id)?.name ?? id;
    }
    function deviceSummary(u: User): string {
        if (u.admin) return "All devices";
        if (u.socket_ids.length === 0) return "No devices assigned";
        return u.socket_ids.map(socketName).join(", ");
    }

    async function addUser() {
        if (await openModal<boolean>(UserModal, {})) await load();
    }
    async function editUser(u: User) {
        if (await openModal<boolean>(UserModal, { existing: u })) await load();
    }

    async function copyCode(u: User) {
        if (!u.login_code) return;
        if (await copyText(u.login_code)) toasts.success("Code copied", u.login_code);
        else toasts.warn("Couldn't copy", "Copy it manually: " + u.login_code);
    }

    async function regenerate(u: User) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: `New code for ${u.username}?`,
            message: "Their current code stops working immediately. Use this if the code was lost or shared too widely.",
            confirmLabel: "Generate new code",
        });
        if (!ok) return;
        try {
            const updated = await api.updateUser(u.id, { regenerate_code: true });
            await load();
            toasts.success("New code generated", updated.login_code ?? "");
        } catch (e) {
            toasts.error("Couldn't regenerate", (e as Error).message);
        }
    }

    async function removeUser(u: User) {
        if (u.owner) return; // owner cannot be deleted — button is hidden but guard anyway
        const ok = await openModal<boolean>(ConfirmModal, {
            title: `Delete ${u.username}?`,
            message: "This profile will no longer be able to sign in. This can't be undone.",
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteUser(u.id);
            toasts.success("Profile deleted", u.username);
            await load();
        } catch (e) {
            toasts.error("Delete failed", (e as Error).message);
        }
    }
</script>

<Topbar title="Profiles" subtitle="Who can sign in, and which devices each profile controls">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={addUser}>
            <Icon name="plus" size={16} /> Add profile
        </button>
    {/snippet}
</Topbar>

{#if loading}
    <ul class="cards" aria-hidden="true">
        {#each Array.from({ length: 3 }) as _, i (i)}
            <li class="card skel-card">
                <div class="skel-top">
                    <div class="skeleton skel-avatar"></div>
                    <div class="skel-lines">
                        <div class="skeleton skel-line lg"></div>
                        <div class="skeleton skel-line sm"></div>
                    </div>
                </div>
                <div class="skeleton skel-line"></div>
            </li>
        {/each}
    </ul>
{:else if sortedUsers.length === 0}
    <EmptyState
        icon="user"
        title="No profiles yet"
        message="Add a profile to give someone their own login. Limited profiles get a 6-digit code and only see the devices you assign them."
    >
        <button class="btn btn-primary" onclick={addUser}>Add profile</button>
    </EmptyState>
{:else}
    <ul class="cards">
        {#each sortedUsers as u (u.id)}
            <li class="card">
                <div class="top">
                    <div class="ident">
                        <span class="avatar" class:admin={u.admin} class:owner={u.owner}>
                            <Icon name={u.admin ? "settings" : "user"} size={18} />
                        </span>
                        <div class="names">
                            <div class="name-row">
                                <span class="name">{u.username}</span>
                                {#if u.owner}
                                    <span class="badge owner-badge">Owner</span>
                                {:else if u.admin}
                                    <span class="badge">Admin</span>
                                {/if}
                                {#if u.pending_invite}<span class="badge invite-badge">Invite pending</span>{/if}
                                {#if u.kid}<span class="badge kid">Kid</span>{/if}
                                {#if u.id === session.user?.id}<span class="badge you">You</span>{/if}
                            </div>
                            <div class="role">
                                {#if u.owner}
                                    System owner · username + password
                                {:else if u.admin && u.pending_invite}
                                    Waiting for invite to be accepted
                                {:else if u.admin}
                                    Username + password
                                {:else if u.kid}
                                    Kid mode · signs in with a code
                                {:else}
                                    Signs in with a login code
                                {/if}
                            </div>
                        </div>
                    </div>
                    <div class="row-actions">
                        <button class="icon-btn" aria-label="Edit profile" title="Edit" onclick={() => editUser(u)}>
                            <Icon name="edit" size={16} />
                        </button>
                        {#if !u.owner}
                            <button class="icon-btn danger" aria-label="Delete profile" title="Delete" onclick={() => removeUser(u)}>
                                <Icon name="trash" size={16} />
                            </button>
                        {/if}
                    </div>
                </div>

                {#if !u.admin}
                    <div class="codeline">
                        <span class="code-label">Login code</span>
                        {#if u.login_code}
                            <button class="code" onclick={() => copyCode(u)} title="Copy code">{u.login_code}</button>
                        {:else}
                            <span class="muted">—</span>
                        {/if}
                        <button class="btn btn-ghost btn-sm" onclick={() => regenerate(u)}>New code</button>
                    </div>
                {/if}

                <div class="devices">
                    <span class="dev-label">Devices</span>
                    <span class="dev-value" class:none={!u.admin && u.socket_ids.length === 0}>{deviceSummary(u)}</span>
                </div>
            </li>
        {/each}
    </ul>
{/if}

<style>
    .muted { color: var(--text-muted); font-size: 13px; }

    /* Loading placeholders mirror the card layout to avoid a jarring swap. */
    .skel-card { gap: var(--space-4); }
    .skel-top { display: flex; align-items: center; gap: var(--space-3); }
    .skel-avatar { width: 40px; height: 40px; border-radius: 50%; flex-shrink: 0; }
    .skel-lines { flex: 1; display: flex; flex-direction: column; gap: 8px; }
    .skel-line { height: 12px; }
    .skel-line.lg { width: 55%; }
    .skel-line.sm { width: 32%; }
    .cards {
        list-style: none;
        margin: 0;
        padding: 0;
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
        gap: var(--space-4);
    }
    .card {
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }
    .top { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
    .ident { display: flex; align-items: center; gap: var(--space-3); min-width: 0; }
    .avatar {
        width: 40px;
        height: 40px;
        border-radius: 50%;
        display: grid;
        place-items: center;
        background: var(--surface-hover);
        color: var(--text-muted);
        flex-shrink: 0;
    }
    .avatar.admin { background: var(--primary-soft); color: var(--primary); }
    .avatar.owner { background: #fef3c7; color: #92400e; }
    :global([data-theme="dark"]) .avatar.owner { background: rgba(245, 158, 11, 0.15); color: #fbbf24; }
    .names { min-width: 0; }
    .name-row { display: flex; align-items: center; gap: var(--space-2); }
    .name { font-weight: 700; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .role { color: var(--text-muted); font-size: 12px; margin-top: 2px; }
    .badge {
        font-family: var(--font-mono);
        font-size: 10px;
        font-weight: 500;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--on);
        background: var(--on-soft);
        padding: 1px 6px;
        border-radius: var(--r-sm);
    }
    .badge.you { color: var(--text-muted); background: var(--surface-hover); }
    .badge.kid { color: #b15dff; background: rgba(177, 93, 255, 0.14); }
    .badge.owner-badge { color: #92400e; background: #fef3c7; }
    :global([data-theme="dark"]) .badge.owner-badge { color: #fbbf24; background: rgba(245, 158, 11, 0.15); }
    .badge.invite-badge { color: #0369a1; background: #e0f2fe; }
    :global([data-theme="dark"]) .badge.invite-badge { color: #38bdf8; background: rgba(56, 189, 248, 0.15); }
    .row-actions { display: flex; gap: 4px; flex-shrink: 0; }
    .icon-btn {
        display: grid;
        place-items: center;
        width: 32px;
        height: 32px;
        border: 1px solid var(--border);
        background: transparent;
        border-radius: var(--radius-sm);
        color: var(--text-muted);
        cursor: pointer;
        transition: background var(--t-fast), color var(--t-fast), border-color var(--t-fast);
    }
    .icon-btn:hover { background: var(--surface-hover); color: var(--text); }
    .icon-btn.danger:hover { color: var(--danger); border-color: var(--danger); }

    .codeline {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2) var(--space-3);
        background: var(--card-3);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
    }
    .code-label, .dev-label { color: var(--text-mute); font-size: 12px; flex-shrink: 0; }
    .code {
        font-family: var(--font-mono);
        font-size: 1.15rem;
        font-weight: 600;
        letter-spacing: 0.22em;
        font-variant-numeric: tabular-nums;
        color: var(--text);
        background: none;
        border: none;
        cursor: pointer;
        padding: 0;
        margin-right: auto;
    }
    .code:hover { color: var(--primary); }
    @media (pointer: coarse) {
        .code { padding: 8px 0; }
    }
    .btn-sm { padding: 4px 10px; font-size: 13px; }
    .devices { display: flex; gap: var(--space-3); align-items: baseline; }
    .dev-value { font-size: 13px; min-width: 0; }
    .dev-value.none { color: var(--text-muted); font-style: italic; }
</style>

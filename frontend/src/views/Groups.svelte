<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Switch from "../components/Switch.svelte";
    import Icon from "../components/Icon.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { runAction, automationsUsingTarget, plural } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import GroupModal from "../modals/GroupModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);

    function stats(g: typeof v.groups[number]) {
        const names = g.socket_ids.map(id => data.socketById(id)?.name).filter(Boolean) as string[];
        const onCount = g.socket_ids.filter(id => data.socketById(id)?.state).length;
        const preview = names.slice(0, 2).join(", ") + (names.length > 2 ? ` +${names.length - 2}` : "");
        return { onCount, total: g.socket_ids.length, preview };
    }

    function toggle(g: typeof v.groups[number], on: boolean) {
        runAction(() => api.groupAction(g.id, on ? "on" : "off"), `${g.name} ${on ? "on" : "off"}`);
    }

    async function confirmDelete(g: typeof v.groups[number]) {
        openId = null;
        const autoN = automationsUsingTarget(v.automations, "group", g.id);
        const extra = autoN > 0 ? ` ${plural(autoN, "automation")} using it will also be updated or removed.` : "";
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete group?",
            message: `“${g.name}” and any schedules pointing at it will be removed. The sockets themselves are not affected.${extra}`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteGroup(g.id);
            toasts.success("Group deleted", g.name);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }

    let openId = $state<string | null>(null);
    let listEl = $state<HTMLElement>();
    $effect(() => {
        if (openId === null) return;
        function onDoc(e: MouseEvent) { if (!listEl?.contains(e.target as Node)) openId = null; }
        function onKey(e: KeyboardEvent) { if (e.key === "Escape") openId = null; }
        document.addEventListener("click", onDoc, true);
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("click", onDoc, true);
            document.removeEventListener("keydown", onKey, true);
        };
    });
</script>

<Topbar title="Groups" subtitle="Control multiple devices at once">
    {#snippet actions()}
        <button class="chip" onclick={() => openModal(GroupModal, {})}><Icon name="plus" size={14} /> New group</button>
    {/snippet}
</Topbar>

{#if v.groups.length === 0}
    <EmptyState icon="groups" title="No groups yet"
        message="Group sockets together to control them in one click.">
        <button class="chip" onclick={() => openModal(GroupModal, {})}><Icon name="plus" size={14} /> New group</button>
    </EmptyState>
{:else}
    <div class="list" bind:this={listEl}>
        {#each v.groups as g, i (g.id)}
            {@const s = stats(g)}
            {@const anyOn = s.onCount > 0}
            <div class="tile" class:on={anyOn}
                animate:flip={{ duration: dur(280), easing: cubicOut }}
                in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                <div class="top">
                    <span class="ico" class:on={anyOn}><Icon name="groups" size={17} /></span>
                    <Switch checked={anyOn} onChange={(c) => toggle(g, c)} ariaLabel="Toggle {g.name}" />
                </div>
                <button class="body" onclick={() => openModal(GroupModal, { existing: g })}
                    aria-label="Edit {g.name}">
                    <span class="name">{g.name}</span>
                    <span class="meta">
                        <span class="count" class:lit={anyOn}>{s.onCount}</span><span class="slash"> / {s.total}</span> on{#if s.preview} · {s.preview}{/if}
                    </span>
                </button>
                <button class="more-corner" aria-label="Group actions"
                    onclick={(e) => { e.stopPropagation(); openId = openId === g.id ? null : g.id; }}>
                    <Icon name="more" size={16} />
                </button>
                {#if openId === g.id}
                    <div class="overflow-menu" role="menu"
                        in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
                        out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
                        <button class="overflow-item" role="menuitem"
                            onclick={() => { openId = null; openModal(GroupModal, { existing: g }); }}>
                            <Icon name="edit" size={16} /><span>Edit group</span>
                        </button>
                        <button class="overflow-item" role="menuitem"
                            onclick={() => { openId = null; runAction(() => api.groupAction(g.id, 'toggle'), `${g.name} toggled`); }}>
                            <Icon name="power" size={16} /><span>Toggle all</span>
                        </button>
                        <button class="overflow-item danger" role="menuitem" onclick={() => confirmDelete(g)}>
                            <Icon name="trash" size={16} /><span>Delete</span>
                        </button>
                    </div>
                {/if}
            </div>
        {/each}
    </div>
{/if}

<style>
    .list { display: flex; flex-direction: column; gap: 10px; }
    .tile {
        position: relative;
        border-radius: var(--r-lg);
        padding: 16px;
        background: var(--card);
        border: 1px solid var(--hairline);
        display: flex;
        flex-direction: column;
        gap: 12px;
        transition: background var(--t-med), border-color var(--t-med);
        overflow: visible; /* allow dropdown menu to escape the card bounds */
    }
    .tile.on {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    .top { display: flex; justify-content: space-between; align-items: flex-start; }
    .ico {
        width: 36px; height: 36px;
        border-radius: 10px;
        background: var(--card-3);
        color: var(--text-mute);
        display: grid; place-items: center;
        transition: background var(--t-med), color var(--t-med);
    }
    .ico.on { background: var(--on); color: var(--primary-fg); }

    .body {
        all: unset;
        cursor: pointer;
        touch-action: manipulation;
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
        /* more-corner is absolute at bottom:10px right:10px (28px wide),
           its left edge sits 22px into the content area — clear it. */
        padding-right: 28px;
    }
    .body:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-sm); }
    .name { font-weight: 600; font-size: 16px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .meta { color: var(--text-mute); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .count { font-family: var(--font-mono); color: var(--text-mute); }
    .count.lit { color: var(--on); }
    .slash { color: var(--text-dim); }

    .more-corner {
        position: absolute;
        bottom: 10px; right: 10px;
        width: 28px; height: 28px;
        display: grid; place-items: center;
        border: 0; background: transparent;
        color: var(--text-mute);
        border-radius: var(--r-sm);
        cursor: pointer;
        opacity: 0;
        transition: opacity var(--t-fast), background var(--t-fast), color var(--t-fast);
    }
    .more-corner:hover { background: var(--surface-hover); color: var(--text); }
    @media (hover: hover) { .tile:hover .more-corner { opacity: 1; } }
    @media (pointer: coarse) { .more-corner { opacity: 0.6; } }

    .overflow-menu {
        position: absolute;
        right: 12px; bottom: 44px;
        z-index: 10;
        min-width: 190px;
        display: flex; flex-direction: column;
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .overflow-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: 0;
        border-bottom: 1px solid var(--border);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left;
    }
    .overflow-item:last-child { border-bottom: none; }
    .overflow-item :global(svg) { color: var(--text-muted); flex-shrink: 0; }
    .overflow-item:hover { background: var(--surface-hover); }
    .overflow-item.danger { color: var(--danger); }
    .overflow-item.danger :global(svg) { color: var(--danger); }

    @media (pointer: coarse) {
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
    }
</style>

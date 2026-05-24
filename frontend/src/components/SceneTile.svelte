<script lang="ts">
    import Icon from "./Icon.svelte";
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import { data, toasts } from "../lib/stores.svelte";
    import SceneModal from "../modals/SceneModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";
    import type { Scene } from "../lib/types";
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";

    interface Props { scene: Scene; manage?: boolean; }
    let { scene, manage = false }: Props = $props();

    // Stable per-scene hue from a name hash, matching the mockup's palette.
    const PALETTE = ["var(--on)", "var(--cool)", "#a96bd9", "#d97a45", "#ffd066", "var(--text-mute)"];
    function nameHash(s: string): number {
        let h = 5381;
        for (let i = 0; i < s.length; i++) h = ((h << 5) + h) ^ s.charCodeAt(i);
        return Math.abs(h);
    }
    const hue = $derived(PALETTE[nameHash(scene.name) % PALETTE.length]);

    const onCount = $derived(scene.actions.filter(a => a.action === "on").length);
    const offCount = $derived(scene.actions.filter(a => a.action === "off").length);
    const sub = $derived(
        scene.actions.length === 0 ? "No devices" :
        [onCount ? `${onCount} on` : "", offCount ? `${offCount} off` : ""].filter(Boolean).join(" · ")
    );

    function activate() {
        moreOpen = false;
        runAction(() => api.activateScene(scene.id), `Scene activated: ${scene.name}`);
    }
    function openEdit() { moreOpen = false; openModal(SceneModal, { existing: scene }); }
    async function confirmDelete() {
        moreOpen = false;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete scene?",
            message: `“${scene.name}” and any schedules pointing at it will be removed.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteScene(scene.id);
            toasts.success("Scene deleted", scene.name);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }

    let moreOpen = $state(false);
    let el = $state<HTMLElement>();
    $effect(() => {
        if (!moreOpen) return;
        function onDoc(e: MouseEvent) { if (!el?.contains(e.target as Node)) moreOpen = false; }
        document.addEventListener("click", onDoc, true);
        return () => document.removeEventListener("click", onDoc, true);
    });
</script>

<div class="scene" bind:this={el}>
    <button class="scene-hit" onclick={activate} aria-label="Activate {scene.name}">
        <span class="top">
            <span class="hue-chip"><span class="hue" style="background:{hue}"></span></span>
            <span class="run">Run</span>
        </span>
        <span class="meta">
            <span class="name">{scene.name}</span>
            <span class="sub">{sub}</span>
            <span class="count mono">{scene.actions.length} {scene.actions.length === 1 ? "device" : "devices"}</span>
        </span>
    </button>

    {#if manage}
        <button class="more-corner" aria-label="Scene actions"
            onclick={(e) => { e.stopPropagation(); moreOpen = !moreOpen; }}>
            <Icon name="more" size={16} />
        </button>
        {#if moreOpen}
            <div class="overflow-menu" role="menu"
                in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
                out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
                <button class="overflow-item" role="menuitem" onclick={openEdit}>
                    <Icon name="edit" size={16} /><span>Edit scene</span>
                </button>
                <button class="overflow-item danger" role="menuitem" onclick={confirmDelete}>
                    <Icon name="trash" size={16} /><span>Delete</span>
                </button>
            </div>
        {/if}
    {/if}
</div>

<style>
    .scene {
        position: relative;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        min-height: 130px;
        display: flex;
        transition: border-color var(--t-fast), transform var(--t-fast), box-shadow var(--t-fast);
    }
    @media (hover: hover) {
        .scene:hover { border-color: var(--border-strong); transform: translateY(-2px); box-shadow: var(--shadow-md); }
    }
    .scene-hit {
        all: unset;
        box-sizing: border-box;
        flex: 1;
        min-width: 0;
        cursor: pointer;
        touch-action: manipulation;
        padding: 14px;
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        gap: 10px;
    }
    .scene-hit:active { transform: scale(0.98); }
    .scene-hit:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-md); }

    .top { display: flex; justify-content: space-between; align-items: flex-start; }
    .hue-chip {
        width: 32px; height: 32px;
        border-radius: 10px;
        background: var(--card-3);
        display: grid; place-items: center;
        flex-shrink: 0;
    }
    .hue { width: 14px; height: 14px; border-radius: 50%; display: block; }
    .run { color: var(--text-mute); font-size: 11px; }

    .meta { display: flex; flex-direction: column; min-width: 0; }
    .name { font-weight: 600; font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .sub { color: var(--text-mute); font-size: 12px; margin-top: 3px; line-height: 1.3; }
    .count { color: var(--text-dim); font-size: 11.5px; margin-top: 6px; }

    .more-corner {
        position: absolute;
        top: 10px; right: 10px;
        width: 26px; height: 26px;
        display: grid; place-items: center;
        border: 0; background: transparent;
        color: var(--text-mute);
        border-radius: var(--r-sm);
        cursor: pointer;
    }
    .more-corner:hover { background: var(--surface-hover); color: var(--text); }

    .overflow-menu {
        position: absolute;
        right: 10px; top: 40px;
        z-index: 10;
        min-width: 170px;
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
        .more-corner { width: 32px; height: 32px; }
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
    }
</style>

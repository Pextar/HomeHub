<script lang="ts">
    import Icon from "./Icon.svelte";
    import { api } from "../lib/api";
    import { runAction, automationsUsingTarget, plural, formatAgo } from "../lib/utils";
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
    // Accent presets map to design tokens; fall back to the name-hash hue.
    const ACCENTS: Record<string, string> = {
        amber: "var(--on)", cool: "var(--cool)", violet: "var(--p-matter)",
        orange: "var(--p-rf)", green: "var(--good)", gold: "var(--p-mqtt)",
    };
    const hue = $derived(
        scene.color && ACCENTS[scene.color] ? ACCENTS[scene.color] : PALETTE[nameHash(scene.name) % PALETTE.length],
    );

    // Activation telemetry, surfaced so a scene shows it actually runs.
    const ranCount = $derived(scene.activate_count ?? 0);
    const ranAgo = $derived(formatAgo(scene.last_activated_at));

    // Flatten all actions across all steps for summary display.
    const allActions = $derived((scene.steps ?? []).flatMap(s => s.actions));

    const onCount  = $derived(allActions.filter(a => a.action === "on").length);
    const offCount = $derived(allActions.filter(a => a.action === "off").length);
    const stepCount = $derived((scene.steps ?? []).length);

    // Brightness/colour hint: levels set on any on-action, and distinct colours.
    const dimLevels = $derived(
        allActions.filter(a => a.action === "on" && a.level != null).map(a => a.level as number),
    );
    const brightHint = $derived(
        dimLevels.length === 0 ? "" :
        dimLevels.length === 1 ? `${dimLevels[0]}%` :
        `${Math.min(...dimLevels)}–${Math.max(...dimLevels)}%`,
    );
    const sceneColors = $derived([...new Set(allActions.filter(a => a.color).map(a => a.color as string))]);
    const ruleCount = $derived(
        data.value.automations.filter(a => a.scene_id === scene.id).length
    );

    // Solar schedules that fire this scene at sunrise/sunset.
    const solarSchedules = $derived(
        data.value.schedules.filter(s =>
            s.target_type === "scene" &&
            s.target_id === scene.id &&
            (s.time_mode === "sunrise" || s.time_mode === "sunset") &&
            s.enabled
        )
    );

    const sub = $derived(
        allActions.length === 0
            ? (ruleCount > 0 ? `${ruleCount} rule${ruleCount === 1 ? "" : "s"}` : "No devices")
            : [onCount ? `${onCount} on` : "", offCount ? `${offCount} off` : ""].filter(Boolean).join(" · ")
    );

    function activate() {
        moreOpen = false;
        runAction(() => api.activateScene(scene.id), `Scene activated: ${scene.name}`);
    }
    function openEdit() { moreOpen = false; openModal(SceneModal, { existing: scene }); }
    async function confirmDelete() {
        moreOpen = false;
        const autoN = automationsUsingTarget(data.value.automations, "scene", scene.id);
        const extra = autoN > 0 ? ` ${plural(autoN, "automation")} using it will also be updated or removed.` : "";
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete scene?",
            message: `"${scene.name}" and any schedules pointing at it will be removed.${extra}`,
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
            <span class="hue-chip" style:color={hue}>
                {#if scene.icon}
                    <Icon name={scene.icon as never} size={16} />
                {:else}
                    <span class="hue" style="background:{hue}"></span>
                {/if}
            </span>
            <span class="run">Run</span>
        </span>
        <span class="meta">
            <span class="name">{scene.name}</span>
            <span class="sub">{sub}</span>
            {#if brightHint || sceneColors.length}
                <span class="tags">
                    {#if brightHint}
                        <span class="dim-badge"><Icon name="sun" size={11} />{brightHint}</span>
                    {/if}
                    {#each sceneColors as c (c)}
                        <span class="cdot" style="background:#{c}"></span>
                    {/each}
                </span>
            {/if}
            <span class="count mono">
                {#if allActions.length > 0}
                    {allActions.length} {allActions.length === 1 ? "device" : "devices"}
                    {#if stepCount > 1}<span class="step-hint">· {stepCount} steps</span>{/if}
                {:else}
                    {ruleCount} {ruleCount === 1 ? "rule" : "rules"}
                {/if}
            </span>
            {#if ranCount > 0}
                <span class="ran mono" title="Manual activations">Ran {ranCount}×{ranAgo ? ` · ${ranAgo}` : ""}</span>
            {/if}
            {#if solarSchedules.length > 0}
                <span class="solar-times">
                    {#each solarSchedules as s (s.id)}
                        <span class="solar-badge">
                            <Icon name={s.time_mode === "sunrise" ? "sunrise" : "sunset"} size={11} />
                            <span class="mono">{s.effective_time ? `≈ ${s.effective_time}` : (s.time_mode === "sunrise" ? "Sunrise" : "Sunset")}</span>
                        </span>
                    {/each}
                </span>
            {/if}
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
    .name {
        font-weight: 600; font-size: 15px;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        -webkit-box-orient: vertical;
        overflow: hidden;
        line-height: 1.25;
    }
    .sub { color: var(--text-mute); font-size: 12px; margin-top: 3px; line-height: 1.3; }
    .tags { display: flex; align-items: center; gap: 6px; margin-top: 6px; flex-wrap: wrap; }
    .dim-badge {
        display: inline-flex; align-items: center; gap: 3px;
        font-size: 10.5px; color: var(--on);
        background: var(--on-soft);
        padding: 1px 6px; border-radius: var(--r-pill);
    }
    .dim-badge :global(svg) { color: var(--on); }
    .cdot {
        width: 12px; height: 12px; border-radius: 50%;
        border: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    .count { color: var(--text-dim); font-size: 11.5px; margin-top: 6px; }
    .step-hint { color: var(--on); }
    .ran { color: var(--text-dim); font-size: 11px; margin-top: 3px; }

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

    .solar-times { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; align-items: center; }
    .solar-badge {
        display: inline-flex; align-items: center; gap: 3px;
        font-size: 11px; color: var(--text-dim);
    }
    .solar-badge :global(svg) { color: var(--text-mute); flex-shrink: 0; }

    @media (pointer: coarse) {
        .more-corner { width: 32px; height: 32px; }
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
    }
</style>

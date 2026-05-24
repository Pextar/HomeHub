<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { sortedSockets } from "../lib/utils";
    import { untrack } from "svelte";
    import type { Scene } from "../lib/types";

    interface Props { existing?: Scene | null; }
    let { existing = null }: Props = $props();
    const isEdit = $derived(!!existing);

    const sockets = $derived(sortedSockets(data.value.sockets));

    const isSmart = (protocol: string) =>
        protocol === "tasmota" || protocol === "matter" || protocol === "matter-thread";

    // Colour presets mirror the light-detail mockup; "" means leave colour as-is.
    const COLOURS: { hex: string; name: string }[] = [
        { hex: "", name: "Auto" },
        { hex: "f5bd6e", name: "Warm" },
        { hex: "ffe9c4", name: "Soft" },
        { hex: "ffffff", name: "Bright" },
        { hex: "c4a4e0", name: "Lilac" },
        { hex: "7aa4d9", name: "Cool" },
    ];

    const initial = untrack(() => {
        const m = new Map<string, "ignore" | "on" | "off">();
        if (existing) for (const a of existing.actions) m.set(a.socket_id, a.action);
        return m;
    });
    const initialLevels = untrack(() => {
        const m = new Map<string, number>();
        if (existing) for (const a of existing.actions) if (a.level != null) m.set(a.socket_id, a.level);
        return m;
    });
    const initialColors = untrack(() => {
        const m = new Map<string, string>();
        if (existing) for (const a of existing.actions) if (a.color) m.set(a.socket_id, a.color);
        return m;
    });

    let name = $state(untrack(() => existing?.name ?? ""));
    let perSocket = $state<Record<string, "ignore" | "on" | "off">>(
        untrack(() => Object.fromEntries(sockets.map(s => [s.id, initial.get(s.id) ?? "ignore"])))
    );
    let levels = $state<Record<string, number>>(
        untrack(() => Object.fromEntries(sockets.map(s => [s.id, initialLevels.get(s.id) ?? 100])))
    );
    let colors = $state<Record<string, string>>(
        untrack(() => Object.fromEntries(sockets.map(s => [s.id, initialColors.get(s.id) ?? ""])))
    );
    let saving = $state(false);
    let nameError = $state("");
    let actionsError = $state("");

    async function save() {
        if (saving) return;
        const actions = Object.entries(perSocket)
            .filter(([, v]) => v !== "ignore")
            .map(([socket_id, action]) => {
                const a: { socket_id: string; action: "on" | "off"; level?: number; color?: string } =
                    { socket_id, action: action as "on" | "off" };
                const sock = sockets.find(s => s.id === socket_id);
                if (action === "on" && sock && isSmart(sock.protocol)) {
                    a.level = levels[socket_id];
                    if (colors[socket_id]) a.color = colors[socket_id];
                }
                return a;
            });
        const payload = { name: name.trim(), actions };
        nameError = payload.name ? "" : "Give the scene a name.";
        actionsError = actions.length === 0 ? "Set at least one device to On or Off." : "";
        if (nameError || actionsError) return;
        saving = true;
        try {
            if (existing) {
                await api.updateScene(existing.id, payload);
                toasts.success("Scene updated", payload.name);
            } else {
                await api.createScene(payload);
                toasts.success("Scene created", payload.name);
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }
</script>

<Modal
    title={isEdit ? "Edit scene" : "New scene"}
    subtitle="A scene drives selected sockets to specific states in one tap."
    size="wide"
>
    {#snippet body()}
        <form onsubmit={(e) => { e.preventDefault(); save(); }}>
            <div class="field">
                <label for="scn-name">Name</label>
                <input id="scn-name" type="text" bind:value={name}
                    placeholder="e.g. Movie night" autocomplete="off" required
                    aria-invalid={nameError ? "true" : undefined}
                    aria-describedby={nameError ? "scn-name-err" : undefined}
                    oninput={() => nameError = ""} />
                {#if nameError}<div id="scn-name-err" class="field-error">{nameError}</div>{/if}
            </div>
            <div class="field" style="margin-top:var(--space-4)">
                <span class="field-label">Per-socket actions</span>
                <div class="picker">
                    {#each sockets as s (s.id)}
                        <div class="picker-row">
                            <div class="info">
                                <div>{s.name}</div>
                                <div class="field-help">{s.room || "Unassigned"}</div>
                            </div>
                            <select bind:value={perSocket[s.id]} aria-label="Action for {s.name}"
                                onchange={() => actionsError = ""}>
                                <option value="ignore">Ignore</option>
                                <option value="on">Turn on</option>
                                <option value="off">Turn off</option>
                            </select>
                        </div>
                        {#if perSocket[s.id] === "on" && isSmart(s.protocol)}
                            <div class="light-row">
                                <div class="bright">
                                    <span class="bright-ico"><Icon name="sun" size={14} /></span>
                                    <input type="range" min="1" max="100" step="1"
                                        bind:value={levels[s.id]} aria-label="Brightness for {s.name}" />
                                    <span class="bright-val mono">{levels[s.id]}%</span>
                                </div>
                                <div class="swatches">
                                    {#each COLOURS as c (c.name)}
                                        <button type="button" class="swatch" class:active={colors[s.id] === c.hex}
                                            class:auto={c.hex === ""}
                                            style={c.hex ? `background:#${c.hex}` : ""}
                                            title={c.name} aria-label="{c.name} for {s.name}"
                                            onclick={() => colors[s.id] = c.hex}>
                                            {#if c.hex === ""}<Icon name="close" size={12} />{/if}
                                        </button>
                                    {/each}
                                </div>
                            </div>
                        {/if}
                    {/each}
                </div>
                {#if actionsError}<div class="field-error">{actionsError}</div>{/if}
                <div class="field-help">Ignored sockets are not touched when the scene runs.</div>
            </div>
        </form>
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save} disabled={saving}>
            {saving ? "Saving…" : isEdit ? "Save" : "Create scene"}
        </button>
    {/snippet}
</Modal>

<style>
    .picker {
        display: flex;
        flex-direction: column;
        gap: 4px;
        max-height: 360px;
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
    }
    .info { flex: 1; min-width: 0; }
    .picker-row select {
        width: auto;
        padding: 4px 10px;
        font-size: 13px;
    }

    /* Smart-light brightness + colour, shown under a light set to "on". */
    .light-row {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding: 2px 10px 10px 10px;
        margin: -2px 0 4px;
    }
    .bright { display: flex; align-items: center; gap: 8px; }
    .bright-ico { color: var(--on); display: inline-flex; flex-shrink: 0; }
    .bright input[type="range"] { flex: 1; }
    .bright-val { font-size: 12px; color: var(--text-mute); min-width: 38px; text-align: right; }
    .swatches { display: flex; gap: 8px; }
    .swatch {
        width: 24px; height: 24px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        cursor: pointer;
        display: grid; place-items: center;
        padding: 0;
        color: var(--text-mute);
    }
    .swatch.auto { background: var(--card-3); }
    .swatch.active { box-shadow: 0 0 0 2px var(--on), 0 0 0 4px var(--bg-elevated); }
    @media (pointer: coarse) {
        .swatch { width: 30px; height: 30px; }
        .bright input[type="range"] { height: 28px; }
    }
    @media (pointer: coarse) {
        .picker-row { min-height: 44px; padding: 10px; }
        /* 16px stops iOS zoom-on-focus; min-height meets the touch target. */
        .picker-row select { font-size: 16px; padding: 8px 12px; min-height: 44px; }
    }
</style>

<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import { untrack } from "svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import ShortcutsModal from "../modals/ShortcutsModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";

    const v = $derived(data.value);

    let latitude     = $state(untrack(() => data.value.settings.latitude));
    let longitude    = $state(untrack(() => data.value.settings.longitude));
    let locationName = $state(untrack(() => data.value.settings.location_name ?? ""));
    let saving       = $state(false);

    let lastApplied = $state(untrack(() => ({
        lat:  data.value.settings.latitude,
        lon:  data.value.settings.longitude,
        name: data.value.settings.location_name ?? "",
    })));
    $effect(() => {
        const next = { lat: v.settings.latitude, lon: v.settings.longitude, name: v.settings.location_name ?? "" };
        if (next.lat !== lastApplied.lat || next.lon !== lastApplied.lon || next.name !== lastApplied.name) {
            latitude     = next.lat;
            longitude    = next.lon;
            locationName = next.name;
            lastApplied  = next;
        }
    });

    const dirty = $derived(
        latitude     !== v.settings.latitude  ||
        longitude    !== v.settings.longitude ||
        (locationName || "") !== (v.settings.location_name ?? "")
    );

    async function save() {
        if (saving) return;
        saving = true;
        try {
            await api.updateSettings({
                latitude:      Number(latitude),
                longitude:     Number(longitude),
                location_name: locationName.trim() || undefined,
            });
            toasts.success("Settings saved");
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            saving = false;
        }
    }

    let importing = $state(false);
    let fileInput = $state<HTMLInputElement>();

    // Export streams a download straight from the API (cookie-authenticated,
    // same-origin) — a plain anchor click is all that's needed.
    function exportConfig() {
        const a = document.createElement("a");
        a.href = "/api/export";
        a.download = "";
        document.body.appendChild(a);
        a.click();
        a.remove();
    }

    async function onImportFile(e: Event) {
        const input = e.currentTarget as HTMLInputElement;
        const file = input.files?.[0];
        input.value = ""; // allow re-importing the same file
        if (!file) return;

        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Restore from backup?",
            message: "This replaces your devices, schedules, groups, scenes and sensors with the contents of the file. Profiles are not affected.",
            confirmLabel: "Restore",
            danger: true,
        });
        if (!ok) return;

        importing = true;
        try {
            const bundle = JSON.parse(await file.text());
            const r = await api.importConfig(bundle);
            toasts.success("Backup restored", `${r.sockets} devices, ${r.schedules} schedules, ${r.scenes} scenes.`);
            await data.refresh();
        } catch (e) {
            toasts.error("Import failed", (e as Error).message);
        } finally {
            importing = false;
        }
    }

    function useBrowserLocation() {
        if (!navigator.geolocation) {
            toasts.warn("Not available", "Your browser doesn't expose a location.");
            return;
        }
        navigator.geolocation.getCurrentPosition(
            (pos) => {
                latitude  = Math.round(pos.coords.latitude  * 10000) / 10000;
                longitude = Math.round(pos.coords.longitude * 10000) / 10000;
                toasts.info("Location filled", "Click Save to apply.");
            },
            (err) => toasts.error("Location denied", err.message),
            { enableHighAccuracy: false, timeout: 8000 },
        );
    }
</script>

<Topbar title="Settings" subtitle="Controller configuration" />

<section class="card">
    <header>
        <h2>Location</h2>
        <p>Used to compute sunrise and sunset for solar-based schedules.</p>
    </header>

    <form onsubmit={(e) => { e.preventDefault(); save(); }}>
        <div class="field">
            <label for="loc-name">Label <span class="optional">(optional)</span></label>
            <input id="loc-name" type="text" bind:value={locationName} placeholder="Home" maxlength="60" />
        </div>
        <div class="field-row">
            <div class="field">
                <label for="lat">Latitude</label>
                <input id="lat" type="number" step="0.0001" min="-90" max="90"
                       bind:value={latitude} required />
                <div class="field-help">Decimal degrees. North is positive.</div>
            </div>
            <div class="field">
                <label for="lon">Longitude</label>
                <input id="lon" type="number" step="0.0001" min="-180" max="180"
                       bind:value={longitude} required />
                <div class="field-help">Decimal degrees. East is positive.</div>
            </div>
        </div>
        <div class="actions">
            <button type="button" class="btn btn-ghost" onclick={useBrowserLocation}>
                Use this device's location
            </button>
            <button type="submit" class="btn btn-primary" disabled={!dirty || saving}>
                {saving ? "Saving…" : "Save"}
            </button>
        </div>
    </form>
</section>

<section class="card">
    <header>
        <h2>Integrations</h2>
        <p>Control your devices from outside the app.</p>
    </header>
    <div class="actions" style="justify-content:flex-start">
        <button type="button" class="btn btn-ghost" onclick={() => openModal(ShortcutsModal, {})}>
            iOS Shortcuts
        </button>
    </div>
</section>

<section class="card">
    <header>
        <h2>Backup &amp; restore</h2>
        <p>Export your full configuration to a file, or restore it from one. Profiles and passwords are never included.</p>
    </header>
    <div class="actions" style="justify-content:flex-start">
        <button type="button" class="btn btn-ghost" onclick={exportConfig}>
            Export backup
        </button>
        <button type="button" class="btn btn-ghost" onclick={() => fileInput?.click()} disabled={importing}>
            {importing ? "Restoring…" : "Restore backup"}
        </button>
        <input
            bind:this={fileInput}
            type="file"
            accept="application/json,.json"
            onchange={onImportFile}
            hidden
        />
    </div>
</section>

<style>
    .card {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-6);
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
        max-width: 640px;
    }
    header h2 { margin: 0 0 4px; font-size: 1.05rem; }
    header p  { margin: 0; color: var(--text-muted); font-size: 13px; }
    form { display: flex; flex-direction: column; gap: var(--space-4); }
    .actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
        flex-wrap: wrap;
    }
    .optional { color: var(--text-muted); font-weight: 400; font-size: 12px; }
</style>

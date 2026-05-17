<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import { untrack } from "svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";

    const v = $derived(data.value);

    // --- Location ---
    let latitude = $state(untrack(() => data.value.settings.latitude));
    let longitude = $state(untrack(() => data.value.settings.longitude));
    let locationName = $state(untrack(() => data.value.settings.location_name ?? ""));
    let savingLocation = $state(false);

    let lastApplied = $state(untrack(() => ({
        lat: data.value.settings.latitude,
        lon: data.value.settings.longitude,
        name: data.value.settings.location_name ?? "",
    })));
    $effect(() => {
        const next = { lat: v.settings.latitude, lon: v.settings.longitude, name: v.settings.location_name ?? "" };
        if (next.lat !== lastApplied.lat || next.lon !== lastApplied.lon || next.name !== lastApplied.name) {
            latitude = next.lat;
            longitude = next.lon;
            locationName = next.name;
            lastApplied = next;
        }
    });

    const locationDirty = $derived(
        latitude !== v.settings.latitude ||
        longitude !== v.settings.longitude ||
        (locationName || "") !== (v.settings.location_name ?? "")
    );

    async function saveLocation() {
        if (savingLocation) return;
        savingLocation = true;
        try {
            await api.updateSettings({
                latitude: Number(latitude),
                longitude: Number(longitude),
                location_name: locationName.trim() || undefined,
                hue_bridge_ip: v.settings.hue_bridge_ip,
                hue_username: v.settings.hue_username,
            });
            toasts.success("Settings saved");
            await data.refresh();
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            savingLocation = false;
        }
    }

    function useBrowserLocation() {
        if (!navigator.geolocation) {
            toasts.warn("Not available", "Your browser doesn't expose a location.");
            return;
        }
        navigator.geolocation.getCurrentPosition(
            (pos) => {
                latitude = Math.round(pos.coords.latitude * 10000) / 10000;
                longitude = Math.round(pos.coords.longitude * 10000) / 10000;
                toasts.info("Location filled", "Click Save to apply.");
            },
            (err) => toasts.error("Location denied", err.message),
            { enableHighAccuracy: false, timeout: 8000 },
        );
    }

    // --- Hue ---
    let hueBridgeIP = $state(untrack(() => data.value.settings.hue_bridge_ip ?? ""));
    let pairing = $state(false);

    const hueConnected = $derived(!!(v.settings.hue_bridge_ip && v.settings.hue_username));

    $effect(() => {
        const ip = v.settings.hue_bridge_ip ?? "";
        if (ip !== hueBridgeIP && !pairing) hueBridgeIP = ip;
    });

    async function connectHue() {
        if (pairing) return;
        const ip = hueBridgeIP.trim();
        if (!ip) {
            toasts.warn("Bridge IP required", "Enter the IP address of your Hue bridge.");
            return;
        }
        pairing = true;
        try {
            await api.huePair(ip);
            toasts.success("Hue bridge connected", "You can now add Hue lamps as sockets.");
            await data.refresh();
        } catch (e) {
            toasts.error("Pairing failed", (e as Error).message);
        } finally {
            pairing = false;
        }
    }

    async function disconnectHue() {
        try {
            await api.updateSettings({
                latitude: v.settings.latitude,
                longitude: v.settings.longitude,
                location_name: v.settings.location_name,
                hue_bridge_ip: "",
                hue_username: "",
            });
            toasts.success("Hue bridge disconnected");
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        }
    }
</script>

<Topbar title="Settings" subtitle="Controller configuration" />

<section class="card">
    <header>
        <h2>Location</h2>
        <p>Used to compute sunrise and sunset for solar-based schedules.</p>
    </header>

    <form onsubmit={(e) => { e.preventDefault(); saveLocation(); }}>
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
            <button type="submit" class="btn btn-primary" disabled={!locationDirty || savingLocation}>
                {savingLocation ? "Saving…" : "Save"}
            </button>
        </div>
    </form>
</section>

<section class="card">
    <header>
        <h2>Philips Hue</h2>
        <p>Connect your Hue bridge to control Wi-Fi lamps alongside RF sockets.</p>
    </header>

    {#if hueConnected}
        <div class="hue-status connected">
            <span class="dot"></span>
            <div>
                <strong>Connected</strong>
                <div class="sub">{v.settings.hue_bridge_ip}</div>
            </div>
            <button class="btn btn-ghost btn-sm" onclick={disconnectHue}>Disconnect</button>
        </div>
        <p class="field-help" style="margin-top:0">
            To add a Hue lamp, create a new socket and choose <em>Philips Hue (Wi-Fi)</em> as the protocol.
        </p>
    {:else}
        <div class="field">
            <label for="hue-ip">Bridge IP address</label>
            <input id="hue-ip" type="text" bind:value={hueBridgeIP}
                placeholder="e.g. 192.168.1.100" autocomplete="off" />
            <div class="field-help">
                Find it in the Hue app under Settings → Hue Bridges → (i).
            </div>
        </div>
        <div class="actions">
            <button class="btn btn-primary" onclick={connectHue} disabled={pairing || !hueBridgeIP.trim()}>
                {pairing ? "Connecting…" : "Connect"}
            </button>
        </div>
        <p class="field-help">
            Press the link button on your bridge, then click Connect within 30 seconds.
        </p>
    {/if}
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
    header p { margin: 0; color: var(--text-muted); font-size: 13px; }
    form { display: flex; flex-direction: column; gap: var(--space-4); }
    .actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
        flex-wrap: wrap;
    }
    .optional { color: var(--text-muted); font-weight: 400; font-size: 12px; }

    .hue-status {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-3) var(--space-4);
        border-radius: var(--radius-sm);
        border: 1px solid var(--border);
        background: var(--bg-base);
    }
    .hue-status .btn { margin-left: auto; }
    .dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        flex-shrink: 0;
        background: var(--color-success, #22c55e);
    }
    .sub { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
    .btn-sm { padding: 4px 10px; font-size: 12px; }
    .field-help { font-size: 12px; color: var(--text-muted); }
</style>

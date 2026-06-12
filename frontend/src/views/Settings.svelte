<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import { untrack } from "svelte";
    import { api } from "../lib/api";
    import { data, toasts, session, theme, route } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import Icon from "../components/Icon.svelte";
    import ShortcutsModal from "../modals/ShortcutsModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { pushClient, pushSupported } from "../lib/push.svelte";

    const v = $derived(data.value);

    const username = $derived(session.user?.username ?? "You");
    const initial = $derived(username.charAt(0).toUpperCase());
    const roleLabel = $derived(session.user?.admin ? "Admin · signed in" : "Limited · signed in");

    async function signOut() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Sign out?",
            message: "You'll need to sign in again to get back in.",
            confirmLabel: "Sign out",
        });
        if (!ok) return;
        try { await api.logout(); } catch { /* ignore */ }
        window.location.reload();
    }

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

    // ── Push notifications ─────────────────────────────────────────────────
    // Snapshot saved prefs once to seed the editable form. session.user is
    // already loaded by the time Settings renders (LoginGate gates on it).
    const np = session.user?.notif_prefs;
    let notifPrefs = $state({
        sensor_alerts:    np?.sensor_alerts    ?? true,
        state_changes:    np?.state_changes    ?? true,
        schedule_fired:   np?.schedule_fired   ?? true,
        device_offline:   np?.device_offline   ?? true,
        quiet_hours:      np?.quiet_hours      ?? false,
        quiet_start:      np?.quiet_start      ?? "22:00",
        quiet_end:        np?.quiet_end        ?? "07:00",
        muted_socket_ids: [...(np?.muted_socket_ids ?? [])],
        muted_sensor_ids: [...(np?.muted_sensor_ids ?? [])],
    });
    let notifSaving = $state(false);
    let testing = $state(false);
    let showMuted = $state(false);

    $effect(() => {
        pushClient.init();
    });

    // A device is "notifying" when it is NOT in the muted list.
    function socketNotifying(id: string) { return !notifPrefs.muted_socket_ids.includes(id); }
    function sensorNotifying(id: string) { return !notifPrefs.muted_sensor_ids.includes(id); }
    function toggleSocketMute(id: string) {
        notifPrefs.muted_socket_ids = socketNotifying(id)
            ? [...notifPrefs.muted_socket_ids, id]
            : notifPrefs.muted_socket_ids.filter((x) => x !== id);
    }
    function toggleSensorMute(id: string) {
        notifPrefs.muted_sensor_ids = sensorNotifying(id)
            ? [...notifPrefs.muted_sensor_ids, id]
            : notifPrefs.muted_sensor_ids.filter((x) => x !== id);
    }

    async function toggleNotifications() {
        if (!pushSupported || pushClient.loading) return;
        if (pushClient.isSubscribed) {
            await pushClient.unsubscribe();
            toasts.info("Notifications disabled");
        } else {
            const ok = await pushClient.subscribe();
            if (ok) {
                toasts.success("Notifications enabled", "You'll receive alerts even when the app is closed.");
            } else if (pushClient.permission === "denied") {
                toasts.warn("Permission denied", "Enable notifications in your browser settings.");
            }
        }
    }

    async function saveNotifPrefs() {
        if (notifSaving) return;
        notifSaving = true;
        try {
            await api.updatePushPrefs(notifPrefs);
            toasts.success("Notification preferences saved");
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            notifSaving = false;
        }
    }

    async function sendTest() {
        if (testing) return;
        testing = true;
        try {
            await api.testPush();
            toasts.info("Test sent", "A notification should appear shortly.");
        } catch (e) {
            toasts.error("Test failed", (e as Error).message);
        } finally {
            testing = false;
        }
    }

    let locating = $state(false);
    function useBrowserLocation() {
        if (!navigator.geolocation) {
            toasts.warn("Not available", "Your browser doesn't expose a location.");
            return;
        }
        locating = true;
        navigator.geolocation.getCurrentPosition(
            (pos) => {
                latitude  = Math.round(pos.coords.latitude  * 10000) / 10000;
                longitude = Math.round(pos.coords.longitude * 10000) / 10000;
                locating = false;
                toasts.info("Location filled", "Click Save to apply.");
            },
            (err) => { locating = false; toasts.error("Location denied", err.message); },
            { enableHighAccuracy: false, timeout: 8000 },
        );
    }
</script>

<Topbar title="Settings" subtitle="Controller configuration" />

<!-- Profile card -->
<div class="profile-card">
    <span class="avatar mono">{initial}</span>
    <div class="who">
        <div class="who-name">{username}</div>
        <div class="who-role">{roleLabel}</div>
    </div>
    <div class="who-actions">
        <button class="chip" onclick={() => theme.toggle()} aria-label="Toggle theme">
            <Icon name={theme.current === "dark" ? "moon" : "sun"} size={15} />
            {theme.current === "dark" ? "Dark" : "Light"}
        </button>
        <button class="chip danger" onclick={signOut}>
            <Icon name="logout" size={15} /> Sign out
        </button>
    </div>
</div>

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
            <button type="button" class="btn btn-ghost" onclick={useBrowserLocation} disabled={locating}>
                {locating ? "Locating…" : "Use this device's location"}
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
        <h2>Push notifications</h2>
        <p>
            {#if !pushSupported}
                Your browser doesn't support push notifications.
            {:else if pushClient.permission === "denied"}
                Notifications are blocked. Enable them in your browser settings and reload.
            {:else}
                Receive alerts on this device even when the app is closed.
            {/if}
        </p>
    </header>

    {#if pushSupported && pushClient.permission !== "denied"}
        <div class="notif-row">
            <span class="notif-label">
                {pushClient.isSubscribed ? "Notifications enabled" : "Notifications disabled"}
            </span>
            <button
                type="button"
                class="btn {pushClient.isSubscribed ? 'btn-ghost' : 'btn-primary'}"
                onclick={toggleNotifications}
                disabled={pushClient.loading}
            >
                {#if pushClient.loading}
                    {pushClient.isSubscribed ? "Disabling…" : "Enabling…"}
                {:else}
                    {pushClient.isSubscribed ? "Disable" : "Enable"}
                {/if}
            </button>
        </div>

        {#if pushClient.isSubscribed}
            <div class="notif-row">
                <span class="notif-label">Send a test notification</span>
                <button type="button" class="btn btn-ghost" onclick={sendTest} disabled={testing}>
                    {testing ? "Sending…" : "Send test"}
                </button>
            </div>

            <form class="notif-prefs" onsubmit={(e) => { e.preventDefault(); saveNotifPrefs(); }}>
                <p class="prefs-label">Notify me when:</p>
                <label class="check-row">
                    <input type="checkbox" bind:checked={notifPrefs.sensor_alerts} />
                    <span>A sensor crosses its alert threshold</span>
                </label>
                <label class="check-row">
                    <input type="checkbox" bind:checked={notifPrefs.state_changes} />
                    <span>A device is turned on or off</span>
                </label>
                <label class="check-row">
                    <input type="checkbox" bind:checked={notifPrefs.schedule_fired} />
                    <span>A schedule or timer fires</span>
                </label>
                <label class="check-row">
                    <input type="checkbox" bind:checked={notifPrefs.device_offline} />
                    <span>A Wi-Fi or Matter device goes offline</span>
                </label>

                <!-- Quiet hours -->
                <div class="prefs-group">
                    <label class="check-row">
                        <input type="checkbox" bind:checked={notifPrefs.quiet_hours} />
                        <span>Quiet hours <span class="optional">(mutes everything except sensor alerts)</span></span>
                    </label>
                    {#if notifPrefs.quiet_hours}
                        <div class="quiet-times">
                            <label>
                                From
                                <input type="time" bind:value={notifPrefs.quiet_start} />
                            </label>
                            <label>
                                to
                                <input type="time" bind:value={notifPrefs.quiet_end} />
                            </label>
                        </div>
                    {/if}
                </div>

                <!-- Per-device muting -->
                {#if v.sockets.length > 0 || v.sensors.length > 0}
                    <div class="prefs-group">
                        <button type="button" class="link-btn" onclick={() => (showMuted = !showMuted)}>
                            {showMuted ? "▾" : "▸"} Per-device settings
                        </button>
                        {#if showMuted}
                            <p class="prefs-label">Uncheck a device to silence its notifications.</p>
                            {#each v.sockets as sock (sock.id)}
                                <label class="check-row">
                                    <input
                                        type="checkbox"
                                        checked={socketNotifying(sock.id)}
                                        onchange={() => toggleSocketMute(sock.id)}
                                    />
                                    <span>{sock.name}</span>
                                </label>
                            {/each}
                            {#each v.sensors as sensor (sensor.id)}
                                <label class="check-row">
                                    <input
                                        type="checkbox"
                                        checked={sensorNotifying(sensor.id)}
                                        onchange={() => toggleSensorMute(sensor.id)}
                                    />
                                    <span>{sensor.name} <span class="optional">(sensor)</span></span>
                                </label>
                            {/each}
                        {/if}
                    </div>
                {/if}

                <div class="actions">
                    <button type="submit" class="btn btn-primary" disabled={notifSaving}>
                        {notifSaving ? "Saving…" : "Save preferences"}
                    </button>
                </div>
            </form>
        {/if}
    {/if}
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

<section class="card">
    <header>
        <h2>System</h2>
        <p>Power-user tools for inspecting and driving the hub directly.</p>
    </header>
    <button type="button" class="system-row" onclick={() => route.go("console")}>
        <span class="system-icon"><Icon name="monitor" size={18} /></span>
        <span class="system-text">
            <span class="system-title">Console</span>
            <span class="system-sub">Live device table, event tail and a command line.</span>
        </span>
        <Icon name="chevronDown" size={16} />
    </button>
</section>

<style>
    .system-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        width: 100%;
        padding: var(--space-3) var(--space-2);
        background: transparent;
        border: none;
        border-radius: var(--r-md);
        cursor: pointer;
        text-align: left;
        color: var(--text);
        transition: background var(--t-fast);
    }
    .system-row:hover { background: var(--surface-hover); }
    .system-row :global(svg:last-child) { color: var(--text-mute); transform: rotate(-90deg); flex-shrink: 0; }
    .system-icon {
        width: 36px; height: 36px;
        display: grid; place-items: center;
        border-radius: var(--r-sm);
        background: var(--surface);
        color: var(--on);
        flex-shrink: 0;
    }
    .system-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .system-title { font-weight: 600; font-size: 14px; }
    .system-sub { color: var(--text-mute); font-size: 12.5px; }

    .profile-card {
        display: flex;
        align-items: center;
        gap: 14px;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: 16px;
        max-width: 640px;
    }
    .avatar {
        width: 50px; height: 50px;
        border-radius: 50%;
        background: var(--on);
        color: var(--primary-fg);
        display: grid; place-items: center;
        font-weight: 600; font-size: 18px;
        flex-shrink: 0;
    }
    .who { flex: 1; min-width: 0; }
    .who-name { font-weight: 600; font-size: 16px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .who-role { color: var(--text-mute); font-size: 12.5px; }
    .who-actions { display: flex; gap: var(--space-2); flex-shrink: 0; flex-wrap: wrap; justify-content: flex-end; }
    .chip.danger { color: var(--bad); }

    .card {
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: var(--space-5);
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
        max-width: 640px;
    }
    header h2 { margin: 0 0 4px; font-size: 17px; font-weight: 600; letter-spacing: -0.01em; }
    header p  { margin: 0; color: var(--text-mute); font-size: 13px; }
    form { display: flex; flex-direction: column; gap: var(--space-4); }
    .actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
        flex-wrap: wrap;
    }
    .optional { color: var(--text-mute); font-weight: 400; font-size: 12px; }

    /* Coordinates are numeric — render them with tabular mono figures. */
    form input[type="number"] {
        font-family: var(--font-mono);
        font-variant-numeric: tabular-nums;
    }

    /* Push notification section */
    .notif-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .notif-label { font-size: 14px; }
    .notif-prefs {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding-top: var(--space-2);
        border-top: 1px solid var(--border);
    }
    .prefs-label {
        margin: 0;
        font-size: 13px;
        color: var(--text-muted);
    }
    .check-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        font-size: 14px;
        cursor: pointer;
        user-select: none;
    }
    .check-row input[type="checkbox"] {
        width: 16px;
        height: 16px;
        accent-color: var(--primary);
        flex-shrink: 0;
    }
    .prefs-group {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding-top: var(--space-3);
        border-top: 1px solid var(--border);
    }
    .quiet-times {
        display: flex;
        gap: var(--space-4);
        flex-wrap: wrap;
        padding-left: var(--space-5);
    }
    .quiet-times label {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        font-size: 14px;
        color: var(--text-muted);
    }
    .quiet-times input[type="time"] {
        padding: 4px 8px;
        border-radius: var(--r-sm);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        font-family: var(--font-mono);
        font-variant-numeric: tabular-nums;
    }
    .link-btn {
        background: none;
        border: none;
        color: var(--primary);
        font-size: 13px;
        cursor: pointer;
        padding: 0;
        text-align: left;
        width: fit-content;
    }
</style>

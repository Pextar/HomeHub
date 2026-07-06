<!--
  Guided Matter onboarding wizard.

  Three steps:
    1. Pairing code     — user scans the QR or types the manual code.
    2. Commissioning    — bridge talks BLE → Wi-Fi (30–60s); show progress.
    3. Name & room      — auto-fill from the device's vendor/product, let
                          the user adjust, then save as a Socket.

  The wizard owns the whole flow so the regular SocketModal stays clean.
-->
<script lang="ts">
    import { onDestroy } from "svelte";
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import QRScanner from "../components/QRScanner.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";

    // Closing the modal mid-commission must stop the progress animation and
    // the up-to-3-minute job polling loop — the bridge job itself keeps
    // running server-side and the device shows up in the list when done.
    let destroyed = false;
    onDestroy(() => {
        destroyed = true;
        stopProgress();
    });

    type Step = "input" | "commissioning" | "details";
    type InputMode = "scan" | "manual";
    type Transport = "thread" | "wifi";

    let step        = $state<Step>("input");
    let inputMode   = $state<InputMode>("scan");
    let pairingCode = $state("");
    let scannerError = $state<string | null>(null);
    let codeError = $state("");
    let nameError = $state("");

    // Available transports fetched from the bridge on mount.
    // Both "thread" and "wifi" can be configured simultaneously.
    let availableTransports = $state<Transport[]>([]);
    let transport = $state<Transport | null>(null); // null = not yet chosen

    $effect(() => {
        api.matterTransport()
            .then(r => {
                availableTransports = r.transports as Transport[];
                // Auto-select when only one is configured.
                if (r.transports.length === 1) transport = r.transports[0] as Transport;
            })
            .catch(() => { /* non-fatal — wizard still works, transport stays null */ });
    });

    // Commissioning step state
    let progress    = $state(0);              // 0..1, animated
    let progressTimer: ReturnType<typeof setInterval> | null = null;
    let commissionError = $state<string | null>(null);

    // Details step state
    let nodeId      = $state("");
    let suggestedName = $state("");
    let name        = $state("");
    let room        = $state("");
    let vendor      = $state("");
    let product     = $state("");
    let readOnly    = $state(false); // true for sensors (motion, contact, temperature…)
    let saving      = $state(false);

    function onScanned(text: string) {
        pairingCode = text.trim();
        startCommission();
    }
    function onScanError(message: string) {
        // Show the error in the scan tab — don't auto-switch; let the user
        // decide to switch to manual if they prefer.
        scannerError = message;
    }

    // Format 11-digit Matter manual codes as DDDD-DDD-DDDD while typing.
    // MT:… QR payloads are left as-is.
    function onCodeInput(e: Event) {
        codeError = "";
        const raw = (e.target as HTMLInputElement).value;
        if (raw.toUpperCase().startsWith("MT:")) {
            pairingCode = raw;
            return;
        }
        const digits = raw.replace(/[^0-9]/g, "").slice(0, 11);
        if (digits.length > 7) {
            pairingCode = digits.slice(0, 4) + "-" + digits.slice(4, 7) + "-" + digits.slice(7);
        } else if (digits.length > 4) {
            pairingCode = digits.slice(0, 4) + "-" + digits.slice(4);
        } else {
            pairingCode = digits;
        }
    }

    function looksLikePairingCode(code: string): boolean {
        const trimmed = code.trim();
        if (trimmed.toUpperCase().startsWith("MT:")) return true;
        const digits = trimmed.replace(/[^0-9]/g, "");
        // Matter manual codes are 11 digits; the longer form (with vendor/
        // product appendix) is 21 digits.
        return digits.length === 11 || digits.length === 21;
    }

    async function startCommission() {
        const code = pairingCode.trim();
        if (!code || !looksLikePairingCode(code)) {
            const msg = !code
                ? "Type the 11- or 21-digit code printed on the device."
                : "Expecting an 11- or 21-digit code, or an MT:… QR payload.";
            // Manual mode has a visible input to attach the error to; a scanned
            // payload doesn't, so fall back to a toast there.
            if (inputMode === "manual") codeError = msg;
            else toasts.warn("Pairing code problem", msg);
            return;
        }
        codeError = "";
        step = "commissioning";
        commissionError = null;
        // Slow climb up to ~90% over the expected commissioning window so
        // the user has something to look at. We jump to 100% on success.
        progress = 0;
        progressTimer = setInterval(() => {
            if (progress < 0.9) progress = Math.min(0.9, progress + 0.015);
        }, 800);

        let jobId: string;
        try {
            const r = await api.matterCommission({
                pairing_code: code,
                ...(transport ? { transport } : {}),
            });
            jobId = r.job_id;
        } catch (e) {
            stopProgress();
            if (destroyed) return;
            commissionError = (e as Error).message;
            return;
        }
        if (destroyed) return;

        // Poll the job status until the bridge finishes (or errors). The
        // POST above returns immediately so the long-running commission
        // outlives the original fetch — Safari's "Load failed" timeout
        // can't kill us anymore.
        try {
            const job = await pollJob(jobId);
            if (destroyed) return;
            if (job.status === "error") {
                stopProgress();
                commissionError = job.error || "Commissioning failed";
                return;
            }
            nodeId = job.node_id || "";
            progress = 1;
            // Pull the device's name / vendor so we can pre-fill the next step.
            try {
                const state = await api.matterGetState(nodeId);
                vendor = state.vendor || "";
                product = state.product || "";
                suggestedName = state.name || state.product || "";
                name = suggestedName;
            } catch {
                /* non-fatal — user can name it themselves */
            }
            stopProgress();
            step = "details";
        } catch (e) {
            stopProgress();
            if (destroyed) return;
            commissionError = (e as Error).message;
        }
    }

    async function pollJob(jobId: string) {
        // Hard cap of ~3 minutes (~90 polls × 2s) — matches the backend's
        // own commission ceiling. Safe to be a bit longer than the server
        // since the polling fetches themselves are short.
        for (let i = 0; i < 90; i++) {
            await sleep(2000);
            if (destroyed) throw new Error("cancelled");
            const j = await api.matterCommissionJob(jobId);
            if (j.status !== "pending") return j;
        }
        throw new Error("Commissioning timed out after 3 minutes");
    }

    function sleep(ms: number) {
        return new Promise<void>((r) => setTimeout(r, ms));
    }

    function stopProgress() {
        if (progressTimer) { clearInterval(progressTimer); progressTimer = null; }
    }

    function tryAgain() {
        commissionError = null;
        step = "input";
    }

    // BLE-phase errors happen before any network credentials are sent to the
    // device. The device has NOT joined Thread/Wi-Fi yet, so factory-reset is
    // always safe and the Thread-dataset / border-router hint is irrelevant.
    function isBlePhaseError(msg: string | null): boolean {
        if (!msg) return false;
        // All these happen before any Matter protocol is exchanged — the device
        // has not received network credentials, so a retry or factory-reset is safe.
        return /connecting to peripheral|unexpected state.*error|could not find.*device|ble.*scan|scan.*timeout/i.test(msg);
    }

    async function save() {
        const payload = {
            name: name.trim(),
            room: room.trim(),
            code: nodeId,
            // Save as "matter-thread" or "matter" so the UI labels the device correctly.
            protocol: transport === "thread" ? "matter-thread" : "matter",
            ...(readOnly ? { readonly: true } : {}),
        };
        if (!payload.name) {
            nameError = "Give the device a name so you can find it later.";
            return;
        }
        nameError = "";
        saving = true;
        try {
            await api.createSocket(payload);
            toasts.success("Device added", payload.name);
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
    title="Set up Matter device"
    subtitle={step === "input"        ? "Step 1 of 3 · Pairing code"
            : step === "commissioning" ? "Step 2 of 3 · Onboarding"
            :                            "Step 3 of 3 · Name & room"}
>
    {#snippet body()}
        {#if step === "input"}
            {#if availableTransports.length > 1}
                <!-- Both Thread and Wi-Fi are configured — let the user pick. -->
                <div class="field transport-pick">
                    <span class="field-label">Network type</span>
                    <div class="tabs" role="radiogroup">
                        {#each availableTransports as t (t)}
                            <button class="tab" class:active={transport === t}
                                role="radio" aria-checked={transport === t}
                                onclick={() => transport = t}>
                                <Icon name="devices" size={16} />
                                {t === "thread" ? "Matter (Thread)" : "Matter (Wi-Fi)"}
                            </button>
                        {/each}
                    </div>
                    <div class="field-help">
                        Choose Thread for low-power mesh devices (via your Thread Border Router),
                        or Wi-Fi for bulbs and plugs that connect directly.
                    </div>
                </div>
            {/if}
            <div class="tabs" role="tablist">
                <button class="tab" class:active={inputMode === "scan"}
                    role="tab" aria-selected={inputMode === "scan"}
                    onclick={() => { inputMode = "scan"; scannerError = null; }}>
                    <Icon name="qrcode" size={16} /> Scan QR
                </button>
                <button class="tab" class:active={inputMode === "manual"}
                    role="tab" aria-selected={inputMode === "manual"}
                    onclick={() => { inputMode = "manual"; }}>
                    <Icon name="keyboard" size={16} /> Enter manually
                </button>
            </div>

            {#if inputMode === "scan"}
                <!-- QRScanner handles its own error display now; we still show
                     an inline hint if camera failed so user knows to switch. -->
                {#if availableTransports.length > 1 && !transport}
                    <div class="field-help transport-warning">
                        Select a network type above before scanning.
                    </div>
                {:else}
                    <QRScanner onDecoded={onScanned} onError={onScanError} />
                    {#if scannerError}
                        <div class="camera-fallback-hint">
                            Camera didn't work? <button class="link-btn"
                                onclick={() => { inputMode = "manual"; }}>Enter code manually</button>
                        </div>
                    {:else}
                        <div class="field-help">
                            Scan the QR code on the device or its box — it starts with <code>MT:</code>.
                        </div>
                    {/if}
                {/if}
            {:else}
                <div class="field">
                    <label for="mat-pair">Pairing code</label>
                    <input id="mat-pair" type="text" inputmode="numeric"
                        value={pairingCode}
                        oninput={onCodeInput}
                        placeholder="3496-112-0001"
                        autocomplete="off"
                        autocorrect="off"
                        autocapitalize="off"
                        spellcheck={false}
                        aria-invalid={codeError ? "true" : undefined}
                        aria-describedby={codeError ? "mat-pair-err" : undefined} />
                    {#if codeError}<div id="mat-pair-err" class="field-error">{codeError}</div>{/if}
                    <div class="field-help">
                        Type the 11-digit code printed on the device (dashes are added
                        automatically). Or paste the full <code>MT:…</code> QR payload.
                    </div>
                </div>
            {/if}
        {:else if step === "commissioning"}
            {#if commissionError}
                <div class="note error">
                    <strong>Commissioning failed</strong>
                    <span>{commissionError}</span>
                    <span class="hint">
                        {#if isBlePhaseError(commissionError)}
                            Bluetooth found the device but couldn't connect before the
                            commissioning window closed — no network credentials were
                            sent yet. <strong>Re-open the commissioning window</strong>
                            (short button press, 1–5 s, see your device's manual), then
                            hit Try again. A factory-reset is also safe at this point
                            if needed.
                        {:else if transport === "thread"}
                            The device may have already joined your Thread mesh but
                            didn't complete the fabric handshake. Its commissioning
                            window has likely closed — <strong>open it again</strong>
                            with a short button press (1–5 s, check your device's
                            manual), then hit Try again. The device will stay on your
                            network. Only factory-reset (hold ~10 s) as a last resort
                            if short-pressing doesn't help. Other causes:
                            MATTER_BRIDGE_THREAD_DATASET not set or Thread Border
                            Router not reachable, or SRP-to-mDNS bridging not active on your border router.
                        {:else}
                            The device may have already joined your Wi-Fi but didn't
                            complete the fabric handshake. Its commissioning window
                            has likely closed — <strong>open it again</strong> with a
                            short button press (1–5 s, check your device's manual),
                            then hit Try again. The device will stay on your Wi-Fi.
                            Only factory-reset (hold ~10 s) as a last resort if
                            short-pressing doesn't help. Other causes: Wi-Fi
                            credentials not configured on the bridge, or Bluetooth
                            not available.
                        {/if}
                    </span>
                </div>
            {:else}
                <div class="commissioning">
                    <div class="title">Pairing with your device…</div>
                    <div class="hint">
                        This usually takes 30–60 seconds. The bridge talks to the
                        device over Bluetooth, hands it your
                        {#if transport === "thread"}Thread network credentials{:else}Wi-Fi credentials{/if},
                        and confirms it joined the network.
                    </div>
                    <div class="progress">
                        <div class="bar" style:width="{Math.round(progress * 100)}%"></div>
                    </div>
                </div>
            {/if}
        {:else if step === "details"}
            <div class="note success">
                <strong>Device commissioned</strong>
                <span>
                    Assigned node id <code>{nodeId}</code>{#if vendor || product}
                        · {[vendor, product].filter(Boolean).join(" ")}
                    {/if}
                </span>
            </div>
            <div class="field">
                <label for="mat-name">Name</label>
                <input id="mat-name" type="text" bind:value={name}
                    placeholder={suggestedName || "e.g. Living room lamp"}
                    autocomplete="off" required
                    aria-invalid={nameError ? "true" : undefined}
                    aria-describedby={nameError ? "mat-name-err" : undefined}
                    oninput={() => nameError = ""} />
                {#if nameError}<div id="mat-name-err" class="field-error">{nameError}</div>{/if}
            </div>
            <div class="field">
                <label for="mat-room">Room <span class="opt">(optional)</span></label>
                <input id="mat-room" type="text" bind:value={room}
                    placeholder="e.g. Living room" autocomplete="off"
                    list="mat-room-list" />
                <datalist id="mat-room-list">
                    {#each data.value.rooms as r (r.name)}
                        <option value={r.name}></option>
                    {/each}
                </datalist>
            </div>
            <label class="field-checkbox">
                <input type="checkbox" bind:checked={readOnly} />
                <span>Sensor (read-only) — motion, contact, temperature, etc.</span>
            </label>
        {/if}
    {/snippet}
    {#snippet actions()}
        {#if step === "input"}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
            {#if inputMode === "manual"}
                <button class="btn btn-primary" onclick={startCommission}
                    disabled={availableTransports.length > 1 && !transport}>
                    Commission
                </button>
            {/if}
        {:else if step === "commissioning"}
            {#if commissionError}
                <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
                <button class="btn btn-primary" onclick={tryAgain}>Try again</button>
            {:else}
                <button class="btn btn-ghost" disabled>Working…</button>
            {/if}
        {:else if step === "details"}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Skip</button>
            <button class="btn btn-primary" onclick={save} disabled={saving}>
                {saving ? "Saving…" : "Add device"}
            </button>
        {/if}
    {/snippet}
</Modal>

<style>
    .transport-pick { margin-bottom: var(--space-4); }
    .transport-warning {
        text-align: center;
        padding: var(--space-6) 0;
        color: var(--text-muted);
    }

    .tabs {
        display: flex;
        gap: 2px;
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: 4px;
    }
    .tab {
        flex: 1;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        padding: 8px 12px;
        border: 0;
        background: transparent;
        color: var(--text-muted);
        font-size: 13px;
        font-weight: 500;
        cursor: pointer;
        border-radius: calc(var(--radius-md) - 2px);
        transition: background 0.12s, color 0.12s;
    }
    .tab:hover { color: var(--text); }
    .tab.active {
        background: var(--bg-elevated);
        color: var(--text);
        box-shadow: var(--shadow-sm);
    }

    .note {
        display: flex; flex-direction: column; gap: 4px;
        padding: var(--space-3) var(--space-4);
        border-radius: var(--radius-md);
        border: 1px solid var(--border);
        background: var(--surface);
        font-size: 13px;
    }
    .note.error  { border-color: var(--danger); color: var(--danger); }
    .note.success { border-color: var(--success); }
    .note .hint { color: var(--text-muted); font-size: 12px; }
    .note code { font-family: var(--font-mono); font-size: 12px; }

    .commissioning {
        display: flex; flex-direction: column; align-items: center;
        gap: var(--space-3);
        padding: var(--space-6) var(--space-4);
        text-align: center;
    }
    .commissioning .title { font-weight: 600; font-size: 15px; }
    .commissioning .hint  { color: var(--text-muted); font-size: 13px; max-width: 360px; }
    .progress {
        width: 100%; max-width: 320px;
        height: 6px;
        border-radius: 999px;
        background: var(--surface);
        overflow: hidden;
        border: 1px solid var(--border);
    }
    .bar {
        height: 100%;
        background: var(--primary);
        transition: width 0.6s ease;
    }


    .field-help {
        font-size: 12px;
        color: var(--text-muted);
        margin-top: 4px;
    }
    .field-help code { font-family: var(--font-mono); }
    .opt { color: var(--text-muted); font-weight: 400; font-size: 12px; }

    .field-checkbox {
        display: flex;
        align-items: center;
        gap: 10px;
        font-size: 14px;
        cursor: pointer;
        padding: 2px 0;
    }
    .field-checkbox input[type="checkbox"] { width: 16px; height: 16px; flex-shrink: 0; cursor: pointer; }

    .camera-fallback-hint {
        font-size: 13px;
        color: var(--text-muted);
        text-align: center;
        margin-top: 4px;
    }
    .link-btn {
        background: none;
        border: none;
        padding: 0;
        color: var(--primary);
        font-size: inherit;
        cursor: pointer;
        text-decoration: underline;
        text-underline-offset: 2px;
    }

    @media (pointer: coarse) {
        .tab { padding: 12px; font-size: 15px; }
        .link-btn { padding: 8px 4px; }
    }
</style>

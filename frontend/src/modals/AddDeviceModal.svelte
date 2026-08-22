<!--
  The one way into the house.

  Adding a device used to be four buttons across three screens, and it made
  the user answer two questions before they could start: is this thing a
  Socket or a Sensor (which decided *which screen* to be on), and what
  protocol does it speak (an eight-entry dropdown, asked fifth in a form
  whose first four answers Matter then threw away).

  Neither question is the user's to answer. The first is a fact about the
  device, which the bridge reports; the second is a fact about the box in
  their hand, which they know by looking at it. So this sheet asks one
  question — how do you reach it — then discovers the device, then asks for
  a name. Three steps for everything, and what gets created (a Socket, some
  Sensors, or both) is decided by lib/device-setup.ts from what was found.

  It is one sheet with internal steps, never a sheet that opens a sheet
  (DESIGN.md §2).
-->
<script lang="ts">
    import { onDestroy, type ComponentProps } from "svelte";
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import QRScanner from "../components/QRScanner.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import {
        planFromMatterState,
        sensorName,
        socketProtocol,
        defaultUnit,
        defaultField,
        type ConnectionMethod,
        type DevicePlan,
    } from "../lib/device-setup";
    import type { SensorKind, TasmotaDevice, DiscoveryCandidate } from "../lib/types";

    type Step = "method" | "discover" | "details";
    type Transport = "thread" | "wifi";

    let step = $state<Step>("method");
    let method = $state<ConnectionMethod | null>(null);

    // Closing mid-commission must stop the progress animation and the job
    // polling loop. The bridge job keeps running server-side either way.
    let destroyed = false;
    onDestroy(() => {
        destroyed = true;
        stopProgress();
        if (pollTimer) clearInterval(pollTimer);
    });

    // ── What the house can actually offer ────────────────────────────────
    // A method whose backend isn't configured is absent rather than offered
    // and refused (DESIGN.md §15.1). Matter needs the bridge; everything
    // else is always available.
    let matterTransports = $state<Transport[]>([]);
    let matterReady = $state(false);
    let probedMatter = $state(false);
    $effect(() => {
        api.matterTransport()
            .then(r => { matterTransports = r.transports as Transport[]; matterReady = true; })
            .catch(() => { matterReady = false; })
            .finally(() => { probedMatter = true; });
    });

    type IconName = ComponentProps<typeof Icon>["name"];
    const METHODS: { id: ConnectionMethod; icon: IconName; title: string; blurb: string }[] = [
        { id: "matter",    icon: "qrcode", title: "Matter device",
          blurb: "Scan the QR code or type the pairing code printed on it." },
        { id: "tasmota",   icon: "wifi",   title: "Wi-Fi device (Tasmota)",
          blurb: "We'll search your network for it — no IP address needed." },
        { id: "rf-socket", icon: "socket", title: "433 MHz socket",
          blurb: "A remote-controlled plug you pair by pressing its button." },
        { id: "rf-sensor", icon: "sensor", title: "433 MHz sensor",
          blurb: "A thermometer or motion sensor that reports on its own." },
        { id: "mqtt",      icon: "radio",  title: "MQTT device",
          blurb: "Anything on your broker, reached by topic." },
    ];
    const offered = $derived(METHODS.filter(m => m.id !== "matter" || matterReady));

    // ── Step 2 state, per method ─────────────────────────────────────────
    // Matter
    let transport = $state<Transport | null>(null);
    let inputMode = $state<"scan" | "manual">("scan");
    let pairingCode = $state("");
    let scannerError = $state<string | null>(null);
    let codeError = $state("");
    let commissioning = $state(false);
    let commissionError = $state<string | null>(null);
    let progress = $state(0);
    let progressTimer: ReturnType<typeof setInterval> | null = null;
    let nodeId = $state("");
    let plan = $state<DevicePlan | null>(null);

    // Tasmota
    let sweeping = $state(false);
    let sweepDone = $state(false);
    let found = $state<TasmotaDevice[]>([]);
    let sweepError = $state<string | null>(null);

    // RF socket (we transmit a code) / RF sensor (we listen for packets)
    let rfCode = $state("");
    let pairing = $state(false);
    let rfResult = $state<{ ok: boolean; text: string } | null>(null);
    let candidates = $state<DiscoveryCandidate[]>([]);
    let listenUntil = $state(0);
    let now = $state(Date.now());
    let pollTimer: ReturnType<typeof setInterval> | null = null;
    const listenRemaining = $derived(Math.max(0, Math.ceil((listenUntil - now) / 1000)));
    const listening = $derived(listenRemaining > 0);
    const knownCodes = $derived(new Set(data.value.sensors.map(s => s.code)));
    const newCandidates = $derived(candidates.filter(c => !knownCodes.has(c.code)));

    // MQTT
    let topic = $state("");
    let mqttRole = $state<"socket" | "sensor">("socket");
    let mqttResult = $state<{ ok: boolean; text: string } | null>(null);
    let publishing = $state(false);

    // ── Step 3 state ─────────────────────────────────────────────────────
    let name = $state("");
    let suggestedName = $state("");
    let room = $state("");
    let code = $state("");            // whatever identifies the device
    let kind = $state<SensorKind>("temperature");
    let unit = $state("°C");
    let field = $state("temperature_C");
    let saving = $state(false);
    let nameError = $state("");
    let foundSummary = $state("");    // one line describing what we found

    // Changing kind resets unit/field to that kind's defaults, as the
    // standalone sensor editor does. The user can still override.
    let lastKind = $state<SensorKind>("temperature");
    $effect(() => {
        if (kind === lastKind) return;
        lastKind = kind;
        unit = defaultUnit(kind);
        field = defaultField(kind);
    });

    function pick(m: ConnectionMethod) {
        method = m;
        step = "discover";
        if (m === "matter" && matterTransports.length === 1) transport = matterTransports[0];
        if (m === "tasmota") void sweep();
        if (m === "rf-sensor") void startListening();
    }

    function back() {
        if (step === "details") { step = "discover"; return; }
        step = "method";
        method = null;
        resetDiscovery();
    }

    function resetDiscovery() {
        commissionError = null; codeError = ""; pairingCode = "";
        found = []; sweepDone = false; sweepError = null;
        rfCode = ""; rfResult = null; candidates = []; listenUntil = 0;
        topic = ""; mqttResult = null;
        if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
        stopProgress();
    }

    // ── Matter ───────────────────────────────────────────────────────────
    function onScanned(text: string) { pairingCode = text.trim(); void commission(); }

    function onCodeInput(e: Event) {
        codeError = "";
        const raw = (e.target as HTMLInputElement).value;
        if (raw.toUpperCase().startsWith("MT:")) { pairingCode = raw; return; }
        const digits = raw.replace(/[^0-9]/g, "").slice(0, 11);
        if (digits.length > 7) pairingCode = `${digits.slice(0, 4)}-${digits.slice(4, 7)}-${digits.slice(7)}`;
        else if (digits.length > 4) pairingCode = `${digits.slice(0, 4)}-${digits.slice(4)}`;
        else pairingCode = digits;
    }

    function looksLikePairingCode(c: string): boolean {
        const t = c.trim();
        if (t.toUpperCase().startsWith("MT:")) return true;
        const digits = t.replace(/[^0-9]/g, "");
        return digits.length === 11 || digits.length === 21;
    }

    function stopProgress() {
        if (progressTimer) { clearInterval(progressTimer); progressTimer = null; }
    }

    async function commission() {
        const c = pairingCode.trim();
        if (!c || !looksLikePairingCode(c)) {
            const msg = !c
                ? "Type the 11- or 21-digit code printed on the device."
                : "Expecting an 11- or 21-digit code, or an MT:… QR payload.";
            if (inputMode === "manual") codeError = msg;
            else toasts.warn("Pairing code problem", msg);
            return;
        }
        codeError = "";
        commissioning = true;
        commissionError = null;
        progress = 0;
        progressTimer = setInterval(() => {
            if (progress < 0.9) progress = Math.min(0.9, progress + 0.015);
        }, 800);

        try {
            const started = await api.matterCommission({
                pairing_code: c,
                ...(transport ? { transport } : {}),
            });
            const job = await pollJob(started.job_id);
            if (destroyed) return;
            if (job.status === "error") {
                commissionError = job.error || "Commissioning failed";
                return;
            }
            nodeId = job.node_id || "";
            progress = 1;

            // Read the node back to learn what it actually is. This is what
            // decides whether it becomes a Socket, some Sensors, or both —
            // the user is never asked.
            try {
                const state = await api.matterGetState(nodeId);
                plan = planFromMatterState(state);
                suggestedName = state.name || state.product || "";
                name = suggestedName;
                foundSummary = describePlan(plan, [state.vendor, state.product].filter(Boolean).join(" "));
            } catch {
                // Non-fatal: the node is commissioned, we just couldn't read
                // it back. Treat it as a plain switchable device.
                plan = { socket: true, sensors: [] };
                foundSummary = "Commissioned, but its capabilities couldn't be read.";
            }
            code = nodeId;
            step = "details";
        } catch (e) {
            if (!destroyed) commissionError = (e as Error).message;
        } finally {
            stopProgress();
            commissioning = false;
        }
    }

    async function pollJob(jobId: string) {
        for (let i = 0; i < 90; i++) {
            await sleep(2000);
            if (destroyed) throw new Error("cancelled");
            const j = await api.matterCommissionJob(jobId);
            if (j.status !== "pending") return j;
        }
        throw new Error("Commissioning timed out after 3 minutes");
    }

    const sleep = (ms: number) => new Promise<void>(r => setTimeout(r, ms));

    function describePlan(p: DevicePlan, vendor: string): string {
        const parts: string[] = [];
        if (p.socket) parts.push("an on/off switch");
        for (const s of p.sensors) parts.push(`a ${s.kind} sensor`);
        const what = parts.length ? parts.join(" and ") : "no readable capabilities";
        return vendor ? `${vendor} — ${what}.` : `Found ${what}.`;
    }

    // BLE-phase errors happen before any network credentials are sent, so a
    // factory reset is always safe and the Thread hints are irrelevant.
    function isBlePhaseError(msg: string | null): boolean {
        if (!msg) return false;
        return /connecting to peripheral|unexpected state.*error|could not find.*device|ble.*scan|scan.*timeout/i.test(msg);
    }

    // ── Tasmota ──────────────────────────────────────────────────────────
    async function sweep() {
        if (sweeping) return;
        sweeping = true;
        sweepDone = false;
        sweepError = null;
        try {
            const r = await api.tasmotaDiscover();
            if (destroyed) return;
            found = r.devices;
        } catch (e) {
            if (!destroyed) sweepError = (e as Error).message;
        } finally {
            if (!destroyed) { sweeping = false; sweepDone = true; }
        }
    }

    function adoptTasmota(dev: TasmotaDevice) {
        code = dev.ip;
        suggestedName = dev.name || "";
        name = dev.name || "";
        plan = { socket: true, sensors: [] };
        foundSummary = `Tasmota at ${dev.ip}${dev.topic ? ` · ${dev.topic}` : ""}.`;
        step = "details";
    }

    // ── 433 MHz socket: we broadcast a code for it to learn ──────────────
    async function pairRf() {
        if (pairing) return;
        pairing = true;
        rfResult = null;
        try {
            const isRetry = !!rfCode;
            const r = await api.learnSocket({ protocol: "nexa", code: rfCode || undefined });
            rfCode = r.code;
            rfResult = {
                ok: true,
                text: isRetry
                    ? "Signal sent (×2). Sent the same code again — did your socket click on this time?"
                    : "Signal sent (×2). Did your socket click on? If not, long-press its button again and tap Pair — the same code will be resent.",
            };
        } catch (e) {
            rfResult = { ok: false, text: `Pairing failed: ${(e as Error).message}` };
        } finally {
            pairing = false;
        }
    }

    function acceptRfSocket() {
        code = rfCode;
        plan = { socket: true, sensors: [] };
        foundSummary = `433 MHz socket on code ${rfCode}.`;
        step = "details";
    }

    // ── 433 MHz sensor: we listen for packets it sends ───────────────────
    async function startListening() {
        try {
            const res = await api.startSensorPair(60);
            listenUntil = new Date(res.until).getTime();
            candidates = [];
            if (!pollTimer) {
                pollTimer = setInterval(() => {
                    now = Date.now();
                    void pollCandidates();
                }, 1200);
            }
            void pollCandidates();
        } catch (e) {
            toasts.error("Couldn't start listening", (e as Error).message);
        }
    }

    async function pollCandidates() {
        try {
            const res = await api.discoverSensors();
            candidates = res.candidates;
            listenUntil = new Date(res.until).getTime();
        } catch {
            /* transient — the window keeps running */
        }
    }

    function adoptCandidate(c: DiscoveryCandidate) {
        code = c.code;
        const guessed = guessKind(c);
        kind = guessed;
        lastKind = guessed;
        unit = defaultUnit(guessed);
        field = pickField(c, guessed);
        plan = { socket: false, sensors: [{ kind: guessed, unit, field, suffix: guessed }] };
        foundSummary = `433 MHz sensor ${c.code} · heard ${c.count}×.`;
        if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
        step = "details";
    }

    function guessKind(c: DiscoveryCandidate): SensorKind {
        const keys = Object.keys(c.fields).map(k => k.toLowerCase());
        if (keys.some(k => k.startsWith("temperature"))) return "temperature";
        if (keys.some(k => k.includes("humidity") || k.includes("moisture"))) return "humidity";
        if (keys.some(k => k.includes("lux") || k.includes("illuminance"))) return "light";
        if (keys.some(k => k.includes("power") || k.includes("energy"))) return "power";
        if (keys.some(k => k.includes("motion"))) return "motion";
        return "custom";
    }

    function pickField(c: DiscoveryCandidate, k: SensorKind): string {
        const keys = Object.keys(c.fields);
        const preferred = defaultField(k);
        if (keys.includes(preferred)) return preferred;
        const prefer = ["temperature_C", "temperature_F", "temperature", "humidity", "lux", "power_W"];
        for (const p of prefer) if (keys.includes(p)) return p;
        return keys[0] ?? "";
    }

    function fieldSummary(c: DiscoveryCandidate): string {
        const entries = Object.entries(c.fields);
        if (entries.length === 0) return "no numeric fields";
        return entries.slice(0, 4)
            .map(([k, v]) => `${k}=${Number.isInteger(v) ? v : v.toFixed(2)}`)
            .join(", ");
    }

    // ── MQTT ─────────────────────────────────────────────────────────────
    async function sendTestSignal() {
        if (publishing) return;
        const t = topic.trim();
        if (!t) { mqttResult = { ok: false, text: "Type the command topic first." }; return; }
        publishing = true;
        mqttResult = null;
        try {
            await api.mqttPublish({ topic: t, payload: "ON" });
            mqttResult = { ok: true, text: `Sent ON to ${t}. Did the device react?` };
        } catch (e) {
            mqttResult = { ok: false, text: `Publish failed: ${(e as Error).message}` };
        } finally {
            publishing = false;
        }
    }

    function acceptMqtt() {
        const t = topic.trim();
        if (!t) { mqttResult = { ok: false, text: "Type a topic first." }; return; }
        code = t;
        if (mqttRole === "socket") {
            plan = { socket: true, sensors: [] };
            foundSummary = `MQTT device on ${t}.`;
        } else {
            plan = { socket: false, sensors: [{ kind, unit, field, suffix: kind }] };
            foundSummary = `MQTT sensor on ${t}.`;
        }
        step = "details";
    }

    // ── Save ─────────────────────────────────────────────────────────────
    // One physical device can become a Socket, some Sensors, or both, so
    // this is the one place that writes them — and it writes whatever the
    // plan says, never what a screen assumed.
    async function save() {
        if (saving || !plan || !method) return;
        if (!name.trim()) { nameError = "Give the device a name so you can find it later."; return; }
        nameError = "";
        saving = true;
        try {
            if (plan.socket) {
                await api.createSocket({
                    name: name.trim(),
                    room: room.trim(),
                    code,
                    protocol: socketProtocol(method, transport ?? undefined),
                });
            }
            // Sensors carry the same code as the socket when a device is
            // both — the backend reads them off the same device.
            const sensorProtocol = method === "matter" ? "matter"
                                 : method === "mqtt"   ? "mqtt"
                                 : "rtl_433";
            for (const s of plan.sensors) {
                await api.createSensor({
                    name: sensorName(name, s, plan.sensors.length),
                    kind: s.kind,
                    unit: s.kind === kind && plan.sensors.length === 1 ? unit : s.unit,
                    code,
                    protocol: sensorProtocol,
                    field: s.kind === kind && plan.sensors.length === 1 ? field : s.field,
                    room: room.trim(),
                });
            }
            closeModal();
            await data.refresh();
        } catch (e) {
            toasts.error("Couldn't add the device", (e as Error).message);
        } finally {
            saving = false;
        }
    }

    const stepLabel = $derived(
        step === "method"   ? "Step 1 of 3 · What are you adding?"
      : step === "discover" ? "Step 2 of 3 · Finding it"
      :                       "Step 3 of 3 · Name it"
    );
</script>

{#snippet answerLine(result: { ok: boolean; text: string } | null)}
    {#if result}
        <p class="probe" class:bad={!result.ok} role="status">{result.text}</p>
    {/if}
{/snippet}

<Modal title="Add device" subtitle={stepLabel}>
    {#snippet body()}
        {#if step === "method"}
            {#if !probedMatter}
                <div class="skeleton-list" aria-hidden="true">
                    {#each [0, 1, 2, 3, 4] as i (i)}<div class="skeleton-row"></div>{/each}
                </div>
            {:else}
                <ul class="methods" role="list">
                    {#each offered as m (m.id)}
                        <li>
                            <button class="method" type="button" onclick={() => pick(m.id)}>
                                <span class="method-icon"><Icon name={m.icon} size={20} /></span>
                                <span class="method-main">
                                    <span class="method-title">{m.title}</span>
                                    <span class="method-blurb">{m.blurb}</span>
                                </span>
                                <Icon name="chevronRight" size={18} />
                            </button>
                        </li>
                    {/each}
                </ul>
                {#if !matterReady}
                    <p class="note-quiet">
                        Matter isn't set up on this server, so it isn't offered here.
                        Set <code>MATTER_BRIDGE_URL</code> to enable it.
                    </p>
                {/if}
            {/if}

        {:else if step === "discover" && method === "matter"}
            {#if commissioning}
                <div class="working">
                    <div class="working-title">Pairing with your device…</div>
                    <p class="working-hint">
                        This usually takes 30–60 seconds. The bridge talks to the device
                        over Bluetooth, hands it your
                        {#if transport === "thread"}Thread network credentials{:else}Wi-Fi credentials{/if},
                        and confirms it joined the network.
                    </p>
                    <div class="progress"><div class="bar" style:width="{Math.round(progress * 100)}%"></div></div>
                </div>
            {:else if commissionError}
                <div class="note bad">
                    <strong>Commissioning failed</strong>
                    <span>{commissionError}</span>
                    <span class="hint">
                        {#if isBlePhaseError(commissionError)}
                            Bluetooth found the device but couldn't connect before the
                            commissioning window closed — no network credentials were sent
                            yet. <strong>Re-open the commissioning window</strong> (short
                            button press, 1–5 s, see your device's manual), then try again.
                            A factory reset is also safe at this point.
                        {:else if transport === "thread"}
                            The device may have joined your Thread mesh but didn't complete
                            the fabric handshake. Its commissioning window has likely closed —
                            <strong>open it again</strong> with a short button press (1–5 s),
                            then try again. Other causes: MATTER_BRIDGE_THREAD_DATASET not
                            set, or the Thread Border Router not reachable.
                        {:else}
                            The device may have joined your Wi-Fi but didn't complete the
                            fabric handshake. Its commissioning window has likely closed —
                            <strong>open it again</strong> with a short button press (1–5 s),
                            then try again. Other causes: Wi-Fi credentials not configured on
                            the bridge, or Bluetooth not available.
                        {/if}
                    </span>
                </div>
            {:else}
                {#if matterTransports.length > 1}
                    <div class="field">
                        <span class="field-label">Network</span>
                        <div class="segmented" role="radiogroup" aria-label="Network type">
                            {#each matterTransports as t (t)}
                                <button class="seg" class:active={transport === t} type="button"
                                    role="radio" aria-checked={transport === t}
                                    onclick={() => transport = t}>
                                    {t === "thread" ? "Thread" : "Wi-Fi"}
                                </button>
                            {/each}
                        </div>
                        <div class="field-help">
                            Thread for low-power mesh devices (via your border router),
                            Wi-Fi for bulbs and plugs that connect directly.
                        </div>
                    </div>
                {/if}
                <div class="segmented" role="radiogroup" aria-label="How to enter the code">
                    <button class="seg" class:active={inputMode === "scan"} type="button"
                        role="radio" aria-checked={inputMode === "scan"}
                        onclick={() => { inputMode = "scan"; scannerError = null; }}>
                        <Icon name="qrcode" size={15} /> Scan
                    </button>
                    <button class="seg" class:active={inputMode === "manual"} type="button"
                        role="radio" aria-checked={inputMode === "manual"}
                        onclick={() => inputMode = "manual"}>
                        <Icon name="keyboard" size={15} /> Type it
                    </button>
                </div>
                {#if inputMode === "scan"}
                    {#if matterTransports.length > 1 && !transport}
                        <p class="field-help">Choose a network above before scanning.</p>
                    {:else}
                        <QRScanner onDecoded={onScanned} onError={(m: string) => scannerError = m} />
                        {#if scannerError}
                            <p class="field-help">
                                Camera didn't work?
                                <button class="link-btn" type="button" onclick={() => inputMode = "manual"}>
                                    Type the code instead
                                </button>
                            </p>
                        {:else}
                            <p class="field-help">
                                Scan the QR code on the device or its box — it starts with <code>MT:</code>.
                            </p>
                        {/if}
                    {/if}
                {:else}
                    <div class="field">
                        <label for="add-pair">Pairing code</label>
                        <input id="add-pair" type="text" inputmode="numeric" value={pairingCode}
                            oninput={onCodeInput} placeholder="3496-112-0001"
                            autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck={false}
                            aria-invalid={codeError ? "true" : undefined}
                            aria-describedby={codeError ? "add-pair-err" : undefined} />
                        {#if codeError}<div id="add-pair-err" class="field-error">{codeError}</div>{/if}
                        <div class="field-help">
                            The 11-digit code printed on the device (dashes are added for you),
                            or the full <code>MT:…</code> payload.
                        </div>
                    </div>
                {/if}
            {/if}

        {:else if step === "discover" && method === "tasmota"}
            {#if sweeping}
                <div class="working">
                    <div class="working-title">Searching your network…</div>
                    <p class="working-hint">
                        Checking every address on your local network for devices answering
                        the Tasmota API. This takes a few seconds.
                    </p>
                </div>
                <div class="skeleton-list" aria-hidden="true">
                    {#each [0, 1, 2] as i (i)}<div class="skeleton-row"></div>{/each}
                </div>
            {:else if sweepError}
                <div class="note bad">
                    <strong>Search failed</strong>
                    <span>{sweepError}</span>
                </div>
            {:else if found.length === 0 && sweepDone}
                <div class="note">
                    <strong>Nothing found</strong>
                    <span>
                        No devices on your network answered the Tasmota API. Check the device
                        is powered on and joined to the same Wi-Fi, then search again.
                    </span>
                </div>
            {:else}
                <ul class="found" role="list">
                    {#each found as dev (dev.ip)}
                        <li>
                            <button class="found-row" type="button" onclick={() => adoptTasmota(dev)}>
                                <span class="found-main">
                                    <span class="found-name">{dev.name || "Tasmota device"}</span>
                                    <span class="found-sub mono">{dev.ip}{dev.topic ? ` · ${dev.topic}` : ""}</span>
                                </span>
                                <Icon name="chevronRight" size={18} />
                            </button>
                        </li>
                    {/each}
                </ul>
            {/if}

        {:else if step === "discover" && method === "rf-socket"}
            <p class="lead">
                Long-press the button on your socket until its indicator flashes, then
                tap Pair. We'll pick a code and broadcast it for the socket to learn.
            </p>
            <button class="btn btn-secondary" type="button" onclick={pairRf} disabled={pairing}>
                {pairing ? "Sending…" : rfCode ? "Send again" : "Pair with socket"}
            </button>
            {@render answerLine(rfResult)}
            {#if rfCode}
                <div class="field">
                    <span class="field-label">Code</span>
                    <p class="mono code-line">{rfCode}</p>
                </div>
            {/if}

        {:else if step === "discover" && method === "rf-sensor"}
            <p class="lead">
                Trigger your sensor — press its button, walk in front of it, or wait for
                it to report. Anything we hear that isn't already paired shows up below.
            </p>
            {#if newCandidates.length === 0}
                <div class="waiting">
                    {#if listening}
                        <span class="pulse" aria-hidden="true"></span>
                        Listening — <span class="mono">{listenRemaining}s</span> left
                    {:else}
                        Nothing heard yet.
                    {/if}
                </div>
            {:else}
                <ul class="found" role="list">
                    {#each newCandidates as c (c.code)}
                        <li>
                            <button class="found-row" type="button" onclick={() => adoptCandidate(c)}>
                                <span class="found-main">
                                    <span class="found-name mono">{c.code}</span>
                                    <span class="found-sub">{fieldSummary(c)}</span>
                                </span>
                                <span class="count mono">{c.count}×</span>
                                <Icon name="chevronRight" size={18} />
                            </button>
                        </li>
                    {/each}
                </ul>
            {/if}

        {:else if step === "discover" && method === "mqtt"}
            <div class="field">
                <label for="add-topic">Topic</label>
                <input id="add-topic" type="text" bind:value={topic}
                    placeholder="cmnd/plug/POWER" autocomplete="off" spellcheck={false} />
                <div class="field-help">
                    For a switch, the command topic we publish <code>ON</code>/<code>OFF</code> to.
                    For a sensor, the topic to subscribe to (wildcards <code>+</code> and
                    <code>#</code> allowed).
                </div>
            </div>
            <div class="field">
                <span class="field-label">What does it do?</span>
                <div class="segmented" role="radiogroup" aria-label="Device role">
                    <button class="seg" class:active={mqttRole === "socket"} type="button"
                        role="radio" aria-checked={mqttRole === "socket"}
                        onclick={() => mqttRole = "socket"}>Switches on and off</button>
                    <button class="seg" class:active={mqttRole === "sensor"} type="button"
                        role="radio" aria-checked={mqttRole === "sensor"}
                        onclick={() => mqttRole = "sensor"}>Reports a value</button>
                </div>
            </div>
            {#if mqttRole === "socket"}
                <button class="btn btn-secondary" type="button" onclick={sendTestSignal} disabled={publishing}>
                    {publishing ? "Sending…" : "Send test signal"}
                </button>
                {@render answerLine(mqttResult)}
            {/if}

        {:else if step === "details"}
            {#if foundSummary}
                <div class="note good">
                    <strong>Found it</strong>
                    <span>{foundSummary}</span>
                </div>
            {/if}
            <div class="field">
                <label for="add-name">Name</label>
                <input id="add-name" type="text" bind:value={name}
                    placeholder={suggestedName || "e.g. Living room lamp"}
                    autocomplete="off" required
                    aria-invalid={nameError ? "true" : undefined}
                    aria-describedby={nameError ? "add-name-err" : undefined}
                    oninput={() => nameError = ""} />
                {#if nameError}<div id="add-name-err" class="field-error">{nameError}</div>{/if}
                {#if plan && plan.sensors.length > 1}
                    <div class="field-help">
                        This device reports {plan.sensors.length} measurements, so it becomes
                        {plan.sensors.length} sensors — “{name.trim() || "Name"} {plan.sensors[0].suffix}”
                        and “{name.trim() || "Name"} {plan.sensors[1].suffix}”.
                    </div>
                {/if}
            </div>
            <div class="field">
                <label for="add-room">Room <span class="opt">(optional)</span></label>
                <select id="add-room" bind:value={room}>
                    <option value="">No room</option>
                    {#each [...data.value.rooms].sort((a, b) => a.name.localeCompare(b.name)) as r (r.id)}
                        <option value={r.name}>{r.name}</option>
                    {/each}
                </select>
            </div>

            <!-- Only a sensor whose kind we had to guess asks about it. A
                 Matter device told us, so it never appears there. -->
            {#if plan && !plan.socket && plan.sensors.length === 1 && method !== "matter"}
                <div class="field-row">
                    <div class="field">
                        <label for="add-kind">Measures</label>
                        <select id="add-kind" bind:value={kind}>
                            <option value="temperature">Temperature</option>
                            <option value="humidity">Humidity</option>
                            <option value="motion">Motion</option>
                            <option value="light">Light</option>
                            <option value="power">Power</option>
                            <option value="custom">Something else</option>
                        </select>
                    </div>
                    <div class="field">
                        <label for="add-unit">Unit</label>
                        <input id="add-unit" type="text" bind:value={unit} placeholder="°C" />
                    </div>
                </div>
                {#if method !== "mqtt"}
                    <div class="field">
                        <label for="add-field">Reading</label>
                        <input id="add-field" type="text" bind:value={field} placeholder="temperature_C" />
                        <div class="field-help">
                            Which value in the sensor's report to chart. We picked this from
                            what it sent; change it only if you're charting the wrong number.
                        </div>
                    </div>
                {/if}
            {/if}
        {/if}
    {/snippet}

    {#snippet actions()}
        {#if step === "method"}
            <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        {:else if step === "discover"}
            <button class="btn btn-ghost" onclick={back}>Back</button>
            {#if method === "matter" && commissionError}
                <button class="btn btn-primary" onclick={() => { commissionError = null; }}>Try again</button>
            {:else if method === "matter" && inputMode === "manual" && !commissioning}
                <button class="btn btn-primary" onclick={commission}
                    disabled={matterTransports.length > 1 && !transport}>Pair device</button>
            {:else if method === "tasmota" && !sweeping}
                <button class="btn btn-primary" onclick={sweep}>Search again</button>
            {:else if method === "rf-socket"}
                <button class="btn btn-primary" onclick={acceptRfSocket} disabled={!rfCode}>Next</button>
            {:else if method === "rf-sensor" && !listening}
                <button class="btn btn-primary" onclick={startListening}>Listen again</button>
            {:else if method === "mqtt"}
                <button class="btn btn-primary" onclick={acceptMqtt}>Next</button>
            {/if}
        {:else}
            <button class="btn btn-ghost" onclick={back}>Back</button>
            <button class="btn btn-primary" onclick={save} disabled={saving}>
                {saving ? "Adding…" : "Add device"}
            </button>
        {/if}
    {/snippet}
</Modal>

<style>
    .lead { font-size: 13.5px; color: var(--text-muted); line-height: 1.5; }
    .opt { color: var(--text-muted); font-weight: 400; font-size: 12px; }
    .mono { font-family: var(--font-mono); font-feature-settings: "tnum" 1; }

    /* ── Step 1: the method list ── */
    .methods, .found { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }

    .method, .found-row {
        width: 100%;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-3);
        cursor: pointer;
        text-align: left;
        color: inherit;
        min-height: 60px;
        transition: background var(--t-fast), border-color var(--t-fast);
    }
    @media (hover: hover) {
        .method:hover, .found-row:hover { background: var(--surface-hover); border-color: var(--primary); }
    }
    .method:active, .found-row:active { transform: scale(0.99); transition: transform 60ms ease; }

    .method-icon {
        display: grid;
        place-items: center;
        width: 36px; height: 36px;
        flex-shrink: 0;
        border-radius: var(--radius-sm);
        background: var(--bg-elevated);
        color: var(--primary);
    }
    .method-main, .found-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
    .method-title, .found-name { font-size: 14px; font-weight: 600; }
    .method-blurb, .found-sub {
        font-size: 12.5px; color: var(--text-muted); line-height: 1.4;
    }
    .found-sub { word-break: break-all; }
    .count {
        font-size: 11px; color: var(--text-muted);
        background: var(--bg-elevated);
        border-radius: 999px; padding: 2px 8px;
        flex-shrink: 0;
    }

    /* ── Segmented pickers ── */
    .segmented { display: flex; gap: var(--space-1); background: var(--surface); border-radius: var(--radius-md); padding: 3px; }
    .seg {
        flex: 1;
        display: flex; align-items: center; justify-content: center; gap: 6px;
        padding: 9px var(--space-2);
        border: 0; border-radius: calc(var(--radius-md) - 3px);
        background: transparent; color: var(--text-muted);
        font-size: 13px; font-weight: 500; cursor: pointer;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .seg.active { background: var(--bg-elevated); color: var(--text); }
    @media (pointer: coarse) { .seg { min-height: 44px; } }

    /* ── Working / progress ── */
    .working { display: flex; flex-direction: column; gap: var(--space-2); }
    .working-title { font-size: 14px; font-weight: 600; }
    .working-hint { font-size: 13px; color: var(--text-muted); line-height: 1.5; }
    .progress {
        height: 6px; border-radius: 999px;
        background: var(--surface); overflow: hidden;
    }
    .progress .bar {
        height: 100%; background: var(--primary);
        border-radius: 999px;
        transition: width 700ms linear;
    }

    /* ── Skeletons (DESIGN.md §10: never a spinner) ── */
    .skeleton-list { display: flex; flex-direction: column; gap: var(--space-2); }
    .skeleton-row {
        height: 60px; border-radius: var(--radius-md);
        background: linear-gradient(90deg, var(--surface) 25%, var(--surface-hover) 50%, var(--surface) 75%);
        background-size: 200% 100%;
        animation: shimmer 1.4s ease-in-out infinite;
    }
    @keyframes shimmer {
        0% { background-position: 200% 0; }
        100% { background-position: -200% 0; }
    }

    /* ── Listening state ── */
    .waiting {
        display: flex; align-items: center; justify-content: center; gap: var(--space-2);
        color: var(--text-muted); font-size: 13.5px;
        padding: var(--space-5) 0;
    }
    .pulse {
        width: 10px; height: 10px; border-radius: 50%;
        background: var(--primary);
        animation: pulse 1.2s ease-in-out infinite;
    }
    @keyframes pulse {
        0%, 100% { opacity: 0.35; transform: scale(0.9); }
        50%      { opacity: 1;    transform: scale(1.15); }
    }

    /* ── Notes ── */
    .note {
        display: flex; flex-direction: column; gap: 4px;
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-3);
        font-size: 13px;
    }
    .note strong { font-size: 13.5px; }
    .note span { color: var(--text-muted); line-height: 1.5; }
    .note .hint { font-size: 12.5px; }
    .note.good { border-color: var(--good); }
    .note.bad { border-color: var(--danger); }
    .note-quiet { font-size: 12.5px; color: var(--text-faint); line-height: 1.5; }

    .probe { margin: 6px 0 0; font-size: 13px; line-height: 1.5; color: var(--good); }
    .probe.bad { color: var(--danger); }

    .code-line { font-size: 15px; font-weight: 600; }

    .link-btn {
        background: none; border: 0; padding: 0;
        color: var(--primary); font-size: inherit; cursor: pointer;
        text-decoration: underline;
    }

    @media (prefers-reduced-motion: reduce) {
        .skeleton-row, .pulse { animation-duration: 0.001ms; }
        .progress .bar { transition-duration: 0.001ms; }
    }
</style>

<!--
  QR code scanner with two paths:

  1. Live scan via getUserMedia + jsQR — preferred when the page is in a
     secure context (HTTPS or localhost). Tapping "Open camera" runs
     getUserMedia synchronously inside the click handler so iOS allows
     the subsequent video.play() to start the preview.

  2. Photo capture via <input type="file" capture="environment"> — used
     when getUserMedia is unavailable (HTTP from a phone, or permission
     denied). The OS camera app opens, the user takes one shot, jsQR
     decodes the still image. Slightly clunkier but works everywhere.
-->
<script lang="ts">
    import jsQR from "jsqr";
    import { onDestroy } from "svelte";
    import Icon from "./Icon.svelte";

    interface Props {
        onDecoded: (text: string) => void;
        onError?: (message: string) => void;
    }
    let { onDecoded, onError }: Props = $props();

    let video: HTMLVideoElement | undefined = $state();
    let fileInput: HTMLInputElement | undefined = $state();
    let stream: MediaStream | null = null;
    let scanTimer: ReturnType<typeof setInterval> | null = null;
    let liveCanvas: HTMLCanvasElement | undefined;

    type Mode =
        | "idle"        // showing the picker buttons
        | "starting"    // requesting camera permission
        | "scanning"    // live camera preview running
        | "decoding"    // crunching a captured photo
        | "error";      // unrecoverable; user can pick another path
    let mode    = $state<Mode>("idle");
    let errorMsg = $state("");
    let stopped = false;

    // Live scanning needs a secure context. createImageBitmap (used by the
    // photo path) does not — that path works on plain HTTP too.
    const canLive = $derived(
        typeof window !== "undefined" &&
        window.isSecureContext &&
        !!navigator.mediaDevices?.getUserMedia,
    );

    onDestroy(() => stopLive());

    async function openCamera() {
        if (mode === "starting" || mode === "scanning") return;
        mode = "starting";
        errorMsg = "";

        if (!navigator.mediaDevices?.getUserMedia) {
            showError("Live camera needs HTTPS. Use the photo option below instead.");
            return;
        }
        try {
            stream = await navigator.mediaDevices.getUserMedia({
                video: { facingMode: { ideal: "environment" } },
                audio: false,
            });
        } catch (e) {
            const err = e as Error;
            showError(
                err.name === "NotAllowedError"
                    ? "Camera permission denied. Try the photo option below, or allow camera access in your browser settings."
                    : err.name === "NotFoundError"
                        ? "No camera found on this device."
                        : err.message || "Could not open the camera."
            );
            return;
        }
        if (stopped) {
            stream.getTracks().forEach((t) => t.stop());
            return;
        }
        if (video) {
            video.srcObject = stream;
            try { await video.play(); } catch { /* autoplay attribute covers it */ }
        }
        liveCanvas = document.createElement("canvas");
        mode = "scanning";
        scanTimer = setInterval(scanLiveFrame, 120);
    }

    function showError(msg: string) {
        mode = "error";
        errorMsg = msg;
        onError?.(msg);
    }

    function stopLive() {
        stopped = true;
        if (scanTimer) { clearInterval(scanTimer); scanTimer = null; }
        if (stream) { stream.getTracks().forEach((t) => t.stop()); stream = null; }
        if (video) video.srcObject = null;
    }

    function scanLiveFrame() {
        if (!video || !liveCanvas) return;
        if (video.readyState !== video.HAVE_ENOUGH_DATA) return;
        const targetW = 480;
        const scale = Math.min(1, targetW / video.videoWidth);
        const w = Math.round(video.videoWidth * scale);
        const h = Math.round(video.videoHeight * scale);
        if (w === 0 || h === 0) return;
        liveCanvas.width = w; liveCanvas.height = h;
        const ctx = liveCanvas.getContext("2d", { willReadFrequently: true });
        if (!ctx) return;
        ctx.drawImage(video, 0, 0, w, h);
        const img = ctx.getImageData(0, 0, w, h);
        const code = jsQR(img.data, img.width, img.height, { inversionAttempts: "dontInvert" });
        if (code?.data) { stopLive(); onDecoded(code.data); }
    }

    function pickPhoto() {
        fileInput?.click();
    }

    async function onPhotoChosen(e: Event) {
        const input = e.target as HTMLInputElement;
        const file = input.files?.[0];
        // Reset so picking the same file twice fires onchange again.
        input.value = "";
        if (!file) return;

        mode = "decoding";
        errorMsg = "";
        try {
            const text = await decodePhoto(file);
            if (text) { onDecoded(text); return; }
            showError("No QR code found in that photo. Try again — get closer, hold steady, make sure the whole code is in frame.");
        } catch (e) {
            showError((e as Error).message || "Could not read that image.");
        }
    }

    // Decode a single still image. Downsamples large camera photos so jsQR
    // stays fast (a 12 MP iPhone shot would otherwise pin the main thread).
    // Tries both "attempt inversion" passes since print contrast varies.
    async function decodePhoto(file: File): Promise<string | null> {
        const bitmap = await createBitmap(file);
        const maxDim = 1024;
        const scale = Math.min(1, maxDim / Math.max(bitmap.width, bitmap.height));
        const w = Math.max(1, Math.round(bitmap.width * scale));
        const h = Math.max(1, Math.round(bitmap.height * scale));
        const c = document.createElement("canvas");
        c.width = w; c.height = h;
        const ctx = c.getContext("2d", { willReadFrequently: true });
        if (!ctx) throw new Error("canvas unavailable");
        ctx.drawImage(bitmap as CanvasImageSource, 0, 0, w, h);
        if ("close" in bitmap) (bitmap as ImageBitmap).close();
        const data = ctx.getImageData(0, 0, w, h);
        const result =
            jsQR(data.data, data.width, data.height, { inversionAttempts: "attemptBoth" });
        return result?.data || null;
    }

    // Prefer createImageBitmap when available (it decodes off the main
    // thread on modern browsers); fall back to <img> + onload otherwise.
    async function createBitmap(file: File): Promise<ImageBitmap | HTMLImageElement> {
        if (typeof createImageBitmap === "function") {
            try { return await createImageBitmap(file); } catch { /* fall through */ }
        }
        return new Promise<HTMLImageElement>((resolve, reject) => {
            const url = URL.createObjectURL(file);
            const img = new Image();
            img.onload = () => { URL.revokeObjectURL(url); resolve(img); };
            img.onerror = () => { URL.revokeObjectURL(url); reject(new Error("Could not load image")); };
            img.src = url;
        });
    }

    function reset() {
        mode = "idle";
        errorMsg = "";
    }
</script>

{#if mode === "idle"}
    <div class="picker">
        {#if canLive}
            <button class="action-btn primary" onclick={openCamera}>
                <Icon name="qrcode" size={28} />
                <span>Scan with camera</span>
                <span class="sub">Live preview</span>
            </button>
            <button class="action-btn" onclick={pickPhoto}>
                <Icon name="qrcode" size={22} />
                <span>Take a photo instead</span>
            </button>
        {:else}
            <button class="action-btn primary" onclick={pickPhoto}>
                <Icon name="qrcode" size={28} />
                <span>Take a photo of the QR code</span>
                <span class="sub">Live scanning needs HTTPS — this works anywhere</span>
            </button>
        {/if}
    </div>
{:else if mode === "starting"}
    <div class="scanner placeholder">
        <div class="spinner"></div>
        <span class="hint">Starting camera…</span>
    </div>
{:else if mode === "decoding"}
    <div class="scanner placeholder">
        <div class="spinner"></div>
        <span class="hint">Reading the photo…</span>
    </div>
{:else if mode === "error"}
    <div class="error-box">
        <strong>Couldn't read a QR code</strong>
        <span>{errorMsg}</span>
        <button class="link-btn" onclick={reset}>Try again</button>
    </div>
{:else}
    <div class="scanner">
        <video bind:this={video} muted autoplay playsinline></video>
        <div class="overlay"><div class="reticle"></div></div>
        <div class="hint">Point at the QR code on the device or its box</div>
    </div>
{/if}

<!-- Hidden file input drives the photo path. capture="environment" tells
     mobile browsers to open the rear camera straight into capture mode. -->
<input
    bind:this={fileInput}
    type="file"
    accept="image/*"
    capture="environment"
    onchange={onPhotoChosen}
    style="display:none" />

<style>
    .picker {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .action-btn {
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: 6px;
        width: 100%;
        padding: var(--space-5) var(--space-4);
        border: 2px dashed var(--border);
        border-radius: var(--radius-md);
        background: var(--surface);
        color: var(--text-muted);
        font-size: 14px;
        font-weight: 500;
        cursor: pointer;
        transition: border-color 0.15s, color 0.15s, background 0.15s;
    }
    .action-btn.primary {
        border-style: solid;
        border-color: var(--primary);
        color: var(--primary);
        background: var(--bg-elevated);
        font-size: 15px;
        padding: var(--space-6) var(--space-4);
    }
    .action-btn:hover, .action-btn:focus-visible {
        border-color: var(--primary);
        color: var(--primary);
    }
    .action-btn .sub {
        font-size: 12px;
        color: var(--text-muted);
        font-weight: 400;
    }

    .scanner {
        position: relative;
        width: 100%;
        aspect-ratio: 4 / 3;
        background: #000;
        border-radius: var(--radius-md);
        overflow: hidden;
        border: 1px solid var(--border);
    }
    .scanner.placeholder {
        display: flex; flex-direction: column;
        align-items: center; justify-content: center;
        gap: var(--space-3);
        background: var(--surface);
    }
    video {
        width: 100%; height: 100%;
        object-fit: cover;
        display: block;
    }
    .overlay {
        position: absolute; inset: 0;
        display: grid; place-items: center;
        pointer-events: none;
    }
    .reticle {
        width: 60%; max-width: 240px;
        aspect-ratio: 1;
        border: 2px solid rgba(255,255,255,0.85);
        border-radius: var(--radius-md);
        box-shadow: 0 0 0 9999px rgba(0,0,0,0.30);
    }
    .hint {
        position: absolute;
        left: 0; right: 0; bottom: var(--space-3);
        text-align: center; color: #fff;
        font-size: 13px;
        text-shadow: 0 1px 2px rgba(0,0,0,0.6);
        pointer-events: none;
    }
    .scanner.placeholder .hint {
        position: static;
        color: var(--text-muted);
        text-shadow: none;
    }

    .spinner {
        width: 28px; height: 28px;
        border: 3px solid var(--border);
        border-top-color: var(--primary);
        border-radius: 50%;
        animation: spin 0.9s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }

    .error-box {
        display: flex; flex-direction: column; gap: 6px;
        padding: var(--space-3) var(--space-4);
        border: 1px solid var(--danger);
        border-radius: var(--radius-md);
        font-size: 13px;
        color: var(--danger);
    }
    .error-box strong { font-weight: 600; }
    .link-btn {
        align-self: flex-start;
        background: none; border: none; padding: 0;
        color: var(--primary);
        font-size: 13px;
        cursor: pointer;
        text-decoration: underline;
        text-underline-offset: 2px;
    }
</style>

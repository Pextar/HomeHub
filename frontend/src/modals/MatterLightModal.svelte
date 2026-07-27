<!--
  Smart-light control for a commissioned Matter device.

  Everything visual lives in SmartLightControl; this only adapts the
  matter-bridge's state shape to the shared LightSnapshot/LightUpdate and
  contributes the Matter-specific metadata (vendor/product, node id and
  reachability, which Tasmota has no equivalent of).
-->
<script lang="ts">
    import SmartLightControl from "../components/SmartLightControl.svelte";
    import { api } from "../lib/api";
    import type { Socket, MatterStateUpdate } from "../lib/types";
    import type { LightSnapshot, LightUpdate } from "../lib/light";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    // Kept so the meta column can show what the bridge reported. The control
    // surface itself works off the snapshot returned by load().
    let vendor = $state<string | undefined>();
    let product = $state<string | undefined>();
    let reachable = $state(true);

    async function load(): Promise<LightSnapshot> {
        const s = await api.matterGetState(socket.id);
        vendor = s.vendor;
        product = s.product;
        reachable = s.reachable;
        // Matter names the brightness channel "level", which is already the
        // neutral name, so the mapping is a straight pick of the four knobs.
        return { on: s.on ?? false, level: s.level, color: s.color, ct: s.ct };
    }

    function save(u: LightUpdate): Promise<void> {
        const update: MatterStateUpdate = {
            on: u.on, level: u.level, color: u.color, ct: u.ct,
        };
        return api.matterSetState(socket.id, update);
    }
</script>

<SmartLightControl {socket} {load} {save}>
    {#snippet meta()}
        {#if vendor || product}
            <div class="device-name">{[vendor, product].filter(Boolean).join(" ")}</div>
        {/if}
        <div class="device-id">Node {socket.code}</div>
        {#if !reachable}
            <div class="hint warn">Unreachable</div>
        {/if}
    {/snippet}
</SmartLightControl>

<style>
    /* Snippet content is scoped to where it is declared, so the meta column's
       styles live here rather than in SmartLightControl. */
    .device-name { font-size: 14px; font-weight: 600; color: var(--text); }
    .device-id { font-size: 12px; color: var(--text-muted); font-family: var(--font-mono); }
    .hint { font-size: 12px; color: var(--text-muted); }
    .hint.warn { color: var(--warn); }
</style>

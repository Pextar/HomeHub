<!--
  Smart-light control for a Tasmota Wi-Fi device.

  Everything visual lives in SmartLightControl; this only adapts Tasmota's
  state shape to the shared LightSnapshot/LightUpdate — chiefly renaming its
  "dimmer" channel to the neutral "level" — and shows the device address.
-->
<script lang="ts">
    import SmartLightControl from "../components/SmartLightControl.svelte";
    import { api } from "../lib/api";
    import type { Socket } from "../lib/types";
    import type { LightSnapshot, LightUpdate } from "../lib/light";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    async function load(): Promise<LightSnapshot> {
        const s = await api.tasmotaGetState(socket.id);
        return { on: s.on, level: s.dimmer, color: s.color, ct: s.ct };
    }

    function save(u: LightUpdate): Promise<void> {
        return api.tasmotaSetState(socket.id, {
            on: u.on, dimmer: u.level, color: u.color, ct: u.ct,
        });
    }
</script>

<SmartLightControl {socket} {load} {save}>
    {#snippet meta()}
        <div class="device-ip">{socket.code}</div>
    {/snippet}
</SmartLightControl>

<style>
    /* Snippet content is scoped to where it is declared, so the meta column's
       styles live here rather than in SmartLightControl. */
    .device-ip { font-size: 12px; color: var(--text-muted); font-family: var(--font-mono); }
</style>

<script lang="ts">
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import type { RoomSummary } from "../lib/types";

    interface Props { room: RoomSummary; }
    let { room }: Props = $props();
</script>

<div class="room">
    <div class="name">{room.name}</div>
    <div class="meta">{room.sockets} socket{room.sockets === 1 ? "" : "s"} · {room.on} on</div>
    <div class="actions">
        <button class="btn btn-success"
            onclick={() => runAction(() => api.roomOn(room.name), `${room.name} on`)}>On</button>
        <button class="btn btn-danger"
            onclick={() => runAction(() => api.roomOff(room.name), `${room.name} off`)}>Off</button>
    </div>
</div>

<style>
    .room {
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .name { font-weight: 600; }
    .meta { color: var(--text-muted); font-size: 12px; }
    .actions {
        display: flex;
        gap: var(--space-2);
        margin-top: var(--space-2);
    }
    .actions :global(.btn) { flex: 1; }
</style>

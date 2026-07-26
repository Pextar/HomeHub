<script lang="ts">
    /**
     * One fader row: a leading control, a name, the slider, and the level.
     *
     * Three of them across the two players — a KEF speaker's, a Sonos group's
     * "All rooms", and one per member of that group — which differ only in
     * what leads the row and whether it can be removed from the group.
     *
     * `mute` present makes the leading control a mute toggle; absent leaves a
     * plain icon, for the group fader that has nothing to mute on its own.
     */
    import Icon from "../Icon.svelte";
    import Slider from "./Slider.svelte";

    let {
        name,
        value,
        label,
        mute = undefined,
        /** Removes this speaker from the group. Only on a grouped member. */
        onRemove = undefined,
        removeBusy = false,
        onInput,
        onChange,
    }: {
        name: string;
        value: number;
        label: string;
        mute?: { muted: boolean; busy: boolean; onToggle: () => void };
        onRemove?: () => void;
        removeBusy?: boolean;
        onInput: (v: number) => void;
        onChange: (v: number) => void;
    } = $props();
</script>

<div class="member">
    {#if mute}
        <button
            class="icon-btn m-mute"
            aria-label={mute.muted ? `Unmute ${name}` : `Mute ${name}`}
            aria-pressed={mute.muted}
            disabled={mute.busy}
            onclick={mute.onToggle}
        >
            <Icon name={mute.muted ? "volumeOff" : "volume"} size={16} />
        </button>
    {:else}
        <span class="m-icon" aria-hidden="true"><Icon name="volume" size={16} /></span>
    {/if}
    <span class="m-name" class:muted={mute?.muted}>{name}</span>
    <Slider {value} {label} {onInput} {onChange} />
    <span class="vol-num mono">{value}</span>
    {#if onRemove}
        <button
            class="icon-btn m-act"
            aria-label="Remove {name} from group"
            disabled={removeBusy}
            onclick={onRemove}
        >
            <Icon name="close" size={14} />
        </button>
    {/if}
</div>

<style>
    .member { display: flex; align-items: center; gap: var(--space-3); min-height: 44px; }
    .m-name {
        font-size: 13.5px; font-weight: 500; width: 110px; flex-shrink: 0;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .m-name.muted { color: var(--text-dim); }
    .m-mute, .m-act { width: 36px; height: 36px; flex-shrink: 0; }
    .m-icon {
        width: 36px; height: 36px; flex-shrink: 0;
        display: grid; place-items: center; color: var(--text-mute);
    }
    .vol-num {
        font-size: 12px; font-feature-settings: "tnum" 1;
        color: var(--text-mute); width: 3ch; text-align: right; flex-shrink: 0;
    }
    @media (pointer: coarse) {
        .m-mute, .m-act, .m-icon { width: 44px; height: 44px; }
        .m-name { width: 90px; }
    }
</style>

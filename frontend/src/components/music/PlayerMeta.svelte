<script lang="ts">
    /**
     * Title and subline under the art. Centred, and the title truncates rather
     * than wrapping — the art is the hero here, not the text.
     *
     * `idle` dims the title, for the two states that aren't a track: an empty
     * Sonos zone and a KEF speaker in standby.
     */
    let {
        title,
        sub,
        idle = false,
        /** The desktop stage's reading size: left-aligned, display-sized. */
        large = false,
    }: {
        title: string;
        sub: string;
        idle?: boolean;
        large?: boolean;
    } = $props();
</script>

<div class="p-meta" class:large>
    <div class="p-title" class:idle>{title}</div>
    <div class="p-sub">{sub}</div>
</div>

<style>
    .p-meta {
        text-align: center;
        display: flex;
        flex-direction: column;
        gap: 4px;
    }
    .p-title {
        font-size: 22px;
        font-weight: 600;
        letter-spacing: -0.02em;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .p-title.idle {
        color: var(--text-mute);
    }
    .p-sub {
        font-size: 14px;
        color: var(--text-mute);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .p-meta.large {
        text-align: left;
        gap: 6px;
    }
    .p-meta.large .p-title {
        font-size: 30px;
        letter-spacing: -0.03em;
    }
    .p-meta.large .p-sub {
        font-size: 15px;
    }
</style>
